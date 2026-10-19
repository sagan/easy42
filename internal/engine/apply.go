package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"easy42/internal/compiler"
	"easy42/internal/config"
	"easy42/internal/crypto"

	"github.com/google/uuid"
)

// ApplyFieldDiff represents a difference in a single attribute
type ApplyFieldDiff struct {
	Field    string      `json:"field"`
	OldValue interface{} `json:"old_value"`
	NewValue interface{} `json:"new_value"`
}

// ApplyNodeDiff represents modifications to a node
type ApplyNodeDiff struct {
	Name    string           `json:"name"`
	Action  string           `json:"action"` // "create" or "update"
	Changes []ApplyFieldDiff `json:"changes"`
}

// ApplyLinkDiff represents modifications to a link
type ApplyLinkDiff struct {
	From    string           `json:"from"`
	To      string           `json:"to"`
	Action  string           `json:"action"` // "create" or "update"
	Changes []ApplyFieldDiff `json:"changes"`
}

// ApplySettingsDiff represents modifications to global settings
type ApplySettingsDiff struct {
	Category string           `json:"category"`
	Changes  []ApplyFieldDiff `json:"changes"`
}

// ApplySummary summarizes planned changes
type ApplySummary struct {
	AddedNodes    int `json:"added_nodes"`
	ModifiedNodes int `json:"modified_nodes"`
	AddedLinks    int `json:"added_links"`
	ModifiedLinks int `json:"modified_links"`
	TotalChanges  int `json:"total_changes"`
}

// ApplyPreviewResult contains full validation and diff details
type ApplyPreviewResult struct {
	Valid          bool                `json:"valid"`
	Format         string              `json:"format"`
	Summary        ApplySummary        `json:"summary"`
	Nodes          []ApplyNodeDiff     `json:"nodes"`
	Links          []ApplyLinkDiff     `json:"links"`
	Settings       []ApplySettingsDiff `json:"settings"`
	UnifiedDiff    string              `json:"unified_diff"`
	Errors         []string            `json:"errors,omitempty"`
	NormalizedJSON string              `json:"normalized_json,omitempty"`
}

// ApplyResult represents the outcome of an applied config change
type ApplyResult struct {
	Success        bool               `json:"success"`
	AppliedAt      time.Time          `json:"applied_at"`
	Summary        ApplySummary       `json:"summary"`
	AffectedNodes  []string           `json:"affected_nodes"`
	AffectedLinks  []string           `json:"affected_links"`
	BackupFile     string             `json:"backup_file,omitempty"`
	Plan           ApplyPreviewResult `json:"plan"`
}

// GetNode returns a copy of a node by name
func (m *Manager) GetNode(name string) (*config.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.store.Get()
	if cfg == nil {
		return nil, errors.New("config not loaded")
	}
	for i := range cfg.Nodes {
		if strings.EqualFold(cfg.Nodes[i].Name, name) {
			copyNode := cfg.Nodes[i]
			return &copyNode, nil
		}
	}
	return nil, ErrNodeNotFound
}

// PreviewApply parses, validates, and simulates applying the patch, returning a detailed preview
func (m *Manager) PreviewApply(content string) (*ApplyPreviewResult, error) {
	patch, format, err := config.ParseAndValidateConfigChange([]byte(content))
	if err != nil {
		return &ApplyPreviewResult{
			Valid:  false,
			Errors: []string{err.Error()},
		}, err
	}

	m.mu.Lock()
	cfg := m.store.Get()
	// Deep clone current config
	cloned, err := cloneConfig(cfg)
	m.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("failed to clone config for preview: %w", err)
	}

	normBytes, _ := json.MarshalIndent(patch, "", "  ")

	// Apply in-memory simulation
	preview, _, err := m.simulateApply(cloned, patch)
	if err != nil {
		return &ApplyPreviewResult{
			Valid:          false,
			Format:         format,
			Errors:         []string{err.Error()},
			NormalizedJSON: string(normBytes),
		}, err
	}

	preview.Format = format
	preview.NormalizedJSON = string(normBytes)
	preview.Valid = true

	// Generate text unified diff
	origJSON, _ := json.MarshalIndent(filterDiffConfig(cfg), "", "  ")
	newJSON, _ := json.MarshalIndent(filterDiffConfig(cloned), "", "  ")
	preview.UnifiedDiff = generateSimpleDiff(string(origJSON), string(newJSON))

	return preview, nil
}

