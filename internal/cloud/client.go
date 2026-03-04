// Package cloud provides an HTTP client for the Spark Cloud API.
package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"spark-cli/internal/model"
)

// Client is the Spark Cloud API client.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new cloud API client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// HealthCheck verifies the API server is reachable.
func (c *Client) HealthCheck() error {
	resp, err := c.httpClient.Get(c.baseURL + "/health")
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
	Suites []SuiteSubmission `json:"suites"`
	Config *RunConfig        `json:"config,omitempty"`
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

	resp, err := c.httpClient.Post(c.baseURL+"/api/runs", "application/json", bytes.NewReader(body))
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
	resp, err := c.httpClient.Post(c.baseURL+"/api/runs/"+runID+"/stop", "application/json", nil)
	if err != nil {
		return fmt.Errorf("failed to stop run: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

// BuildSubmission converts a TestCollection into a CreateRunRequest.
func BuildSubmission(tests *model.TestCollection, services *model.ServiceTemplateCollection, version string) (*CreateRunRequest, error) {
	req := &CreateRunRequest{
		Config: &RunConfig{
			Version: version,
		},
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

		req.Suites = append(req.Suites, SuiteSubmission{
			Name:     suite.Name,
			FilePath: suite.FilePath,
			Tests:    resolvedTests,
		})
	}

	return req, nil
}
