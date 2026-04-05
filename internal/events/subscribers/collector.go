package subscribers

import (
	"sort"
	"sync"
	"time"

	"chiperka-cli/internal/events"
)

// EventCollector stores all events in memory for later processing.
// Used for generating reports, benchmarks, and detailed analysis.
type EventCollector struct {
	mu     sync.RWMutex
	events []*events.Event

	// Indexed access for fast lookups
	byTest map[string][]*events.Event // "suite/test" -> events
	byType map[events.Type][]*events.Event

	// Timing data for benchmarks
	testTimings map[string]*TestTiming // "suite/test" -> timing breakdown
	runTiming   *RunTiming
}

// TestTiming holds detailed timing breakdown for a single test.
type TestTiming struct {
	TestName  string
	SuiteName string
	StartTime time.Time
	EndTime   time.Time

	// Phase durations
	ServiceStartup time.Duration
	HealthCheck    time.Duration
	Setup          time.Duration
	Execution      time.Duration
	Assertion      time.Duration
	Teardown       time.Duration
	Cleanup        time.Duration

	// Total duration
	Total time.Duration

	// Result
	Passed bool
	Error  string
}

// RunTiming holds timing for the entire test run.
type RunTiming struct {
	StartTime time.Time
	EndTime   time.Time

	TestExecution time.Duration
	Total         time.Duration

	// Counts
	TotalTests   int
	PassedTests  int
	FailedTests  int
	SkippedTests int
}

// NewEventCollector creates a new collector.
func NewEventCollector() *EventCollector {
	return &EventCollector{
		events:      make([]*events.Event, 0, 1000),
		byTest:      make(map[string][]*events.Event),
		byType:      make(map[events.Type][]*events.Event),
		testTimings: make(map[string]*TestTiming),
		runTiming:   &RunTiming{},
	}
}

// Register subscribes this collector to receive all events.
func (c *EventCollector) Register(bus *events.Bus) {
	bus.OnAll(c.collect)
}

func (c *EventCollector) collect(e *events.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Store copy to prevent mutation
	eventCopy := *e
	if e.Data.Details != nil {
		eventCopy.Data.Details = make(map[string]any)
		for k, v := range e.Data.Details {
			eventCopy.Data.Details[k] = v
		}
	}

	c.events = append(c.events, &eventCopy)

	// Index by type
	c.byType[e.Type] = append(c.byType[e.Type], &eventCopy)

	// Index by test
	if e.TestName != "" {
		key := e.TestKey()
		c.byTest[key] = append(c.byTest[key], &eventCopy)
	}

	// Update timing data
	c.updateTimings(&eventCopy)
}

func (c *EventCollector) updateTimings(e *events.Event) {
	switch e.Type {
	case events.RunStarted:
		c.runTiming.StartTime = e.Timestamp
		if tests, ok := e.Data.Details["tests"].(int); ok {
			c.runTiming.TotalTests = tests
		}

	case events.RunCompleted:
		c.runTiming.EndTime = e.Timestamp
		c.runTiming.Total = e.Data.Duration

	case events.TestStarted:
		key := e.TestKey()
		c.testTimings[key] = &TestTiming{
			TestName:  e.TestName,
			SuiteName: e.SuiteName,
			StartTime: e.Timestamp,
		}

	case events.TestServiceStarted:
		// Mark service startup start
		key := e.TestKey()
		if tt, ok := c.testTimings[key]; ok {
			// Track start of service phase if not set
			if tt.ServiceStartup == 0 {
				// Will be calculated when services are ready
			}
		}

	case events.TestServiceReady:
		key := e.TestKey()
		if tt, ok := c.testTimings[key]; ok {
			// Accumulate service startup time
			tt.ServiceStartup += e.Data.Duration
		}

	case events.TestHealthCheck:
		key := e.TestKey()
		if tt, ok := c.testTimings[key]; ok {
			tt.HealthCheck += e.Data.Duration
		}

	case events.TestSetupCompleted:
		key := e.TestKey()
		if tt, ok := c.testTimings[key]; ok {
			tt.Setup = e.Data.Duration
		}

	case events.TestTeardownCompleted:
		key := e.TestKey()
		if tt, ok := c.testTimings[key]; ok {
			tt.Teardown = e.Data.Duration
		}

	case events.TestExecCompleted:
		key := e.TestKey()
		if tt, ok := c.testTimings[key]; ok {
			tt.Execution = e.Data.Duration
		}

	case events.TestAssertResult:
		key := e.TestKey()
		if tt, ok := c.testTimings[key]; ok {
			tt.Assertion += e.Data.Duration
		}

	case events.TestCleanup:
		key := e.TestKey()
		if tt, ok := c.testTimings[key]; ok {
			tt.Cleanup = e.Data.Duration
		}

	case events.TestCompleted:
		key := e.TestKey()
		if tt, ok := c.testTimings[key]; ok {
			tt.EndTime = e.Timestamp
			tt.Total = e.Data.Duration
			tt.Passed = true
		}
		c.runTiming.PassedTests++

	case events.TestFailed:
		key := e.TestKey()
		if tt, ok := c.testTimings[key]; ok {
			tt.EndTime = e.Timestamp
			tt.Total = e.Data.Duration
			tt.Passed = false
			if e.Data.Error != nil {
				tt.Error = e.Data.Error.Error()
			} else if e.Data.Message != "" {
				tt.Error = e.Data.Message
			}
		}
		c.runTiming.FailedTests++

	case events.TestSkipped:
		c.runTiming.SkippedTests++
	}
}