// ApplyConfigChange executes the patch atomically and saves the resulting config
func (m *Manager) ApplyConfigChange(content string) (*ApplyResult, error) {
	patch, _, err := config.ParseAndValidateConfigChange([]byte(content))
	if err != nil {
		return nil, fmt.Errorf("config change validation failed: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	cloned, err := cloneConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to clone config: %w", err)
	}

	preview, affectedNodes, err := m.simulateApply(cloned, patch)
	if err != nil {
		return nil, fmt.Errorf("apply simulation failed: %w", err)
	}

	now := time.Now().UTC()

	// Handle cryptographic keys for newly created links
	for i := range cloned.Links {
		l := &cloned.Links[i]
		if l.Type != config.LinkTypeManual && l.From.PublicKey == "" && l.To.PublicKey == "" {
			// Needs key generation
			fromN := findNodeInSlice(cloned.Nodes, l.From.Name)
			toN := findNodeInSlice(cloned.Nodes, l.To.Name)
			if fromN != nil && !fromN.IsExternal && m.vault.IsUnlocked() {
				kpFrom, err := crypto.GenerateWgKeyPair()
				if err == nil {
					l.From.PublicKey = kpFrom.PublicKey
					encPriv, err := m.vault.EncryptField(kpFrom.PrivateKey)
					if err == nil {
						l.From.PrivateKey = encPriv
					}
				}
			}
			if toN != nil && !toN.IsExternal && m.vault.IsUnlocked() {
				kpTo, err := crypto.GenerateWgKeyPair()
				if err == nil {
					l.To.PublicKey = kpTo.PublicKey
					encPriv, err := m.vault.EncryptField(kpTo.PrivateKey)
					if err == nil {
						l.To.PrivateKey = encPriv
					}
				}
			}
			if l.Psk == "" && m.vault.IsUnlocked() {
				rawPsk, err := crypto.GenerateWgPsk()
				if err == nil {
					if encPsk, err := m.vault.EncryptField(rawPsk); err == nil {
						l.Psk = encPsk
					}
				}
			}
		}
	}

	// Recompute link endpoints and link-local IPs
	m.recomputeLinksAndEndpoints(cloned, now)

	// Create backup
	backupFile := filepath.Join(m.store.DataDir(), fmt.Sprintf("config.json.bak.%d", now.Unix()))
	origBytes, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(backupFile, origBytes, 0600)
	_ = os.WriteFile(filepath.Join(m.store.DataDir(), "config.json.bak"), origBytes, 0600)

	// Save to store
	if err := m.store.Save(cloned); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	// Re-trigger host sync in background
	go m.SyncHostsToAgents()

	affectedLinks := make([]string, 0)
	if preview.Links != nil {
		for _, l := range preview.Links {
			affectedLinks = append(affectedLinks, fmt.Sprintf("%s <-> %s", l.From, l.To))
		}
	}

	return &ApplyResult{
		Success:       true,
		AppliedAt:     now,
		Summary:       preview.Summary,
		AffectedNodes: affectedNodes,
		AffectedLinks: affectedLinks,
		BackupFile:    backupFile,
		Plan:          *preview,
	}, nil
}

func (m *Manager) simulateApply(cfg *config.Config, patch *config.ConfigPatch) (*ApplyPreviewResult, []string, error) {
	now := time.Now().UTC()
	result := &ApplyPreviewResult{
		Nodes:    make([]ApplyNodeDiff, 0),
		Links:    make([]ApplyLinkDiff, 0),
		Settings: make([]ApplySettingsDiff, 0),
		Errors:   make([]string, 0),
	}
	affectedNodes := make([]string, 0)
	nodeMap := make(map[string]bool)

	// 1. Apply Nodes
	if patch.Nodes != nil {
		for _, np := range *patch.Nodes {
			existingIdx := -1
			for i, n := range cfg.Nodes {
				if strings.EqualFold(n.Name, np.Name) {
					existingIdx = i
					break
				}
			}

			if existingIdx >= 0 {
				// Update existing node
				diffs, err := mergeNode(&cfg.Nodes[existingIdx], np, now)
				if err != nil {
					return nil, nil, err
				}
				if len(diffs) > 0 {
					result.Nodes = append(result.Nodes, ApplyNodeDiff{
						Name:    np.Name,
						Action:  "update",
						Changes: diffs,
					})
					result.Summary.ModifiedNodes++
					result.Summary.TotalChanges += len(diffs)
					if !nodeMap[np.Name] {
						nodeMap[np.Name] = true
						affectedNodes = append(affectedNodes, np.Name)
					}
				}
			} else {
				// Create new node
				newNode, diffs, err := createNodeFromPatch(np, now)
				if err != nil {
					return nil, nil, err
				}
				cfg.Nodes = append(cfg.Nodes, *newNode)
				result.Nodes = append(result.Nodes, ApplyNodeDiff{
					Name:    np.Name,
					Action:  "create",
					Changes: diffs,
				})
				result.Summary.AddedNodes++
				result.Summary.TotalChanges += len(diffs)
				if !nodeMap[np.Name] {
					nodeMap[np.Name] = true
					affectedNodes = append(affectedNodes, np.Name)
				}
			}
		}
	}

	// 2. Apply Links
	if patch.Links != nil {
		for _, lp := range *patch.Links {
			existingIdx := -1
			for i, l := range cfg.Links {
				if (strings.EqualFold(l.From.Name, lp.From.Name) && strings.EqualFold(l.To.Name, lp.To.Name)) ||
					(strings.EqualFold(l.From.Name, lp.To.Name) && strings.EqualFold(l.To.Name, lp.From.Name)) {
					existingIdx = i
					break
				}
			}

			if existingIdx >= 0 {
				// Update existing link
				diffs, err := mergeLink(&cfg.Links[existingIdx], lp, now)
				if err != nil {
					return nil, nil, err
				}
				if len(diffs) > 0 {
					result.Links = append(result.Links, ApplyLinkDiff{
						From:    lp.From.Name,
						To:      lp.To.Name,
						Action:  "update",
						Changes: diffs,
					})
					result.Summary.ModifiedLinks++
					result.Summary.TotalChanges += len(diffs)
					if !nodeMap[lp.From.Name] {
						nodeMap[lp.From.Name] = true
						affectedNodes = append(affectedNodes, lp.From.Name)
					}
					if !nodeMap[lp.To.Name] {
						nodeMap[lp.To.Name] = true
						affectedNodes = append(affectedNodes, lp.To.Name)
					}
				}
			} else {
				// Verify both nodes exist
				if findNodeInSlice(cfg.Nodes, lp.From.Name) == nil {
					return nil, nil, fmt.Errorf("cannot create link: endpoint node '%s' does not exist", lp.From.Name)
				}
				if findNodeInSlice(cfg.Nodes, lp.To.Name) == nil {
					return nil, nil, fmt.Errorf("cannot create link: endpoint node '%s' does not exist", lp.To.Name)
				}

				newLink, diffs, err := createLinkFromPatch(lp, now)
				if err != nil {
					return nil, nil, err
				}
				cfg.Links = append(cfg.Links, *newLink)
				result.Links = append(result.Links, ApplyLinkDiff{
					From:    lp.From.Name,
					To:      lp.To.Name,
					Action:  "create",
					Changes: diffs,
				})
				result.Summary.AddedLinks++
				result.Summary.TotalChanges += len(diffs)
				if !nodeMap[lp.From.Name] {
					nodeMap[lp.From.Name] = true
					affectedNodes = append(affectedNodes, lp.From.Name)
				}
				if !nodeMap[lp.To.Name] {
					nodeMap[lp.To.Name] = true
					affectedNodes = append(affectedNodes, lp.To.Name)
				}
			}
		}
	}

	// 3. Apply NetworkSettings
	if patch.NetworkSettings != nil {
		diffs := mergeNetworkSettings(&cfg.NetworkSettings, patch.NetworkSettings)
		if len(diffs) > 0 {
			result.Settings = append(result.Settings, ApplySettingsDiff{
				Category: "network_settings",
				Changes:  diffs,
			})
			result.Summary.TotalChanges += len(diffs)
		}
	}

	// 4. Apply Database
	if patch.Database != nil {
		diffs := mergeDatabaseSettings(&cfg.Database, patch.Database)
		if len(diffs) > 0 {
			result.Settings = append(result.Settings, ApplySettingsDiff{
				Category: "database",
				Changes:  diffs,
			})
			result.Summary.TotalChanges += len(diffs)
		}
	}

	// 5. Apply DNS
	if patch.DNS != nil {
		diffs := mergeDNSSettings(&cfg.DNS, patch.DNS)
		if len(diffs) > 0 {
			result.Settings = append(result.Settings, ApplySettingsDiff{
				Category: "dns",
				Changes:  diffs,
			})
			result.Summary.TotalChanges += len(diffs)
		}
	}

	// 6. Apply Integrations
	if patch.Integrations != nil {
		diffs := mergeIntegrationsSettings(&cfg.Integrations, patch.Integrations)
		if len(diffs) > 0 {
			result.Settings = append(result.Settings, ApplySettingsDiff{
				Category: "integrations",
				Changes:  diffs,
			})
			result.Summary.TotalChanges += len(diffs)
		}
	}

	// Check post-patch integrity
	if err := validateMeshIntegrity(cfg); err != nil {
		return nil, nil, fmt.Errorf("mesh integrity validation failed: %w", err)
	}

	return result, affectedNodes, nil
}

