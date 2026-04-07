package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// TelemetryConfig holds the telemetry consent state.
type TelemetryConfig struct {
	Enabled     bool `json:"enabled"`
	NoticeShown bool `json:"notice_shown"`
}

// MachineConfig is the top-level structure for ~/.chiperka/config.json.
type MachineConfig struct {
	InstallID string          `json:"install_id,omitempty"`
	Telemetry TelemetryConfig `json:"telemetry"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// legacyConfig mirrors the old flat telemetry.json format for migration.
type legacyConfig struct {
	Enabled     bool      `json:"enabled"`
	NoticeShown bool      `json:"notice_shown"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// configDir returns the path to ~/.chiperka/
func configDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".chiperka")
}

// configPath returns the path to ~/.chiperka/config.json
func configPath() string {
	dir := configDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "config.json")
}

// legacyConfigPath returns the path to the old ~/.chiperka/telemetry.json
func legacyConfigPath() string {
	dir := configDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "telemetry.json")
}

// LoadConfig reads the telemetry config from ~/.chiperka/config.json.
// If config.json is missing but telemetry.json exists, migrates the old format.
// Returns nil if no config exists or can't be read.
func LoadConfig() *TelemetryConfig {
	path := configPath()
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err == nil {
		var cfg MachineConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil
		}
		return &cfg.Telemetry
	}

	// config.json doesn't exist — try migrating from telemetry.json
	legacyPath := legacyConfigPath()
	if legacyPath == "" {
		return nil
	}

	legacyData, err := os.ReadFile(legacyPath)
	if err != nil {
		return nil
	}

	var old legacyConfig
	if err := json.Unmarshal(legacyData, &old); err != nil {
		return nil
	}

	// Migrate: save in new format
	tc := &TelemetryConfig{
		Enabled:     old.Enabled,
		NoticeShown: old.NoticeShown,
	}
	SaveConfig(tc)
	return tc
}

// LoadMachineConfig reads the full machine config from ~/.chiperka/config.json.
// Returns nil if no config exists or can't be read.
func LoadMachineConfig() *MachineConfig {
	path := configPath()
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var cfg MachineConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}

	return &cfg
}

// SaveConfig writes the telemetry config to ~/.chiperka/config.json.
// Loads the existing MachineConfig first to preserve other sections,
// then updates .Telemetry and .UpdatedAt.
// Creates ~/.chiperka/ if it doesn't exist. Silently ignores errors.
func SaveConfig(cfg *TelemetryConfig) {
	dir := configDir()
	if dir == "" {
		return
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	path := configPath()
	if path == "" {
		return
	}

	// Load existing machine config to preserve other sections
	var machine MachineConfig
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &machine)
	}

	machine.Telemetry = *cfg
	machine.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(machine, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(path, data, 0o644)
}

// saveMachineConfigWithInstallID saves telemetry config + install ID.
func saveMachineConfigWithInstallID(cfg *TelemetryConfig, installID string) {
	dir := configDir()
	if dir == "" {
		return
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	path := configPath()
	if path == "" {
		return
	}

	var machine MachineConfig
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &machine)
	}

	machine.InstallID = installID
	machine.Telemetry = *cfg
	machine.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(machine, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(path, data, 0o644)
}
