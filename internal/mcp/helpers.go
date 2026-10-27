package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// jsonResult marshals any response into indented JSON text result
func jsonResult(data any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response JSON: %v", err)), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

// errorResult wraps a Go error into an MCP tool error result
func errorResult(err error) (*mcp.CallToolResult, error) {
	if err == nil {
		return mcp.NewToolResultText("OK"), nil
	}
	return mcp.NewToolResultError(err.Error()), nil
}

// successText wraps a text message into an MCP tool success result
func successText(msg string) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText(msg), nil
}
