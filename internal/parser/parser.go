// Package parser handles reading and parsing of .chiperka files.
//
// A .chiperka file declares one resource. The top-level `kind:` field selects
// what is being declared:
//
//   - kind: test    (or no kind field) → a test suite (model.Suite)
//   - kind: service → a service template (model.ServiceTemplate)
//
// ParseAll walks a list of file paths, dispatches each file by kind, and
// returns a ParseResult with both collections populated.
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

// kindPeek is a minimal struct used to read just the top-level `kind` field
// before deciding how to fully decode a document.
type kindPeek struct {
	Kind string `yaml:"kind"`
}

// detectKind reads only the top-level `kind:` field and returns it. If the
// field is missing or empty, returns model.KindTest (the default).
func detectKind(data []byte) (string, error) {
	var peek kindPeek
	if err := yaml.Unmarshal(data, &peek); err != nil {
		return "", err
	}
	if peek.Kind == "" {
		return model.KindTest, nil
	}
	return peek.Kind, nil
}

// Parser reads and parses .chiperka files into model resources.
type Parser struct{}

// New creates a new Parser instance.
func New() *Parser {
	return &Parser{}
}

// ParseResult contains all resources discovered from a set of .chiperka files,
// split by kind.
type ParseResult struct {
	Tests    *model.TestCollection
	Services *model.ServiceTemplateCollection
	Errors   []error
}

// ParseFile reads a single .chiperka file as a test suite. It returns an
// error if the file is not a test (e.g. kind: service). For dispatch by kind,
// use ParseAll instead.
func (p *Parser) ParseFile(filePath string) (*model.Suite, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	data = expandEnvVars(data)

	kind, err := detectKind(data)
	if err != nil {
		return nil, fmt.Errorf("failed to read kind in %s: %w", filePath, err)
	}
	if kind != model.KindTest {
		return nil, fmt.Errorf("file %s has kind %q, expected %q", filePath, kind, model.KindTest)
	}

	var suite model.Suite
	if err := yaml.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("failed to parse YAML in %s: %w", filePath, err)
	}

	suite.FilePath = filePath
	return &suite, nil
}

// ParseBytes parses YAML test definition from raw bytes as a test suite.
// Used for API/MCP-submitted inline tests. Errors if the inline document is
// not a test.
func (p *Parser) ParseBytes(data []byte) (*model.Suite, error) {
	data = expandEnvVars(data)

	kind, err := detectKind(data)
	if err != nil {
		return nil, fmt.Errorf("failed to read kind: %w", err)
	}
	if kind != model.KindTest {
		return nil, fmt.Errorf("inline document has kind %q, expected %q", kind, model.KindTest)
	}

	var suite model.Suite
	if err := yaml.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	suite.FilePath = "<api>"
	return &suite, nil
}

// ParseAll reads multiple .chiperka files, dispatches each by kind, and
// returns a populated ParseResult. Per-file errors are collected in
// result.Errors and do not abort processing of remaining files.
func (p *Parser) ParseAll(filePaths []string) *ParseResult {
	result := &ParseResult{
		Tests:    model.NewTestCollection(),
		Services: model.NewServiceTemplateCollection(),
		Errors:   make([]error, 0),
	}

	for _, path := range filePaths {
		data, err := os.ReadFile(path)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to read file %s: %w", path, err))
			continue
		}

		data = expandEnvVars(data)

		kind, err := detectKind(data)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to read kind in %s: %w", path, err))
			continue
		}

		switch kind {
		case model.KindTest:
			var suite model.Suite
			if err := yaml.Unmarshal(data, &suite); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("failed to parse YAML in %s: %w", path, err))
				continue
			}
			suite.FilePath = path
			result.Tests.AddSuite(suite)

		case model.KindService:
			var template model.ServiceTemplate
			if err := yaml.Unmarshal(data, &template); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("failed to parse YAML in %s: %w", path, err))
				continue
			}
			if template.Name == "" {
				result.Errors = append(result.Errors, fmt.Errorf("%s: service is missing required 'name' field", path))
				continue
			}
			if existing := result.Services.GetTemplate(template.Name); existing != nil {
				result.Errors = append(result.Errors, fmt.Errorf(
					"%s: duplicate service name %q (already declared in %s)",
					path, template.Name, existing.FilePath,
				))
				continue
			}
			template.FilePath = path
			result.Services.AddTemplate(&template)

		default:
			result.Errors = append(result.Errors, fmt.Errorf("%s: unknown kind %q (expected %q or %q)", path, kind, model.KindTest, model.KindService))
		}
	}

	return result
}
