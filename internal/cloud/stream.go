package cloud

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"chiperka-cli/internal/events"
)

// SSEEvent represents a parsed Server-Sent Event.
type SSEEvent struct {
	Event string
	Data  json.RawMessage
}

// RunResult holds the final outcome of a cloud run.
type RunResult struct {
	Passed    int
	Failed    int
	Skipped   int
	Cancelled bool
}

// HasFailures returns true if any tests failed.
func (r *RunResult) HasFailures() bool {
	return r.Failed > 0
}

// snapshotRun is the structure received in the "snapshot" SSE event.
type snapshotRun struct {
	ID     string          `json:"id"`
	Suites []snapshotSuite `json:"suites"`
}

type snapshotSuite struct {
	Name  string         `json:"name"`
	Tests []snapshotTest `json:"tests"`
}

type snapshotTest struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// testUpdate is the structure received in "test_update" SSE events.
type testUpdate struct {
	TestID     int64  `json:"test_id"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration"`
	Message    string `json:"message"`
}

// runCompleted is the structure received in "run_completed" SSE events.
type runCompleted struct {
	RunID   string `json:"run_id"`
	Passed  int    `json:"passed"`
	Failed  int    `json:"failed"`
	Skipped int    `json:"skipped"`
}

// StreamRun connects to the SSE stream for a run and emits events on the bus.
// It blocks until the run completes or the context is cancelled.
func (c *Client) StreamRun(ctx context.Context, runID string, bus *events.Bus) (*RunResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/events/runs?id="+runID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSE request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	c.setAuth(req)

	// Use a client without timeout for streaming
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SSE stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("SSE stream returned %d (failed to read body: %v)", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("SSE stream returned %d: %s", resp.StatusCode, string(body))
	}

	adapter := NewSSEAdapter(bus)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // 4MB max for large snapshots
	var eventType string
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		// Comment (keepalive)
		if strings.HasPrefix(line, ":") {
			continue
		}

		// Event type
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}

		// Data line
		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
			continue
		}

		// Empty line = dispatch event
		if line == "" && eventType != "" && len(dataLines) > 0 {
			data := json.RawMessage(strings.Join(dataLines, "\n"))
			result, done := adapter.HandleEvent(SSEEvent{Event: eventType, Data: data})
			if done {
				return result, nil
			}
			eventType = ""
			dataLines = nil
		}
	}

	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("SSE stream error: %w", err)
	}

	return nil, fmt.Errorf("SSE stream closed unexpectedly")
}
