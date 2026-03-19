package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"spark-cli/internal/envfile"
)

// Exit codes for the CLI.
const (
	ExitOK              = 0 // All tests passed / command succeeded
	ExitTestFailure     = 1 // Test assertion failures
	ExitInfraError      = 2 // Infrastructure error (Docker, network, config loading, etc.)
	ExitValidationError = 3 // Validation errors in test files
	ExitCancelled       = 4 // Run was manually cancelled
)

// ExitError wraps an error with a specific exit code.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

// exitErrorf creates an ExitError with the given code and formatted message.
func exitErrorf(code int, format string, args ...interface{}) *ExitError {
	return &ExitError{Code: code, Err: fmt.Errorf(format, args...)}
}

var envFiles []string

var rootCmd = &cobra.Command{
	Use:     "spark",
	Short:   "Integration test runner with Docker isolation",
	Long:    `Spark discovers and executes integration tests defined in *.spark files, running each test in isolated Docker containers with dedicated networks.`,
	Version: Version,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if len(envFiles) > 0 {
			if err := envfile.LoadAll(envFiles); err != nil {
				return fmt.Errorf("failed to load env file: %w", err)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringSliceVar(&envFiles, "env-file", nil, "Load environment variables from file (can be specified multiple times)")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			fmt.Fprintln(os.Stderr, exitErr.Err)
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitTestFailure)
	}
}
