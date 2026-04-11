package config

import (
	"os"
	"path/filepath"
	"testing"
)

// --- Load: valid (no services) ---

func TestConfig_Load_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chiperka.yaml")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatalf("expected non-nil config")
	}
}

func TestConfig_Load_ExecutionVariables(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chiperka.yaml")
	content := `executionVariables:
  TARGET_URL: http://localhost:8080
  API_TOKEN: secret
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ExecutionVariables["TARGET_URL"] != "http://localhost:8080" {
		t.Errorf("expected TARGET_URL to be set, got %q", cfg.ExecutionVariables["TARGET_URL"])
	}
}

func TestConfig_Load_Cloud(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chiperka.yaml")
	content := `cloud:
  url: https://cloud.example.com
  project: my-project
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Cloud.URL != "https://cloud.example.com" {
		t.Errorf("expected cloud.url to be set, got %q", cfg.Cloud.URL)
	}
	if cfg.Cloud.Project != "my-project" {
		t.Errorf("expected cloud.project to be set, got %q", cfg.Cloud.Project)
	}
}

func TestConfig_Load_NonExistent(t *testing.T) {
	_, err := Load("/nonexistent/chiperka.yaml")
	if err == nil {
		t.Errorf("expected error for non-existent file")
	}
}

func TestConfig_Load_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chiperka.yaml")
	if err := os.WriteFile(path, []byte("invalid: [yaml"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Errorf("expected error for invalid YAML")
	}
}

// --- Discover ---

func TestConfig_Discover_ChiperkaYaml(t *testing.T) {
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	subdir := filepath.Join(dir, ".chiperka")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("failed to mkdir: %v", err)
	}

	content := "executionVariables:\n  KEY: value\n"
	if err := os.WriteFile(filepath.Join(subdir, "chiperka.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, found := Discover()
	if !found {
		t.Fatalf("expected to discover chiperka.yaml")
	}
	if cfg.ExecutionVariables["KEY"] != "value" {
		t.Errorf("expected KEY=value, got %q", cfg.ExecutionVariables["KEY"])
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
