package cloud

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"chiperka-cli/internal/model"
)

// --- CollectSnapshotFiles ---

func TestCollectSnapshotFiles_ResponseBodySnapshot(t *testing.T) {
	dir := t.TempDir()

	// Create snapshot file
	snapshotDir := filepath.Join(dir, "tests", "__snapshots__")
	if err := os.MkdirAll(snapshotDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshotDir, "response.json"), []byte(`{"ok":true}`), 0644); err != nil {
		t.Fatal(err)
	}

	suites := []SuiteSubmission{
		{
			Name:     "suite1",
			FilePath: filepath.Join(dir, "tests", "suite.chiperka"),
			Tests: []model.Test{
				{
					Name: "test1",
					Assertions: []model.Assertion{
						{
							Response: &model.ResponseAssertion{
								Body: &model.ResponseBodyAssertion{
									Snapshot: "__snapshots__/response.json",
								},
							},
						},
					},
				},
			},
		},
	}

	snapshots, err := CollectSnapshotFiles(suites)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}

	// Verify the key is the correct path (suiteDir + snapshotRelPath with forward slashes)
	expectedKey := filepath.ToSlash(filepath.Join(dir, "tests", "__snapshots__/response.json"))
	content, ok := snapshots[expectedKey]
	if !ok {
		// Print all keys for debugging
		for k := range snapshots {
			t.Logf("  key: %q", k)
		}
		t.Fatalf("expected key %q not found", expectedKey)
	}
	if string(content) != `{"ok":true}` {
		t.Errorf("unexpected content: %q", string(content))
	}
}

func TestCollectSnapshotFiles_NoSnapshots(t *testing.T) {
	suites := []SuiteSubmission{
		{
			Name:     "suite1",
			FilePath: "tests/suite.chiperka",
			Tests: []model.Test{
				{
					Name: "test1",
					Assertions: []model.Assertion{
						{
							Response: &model.ResponseAssertion{
								StatusCode: intPtr(200),
							},
						},
					},
				},
			},
		},
	}

	snapshots, err := CollectSnapshotFiles(suites)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snapshots) != 0 {
		t.Errorf("expected 0 snapshots, got %d", len(snapshots))
	}
}

func TestCollectSnapshotFiles_Deduplication(t *testing.T) {
	dir := t.TempDir()
	snapshotDir := filepath.Join(dir, "tests", "__snapshots__")
	os.MkdirAll(snapshotDir, 0755)
	os.WriteFile(filepath.Join(snapshotDir, "response.json"), []byte("content"), 0644)

	// Two tests referencing the same snapshot file
	suites := []SuiteSubmission{
		{
			Name:     "suite1",
			FilePath: filepath.Join(dir, "tests", "suite.chiperka"),
			Tests: []model.Test{
				{
					Name: "test1",
					Assertions: []model.Assertion{
						{Response: &model.ResponseAssertion{
							Body: &model.ResponseBodyAssertion{Snapshot: "__snapshots__/response.json"},
						}},
					},
				},
				{
					Name: "test2",
					Assertions: []model.Assertion{
						{Response: &model.ResponseAssertion{
							Body: &model.ResponseBodyAssertion{Snapshot: "__snapshots__/response.json"},
						}},
					},
				},
			},
		},
	}

	snapshots, err := CollectSnapshotFiles(suites)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snapshots) != 1 {
		t.Errorf("expected 1 snapshot (deduplicated), got %d", len(snapshots))
	}
}