func mergeNode(target *config.Node, patch config.NodePatch, now time.Time) ([]ApplyFieldDiff, error) {
	var diffs []ApplyFieldDiff

	checkField := func(name string, oldVal, newVal interface{}) {
		if !reflect.DeepEqual(oldVal, newVal) {
			diffs = append(diffs, ApplyFieldDiff{
				Field:    name,
				OldValue: oldVal,
				NewValue: newVal,
			})
		}
	}

	if patch.Host != nil && *patch.Host != target.Host {
		checkField("host", target.Host, *patch.Host)
		target.Host = *patch.Host
	}
	if patch.IsExternal != nil && *patch.IsExternal != target.IsExternal {
		checkField("is_external", target.IsExternal, *patch.IsExternal)
		target.IsExternal = *patch.IsExternal
	}
	if patch.Description != nil && *patch.Description != target.Description {
		checkField("description", target.Description, *patch.Description)
		target.Description = *patch.Description
	}
	if patch.Note != nil && *patch.Note != target.Note {
		checkField("note", target.Note, *patch.Note)
		target.Note = *patch.Note
	}
	if patch.IP != nil && *patch.IP != target.IP {
		checkField("ip", target.IP, *patch.IP)
		target.IP = *patch.IP
	}
	if patch.IP6 != nil && *patch.IP6 != target.IP6 {
		checkField("ip6", target.IP6, *patch.IP6)
		target.IP6 = *patch.IP6
	}
	if patch.ExternalIP != nil && *patch.ExternalIP != target.ExternalIP {
		checkField("external_ip", target.ExternalIP, *patch.ExternalIP)
		target.ExternalIP = *patch.ExternalIP
	}
	if patch.ExternalIP6 != nil && *patch.ExternalIP6 != target.ExternalIP6 {
		checkField("external_ip6", target.ExternalIP6, *patch.ExternalIP6)
		target.ExternalIP6 = *patch.ExternalIP6
	}
	if patch.Interface != nil && *patch.Interface != target.Interface {
		checkField("interface", target.Interface, *patch.Interface)
		target.Interface = *patch.Interface
	}
	if patch.ASN != nil && *patch.ASN != target.ASN {
		checkField("asn", target.ASN, *patch.ASN)
		target.ASN = *patch.ASN
	}
	if patch.Entrypoints != nil {
		checkField("entrypoints", target.Entrypoints, *patch.Entrypoints)
		target.Entrypoints = *patch.Entrypoints
	}
	if patch.Tags != nil {
		checkField("tags", target.Tags, *patch.Tags)
		target.Tags = *patch.Tags
	}
	if patch.Table != nil && *patch.Table != target.Table {
		checkField("table", target.Table, *patch.Table)
		target.Table = *patch.Table
	}
	if patch.ExternalTable != nil && *patch.ExternalTable != target.ExternalTable {
		checkField("external_table", target.ExternalTable, *patch.ExternalTable)
		target.ExternalTable = *patch.ExternalTable
	}
	if patch.InternetTable != nil && *patch.InternetTable != target.InternetTable {
		checkField("internet_table", target.InternetTable, *patch.InternetTable)
		target.InternetTable = *patch.InternetTable
	}
	if patch.Metric != nil {
		checkField("metric", target.Metric, patch.Metric)
		target.Metric = patch.Metric
	}
	if patch.StaticRoutes != nil {
		checkField("static_routes", target.StaticRoutes, *patch.StaticRoutes)
		target.StaticRoutes = *patch.StaticRoutes
	}
	if patch.Routes != nil {
		checkField("routes", target.Routes, *patch.Routes)
		target.Routes = *patch.Routes
	}
	if patch.ConfigHooks != nil {
		checkField("config_hooks", target.ConfigHooks, *patch.ConfigHooks)
		target.ConfigHooks = *patch.ConfigHooks
	}
	if patch.WgTemplate != nil && *patch.WgTemplate != target.WgTemplate {
		checkField("wg_template", target.WgTemplate, *patch.WgTemplate)
		target.WgTemplate = *patch.WgTemplate
	}
	if patch.BirdTemplate != nil && *patch.BirdTemplate != target.BirdTemplate {
		checkField("bird_template", target.BirdTemplate, *patch.BirdTemplate)
		target.BirdTemplate = *patch.BirdTemplate
	}
	if patch.NftTemplate != nil && *patch.NftTemplate != target.NftTemplate {
		checkField("nft_template", target.NftTemplate, *patch.NftTemplate)
		target.NftTemplate = *patch.NftTemplate
	}
	if patch.X != nil {
		checkField("x", target.X, patch.X)
		target.X = patch.X
	}
	if patch.Y != nil {
		checkField("y", target.Y, patch.Y)
		target.Y = patch.Y
	}
	if patch.Mode != nil && *patch.Mode != target.Mode {
		checkField("mode", target.Mode, *patch.Mode)
		target.Mode = *patch.Mode
	}
	if patch.EnableFA != nil && *patch.EnableFA != target.EnableFA {
		checkField("enable_fa", target.EnableFA, *patch.EnableFA)
		target.EnableFA = *patch.EnableFA
	}
	if patch.UpdateHosts != nil && *patch.UpdateHosts != target.UpdateHosts {
		checkField("update_hosts", target.UpdateHosts, *patch.UpdateHosts)
		target.UpdateHosts = *patch.UpdateHosts
	}
	if patch.CustomNetworkScript != nil && *patch.CustomNetworkScript != target.CustomNetworkScript {
		checkField("custom_network_script", target.CustomNetworkScript, *patch.CustomNetworkScript)
		target.CustomNetworkScript = *patch.CustomNetworkScript
	}

	if len(diffs) > 0 {
		target.ModifiedAt = now
	}
	return diffs, nil
}

