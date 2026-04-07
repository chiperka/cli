package cmd

import (
	"fmt"
	"os"
	"time"

	"chiperka-cli/internal/telemetry"
	"path/filepath"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new Chiperka project",
	Long: `Init scaffolds a starter Chiperka project in the current directory.

It creates:
  - chiperka.yaml       Configuration with an example service template
  - tests/health.chiperka  Health check test (1 test case)
  - tests/api.chiperka     API tests (2 test cases)

If chiperka.yaml already exists, init exits without modifying anything.

Example:
  mkdir my-project && cd my-project
  chiperka init
  chiperka run tests`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE:         runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

const chiperkaYAMLContent = `services:
  api:
    image: nginx:alpine
    healthcheck:
      test: "wget -q --spider http://localhost:80/"
      retries: 30
`

const healthChiperkaContent = `name: Health checks

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

const apiChiperkaContent = `name: API tests

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
	telemetry.ShowNoticeIfNeeded(false)
	startTime := time.Now()
	defer func() {
		telemetry.RecordCommand(Version, "init", "", true, time.Since(startTime).Milliseconds())
		telemetry.Wait(2 * time.Second)
	}()

	// Check if chiperka.yaml already exists
	if _, err := os.Stat("chiperka.yaml"); err == nil {
		fmt.Println("chiperka.yaml already exists, skipping initialization")
		return nil
	}

	// Create tests directory
	if err := os.MkdirAll("tests", 0755); err != nil {
		return fmt.Errorf("failed to create tests directory: %w", err)
	}

	// Write chiperka.yaml
	if err := os.WriteFile("chiperka.yaml", []byte(chiperkaYAMLContent), 0644); err != nil {
		return fmt.Errorf("failed to write chiperka.yaml: %w", err)
	}

	// Write tests/health.chiperka
	if err := os.WriteFile(filepath.Join("tests", "health.chiperka"), []byte(healthChiperkaContent), 0644); err != nil {
		return fmt.Errorf("failed to write tests/health.chiperka: %w", err)
	}

	// Write tests/api.chiperka
	if err := os.WriteFile(filepath.Join("tests", "api.chiperka"), []byte(apiChiperkaContent), 0644); err != nil {
		return fmt.Errorf("failed to write tests/api.chiperka: %w", err)
	}

	fmt.Println("Created chiperka.yaml")
	fmt.Println("Created tests/health.chiperka")
	fmt.Println("Created tests/api.chiperka")
	fmt.Println()
	fmt.Println("Run your tests with: chiperka run tests")

	return nil
}
