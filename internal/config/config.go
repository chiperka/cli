// Package config handles loading of spark.yaml configuration files.
package config

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
	"spark-cli/internal/model"
)

// envVarPattern matches environment variables with $SPARK_ prefix.
var envVarPattern = regexp.MustCompile(`\$SPARK_[A-Za-z0-9_]+`)

// expandEnvVars replaces all $SPARK_* patterns with their environment variable values.
func expandEnvVars(data []byte) []byte {
	return envVarPattern.ReplaceAllFunc(data, func(match []byte) []byte {
		varName := string(match[1:]) // Remove the $ prefix
		return []byte(os.Getenv(varName))
	})
}

// ServiceConfig defines a service in the configuration file.
type ServiceConfig struct {
	Image        string                  `yaml:"image,omitempty"`
	Command      model.ShellCommand      `yaml:"command,omitempty"`
	WorkingDir   string                  `yaml:"workingDir,omitempty"`
	Environment  map[string]string       `yaml:"environment,omitempty"`
	HealthCheck  *model.HealthCheck      `yaml:"healthcheck,omitempty"`
	Artifacts    []model.ServiceArtifact `yaml:"artifacts,omitempty"`
	MaxInstances int                     `yaml:"maxInstances,omitempty"`
}

// Config represents the contents of a spark.yaml configuration file.
type Config struct {
	Services           map[string]ServiceConfig `yaml:"services"`
	ExecutionVariables map[string]string        `yaml:"executionVariables"`
}

// Load reads a configuration file from the given path.
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

// Discover looks for spark.yaml or spark.yml in the current working directory.
// Returns the config and true if found, or an empty config and false if not found.
func Discover() (*Config, bool) {
	for _, name := range []string{"spark.yaml", "spark.yml"} {
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

// ServiceTemplates converts the config's services into a ServiceTemplateCollection
// compatible with the existing runner and cloud packages.
func (c *Config) ServiceTemplates() *model.ServiceTemplateCollection {
	collection := model.NewServiceTemplateCollection()
	if c == nil || c.Services == nil {
		return collection
	}

	for name, svc := range c.Services {
		template := &model.ServiceTemplate{
			Name:         name,
			Image:        svc.Image,
			Command:      svc.Command,
			WorkingDir:   svc.WorkingDir,
			Environment:  svc.Environment,
			HealthCheck:  svc.HealthCheck,
			Artifacts:    svc.Artifacts,
			MaxInstances: svc.MaxInstances,
		}
		collection.AddTemplate(template)
	}

	return collection
}
