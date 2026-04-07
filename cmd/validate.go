package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"chiperka-cli/internal/config"
	"chiperka-cli/internal/finder"
	"chiperka-cli/internal/model"
	"chiperka-cli/internal/parser"
	"chiperka-cli/internal/telemetry"
)

var validateJSON bool
var validateTags []string
var validateFilter string
var validateConfigFile string

var validateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate test files without executing them",
	Long: `Validate checks chiperka test files for structural and semantic errors
without starting any Docker containers or executing tests.

Catches issues like missing service images, broken template references,
missing execution blocks, and other configuration problems upfront.

Exit codes:
  0  All files valid (warnings are OK)
  1  General error (path not found, config loading, etc.)
  3  Validation errors found in test files

Example:
  chiperka validate ./tests
  chiperka validate ./tests/auth.chiperka
  chiperka validate ./tests --json
  chiperka validate ./tests --tags smoke
  chiperka validate ./tests --filter "login*"`,
	Args:          cobra.MaximumNArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
	validateCmd.Flags().BoolVar(&validateJSON, "json", false, "Output NDJSON for machine consumption")
	validateCmd.Flags().StringSliceVar(&validateTags, "tags", nil, "Validate only tests with specified tags")
	validateCmd.Flags().StringVar(&validateFilter, "filter", "", "Validate only tests matching pattern")
	validateCmd.Flags().StringVar(&validateConfigFile, "configuration", "", "Path to chiperka.yaml configuration file (auto-discovered if not set)")
}

type validationIssue struct {
	Level   string `json:"level"`
	File    string `json:"file"`
	Suite   string `json:"suite,omitempty"`
	Test    string `json:"test,omitempty"`
	Message string `json:"message"`
}

func runValidate(cmd *cobra.Command, args []string) error {
	telemetry.ShowNoticeIfNeeded(validateJSON)
	startTime := time.Now()
	defer func() {
		telemetry.RecordCommand(Version, "validate", "", true, time.Since(startTime).Milliseconds())
		telemetry.Wait(2 * time.Second)
	}()

	searchPath := "."
	if len(args) > 0 {
		searchPath = args[0]
	}

	// Verify path exists
	info, err := os.Stat(searchPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", searchPath)
	}

	// Find test files
	var files []string
	if !info.IsDir() && strings.HasSuffix(searchPath, ".chiperka") {
		files = []string{searchPath}
	} else {
		f := finder.New(searchPath)
		files, err = f.FindTestFiles()
		if err != nil {
			return fmt.Errorf("failed to find test files: %w", err)
		}
	}

	if len(files) == 0 {
		fmt.Printf("No *.chiperka files found in %s\n", searchPath)
		return nil
	}

	// Parse all files
	p := parser.New()
	parseResult := p.ParseAll(files)

	// Load configuration
	cfg, err := loadValidateConfig()
	if err != nil {
		return err
	}
	services := cfg.ServiceTemplates()

	// Collect issues per file
	var allIssues []validationIssue
	validFiles := 0
	totalTests := 0
	totalSuites := 0

	// Report parse errors as issues
	for _, parseErr := range parseResult.Errors {
		issue := validationIssue{
			Level:   "error",
			File:    parseErr.Error(), // parse error includes the file path
			Message: parseErr.Error(),
		}
		allIssues = append(allIssues, issue)
	}

	// Apply filters
	tests := parseResult.Tests
	if len(validateTags) > 0 {
		tests = tests.FilterByTags(validateTags)
	}
	if validateFilter != "" {
		tests = tests.FilterByName(validateFilter)
	}

	if tests.TotalTests() == 0 && len(parseResult.Errors) == 0 {
		if len(validateTags) > 0 || validateFilter != "" {
			if validateJSON {
				writeJSON(map[string]interface{}{
					"event":    "summary",
					"files":    len(files),
					"suites":   0,
					"tests":    0,
					"errors":   0,
					"warnings": 0,
				})
			} else {
				fmt.Println("No tests match the specified filters")
			}
			return nil
		}
	}

	// Validate each suite
	for _, suite := range tests.Suites {
		totalSuites++
		var fileIssues []validationIssue

		// Suite-level checks
		if suite.Name == "" {
			fileIssues = append(fileIssues, validationIssue{
				Level:   "error",
				File:    suite.FilePath,
				Suite:   suite.Name,
				Message: "suite name is empty",
			})
		}
		if len(suite.Tests) == 0 {
			fileIssues = append(fileIssues, validationIssue{
				Level:   "error",
				File:    suite.FilePath,
				Suite:   suite.Name,
				Message: "suite has no tests",
			})
		}

		// Per-test checks
		for _, test := range suite.Tests {
			totalTests++
			issues := validateTest(test, suite, services)
			fileIssues = append(fileIssues, issues...)
		}

		if validateJSON {
			if hasErrors(fileIssues) {
				for _, issue := range fileIssues {
					writeJSON(map[string]interface{}{
						"event":   "issue",
						"level":   issue.Level,
						"file":    suite.FilePath,
						"suite":   issue.Suite,
						"test":    issue.Test,
						"message": issue.Message,
					})
				}
			} else {
				writeJSON(map[string]interface{}{
					"event": "file.valid",
					"file":  suite.FilePath,
					"tests": len(suite.Tests),
				})
			}
		} else {
			if len(fileIssues) == 0 {
				fmt.Printf("  \u2713 %s (%d tests)\n", suite.FilePath, len(suite.Tests))
				validFiles++
			} else {
				hasErr := hasErrors(fileIssues)
				if !hasErr {
					validFiles++
				}
				fmt.Printf("  \u2717 %s\n", suite.FilePath)
				for _, issue := range fileIssues {
					fmt.Printf("      %s: %s\n", issue.Level, formatIssueMessage(issue))
				}
			}
		}

		allIssues = append(allIssues, fileIssues...)
	}

	// Summary
	errorCount := 0
	warningCount := 0
	for _, issue := range allIssues {
		if issue.Level == "error" {
			errorCount++
		} else {
			warningCount++
		}
	}

	if validateJSON {
		writeJSON(map[string]interface{}{
			"event":    "summary",
			"files":    len(files),
			"suites":   totalSuites,
			"tests":    totalTests,
			"errors":   errorCount,
			"warnings": warningCount,
		})
	} else {
		fmt.Printf("\nValidated %d tests in %d suites: %d errors, %d warnings\n",
			totalTests, totalSuites, errorCount, warningCount)
	}

	if errorCount > 0 {
		return exitErrorf(ExitValidationError, "validation failed: %d error(s) found", errorCount)
	}

	return nil
}

