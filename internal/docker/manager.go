// Package docker provides Docker container management for test services.
package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	imagetypes "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"chiperka-cli/internal/artifact"
	"chiperka-cli/internal/events"
	"chiperka-cli/internal/model"
)

// Manager handles Docker container lifecycle for test services.
// Each test gets its own isolated network for parallel execution.
type Manager struct {
	// networkID is the isolated network for this test
	networkID string
	// networkName is the human-readable network name
	networkName string
	// runningContainers tracks containers started by this manager for cleanup
	runningContainers map[string]string // service name -> container ID
	// containersMu protects runningContainers
	containersMu sync.Mutex
	// events is the event emitter for this manager
	events *events.Emitter
	// testName is the name of the test this manager belongs to
	testName string
	// pool is the network pool for reusing networks (nil if not using pool)
	pool *NetworkPool
	// serviceArtifactDefs stores artifact definitions per service name
	serviceArtifactDefs map[string][]model.ServiceArtifact
}

// dockerClient is a shared Docker SDK client for API calls.
// Uses client.FromEnv to respect DOCKER_HOST, DOCKER_TLS_VERIFY, etc.
var dockerClient *dockerclient.Client
var dockerClientErr error

func init() {
	dockerClient, dockerClientErr = dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
}

const (
	maxServiceAttempts    = 2
	serviceRetryDelay    = 2 * time.Second
	dockerRetryAttempts  = 3
	dockerRetryBaseDelay = 500 * time.Millisecond
)

// retryDockerCall retries a Docker API call with exponential backoff.
// Only retries on transient errors (timeouts, connection resets), not on
// logical errors (container not found, etc.).
func retryDockerCall[T any](ctx context.Context, operation string, emitter *events.Emitter, testName string, fn func() (T, error)) (T, error) {
	var lastErr error
	for attempt := 1; attempt <= dockerRetryAttempts; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}
		if !isTransientDockerError(err) || ctx.Err() != nil {
			return result, err
		}
		lastErr = err
		if attempt < dockerRetryAttempts {
			delay := dockerRetryBaseDelay * time.Duration(1<<(attempt-1)) // 500ms, 1s
			if emitter != nil {
				emitter.Warn(events.Fields{
					"test":    testName,
					"action":  "docker_retry",
					"attempt": fmt.Sprintf("%d/%d", attempt, dockerRetryAttempts),
					"msg":     fmt.Sprintf("Docker API call %s failed, retrying in %s: %v", operation, delay, err),
				})
			}
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return result, ctx.Err()
			}
		}
	}
	var zero T
	return zero, lastErr
}

// retryDockerCallNoResult is like retryDockerCall but for functions that return only error.
func retryDockerCallNoResult(ctx context.Context, operation string, emitter *events.Emitter, testName string, fn func() error) error {
	_, err := retryDockerCall(ctx, operation, emitter, testName, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

func isTransientDockerError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "TLS handshake timeout") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection timed out") ||
		strings.Contains(msg, "server misbehaving")
}

// containerSem limits concurrent ContainerCreate+ContainerStart calls across all
// Manager instances to prevent overwhelming the Docker daemon.
var containerSem chan struct{}

// SetMaxConcurrentContainers limits how many Docker containers can be
// created simultaneously across all managers. Default (0) means unlimited.
func SetMaxConcurrentContainers(n int) {
	if n > 0 {
		containerSem = make(chan struct{}, n)
	}
}

func acquireContainerSlot(ctx context.Context) error {
	if containerSem == nil {
		return nil
	}
	select {
	case containerSem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func releaseContainerSlot() {
	if containerSem != nil {
		<-containerSem
	}
}

// NewManager creates a new Docker manager with an isolated network.
func NewManager(emitter *events.Emitter, testName string) (*Manager, error) {
	if dockerClientErr != nil {
		return nil, fmt.Errorf("docker client init failed: %w", dockerClientErr)
	}
	// Verify docker is available
	if _, err := dockerClient.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("docker not available: %w", err)
	}

	// Create unique network name using random bytes
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random network name: %w", err)
	}
	networkName := fmt.Sprintf("chiperka-%s", hex.EncodeToString(randomBytes))

	emitter.Info(events.Fields{
		"test":    testName,
		"network": networkName,
		"action":  "network_create",
		"msg":     "Creating isolated Docker network",
	})

	// Create isolated network with retry on pool exhaustion
	networkID, err := createNetworkWithRetry(networkName, emitter)
	if err != nil {
		return nil, fmt.Errorf("failed to create network: %w", err)
	}

	m := &Manager{
		networkID:          networkID,
		networkName:        networkName,
		runningContainers: make(map[string]string),
		events:            emitter,
		testName:          testName,
		pool:              nil,
	}

	return m, nil
}

