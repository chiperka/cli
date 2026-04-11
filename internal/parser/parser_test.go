package parser

import (
	"os"
	"path/filepath"
	"strings"
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
	os.Setenv("CHIPERKA_TEST_TARGET", "http://localhost:9090")
	os.Setenv("CHIPERKA_TEST_PATH", "users")
	t.Cleanup(func() {
		os.Unsetenv("CHIPERKA_TEST_TARGET")
		os.Unsetenv("CHIPERKA_TEST_PATH")
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
	os.Unsetenv("CHIPERKA_TEST_TARGET")
	os.Unsetenv("CHIPERKA_TEST_PATH")

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

func TestParser_EnvVarExpansion_NonChiperkaPrefix(t *testing.T) {
	// $HOME should NOT be expanded (only $CHIPERKA_ prefix)
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

// --- Kind dispatch ---

func TestParser_ParseAll_ServiceKind(t *testing.T) {
	p := New()
	result := p.ParseAll([]string{testdataPath(t, "service-valid.chiperka")})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if result.Tests.TotalTests() != 0 {
		t.Errorf("expected 0 tests, got %d", result.Tests.TotalTests())
	}
	tmpl := result.Services.GetTemplate("api")
	if tmpl == nil {
		t.Fatalf("expected service template 'api'")
	}
	if tmpl.Image != "ghcr.io/myorg/api:latest" {
		t.Errorf("expected image, got %q", tmpl.Image)
	}
	if tmpl.Environment["DB_URL"] != "postgres://db:5432/test" {
		t.Errorf("expected env DB_URL, got %q", tmpl.Environment["DB_URL"])
	}
	if tmpl.HealthCheck == nil || tmpl.HealthCheck.Retries != 30 {
		t.Errorf("expected healthcheck retries=30, got %+v", tmpl.HealthCheck)
	}
	if tmpl.FilePath == "" {
		t.Errorf("expected FilePath to be set")
	}
}

func TestParser_ParseAll_ServiceMissingName(t *testing.T) {
	p := New()
	result := p.ParseAll([]string{testdataPath(t, "service-missing-name.chiperka")})
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %v", result.Errors)
	}
	if !strings.Contains(result.Errors[0].Error(), "name") {
		t.Errorf("expected error to mention name, got: %v", result.Errors[0])
	}
	if result.Services.HasTemplates() {
		t.Errorf("expected no templates added when name is missing")
	}
}

func TestParser_ParseAll_UnknownKind(t *testing.T) {
	p := New()
	result := p.ParseAll([]string{testdataPath(t, "unknown-kind.chiperka")})
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %v", result.Errors)
	}
	if !strings.Contains(result.Errors[0].Error(), "fixture") {
		t.Errorf("expected error to name the unknown kind, got: %v", result.Errors[0])
	}
}

func TestParser_ParseAll_KindTestExplicit(t *testing.T) {
	p := New()
	result := p.ParseAll([]string{testdataPath(t, "test-explicit-kind.chiperka")})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if result.Tests.TotalTests() != 1 {
		t.Errorf("expected 1 test, got %d", result.Tests.TotalTests())
	}
	if result.Services.HasTemplates() {
		t.Errorf("expected no service templates")
	}
}

func TestParser_ParseAll_KindDefaultsToTest(t *testing.T) {
	// Existing valid-simple.spark has no `kind:` field — must still parse as test.
	p := New()
	result := p.ParseAll([]string{testdataPath(t, "valid-simple.spark")})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if result.Tests.TotalTests() != 1 {
		t.Errorf("expected 1 test (default kind), got %d", result.Tests.TotalTests())
	}
}

func TestParser_ParseAll_DuplicateServiceName(t *testing.T) {
	p := New()
	result := p.ParseAll([]string{
		testdataPath(t, "service-valid.chiperka"),
		testdataPath(t, "service-duplicate.chiperka"),
	})
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 duplicate error, got %v", result.Errors)
	}
	if !strings.Contains(result.Errors[0].Error(), "duplicate") {
		t.Errorf("expected duplicate error, got: %v", result.Errors[0])
	}
	// First one wins
	tmpl := result.Services.GetTemplate("api")
	if tmpl == nil || tmpl.Image != "ghcr.io/myorg/api:latest" {
		t.Errorf("expected first service to win, got %+v", tmpl)
	}
}

func TestParser_ParseAll_MixedKinds(t *testing.T) {
	p := New()
	result := p.ParseAll([]string{
		testdataPath(t, "service-valid.chiperka"),
		testdataPath(t, "service-postgres.chiperka"),
		testdataPath(t, "test-explicit-kind.chiperka"),
	})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if result.Tests.TotalTests() != 1 {
		t.Errorf("expected 1 test, got %d", result.Tests.TotalTests())
	}
	if !result.Services.HasTemplates() || len(result.Services.Templates) != 2 {
		t.Errorf("expected 2 service templates, got %d", len(result.Services.Templates))
	}
}

// --- ParseFile rejects non-test kinds ---

func TestParser_ParseFile_RejectsServiceKind(t *testing.T) {
	p := New()
	_, err := p.ParseFile(testdataPath(t, "service-valid.chiperka"))
	if err == nil {
		t.Fatalf("expected error: ParseFile should reject kind: service")
	}
}

func TestParser_ParseBytes_RejectsServiceKind(t *testing.T) {
	p := New()
	_, err := p.ParseBytes([]byte("kind: service\nname: api\nimage: nginx"))
	if err == nil {
		t.Fatalf("expected error: ParseBytes should reject kind: service")
	}
}