func TestCollectSnapshotFiles_AllAssertionTypes(t *testing.T) {
	dir := t.TempDir()
	snapshotDir := filepath.Join(dir, "tests", "__snapshots__")
	os.MkdirAll(snapshotDir, 0755)

	files := []string{"body.json", "stdout.txt", "stderr.txt", "artifact.bin"}
	for _, f := range files {
		os.WriteFile(filepath.Join(snapshotDir, f), []byte("content-"+f), 0644)
	}

	suites := []SuiteSubmission{
		{
			Name:     "suite1",
			FilePath: filepath.Join(dir, "tests", "suite.chiperka"),
			Tests: []model.Test{
				{
					Name: "test1",
					Assertions: []model.Assertion{
						{Response: &model.ResponseAssertion{
							Body: &model.ResponseBodyAssertion{Snapshot: "__snapshots__/body.json"},
						}},
						{CLI: &model.CLIAssertion{
							Stdout: &model.CLIOutputAssertion{Snapshot: "__snapshots__/stdout.txt"},
							Stderr: &model.CLIOutputAssertion{Snapshot: "__snapshots__/stderr.txt"},
						}},
						{Artifact: &model.ArtifactAssertion{
							Name:     "output",
							Snapshot: "__snapshots__/artifact.bin",
						}},
					},
				},
			},
		},
	}

	snapshots, err := CollectSnapshotFiles(suites)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snapshots) != 4 {
		t.Errorf("expected 4 snapshots, got %d", len(snapshots))
		for k := range snapshots {
			t.Logf("  key: %q", k)
		}
	}
}

func TestCollectSnapshotFiles_MissingFileReturnsError(t *testing.T) {
	dir := t.TempDir()
	snapshotDir := filepath.Join(dir, "tests", "__snapshots__")
	os.MkdirAll(snapshotDir, 0755)
	// Only create one file, the other is missing
	os.WriteFile(filepath.Join(snapshotDir, "exists.json"), []byte("ok"), 0644)

	suites := []SuiteSubmission{
		{
			Name:     "suite1",
			FilePath: filepath.Join(dir, "tests", "suite.chiperka"),
			Tests: []model.Test{
				{
					Name: "test1",
					Assertions: []model.Assertion{
						{Response: &model.ResponseAssertion{
							Body: &model.ResponseBodyAssertion{Snapshot: "__snapshots__/exists.json"},
						}},
						{Response: &model.ResponseAssertion{
							Body: &model.ResponseBodyAssertion{Snapshot: "__snapshots__/missing.json"},
						}},
					},
				},
			},
		},
	}

	_, err := CollectSnapshotFiles(suites)
	if err == nil {
		t.Fatal("expected error for missing snapshot file, got nil")
	}
	// Verify error message mentions the missing file
	if !strings.Contains(err.Error(), "missing.json") {
		t.Errorf("error should mention the missing file, got: %v", err)
	}
	if !strings.Contains(err.Error(), "suite.chiperka") {
		t.Errorf("error should mention the suite file, got: %v", err)
	}
}

func TestCollectSnapshotFiles_RelativePaths(t *testing.T) {
	// Test with relative suite file path (as BuildSubmission normalizes to)
	dir := t.TempDir()

	snapshotDir := filepath.Join(dir, "tests", "__snapshots__")
	os.MkdirAll(snapshotDir, 0755)
	os.WriteFile(filepath.Join(snapshotDir, "resp.json"), []byte("data"), 0644)

	// Use absolute path for FilePath (as the parser would return before BuildSubmission)
	suites := []SuiteSubmission{
		{
			Name:     "suite1",
			FilePath: filepath.Join(dir, "tests", "suite.chiperka"),
			Tests: []model.Test{
				{
					Name: "test1",
					Assertions: []model.Assertion{
						{Response: &model.ResponseAssertion{
							Body: &model.ResponseBodyAssertion{Snapshot: "__snapshots__/resp.json"},
						}},
					},
				},
			},
		},
	}

	snapshots, err := CollectSnapshotFiles(suites)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}

	// The key should be suiteDir + snapshotRelPath
	expectedKey := filepath.ToSlash(filepath.Join(dir, "tests", "__snapshots__/resp.json"))
	if _, ok := snapshots[expectedKey]; !ok {
		for k := range snapshots {
			t.Logf("  got key: %q", k)
		}
		t.Errorf("expected key %q not found", expectedKey)
	}
}

// --- UploadSnapshots ---