// NewManagerWithPool creates a new Docker manager using a network from the pool.
func NewManagerWithPool(emitter *events.Emitter, testName string, pool *NetworkPool) (*Manager, error) {
	if dockerClientErr != nil {
		return nil, fmt.Errorf("docker client init failed: %w", dockerClientErr)
	}
	// Verify docker is available
	if _, err := dockerClient.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("docker not available: %w", err)
	}

	// Acquire network from pool
	networkName, err := pool.Acquire()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire network from pool: %w", err)
	}

	emitter.Info(events.Fields{
		"test":    testName,
		"network": networkName,
		"action":  "network_acquire",
		"msg":     "Acquired network from pool",
	})

	m := &Manager{
		networkID:          networkName, // Pool networks use name as ID
		networkName:        networkName,
		runningContainers: make(map[string]string),
		events:            emitter,
		testName:          testName,
		pool:              pool,
	}

	return m, nil
}

// createNetworkWithRetry creates a Docker network, retrying once after cleaning stale
// chiperka networks if the first attempt fails due to address pool exhaustion.
func createNetworkWithRetry(networkName string, emitter *events.Emitter) (string, error) {
	resp, err := dockerClient.NetworkCreate(context.Background(), networkName, network.CreateOptions{})
	if err == nil {
		return resp.ID, nil
	}

	// If the error is NOT about address pool exhaustion, fail immediately
	if !strings.Contains(err.Error(), "non-overlapping") {
		return "", err
	}

	// Address pool exhausted — clean up stale chiperka networks and retry
	emitter.Warn(events.Fields{
		"action": "network_pool_exhausted",
		"msg":    "Docker network pool exhausted, cleaning up stale chiperka networks",
	})
	CleanupStaleNetworks()

	resp, err = dockerClient.NetworkCreate(context.Background(), networkName, network.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed even after cleanup: %w", err)
	}
	return resp.ID, nil
}

// CleanupStaleNetworks removes all chiperka- networks that have no running containers.
func CleanupStaleNetworks() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	networks, err := dockerClient.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(filters.Arg("name", "chiperka-")),
	})
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	for _, net := range networks {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			// NetworkRemove will fail if the network is in use — that's fine
			dockerClient.NetworkRemove(ctx, id)
		}(net.ID)
	}
	wg.Wait()
}

// ServiceTimings holds per-phase timing data for a service startup.
type ServiceTimings struct {
	ImageResolve   time.Duration
	ContainerStart time.Duration
	HealthCheck    time.Duration
}

// NetworkName returns the name of the isolated network.
func (m *Manager) NetworkName() string {
	return m.networkName
}

