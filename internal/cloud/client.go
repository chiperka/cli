// Package cloud provides an HTTP client for the Spark Cloud API.
package cloud

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"spark-cli/internal/model"
)

// Client is the Spark Cloud API client.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new cloud API client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// setAuth sets the Authorization header if a token is configured.
func (c *Client) setAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

// HealthCheck verifies the API server is reachable.
func (c *Client) HealthCheck() error {
	req, err := http.NewRequest("GET", c.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach API server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("API server returned %d (failed to read body: %v)", resp.StatusCode, err)
		}
		return fmt.Errorf("API server returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// CreateRunRequest is the payload for creating a run.
type CreateRunRequest struct {
	Suites    []SuiteSubmission `json:"suites"`
	Config    *RunConfig        `json:"config,omitempty"`
	ProjectID *int64            `json:"project_id,omitempty"`
}

// SuiteSubmission represents a pre-parsed suite.
type SuiteSubmission struct {
	Name     string       `json:"name"`
	FilePath string       `json:"file_path"`
	Tests    []model.Test `json:"tests"`
}

// RunConfig contains optional run configuration.
type RunConfig struct {
	Version string `json:"version,omitempty"`
}

// CreateRunResponse is the response from creating a run.
type CreateRunResponse struct {
	ID string `json:"id"`
}

// CreateRun uploads test definitions and creates a new run.
func (c *Client) CreateRun(req *CreateRunRequest) (*CreateRunResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/runs", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuth(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create run: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("API returned %d (failed to read body: %v)", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result CreateRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}

// StopRun requests the API to cancel a running run.
func (c *Client) StopRun(runID string) error {
	req, err := http.NewRequest("POST", c.baseURL+"/api/runs/"+runID+"/stop", nil)
	if err != nil {
		return fmt.Errorf("failed to create stop request: %w", err)
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to stop run: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

// DownloadReport downloads a report file from a completed run.
// Format must be "xml" (downloads report.xml to outputPath).
func (c *Client) DownloadReport(runID, format, outputPath string) error {
	var endpoint string
	switch format {
	case "xml":
		endpoint = fmt.Sprintf("/api/runs/%s/report.xml", runID)
	default:
		return fmt.Errorf("unsupported report format: %s", format)
	}

	req, err := http.NewRequest("GET", c.baseURL+endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create report request: %w", err)
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download report: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	// Ensure parent directory exists
	if dir := filepath.Dir(outputPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("failed to write report: %w", err)
	}

	return nil
}

// DownloadHTMLReportZip downloads the HTML report as a ZIP and extracts it to outputDir.
func (c *Client) DownloadHTMLReportZip(runID, outputDir string) error {
	endpoint := fmt.Sprintf("/api/runs/%s/report.zip", runID)

	req, err := http.NewRequest("GET", c.baseURL+endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create report request: %w", err)
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download report: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	// Read entire ZIP into memory
	zipData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read report: %w", err)
	}

	// Open ZIP archive
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}

	// Extract all files
	for _, f := range zr.File {
		target := filepath.Join(outputDir, f.Name)

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", f.Name, err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open zip entry %s: %w", f.Name, err)
		}

		outFile, err := os.Create(target)
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to create file %s: %w", target, err)
		}

		if _, err := io.Copy(outFile, rc); err != nil {
			outFile.Close()
			rc.Close()
			return fmt.Errorf("failed to extract %s: %w", f.Name, err)
		}

		outFile.Close()
		rc.Close()
	}

	return nil
}

// DownloadArtifactsZip downloads the artifacts ZIP for a run and extracts it to outputDir.
// Returns nil if the server returns 404 (no artifacts is not an error).
func (c *Client) DownloadArtifactsZip(runID, outputDir string) error {
	endpoint := fmt.Sprintf("/api/runs/%s/artifacts.zip", runID)

	req, err := http.NewRequest("GET", c.baseURL+endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create artifacts request: %w", err)
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download artifacts: %w", err)
	}
	defer resp.Body.Close()

	// 404 means no artifacts — not an error
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	// Read entire ZIP into memory
	zipData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read artifacts zip: %w", err)
	}

	// Open ZIP archive
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("failed to open artifacts zip: %w", err)
	}

	// Extract all files
	for _, f := range zr.File {
		target := filepath.Join(outputDir, f.Name)

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", f.Name, err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open zip entry %s: %w", f.Name, err)
		}

		outFile, err := os.Create(target)
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to create file %s: %w", target, err)
		}

		if _, err := io.Copy(outFile, rc); err != nil {
			outFile.Close()
			rc.Close()
			return fmt.Errorf("failed to extract %s: %w", f.Name, err)
		}

		outFile.Close()
		rc.Close()
	}

	return nil
}