func TestUploadSnapshots_CreatesValidZip(t *testing.T) {
	var receivedBody []byte
	var receivedContentType string
	var receivedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedContentType = r.Header.Get("Content-Type")
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	snapshots := map[string][]byte{
		"tests/__snapshots__/response.json": []byte(`{"status":"ok"}`),
		"tests/__snapshots__/body.txt":      []byte("hello world"),
	}

	if err := client.UploadSnapshots("run-123", snapshots); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify endpoint
	if receivedPath != "/api/runs/run-123/snapshots" {
		t.Errorf("expected path /api/runs/run-123/snapshots, got %q", receivedPath)
	}

	// Verify content type
	if receivedContentType != "application/zip" {
		t.Errorf("expected content-type application/zip, got %q", receivedContentType)
	}

	// Verify zip is valid and contains expected entries
	zr, err := zip.NewReader(bytes.NewReader(receivedBody), int64(len(receivedBody)))
	if err != nil {
		t.Fatalf("received body is not a valid zip: %v", err)
	}

	entries := make(map[string]string)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("failed to open zip entry: %v", err)
		}
		content, _ := io.ReadAll(rc)
		rc.Close()
		entries[f.Name] = string(content)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 zip entries, got %d", len(entries))
	}

	if entries["tests/__snapshots__/response.json"] != `{"status":"ok"}` {
		t.Errorf("unexpected content for response.json: %q", entries["tests/__snapshots__/response.json"])
	}
	if entries["tests/__snapshots__/body.txt"] != "hello world" {
		t.Errorf("unexpected content for body.txt: %q", entries["tests/__snapshots__/body.txt"])
	}
}