func createNodeFromPatch(patch config.NodePatch, now time.Time) (*config.Node, []ApplyFieldDiff, error) {
	node := config.Node{
		Name:        patch.Name,
		ModifiedAt:  now,
		Table:       254,
		Interface:   "lo",
		Entrypoints: make([]config.Entrypoint, 0),
	}
	if patch.Host != nil {
		node.Host = *patch.Host
	}
	if patch.IsExternal != nil {
		node.IsExternal = *patch.IsExternal
	}
	if patch.Description != nil {
		node.Description = *patch.Description
	}
	if patch.Note != nil {
		node.Note = *patch.Note
	}
	if patch.IP != nil {
		node.IP = *patch.IP
	}
	if patch.IP6 != nil {
		node.IP6 = *patch.IP6
	}
	if patch.ExternalIP != nil {
		node.ExternalIP = *patch.ExternalIP
	}
	if patch.ExternalIP6 != nil {
		node.ExternalIP6 = *patch.ExternalIP6
	}
	if patch.Interface != nil {
		node.Interface = *patch.Interface
	}
	if patch.ASN != nil {
		node.ASN = *patch.ASN
	}
	if patch.Entrypoints != nil {
		node.Entrypoints = *patch.Entrypoints
	}
	if patch.Tags != nil {
		node.Tags = *patch.Tags
	}
	if patch.Table != nil {
		node.Table = *patch.Table
	}
	if patch.ExternalTable != nil {
		node.ExternalTable = *patch.ExternalTable
	}
	if patch.InternetTable != nil {
		node.InternetTable = *patch.InternetTable
	}
	if patch.Metric != nil {
		node.Metric = patch.Metric
	}
	if patch.StaticRoutes != nil {
		node.StaticRoutes = *patch.StaticRoutes
	}
	if patch.Routes != nil {
		node.Routes = *patch.Routes
	}
	if patch.ConfigHooks != nil {
		node.ConfigHooks = *patch.ConfigHooks
	}
	if patch.WgTemplate != nil {
		node.WgTemplate = *patch.WgTemplate
	}
	if patch.BirdTemplate != nil {
		node.BirdTemplate = *patch.BirdTemplate
	}
	if patch.NftTemplate != nil {
		node.NftTemplate = *patch.NftTemplate
	}
	if patch.X != nil {
		node.X = patch.X
	}
	if patch.Y != nil {
		node.Y = patch.Y
	}
	if patch.Mode != nil {
		node.Mode = *patch.Mode
	}
	if patch.EnableFA != nil {
		node.EnableFA = *patch.EnableFA
	}
	if patch.UpdateHosts != nil {
		node.UpdateHosts = *patch.UpdateHosts
	}
	if patch.CustomNetworkScript != nil {
		node.CustomNetworkScript = *patch.CustomNetworkScript
	}

	var diffs []ApplyFieldDiff
	diffs = append(diffs, ApplyFieldDiff{
		Field:    "node",
		OldValue: nil,
		NewValue: node.Name,
	})
	return &node, diffs, nil
}

