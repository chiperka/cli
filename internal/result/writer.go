package result

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"chiperka-cli/internal/model"
)

// Writer persists run.json and test.json metadata into the result directory.
// Artifacts are already written by the collector during test execution with
// their final UUIDs — the writer only adds JSON metadata files.
type Writer struct {
	baseDir string // e.g. ".chiperka/results/runs"
}

// NewWriter creates a new result writer.
func NewWriter(baseDir string) *Writer {
	return &Writer{baseDir: baseDir}
}

// Persist writes run.json and test.json files alongside existing artifacts.
func (w *Writer) Persist(runUUID string, runResult *model.RunResult, startedAt time.Time) error {
	runDir := filepath.Join(w.baseDir, runUUID)

	summary := RunSummary{
		UUID:      runUUID,
		StartedAt: startedAt,
		Duration:  totalDuration(runResult),
		Passed:    runResult.TotalPassed(),
		Failed:    runResult.TotalFailed(),
		Errored:   runResult.TotalErrors(),
		Skipped:   runResult.TotalSkipped(),
		Total:     runResult.TotalTests(),
	}

	if runResult.TotalErrors() > 0 {
		summary.Status = "error"
	} else if runResult.HasFailures() {
		summary.Status = "failed"
	} else {
		summary.Status = "passed"
	}

	index := &Index{
		Tests: make(map[string]string),
	}

	for _, sr := range runResult.SuiteResults {
		for _, tr := range sr.TestResults {
			testDir := filepath.Join(runDir, "tests", tr.UUID)
			if err := os.MkdirAll(testDir, 0755); err != nil {
				return fmt.Errorf("create test directory: %w", err)
			}

			var artifactRefs []ArtifactRef
			for _, a := range tr.Artifacts {
				artifactRefs = append(artifactRefs, ArtifactRef{
					Name: a.Name,
					Size: a.Size,
				})
			}

			// Write test.json
			detail := buildTestDetail(tr.UUID, sr.Suite.Name, &tr, artifactRefs)
			if err := writeJSON(filepath.Join(testDir, "test.json"), detail); err != nil {
				return fmt.Errorf("write test.json: %w", err)
			}

			summary.Tests = append(summary.Tests, TestRef{
				UUID:     tr.UUID,
				Name:     tr.Test.Name,
				Suite:    sr.Suite.Name,
				Status:   string(tr.Status),
				Duration: tr.Duration.Milliseconds(),
			})

			index.Tests[tr.UUID] = runUUID
		}
	}

	if err := writeJSON(filepath.Join(runDir, "run.json"), summary); err != nil {
		return fmt.Errorf("write run.json: %w", err)
	}

	if err := w.updateIndex(index); err != nil {
		return fmt.Errorf("update index: %w", err)
	}

	return nil
}

func buildTestDetail(testUUID, suiteName string, tr *model.TestResult, artifacts []ArtifactRef) TestDetail {
	detail := TestDetail{
		UUID:     testUUID,
		Name:     tr.Test.Name,
		Suite:    suiteName,
		Status:   string(tr.Status),
		Duration: tr.Duration.Milliseconds(),
		Phases: PhaseBreakdown{
			Network:   tr.NetworkDuration.Milliseconds(),
			Services:  tr.ServicesDuration.Milliseconds(),
			Setup:     tr.SetupDuration.Milliseconds(),
			Execution: tr.ExecutionDuration.Milliseconds(),
			Assertion: tr.AssertionDuration.Milliseconds(),
			Teardown:  tr.TeardownDuration.Milliseconds(),
			Cleanup:   tr.CleanupDuration.Milliseconds(),
		},
		Artifacts: artifacts,
	}

	if tr.Error != nil {
		detail.Error = tr.Error.Error()
	}

	for _, a := range tr.AssertionResults {
		detail.Assertions = append(detail.Assertions, AssertionDetail{
			Passed:   a.Passed,
			Type:     a.Type,
			Expected: a.Expected,
			Actual:   a.Actual,
			Message:  a.Message,
		})
	}

	for _, h := range tr.HTTPExchanges {
		entry := HTTPExchangeJSON{
			Phase:          h.Phase,
			Sequence:       h.PhaseSeq,
			Method:         h.RequestMethod,
			URL:            h.RequestURL,
			RequestHeaders: h.RequestHeaders,
			RequestBody:    h.RequestBody,
			StatusCode:     h.ResponseStatusCode,
			ResponseBody:   h.ResponseBody,
			Duration:       h.Duration.Milliseconds(),
		}
		if h.Error != nil {
			entry.Error = h.Error.Error()
		}
		detail.HTTPExchanges = append(detail.HTTPExchanges, entry)
	}

	for _, c := range tr.CLIExecutions {
		entry := CLIExecutionJSON{
			Phase:      c.Phase,
			Sequence:   c.PhaseSeq,
			Service:    c.Service,
			Command:    c.Command,
			WorkingDir: c.WorkingDir,
			ExitCode:   c.ExitCode,
			Stdout:     c.Stdout,
			Stderr:     c.Stderr,
			Duration:   c.Duration.Milliseconds(),
		}
		if c.Error != nil {
			entry.Error = c.Error.Error()
		}
		detail.CLIExecutions = append(detail.CLIExecutions, entry)
	}

	for _, s := range tr.ServiceResults {
		detail.Services = append(detail.Services, ServiceDetail{
			Name:     s.Name,
			Image:    s.Image,
			Duration: s.Duration.Milliseconds(),
		})
	}

	for _, s := range tr.SetupResults {
		step := StepDetail{
			Type:     s.Type,
			Duration: s.Duration.Milliseconds(),
			Success:  s.Success,
		}
		if s.Error != nil {
			step.Error = s.Error.Error()
		}
		detail.Setup = append(detail.Setup, step)
	}

	for _, s := range tr.TeardownResults {
		step := StepDetail{
			Type:     s.Type,
			Duration: s.Duration.Milliseconds(),
			Success:  s.Success,
		}
		if s.Error != nil {
			step.Error = s.Error.Error()
		}
		detail.Teardown = append(detail.Teardown, step)
	}

	return detail
}

// Index maps test UUIDs to their parent run UUID for fast lookup.
type Index struct {
	Tests map[string]string `json:"tests"` // testUUID → runUUID
}

func (w *Writer) updateIndex(newEntries *Index) error {
	indexPath := filepath.Join(filepath.Dir(w.baseDir), "index.json")

	existing := &Index{
		Tests: make(map[string]string),
	}

	if data, err := os.ReadFile(indexPath); err == nil {
		json.Unmarshal(data, existing)
	}

	for k, v := range newEntries.Tests {
		existing.Tests[k] = v
	}

	return writeJSON(indexPath, existing)
}

func totalDuration(r *model.RunResult) int64 {
	var max time.Duration
	for _, sr := range r.SuiteResults {
		for _, tr := range sr.TestResults {
			if tr.Duration > max {
				max = tr.Duration
			}
		}
	}
	return max.Milliseconds()
}

func writeJSON(path string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