// CollectSnapshotFiles walks all suites and tests, finds snapshot file references,
// reads the files from disk, and returns a map of zipEntryPath -> fileContent.
// The zipEntryPath preserves the relative structure so workers can reconstruct paths.
func CollectSnapshotFiles(suites []SuiteSubmission) (map[string][]byte, error) {
	snapshots := make(map[string][]byte)
	seen := make(map[string]bool) // deduplicate by absolute path

	for _, suite := range suites {
		suiteDir := filepath.Dir(suite.FilePath)

		for _, test := range suite.Tests {
			for _, assertion := range test.Assertions {
				// Collect all snapshot paths from this assertion
				var snapshotPaths []string

				if assertion.Response != nil && assertion.Response.Body != nil && assertion.Response.Body.Snapshot != "" {
					snapshotPaths = append(snapshotPaths, assertion.Response.Body.Snapshot)
				}
				if assertion.CLI != nil {
					if assertion.CLI.Stdout != nil && assertion.CLI.Stdout.Snapshot != "" {
						snapshotPaths = append(snapshotPaths, assertion.CLI.Stdout.Snapshot)
					}
					if assertion.CLI.Stderr != nil && assertion.CLI.Stderr.Snapshot != "" {
						snapshotPaths = append(snapshotPaths, assertion.CLI.Stderr.Snapshot)
					}
				}
				if assertion.Artifact != nil && assertion.Artifact.Snapshot != "" {
					snapshotPaths = append(snapshotPaths, assertion.Artifact.Snapshot)
				}

				for _, snapshotRelPath := range snapshotPaths {
					absPath := filepath.Join(suiteDir, snapshotRelPath)
					absPath, err := filepath.Abs(absPath)
					if err != nil {
						return nil, fmt.Errorf("could not resolve snapshot path %s: %w", snapshotRelPath, err)
					}

					if seen[absPath] {
						continue
					}
					seen[absPath] = true

					content, err := os.ReadFile(absPath)
					if err != nil {
						return nil, fmt.Errorf("snapshot file %s not found (referenced in %s): %w", absPath, suite.FilePath, err)
					}

					// Key is the path relative to CLI working directory, using forward slashes
					zipKey := filepath.ToSlash(filepath.Join(suiteDir, snapshotRelPath))
					snapshots[zipKey] = content
				}
			}
		}
	}

	return snapshots, nil
}

// UploadSnapshots builds a zip archive from the snapshot map and uploads it to the API.
func (c *Client) UploadSnapshots(runID string, snapshots map[string][]byte) error {
	// Build zip in memory
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for name, content := range snapshots {
		fw, err := zw.Create(name)
		if err != nil {
			return fmt.Errorf("failed to create zip entry %s: %w", name, err)
		}
		if _, err := fw.Write(content); err != nil {
			return fmt.Errorf("failed to write zip entry %s: %w", name, err)
		}
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("failed to finalize zip: %w", err)
	}

	// Upload to API with extended timeout (snapshot ZIPs can be large)
	endpoint := fmt.Sprintf("%s/api/runs/%s/snapshots", c.baseURL, runID)

	uploadClient := &http.Client{Timeout: 5 * time.Minute}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequest("POST", endpoint, bytes.NewReader(buf.Bytes()))
		if err != nil {
			return fmt.Errorf("failed to create snapshot upload request: %w", err)
		}
		req.Header.Set("Content-Type", "application/zip")
		c.setAuth(req)

		resp, err := uploadClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to upload snapshots (attempt %d): %w", attempt+1, err)
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusNoContent {
			return nil
		}

		body, _ := io.ReadAll(resp.Body)
		lastErr = fmt.Errorf("snapshot upload failed with status %d: %s", resp.StatusCode, string(body))

		if resp.StatusCode >= 500 {
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
			continue
		}
		return lastErr
	}

	return lastErr
}

// ResolveProject resolves a project slug to its numeric ID.
func (c *Client) ResolveProject(slug string) (int64, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/projects/resolve?slug="+slug, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create project resolve request: %w", err)
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve project: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("project %q not found (API returned %d: %s)", slug, resp.StatusCode, string(body))
	}

	var result struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode project response: %w", err)
	}
	return result.ID, nil
}

// BuildSubmission converts a TestCollection into a CreateRunRequest.
func BuildSubmission(tests *model.TestCollection, services *model.ServiceTemplateCollection, version string, projectID *int64) (*CreateRunRequest, error) {
	req := &CreateRunRequest{
		Config: &RunConfig{
			Version: version,
		},
		ProjectID: projectID,
	}

	for _, suite := range tests.Suites {
		// Resolve service template references for each test
		resolvedTests := make([]model.Test, len(suite.Tests))
		for i, test := range suite.Tests {
			resolvedServices := make([]model.Service, len(test.Services))
			for j, svc := range test.Services {
				resolved, err := services.ResolveService(svc)
				if err != nil {
					return nil, fmt.Errorf("test %q: %w", test.Name, err)
				}
				resolvedServices[j] = resolved
			}
			resolvedTests[i] = test
			resolvedTests[i].Services = resolvedServices
		}

		// Normalize FilePath to relative (prevents absolute paths in zip keys and DB)
		filePath := suite.FilePath
		if filepath.IsAbs(filePath) {
			if cwd, err := os.Getwd(); err == nil {
				if rel, err := filepath.Rel(cwd, filePath); err == nil {
					filePath = rel
				}
			}
		}

		req.Suites = append(req.Suites, SuiteSubmission{
			Name:     suite.Name,
			FilePath: filePath,
			Tests:    resolvedTests,
		})
	}

	return req, nil
}
