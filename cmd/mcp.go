package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"chiperka-cli/internal/mcp"
)

var mcpConfigFile string

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server for AI tool integration",
	Long: `Starts a Model Context Protocol server over stdio.

The MCP server exposes Chiperka functionality as structured tools that AI agents
(Claude Code, Cursor, Claude Desktop) can call directly:

  chiperka_context   - Get AI-readable tool reference
  chiperka_list      - Discover tests in a directory
  chiperka_read      - Read test files as structured JSON
  chiperka_validate  - Validate test files without executing
  chiperka_execute   - Run inline YAML test definition
  chiperka_run       - Execute tests and return results

Configure in .mcp.json for Claude Code:
  {"mcpServers": {"chiperka": {"command": "chiperka", "args": ["mcp"]}}}

With configuration:
  {"mcpServers": {"chiperka": {"command": "chiperka", "args": ["mcp", "--configuration", "chiperka.yaml"]}}}`,
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
	mcpCmd.Flags().StringVar(&mcpConfigFile, "configuration", "", "Path to chiperka.yaml configuration file (used as default for all tools)")
}
