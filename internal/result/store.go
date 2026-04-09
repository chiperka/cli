package result

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Store provides read access to persisted test results.
type Store interface {
	ListRuns(limit int) ([]RunSummary, error)
	GetRun(uuid string) (*RunSummary, error)
	GetTest(uuid string) (*TestDetail, error)
	GetArtifact(testUUID string, name string) (content []byte, err error)
}

// LocalStore reads results from the local filesystem.
type LocalStore struct {
	baseDir  string // e.g. ".chiperka/results"
	runsDir  string // e.g. ".chiperka/results/runs"
}

// NewLocalStore creates a new local result store.
func NewLocalStore(baseDir string) *LocalStore {
	return &LocalStore{
		baseDir: baseDir,
		runsDir: filepath.Join(baseDir, "runs"),
	}
}

// DefaultLocalStore returns a store using the default .chiperka/results directory.
func DefaultLocalStore() *LocalStore {
	return NewLocalStore(".chiperka/results")
}

// ListRuns returns the most recent runs, sorted by start time descending.
func (s *LocalStore) ListRuns(limit int) ([]RunSummary, error) {
	entries, err := os.ReadDir(s.runsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read runs directory: %w", err)
	}

	var runs []RunSummary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runJSON := filepath.Join(s.runsDir, entry.Name(), "run.json")
		data, err := os.ReadFile(runJSON)
		if err != nil {
			continue
		}
		var summary RunSummary
		if err := json.Unmarshal(data, &summary); err != nil {
			continue
		}
		runs = append(runs, summary)
	}

	// Sort by start time descending (most recent first)
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].StartedAt.After(runs[j].StartedAt)
	})

	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}

	return runs, nil
}

// GetRun returns the run summary for a given run UUID.
func (s *LocalStore) GetRun(uuid string) (*RunSummary, error) {
	runJSON := filepath.Join(s.runsDir, uuid, "run.json")
	data, err := os.ReadFile(runJSON)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("run not found: %s", uuid)
		}
		return nil, fmt.Errorf("read run: %w", err)
	}

	var summary RunSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, fmt.Errorf("parse run: %w", err)
	}
	return &summary, nil
}

// GetTest returns the test detail for a given test UUID.
// Uses the index to find which run contains this test.
func (s *LocalStore) GetTest(uuid string) (*TestDetail, error) {
	runUUID, err := s.lookupTestRun(uuid)
	if err != nil {
		return nil, err
	}

	testJSON := filepath.Join(s.runsDir, runUUID, "tests", uuid, "test.json")
	data, err := os.ReadFile(testJSON)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("test not found: %s", uuid)
		}
		return nil, fmt.Errorf("read test: %w", err)
	}

	var detail TestDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		return nil, fmt.Errorf("parse test: %w", err)
	}
	return &detail, nil
}

// GetArtifact returns the raw content of an artifact by test UUID and filename.
func (s *LocalStore) GetArtifact(testUUID string, name string) ([]byte, error) {
	runUUID, err := s.lookupTestRun(testUUID)
	if err != nil {
		return nil, err
	}

	filePath := filepath.Join(s.runsDir, runUUID, "tests", testUUID, "artifacts", name)
	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("artifact not found: %s/%s", testUUID, name)
		}
		return nil, fmt.Errorf("read artifact: %w", err)
	}
	return content, nil
}

func (s *LocalStore) lookupTestRun(testUUID string) (string, error) {
	index, err := s.readIndex()
	if err != nil {
		return "", err
	}
	runUUID, ok := index.Tests[testUUID]
	if !ok {
		return "", fmt.Errorf("test not found in index: %s", testUUID)
	}
	return runUUID, nil
}

func (s *LocalStore) readIndex() (*Index, error) {
	indexPath := filepath.Join(s.baseDir, "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no results stored yet (index.json not found)")
		}
		return nil, fmt.Errorf("read index: %w", err)
	}
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("parse index: %w", err)
	}
	return &index, nil
}
