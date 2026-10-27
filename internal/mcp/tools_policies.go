package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"easy42/internal/config"
	"easy42/internal/engine"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerPolicyTools(s *server.MCPServer, mgr *engine.Manager) {
	// 1. list_network_policies
	s.AddTool(mcp.NewTool("list_network_policies",
		mcp.WithDescription("List all BGP routing and firewall network policies defined in Easy42"),
		mcp.WithTitleAnnotation("List Network Policies"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		policies := mgr.GetNetworkPolicies()
		return jsonResult(policies)
	})

	// 2. get_network_policy
	s.AddTool(mcp.NewTool("get_network_policy",
		mcp.WithDescription("Get a specific network policy by its ID"),
		mcp.WithTitleAnnotation("Get Network Policy"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("id", mcp.Required(), mcp.Description("Policy ID (e.g. 'default', 'dn42', or custom ID)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		policy, err := mgr.GetNetworkPolicy(id)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(policy)
	})

	// 3. create_network_policy
	s.AddTool(mcp.NewTool("create_network_policy",
		mcp.WithDescription("Create a new network policy for BGP route filtering, firewall rules, and ROA validation. Requires full-control token."),
		mcp.WithTitleAnnotation("Create Network Policy"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("id", mcp.Required(), mcp.Description("Unique policy identifier (alphanumeric/hyphen)")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Human-readable policy name")),
		mcp.WithString("description", mcp.Description("Optional description of the policy")),
		mcp.WithInteger("cost", mcp.Description("Cost deducted from bgp_local_pref on internal hops (default 100)")),
		mcp.WithString("routing_policy", mcp.Description("BGP meta routing policy: 'full', 'stub', 'receive_only', or 'advertise_only'")),
		mcp.WithString("policy_json", mcp.Description("Optional full JSON representation for advanced firewall rules, ROA, SNAT, and BIRD filters")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		id, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		name, err := req.RequireString("name")
		if err != nil {
			return errorResult(err)
		}

		policy := config.NetworkPolicy{
			ID:            id,
			Name:          name,
			Description:   req.GetString("description", ""),
			Cost:          req.GetInt("cost", 100),
			RoutingPolicy: req.GetString("routing_policy", "full"),
		}

		if rawJSON := req.GetString("policy_json", ""); rawJSON != "" {
			if err := json.Unmarshal([]byte(strings.TrimSpace(rawJSON)), &policy); err != nil {
				return errorResult(fmt.Errorf("failed to parse policy_json: %w", err))
			}
			policy.ID = id
			policy.Name = name
		}

		created, err := mgr.CreateNetworkPolicy(policy)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(map[string]any{
			"message": fmt.Sprintf("Network policy %q created successfully", id),
			"policy":  created,
		})
	})

	// 4. update_network_policy
	s.AddTool(mcp.NewTool("update_network_policy",
		mcp.WithDescription("Update an existing network policy. Requires full-control token."),
		mcp.WithTitleAnnotation("Update Network Policy"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("id", mcp.Required(), mcp.Description("Policy ID to update")),
		mcp.WithString("name", mcp.Description("Updated human-readable name")),
		mcp.WithString("description", mcp.Description("Updated description")),
		mcp.WithInteger("cost", mcp.Description("Routing cost")),
		mcp.WithString("routing_policy", mcp.Description("Routing policy mode")),
		mcp.WithString("policy_json", mcp.Description("Optional full JSON representation to overwrite entire policy")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		id, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		existing, err := mgr.GetNetworkPolicy(id)
		if err != nil {
			return errorResult(err)
		}

		policy := *existing
		if rawJSON := req.GetString("policy_json", ""); rawJSON != "" {
			if err := json.Unmarshal([]byte(strings.TrimSpace(rawJSON)), &policy); err != nil {
				return errorResult(fmt.Errorf("failed to parse policy_json: %w", err))
			}
			policy.ID = id
		} else {
			if name := req.GetString("name", ""); name != "" {
				policy.Name = name
			}
			if desc := req.GetString("description", ""); desc != "" {
				policy.Description = desc
			}
			if cost := req.GetInt("cost", 0); cost > 0 {
				policy.Cost = cost
			}
			if rp := req.GetString("routing_policy", ""); rp != "" {
				policy.RoutingPolicy = rp
			}
		}

		updated, err := mgr.UpdateNetworkPolicy(id, policy)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(map[string]any{
			"message": fmt.Sprintf("Network policy %q updated successfully", id),
			"policy":  updated,
		})
	})

	// 5. delete_network_policy
	s.AddTool(mcp.NewTool("delete_network_policy",
		mcp.WithDescription("Delete a custom network policy. Requires full-control token."),
		mcp.WithTitleAnnotation("Delete Network Policy"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("id", mcp.Required(), mcp.Description("Policy ID to delete")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		id, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		if err := mgr.DeleteNetworkPolicy(id); err != nil {
			return errorResult(err)
		}
		return successText(fmt.Sprintf("Network policy %q deleted successfully", id))
	})

	// 6. refresh_policy_roa
	s.AddTool(mcp.NewTool("refresh_policy_roa",
		mcp.WithDescription("Refresh Route Origin Authorization (ROA) tables for a specific policy or all policies. Requires full-control token."),
		mcp.WithTitleAnnotation("Refresh Policy ROA"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("policy_id", mcp.Description("Optional specific policy ID. If omitted, refreshes all policies")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		policyID := req.GetString("policy_id", "")
		if policyID != "" {
			if err := mgr.RefreshPolicyROA(ctx, policyID); err != nil {
				return errorResult(err)
			}
			return successText(fmt.Sprintf("Refreshed ROA for policy %s", policyID))
		}

		if err := mgr.RefreshAllROA(ctx, true); err != nil {
			return errorResult(err)
		}
		return successText("Refreshed all network policy ROA tables successfully")
	})
}
