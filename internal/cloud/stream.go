package cloud

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

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

const (
	maxReconnectAttempts = 10
	reconnectBaseDelay   = 1 * time.Second
	reconnectMaxDelay    = 15 * time.Second
)

// StreamRun connects to the SSE stream for a run and emits events on the bus.
// It blocks until the run completes or the context is cancelled.
// On transient errors (connection reset, HTTP/2 stream errors), it reconnects
// automatically. The adapter deduplicates events so no test is reported twice.
func (c *Client) StreamRun(ctx context.Context, runID string, bus *events.Bus) (*RunResult, error) {
	adapter := NewSSEAdapter(bus)

	var lastErr error
	for attempt := 0; attempt <= maxReconnectAttempts; attempt++ {
		if attempt > 0 {
			delay := reconnectBaseDelay * time.Duration(1<<(attempt-1))
			if delay > reconnectMaxDelay {
				delay = reconnectMaxDelay
			}
			log.Printf("SSE stream disconnected, reconnecting in %v (attempt %d/%d)...", delay, attempt, maxReconnectAttempts)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		result, err := c.streamOnce(ctx, runID, adapter)
		if err == nil {
			return result, nil
		}

		// Context cancelled — don't retry
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Non-retryable errors (auth, not found, etc.)
		if isNonRetryable(err) {
			return nil, err
		}

		lastErr = err
	}

	return nil, fmt.Errorf("SSE stream failed after %d reconnect attempts, last error: %w", maxReconnectAttempts, lastErr)
}

// streamOnce performs a single SSE stream connection. Returns (result, nil) on
// successful completion, or (nil, error) on failure.
func (c *Client) streamOnce(ctx context.Context, runID string, adapter *SSEAdapter) (*RunResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/events/runs?id="+runID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSE request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	c.setAuth(req)

	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SSE stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, &nonRetryableError{fmt.Errorf("SSE stream returned %d (failed to read body: %v)", resp.StatusCode, err)}
		}
		return nil, &nonRetryableError{fmt.Errorf("SSE stream returned %d: %s", resp.StatusCode, string(body))}
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var eventType string
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, ":") {
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
			continue
		}

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

// nonRetryableError wraps errors that should not trigger a reconnect.
type nonRetryableError struct {
	err error
}

func (e *nonRetryableError) Error() string { return e.err.Error() }
func (e *nonRetryableError) Unwrap() error { return e.err }

func isNonRetryable(err error) bool {
	_, ok := err.(*nonRetryableError)
	return ok
}