// --- Query methods ---

// AllEvents returns all collected events.
func (c *EventCollector) AllEvents() []*events.Event {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]*events.Event, len(c.events))
	copy(result, c.events)
	return result
}

// EventsForTest returns all events for a specific test.
func (c *EventCollector) EventsForTest(suite, test string) []*events.Event {
	c.mu.RLock()
	defer c.mu.RUnlock()
	key := suite + "/" + test
	result := make([]*events.Event, len(c.byTest[key]))
	copy(result, c.byTest[key])
	return result
}

// EventsByType returns all events of a specific type.
func (c *EventCollector) EventsByType(t events.Type) []*events.Event {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]*events.Event, len(c.byType[t]))
	copy(result, c.byType[t])
	return result
}

// --- Timing methods (for benchmarks) ---

// TestTiming returns the timing breakdown for a test.
func (c *EventCollector) TestTiming(suite, test string) *TestTiming {
	c.mu.RLock()
	defer c.mu.RUnlock()
	key := suite + "/" + test
	if tt, ok := c.testTimings[key]; ok {
		copy := *tt
		return &copy
	}
	return nil
}

// AllTestTimings returns timing for all tests.
func (c *EventCollector) AllTestTimings() []*TestTiming {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*TestTiming, 0, len(c.testTimings))
	for _, tt := range c.testTimings {
		copy := *tt
		result = append(result, &copy)
	}

	// Sort by suite/test name for consistent order
	sort.Slice(result, func(i, j int) bool {
		if result[i].SuiteName != result[j].SuiteName {
			return result[i].SuiteName < result[j].SuiteName
		}
		return result[i].TestName < result[j].TestName
	})

	return result
}

// RunTiming returns the overall run timing.
func (c *EventCollector) RunTiming() *RunTiming {
	c.mu.RLock()
	defer c.mu.RUnlock()
	copy := *c.runTiming
	return &copy
}

// --- Benchmark helpers ---

// SlowestTests returns the N slowest tests by total duration.
func (c *EventCollector) SlowestTests(n int) []*TestTiming {
	all := c.AllTestTimings()
	sort.Slice(all, func(i, j int) bool {
		return all[i].Total > all[j].Total
	})
	if len(all) > n {
		all = all[:n]
	}
	return all
}

// SlowestByPhase returns the N tests with longest duration in a specific phase.
func (c *EventCollector) SlowestByPhase(phase events.Phase, n int) []*TestTiming {
	all := c.AllTestTimings()

	sort.Slice(all, func(i, j int) bool {
		return c.phaseDuration(all[i], phase) > c.phaseDuration(all[j], phase)
	})

	if len(all) > n {
		all = all[:n]
	}
	return all
}

func (c *EventCollector) phaseDuration(tt *TestTiming, phase events.Phase) time.Duration {
	switch phase {
	case events.PhaseServiceStartup:
		return tt.ServiceStartup
	case events.PhaseHealthCheck:
		return tt.HealthCheck
	case events.PhaseSetup:
		return tt.Setup
	case events.PhaseExecution:
		return tt.Execution
	case events.PhaseAssertion:
		return tt.Assertion
	case events.PhaseTeardown:
		return tt.Teardown
	case events.PhaseCleanup:
		return tt.Cleanup
	default:
		return 0
	}
}

// AverageTestDuration returns the average test duration.
func (c *EventCollector) AverageTestDuration() time.Duration {
	all := c.AllTestTimings()
	if len(all) == 0 {
		return 0
	}

	var total time.Duration
	for _, tt := range all {
		total += tt.Total
	}
	return total / time.Duration(len(all))
}

// PhaseBreakdown returns the total time spent in each phase across all tests.
func (c *EventCollector) PhaseBreakdown() map[events.Phase]time.Duration {
	all := c.AllTestTimings()
	breakdown := make(map[events.Phase]time.Duration)

	for _, tt := range all {
		breakdown[events.PhaseServiceStartup] += tt.ServiceStartup
		breakdown[events.PhaseHealthCheck] += tt.HealthCheck
		breakdown[events.PhaseSetup] += tt.Setup
		breakdown[events.PhaseExecution] += tt.Execution
		breakdown[events.PhaseAssertion] += tt.Assertion
		breakdown[events.PhaseTeardown] += tt.Teardown
		breakdown[events.PhaseCleanup] += tt.Cleanup
	}

	return breakdown
}
