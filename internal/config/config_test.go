package config

import (
	"os"
	"path/filepath"
	"testing"
)

// --- Load ---

func TestConfig_Load_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spark.yaml")
	content := `services:
  db:
    image: postgres:15
    environment:
      POSTGRES_DB: test
  redis:
    image: redis:7
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(cfg.Services))
	}
	db, ok := cfg.Services["db"]
	if !ok {
		t.Fatalf("expected 'db' service")
	}
	if db.Image != "postgres:15" {
		t.Errorf("expected postgres:15, got %q", db.Image)
	}
	if db.Environment["POSTGRES_DB"] != "test" {
		t.Errorf("expected POSTGRES_DB=test, got %q", db.Environment["POSTGRES_DB"])
	}
}

func TestConfig_Load_NonExistent(t *testing.T) {
	_, err := Load("/nonexistent/spark.yaml")
	if err == nil {
		t.Errorf("expected error for non-existent file")
	}
}

func TestConfig_Load_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spark.yaml")
	if err := os.WriteFile(path, []byte("invalid: [yaml"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Errorf("expected error for invalid YAML")
	}
}

func TestConfig_Load_WithEnvVars(t *testing.T) {
	os.Setenv("SPARK_DB_IMAGE", "postgres:16")
	t.Cleanup(func() { os.Unsetenv("SPARK_DB_IMAGE") })

	dir := t.TempDir()
	path := filepath.Join(dir, "spark.yaml")
	content := `services:
  db:
    image: $SPARK_DB_IMAGE
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Services["db"].Image != "postgres:16" {
		t.Errorf("expected postgres:16 (env expanded), got %q", cfg.Services["db"].Image)
	}
}

func TestConfig_Load_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spark.yaml")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Services != nil && len(cfg.Services) != 0 {
		t.Errorf("expected empty services, got %d", len(cfg.Services))
	}
}

// --- Discover ---

func TestConfig_Discover_SparkYaml(t *testing.T) {
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	content := "services:\n  db:\n    image: postgres:15\n"
	if err := os.WriteFile(filepath.Join(dir, "spark.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, found := Discover()
	if !found {
		t.Fatalf("expected to discover spark.yaml")
	}
	if len(cfg.Services) != 1 {
		t.Errorf("expected 1 service, got %d", len(cfg.Services))
	}
}

func TestConfig_Discover_SparkYml(t *testing.T) {
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	content := "services:\n  redis:\n    image: redis:7\n"
	if err := os.WriteFile(filepath.Join(dir, "spark.yml"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, found := Discover()
	if !found {
		t.Fatalf("expected to discover spark.yml")
	}
	if len(cfg.Services) != 1 {
		t.Errorf("expected 1 service, got %d", len(cfg.Services))
	}
}

func TestConfig_Discover_NotFound(t *testing.T) {
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	_, found := Discover()
	if found {
		t.Errorf("expected not found in empty directory")
	}
}

func TestConfig_Discover_YamlPrefersOverYml(t *testing.T) {
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	if err := os.WriteFile(filepath.Join(dir, "spark.yaml"), []byte("services:\n  db:\n    image: postgres:15\n"), 0644); err != nil {
		t.Fatalf("failed to write spark.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spark.yml"), []byte("services:\n  redis:\n    image: redis:7\n"), 0644); err != nil {
		t.Fatalf("failed to write spark.yml: %v", err)
	}

	cfg, found := Discover()
	if !found {
		t.Fatalf("expected to discover config")
	}
	// spark.yaml should be preferred (checked first)
	if _, ok := cfg.Services["db"]; !ok {
		t.Errorf("expected spark.yaml to be preferred over spark.yml")
	}
}

// --- ServiceTemplates ---

func TestConfig_ServiceTemplates(t *testing.T) {
	cfg := &Config{
		Services: map[string]ServiceConfig{
			"db": {
				Image:       "postgres:15",
				Environment: map[string]string{"POSTGRES_DB": "test"},
			},
			"redis": {
				Image: "redis:7",
			},
		},
	}

	collection := cfg.ServiceTemplates()
	if !collection.HasTemplates() {
		t.Fatalf("expected templates")
	}

	db := collection.GetTemplate("db")
	if db == nil {
		t.Fatalf("expected db template")
	}
	if db.Image != "postgres:15" {
		t.Errorf("expected postgres:15, got %q", db.Image)
	}
	if db.Environment["POSTGRES_DB"] != "test" {
		t.Errorf("expected POSTGRES_DB=test, got %q", db.Environment["POSTGRES_DB"])
	}

	redis := collection.GetTemplate("redis")
	if redis == nil {
		t.Fatalf("expected redis template")
	}
	if redis.Image != "redis:7" {
		t.Errorf("expected redis:7, got %q", redis.Image)
	}
}

func TestConfig_ServiceTemplates_Nil(t *testing.T) {
	var cfg *Config
	collection := cfg.ServiceTemplates()
	if collection.HasTemplates() {
		t.Errorf("expected no templates for nil config")
	}
}

func TestConfig_ServiceTemplates_NoServices(t *testing.T) {
	cfg := &Config{}
	collection := cfg.ServiceTemplates()
	if collection.HasTemplates() {
		t.Errorf("expected no templates for empty config")
	}
}

func TestConfig_ServiceTemplates_WithHealthcheck(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spark.yaml")
	content := `services:
  api:
    image: myapp:latest
    command: "serve --port 8080"
    healthcheck:
      test: "curl -f http://localhost:8080/health"
      retries: 30
    artifacts:
      - path: /var/log/app.log
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	collection := cfg.ServiceTemplates()
	api := collection.GetTemplate("api")
	if api == nil {
		t.Fatalf("expected api template")
	}
	if api.HealthCheck == nil {
		t.Fatalf("expected healthcheck")
	}
	if string(api.HealthCheck.Test) != "curl -f http://localhost:8080/health" {
		t.Errorf("expected healthcheck test, got %q", api.HealthCheck.Test)
	}
	if api.HealthCheck.Retries != 30 {
		t.Errorf("expected retries 30, got %d", api.HealthCheck.Retries)
	}
	if len(api.Artifacts) != 1 {
		t.Errorf("expected 1 artifact, got %d", len(api.Artifacts))
	}
	if len(api.Command) == 0 {
		t.Errorf("expected command to be set")
	}
}