func TestUploadSnapshots_WithAuth(t *testing.T) {
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	err := client.UploadSnapshots("run-1", map[string][]byte{"a.json": []byte("{}")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedAuth != "Bearer test-token" {
		t.Errorf("expected 'Bearer test-token', got %q", receivedAuth)
	}
}

func TestUploadSnapshots_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	err := client.UploadSnapshots("run-1", map[string][]byte{"a.json": []byte("{}")})
	if err == nil {
		t.Errorf("expected error for 500 status")
	}
}

func TestUploadSnapshots_EmptyMap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// Should still be a valid (empty) zip
		zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			t.Errorf("expected valid zip: %v", err)
		}
		if len(zr.File) != 0 {
			t.Errorf("expected 0 entries, got %d", len(zr.File))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	err := client.UploadSnapshots("run-1", map[string][]byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Path consistency: CLI upload key must match worker download key ---

func TestSnapshotPathConsistency_RelativeSuitePath(t *testing.T) {
	// This test verifies that the zip key created by CollectSnapshotFiles
	// matches the API path constructed by the worker's downloadSnapshots.
	//
	// CLI side (CollectSnapshotFiles):
	//   suiteDir = filepath.Dir(suite.FilePath)
	//   zipKey = filepath.ToSlash(filepath.Join(suiteDir, snapshotRelPath))
	//
	// Worker side (downloadSnapshots):
	//   apiPath = filepath.ToSlash(filepath.Join(filepath.Dir(suiteFilePath), snapshotPath))
	//
	// For these to match, suite.FilePath in the submission must equal
	// claimed.SuiteFilePath in the worker's claim response.

	dir := t.TempDir()
	snapshotDir := filepath.Join(dir, "tests", "__snapshots__")
	os.MkdirAll(snapshotDir, 0755)
	os.WriteFile(filepath.Join(snapshotDir, "response.json"), []byte("data"), 0644)

	// Simulate what BuildSubmission does: normalize to relative path
	absPath := filepath.Join(dir, "tests", "suite.chiperka")

	suites := []SuiteSubmission{
		{
			Name:     "suite1",
			FilePath: absPath,
			Tests: []model.Test{
				{
					Name: "test1",
					Assertions: []model.Assertion{
						{Response: &model.ResponseAssertion{
							Body: &model.ResponseBodyAssertion{Snapshot: "__snapshots__/response.json"},
						}},
					},
				},
			},
		},
	}

	snapshots, err := CollectSnapshotFiles(suites)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Get the actual zip key that CLI would store
	var cliZipKey string
	for k := range snapshots {
		cliZipKey = k
	}

	// Simulate what worker does: construct API path from claimed.SuiteFilePath
	claimedSuiteFilePath := absPath // This is what the API returns to the worker
	snapshotRelPath := "__snapshots__/response.json"
	workerAPIPath := filepath.ToSlash(filepath.Join(filepath.Dir(claimedSuiteFilePath), snapshotRelPath))

	if cliZipKey != workerAPIPath {
		t.Errorf("PATH MISMATCH!\n  CLI zip key:    %q\n  Worker API path: %q", cliZipKey, workerAPIPath)
	}
}

func TestSnapshotPathConsistency_AfterBuildSubmission(t *testing.T) {
	// This is the critical test: BuildSubmission normalizes FilePath to relative,
	// but the worker receives suiteFilePath from the API which stores whatever was submitted.
	// If these diverge, snapshot download will fail with 404.

	dir := t.TempDir()
	snapshotDir := filepath.Join(dir, "__snapshots__")
	os.MkdirAll(snapshotDir, 0755)
	os.WriteFile(filepath.Join(snapshotDir, "response.json"), []byte("data"), 0644)

	// Create test collection with absolute path (as parser produces)
	tests := model.NewTestCollection()
	tests.AddSuite(model.Suite{
		Name:     "suite1",
		FilePath: filepath.Join(dir, "suite.chiperka"),
		Tests: []model.Test{
			{
				Name: "test1",
				Assertions: []model.Assertion{
					{Response: &model.ResponseAssertion{
						Body: &model.ResponseBodyAssertion{Snapshot: "__snapshots__/response.json"},
					}},
				},
			},
		},
	})

	services := model.NewServiceTemplateCollection()

	// BuildSubmission normalizes to relative path
	submission, err := BuildSubmission(tests, services, "1.0.0", nil)
	if err != nil {
		t.Fatalf("BuildSubmission error: %v", err)
	}

	// CollectSnapshotFiles uses the (potentially normalized) path
	snapshots, err := CollectSnapshotFiles(submission.Suites)
	if err != nil {
		t.Fatalf("CollectSnapshotFiles error: %v", err)
	}

	var cliZipKey string
	for k := range snapshots {
		cliZipKey = k
	}

	// The worker receives suiteFilePath from the API (which stored submission.Suites[0].FilePath)
	workerSuiteFilePath := submission.Suites[0].FilePath
	snapshotRelPath := "__snapshots__/response.json"
	workerAPIPath := filepath.ToSlash(filepath.Join(filepath.Dir(workerSuiteFilePath), snapshotRelPath))

	t.Logf("BuildSubmission FilePath: %q", submission.Suites[0].FilePath)
	t.Logf("CLI zip key:              %q", cliZipKey)
	t.Logf("Worker API path:          %q", workerAPIPath)

	if cliZipKey != workerAPIPath {
		t.Errorf("PATH MISMATCH after BuildSubmission normalization!\n  CLI zip key:    %q\n  Worker API path: %q\n  This means the worker will get 404 when downloading snapshots!", cliZipKey, workerAPIPath)
	}
}

// --- End-to-end: Upload then download via mock server ---

func TestSnapshotRoundTrip_UploadAndDownload(t *testing.T) {
	// Simulates the full flow: CLI uploads snapshots, worker downloads them
	storedSnapshots := make(map[string][]byte)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/runs/run-1/snapshots":
			// Handle upload (like API server handleUploadSnapshots)
			body, _ := io.ReadAll(r.Body)
			zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			count := 0
			for _, entry := range zr.File {
				if entry.FileInfo().IsDir() {
					continue
				}
				rc, _ := entry.Open()
				content, _ := io.ReadAll(rc)
				rc.Close()
				storedSnapshots[entry.Name] = content
				count++
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]int{"count": count})

		case r.Method == "GET":
			// Handle download (like API server handleDownloadSnapshot)
			// Path: /api/runs/run-1/snapshots/tests/__snapshots__/response.json
			prefix := "/api/runs/run-1/snapshots/"
			if len(r.URL.Path) <= len(prefix) {
				// List snapshots
				var result []map[string]interface{}
				for k, v := range storedSnapshots {
					result = append(result, map[string]interface{}{
						"file_path": k,
						"size":      len(v),
					})
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(result)
				return
			}
			filePath := r.URL.Path[len(prefix):]
			content, ok := storedSnapshots[filePath]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("snapshot not found: " + filePath))
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write(content)
		}
	}))
	defer server.Close()

	// --- CLI side: collect and upload ---
	dir := t.TempDir()
	snapshotDir := filepath.Join(dir, "tests", "__snapshots__")
	os.MkdirAll(snapshotDir, 0755)
	os.WriteFile(filepath.Join(snapshotDir, "response.json"), []byte(`{"id":1}`), 0644)
	os.WriteFile(filepath.Join(snapshotDir, "body.txt"), []byte("hello"), 0644)

	suiteFilePath := filepath.Join(dir, "tests", "suite.chiperka")

	suites := []SuiteSubmission{
		{
			Name:     "suite1",
			FilePath: suiteFilePath,
			Tests: []model.Test{
				{
					Name: "test1",
					Assertions: []model.Assertion{
						{Response: &model.ResponseAssertion{
							Body: &model.ResponseBodyAssertion{Snapshot: "__snapshots__/response.json"},
						}},
						{CLI: &model.CLIAssertion{
							Stdout: &model.CLIOutputAssertion{Snapshot: "__snapshots__/body.txt"},
						}},
					},
				},
			},
		},
	}

	snapshots, err := CollectSnapshotFiles(suites)
	if err != nil {
		t.Fatalf("CollectSnapshotFiles error: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}

	client := NewClient(server.URL, "")
	if err := client.UploadSnapshots("run-1", snapshots); err != nil {
		t.Fatalf("UploadSnapshots error: %v", err)
	}

	// Verify snapshots are stored
	if len(storedSnapshots) != 2 {
		t.Fatalf("expected 2 stored snapshots, got %d", len(storedSnapshots))
	}

	// --- Worker side: download ---
	// The worker constructs paths like: filepath.ToSlash(filepath.Join(filepath.Dir(suiteFilePath), snapshotPath))
	snapshotPaths := []string{"__snapshots__/response.json", "__snapshots__/body.txt"}

	for _, snapshotPath := range snapshotPaths {
		apiPath := filepath.ToSlash(filepath.Join(filepath.Dir(suiteFilePath), snapshotPath))
		url := server.URL + "/api/runs/run-1/snapshots/" + apiPath

		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("download error for %s: %v", snapshotPath, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("expected 200 for %s, got %d: %s\n  requested URL: %s\n  stored keys: %v",
				snapshotPath, resp.StatusCode, string(body), url, storedSnapshotKeys(storedSnapshots))
		}
	}
}

func storedSnapshotKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// --- BuildSubmission path normalization ---

func TestBuildSubmission_NormalizesFilePath(t *testing.T) {
	tests := model.NewTestCollection()
	tests.AddSuite(model.Suite{
		Name:     "suite1",
		FilePath: "tests/suite.chiperka", // already relative
		Tests: []model.Test{
			{Name: "t1"},
		},
	})

	services := model.NewServiceTemplateCollection()
	req, err := BuildSubmission(tests, services, "1.0.0", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Suites[0].FilePath != "tests/suite.chiperka" {
		t.Errorf("expected relative path preserved, got %q", req.Suites[0].FilePath)
	}
}

func TestBuildSubmission_NormalizesAbsolutePath(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	absPath := filepath.Join(cwd, "tests", "suite.chiperka")

	tests := model.NewTestCollection()
	tests.AddSuite(model.Suite{
		Name:     "suite1",
		FilePath: absPath,
		Tests:    []model.Test{{Name: "t1"}},
	})

	services := model.NewServiceTemplateCollection()
	req, err := BuildSubmission(tests, services, "1.0.0", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := filepath.Join("tests", "suite.chiperka")
	if req.Suites[0].FilePath != expected {
		t.Errorf("expected normalized path %q, got %q", expected, req.Suites[0].FilePath)
	}
}

// --- Multi-suite tests ---

func TestCollectSnapshotFiles_MultipleSuitesDifferentDirs(t *testing.T) {
	dir := t.TempDir()

	// Create two suites in different directories, each with its own snapshot
	os.MkdirAll(filepath.Join(dir, "tests/auth/__snapshots__"), 0755)
	os.MkdirAll(filepath.Join(dir, "tests/api/__snapshots__"), 0755)
	os.WriteFile(filepath.Join(dir, "tests/auth/__snapshots__/login.json"), []byte(`{"token":"abc"}`), 0644)
	os.WriteFile(filepath.Join(dir, "tests/api/__snapshots__/users.json"), []byte(`[{"id":1}]`), 0644)

	suites := []SuiteSubmission{
		{
			Name:     "auth",
			FilePath: filepath.Join(dir, "tests/auth/suite.chiperka"),
			Tests: []model.Test{
				{
					Name: "login",
					Assertions: []model.Assertion{
						{Response: &model.ResponseAssertion{
							Body: &model.ResponseBodyAssertion{Snapshot: "__snapshots__/login.json"},
						}},
					},
				},
			},
		},
		{
			Name:     "api",
			FilePath: filepath.Join(dir, "tests/api/suite.chiperka"),
			Tests: []model.Test{
				{
					Name: "list-users",
					Assertions: []model.Assertion{
						{Response: &model.ResponseAssertion{
							Body: &model.ResponseBodyAssertion{Snapshot: "__snapshots__/users.json"},
						}},
					},
				},
			},
		},
	}

	snapshots, err := CollectSnapshotFiles(suites)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}

	// Each suite should produce a different key
	keys := make([]string, 0, len(snapshots))
	for k := range snapshots {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if !strings.Contains(keys[0], "api/__snapshots__/users.json") {
		t.Errorf("expected api snapshot key, got %q", keys[0])
	}
	if !strings.Contains(keys[1], "auth/__snapshots__/login.json") {
		t.Errorf("expected auth snapshot key, got %q", keys[1])
	}
}

func TestCollectSnapshotFiles_CrossSuiteDedup(t *testing.T) {
	// Two suites in the same directory referencing the same snapshot file
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "tests/__snapshots__"), 0755)
	os.WriteFile(filepath.Join(dir, "tests/__snapshots__/shared.json"), []byte("shared"), 0644)

	suites := []SuiteSubmission{
		{
			Name:     "suite-a",
			FilePath: filepath.Join(dir, "tests/a.chiperka"),
			Tests: []model.Test{{
				Name: "test-a",
				Assertions: []model.Assertion{{Response: &model.ResponseAssertion{
					Body: &model.ResponseBodyAssertion{Snapshot: "__snapshots__/shared.json"},
				}}},
			}},
		},
		{
			Name:     "suite-b",
			FilePath: filepath.Join(dir, "tests/b.chiperka"),
			Tests: []model.Test{{
				Name: "test-b",
				Assertions: []model.Assertion{{Response: &model.ResponseAssertion{
					Body: &model.ResponseBodyAssertion{Snapshot: "__snapshots__/shared.json"},
				}}},
			}},
		},
	}

	snapshots, err := CollectSnapshotFiles(suites)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snapshots) != 1 {
		t.Errorf("expected 1 snapshot (cross-suite dedup), got %d", len(snapshots))
	}
}