// StartServices starts all services defined in the test in parallel.
// Returns service results with setup durations, or an error if any service fails.
func (m *Manager) StartServices(ctx context.Context, services []model.Service) ([]model.ServiceResult, error) {
	// Store artifact definitions for later collection
	m.serviceArtifactDefs = make(map[string][]model.ServiceArtifact)
	for _, svc := range services {
		if len(svc.Artifacts) > 0 {
			m.serviceArtifactDefs[svc.Name] = svc.Artifacts
		}
	}

	results := make([]model.ServiceResult, len(services))
	errors := make([]error, len(services))

	var wg sync.WaitGroup
	for i, service := range services {
		wg.Add(1)
		go func(idx int, svc model.Service) {
			defer wg.Done()
			imageName, timings, err := m.startService(ctx, svc)
			if err != nil {
				errors[idx] = fmt.Errorf("failed to start service %s: %w", svc.Name, err)
				return
			}
			results[idx] = model.ServiceResult{
				Name:                   svc.Name,
				Image:                  imageName,
				Duration:               timings.ImageResolve + timings.ContainerStart + timings.HealthCheck,
				ImageResolveDuration:   timings.ImageResolve,
				ContainerStartDuration: timings.ContainerStart,
				HealthCheckDuration:    timings.HealthCheck,
			}
		}(i, service)
	}
	wg.Wait()

	// Check for errors
	for _, err := range errors {
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// startService starts a single Docker service in the isolated network.
// Returns the image name, phase timings, and any error.
// If the service fails healthcheck, it is restarted up to maxAttempts times.
func (m *Manager) startService(ctx context.Context, service model.Service) (string, ServiceTimings, error) {
	var timings ServiceTimings

	// Resolve image once (build or pull)
	resolveStart := time.Now()
	imageName, err := m.resolveImage(ctx, service)
	timings.ImageResolve = time.Since(resolveStart)
	if err != nil {
		return "", timings, err
	}

	var lastErr error

	for attempt := 1; attempt <= maxServiceAttempts; attempt++ {
		if attempt > 1 {
			m.events.Warn(events.Fields{
				"test":    m.testName,
				"service": service.Name,
				"action":  "service_restart",
				"attempt": fmt.Sprintf("%d/%d", attempt, maxServiceAttempts),
				"msg":     fmt.Sprintf("Restarting service (attempt %d/%d): %v", attempt, maxServiceAttempts, lastErr),
			})
			time.Sleep(serviceRetryDelay)
			// Clean up previous failed container
			containerName := fmt.Sprintf("%s-%s", m.networkName, service.Name)
			if service.ContainerName != "" {
				containerName = service.ContainerName
			}
			m.containersMu.Lock()
			if oldID, ok := m.runningContainers[service.Name]; ok {
				delete(m.runningContainers, service.Name)
				m.containersMu.Unlock()
				dockerClient.ContainerRemove(ctx, oldID, container.RemoveOptions{Force: true, RemoveVolumes: true})
			} else {
				m.containersMu.Unlock()
				dockerClient.ContainerRemove(ctx, containerName, container.RemoveOptions{Force: true, RemoveVolumes: true})
			}
		}

		containerStart, healthCheck, err := m.startAndHealthcheck(ctx, service, imageName)
		timings.ContainerStart = containerStart
		timings.HealthCheck = healthCheck
		if err == nil {
			return imageName, timings, nil
		}
		lastErr = err
	}

	return "", timings, fmt.Errorf("service %s failed after %d attempts: %w", service.Name, maxServiceAttempts, lastErr)
}

// resolveImage pulls the Docker image for a service if not already present.
func (m *Manager) resolveImage(ctx context.Context, service model.Service) (string, error) {
	if service.Image == "" {
		return "", fmt.Errorf("service %s must have 'image' specified", service.Name)
	}

	_, _, err := dockerClient.ImageInspectWithRaw(ctx, service.Image)
	if err != nil {
		m.events.Info(events.Fields{
			"test":    m.testName,
			"service": service.Name,
			"action":  "image_pull",
			"target":  service.Image,
			"msg":     "Pulling Docker image",
		})
		reader, pullErr := dockerClient.ImagePull(ctx, service.Image, imagetypes.PullOptions{
			RegistryAuth: getRegistryAuth(service.Image),
		})
		if pullErr != nil {
			return "", fmt.Errorf("failed to pull image %s: %w", service.Image, pullErr)
		}
		defer reader.Close()
		io.Copy(io.Discard, reader) // drain to complete pull
	} else {
		m.events.Info(events.Fields{
			"test":    m.testName,
			"service": service.Name,
			"action":  "image_found",
			"target":  service.Image,
			"msg":     "Using existing local image",
		})
	}
	return service.Image, nil
}

// buildHealthConfig converts a model.HealthCheck to a Docker SDK HealthConfig.
func buildHealthConfig(hc *model.HealthCheck) *container.HealthConfig {
	if hc.Test == "" {
		return nil
	}

	interval := parseDurationOrDefault(hc.Interval, time.Second)
	timeout := parseDurationOrDefault(hc.Timeout, 3*time.Second)
	startPeriod := parseDurationOrDefault(hc.StartPeriod, 0)
	retries := hc.Retries
	if retries == 0 {
		retries = 30
	}

	cfg := &container.HealthConfig{
		Test:        []string{"CMD-SHELL", string(hc.Test)},
		Interval:    interval,
		Timeout:     timeout,
		Retries:     retries,
		StartPeriod: startPeriod,
	}
	if hc.StartInterval != "" {
		cfg.StartInterval = parseDurationOrDefault(hc.StartInterval, 0)
	}

	return cfg
}

// parseDurationOrDefault parses a duration string, returning the default on error or empty input.
func parseDurationOrDefault(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}

// startAndHealthcheck starts a container and runs its healthcheck.
// Returns (containerStartDuration, healthCheckDuration, error).
func (m *Manager) startAndHealthcheck(ctx context.Context, service model.Service, imageName string) (time.Duration, time.Duration, error) {
	m.events.Info(events.Fields{
		"test":    m.testName,
		"service": service.Name,
		"network": m.networkName,
		"action":  "container_start",
		"msg":     "Starting container",
	})

	// Determine container name
	containerName := fmt.Sprintf("%s-%s", m.networkName, service.Name)
	if service.ContainerName != "" {
		containerName = service.ContainerName
	}

	// Pre-cleanup: remove any stale container with the same name (from interrupted runs)
	dockerClient.ContainerRemove(ctx, containerName, container.RemoveOptions{Force: true, RemoveVolumes: true})

	// Build container config
	cfg := &container.Config{
		Image: imageName,
	}

	if service.HealthCheck != nil {
		cfg.Healthcheck = buildHealthConfig(service.HealthCheck)
	}

	if service.WorkingDir != "" {
		cfg.WorkingDir = service.WorkingDir
	}

	for k, v := range service.Environment {
		cfg.Env = append(cfg.Env, fmt.Sprintf("%s=%s", k, v))
	}

	if len(service.Command) > 0 {
		cfg.Cmd = []string(service.Command)
	}

	// Host config
	hostCfg := &container.HostConfig{
		NetworkMode: container.NetworkMode(m.networkName),
	}

	// Network config with alias
	networkCfg := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			m.networkName: {
				Aliases: []string{service.Name},
			},
		},
	}

	containerStartTime := time.Now()
	if err := acquireContainerSlot(ctx); err != nil {
		return 0, 0, fmt.Errorf("failed to acquire container slot: %w", err)
	}
	resp, err := dockerClient.ContainerCreate(ctx, cfg, hostCfg, networkCfg, nil, containerName)
	if err != nil {
		releaseContainerSlot()
		return time.Since(containerStartTime), 0, fmt.Errorf("failed to create container: %w", err)
	}

	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		releaseContainerSlot()
		return time.Since(containerStartTime), 0, fmt.Errorf("failed to start container: %w", err)
	}
	releaseContainerSlot()

	m.containersMu.Lock()
	m.runningContainers[service.Name] = resp.ID
	m.containersMu.Unlock()
	containerStartDuration := time.Since(containerStartTime)

	// Wait for health check if configured
	var healthCheckDuration time.Duration
	if service.HealthCheck != nil {
		hcStart := time.Now()
		if err := m.waitForHealthy(ctx, service); err != nil {
			return containerStartDuration, time.Since(hcStart), fmt.Errorf("health check failed: %w", err)
		}
		healthCheckDuration = time.Since(hcStart)
	}

	return containerStartDuration, healthCheckDuration, nil
}

