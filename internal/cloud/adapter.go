package cloud

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"chiperka-cli/internal/events"
)

// SSEAdapter bridges SSE events into the internal event bus so that all
// existing reporters (CLI, TeamCity, JSON, Verbose) work in cloud mode.
type SSEAdapter struct {
	bus          *events.Bus
	testNames    map[int64]string
	suiteName    map[int64]string // test ID → suite name
	finishedTest map[int64]bool   // test IDs that already emitted a terminal event
	startTime    time.Time
	started      bool // true after first RunStarted emission
}

// NewSSEAdapter creates an adapter that translates SSE events into event bus emissions.
func NewSSEAdapter(bus *events.Bus) *SSEAdapter {
	return &SSEAdapter{
		bus:          bus,
		testNames:    make(map[int64]string),
		suiteName:    make(map[int64]string),
		finishedTest: make(map[int64]bool),
		startTime:    time.Now(),
	}
}

// runCancelled is the structure received in "run_cancelled" SSE events.
type runCancelled struct {
	RunID     string `json:"run_id"`
	Passed    int    `json:"passed"`
	Failed    int    `json:"failed"`
	Cancelled int    `json:"cancelled"`
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
	case "run_cancelled":
		return a.handleRunCancelled(event.Data)
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

	// Only emit RunStarted on first snapshot (reconnects send snapshot again)
	if !a.started {
		a.started = true
		a.bus.Emit(events.NewEvent(events.RunStarted).
			WithDetail("tests", totalTests).
			WithDetail("suites", suiteCount))
	}

	// Emit events for tests that already have a terminal status in the snapshot,
	// but skip tests we've already processed (from before reconnect)
	for _, suite := range snap.Suites {
		for _, test := range suite.Tests {
			if a.finishedTest[test.ID] {
				continue
			}
			switch test.Status {
			case "running":
				te := events.NewTestEvent(events.TestStarted, suite.Name, test.Name)
				a.bus.Emit(te)
			case "passed":
				a.finishedTest[test.ID] = true
				te := events.NewTestEvent(events.TestCompleted, suite.Name, test.Name)
				a.bus.Emit(te)
			case "failed", "error":
				a.finishedTest[test.ID] = true
				te := events.NewTestEvent(events.TestFailed, suite.Name, test.Name)
				a.bus.Emit(te)
			case "skipped", "cancelled":
				a.finishedTest[test.ID] = true
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
		if a.finishedTest[update.TestID] {
			return nil, false // already counted from snapshot
		}
		a.finishedTest[update.TestID] = true
		e := events.NewTestEvent(events.TestCompleted, suite, name).WithDuration(duration)
		a.bus.Emit(e)
	case "failed", "error":
		if a.finishedTest[update.TestID] {
			return nil, false // already counted from snapshot
		}
		a.finishedTest[update.TestID] = true
		e := events.NewTestEvent(events.TestFailed, suite, name).WithDuration(duration).WithMessage(update.Message)
		a.bus.Emit(e)
	case "skipped", "cancelled":
		if a.finishedTest[update.TestID] {
			return nil, false // already counted from snapshot
		}
		a.finishedTest[update.TestID] = true
		e := events.NewTestEvent(events.TestSkipped, suite, name)
		a.bus.Emit(e)
	}

	return nil, false
}

func (a *SSEAdapter) handleRunCancelled(data json.RawMessage) (*RunResult, bool) {
	var rc runCancelled
	if err := json.Unmarshal(data, &rc); err != nil {
		log.Printf("Warning: failed to unmarshal run_cancelled event: %v", err)
		return nil, false
	}

	elapsed := time.Since(a.startTime)
	a.bus.Emit(events.NewEvent(events.RunCompleted).
		WithDuration(elapsed).
		WithDetail("passed", rc.Passed).
		WithDetail("failed", rc.Failed).
		WithDetail("skipped", rc.Cancelled))

	return &RunResult{
		Passed:    rc.Passed,
		Failed:    rc.Failed,
		Skipped:   rc.Cancelled,
		Cancelled: true,
	}, true
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