func TestCollectSnapshotFiles_EmptySnapshotStringIgnored(t *testing.T) {
	// Empty snapshot strings should not cause errors
	suites := []SuiteSubmission{
		{
			Name:     "suite1",
			FilePath: "tests/suite.chiperka",
			Tests: []model.Test{{
				Name: "test1",
				Assertions: []model.Assertion{
					{Response: &model.ResponseAssertion{
						Body: &model.ResponseBodyAssertion{Snapshot: ""},
					}},
					{CLI: &model.CLIAssertion{
						Stdout: &model.CLIOutputAssertion{Snapshot: ""},
						Stderr: &model.CLIOutputAssertion{Snapshot: ""},
					}},
					{Artifact: &model.ArtifactAssertion{
						Name:     "output",
						Snapshot: "",
					}},
				},
			}},
		},
	}

	snapshots, err := CollectSnapshotFiles(suites)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snapshots) != 0 {
		t.Errorf("expected 0 snapshots for empty strings, got %d", len(snapshots))
	}
}

func TestCollectSnapshotFiles_NestedSubdirectory(t *testing.T) {
	// Snapshot path that traverses deeper subdirectory
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "tests/api/v2/__snapshots__"), 0755)
	os.WriteFile(filepath.Join(dir, "tests/api/v2/__snapshots__/resp.json"), []byte("v2"), 0644)

	suites := []SuiteSubmission{
		{
			Name:     "suite1",
			FilePath: filepath.Join(dir, "tests/api/suite.chiperka"),
			Tests: []model.Test{{
				Name: "test1",
				Assertions: []model.Assertion{{Response: &model.ResponseAssertion{
					Body: &model.ResponseBodyAssertion{Snapshot: "v2/__snapshots__/resp.json"},
				}}},
			}},
		},
	}

	snapshots, err := CollectSnapshotFiles(suites)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}

	for k, v := range snapshots {
		if !strings.Contains(k, "v2/__snapshots__/resp.json") {
			t.Errorf("expected nested path in key, got %q", k)
		}
		if string(v) != "v2" {
			t.Errorf("unexpected content: %q", string(v))
		}
	}
}

