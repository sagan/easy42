package mcp

import (
	"context"
	"errors"
)

type roleContextKey struct{}

const (
	// RoleReadOnly allows inspecting and querying cluster resources
	RoleReadOnly = "read_only"
	// RoleFullControl allows modifying configuration and executing cluster operations
	RoleFullControl = "full_control"
)

var (
	// ErrUnauthorized is returned when credentials are invalid or missing
	ErrUnauthorized = errors.New("unauthorized: missing or invalid MCP token")
	// ErrForbiddenReadOnly is returned when a mutating operation is attempted with a read-only token
	ErrForbiddenReadOnly = errors.New("permission denied: this operation modifies cluster configuration and requires a full-control MCP token (read-only token was provided)")
	// ErrMCPServerDisabled is returned when the MCP server is disabled in settings
	ErrMCPServerDisabled = errors.New("mcp server is currently disabled in easy42 settings")
)

// WithRole injects the authenticated MCP role into the context
func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, roleContextKey{}, role)
}

// GetRole extracts the authenticated MCP role from the context
func GetRole(ctx context.Context) string {
	if val, ok := ctx.Value(roleContextKey{}).(string); ok {
		return val
	}
	return ""
}

// RequireFullControl enforces that the calling context has full control privileges
func RequireFullControl(ctx context.Context) error {
	role := GetRole(ctx)
	if role != RoleFullControl {
		return ErrForbiddenReadOnly
	}
	return nil
}