func mergeLink(target *config.Link, patch config.LinkPatch, now time.Time) ([]ApplyFieldDiff, error) {
	var diffs []ApplyFieldDiff

	checkField := func(name string, oldVal, newVal interface{}) {
		if !reflect.DeepEqual(oldVal, newVal) {
			diffs = append(diffs, ApplyFieldDiff{
				Field:    name,
				OldValue: oldVal,
				NewValue: newVal,
			})
		}
	}

	isReversed := strings.EqualFold(target.From.Name, patch.To.Name) && strings.EqualFold(target.To.Name, patch.From.Name)
	targetFrom := &target.From
	targetTo := &target.To
	patchFrom := patch.From
	patchTo := patch.To
	if isReversed {
		patchFrom, patchTo = patchTo, patchFrom
	}

	if patch.Type != nil && *patch.Type != target.Type {
		checkField("type", target.Type, *patch.Type)
		target.Type = *patch.Type
	}
	if patch.Tags != nil {
		checkField("tags", target.Tags, *patch.Tags)
		target.Tags = *patch.Tags
	}
	if patch.AssignIPv4 != nil && *patch.AssignIPv4 != target.AssignIPv4 {
		checkField("assign_ipv4", target.AssignIPv4, *patch.AssignIPv4)
		target.AssignIPv4 = *patch.AssignIPv4
	}
	if patch.Psk != nil && *patch.Psk != target.Psk {
		checkField("psk", "***", "***")
		target.Psk = *patch.Psk
	}

	mergeLinkEnd(targetFrom, patchFrom, "from", &diffs)
	mergeLinkEnd(targetTo, patchTo, "to", &diffs)

	if len(diffs) > 0 {
		target.ModifiedAt = now
	}
	return diffs, nil
}

func mergeLinkEnd(target *config.LinkEnd, patch *config.LinkEndPatch, prefix string, diffs *[]ApplyFieldDiff) {
	if patch == nil {
		return
	}
	checkField := func(name string, oldVal, newVal interface{}) {
		if !reflect.DeepEqual(oldVal, newVal) {
			*diffs = append(*diffs, ApplyFieldDiff{
				Field:    prefix + "." + name,
				OldValue: oldVal,
				NewValue: newVal,
			})
		}
	}

	if patch.Type != nil && *patch.Type != target.Type {
		checkField("type", target.Type, *patch.Type)
		target.Type = *patch.Type
	}
	if patch.Interface != nil && *patch.Interface != target.Interface {
		checkField("interface", target.Interface, *patch.Interface)
		target.Interface = *patch.Interface
	}
	if patch.Address != nil && *patch.Address != target.Address {
		checkField("address", target.Address, *patch.Address)
		target.Address = *patch.Address
	}
	if patch.Address4 != nil && *patch.Address4 != target.Address4 {
		checkField("address4", target.Address4, *patch.Address4)
		target.Address4 = *patch.Address4
	}
	if patch.NeighborAddress != nil && *patch.NeighborAddress != target.NeighborAddress {
		checkField("neighbor_address", target.NeighborAddress, *patch.NeighborAddress)
		target.NeighborAddress = *patch.NeighborAddress
	}
	if patch.ListenPort != nil && *patch.ListenPort != target.ListenPort {
		checkField("listen_port", target.ListenPort, *patch.ListenPort)
		target.ListenPort = *patch.ListenPort
	}
	if patch.Endpoint != nil && *patch.Endpoint != target.Endpoint {
		checkField("endpoint", target.Endpoint, *patch.Endpoint)
		target.Endpoint = *patch.Endpoint
	}
	if patch.PersistentKeepalive != nil && *patch.PersistentKeepalive != target.PersistentKeepalive {
		checkField("persistent_keepalive", target.PersistentKeepalive, *patch.PersistentKeepalive)
		target.PersistentKeepalive = *patch.PersistentKeepalive
	}
	if patch.MTU != nil && *patch.MTU != target.MTU {
		checkField("mtu", target.MTU, *patch.MTU)
		target.MTU = *patch.MTU
	}
	if patch.UseIp != nil && *patch.UseIp != target.UseIp {
		checkField("use_ip", target.UseIp, *patch.UseIp)
		target.UseIp = *patch.UseIp
	}
	if patch.Policy != nil && *patch.Policy != target.Policy {
		checkField("policy", target.Policy, *patch.Policy)
		target.Policy = *patch.Policy
	}
	if patch.Cost != nil && *patch.Cost != target.Cost {
		checkField("cost", target.Cost, *patch.Cost)
		target.Cost = *patch.Cost
	}
	if patch.Fwmark != nil && *patch.Fwmark != target.Fwmark {
		checkField("fwmark", target.Fwmark, *patch.Fwmark)
		target.Fwmark = *patch.Fwmark
	}
	if patch.Preference != nil {
		checkField("preference", target.Preference, patch.Preference)
		target.Preference = patch.Preference
	}
	if patch.Mark != nil && *patch.Mark != target.Mark {
		checkField("mark", target.Mark, *patch.Mark)
		target.Mark = *patch.Mark
	}
	if patch.RoutingPolicy != nil && *patch.RoutingPolicy != target.RoutingPolicy {
		checkField("routing_policy", target.RoutingPolicy, *patch.RoutingPolicy)
		target.RoutingPolicy = *patch.RoutingPolicy
	}
	if patch.Note != nil && *patch.Note != target.Note {
		checkField("note", target.Note, *patch.Note)
		target.Note = *patch.Note
	}
}