func TestUploadSnapshots_LargePayload(t *testing.T) {
	// Verify large snapshot content survives upload
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	// Create a 1MB snapshot
	largeContent := make([]byte, 1024*1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	client := NewClient(server.URL, "")
	err := client.UploadSnapshots("run-1", map[string][]byte{
		"large.bin": largeContent,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the zip contains the correct data
	zr, err := zip.NewReader(bytes.NewReader(receivedBody), int64(len(receivedBody)))
	if err != nil {
		t.Fatalf("invalid zip: %v", err)
	}
	if len(zr.File) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(zr.File))
	}
	rc, _ := zr.File[0].Open()
	content, _ := io.ReadAll(rc)
	rc.Close()
	if len(content) != len(largeContent) {
		t.Errorf("content size mismatch: expected %d, got %d", len(largeContent), len(content))
	}
}

func TestBuildSubmission_PreservesVersion(t *testing.T) {
	tests := model.NewTestCollection()
	tests.AddSuite(model.Suite{
		Name:     "suite1",
		FilePath: "tests/suite.chiperka",
		Tests:    []model.Test{{Name: "t1"}},
	})

	services := model.NewServiceTemplateCollection()
	req, err := BuildSubmission(tests, services, "2.5.0", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Config == nil || req.Config.Version != "2.5.0" {
		t.Errorf("expected version '2.5.0', got %v", req.Config)
	}
}

func TestBuildSubmission_MultipleSuites(t *testing.T) {
	tests := model.NewTestCollection()
	tests.AddSuite(model.Suite{
		Name:     "auth",
		FilePath: "tests/auth/suite.chiperka",
		Tests:    []model.Test{{Name: "login"}, {Name: "logout"}},
	})
	tests.AddSuite(model.Suite{
		Name:     "api",
		FilePath: "tests/api/suite.chiperka",
		Tests:    []model.Test{{Name: "users"}},
	})

	services := model.NewServiceTemplateCollection()
	req, err := BuildSubmission(tests, services, "1.0.0", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(req.Suites) != 2 {
		t.Fatalf("expected 2 suites, got %d", len(req.Suites))
	}
	if len(req.Suites[0].Tests) != 2 {
		t.Errorf("expected 2 tests in first suite, got %d", len(req.Suites[0].Tests))
	}
	if len(req.Suites[1].Tests) != 1 {
		t.Errorf("expected 1 test in second suite, got %d", len(req.Suites[1].Tests))
	}
}

// --- Critical integration test: exact run.go flow ---

func TestRunGoFlow_BuildSubmissionThenCollectSnapshots(t *testing.T) {
	// This test replicates the EXACT sequence from run.go:
	//   1. Parser produces absolute FilePath
	//   2. BuildSubmission normalizes to relative
	//   3. CollectSnapshotFiles uses the normalized suites
	//   4. CLI uploads with zip key
	//   5. Worker constructs API path from claimed.SuiteFilePath
	// The zip key (step 4) must match the worker API path (step 5).

	// Create test files inside a subdirectory of CWD (realistic scenario)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// Create temp dirs inside CWD to get proper relative path normalization
	testDir := filepath.Join(cwd, "_test_run_flow")
	snapshotDir := filepath.Join(testDir, "tests", "api", "__snapshots__")
	os.MkdirAll(snapshotDir, 0755)
	defer os.RemoveAll(testDir)

	os.WriteFile(filepath.Join(snapshotDir, "response.json"), []byte(`{"status":"ok"}`), 0644)

	// Step 1: Parser produces absolute path
	absFilePath := filepath.Join(testDir, "tests", "api", "suite.chiperka")

	tests := model.NewTestCollection()
	tests.AddSuite(model.Suite{
		Name:     "api-tests",
		FilePath: absFilePath, // absolute, as parser produces
		Tests: []model.Test{
			{
				Name: "get-users",
				Assertions: []model.Assertion{
					{Response: &model.ResponseAssertion{
						Body: &model.ResponseBodyAssertion{Snapshot: "__snapshots__/response.json"},
					}},
				},
			},
		},
	})

	services := model.NewServiceTemplateCollection()

	// Step 2: BuildSubmission normalizes to relative
	submission, err := BuildSubmission(tests, services, "1.0.0", nil)
	if err != nil {
		t.Fatalf("BuildSubmission error: %v", err)
	}

	normalizedPath := submission.Suites[0].FilePath
	t.Logf("Absolute path:    %q", absFilePath)
	t.Logf("Normalized path:  %q", normalizedPath)

	// The normalized path should be relative
	if filepath.IsAbs(normalizedPath) {
		t.Errorf("expected relative path after normalization, got absolute: %q", normalizedPath)
	}

	// Step 3: CollectSnapshotFiles uses normalized suites (same as run.go line 507)
	snapshots, err := CollectSnapshotFiles(submission.Suites)
	if err != nil {
		t.Fatalf("CollectSnapshotFiles error: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}

	// Step 4: Get the zip key that would be uploaded
	var cliZipKey string
	for k := range snapshots {
		cliZipKey = k
	}
	t.Logf("CLI zip key:      %q", cliZipKey)

	// Step 5: Worker receives suiteFilePath from API (= submission.Suites[0].FilePath)
	workerSuiteFilePath := normalizedPath
	snapshotRelPath := "__snapshots__/response.json"
	workerAPIPath := filepath.ToSlash(filepath.Join(filepath.Dir(workerSuiteFilePath), snapshotRelPath))
	t.Logf("Worker API path:  %q", workerAPIPath)

	// THE CRITICAL CHECK: these must match
	if cliZipKey != workerAPIPath {
		t.Fatalf("PATH MISMATCH!\n"+
			"  CLI zip key:      %q\n"+
			"  Worker API path:  %q\n"+
			"  This means snapshots uploaded by CLI cannot be found by workers!\n"+
			"  Normalized suite path: %q",
			cliZipKey, workerAPIPath, normalizedPath)
	}
}

func intPtr(v int) *int { return &v }
