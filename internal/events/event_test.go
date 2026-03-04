package events

import (
	"testing"
)

func TestEvent_NewEvent(t *testing.T) {
	e := NewEvent(TestStarted)
	if e.Type != TestStarted {
		t.Errorf("expected TestStarted, got %q", e.Type)
	}
	if e.Timestamp.IsZero() {
		t.Errorf("expected timestamp to be set")
	}
	if e.Data.Details == nil {
		t.Errorf("expected Details to be initialized")
	}
}

func TestEvent_NewTestEvent(t *testing.T) {
	e := NewTestEvent(TestStarted, "suite1", "test1")
	if e.Type != TestStarted {
		t.Errorf("expected TestStarted, got %q", e.Type)
	}
	if e.SuiteName != "suite1" {
		t.Errorf("expected suite1, got %q", e.SuiteName)
	}
	if e.TestName != "test1" {
		t.Errorf("expected test1, got %q", e.TestName)
	}
}

func TestEvent_NewDockerEvent(t *testing.T) {
	e := NewDockerEvent(DockerCommand, "docker", []string{"run", "nginx"})
	if e.Type != DockerCommand {
		t.Errorf("expected DockerCommand, got %q", e.Type)
	}
	cmd, ok := e.Data.Details["command"].(string)
	if !ok || cmd != "docker" {
		t.Errorf("expected command=docker, got %v", e.Data.Details["command"])
	}
	args, ok := e.Data.Details["args"].([]string)
	if !ok || len(args) != 2 {
		t.Errorf("expected 2 args, got %v", e.Data.Details["args"])
	}
}

func TestEvent_WithMessage(t *testing.T) {
	e := NewEvent(TestStarted).WithMessage("hello")
	if e.Data.Message != "hello" {
		t.Errorf("expected 'hello', got %q", e.Data.Message)
	}
}

func TestEvent_WithDuration(t *testing.T) {
	e := NewEvent(TestStarted).WithDuration(100)
	if e.Data.Duration != 100 {
		t.Errorf("expected 100, got %d", e.Data.Duration)
	}
}

func TestEvent_WithError(t *testing.T) {
	err := &testError{msg: "oops"}
	e := NewEvent(TestFailed).WithError(err)
	if e.Data.Error == nil {
		t.Fatalf("expected error to be set")
	}
	if e.Data.Error.Error() != "oops" {
		t.Errorf("expected 'oops', got %q", e.Data.Error.Error())
	}
}

func TestEvent_WithProgress(t *testing.T) {
	e := NewEvent(TestStarted).WithProgress(5, 10)
	if e.Data.Current != 5 {
		t.Errorf("expected current=5, got %d", e.Data.Current)
	}
	if e.Data.Total != 10 {
		t.Errorf("expected total=10, got %d", e.Data.Total)
	}
}

func TestEvent_WithStatus(t *testing.T) {
	e := NewEvent(TestStarted).WithStatus("pass")
	if e.Data.Status != "pass" {
		t.Errorf("expected 'pass', got %q", e.Data.Status)
	}
}

func TestEvent_WithDetail(t *testing.T) {
	e := NewEvent(TestStarted).WithDetail("key", "value")
	if e.Data.Details["key"] != "value" {
		t.Errorf("expected 'value', got %v", e.Data.Details["key"])
	}
}

func TestEvent_WithDetail_NilDetails(t *testing.T) {
	e := &Event{Type: TestStarted}
	e.WithDetail("key", "value")
	if e.Data.Details["key"] != "value" {
		t.Errorf("expected 'value', got %v", e.Data.Details["key"])
	}
}

func TestEvent_BuilderChaining(t *testing.T) {
	e := NewEvent(TestCompleted).
		WithMessage("done").
		WithDuration(500).
		WithStatus("pass").
		WithDetail("tests", 10)

	if e.Data.Message != "done" {
		t.Errorf("expected message 'done'")
	}
	if e.Data.Duration != 500 {
		t.Errorf("expected duration 500")
	}
	if e.Data.Status != "pass" {
		t.Errorf("expected status 'pass'")
	}
	if e.Data.Details["tests"] != 10 {
		t.Errorf("expected tests=10")
	}
}

func TestEvent_TestKey(t *testing.T) {
	e := NewTestEvent(TestStarted, "suite1", "test1")
	if e.TestKey() != "suite1/test1" {
		t.Errorf("expected 'suite1/test1', got %q", e.TestKey())
	}
}

func TestEvent_TestKey_Empty(t *testing.T) {
	e := NewEvent(RunStarted)
	if e.TestKey() != "" {
		t.Errorf("expected empty TestKey, got %q", e.TestKey())
	}
}

func TestEvent_TestKey_OnlySuite(t *testing.T) {
	e := NewEvent(TestStarted)
	e.SuiteName = "suite1"
	// TestName is empty but SuiteName is set - still returns key
	if e.TestKey() != "suite1/" {
		t.Errorf("expected 'suite1/', got %q", e.TestKey())
	}
}

// testError is a simple error implementation for testing.
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
