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
)

// SSEEvent represents a parsed Server-Sent Event.
type SSEEvent struct {
	Event string
	Data  json.RawMessage
}

// RunResult holds the final outcome of a cloud run.
type RunResult struct {
	Passed  int
	Failed  int
	Skipped int
}

// HasFailures returns true if any tests failed.
func (r *RunResult) HasFailures() bool {
	return r.Failed > 0
}

// snapshotRun is the structure received in the "snapshot" SSE event.
type snapshotRun struct {
	ID     string        `json:"id"`
	Suites []snapshotSuite `json:"suites"`
}

type snapshotSuite struct {
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
}

// runCompleted is the structure received in "run_completed" SSE events.
type runCompleted struct {
	RunID   string `json:"run_id"`
	Passed  int    `json:"passed"`
	Failed  int    `json:"failed"`
	Skipped int    `json:"skipped"`
}

// StreamRun connects to the SSE stream for a run and reports progress.
// It blocks until the run completes or the context is cancelled.
func (c *Client) StreamRun(ctx context.Context, runID string, out io.Writer) (*RunResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/events/runs?id="+runID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSE request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

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

	// Parse SSE stream
	reporter := &progressReporter{
		out:        out,
		testNames:  make(map[int64]string),
		totalTests: 0,
		completed:  0,
		startTime:  time.Now(),
	}

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
			result, done := reporter.handleEvent(SSEEvent{Event: eventType, Data: data})
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

// progressReporter handles SSE events and prints progress.
type progressReporter struct {
	out        io.Writer
	testNames  map[int64]string
	totalTests int
	completed  int
	startTime  time.Time
	headerPrinted bool
}

// handleEvent processes an SSE event and returns a result if the run is complete.
func (p *progressReporter) handleEvent(event SSEEvent) (*RunResult, bool) {
	switch event.Event {
	case "snapshot":
		var snap snapshotRun
		if err := json.Unmarshal(event.Data, &snap); err != nil {
			log.Printf("Warning: failed to unmarshal snapshot event: %v", err)
			return nil, false
		}
		// Build test ID -> name map and count totals
		for _, suite := range snap.Suites {
			for _, test := range suite.Tests {
				p.testNames[test.ID] = test.Name
				p.totalTests++
				// Count already-completed tests from snapshot
				if test.Status == "passed" || test.Status == "failed" || test.Status == "error" || test.Status == "skipped" {
					p.completed++
				}
			}
		}

	case "test_update":
		var update testUpdate
		if err := json.Unmarshal(event.Data, &update); err != nil {
			log.Printf("Warning: failed to unmarshal test_update event: %v", err)
			return nil, false
		}

		// Only print for terminal statuses
		if update.Status == "passed" || update.Status == "failed" || update.Status == "error" || update.Status == "skipped" {
			p.completed++

			if !p.headerPrinted {
				fmt.Fprintf(p.out, "\nRunning tests\n")
				p.headerPrinted = true
			}

			name := p.testNames[update.TestID]
			if name == "" {
				name = fmt.Sprintf("test-%d", update.TestID)
			}

			pct := 0
			if p.totalTests > 0 {
				pct = (p.completed * 100) / p.totalTests
			}

			duration := float64(update.DurationMs) / 1000.0
			icon := "✓"
			if update.Status == "failed" || update.Status == "error" {
				icon = "✗"
			} else if update.Status == "skipped" {
				icon = "-"
			}

			fmt.Fprintf(p.out, "  %s [%3d%%] %s (%.3fs)\n", icon, pct, name, duration)
		}

	case "run_completed":
		var rc runCompleted
		if err := json.Unmarshal(event.Data, &rc); err != nil {
			log.Printf("Warning: failed to unmarshal run_completed event: %v", err)
			return nil, false
		}

		elapsed := time.Since(p.startTime).Seconds()

		fmt.Fprintln(p.out)
		total := rc.Passed + rc.Failed + rc.Skipped
		if rc.Failed > 0 {
			fmt.Fprintf(p.out, "FAILED %d/%d (%d failed) in %.1fs\n", rc.Passed, total, rc.Failed, elapsed)
		} else {
			fmt.Fprintf(p.out, "PASSED %d/%d in %.1fs\n", rc.Passed, total, elapsed)
		}

		return &RunResult{
			Passed:  rc.Passed,
			Failed:  rc.Failed,
			Skipped: rc.Skipped,
		}, true
	}

	return nil, false
}
