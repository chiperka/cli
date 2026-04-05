// Package parser handles reading and parsing of chiperka.yaml test files.
package parser

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
	"chiperka-cli/internal/model"
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

// Parser reads and parses chiperka.yaml files into Suite structures.
type Parser struct{}

// New creates a new Parser instance.
func New() *Parser {
	return &Parser{}
}

// ParseResult contains test suites from parsed files.
type ParseResult struct {
	Tests  *model.TestCollection
	Errors []error
}

// ParseFile reads a single chiperka.yaml file and returns a Suite.
func (p *Parser) ParseFile(filePath string) (*model.Suite, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	data = expandEnvVars(data)

	var suite model.Suite
	if err := yaml.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("failed to parse YAML in %s: %w", filePath, err)
	}

	// Store the source file path for reference
	suite.FilePath = filePath

	return &suite, nil
}

// ParseBytes parses YAML test definition from raw bytes.
// This is used for API-submitted tests where the YAML comes from HTTP requests.
func (p *Parser) ParseBytes(data []byte) (*model.Suite, error) {
	data = expandEnvVars(data)

	var suite model.Suite
	if err := yaml.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Set a placeholder file path for API-submitted tests
	suite.FilePath = "<api>"

	return &suite, nil
}

// ParseAll reads multiple chiperka files and returns a test collection.
// All files are parsed as test files.
func (p *Parser) ParseAll(filePaths []string) *ParseResult {
	result := &ParseResult{
		Tests:  model.NewTestCollection(),
		Errors: make([]error, 0),
	}

	for _, path := range filePaths {
		data, err := os.ReadFile(path)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to read file %s: %w", path, err))
			continue
		}

		data = expandEnvVars(data)

		var suite model.Suite
		if err := yaml.Unmarshal(data, &suite); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to parse YAML in %s: %w", path, err))
			continue
		}
		suite.FilePath = path
		result.Tests.AddSuite(suite)
	}

	return result
}