// waitForHealthy polls Docker's native health status until the container reports healthy.
// The actual health checks are performed by Docker daemon (configured via --health-* flags on docker run).
func (m *Manager) waitForHealthy(ctx context.Context, service model.Service) error {
	hc := service.HealthCheck

	m.containersMu.Lock()
	containerID := m.runningContainers[service.Name]
	m.containersMu.Unlock()

	hcMode := "image"
	if hc.Test != "" {
		hcMode = "test"
	}
	m.events.Info(events.Fields{
		"test":    m.testName,
		"service": service.Name,
		"action":  "healthcheck_start",
		"msg":     fmt.Sprintf("Waiting for service (type=%s)", hcMode),
	})

	start := time.Now()

	// Match polling interval to Docker healthcheck interval.
	// Docker only updates health status each interval, so polling faster
	// just returns stale "starting" status and wastes Docker API calls.
	pollInterval := parseDurationOrDefault(hc.Interval, time.Second)
	if pollInterval < 200*time.Millisecond {
		pollInterval = 200 * time.Millisecond
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Check immediately first, then poll
	checkHealth := func() (done bool, err error) {
		status := GetHealthStatus(ctx, containerID)
		switch status {
		case "healthy":
			m.events.Info(events.Fields{
				"test":    m.testName,
				"service": service.Name,
				"action":  "healthcheck_pass",
				"msg":     fmt.Sprintf("Service is healthy (waited %.1fs)", time.Since(start).Seconds()),
			})
			return true, nil
		case "unhealthy":
			m.events.Fail(events.Fields{
				"test":    m.testName,
				"service": service.Name,
				"action":  "healthcheck_fail",
				"msg":     fmt.Sprintf("Service unhealthy after %.1fs", time.Since(start).Seconds()),
			})
			return true, fmt.Errorf("service %s is unhealthy", service.Name)
		}
		return false, nil
	}

	// First check immediately
	if done, err := checkHealth(); done {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			m.events.Fail(events.Fields{
				"test":    m.testName,
				"service": service.Name,
				"action":  "healthcheck_timeout",
				"msg":     fmt.Sprintf("Healthcheck timed out after %.1fs", time.Since(start).Seconds()),
			})
			return fmt.Errorf("service %s healthcheck timed out", service.Name)
		case <-ticker.C:
			if done, err := checkHealth(); done {
				return err
			}
		}
	}
}

// GetHealthStatus queries Docker for the container's health status via SDK.
func GetHealthStatus(ctx context.Context, containerID string) string {
	inspect, err := retryDockerCall(ctx, "ContainerInspect", nil, "", func() (container.InspectResponse, error) {
		return dockerClient.ContainerInspect(ctx, containerID)
	})
	if err != nil {
		return "unhealthy"
	}
	if inspect.State == nil || inspect.State.Health == nil {
		return "unhealthy"
	}
	return inspect.State.Health.Status
}

