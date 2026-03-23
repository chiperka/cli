package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new Spark project",
	Long: `Init scaffolds a starter Spark project in the current directory.

It creates:
  - spark.yaml       Configuration with an example service template
  - tests/health.spark  Health check test (1 test case)
  - tests/api.spark     API tests (2 test cases)

If spark.yaml already exists, init exits without modifying anything.

Example:
  mkdir my-project && cd my-project
  spark init
  spark run tests`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE:         runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

const sparkYAMLContent = `services:
  api:
    image: nginx:alpine
    healthcheck:
      test: "wget -q --spider http://localhost:80/"
      retries: 30
`

const healthSparkContent = `name: Health checks

tests:
  - name: API is reachable
    services:
      - name: api
        ref: api
    execution:
      target: http://api
      request:
        method: GET
        url: /
    assertions:
      - response:
          statusCode: 200
`

const apiSparkContent = `name: API tests

tests:
  - name: Homepage returns content
    services:
      - name: api
        ref: api
    execution:
      target: http://api
      request:
        method: GET
        url: /
    assertions:
      - response:
          statusCode: 200

  - name: Returns 404 for missing page
    services:
      - name: api
        ref: api
    execution:
      target: http://api
      request:
        method: GET
        url: /missing-page
    assertions:
      - response:
          statusCode: 404
`

func runInit(cmd *cobra.Command, args []string) error {
	// Check if spark.yaml already exists
	if _, err := os.Stat("spark.yaml"); err == nil {
		fmt.Println("spark.yaml already exists, skipping initialization")
		return nil
	}

	// Create tests directory
	if err := os.MkdirAll("tests", 0755); err != nil {
		return fmt.Errorf("failed to create tests directory: %w", err)
	}

	// Write spark.yaml
	if err := os.WriteFile("spark.yaml", []byte(sparkYAMLContent), 0644); err != nil {
		return fmt.Errorf("failed to write spark.yaml: %w", err)
	}

	// Write tests/health.spark
	if err := os.WriteFile(filepath.Join("tests", "health.spark"), []byte(healthSparkContent), 0644); err != nil {
		return fmt.Errorf("failed to write tests/health.spark: %w", err)
	}

	// Write tests/api.spark
	if err := os.WriteFile(filepath.Join("tests", "api.spark"), []byte(apiSparkContent), 0644); err != nil {
		return fmt.Errorf("failed to write tests/api.spark: %w", err)
	}

	fmt.Println("Created spark.yaml")
	fmt.Println("Created tests/health.spark")
	fmt.Println("Created tests/api.spark")
	fmt.Println()
	fmt.Println("Run your tests with: spark run tests")

	return nil
}
