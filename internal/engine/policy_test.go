package engine

import (
	"testing"

	"easy42/internal/config"
)

func TestNetworkPolicyManagerCRUD(t *testing.T) {
	tempDir := t.TempDir()
	store := config.NewStore(tempDir)
	pass, err := store.Initialize()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}

	mgr := NewManager(store)
	if err := mgr.Unlock(pass); err != nil {
		t.Fatalf("Failed to unlock manager: %v", err)
	}

	// 1. Initial policies should include the 3 built-ins
	policies := mgr.GetNetworkPolicies()
	if len(policies) != 3 {
		t.Fatalf("expected 3 initial builtin policies, got %d", len(policies))
	}

	// 2. Reject creating policy with reserved ID
	_, err = mgr.CreateNetworkPolicy(config.NetworkPolicy{
		ID:   "default",
		Name: "Hacked Default",
	})
	if err == nil {
		t.Fatalf("expected error creating policy with reserved ID 'default'")
	}

	// 3. Create custom policy (unset Cost should default to 100)
	dscpIn := 46
	dscpOut := 10
	custom, err := mgr.CreateNetworkPolicy(config.NetworkPolicy{
		ID:                 "pol-office",
		Name:               "Branch Office",
		Description:        "Office subnet policy",
		AllowedDstCIDRs:    []string{"172.20.100.0/24"},
		AllowedSrcCIDRs:    []string{"172.20.200.0/24"},
		DisallowedDstCIDRs: []string{"172.20.100.128/25"},
		DisallowedSrcCIDRs: []string{"172.20.200.128/25"},
		AllowedImportCIDRs: []string{"172.20.200.0/24"},
		RejectInternet:     true,
		FilterForward:      true,
		FilterInput:        true,
		DSCPIngress:        &dscpIn,
		DSCPEgress:         &dscpOut,
	})
	if err != nil {
		t.Fatalf("CreateNetworkPolicy failed: %v", err)
	}
	if custom.ID != "pol-office" || custom.IsInternal || custom.Cost != 100 {
		t.Errorf("unexpected custom policy: %+v", custom)
	}
	if len(custom.DisallowedDstCIDRs) != 1 || custom.DisallowedDstCIDRs[0] != "172.20.100.128/25" {
		t.Errorf("expected DisallowedDstCIDRs [172.20.100.128/25], got %v", custom.DisallowedDstCIDRs)
	}
	if len(custom.DisallowedSrcCIDRs) != 1 || custom.DisallowedSrcCIDRs[0] != "172.20.200.128/25" {
		t.Errorf("expected DisallowedSrcCIDRs [172.20.200.128/25], got %v", custom.DisallowedSrcCIDRs)
	}
	if custom.DSCPIngress == nil || *custom.DSCPIngress != 46 {
		t.Errorf("expected DSCPIngress 46, got %v", custom.DSCPIngress)
	}
	if custom.DSCPEgress == nil || *custom.DSCPEgress != 10 {
		t.Errorf("expected DSCPEgress 10, got %v", custom.DSCPEgress)
	}

	// 4. Reject duplicate ID
	_, err = mgr.CreateNetworkPolicy(config.NetworkPolicy{
		ID:   "pol-office",
		Name: "Duplicate",
	})
	if err == nil {
		t.Fatalf("expected error creating duplicate policy ID")
	}

	// 5. Update custom policy with custom Cost and updated DSCP
	dscpInUpdated := 0
	updated, err := mgr.UpdateNetworkPolicy("pol-office", config.NetworkPolicy{
		Name:               "Branch Office Updated",
		Cost:               250,
		AllowedDstCIDRs:    []string{"172.20.101.0/24"},
		AllowedSrcCIDRs:    []string{"172.20.201.0/24"},
		DisallowedDstCIDRs: []string{"172.20.101.128/25"},
		DisallowedSrcCIDRs: []string{"172.20.201.128/25"},
		AllowedImportCIDRs: []string{"172.20.201.0/24"},
		DSCPIngress:        &dscpInUpdated,
	})
	if err != nil {
		t.Fatalf("UpdateNetworkPolicy failed: %v", err)
	}
	if updated.Name != "Branch Office Updated" || updated.Cost != 250 {
		t.Errorf("expected updated name and cost 250, got name=%s cost=%d", updated.Name, updated.Cost)
	}
	if len(updated.DisallowedDstCIDRs) != 1 || updated.DisallowedDstCIDRs[0] != "172.20.101.128/25" {
		t.Errorf("expected updated DisallowedDstCIDRs [172.20.101.128/25], got %v", updated.DisallowedDstCIDRs)
	}
	if len(updated.DisallowedSrcCIDRs) != 1 || updated.DisallowedSrcCIDRs[0] != "172.20.201.128/25" {
		t.Errorf("expected updated DisallowedSrcCIDRs [172.20.201.128/25], got %v", updated.DisallowedSrcCIDRs)
	}
	if updated.DSCPIngress == nil || *updated.DSCPIngress != 0 {
		t.Errorf("expected updated DSCPIngress 0, got %v", updated.DSCPIngress)
	}
	if updated.DSCPEgress != nil {
		t.Errorf("expected updated DSCPEgress nil, got %v", updated.DSCPEgress)
	}

	// 6. Reject updating built-in policy
	_, err = mgr.UpdateNetworkPolicy("default", config.NetworkPolicy{Name: "Modified Default"})
	if err == nil {
		t.Fatalf("expected error updating built-in policy 'default'")
	}

	// 7. Reject deleting built-in policy
	err = mgr.DeleteNetworkPolicy("dn42")
	if err == nil {
		t.Fatalf("expected error deleting built-in policy 'dn42'")
	}

	// 8. Create nodes and a link using custom policy
	n1 := config.Node{Name: "node-a", Host: "host1", IP: "192.168.1.1", Interface: "eth0", ASN: 4224420001}
	if err := mgr.AddNode(n1); err != nil {
		t.Fatalf("AddNode node-a failed: %v", err)
	}
	n2 := config.Node{Name: "node-b", Host: "host2", IP: "192.168.1.2", Interface: "eth0", ASN: 4224420002}
	if err := mgr.AddNode(n2); err != nil {
		t.Fatalf("AddNode node-b failed: %v", err)
	}

	link, err := mgr.AddLinkAdvanced(n1.Name, n2.Name, &config.LinkEnd{Policy: "pol-office"}, &config.LinkEnd{Policy: "default"}, nil)
	if err != nil {
		t.Fatalf("AddLinkAdvanced failed: %v", err)
	}
	if link.From.Policy != "pol-office" && link.To.Policy != "pol-office" {
		t.Fatalf("expected one LinkEnd to have policy 'pol-office', got from=%s, to=%s", link.From.Policy, link.To.Policy)
	}

	// 9. Cannot delete policy in use
	err = mgr.DeleteNetworkPolicy("pol-office")
	if err == nil {
		t.Fatalf("expected error deleting policy in use by link")
	}

	// 10. Update link to not use the policy, then delete policy succeeds
	_, err = mgr.UpdateLinkAdvanced(n1.Name, n2.Name, &config.LinkEnd{Policy: "none"}, &config.LinkEnd{Policy: "none"}, nil)
	if err != nil {
		t.Fatalf("UpdateLinkAdvanced failed: %v", err)
	}

	err = mgr.DeleteNetworkPolicy("pol-office")
	if err != nil {
		t.Fatalf("DeleteNetworkPolicy failed after link updated: %v", err)
	}

	// Verify it is no longer listed
	policies = mgr.GetNetworkPolicies()
	if len(policies) != 3 {
		t.Fatalf("expected 3 policies after deletion, got %d", len(policies))
	}
}
