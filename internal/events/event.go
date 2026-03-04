package events

import "time"

// Event represents a lifecycle event in the test runner.
type Event struct {
	// Type identifies what kind of event this is
	Type Type

	// Timestamp when the event occurred
	Timestamp time.Time

	// Context identifiers - which test/run this belongs to
	RunID     string
	SuiteName string
	TestName  string
	TestUUID  string
	FilePath  string

	// Data contains event-specific information
	Data EventData
}

// EventData holds the payload for an event.
type EventData struct {
	// Message is a human-readable description
	Message string

	// Duration for completed events
	Duration time.Duration

	// Error if something failed
	Error error

	// Progress tracking
	Current int
	Total   int

	// Status for healthchecks, assertions, etc.
	Status string // "pass", "fail", "waiting", "retry"

	// Details for flexible event-specific data
	// Examples:
	//   - docker.command: {"command": "docker run ...", "args": [...]}
	//   - test.assert.result: {"assertion": "status_code", "expected": 200, "actual": 404}
	//   - container.started: {"container_id": "abc123", "image": "nginx:latest"}
	Details map[string]any
}

// NewEvent creates a new event with the current timestamp.
func NewEvent(eventType Type) *Event {
	return &Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Data: EventData{
			Details: make(map[string]any),
		},
	}
}

// NewTestEvent creates an event in the context of a test.
func NewTestEvent(eventType Type, suite, test string) *Event {
	e := NewEvent(eventType)
	e.SuiteName = suite
	e.TestName = test
	return e
}

// NewDockerEvent creates an event for docker operations (debug mode).
func NewDockerEvent(eventType Type, command string, args []string) *Event {
	e := NewEvent(eventType)
	e.Data.Details["command"] = command
	e.Data.Details["args"] = args
	return e
}

// WithMessage sets the message and returns the event for chaining.
func (e *Event) WithMessage(msg string) *Event {
	e.Data.Message = msg
	return e
}

// WithDuration sets the duration and returns the event for chaining.
func (e *Event) WithDuration(d time.Duration) *Event {
	e.Data.Duration = d
	return e
}

// WithError sets the error and returns the event for chaining.
func (e *Event) WithError(err error) *Event {
	e.Data.Error = err
	return e
}

// WithProgress sets current/total progress and returns the event for chaining.
func (e *Event) WithProgress(current, total int) *Event {
	e.Data.Current = current
	e.Data.Total = total
	return e
}

// WithStatus sets the status and returns the event for chaining.
func (e *Event) WithStatus(status string) *Event {
	e.Data.Status = status
	return e
}

// WithDetail adds a detail key-value pair and returns the event for chaining.
func (e *Event) WithDetail(key string, value any) *Event {
	if e.Data.Details == nil {
		e.Data.Details = make(map[string]any)
	}
	e.Data.Details[key] = value
	return e
}

// TestKey returns a unique key for identifying events by test.
func (e *Event) TestKey() string {
	if e.SuiteName == "" && e.TestName == "" {
		return ""
	}
	return e.SuiteName + "/" + e.TestName
}
