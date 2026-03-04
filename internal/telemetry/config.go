package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Config holds the telemetry consent state.
type Config struct {
	Enabled     bool      `json:"enabled"`
	NoticeShown bool      `json:"notice_shown"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// configDir returns the path to ~/.spark/
func configDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".spark")
}

// configPath returns the path to ~/.spark/telemetry
func configPath() string {
	dir := configDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "telemetry.json")
}

// LoadConfig reads the telemetry config from ~/.spark/telemetry.
// Returns nil if the file doesn't exist or can't be read.
func LoadConfig() *Config {
	path := configPath()
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}

	return &cfg
}

// SaveConfig writes the telemetry config to ~/.spark/telemetry.
// Creates ~/.spark/ if it doesn't exist. Silently ignores errors.
func SaveConfig(cfg *Config) {
	dir := configDir()
	if dir == "" {
		return
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	cfg.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}

	path := configPath()
	if path == "" {
		return
	}

	os.WriteFile(path, data, 0o644)
}
