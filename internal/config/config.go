// Package config handles loading of chiperka.yaml configuration files.
//
// chiperka.yaml is the optional CLI configuration file (typically at
// .chiperka/chiperka.yaml). It holds settings that apply across runs:
// execution variables, cloud configuration, etc. Service templates do not
// live here — they are declared as standalone .chiperka files with
// `kind: service`.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

// envVarPattern matches environment variables with $CHIPERKA_ prefix.
var envVarPattern = regexp.MustCompile(`\$CHIPERKA_[A-Za-z0-9_]+`)

// expandEnvVars replaces all $CHIPERKA_* patterns with their environment variable values.
func expandEnvVars(data []byte) []byte {
	return envVarPattern.ReplaceAllFunc(data, func(match []byte) []byte {
		varName := string(match[1:]) // Remove the $ prefix
		return []byte(os.Getenv(varName))
	})
}

// CloudConfig defines cloud-related configuration in chiperka.yaml.
type CloudConfig struct {
	URL     string `yaml:"url,omitempty"`
	Project string `yaml:"project,omitempty"` // project slug
}

// Config represents the contents of a chiperka.yaml configuration file.
type Config struct {
	ExecutionVariables map[string]string `yaml:"executionVariables"`
	Cloud              CloudConfig       `yaml:"cloud,omitempty"`
}

// Load reads a configuration file from the given path.
//
// Returns an error if the file cannot be read or parsed.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file %s: %w", path, err)
	}

	data = expandEnvVars(data)

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse configuration file %s: %w", path, err)
	}

	return &cfg, nil
}

// Discover looks for .chiperka/chiperka.yaml or .chiperka/chiperka.yml in the current working directory.
// Returns the config and true if found, or an empty config and false if not found.
//
// If the file is found but fails to load, Discover returns an empty config
// and false. Callers that need the load error should call Load directly.
func Discover() (*Config, bool) {
	for _, name := range []string{
		filepath.Join(".chiperka", "chiperka.yaml"),
		filepath.Join(".chiperka", "chiperka.yml"),
	} {
		if _, err := os.Stat(name); err == nil {
			cfg, err := Load(name)
			if err != nil {
				return &Config{}, false
			}
			return cfg, true
		}
	}
	return &Config{}, false
}
