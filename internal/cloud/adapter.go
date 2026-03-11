package cloud

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"spark-cli/internal/events"
)

// SSEAdapter bridges SSE events into the internal event bus so that all
// existing reporters (CLI, TeamCity, JSON, Verbose) work in cloud mode.
type SSEAdapter struct {
	bus       *events.Bus
	testNames map[int64]string
	suiteName map[int64]string // test ID → suite name
	startTime time.Time
}

// NewSSEAdapter creates an adapter that translates SSE events into event bus emissions.
func NewSSEAdapter(bus *events.Bus) *SSEAdapter {
	return &SSEAdapter{
		bus:       bus,
		testNames: make(map[int64]string),
		suiteName: make(map[int64]string),
		startTime: time.Now(),
	}
}

// HandleEvent processes an SSE event and returns a RunResult when the run completes.
func (a *SSEAdapter) HandleEvent(event SSEEvent) (*RunResult, bool) {
	switch event.Event {
	case "snapshot":
		return a.handleSnapshot(event.Data)
	case "test_update":
		return a.handleTestUpdate(event.Data)
	case "run_completed":
		return a.handleRunCompleted(event.Data)
	}
	return nil, false
}

func (a *SSEAdapter) handleSnapshot(data json.RawMessage) (*RunResult, bool) {
	var snap snapshotRun
	if err := json.Unmarshal(data, &snap); err != nil {
		log.Printf("Warning: failed to unmarshal snapshot event: %v", err)
		return nil, false
	}

	totalTests := 0
	suiteCount := len(snap.Suites)
	for _, suite := range snap.Suites {
		for _, test := range suite.Tests {
			a.testNames[test.ID] = test.Name
			a.suiteName[test.ID] = suite.Name
			totalTests++
		}
	}

	a.bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", totalTests).
		WithDetail("suites", suiteCount))

	// Emit events for tests that already have a terminal status in the snapshot
	for _, suite := range snap.Suites {
		for _, test := range suite.Tests {
			switch test.Status {
			case "running":
				te := events.NewTestEvent(events.TestStarted, suite.Name, test.Name)
				a.bus.Emit(te)
			case "passed":
				te := events.NewTestEvent(events.TestCompleted, suite.Name, test.Name)
				a.bus.Emit(te)
			case "failed", "error":
				te := events.NewTestEvent(events.TestFailed, suite.Name, test.Name)
				a.bus.Emit(te)
			case "skipped":
				te := events.NewTestEvent(events.TestSkipped, suite.Name, test.Name)
				a.bus.Emit(te)
			}
		}
	}

	return nil, false
}

func (a *SSEAdapter) handleTestUpdate(data json.RawMessage) (*RunResult, bool) {
	var update testUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		log.Printf("Warning: failed to unmarshal test_update event: %v", err)
		return nil, false
	}

	name := a.testNames[update.TestID]
	if name == "" {
		name = fmt.Sprintf("test-%d", update.TestID)
	}
	suite := a.suiteName[update.TestID]

	duration := time.Duration(update.DurationMs) * time.Millisecond

	switch update.Status {
	case "running":
		e := events.NewTestEvent(events.TestStarted, suite, name)
		a.bus.Emit(e)
	case "passed":
		e := events.NewTestEvent(events.TestCompleted, suite, name).WithDuration(duration)
		a.bus.Emit(e)
	case "failed", "error":
		e := events.NewTestEvent(events.TestFailed, suite, name).WithDuration(duration).WithMessage(update.Message)
		a.bus.Emit(e)
	case "skipped":
		e := events.NewTestEvent(events.TestSkipped, suite, name)
		a.bus.Emit(e)
	}

	return nil, false
}

func (a *SSEAdapter) handleRunCompleted(data json.RawMessage) (*RunResult, bool) {
	var rc runCompleted
	if err := json.Unmarshal(data, &rc); err != nil {
		log.Printf("Warning: failed to unmarshal run_completed event: %v", err)
		return nil, false
	}

	elapsed := time.Since(a.startTime)
	a.bus.Emit(events.NewEvent(events.RunCompleted).
		WithDuration(elapsed).
		WithDetail("passed", rc.Passed).
		WithDetail("failed", rc.Failed).
		WithDetail("skipped", rc.Skipped))

	return &RunResult{
		Passed:  rc.Passed,
		Failed:  rc.Failed,
		Skipped: rc.Skipped,
	}, true
}
