package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chiperka-cli/internal/telemetry"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new Chiperka project",
	Long: `Init scaffolds a starter Chiperka project in the current directory.

It creates:
  - .chiperka/chiperka.yaml  Configuration with discovery paths
  - services/api.chiperka    Example service template
  - tests/health.chiperka    Health check test (1 test case)
  - tests/api.chiperka       API tests (2 test cases)

If .chiperka/chiperka.yaml already exists, init exits without modifying anything.

Example:
  mkdir my-project && cd my-project
  chiperka init
  chiperka test`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE:         runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

const chiperkaYAMLContent = `discovery:
  - ./tests
  - ./services
`

const apiServiceContent = `kind: service
name: api
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

	configPath := filepath.Join(".chiperka", "chiperka.yaml")

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		fmt.Println(".chiperka/chiperka.yaml already exists, skipping initialization")
		return nil
	}

	// Create directories
	if err := os.MkdirAll(".chiperka", 0755); err != nil {
		return fmt.Errorf("failed to create .chiperka directory: %w", err)
	}
	if err := os.MkdirAll("tests", 0755); err != nil {
		return fmt.Errorf("failed to create tests directory: %w", err)
	}
	if err := os.MkdirAll("services", 0755); err != nil {
		return fmt.Errorf("failed to create services directory: %w", err)
	}

	// Write .chiperka/chiperka.yaml
	if err := os.WriteFile(configPath, []byte(chiperkaYAMLContent), 0644); err != nil {
		return fmt.Errorf("failed to write .chiperka/chiperka.yaml: %w", err)
	}

	// Write tests/health.chiperka
	if err := os.WriteFile(filepath.Join("tests", "health.chiperka"), []byte(healthChiperkaContent), 0644); err != nil {
		return fmt.Errorf("failed to write tests/health.chiperka: %w", err)
	}

	// Write tests/api.chiperka
	if err := os.WriteFile(filepath.Join("tests", "api.chiperka"), []byte(apiChiperkaContent), 0644); err != nil {
		return fmt.Errorf("failed to write tests/api.chiperka: %w", err)
	}

	// Write services/api.chiperka (service template)
	if err := os.WriteFile(filepath.Join("services", "api.chiperka"), []byte(apiServiceContent), 0644); err != nil {
		return fmt.Errorf("failed to write services/api.chiperka: %w", err)
	}

	// Add .chiperka/results/ to .gitignore if it exists
	if _, err := os.Stat(".gitignore"); err == nil {
		content, err := os.ReadFile(".gitignore")
		if err == nil && !containsLine(string(content), ".chiperka/results/") {
			f, err := os.OpenFile(".gitignore", os.O_APPEND|os.O_WRONLY, 0644)
			if err == nil {
				if len(content) > 0 && content[len(content)-1] != '\n' {
					f.Write([]byte("\n"))
				}
				f.Write([]byte(".chiperka/results/\n"))
				f.Close()
				fmt.Println("Added .chiperka/results/ to .gitignore")
			}
		}
	}

	fmt.Println("Created .chiperka/chiperka.yaml")
	fmt.Println("Created services/api.chiperka")
	fmt.Println("Created tests/health.chiperka")
	fmt.Println("Created tests/api.chiperka")
	fmt.Println()
	fmt.Println("Run your tests with: chiperka test")

	return nil
}

func containsLine(content, line string) bool {
	for _, l := range strings.Split(content, "\n") {
		if strings.TrimSpace(l) == line {
			return true
		}
	}
	return false
}
