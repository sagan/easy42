package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// jsonResult returns a CallToolResult containing both structured data (for MCP clients with output schema validation)
// and formatted JSON text content (for backwards compatibility with text-only LLMs).
func jsonResult(data any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response JSON: %v", err)), nil
	}
	return mcp.NewToolResultStructured(data, string(b)), nil
}

// structuredResult is an alias for jsonResult
func structuredResult(data any) (*mcp.CallToolResult, error) {
	return jsonResult(data)
}

// errorResult wraps a Go error into an MCP tool error result
func errorResult(err error) (*mcp.CallToolResult, error) {
	if err == nil {
		return messageResult("OK")
	}
	return mcp.NewToolResultError(err.Error()), nil
}

// successText wraps a text message into a SimpleMessageOutput with both structured content and text
func successText(msg string) (*mcp.CallToolResult, error) {
	return messageResult(msg)
}

// messageResult wraps a text message into a SimpleMessageOutput with both structured content and text
func messageResult(msg string) (*mcp.CallToolResult, error) {
	return jsonResult(SimpleMessageOutput{Message: msg})
}