// RunInNetwork executes a command in the isolated network.
// This is used to run the test executor (e.g., HTTP client) inside the network.
func (m *Manager) RunInNetwork(ctx context.Context, image string, command []string) ([]byte, error) {
	cfg := &container.Config{
		Image: image,
		Cmd:   command,
	}
	hostCfg := &container.HostConfig{
		NetworkMode: container.NetworkMode(m.networkName),
	}

	if err := acquireContainerSlot(ctx); err != nil {
		return nil, fmt.Errorf("failed to acquire container slot: %w", err)
	}
	resp, err := dockerClient.ContainerCreate(ctx, cfg, hostCfg, nil, nil, "")
	if err != nil {
		releaseContainerSlot()
		return nil, fmt.Errorf("failed to create container: %w", err)
	}
	// Use background context so cleanup succeeds even when test ctx is cancelled
	defer dockerClient.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})

	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		releaseContainerSlot()
		return nil, err
	}
	releaseContainerSlot()

	// Wait for container to finish
	statusCh, errCh := dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	var statusCode int64
	select {
	case err := <-errCh:
		if err != nil {
			return nil, err
		}
	case status := <-statusCh:
		statusCode = status.StatusCode
	}

	// Read logs
	var stdoutBuf, stderrBuf bytes.Buffer
	logReader, err := retryDockerCall(ctx, "ContainerLogs", m.events, m.testName, func() (io.ReadCloser, error) {
		return dockerClient.ContainerLogs(ctx, resp.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	})
	if err == nil {
		stdcopy.StdCopy(&stdoutBuf, &stderrBuf, logReader)
		logReader.Close()
	}

	if statusCode != 0 {
		if stderrBuf.Len() > 0 {
			return stdoutBuf.Bytes(), fmt.Errorf("exit code %d: %s", statusCode, stderrBuf.String())
		}
		return stdoutBuf.Bytes(), fmt.Errorf("exit code %d", statusCode)
	}
	return stdoutBuf.Bytes(), nil
}

// RunInNetworkWithFiles executes a command in the isolated network with files
// copied into the container before starting. The files map keys are paths
// relative to / (e.g. "tmp/chiperka-body" becomes /tmp/chiperka-body).
func (m *Manager) RunInNetworkWithFiles(ctx context.Context, image string, command []string, files map[string][]byte) ([]byte, error) {
	cfg := &container.Config{
		Image: image,
		Cmd:   command,
	}
	hostCfg := &container.HostConfig{
		NetworkMode: container.NetworkMode(m.networkName),
	}

	// Create container without starting
	if err := acquireContainerSlot(ctx); err != nil {
		return nil, fmt.Errorf("failed to acquire container slot: %w", err)
	}
	resp, err := dockerClient.ContainerCreate(ctx, cfg, hostCfg, nil, nil, "")
	if err != nil {
		releaseContainerSlot()
		return nil, fmt.Errorf("failed to create container: %w", err)
	}
	defer dockerClient.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})

	// Build tar archive with all files
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	for path, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: path,
			Mode: 0644,
			Size: int64(len(content)),
		}); err != nil {
			return nil, fmt.Errorf("failed to write tar header: %w", err)
		}
		if _, err := tw.Write(content); err != nil {
			return nil, fmt.Errorf("failed to write tar content: %w", err)
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close tar: %w", err)
	}

	// Copy files into container
	if err := dockerClient.CopyToContainer(ctx, resp.ID, "/", &tarBuf, container.CopyToContainerOptions{}); err != nil {
		releaseContainerSlot()
		return nil, fmt.Errorf("failed to copy files to container: %w", err)
	}

	// Start container
	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		releaseContainerSlot()
		return nil, err
	}
	releaseContainerSlot()

	// Wait for container to finish
	statusCh, errCh := dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	var statusCode int64
	select {
	case err := <-errCh:
		if err != nil {
			return nil, err
		}
	case status := <-statusCh:
		statusCode = status.StatusCode
	}

	// Read logs
	var stdoutBuf, stderrBuf bytes.Buffer
	logReader, err := retryDockerCall(ctx, "ContainerLogs", m.events, m.testName, func() (io.ReadCloser, error) {
		return dockerClient.ContainerLogs(ctx, resp.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	})
	if err == nil {
		stdcopy.StdCopy(&stdoutBuf, &stderrBuf, logReader)
		logReader.Close()
	}

	if statusCode != 0 {
		if stderrBuf.Len() > 0 {
			return stdoutBuf.Bytes(), fmt.Errorf("exit code %d: %s", statusCode, stderrBuf.String())
		}
		return stdoutBuf.Bytes(), fmt.Errorf("exit code %d", statusCode)
	}
	return stdoutBuf.Bytes(), nil
}

