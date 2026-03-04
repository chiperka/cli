package model

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

// --- ShellCommand YAML unmarshaling ---

func TestModel_ShellCommand_FromString(t *testing.T) {
	input := `command: "echo hello world"`
	var s struct {
		Command ShellCommand `yaml:"command"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(s.Command) != 3 {
		t.Fatalf("expected 3 tokens, got %d", len(s.Command))
	}
	if s.Command[0] != "echo" || s.Command[1] != "hello" || s.Command[2] != "world" {
		t.Errorf("expected [echo hello world], got %v", s.Command)
	}
}

func TestModel_ShellCommand_FromList(t *testing.T) {
	input := "command:\n  - echo\n  - hello world"
	var s struct {
		Command ShellCommand `yaml:"command"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(s.Command) != 2 {
		t.Fatalf("expected 2 items, got %d", len(s.Command))
	}
	if s.Command[0] != "echo" || s.Command[1] != "hello world" {
		t.Errorf("expected [echo, hello world], got %v", s.Command)
	}
}

func TestModel_ShellCommand_QuotedString(t *testing.T) {
	input := `command: "echo 'hello world' done"`
	var s struct {
		Command ShellCommand `yaml:"command"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(s.Command) != 3 {
		t.Fatalf("expected 3 tokens, got %d: %v", len(s.Command), s.Command)
	}
	if s.Command[1] != "hello world" {
		t.Errorf("expected 'hello world' (without quotes), got %q", s.Command[1])
	}
}

func TestModel_ShellCommand_DoubleQuotedString(t *testing.T) {
	input := "command: 'echo \"hello world\" done'"
	var s struct {
		Command ShellCommand `yaml:"command"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(s.Command) != 3 {
		t.Fatalf("expected 3 tokens, got %d: %v", len(s.Command), s.Command)
	}
	if s.Command[1] != "hello world" {
		t.Errorf("expected 'hello world' (without quotes), got %q", s.Command[1])
	}
}

func TestModel_ShellCommand_EmptyString(t *testing.T) {
	input := `command: ""`
	var s struct {
		Command ShellCommand `yaml:"command"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(s.Command) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(s.Command))
	}
}

// --- Body YAML unmarshaling ---

func TestModel_Body_RawString(t *testing.T) {
	input := `body: '{"key":"value"}'`
	var s struct {
		Body Body `yaml:"body"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if s.Body.Raw != `{"key":"value"}` {
		t.Errorf("expected raw body, got %q", s.Body.Raw)
	}
	if s.Body.IsFile() || s.Body.IsMultipart() || s.Body.IsZero() {
		t.Errorf("expected raw body flags: IsFile=false, IsMultipart=false, IsZero=false")
	}
}

func TestModel_Body_FileReference(t *testing.T) {
	input := "body:\n  file: ./data/payload.json"
	var s struct {
		Body Body `yaml:"body"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if s.Body.File != "./data/payload.json" {
		t.Errorf("expected file reference, got %q", s.Body.File)
	}
	if !s.Body.IsFile() {
		t.Errorf("expected IsFile=true")
	}
}

func TestModel_Body_Multipart(t *testing.T) {
	input := "body:\n  multipart:\n    field1: value1\n    file_field:\n      file: ./photo.jpg"
	var s struct {
		Body Body `yaml:"body"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if !s.Body.IsMultipart() {
		t.Fatalf("expected IsMultipart=true")
	}
	if len(s.Body.Multipart) != 2 {
		t.Fatalf("expected 2 multipart fields, got %d", len(s.Body.Multipart))
	}
	if s.Body.Multipart["field1"].Value != "value1" {
		t.Errorf("expected field1=value1, got %q", s.Body.Multipart["field1"].Value)
	}
	if s.Body.Multipart["file_field"].File != "./photo.jpg" {
		t.Errorf("expected file_field file=./photo.jpg, got %q", s.Body.Multipart["file_field"].File)
	}
}

func TestModel_Body_FileAndMultipartError(t *testing.T) {
	input := "body:\n  file: ./data.json\n  multipart:\n    f: v"
	var s struct {
		Body Body `yaml:"body"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err == nil {
		t.Errorf("expected error when both file and multipart are specified")
	}
}

func TestModel_Body_InvalidMappingKey(t *testing.T) {
	input := "body:\n  unknown: value"
	var s struct {
		Body Body `yaml:"body"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err == nil {
		t.Errorf("expected error for mapping without file or multipart key")
	}
}

func TestModel_Body_IsZero(t *testing.T) {
	b := Body{}
	if !b.IsZero() {
		t.Errorf("expected empty body to be zero")
	}
}

func TestModel_Body_DisplayString_Raw(t *testing.T) {
	b := Body{Raw: "hello"}
	if b.DisplayString() != "hello" {
		t.Errorf("expected 'hello', got %q", b.DisplayString())
	}
}

func TestModel_Body_DisplayString_File(t *testing.T) {
	b := Body{File: "./data.json"}
	expected := "[file: ./data.json]"
	if b.DisplayString() != expected {
		t.Errorf("expected %q, got %q", expected, b.DisplayString())
	}
}

func TestModel_Body_DisplayString_Multipart(t *testing.T) {
	b := Body{Multipart: map[string]MultipartField{
		"b": {Value: "2"},
		"a": {Value: "1"},
	}}
	expected := "[multipart: a, b]"
	if b.DisplayString() != expected {
		t.Errorf("expected %q, got %q", expected, b.DisplayString())
	}
}

func TestModel_Body_DisplayString_Empty(t *testing.T) {
	b := Body{}
	if b.DisplayString() != "" {
		t.Errorf("expected empty string, got %q", b.DisplayString())
	}
}

// --- Body JSON round-trip ---

func TestModel_Body_JSONRoundTrip_Raw(t *testing.T) {
	original := Body{Raw: "hello"}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var decoded Body
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if decoded.Raw != original.Raw {
		t.Errorf("expected %q, got %q", original.Raw, decoded.Raw)
	}
}

func TestModel_Body_JSONRoundTrip_File(t *testing.T) {
	original := Body{File: "./data.json"}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var decoded Body
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if decoded.File != original.File {
		t.Errorf("expected %q, got %q", original.File, decoded.File)
	}
}

func TestModel_Body_JSONRoundTrip_Multipart(t *testing.T) {
	original := Body{Multipart: map[string]MultipartField{
		"text":  {Value: "hello"},
		"photo": {File: "./photo.jpg"},
	}}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var decoded Body
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if decoded.Multipart["text"].Value != "hello" {
		t.Errorf("expected text=hello, got %q", decoded.Multipart["text"].Value)
	}
	if decoded.Multipart["photo"].File != "./photo.jpg" {
		t.Errorf("expected photo file=./photo.jpg, got %q", decoded.Multipart["photo"].File)
	}
}

func TestModel_Body_JSONRoundTrip_Empty(t *testing.T) {
	original := Body{}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var decoded Body
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if !decoded.IsZero() {
		t.Errorf("expected zero body after round-trip")
	}
}

func TestModel_Body_JSON_InvalidInput(t *testing.T) {
	var b Body
	if err := json.Unmarshal([]byte(`123`), &b); err == nil {
		t.Errorf("expected error for invalid JSON body")
	}
}

func TestModel_Body_JSON_InvalidObjectKey(t *testing.T) {
	var b Body
	if err := json.Unmarshal([]byte(`{"unknown":"value"}`), &b); err == nil {
		t.Errorf("expected error for object without file or multipart key")
	}
}

// --- HealthCheckTest YAML unmarshaling ---

func TestModel_HealthCheckTest_String(t *testing.T) {
	input := `test: "curl -f http://localhost/"`
	var hc HealthCheck
	if err := yaml.Unmarshal([]byte(input), &hc); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if string(hc.Test) != "curl -f http://localhost/" {
		t.Errorf("expected 'curl -f http://localhost/', got %q", hc.Test)
	}
}

func TestModel_HealthCheckTest_CMDShell(t *testing.T) {
	input := "test:\n  - CMD-SHELL\n  - curl -f http://localhost/"
	var hc HealthCheck
	if err := yaml.Unmarshal([]byte(input), &hc); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if string(hc.Test) != "curl -f http://localhost/" {
		t.Errorf("expected 'curl -f http://localhost/', got %q", hc.Test)
	}
}

func TestModel_HealthCheckTest_CMD(t *testing.T) {
	input := "test:\n  - CMD\n  - curl\n  - -f\n  - http://localhost/"
	var hc HealthCheck
	if err := yaml.Unmarshal([]byte(input), &hc); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if string(hc.Test) != "curl -f http://localhost/" {
		t.Errorf("expected 'curl -f http://localhost/', got %q", hc.Test)
	}
}

func TestModel_HealthCheckTest_NONE(t *testing.T) {
	input := "test:\n  - NONE"
	var hc HealthCheck
	if err := yaml.Unmarshal([]byte(input), &hc); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if string(hc.Test) != "" {
		t.Errorf("expected empty string for NONE, got %q", hc.Test)
	}
}

func TestModel_HealthCheckTest_EmptyList(t *testing.T) {
	input := "test: []"
	var hc HealthCheck
	if err := yaml.Unmarshal([]byte(input), &hc); err == nil {
		t.Errorf("expected error for empty list")
	}
}

func TestModel_HealthCheckTest_CMDShellWrongArgs(t *testing.T) {
	input := "test:\n  - CMD-SHELL\n  - arg1\n  - arg2"
	var hc HealthCheck
	if err := yaml.Unmarshal([]byte(input), &hc); err == nil {
		t.Errorf("expected error for CMD-SHELL with multiple args")
	}
}

// --- HealthCheck YAML unmarshaling ---

func TestModel_HealthCheck_BoolTrue(t *testing.T) {
	input := "healthcheck: true"
	var s struct {
		HealthCheck *HealthCheck `yaml:"healthcheck,omitempty"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if s.HealthCheck == nil {
		t.Fatalf("expected non-nil HealthCheck")
	}
	if string(s.HealthCheck.Test) != "" {
		t.Errorf("expected empty test for bool true, got %q", s.HealthCheck.Test)
	}
}

func TestModel_HealthCheck_BoolFalse(t *testing.T) {
	input := "healthcheck: false"
	var s struct {
		HealthCheck *HealthCheck `yaml:"healthcheck,omitempty"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err == nil {
		t.Errorf("expected error for healthcheck: false")
	}
}

func TestModel_HealthCheck_Mapping(t *testing.T) {
	input := "healthcheck:\n  test: curl -f http://localhost/\n  interval: 2s\n  retries: 10"
	var s struct {
		HealthCheck *HealthCheck `yaml:"healthcheck,omitempty"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if string(s.HealthCheck.Test) != "curl -f http://localhost/" {
		t.Errorf("expected test command, got %q", s.HealthCheck.Test)
	}
	if s.HealthCheck.Interval != "2s" {
		t.Errorf("expected interval 2s, got %q", s.HealthCheck.Interval)
	}
	if s.HealthCheck.Retries != 10 {
		t.Errorf("expected retries 10, got %d", s.HealthCheck.Retries)
	}
}

// --- TestCollection filtering ---

func TestModel_TestCollection_TotalTests(t *testing.T) {
	c := NewTestCollection()
	c.AddSuite(Suite{Name: "s1", Tests: []Test{{Name: "t1"}, {Name: "t2"}}})
	c.AddSuite(Suite{Name: "s2", Tests: []Test{{Name: "t3"}}})
	if c.TotalTests() != 3 {
		t.Errorf("expected 3, got %d", c.TotalTests())
	}
}

func TestModel_TestCollection_FilterByTags_Matching(t *testing.T) {
	c := NewTestCollection()
	c.AddSuite(Suite{Name: "s1", Tests: []Test{
		{Name: "t1", Tags: []string{"smoke", "api"}},
		{Name: "t2", Tags: []string{"slow"}},
		{Name: "t3", Tags: []string{"api"}},
	}})
	filtered := c.FilterByTags([]string{"api"})
	if filtered.TotalTests() != 2 {
		t.Errorf("expected 2, got %d", filtered.TotalTests())
	}
}

func TestModel_TestCollection_FilterByTags_NoMatch(t *testing.T) {
	c := NewTestCollection()
	c.AddSuite(Suite{Name: "s1", Tests: []Test{
		{Name: "t1", Tags: []string{"smoke"}},
	}})
	filtered := c.FilterByTags([]string{"api"})
	if filtered.TotalTests() != 0 {
		t.Errorf("expected 0, got %d", filtered.TotalTests())
	}
}

func TestModel_TestCollection_FilterByTags_Empty(t *testing.T) {
	c := NewTestCollection()
	c.AddSuite(Suite{Name: "s1", Tests: []Test{{Name: "t1"}}})
	filtered := c.FilterByTags([]string{})
	if filtered != c {
		t.Errorf("expected same collection when no tags specified")
	}
}

func TestModel_TestCollection_FilterByName_ExactSubstring(t *testing.T) {
	c := NewTestCollection()
	c.AddSuite(Suite{Name: "s1", Tests: []Test{
		{Name: "login-test"},
		{Name: "logout-test"},
		{Name: "signup-test"},
	}})
	filtered := c.FilterByName("login")
	if filtered.TotalTests() != 1 {
		t.Errorf("expected 1, got %d", filtered.TotalTests())
	}
}

func TestModel_TestCollection_FilterByName_Wildcard(t *testing.T) {
	c := NewTestCollection()
	c.AddSuite(Suite{Name: "s1", Tests: []Test{
		{Name: "auth/login"},
		{Name: "auth/logout"},
		{Name: "api/users"},
	}})
	filtered := c.FilterByName("auth/*")
	if filtered.TotalTests() != 2 {
		t.Errorf("expected 2, got %d", filtered.TotalTests())
	}
}

func TestModel_TestCollection_FilterByName_StarAll(t *testing.T) {
	c := NewTestCollection()
	c.AddSuite(Suite{Name: "s1", Tests: []Test{{Name: "t1"}, {Name: "t2"}}})
	filtered := c.FilterByName("*")
	if filtered.TotalTests() != 2 {
		t.Errorf("expected 2, got %d", filtered.TotalTests())
	}
}

func TestModel_TestCollection_FilterByName_Empty(t *testing.T) {
	c := NewTestCollection()
	c.AddSuite(Suite{Name: "s1", Tests: []Test{{Name: "t1"}}})
	filtered := c.FilterByName("")
	if filtered != c {
		t.Errorf("expected same collection when no pattern specified")
	}
}

func TestModel_TestCollection_FilterByName_CaseInsensitive(t *testing.T) {
	c := NewTestCollection()
	c.AddSuite(Suite{Name: "s1", Tests: []Test{
		{Name: "Login-Test"},
	}})
	filtered := c.FilterByName("login*")
	if filtered.TotalTests() != 1 {
		t.Errorf("expected 1 (case insensitive), got %d", filtered.TotalTests())
	}
}

func TestModel_TestCollection_FilterByName_SuffixWildcard(t *testing.T) {
	c := NewTestCollection()
	c.AddSuite(Suite{Name: "s1", Tests: []Test{
		{Name: "test-login"},
		{Name: "test-logout"},
		{Name: "other"},
	}})
	filtered := c.FilterByName("*login")
	if filtered.TotalTests() != 1 {
		t.Errorf("expected 1, got %d", filtered.TotalTests())
	}
}

func TestModel_TestCollection_FilterByName_MiddleWildcard(t *testing.T) {
	c := NewTestCollection()
	c.AddSuite(Suite{Name: "s1", Tests: []Test{
		{Name: "test-login-success"},
		{Name: "test-logout-success"},
	}})
	filtered := c.FilterByName("*login*")
	if filtered.TotalTests() != 1 {
		t.Errorf("expected 1, got %d", filtered.TotalTests())
	}
}

func TestModel_TestCollection_FilterByName_ComplexPattern(t *testing.T) {
	c := NewTestCollection()
	c.AddSuite(Suite{Name: "s1", Tests: []Test{
		{Name: "auth/login/success"},
		{Name: "auth/login/failure"},
		{Name: "auth/logout/success"},
	}})
	filtered := c.FilterByName("auth/*/success")
	if filtered.TotalTests() != 2 {
		t.Errorf("expected 2, got %d", filtered.TotalTests())
	}
}

// --- ServiceTemplateCollection ---

func TestModel_ServiceTemplateCollection_AddAndGet(t *testing.T) {
	c := NewServiceTemplateCollection()
	c.AddTemplate(&ServiceTemplate{Name: "db", Image: "postgres:15"})
	tmpl := c.GetTemplate("db")
	if tmpl == nil {
		t.Fatalf("expected template, got nil")
	}
	if tmpl.Image != "postgres:15" {
		t.Errorf("expected postgres:15, got %q", tmpl.Image)
	}
}

func TestModel_ServiceTemplateCollection_GetNotFound(t *testing.T) {
	c := NewServiceTemplateCollection()
	if c.GetTemplate("nonexistent") != nil {
		t.Errorf("expected nil for nonexistent template")
	}
}

func TestModel_ServiceTemplateCollection_HasTemplates(t *testing.T) {
	c := NewServiceTemplateCollection()
	if c.HasTemplates() {
		t.Errorf("expected false for empty collection")
	}
	c.AddTemplate(&ServiceTemplate{Name: "db"})
	if !c.HasTemplates() {
		t.Errorf("expected true after adding template")
	}
}

func TestModel_ServiceTemplateCollection_ResolveService_NoRef(t *testing.T) {
	c := NewServiceTemplateCollection()
	svc := Service{Name: "app", Image: "myapp:latest"}
	resolved, err := c.ResolveService(svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Image != "myapp:latest" {
		t.Errorf("expected myapp:latest, got %q", resolved.Image)
	}
}

func TestModel_ServiceTemplateCollection_ResolveService_WithRef(t *testing.T) {
	c := NewServiceTemplateCollection()
	c.AddTemplate(&ServiceTemplate{
		Name:  "db",
		Image: "postgres:15",
		Environment: map[string]string{
			"POSTGRES_DB": "test",
		},
		Artifacts: []ServiceArtifact{{Path: "/var/log/pg.log"}},
	})

	svc := Service{Ref: "db"}
	resolved, err := c.ResolveService(svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Image != "postgres:15" {
		t.Errorf("expected postgres:15, got %q", resolved.Image)
	}
	if resolved.Name != "db" {
		t.Errorf("expected name=db, got %q", resolved.Name)
	}
	if resolved.Environment["POSTGRES_DB"] != "test" {
		t.Errorf("expected POSTGRES_DB=test, got %q", resolved.Environment["POSTGRES_DB"])
	}
	if len(resolved.Artifacts) != 1 {
		t.Errorf("expected 1 artifact, got %d", len(resolved.Artifacts))
	}
}

func TestModel_ServiceTemplateCollection_ResolveService_WithOverrides(t *testing.T) {
	c := NewServiceTemplateCollection()
	c.AddTemplate(&ServiceTemplate{
		Name:  "db",
		Image: "postgres:15",
		Environment: map[string]string{
			"POSTGRES_DB":       "default",
			"POSTGRES_PASSWORD": "secret",
		},
	})

	svc := Service{
		Ref:   "db",
		Name:  "custom-db",
		Image: "postgres:16",
		Environment: map[string]string{
			"POSTGRES_DB": "overridden",
		},
	}
	resolved, err := c.ResolveService(svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Name != "custom-db" {
		t.Errorf("expected custom-db, got %q", resolved.Name)
	}
	if resolved.Image != "postgres:16" {
		t.Errorf("expected postgres:16, got %q", resolved.Image)
	}
	if resolved.Environment["POSTGRES_DB"] != "overridden" {
		t.Errorf("expected overridden, got %q", resolved.Environment["POSTGRES_DB"])
	}
	if resolved.Environment["POSTGRES_PASSWORD"] != "secret" {
		t.Errorf("expected secret preserved, got %q", resolved.Environment["POSTGRES_PASSWORD"])
	}
}

func TestModel_ServiceTemplateCollection_ResolveService_AppendArtifacts(t *testing.T) {
	c := NewServiceTemplateCollection()
	c.AddTemplate(&ServiceTemplate{
		Name:      "db",
		Image:     "postgres:15",
		Artifacts: []ServiceArtifact{{Path: "/var/log/template.log"}},
	})

	svc := Service{
		Ref:       "db",
		Artifacts: []ServiceArtifact{{Path: "/var/log/service.log"}},
	}
	resolved, err := c.ResolveService(svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Artifacts) != 2 {
		t.Fatalf("expected 2 artifacts (template + service), got %d", len(resolved.Artifacts))
	}
	if resolved.Artifacts[0].Path != "/var/log/template.log" {
		t.Errorf("expected template artifact first, got %q", resolved.Artifacts[0].Path)
	}
	if resolved.Artifacts[1].Path != "/var/log/service.log" {
		t.Errorf("expected service artifact second, got %q", resolved.Artifacts[1].Path)
	}
}

func TestModel_ServiceTemplateCollection_ResolveService_NotFound(t *testing.T) {
	c := NewServiceTemplateCollection()
	svc := Service{Ref: "nonexistent"}
	_, err := c.ResolveService(svc)
	if err == nil {
		t.Errorf("expected error for nonexistent template ref")
	}
}

// --- matchesPattern ---

func TestModel_MatchesPattern_NoWildcard(t *testing.T) {
	if !matchesPattern("hello-world", "hello") {
		t.Errorf("expected substring match")
	}
	if matchesPattern("world", "hello") {
		t.Errorf("expected no match")
	}
}

func TestModel_MatchesPattern_PrefixWildcard(t *testing.T) {
	if !matchesPattern("auth-test", "auth*") {
		t.Errorf("expected prefix match")
	}
	if matchesPattern("test-auth", "auth*") {
		t.Errorf("expected no match for prefix pattern")
	}
}

func TestModel_MatchesPattern_SuffixWildcard(t *testing.T) {
	if !matchesPattern("test-auth", "*auth") {
		t.Errorf("expected suffix match")
	}
	if matchesPattern("auth-test", "*auth") {
		t.Errorf("expected no match for suffix pattern")
	}
}

func TestModel_MatchesPattern_DoubleWildcard(t *testing.T) {
	if !matchesPattern("test-auth-success", "*auth*") {
		t.Errorf("expected contains match")
	}
}

func TestModel_MatchesPattern_AllWildcard(t *testing.T) {
	if !matchesPattern("anything", "*") {
		t.Errorf("expected * to match anything")
	}
}