func createLinkFromPatch(patch config.LinkPatch, now time.Time) (*config.Link, []ApplyFieldDiff, error) {
	link := config.Link{
		ModifiedAt: now,
		From: config.LinkEnd{
			Name: patch.From.Name,
		},
		To: config.LinkEnd{
			Name: patch.To.Name,
		},
	}
	if patch.Type != nil {
		link.Type = *patch.Type
	}
	if patch.Tags != nil {
		link.Tags = *patch.Tags
	}
	if patch.AssignIPv4 != nil {
		link.AssignIPv4 = *patch.AssignIPv4
	}
	if patch.Psk != nil {
		link.Psk = *patch.Psk
	}

	var dummyDiffs []ApplyFieldDiff
	mergeLinkEnd(&link.From, patch.From, "from", &dummyDiffs)
	mergeLinkEnd(&link.To, patch.To, "to", &dummyDiffs)

	diffs := []ApplyFieldDiff{
		{
			Field:    "link",
			OldValue: nil,
			NewValue: fmt.Sprintf("%s <-> %s", patch.From.Name, patch.To.Name),
		},
	}
	return &link, diffs, nil
}

func mergeNetworkSettings(target *config.NetworkSettings, patch *config.NetworkSettingsPatch) []ApplyFieldDiff {
	var diffs []ApplyFieldDiff
	checkField := func(name string, oldVal, newVal interface{}) {
		if !reflect.DeepEqual(oldVal, newVal) {
			diffs = append(diffs, ApplyFieldDiff{
				Field:    name,
				OldValue: oldVal,
				NewValue: newVal,
			})
		}
	}

	if patch.PublicASN != nil && *patch.PublicASN != target.PublicASN {
		checkField("public_asn", target.PublicASN, *patch.PublicASN)
		target.PublicASN = *patch.PublicASN
	}
	if patch.Prefixes != nil {
		checkField("prefixes", target.Prefixes, *patch.Prefixes)
		target.Prefixes = *patch.Prefixes
	}
	if patch.DisallowedDN42Networks != nil {
		checkField("disallowed_dn42_networks", target.DisallowedDN42Networks, *patch.DisallowedDN42Networks)
		target.DisallowedDN42Networks = *patch.DisallowedDN42Networks
	}
	if patch.DisallowedDN42CIDRs != nil {
		checkField("disallowed_dn42_cidrs", target.DisallowedDN42CIDRs, *patch.DisallowedDN42CIDRs)
		target.DisallowedDN42CIDRs = *patch.DisallowedDN42CIDRs
	}
	if patch.FlapAlertedAddress != nil && *patch.FlapAlertedAddress != target.FlapAlertedAddress {
		checkField("flapalerted_address", target.FlapAlertedAddress, *patch.FlapAlertedAddress)
		target.FlapAlertedAddress = *patch.FlapAlertedAddress
	}
	if patch.FlapAlertedPort != nil && *patch.FlapAlertedPort != target.FlapAlertedPort {
		checkField("flapalerted_port", target.FlapAlertedPort, *patch.FlapAlertedPort)
		target.FlapAlertedPort = *patch.FlapAlertedPort
	}
	if patch.FlapAlertedASN != nil && *patch.FlapAlertedASN != target.FlapAlertedASN {
		checkField("flapalerted_asn", target.FlapAlertedASN, *patch.FlapAlertedASN)
		target.FlapAlertedASN = *patch.FlapAlertedASN
	}
	if patch.FlapAlertedURL != nil && *patch.FlapAlertedURL != target.FlapAlertedURL {
		checkField("flapalerted_url", target.FlapAlertedURL, *patch.FlapAlertedURL)
		target.FlapAlertedURL = *patch.FlapAlertedURL
	}
	return diffs
}

func mergeDatabaseSettings(target *config.DatabaseSettings, patch *config.DatabaseSettingsPatch) []ApplyFieldDiff {
	var diffs []ApplyFieldDiff
	if patch.RetentionDays != nil && *patch.RetentionDays != target.RetentionDays {
		diffs = append(diffs, ApplyFieldDiff{
			Field:    "retention_days",
			OldValue: target.RetentionDays,
			NewValue: *patch.RetentionDays,
		})
		target.RetentionDays = *patch.RetentionDays
	}
	if patch.RawRetentionDays != nil && *patch.RawRetentionDays != target.RawRetentionDays {
		diffs = append(diffs, ApplyFieldDiff{
			Field:    "raw_retention_days",
			OldValue: target.RawRetentionDays,
			NewValue: *patch.RawRetentionDays,
		})
		target.RawRetentionDays = *patch.RawRetentionDays
	}
	return diffs
}

func mergeDNSSettings(target *config.DNSConfig, patch *config.DNSPatch) []ApplyFieldDiff {
	var diffs []ApplyFieldDiff
	if patch.ZoneID != nil && *patch.ZoneID != target.ZoneID {
		diffs = append(diffs, ApplyFieldDiff{
			Field:    "zone_id",
			OldValue: target.ZoneID,
			NewValue: *patch.ZoneID,
		})
		target.ZoneID = *patch.ZoneID
	}
	if patch.APIToken != nil && *patch.APIToken != target.APIToken {
		diffs = append(diffs, ApplyFieldDiff{
			Field:    "api_token",
			OldValue: "***",
			NewValue: "***",
		})
		target.APIToken = *patch.APIToken
	}
	if patch.BaseDomain != nil && *patch.BaseDomain != target.BaseDomain {
		diffs = append(diffs, ApplyFieldDiff{
			Field:    "base_domain",
			OldValue: target.BaseDomain,
			NewValue: *patch.BaseDomain,
		})
		target.BaseDomain = *patch.BaseDomain
	}
	if patch.PublishIPv6OwnName != nil && *patch.PublishIPv6OwnName != target.PublishIPv6OwnName {
		diffs = append(diffs, ApplyFieldDiff{
			Field:    "publish_ipv6_own_name",
			OldValue: target.PublishIPv6OwnName,
			NewValue: *patch.PublishIPv6OwnName,
		})
		target.PublishIPv6OwnName = *patch.PublishIPv6OwnName
	}
	return diffs
}

