package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"spark-cli/internal/mcp"
)

var mcpConfigFile string

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server for AI tool integration",
	Long: `Starts a Model Context Protocol server over stdio.

The MCP server exposes Spark functionality as structured tools that AI agents
(Claude Code, Cursor, Claude Desktop) can call directly:

  spark_context   - Get AI-readable tool reference
  spark_list      - Discover tests in a directory
  spark_read      - Read test files as structured JSON
  spark_validate  - Validate test files without executing
  spark_execute   - Run inline YAML test definition
  spark_run       - Execute tests and return results

Configure in .mcp.json for Claude Code:
  {"mcpServers": {"spark": {"command": "spark", "args": ["mcp"]}}}

With configuration:
  {"mcpServers": {"spark": {"command": "spark", "args": ["mcp", "--configuration", "spark.yaml"]}}}`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		contextText := strings.ReplaceAll(ContextText, "{{VERSION}}", Version)
		return mcp.Run(Version, contextText, mcpConfigFile)
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
	mcpCmd.Flags().StringVar(&mcpConfigFile, "configuration", "", "Path to spark.yaml configuration file (used as default for all tools)")
}