// RunInNetworkAndCopyOut executes a command in the isolated network and, after
// the container finishes, extracts the contents of outPaths from the container
// filesystem. Use this when the command produces binary output that must not
// pass through Docker's logging driver (which would mangle invalid UTF-8 byte
// sequences via the json-file driver).
//
// inFiles are tar-copied into the container before start (same shape as
// RunInNetworkWithFiles: keys are paths relative to /). outPaths are absolute
// container paths whose contents are copied out and returned in outFiles, keyed
// by the requested path. Missing outPaths are silently omitted.
func (m *Manager) RunInNetworkAndCopyOut(ctx context.Context, image string, command []string, inFiles map[string][]byte, outPaths []string) (stdout []byte, outFiles map[string][]byte, err error) {
	cfg := &container.Config{
		Image: image,
		Cmd:   command,
	}
	hostCfg := &container.HostConfig{
		NetworkMode: container.NetworkMode(m.networkName),
	}

	if err := acquireContainerSlot(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to acquire container slot: %w", err)
	}
	resp, err := dockerClient.ContainerCreate(ctx, cfg, hostCfg, nil, nil, "")
	if err != nil {
		releaseContainerSlot()
		return nil, nil, fmt.Errorf("failed to create container: %w", err)
	}
	defer dockerClient.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})

	if len(inFiles) > 0 {
		var tarBuf bytes.Buffer
		tw := tar.NewWriter(&tarBuf)
		for path, content := range inFiles {
			if err := tw.WriteHeader(&tar.Header{
				Name: path,
				Mode: 0644,
				Size: int64(len(content)),
			}); err != nil {
				releaseContainerSlot()
				return nil, nil, fmt.Errorf("failed to write tar header: %w", err)
			}
			if _, err := tw.Write(content); err != nil {
				releaseContainerSlot()
				return nil, nil, fmt.Errorf("failed to write tar content: %w", err)
			}
		}
		if err := tw.Close(); err != nil {
			releaseContainerSlot()
			return nil, nil, fmt.Errorf("failed to close tar: %w", err)
		}
		if err := dockerClient.CopyToContainer(ctx, resp.ID, "/", &tarBuf, container.CopyToContainerOptions{}); err != nil {
			releaseContainerSlot()
			return nil, nil, fmt.Errorf("failed to copy files to container: %w", err)
		}
	}

	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		releaseContainerSlot()
		return nil, nil, err
	}
	releaseContainerSlot()

	statusCh, errCh := dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	var exitCode int64
	select {
	case err := <-errCh:
		if err != nil {
			return nil, nil, err
		}
	case status := <-statusCh:
		exitCode = status.StatusCode
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	logReader, logErr := retryDockerCall(ctx, "ContainerLogs", m.events, m.testName, func() (io.ReadCloser, error) {
		return dockerClient.ContainerLogs(ctx, resp.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	})
	if logErr == nil {
		stdcopy.StdCopy(&stdoutBuf, &stderrBuf, logReader)
		logReader.Close()
	}

	outFiles = make(map[string][]byte, len(outPaths))
	for _, p := range outPaths {
		data, copyErr := copyFileFromContainer(ctx, resp.ID, p)
		if copyErr != nil {
			continue
		}
		outFiles[p] = data
	}

	if exitCode != 0 {
		if stderrBuf.Len() > 0 {
			return stdoutBuf.Bytes(), outFiles, fmt.Errorf("exit code %d: %s", exitCode, stderrBuf.String())
		}
		return stdoutBuf.Bytes(), outFiles, fmt.Errorf("exit code %d", exitCode)
	}
	return stdoutBuf.Bytes(), outFiles, nil
}

// copyFileFromContainer extracts a single regular file from a stopped or
// running container. Returns os.ErrNotExist-equivalent when the path is
// missing or not a regular file.
func copyFileFromContainer(ctx context.Context, containerID, path string) ([]byte, error) {
	reader, _, err := dockerClient.CopyFromContainer(ctx, containerID, path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	tr := tar.NewReader(reader)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("file %q not found in container", path)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar entry: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("failed to read tar content: %w", err)
		}
		return data, nil
	}
}

// Cleanup stops all containers and removes the network.
func (m *Manager) Cleanup(ctx context.Context) {
	m.CleanupWithArtifacts(ctx, nil, "")
}