func mergeIntegrationsSettings(target *config.IntegrationsSettings, patch *config.IntegrationsSettingsPatch) []ApplyFieldDiff {
	var diffs []ApplyFieldDiff
	if patch.PrometheusEnabled != nil && *patch.PrometheusEnabled != target.PrometheusEnabled {
		diffs = append(diffs, ApplyFieldDiff{
			Field:    "prometheus_enabled",
			OldValue: target.PrometheusEnabled,
			NewValue: *patch.PrometheusEnabled,
		})
		target.PrometheusEnabled = *patch.PrometheusEnabled
	}
	if patch.PrometheusAuthToken != nil && *patch.PrometheusAuthToken != target.PrometheusAuthToken {
		diffs = append(diffs, ApplyFieldDiff{
			Field:    "prometheus_auth_token",
			OldValue: "***",
			NewValue: "***",
		})
		target.PrometheusAuthToken = *patch.PrometheusAuthToken
	}
	if patch.TelegramEnabled != nil && *patch.TelegramEnabled != target.TelegramEnabled {
		diffs = append(diffs, ApplyFieldDiff{
			Field:    "telegram_enabled",
			OldValue: target.TelegramEnabled,
			NewValue: *patch.TelegramEnabled,
		})
		target.TelegramEnabled = *patch.TelegramEnabled
	}
	if patch.TelegramBotToken != nil && *patch.TelegramBotToken != target.TelegramBotToken {
		diffs = append(diffs, ApplyFieldDiff{
			Field:    "telegram_bot_token",
			OldValue: "***",
			NewValue: "***",
		})
		target.TelegramBotToken = *patch.TelegramBotToken
	}
	if patch.TelegramAdminID != nil && *patch.TelegramAdminID != target.TelegramAdminID {
		diffs = append(diffs, ApplyFieldDiff{
			Field:    "telegram_admin_id",
			OldValue: target.TelegramAdminID,
			NewValue: *patch.TelegramAdminID,
		})
		target.TelegramAdminID = *patch.TelegramAdminID
	}
	if patch.TelegramSecretToken != nil && *patch.TelegramSecretToken != target.TelegramSecretToken {
		diffs = append(diffs, ApplyFieldDiff{
			Field:    "telegram_secret_token",
			OldValue: "***",
			NewValue: "***",
		})
		target.TelegramSecretToken = *patch.TelegramSecretToken
	}
	if patch.ServerURL != nil && *patch.ServerURL != target.ServerURL {
		diffs = append(diffs, ApplyFieldDiff{
			Field:    "server_url",
			OldValue: target.ServerURL,
			NewValue: *patch.ServerURL,
		})
		target.ServerURL = *patch.ServerURL
	}
	return diffs
}

func (m *Manager) recomputeLinksAndEndpoints(cfg *config.Config, now time.Time) {
	for i := range cfg.Links {
		l := &cfg.Links[i]
		fromN := findNodeInSlice(cfg.Nodes, l.From.Name)
		toN := findNodeInSlice(cfg.Nodes, l.To.Name)
		if fromN == nil || toN == nil {
			continue
		}

		if l.From.Interface == "" {
			l.From.Interface = compiler.GetInterfaceName(toN.Name, toN.IsExternal)
		}
		if l.To.Interface == "" {
			l.To.Interface = compiler.GetInterfaceName(fromN.Name, fromN.IsExternal)
		}

		// Recompute IPv6 link local if empty
		if l.From.Address == "" && fromN.IP != "" {
			if addr, err := compiler.DeriveIPv6LinkLocal(fromN.IP); err == nil {
				l.From.Address = addr
			}
		}
		if l.To.Address == "" && toN.IP != "" {
			if addr, err := compiler.DeriveIPv6LinkLocal(toN.IP); err == nil {
				l.To.Address = addr
			}
		}

		// Endpoint resolution
		fromEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(fromN, toN, nil, l.To.ListenPort)
		toEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(toN, fromN, nil, l.From.ListenPort)
		if l.From.Endpoint == "" || fromEP != "" {
			l.From.Endpoint = fromEP
		}
		if l.To.Endpoint == "" || toEP != "" {
			l.To.Endpoint = toEP
		}
		if fromEP != "" && !fromN.IsExternal && l.From.PersistentKeepalive == 0 {
			l.From.PersistentKeepalive = 25
		}
	}
}

func validateMeshIntegrity(cfg *config.Config) error {
	seenNames := make(map[string]bool)
	seenIP6 := make(map[string]string)

	for _, n := range cfg.Nodes {
		lowerName := strings.ToLower(n.Name)
		if seenNames[lowerName] {
			return fmt.Errorf("duplicate node name '%s'", n.Name)
		}
		seenNames[lowerName] = true

		if n.IP6 != "" && !n.IsExternal {
			cleanIP6 := strings.ToLower(strings.TrimSpace(n.IP6))
			if idx := strings.Index(cleanIP6, "/"); idx != -1 {
				cleanIP6 = cleanIP6[:idx]
			}
			if existingNode, exists := seenIP6[cleanIP6]; exists {
				return fmt.Errorf("duplicate IPv6 '%s' shared between '%s' and '%s'", cleanIP6, existingNode, n.Name)
			}
			seenIP6[cleanIP6] = n.Name
		}
	}

	for _, l := range cfg.Links {
		if !seenNames[strings.ToLower(l.From.Name)] {
			return fmt.Errorf("link endpoint node '%s' does not exist in nodes", l.From.Name)
		}
		if !seenNames[strings.ToLower(l.To.Name)] {
			return fmt.Errorf("link endpoint node '%s' does not exist in nodes", l.To.Name)
		}
	}

	return nil
}

