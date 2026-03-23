package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func testdataPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", name)
}

// --- ParseFile ---

func TestParser_ParseFile_Simple(t *testing.T) {
	p := New()
	suite, err := p.ParseFile(testdataPath(t, "valid-simple.spark"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suite.Name != "simple-suite" {
		t.Errorf("expected name 'simple-suite', got %q", suite.Name)
	}
	if len(suite.Tests) != 1 {
		t.Fatalf("expected 1 test, got %d", len(suite.Tests))
	}
	if suite.Tests[0].Name != "health-check" {
		t.Errorf("expected test name 'health-check', got %q", suite.Tests[0].Name)
	}
	if suite.Tests[0].Execution.Request.Method != "GET" {
		t.Errorf("expected method GET, got %q", suite.Tests[0].Execution.Request.Method)
	}
}

func TestParser_ParseFile_Full(t *testing.T) {
	p := New()
	suite, err := p.ParseFile(testdataPath(t, "valid-full.spark"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suite.Name != "full-suite" {
		t.Errorf("expected name 'full-suite', got %q", suite.Name)
	}
	test := suite.Tests[0]
	if len(test.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(test.Tags))
	}
	if len(test.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(test.Services))
	}
	if test.Services[0].Image != "myapp:latest" {
		t.Errorf("expected image myapp:latest, got %q", test.Services[0].Image)
	}
	if len(test.Setup) != 1 {
		t.Errorf("expected 1 setup step, got %d", len(test.Setup))
	}
	if len(test.Assertions) != 1 {
		t.Errorf("expected 1 assertion, got %d", len(test.Assertions))
	}
	if len(test.Teardown) != 1 {
		t.Errorf("expected 1 teardown step, got %d", len(test.Teardown))
	}
}

func TestParser_ParseFile_SetsFilePath(t *testing.T) {
	p := New()
	path := testdataPath(t, "valid-simple.spark")
	suite, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suite.FilePath != path {
		t.Errorf("expected FilePath=%q, got %q", path, suite.FilePath)
	}
}

func TestParser_ParseFile_Invalid(t *testing.T) {
	p := New()
	_, err := p.ParseFile(testdataPath(t, "invalid.spark"))
	if err == nil {
		t.Errorf("expected error for invalid YAML")
	}
}

func TestParser_ParseFile_NonExistent(t *testing.T) {
	p := New()
	_, err := p.ParseFile("testdata/nonexistent.spark")
	if err == nil {
		t.Errorf("expected error for non-existent file")
	}
}

// --- ParseBytes ---

func TestParser_ParseBytes_Valid(t *testing.T) {
	p := New()
	yaml := []byte("name: inline\ntests:\n  - name: t1\n    execution:\n      request:\n        method: GET\n        url: /test\n    assertions: []")
	suite, err := p.ParseBytes(yaml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suite.Name != "inline" {
		t.Errorf("expected name 'inline', got %q", suite.Name)
	}
	if suite.FilePath != "<api>" {
		t.Errorf("expected FilePath='<api>', got %q", suite.FilePath)
	}
}

func TestParser_ParseBytes_Invalid(t *testing.T) {
	p := New()
	_, err := p.ParseBytes([]byte("invalid: [yaml"))
	if err == nil {
		t.Errorf("expected error for invalid YAML bytes")
	}
}

// --- ParseAll ---

func TestParser_ParseAll_MultipleFiles(t *testing.T) {
	p := New()
	files := []string{
		testdataPath(t, "valid-simple.spark"),
		testdataPath(t, "valid-multi.spark"),
	}
	result := p.ParseAll(files)
	if len(result.Errors) != 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}
	if len(result.Tests.Suites) != 2 {
		t.Fatalf("expected 2 suites, got %d", len(result.Tests.Suites))
	}
	if result.Tests.TotalTests() != 4 {
		t.Errorf("expected 4 total tests, got %d", result.Tests.TotalTests())
	}
}

func TestParser_ParseAll_WithInvalidFile(t *testing.T) {
	p := New()
	files := []string{
		testdataPath(t, "valid-simple.spark"),
		testdataPath(t, "invalid.spark"),
	}
	result := p.ParseAll(files)
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}
	if len(result.Tests.Suites) != 1 {
		t.Errorf("expected 1 valid suite, got %d", len(result.Tests.Suites))
	}
}

func TestParser_ParseAll_Empty(t *testing.T) {
	p := New()
	result := p.ParseAll(nil)
	if len(result.Errors) != 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}
	if result.Tests.TotalTests() != 0 {
		t.Errorf("expected 0 tests, got %d", result.Tests.TotalTests())
	}
}

func TestParser_ParseAll_NonExistentFile(t *testing.T) {
	p := New()
	result := p.ParseAll([]string{"testdata/nonexistent.spark"})
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}
}

// --- Environment variable expansion ---

func TestParser_EnvVarExpansion(t *testing.T) {
	os.Setenv("SPARK_TEST_TARGET", "http://localhost:9090")
	os.Setenv("SPARK_TEST_PATH", "users")
	t.Cleanup(func() {
		os.Unsetenv("SPARK_TEST_TARGET")
		os.Unsetenv("SPARK_TEST_PATH")
	})

	p := New()
	suite, err := p.ParseFile(testdataPath(t, "env-vars.spark"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	test := suite.Tests[0]
	if test.Execution.Target != "http://localhost:9090" {
		t.Errorf("expected target expanded, got %q", test.Execution.Target)
	}
	if test.Execution.Request.URL != "/api/users" {
		t.Errorf("expected URL expanded, got %q", test.Execution.Request.URL)
	}
}

func TestParser_EnvVarExpansion_Unset(t *testing.T) {
	os.Unsetenv("SPARK_TEST_TARGET")
	os.Unsetenv("SPARK_TEST_PATH")

	p := New()
	suite, err := p.ParseFile(testdataPath(t, "env-vars.spark"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	test := suite.Tests[0]
	// Unset vars should expand to empty string
	if test.Execution.Target != "" {
		t.Errorf("expected empty target for unset var, got %q", test.Execution.Target)
	}
}

func TestParser_EnvVarExpansion_NonSparkPrefix(t *testing.T) {
	// $HOME should NOT be expanded (only $SPARK_ prefix)
	p := New()
	yaml := []byte("name: test\ntests:\n  - name: t\n    execution:\n      target: $HOME/api\n      request:\n        method: GET\n        url: /test\n    assertions: []")
	suite, err := p.ParseBytes(yaml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suite.Tests[0].Execution.Target != "$HOME/api" {
		t.Errorf("expected $HOME not expanded, got %q", suite.Tests[0].Execution.Target)
	}
}
