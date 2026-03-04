// Package mcp implements a Model Context Protocol server for AI tool integration.
package mcp

import (
	"github.com/mark3labs/mcp-go/server"
)

// defaultConfigFile is set at startup via --configuration flag.
var defaultConfigFile string

// Run starts the MCP server over stdio.
func Run(version, contextText, configFile string) error {
	defaultConfigFile = configFile
	s := server.NewMCPServer("spark", version)

	s.AddTool(contextTool(), handleContext(contextText))
	s.AddTool(listTool(), handleList)
	s.AddTool(readTool(), handleRead)
	s.AddTool(validateTool(), handleValidate)
	s.AddTool(executeTool(), handleExecute(version))
	s.AddTool(runTool(), handleRun(version))

	return server.ServeStdio(s)
}
