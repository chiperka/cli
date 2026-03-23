package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Print AI-readable tool reference",
	Long: `Context outputs a markdown reference for AI agents and LLM tools.

Pipe it into your AI agent's context, project instructions, or save as a file:
  spark context >> CLAUDE.md
  spark context > .cursorrules
  spark context > AGENTS.md`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(strings.ReplaceAll(ContextText, "{{VERSION}}", Version))
	},
}

func init() {
	rootCmd.AddCommand(contextCmd)
}

const ContextText = `# Spark Test Runner ({{VERSION}})

Spark runs integration tests in isolated Docker containers.
Tests are defined in ` + "`" + `.spark` + "`" + ` YAML files. Each test starts services,
waits for healthchecks, executes HTTP or CLI commands, and evaluates assertions.

## Commands

### spark run [path]
Run tests. Exit codes: 0=passed, 1=assertion failures, 2=infrastructure errors.

Key flags:
  --json              NDJSON output for machine consumption
  --filter "pattern"  Run tests matching name pattern (supports * wildcard)
  --tags smoke,api    Run tests with specified tags
  --configuration f   Path to spark.yaml config file
  --html dir          Generate HTML reports to directory
  --junit file        Generate JUnit XML report
  --verbose           Detailed log output
  --workers N         Parallel workers (0=auto)
  --timeout N         Seconds per test (default 300)

### spark validate [path]
Validate test files without executing. Exit codes: 0=valid, 1=error, 3=validation errors.

Key flags:
  --json              NDJSON output
  --filter, --tags    Same as run

### spark context
Print this AI-readable tool reference.

## Test file format (.spark)

` + "```" + `yaml
name: Suite Name
tests:
  - name: test-name
    tags: [smoke, api]
    services:
      - name: api
        image: myapp:latest
        healthcheck:
          test: "curl -f http://localhost:8080/health"
          retries: 30
      - name: db
        ref: postgres          # references template from spark.yaml
        environment:
          POSTGRES_DB: testdb  # overrides template values
    setup:
      - http:
          target: http://api:8080
          request:
            method: POST
            url: /seed
    execution:
      executor: http           # "http" (default) or "cli"
      target: http://api:8080
      request:
        method: GET
        url: /api/users
    assertions:
      - statusCode:
          equals: 200
    teardown:
      - http:
          target: http://api:8080
          request:
            method: POST
            url: /cleanup
` + "```" + `

## Configuration (spark.yaml)

Defines reusable service templates referenced via ` + "`" + `ref:` + "`" + ` in test files.
Auto-discovered in working directory, or set with --configuration.

` + "```" + `yaml
services:
  postgres:
    image: postgres:15
    healthcheck:
      test: "pg_isready"
      retries: 30
    environment:
      POSTGRES_PASSWORD: test
` + "```" + `

## JSON output format (spark run --json)

NDJSON — one JSON object per line:

` + "```" + `
{"event":"run.started","timestamp":"...","data":{"tests":5,"suites":2,"workers":4}}
{"event":"test.started","timestamp":"...","suite":"Auth","test":"login","file":"tests/auth.spark"}
{"event":"test.completed","timestamp":"...","suite":"Auth","test":"login","data":{"status":"passed","duration_ms":843,"assertions":[{"assertion":"statusCode == 200","status":"pass"}]}}
{"event":"test.failed","timestamp":"...","suite":"API","test":"get-users","data":{"status":"failed","duration_ms":412,"assertions":[{"assertion":"statusCode == 200","status":"fail","expected":"200","actual":"404"}]}}
{"event":"run.completed","timestamp":"...","data":{"passed":4,"failed":1,"skipped":0,"duration_ms":3200}}
` + "```" + `

## Validate JSON output (spark validate --json)

` + "```" + `
{"event":"file.valid","file":"tests/auth.spark","tests":3}
{"event":"issue","level":"error","file":"tests/api.spark","suite":"API","test":"get-users","message":"no services defined"}
{"event":"summary","files":2,"suites":2,"tests":5,"errors":1,"warnings":0}
` + "```" + `

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | All tests passed / validation OK |
| 1 | Test assertion failures / general error |
| 2 | Infrastructure error (service startup, healthcheck, setup failed) |
| 3 | Validation errors (spark validate only) |

## Workflow

- Run ` + "`" + `spark validate --json` + "`" + ` to catch config errors without Docker
- Use ` + "`" + `--filter` + "`" + ` and ` + "`" + `--tags` + "`" + ` to run subsets
- Use ` + "`" + `--json` + "`" + ` for structured output parseable by scripts and AI agents
- Check exit code to distinguish assertion failures (1) from infra errors (2)

## MCP Server

### spark mcp
Start MCP server for AI tool integration (JSON-RPC over stdio).
Tools: spark_context, spark_list, spark_validate, spark_run.
Configure in .mcp.json for Claude Code or claude_desktop_config.json for Claude Desktop.`
