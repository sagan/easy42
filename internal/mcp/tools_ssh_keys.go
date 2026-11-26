package mcp

import (
	"context"
	"fmt"

	"easy42/internal/config"
	"easy42/internal/engine"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerSSHKeyTools(s *server.MCPServer, mgr *engine.Manager) {
	// 1. list_ssh_keys
	s.AddTool(mcp.NewTool("list_ssh_keys",
		mcp.WithDescription("List the root user SSH public keys collected from each node agent, the SSH key rules, and the SSH key management support state per node. Requires agent mode; nodes with disable_apps = true and agents older than 0.1.18 are excluded."),
		mcp.WithTitleAnnotation("List SSH Keys"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[ListSSHKeysOutput](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodes := mgr.GetSSHKeysOverview()
		if nodes == nil {
			nodes = []engine.SSHKeyNodeState{}
		}
		rules := mgr.GetSSHKeyRules()
		if rules == nil {
			rules = []config.SSHKeyRule{}
		}
		return jsonResult(ListSSHKeysOutput{
			Nodes:   nodes,
			Rules:   rules,
			Summary: mgr.GetSSHKeysSummary(),
		})
	})

	// 2. create_ssh_key_rule
	s.AddTool(mcp.NewTool("create_ssh_key_rule",
		mcp.WithDescription("Create a rule that installs the SSH public keys of the source nodes (or direct source key lines) into the known_hosts and/or authorized_keys files of the destination nodes. Requires full-control token."),
		mcp.WithTitleAnnotation("Create SSH Key Rule"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[SSHKeyRuleMutationOutput](),
		mcp.WithString("id", mcp.Description("Optional custom SSH key rule ID (1-64 chars: letters, digits, '.', '_', '-'). Auto-generated if omitted.")),
		mcp.WithString("type", mcp.Required(), mcp.Description("Target file type: 'known_hosts', 'authorized_keys' or 'all'")),
		mcp.WithObject("src_selector", mcp.Description("Node selector for the source nodes whose public keys are distributed, e.g. {\"tag\":\"core\"}. Use {\"all\":\"1\"} for every node. Provide either src_selector or source_keys."), mcp.AdditionalProperties(map[string]any{"type": "string"})),
		mcp.WithString("source_keys", mcp.Description("Direct source key lines in authorized_keys ('<algorithm> <key> [comment]') or known_hosts ('<hostnames> <algorithm> <key> [comment]') format, one per line. Provide either src_selector or source_keys.")),
		mcp.WithObject("dst_selector", mcp.Required(), mcp.Description("Node selector for the destination nodes whose SSH key files are updated, e.g. {\"name\":\"node-a,node-b\"}. Use {\"all\":\"1\"} for every node."), mcp.AdditionalProperties(map[string]any{"type": "string"})),
		mcp.WithBoolean("enabled", mcp.Description("Enable the rule immediately (default true)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		ruleType, err := req.RequireString("type")
		if err != nil {
			return errorResult(err)
		}
		srcSelector, err := parseOptionalSelectorArg(req, "src_selector")
		if err != nil {
			return errorResult(err)
		}
		sourceKeys := req.GetString("source_keys", "")
		dstSelector, err := parseRequiredSelectorArg(req, "dst_selector")
		if err != nil {
			return errorResult(err)
		}

		enabled := true
		if args := req.GetArguments(); args != nil {
			if _, ok := args["enabled"]; ok {
				enabled = req.GetBool("enabled", true)
			}
		}

		saved, err := mgr.CreateSSHKeyRule(config.SSHKeyRule{
			ID:          req.GetString("id", ""),
			Type:        ruleType,
			SrcSelector: srcSelector,
			SourceKeys:  sourceKeys,
			DstSelector: dstSelector,
			Enabled:     enabled,
		})
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(SSHKeyRuleMutationOutput{
			Message: fmt.Sprintf("SSH key rule %q created successfully", saved.ID),
			Rule:    saved,
		})
	})

	// 3. update_ssh_key_rule
	s.AddTool(mcp.NewTool("update_ssh_key_rule",
		mcp.WithDescription("Update an existing SSH key rule. Requires full-control token."),
		mcp.WithTitleAnnotation("Update SSH Key Rule"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[SSHKeyRuleMutationOutput](),
		mcp.WithString("id", mcp.Required(), mcp.Description("SSH key rule ID")),
		mcp.WithString("new_id", mcp.Description("Optional new SSH key rule ID when renaming the rule")),
		mcp.WithString("type", mcp.Required(), mcp.Description("Target file type: 'known_hosts', 'authorized_keys' or 'all'")),
		mcp.WithObject("src_selector", mcp.Description("Node selector for the source nodes. Provide either src_selector or source_keys."), mcp.AdditionalProperties(map[string]any{"type": "string"})),
		mcp.WithString("source_keys", mcp.Description("Direct source key lines in authorized_keys or known_hosts format, one per line. Provide either src_selector or source_keys.")),
		mcp.WithObject("dst_selector", mcp.Required(), mcp.Description("Node selector for the destination nodes (use {\"all\":\"1\"} for every node)"), mcp.AdditionalProperties(map[string]any{"type": "string"})),
		mcp.WithBoolean("enabled", mcp.Description("Whether the rule is enabled (default true)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		id, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		if _, err := mgr.GetSSHKeyRule(id); err != nil {
			return errorResult(err)
		}
		ruleType, err := req.RequireString("type")
		if err != nil {
			return errorResult(err)
		}
		srcSelector, err := parseOptionalSelectorArg(req, "src_selector")
		if err != nil {
			return errorResult(err)
		}
		sourceKeys := req.GetString("source_keys", "")
		dstSelector, err := parseRequiredSelectorArg(req, "dst_selector")
		if err != nil {
			return errorResult(err)
		}
		enabled := req.GetBool("enabled", true)

		newID := req.GetString("new_id", "")
		if newID == "" {
			newID = id
		}
		saved, err := mgr.UpdateSSHKeyRule(id, config.SSHKeyRule{
			ID:          newID,
			Type:        ruleType,
			SrcSelector: srcSelector,
			SourceKeys:  sourceKeys,
			DstSelector: dstSelector,
			Enabled:     enabled,
		})
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(SSHKeyRuleMutationOutput{
			Message: fmt.Sprintf("SSH key rule %q updated successfully", saved.ID),
			Rule:    saved,
		})
	})

	// 4. delete_ssh_key_rule
	s.AddTool(mcp.NewTool("delete_ssh_key_rule",
		mcp.WithDescription("Delete an SSH key rule. The managed SSH key blocks are removed from the destination nodes on the next sync. Requires full-control token."),
		mcp.WithTitleAnnotation("Delete SSH Key Rule"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[SimpleMessageOutput](),
		mcp.WithString("id", mcp.Required(), mcp.Description("SSH key rule ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		id, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		if err := mgr.DeleteSSHKeyRule(id); err != nil {
			return errorResult(err)
		}
		return successText(fmt.Sprintf("SSH key rule %q deleted", id))
	})
}

// parseOptionalSelectorArg parses an optional string-valued selector object argument.
func parseOptionalSelectorArg(req mcp.CallToolRequest, name string) (map[string]string, error) {
	args := req.GetArguments()
	if args == nil {
		return nil, nil
	}
	raw, ok := args[name]
	if !ok || raw == nil {
		return nil, nil
	}
	return parseRequiredSelectorArg(req, name)
}

// parseRequiredSelectorArg parses a required string-valued selector object argument.
func parseRequiredSelectorArg(req mcp.CallToolRequest, name string) (map[string]string, error) {
	args := req.GetArguments()
	if args == nil {
		return nil, fmt.Errorf("%s is required", name)
	}
	raw, ok := args[name]
	if !ok || raw == nil {
		return nil, fmt.Errorf("%s is required", name)
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object with string values", name)
	}
	selector := make(map[string]string, len(obj))
	for key, value := range obj {
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%s value for %q must be a string", name, key)
		}
		selector[key] = str
	}
	return selector, nil
}