// CleanupWithArtifacts stops all containers, collects logs and external artifacts, and removes the network.
func (m *Manager) CleanupWithArtifacts(ctx context.Context, collector *artifact.Collector, uuid string) {
	// Collect artifacts and stop all containers in parallel
	var wg sync.WaitGroup
	for name, containerID := range m.runningContainers {
		wg.Add(1)
		go func(name, containerID string) {
			defer wg.Done()

			// Collect logs before stopping (if collector is provided)
			if collector != nil && uuid != "" {
				m.collectServiceLogs(ctx, collector, uuid, name, containerID)
				// Collect external artifacts from container
				if defs, ok := m.serviceArtifactDefs[name]; ok {
					m.collectServiceArtifacts(ctx, collector, uuid, name, containerID, defs)
				}
			}

			m.events.Info(events.Fields{
				"test":    m.testName,
				"service": name,
				"action":  "container_stop",
				"msg":     "Stopping container",
			})

			if err := dockerClient.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true, RemoveVolumes: true}); err != nil {
				m.events.Warn(events.Fields{
					"test":    m.testName,
					"service": name,
					"action":  "container_stop",
					"msg":     fmt.Sprintf("Failed to remove container: %v", err),
				})
			}
		}(name, containerID)
	}
	wg.Wait()
	m.runningContainers = make(map[string]string)

	// Release network back to pool or remove it
	if m.networkName != "" {
		if m.pool != nil {
			m.events.Info(events.Fields{
				"test":    m.testName,
				"network": m.networkName,
				"action":  "network_release",
				"msg":     "Releasing network back to pool",
			})
			m.pool.Release(ctx, m.networkName)
		} else {
			m.events.Info(events.Fields{
				"test":    m.testName,
				"network": m.networkName,
				"action":  "network_remove",
				"msg":     "Removing Docker network",
			})

			if err := dockerClient.NetworkRemove(ctx, m.networkName); err != nil {
				m.events.Warn(events.Fields{
					"test":    m.testName,
					"network": m.networkName,
					"action":  "network_remove",
					"msg":     fmt.Sprintf("Failed to remove network: %v", err),
				})
			}
		}
	}
}

// collectServiceLogs collects logs from a container and saves them as an artifact.
func (m *Manager) collectServiceLogs(ctx context.Context, collector *artifact.Collector, uuid, serviceName, containerID string) {
	m.events.Info(events.Fields{
		"test":    m.testName,
		"service": serviceName,
		"action":  "logs_collect",
		"msg":     "Collecting service logs",
	})

	logReader, err := retryDockerCall(ctx, "ContainerLogs", m.events, m.testName, func() (io.ReadCloser, error) {
		return dockerClient.ContainerLogs(ctx, containerID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	})
	if err != nil {
		m.events.Warn(events.Fields{
			"test":    m.testName,
			"service": serviceName,
			"action":  "logs_collect",
			"msg":     fmt.Sprintf("Failed to collect logs: %v", err),
		})
		return
	}
	defer logReader.Close()

	var buf bytes.Buffer
	stdcopy.StdCopy(&buf, &buf, logReader)

	// Save logs as artifact
	filename := fmt.Sprintf("%s.log", serviceName)
	path, err := collector.SaveArtifact(uuid, filename, buf.Bytes())
	if err != nil {
		m.events.Warn(events.Fields{
			"test":    m.testName,
			"service": serviceName,
			"action":  "logs_save",
			"msg":     fmt.Sprintf("Failed to save logs: %v", err),
		})
		return
	}

	size := int64(buf.Len())
	m.events.ArtifactSave(filename, path, size)
}

// CollectServiceArtifacts collects user-defined artifacts from running containers without stopping them.
// This allows artifact assertions to access service artifacts before cleanup.
func (m *Manager) CollectServiceArtifacts(ctx context.Context, collector *artifact.Collector, uuid string) {
	var wg sync.WaitGroup
	for name, containerID := range m.runningContainers {
		if defs, ok := m.serviceArtifactDefs[name]; ok {
			wg.Add(1)
			go func(name, containerID string, defs []model.ServiceArtifact) {
				defer wg.Done()
				m.collectServiceArtifacts(ctx, collector, uuid, name, containerID, defs)
			}(name, containerID, defs)
		}
	}
	wg.Wait()
}

// collectServiceArtifacts extracts external artifacts from a container using Docker CopyFromContainer API.
func (m *Manager) collectServiceArtifacts(ctx context.Context, collector *artifact.Collector, uuid, serviceName, containerID string, defs []model.ServiceArtifact) {
	for _, def := range defs {
		m.collectOneServiceArtifact(ctx, collector, uuid, serviceName, containerID, def)
	}
}

// collectOneServiceArtifact extracts a single artifact (file or directory) from a container.
func (m *Manager) collectOneServiceArtifact(ctx context.Context, collector *artifact.Collector, uuid, serviceName, containerID string, def model.ServiceArtifact) {
	// Determine artifact name
	artifactName := def.Name
	if artifactName == "" {
		artifactName = filepath.Base(def.Path)
	}

	m.events.Info(events.Fields{
		"test":    m.testName,
		"service": serviceName,
		"action":  "artifact_collect",
		"target":  def.Path,
		"msg":     fmt.Sprintf("Collecting artifact %s from %s", artifactName, def.Path),
	})

	reader, stat, err := dockerClient.CopyFromContainer(ctx, containerID, def.Path)
	if err != nil {
		m.events.Warn(events.Fields{
			"test":    m.testName,
			"service": serviceName,
			"action":  "artifact_collect",
			"target":  def.Path,
			"msg":     fmt.Sprintf("Failed to copy artifact: %v", err),
		})
		return
	}
	defer reader.Close()

	tr := tar.NewReader(reader)
	isDir := stat.Mode.IsDir()

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			m.events.Warn(events.Fields{
				"test":    m.testName,
				"service": serviceName,
				"action":  "artifact_collect",
				"msg":     fmt.Sprintf("Failed to read tar entry: %v", err),
			})
			return
		}

		// Skip directories
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}

		content, err := io.ReadAll(tr)
		if err != nil {
			m.events.Warn(events.Fields{
				"test":    m.testName,
				"service": serviceName,
				"action":  "artifact_collect",
				"msg":     fmt.Sprintf("Failed to read tar content: %v", err),
			})
			continue
		}

		var savePath string
		var savedPath string
		if isDir {
			// For directories, preserve relative path under artifact name
			relPath := header.Name
			// Docker tar entries have the base dir name as prefix - strip it
			if idx := strings.Index(relPath, "/"); idx >= 0 {
				relPath = relPath[idx+1:]
			}
			if relPath == "" {
				continue
			}
			savePath = artifactName + "/" + relPath
			savedPath, err = collector.SaveArtifactWithPath(uuid, savePath, content)
		} else {
			savePath = artifactName
			savedPath, err = collector.SaveArtifact(uuid, savePath, content)
		}
		if err != nil {
			m.events.Warn(events.Fields{
				"test":    m.testName,
				"service": serviceName,
				"action":  "artifact_save",
				"msg":     fmt.Sprintf("Failed to save artifact %s: %v", savePath, err),
			})
			continue
		}

		m.events.ArtifactSave(savePath, savedPath, int64(len(content)))
	}
}

