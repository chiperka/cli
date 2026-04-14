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
  chiperka context >> CLAUDE.md
  chiperka context > .cursorrules
  chiperka context > AGENTS.md`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(strings.ReplaceAll(ContextText, "{{VERSION}}", Version))
	},
}

func init() {
	rootCmd.AddCommand(contextCmd)
}

const ContextText = `# Chiperka Test Runner ({{VERSION}})

Chiperka runs integration tests in isolated Docker containers.
Tests are defined in ` + "`" + `.chiperka` + "`" + ` YAML files. Each test starts services,
waits for healthchecks, executes HTTP or CLI commands, and evaluates assertions.

## Commands

### chiperka test [path]
Run tests. Exit codes: 0=passed, 1=assertion failures, 2=infrastructure errors.

Key flags:
  --json              NDJSON output for machine consumption
  --filter "pattern"  Run tests matching name pattern (supports * wildcard)
  --tags smoke,api    Run tests with specified tags
  --configuration f   Path to chiperka.yaml config file
  --html dir          Generate HTML reports to directory
  --junit file        Generate JUnit XML report
  --verbose           Detailed log output
  --timeout N         Seconds per test (default 300)

### chiperka validate [path]
Validate test files without executing. Exit codes: 0=valid, 1=error, 3=validation errors.

Key flags:
  --json              NDJSON output
  --filter, --tags    Same as run

### chiperka context
Print this AI-readable tool reference.

### chiperka result runs
List recent test runs with summary (UUID, status, pass/fail counts).
  --limit N           Maximum runs to show (default 20)
  --json              JSON output

### chiperka result run <run-uuid>
Show run summary with test list. Each test has a UUID for drill-down.
  --last              Show most recent run
  --json              JSON output

### chiperka result test <test-uuid>
Show full test detail: assertions, HTTP exchanges, CLI executions, services, artifacts.
Available artifact names are listed in the test detail output.
  --json              JSON output

### chiperka result artifact <test-uuid> <filename>
Output raw artifact content to stdout (response bodies, logs, etc.).

## Test file format (.chiperka)

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
        ref: postgres          # references template from chiperka.yaml
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

## Configuration (.chiperka/chiperka.yaml)

Defines reusable service templates referenced via ` + "`" + `ref:` + "`" + ` in test files.
Auto-discovered in .chiperka/ directory, or set with --configuration.

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

## JSON output format (chiperka test --json)

NDJSON — one JSON object per line:

` + "```" + `
{"event":"run.started","timestamp":"...","data":{"tests":5,"suites":2,"workers":4}}
{"event":"test.started","timestamp":"...","suite":"Auth","test":"login","file":"tests/auth.chiperka"}
{"event":"test.completed","timestamp":"...","suite":"Auth","test":"login","data":{"status":"passed","duration_ms":843,"assertions":[{"assertion":"statusCode == 200","status":"pass"}]}}
{"event":"test.failed","timestamp":"...","suite":"API","test":"get-users","data":{"status":"failed","duration_ms":412,"assertions":[{"assertion":"statusCode == 200","status":"fail","expected":"200","actual":"404"}]}}
{"event":"run.completed","timestamp":"...","data":{"passed":4,"failed":1,"skipped":0,"duration_ms":3200}}
` + "```" + `

## Validate JSON output (chiperka validate --json)

` + "```" + `
{"event":"file.valid","file":"tests/auth.chiperka","tests":3}
{"event":"issue","level":"error","file":"tests/api.chiperka","suite":"API","test":"get-users","message":"no services defined"}
{"event":"summary","files":2,"suites":2,"tests":5,"errors":1,"warnings":0}
` + "```" + `

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | All tests passed / validation OK |
| 1 | Test assertion failures / general error |
| 2 | Infrastructure error (service startup, healthcheck, setup failed) |
| 3 | Validation errors (chiperka validate only) |

## Workflow

- Run ` + "`" + `chiperka validate --json` + "`" + ` to catch config errors without Docker
- Use ` + "`" + `--filter` + "`" + ` and ` + "`" + `--tags` + "`" + ` to run subsets
- Use ` + "`" + `--json` + "`" + ` for structured output parseable by scripts and AI agents
- Check exit code to distinguish assertion failures (1) from infra errors (2)

## MCP Server

### chiperka mcp
Start MCP server for AI tool integration (JSON-RPC over stdio).
Configure in .mcp.json for Claude Code or claude_desktop_config.json for Claude Desktop.

MCP tools:
  chiperka_context      - AI reference documentation
  chiperka_list         - Discover tests and service templates
  chiperka_read         - Read and parse test files as JSON
  chiperka_validate     - Validate without executing
  chiperka_execute      - Run inline YAML test
  chiperka_run          - Execute tests, returns run UUID + summary
  chiperka_read_runs    - List recent test runs
  chiperka_read_run     - Read run summary with test UUIDs
  chiperka_read_test    - Read test detail with artifact names
  chiperka_read_artifact - Read raw artifact content by test UUID + filename

## Progressive result disclosure

After chiperka_run, results are stored hierarchically. Instead of reading everything
at once, drill down only where needed:

1. chiperka_run → run UUID + pass/fail summary
2. chiperka_read_run(uuid) → test list with statuses + UUIDs
3. chiperka_read_test(uuid) → assertions, HTTP exchanges, artifact names
4. chiperka_read_artifact(test_uuid, name) → raw response body, logs, etc.

Run UUID prefixes: lr- (local run), cr- (cloud run).`