func validateTest(test model.Test, suite model.Suite, services *model.ServiceTemplateCollection) []validationIssue {
	var issues []validationIssue

	// Test name
	if test.Name == "" {
		issues = append(issues, validationIssue{
			Level:   "error",
			File:    suite.FilePath,
			Suite:   suite.Name,
			Message: "test name is empty",
		})
	}

	// Services required
	if len(test.Services) == 0 {
		issues = append(issues, validationIssue{
			Level:   "error",
			File:    suite.FilePath,
			Suite:   suite.Name,
			Test:    test.Name,
			Message: "no services defined",
		})
	}

	// Per-service checks
	for _, svc := range test.Services {
		if svc.Ref != "" {
			// Resolve template reference
			resolved, err := services.ResolveService(svc)
			if err != nil {
				issues = append(issues, validationIssue{
					Level:   "error",
					File:    suite.FilePath,
					Suite:   suite.Name,
					Test:    test.Name,
					Message: fmt.Sprintf("service %q: template %q not found", svcDisplayName(svc), svc.Ref),
				})
				continue
			}
			// Check resolved image
			if resolved.Image == "" {
				issues = append(issues, validationIssue{
					Level:   "error",
					File:    suite.FilePath,
					Suite:   suite.Name,
					Test:    test.Name,
					Message: fmt.Sprintf("service %q: image is empty after resolving template %q", svcDisplayName(svc), svc.Ref),
				})
			}
		} else if svc.Image == "" {
			issues = append(issues, validationIssue{
				Level:   "error",
				File:    suite.FilePath,
				Suite:   suite.Name,
				Test:    test.Name,
				Message: fmt.Sprintf("service %q: image is empty", svcDisplayName(svc)),
			})
		}
	}

	// Execution checks
	exec := test.Execution
	switch exec.Executor {
	case model.ExecutorHTTP, "":
		// HTTP executor (default)
		if exec.Target == "" {
			issues = append(issues, validationIssue{
				Level:   "error",
				File:    suite.FilePath,
				Suite:   suite.Name,
				Test:    test.Name,
				Message: "execution target is empty",
			})
		}
		if exec.Request.Method == "" {
			issues = append(issues, validationIssue{
				Level:   "error",
				File:    suite.FilePath,
				Suite:   suite.Name,
				Test:    test.Name,
				Message: "execution request method is empty",
			})
		}
	case model.ExecutorCLI:
		if exec.CLI == nil {
			issues = append(issues, validationIssue{
				Level:   "error",
				File:    suite.FilePath,
				Suite:   suite.Name,
				Test:    test.Name,
				Message: "cli executor requires cli configuration",
			})
		} else {
			if exec.CLI.Service == "" {
				issues = append(issues, validationIssue{
					Level:   "error",
					File:    suite.FilePath,
					Suite:   suite.Name,
					Test:    test.Name,
					Message: "cli.service is empty",
				})
			}
			if exec.CLI.Command == "" {
				issues = append(issues, validationIssue{
					Level:   "error",
					File:    suite.FilePath,
					Suite:   suite.Name,
					Test:    test.Name,
					Message: "cli.command is empty",
				})
			}
		}
	default:
		issues = append(issues, validationIssue{
			Level:   "error",
			File:    suite.FilePath,
			Suite:   suite.Name,
			Test:    test.Name,
			Message: fmt.Sprintf("unknown executor type %q (must be \"http\" or \"cli\")", exec.Executor),
		})
	}

	// Assertions warning
	if len(test.Assertions) == 0 {
		issues = append(issues, validationIssue{
			Level:   "warning",
			File:    suite.FilePath,
			Suite:   suite.Name,
			Test:    test.Name,
			Message: "no assertions defined",
		})
	}

	return issues
}

func svcDisplayName(svc model.Service) string {
	if svc.Name != "" {
		return svc.Name
	}
	if svc.Ref != "" {
		return svc.Ref
	}
	return "(unnamed)"
}

func formatIssueMessage(issue validationIssue) string {
	if issue.Test != "" {
		return fmt.Sprintf("test %q %s", issue.Test, issue.Message)
	}
	return issue.Message
}

func hasErrors(issues []validationIssue) bool {
	for _, issue := range issues {
		if issue.Level == "error" {
			return true
		}
	}
	return false
}

func writeJSON(data map[string]interface{}) {
	b, _ := json.Marshal(data)
	fmt.Println(string(b))
}

func loadValidateConfig() (*config.Config, error) {
	if validateConfigFile != "" {
		cfg, err := config.Load(validateConfigFile)
		if err != nil {
			return nil, err
		}
		return cfg, nil
	}

	cfg, _ := config.Discover()
	return cfg, nil
}