// ExecResult holds the result of executing a command in a container.
type ExecResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
}

// ExecInContainer executes a command inside a running service container.
// The command is passed to sh -c for execution.
// Optional env slice injects extra environment variables (format: "KEY=VALUE").
func (m *Manager) ExecInContainer(ctx context.Context, serviceName, command, workingDir string, env []string) (*ExecResult, error) {
	m.containersMu.Lock()
	containerID, ok := m.runningContainers[serviceName]
	m.containersMu.Unlock()

	if !ok {
		return nil, fmt.Errorf("service %s not found in running containers", serviceName)
	}

	m.events.Info(events.Fields{
		"test":    m.testName,
		"service": serviceName,
		"action":  "exec_start",
		"msg":     fmt.Sprintf("Executing command in container: %s", command),
	})

	// Create exec
	execCfg := container.ExecOptions{
		Cmd:          []string{"sh", "-c", command},
		AttachStdout: true,
		AttachStderr: true,
	}
	if workingDir != "" {
		execCfg.WorkingDir = workingDir
	}
	if len(env) > 0 {
		execCfg.Env = env
	}

	execResp, err := retryDockerCall(ctx, "ContainerExecCreate", m.events, m.testName, func() (container.ExecCreateResponse, error) {
		return dockerClient.ContainerExecCreate(ctx, containerID, execCfg)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create exec: %w", err)
	}

	attachResp, err := dockerClient.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to attach exec: %w", err)
	}

	// Read stdout/stderr in a goroutine. Close the attach connection when context
	// is cancelled so StdCopy unblocks — otherwise a long-running command keeps
	// the goroutine stuck until the process finishes, ignoring the timeout.
	var stdout, stderr bytes.Buffer
	copyDone := make(chan struct{})
	go func() {
		defer close(copyDone)
		stdcopy.StdCopy(&stdout, &stderr, attachResp.Reader)
	}()

	select {
	case <-copyDone:
		attachResp.Close()
	case <-ctx.Done():
		attachResp.Close()
		<-copyDone
		return nil, fmt.Errorf("exec timed out: %w", ctx.Err())
	}

	// Get exit code — use fresh context since the exec already finished
	inspectCtx, inspectCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer inspectCancel()
	inspectResp, err := retryDockerCall(inspectCtx, "ContainerExecInspect", m.events, m.testName, func() (container.ExecInspect, error) {
		return dockerClient.ContainerExecInspect(inspectCtx, execResp.ID)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to inspect exec: %w", err)
	}

	m.events.Info(events.Fields{
		"test":     m.testName,
		"service":  serviceName,
		"action":   "exec_complete",
		"exitCode": fmt.Sprintf("%d", inspectResp.ExitCode),
		"msg":      fmt.Sprintf("Command completed with exit code %d", inspectResp.ExitCode),
	})

	return &ExecResult{
		ExitCode: inspectResp.ExitCode,
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
	}, nil
}