func cloneConfig(c *config.Config) (*config.Config, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var cloned config.Config
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, err
	}
	return &cloned, nil
}

func findNodeInSlice(nodes []config.Node, name string) *config.Node {
	for i := range nodes {
		if strings.EqualFold(nodes[i].Name, name) {
			return &nodes[i]
		}
	}
	return nil
}

func filterDiffConfig(c *config.Config) map[string]interface{} {
	m := make(map[string]interface{})
	if c.NetworkSettings.PublicASN > 0 || len(c.NetworkSettings.Prefixes) > 0 {
		m["network_settings"] = c.NetworkSettings
	}
	if len(c.Nodes) > 0 {
		nodes := make([]map[string]interface{}, len(c.Nodes))
		for i, n := range c.Nodes {
			nodes[i] = map[string]interface{}{
				"name":          n.Name,
				"host":          n.Host,
				"ip":            n.IP,
				"ip6":           n.IP6,
				"asn":           n.ASN,
				"static_routes": n.StaticRoutes,
				"tags":          n.Tags,
			}
		}
		m["nodes"] = nodes
	}
	if len(c.Links) > 0 {
		links := make([]map[string]interface{}, len(c.Links))
		for i, l := range c.Links {
			links[i] = map[string]interface{}{
				"from": l.From.Name,
				"to":   l.To.Name,
				"cost": l.From.Cost,
			}
		}
		m["links"] = links
	}
	return m
}

func generateSimpleDiff(oldStr, newStr string) string {
	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")

	var diffLines []string
	diffLines = append(diffLines, "--- current config")
	diffLines = append(diffLines, "+++ proposed config")

	i, j := 0, 0
	for i < len(oldLines) && j < len(newLines) {
		if oldLines[i] == newLines[j] {
			diffLines = append(diffLines, "  "+oldLines[i])
			i++
			j++
		} else {
			diffLines = append(diffLines, "- "+oldLines[i])
			diffLines = append(diffLines, "+ "+newLines[j])
			i++
			j++
		}
	}
	for ; i < len(oldLines); i++ {
		diffLines = append(diffLines, "- "+oldLines[i])
	}
	for ; j < len(newLines); j++ {
		diffLines = append(diffLines, "+ "+newLines[j])
	}

	return strings.Join(diffLines, "\n")
}

// ConfigChange CRUD on Manager

// ListConfigChanges returns all saved config change snippets
func (m *Manager) ListConfigChanges() []config.ConfigChange {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.store.Get()
	if cfg == nil || cfg.ConfigChanges == nil {
		return []config.ConfigChange{}
	}
	out := make([]config.ConfigChange, len(cfg.ConfigChanges))
	copy(out, cfg.ConfigChanges)
	return out
}

// GetConfigChange returns a saved config change snippet by ID
func (m *Manager) GetConfigChange(id string) (*config.ConfigChange, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.store.Get()
	if cfg == nil {
		return nil, errors.New("config not loaded")
	}
	for _, cc := range cfg.ConfigChanges {
		if cc.ID == id {
			return &cc, nil
		}
	}
	return nil, fmt.Errorf("config change snippet '%s' not found", id)
}

// SaveConfigChange validates and persists a saved config change snippet
func (m *Manager) SaveConfigChange(cc config.ConfigChange) (*config.ConfigChange, error) {
	// Strict verification before saving!
	if _, _, err := config.ParseAndValidateConfigChange([]byte(cc.Content)); err != nil {
		return nil, fmt.Errorf("cannot save invalid config change snippet: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	if cfg == nil {
		return nil, errors.New("config not loaded")
	}

	now := time.Now().UTC()
	if cc.ID == "" {
		cc.ID = uuid.New().String()
		cc.CreatedAt = now
		cc.ModifiedAt = now
	} else {
		cc.ModifiedAt = now
	}

	if cc.Name == "" {
		cc.Name = "Snippet " + now.Format("2006-01-02 15:04:05")
	}

	found := false
	for i := range cfg.ConfigChanges {
		if cfg.ConfigChanges[i].ID == cc.ID {
			cfg.ConfigChanges[i] = cc
			found = true
			break
		}
	}
	if !found {
		cfg.ConfigChanges = append(cfg.ConfigChanges, cc)
	}

	if err := m.store.Save(cfg); err != nil {
		return nil, fmt.Errorf("failed to persist config changes list: %w", err)
	}

	return &cc, nil
}

// DeleteConfigChange removes a saved config change snippet
func (m *Manager) DeleteConfigChange(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	if cfg == nil {
		return errors.New("config not loaded")
	}

	idx := -1
	for i, cc := range cfg.ConfigChanges {
		if cc.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("config change '%s' not found", id)
	}

	cfg.ConfigChanges = append(cfg.ConfigChanges[:idx], cfg.ConfigChanges[idx+1:]...)
	return m.store.Save(cfg)
}

// ApplySavedConfigChange applies a previously saved config change snippet
func (m *Manager) ApplySavedConfigChange(id string) (*ApplyResult, error) {
	cc, err := m.GetConfigChange(id)
	if err != nil {
		return nil, err
	}
	return m.ApplyConfigChange(cc.Content)
}
