package model

import (
	"fmt"
	"testing"
	"time"
)

func TestResult_TestResult_IsPassed(t *testing.T) {
	tr := TestResult{Status: StatusPassed}
	if !tr.IsPassed() {
		t.Errorf("expected IsPassed=true for passed test")
	}
	tr.Status = StatusFailed
	if tr.IsPassed() {
		t.Errorf("expected IsPassed=false for failed test")
	}
}

func TestResult_SuiteResult_PassedCount(t *testing.T) {
	sr := SuiteResult{
		TestResults: []TestResult{
			{Status: StatusPassed},
			{Status: StatusPassed},
			{Status: StatusFailed},
		},
	}
	if sr.PassedCount() != 2 {
		t.Errorf("expected 2, got %d", sr.PassedCount())
	}
}

func TestResult_SuiteResult_FailedCount(t *testing.T) {
	sr := SuiteResult{
		TestResults: []TestResult{
			{Status: StatusPassed},
			{Status: StatusFailed},
			{Status: StatusError},
			{Status: StatusSkipped},
		},
	}
	if sr.FailedCount() != 2 {
		t.Errorf("expected 2 (failed + error), got %d", sr.FailedCount())
	}
}

func TestResult_SuiteResult_SkippedCount(t *testing.T) {
	sr := SuiteResult{
		TestResults: []TestResult{
			{Status: StatusPassed},
			{Status: StatusSkipped},
			{Status: StatusSkipped},
		},
	}
	if sr.SkippedCount() != 2 {
		t.Errorf("expected 2, got %d", sr.SkippedCount())
	}
}

func TestResult_RunResult_TotalPassed(t *testing.T) {
	rr := RunResult{
		SuiteResults: []SuiteResult{
			{TestResults: []TestResult{
				{Status: StatusPassed},
				{Status: StatusFailed},
			}},
			{TestResults: []TestResult{
				{Status: StatusPassed},
				{Status: StatusPassed},
			}},
		},
	}
	if rr.TotalPassed() != 3 {
		t.Errorf("expected 3, got %d", rr.TotalPassed())
	}
}

func TestResult_RunResult_TotalFailed(t *testing.T) {
	rr := RunResult{
		SuiteResults: []SuiteResult{
			{TestResults: []TestResult{
				{Status: StatusFailed},
				{Status: StatusError},
			}},
			{TestResults: []TestResult{
				{Status: StatusPassed},
			}},
		},
	}
	if rr.TotalFailed() != 2 {
		t.Errorf("expected 2, got %d", rr.TotalFailed())
	}
}

func TestResult_RunResult_TotalTests(t *testing.T) {
	rr := RunResult{
		SuiteResults: []SuiteResult{
			{TestResults: []TestResult{{}, {}}},
			{TestResults: []TestResult{{}}},
		},
	}
	if rr.TotalTests() != 3 {
		t.Errorf("expected 3, got %d", rr.TotalTests())
	}
}

func TestResult_RunResult_TotalErrors(t *testing.T) {
	rr := RunResult{
		SuiteResults: []SuiteResult{
			{TestResults: []TestResult{
				{Status: StatusError},
				{Status: StatusFailed},
				{Status: StatusError},
			}},
		},
	}
	if rr.TotalErrors() != 2 {
		t.Errorf("expected 2, got %d", rr.TotalErrors())
	}
}

func TestResult_RunResult_TotalSkipped(t *testing.T) {
	rr := RunResult{
		SuiteResults: []SuiteResult{
			{TestResults: []TestResult{
				{Status: StatusSkipped},
				{Status: StatusPassed},
			}},
			{TestResults: []TestResult{
				{Status: StatusSkipped},
			}},
		},
	}
	if rr.TotalSkipped() != 2 {
		t.Errorf("expected 2, got %d", rr.TotalSkipped())
	}
}

func TestResult_RunResult_HasFailures_True(t *testing.T) {
	rr := RunResult{
		SuiteResults: []SuiteResult{
			{TestResults: []TestResult{{Status: StatusFailed}}},
		},
	}
	if !rr.HasFailures() {
		t.Errorf("expected HasFailures=true")
	}
}

func TestResult_RunResult_HasFailures_False(t *testing.T) {
	rr := RunResult{
		SuiteResults: []SuiteResult{
			{TestResults: []TestResult{{Status: StatusPassed}}},
		},
	}
	if rr.HasFailures() {
		t.Errorf("expected HasFailures=false")
	}
}

func TestResult_RunResult_Empty(t *testing.T) {
	rr := RunResult{}
	if rr.TotalTests() != 0 {
		t.Errorf("expected 0 total tests, got %d", rr.TotalTests())
	}
	if rr.TotalPassed() != 0 {
		t.Errorf("expected 0 passed, got %d", rr.TotalPassed())
	}
	if rr.HasFailures() {
		t.Errorf("expected no failures for empty result")
	}
}

func TestResult_RunResult_MixedStatuses(t *testing.T) {
	rr := RunResult{
		SuiteResults: []SuiteResult{
			{TestResults: []TestResult{
				{Status: StatusPassed, Duration: 100 * time.Millisecond},
				{Status: StatusFailed, Duration: 200 * time.Millisecond, Error: fmt.Errorf("assertion failed")},
				{Status: StatusError, Duration: 50 * time.Millisecond, Error: fmt.Errorf("timeout")},
				{Status: StatusSkipped},
			}},
		},
	}
	if rr.TotalTests() != 4 {
		t.Errorf("expected 4 total, got %d", rr.TotalTests())
	}
	if rr.TotalPassed() != 1 {
		t.Errorf("expected 1 passed, got %d", rr.TotalPassed())
	}
	if rr.TotalFailed() != 2 {
		t.Errorf("expected 2 failed, got %d", rr.TotalFailed())
	}
	if rr.TotalErrors() != 1 {
		t.Errorf("expected 1 error, got %d", rr.TotalErrors())
	}
	if rr.TotalSkipped() != 1 {
		t.Errorf("expected 1 skipped, got %d", rr.TotalSkipped())
	}
}
