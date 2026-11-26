package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"easy42/internal/apps"
	"easy42/internal/compiler"
	"easy42/internal/config"
	"easy42/internal/crypto"
	"easy42/internal/lookingglass"
	"easy42/internal/roa"

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

// ApplyAppDiff represents modifications to an app
type ApplyAppDiff struct {
	Name    string           `json:"name"`
	Action  string           `json:"action"` // "create" or "update"
	Changes []ApplyFieldDiff `json:"changes"`
}

// ApplySettingsDiff represents modifications to global settings
type ApplySettingsDiff struct {
	Category string           `json:"category"`
	Changes  []ApplyFieldDiff `json:"changes"`
}

// ApplyResourceDiff represents modifications to a generic identifiable resource
type ApplyResourceDiff struct {
	ID      string           `json:"id"`
	Name    string           `json:"name,omitempty"`
	Action  string           `json:"action"` // "create" or "update"
	Changes []ApplyFieldDiff `json:"changes"`
}

// ApplySummary summarizes planned changes
type ApplySummary struct {
	AddedNodes              int `json:"added_nodes"`
	ModifiedNodes           int `json:"modified_nodes"`
	AddedLinks              int `json:"added_links"`
	ModifiedLinks           int `json:"modified_links"`
	AddedApps               int `json:"added_apps,omitempty"`
	ModifiedApps            int `json:"modified_apps,omitempty"`
	AddedNetworkPolicies    int `json:"added_network_policies,omitempty"`
	ModifiedNetworkPolicies int `json:"modified_network_policies,omitempty"`
	AddedBlocks             int `json:"added_blocks,omitempty"`
	ModifiedBlocks          int `json:"modified_blocks,omitempty"`
	AddedTemplates          int `json:"added_templates,omitempty"`
	ModifiedTemplates       int `json:"modified_templates,omitempty"`
	ModifiedServerTasks     int `json:"modified_server_tasks,omitempty"`
	AddedCronTasks          int `json:"added_cron_tasks,omitempty"`
	ModifiedCronTasks       int `json:"modified_cron_tasks,omitempty"`
	AddedSSHKeyRules        int `json:"added_ssh_key_rules,omitempty"`
	ModifiedSSHKeyRules     int `json:"modified_ssh_key_rules,omitempty"`
	TotalChanges            int `json:"total_changes"`
}

// ApplyPreviewResult contains full validation and diff details
type ApplyPreviewResult struct {
	Valid           bool                `json:"valid"`
	Format          string              `json:"format"`
	Summary         ApplySummary        `json:"summary"`
	Nodes           []ApplyNodeDiff     `json:"nodes"`
	Links           []ApplyLinkDiff     `json:"links"`
	Apps            []ApplyAppDiff      `json:"apps"`
	NetworkPolicies []ApplyResourceDiff `json:"network_policies,omitempty"`
	Blocks          []ApplyResourceDiff `json:"blocks,omitempty"`
	Templates       []ApplyResourceDiff `json:"templates,omitempty"`
	ServerTasks     []ApplyResourceDiff `json:"server_tasks,omitempty"`
	CronTasks       []ApplyResourceDiff `json:"cron_tasks,omitempty"`
	SSHKeyRules     []ApplyResourceDiff `json:"ssh_key_rules,omitempty"`
	Settings        []ApplySettingsDiff `json:"settings"`
	UnifiedDiff     string              `json:"unified_diff"`
	Errors          []string            `json:"errors,omitempty"`
	NormalizedJSON  string              `json:"normalized_json,omitempty"`
}

// ApplyResult represents the outcome of an applied config change
type ApplyResult struct {
	Success                 bool               `json:"success"`
	AppliedAt               time.Time          `json:"applied_at"`
	Summary                 ApplySummary       `json:"summary"`
	AffectedNodes           []string           `json:"affected_nodes"`
	AffectedLinks           []string           `json:"affected_links"`
	AffectedApps            []string           `json:"affected_apps,omitempty"`
	AffectedNetworkPolicies []string           `json:"affected_network_policies,omitempty"`
	AffectedBlocks          []string           `json:"affected_blocks,omitempty"`
	AffectedTemplates       []string           `json:"affected_templates,omitempty"`
	AffectedServerTasks     []string           `json:"affected_server_tasks,omitempty"`
	AffectedCronTasks       []string           `json:"affected_cron_tasks,omitempty"`
	AffectedSSHKeyRules     []string           `json:"affected_ssh_key_rules,omitempty"`
	BackupFile              string             `json:"backup_file,omitempty"`
	Plan                    ApplyPreviewResult `json:"plan"`
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
		if l.Type != config.LinkTypeManual && l.Type != config.LinkTypeVirtual && !l.IsVirtual() && l.From.PublicKey == "" && l.To.PublicKey == "" {
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
	_ = m.PruneConfigBackups(MaxConfigBackups)

	// Save to store
	if err := m.store.Save(cloned); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	// Re-trigger host and WireGuard obfuscator sync in background
	go m.SyncHostsToAgents()
	go m.SyncObfuscatorsToAgents()

	affectedLinks := make([]string, 0)
	if preview.Links != nil {
		for _, l := range preview.Links {
			affectedLinks = append(affectedLinks, fmt.Sprintf("%s <-> %s", l.From, l.To))
		}
	}

	affectedApps := make([]string, 0)
	if preview.Apps != nil {
		for _, a := range preview.Apps {
			affectedApps = append(affectedApps, a.Name)
		}
	}

	collectAffected := func(diffs []ApplyResourceDiff) []string {
		if len(diffs) == 0 {
			return nil
		}
		out := make([]string, 0, len(diffs))
		for _, d := range diffs {
			if d.Name != "" {
				out = append(out, d.Name)
			} else {
				out = append(out, d.ID)
			}
		}
		return out
	}

	affectedPolicies := collectAffected(preview.NetworkPolicies)
	affectedBlocks := collectAffected(preview.Blocks)
	affectedTemplates := collectAffected(preview.Templates)
	affectedServerTasks := collectAffected(preview.ServerTasks)
	affectedCronTasks := collectAffected(preview.CronTasks)
	affectedSSHKeyRules := collectAffected(preview.SSHKeyRules)

	if len(affectedCronTasks) > 0 {
		go m.SyncCronTasksToAgents()
	}

	return &ApplyResult{
		Success:                 true,
		AppliedAt:               now,
		Summary:                 preview.Summary,
		AffectedNodes:           affectedNodes,
		AffectedLinks:           affectedLinks,
		AffectedApps:            affectedApps,
		AffectedNetworkPolicies: affectedPolicies,
		AffectedBlocks:          affectedBlocks,
		AffectedTemplates:       affectedTemplates,
		AffectedServerTasks:     affectedServerTasks,
		AffectedCronTasks:       affectedCronTasks,
		AffectedSSHKeyRules:     affectedSSHKeyRules,
		BackupFile:              backupFile,
		Plan:                    *preview,
	}, nil
}

func (m *Manager) simulateApply(cfg *config.Config, patch *config.ConfigPatch) (*ApplyPreviewResult, []string, error) {
	now := time.Now().UTC()
	result := &ApplyPreviewResult{
		Nodes:    make([]ApplyNodeDiff, 0),
		Links:    make([]ApplyLinkDiff, 0),
		Apps:     make([]ApplyAppDiff, 0),
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

	// 3. Apply Apps
	if patch.Apps != nil {
		for _, ap := range *patch.Apps {
			existingIdx := -1
			for i, a := range cfg.Apps {
				if (ap.ID != nil && *ap.ID != "" && strings.EqualFold(a.ID, *ap.ID)) || strings.EqualFold(a.Name, ap.Name) {
					existingIdx = i
					break
				}
			}

			if existingIdx >= 0 {
				// Update existing app
				diffs, err := mergeApp(&cfg.Apps[existingIdx], ap, now)
				if err != nil {
					return nil, nil, err
				}
				if len(diffs) > 0 {
					result.Apps = append(result.Apps, ApplyAppDiff{
						Name:    cfg.Apps[existingIdx].Name,
						Action:  "update",
						Changes: diffs,
					})
					result.Summary.ModifiedApps++
					result.Summary.TotalChanges += len(diffs)
				}
			} else {
				// Create new app
				newApp, diffs, err := createAppFromPatch(ap, now)
				if err != nil {
					return nil, nil, err
				}
				cfg.Apps = append(cfg.Apps, *newApp)
				result.Apps = append(result.Apps, ApplyAppDiff{
					Name:    newApp.Name,
					Action:  "create",
					Changes: diffs,
				})
				result.Summary.AddedApps++
				result.Summary.TotalChanges += len(diffs)
			}
		}
	}

	// 4. Apply NetworkSettings
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

	// 7. Apply NetworkPolicies
	if patch.NetworkPolicies != nil {
		for _, pp := range *patch.NetworkPolicies {
			existingIdx := -1
			patchID := ""
			if pp.ID != nil {
				patchID = strings.TrimSpace(*pp.ID)
			}
			patchName := ""
			if pp.Name != nil {
				patchName = strings.TrimSpace(*pp.Name)
			}
			for i, existing := range cfg.NetworkPolicies {
				if (patchID != "" && existing.ID == patchID) || (patchID == "" && patchName != "" && strings.EqualFold(existing.Name, patchName)) {
					existingIdx = i
					break
				}
			}
			if existingIdx >= 0 {
				if cfg.NetworkPolicies[existingIdx].IsInternal {
					return nil, nil, fmt.Errorf("cannot modify built-in policy %q", cfg.NetworkPolicies[existingIdx].ID)
				}
				diffs, err := mergeNetworkPolicy(&cfg.NetworkPolicies[existingIdx], pp)
				if err != nil {
					return nil, nil, err
				}
				if len(diffs) > 0 {
					result.NetworkPolicies = append(result.NetworkPolicies, ApplyResourceDiff{
						ID:      cfg.NetworkPolicies[existingIdx].ID,
						Name:    cfg.NetworkPolicies[existingIdx].Name,
						Action:  "update",
						Changes: diffs,
					})
					result.Summary.ModifiedNetworkPolicies++
					result.Summary.TotalChanges += len(diffs)
				}
			} else {
				newPol, diffs, err := createNetworkPolicyFromPatch(pp)
				if err != nil {
					return nil, nil, err
				}
				cfg.NetworkPolicies = append(cfg.NetworkPolicies, *newPol)
				result.NetworkPolicies = append(result.NetworkPolicies, ApplyResourceDiff{
					ID:      newPol.ID,
					Name:    newPol.Name,
					Action:  "create",
					Changes: diffs,
				})
				result.Summary.AddedNetworkPolicies++
				result.Summary.TotalChanges += len(diffs)
			}
		}
	}

	// 8. Apply Blocks
	if patch.Blocks != nil {
		for _, bp := range *patch.Blocks {
			existingIdx := -1
			patchID := ""
			if bp.ID != nil {
				patchID = strings.TrimSpace(*bp.ID)
			}
			patchName := ""
			if bp.Name != nil {
				patchName = strings.TrimSpace(*bp.Name)
			}
			for i, existing := range cfg.Blocks {
				if (patchID != "" && existing.ID == patchID) || (patchID == "" && patchName != "" && strings.EqualFold(existing.Name, patchName)) {
					existingIdx = i
					break
				}
			}
			if existingIdx >= 0 {
				diffs := mergeBlock(&cfg.Blocks[existingIdx], bp)
				if len(diffs) > 0 {
					result.Blocks = append(result.Blocks, ApplyResourceDiff{
						ID:      cfg.Blocks[existingIdx].ID,
						Name:    cfg.Blocks[existingIdx].Name,
						Action:  "update",
						Changes: diffs,
					})
					result.Summary.ModifiedBlocks++
					result.Summary.TotalChanges += len(diffs)
				}
			} else {
				newBlock, diffs, err := createBlockFromPatch(bp)
				if err != nil {
					return nil, nil, err
				}
				cfg.Blocks = append(cfg.Blocks, *newBlock)
				result.Blocks = append(result.Blocks, ApplyResourceDiff{
					ID:      newBlock.ID,
					Name:    newBlock.Name,
					Action:  "create",
					Changes: diffs,
				})
				result.Summary.AddedBlocks++
				result.Summary.TotalChanges += len(diffs)
			}
		}
	}

	// 9. Apply Templates
	if patch.Templates != nil {
		for _, tp := range *patch.Templates {
			existingIdx := -1
			patchID := ""
			if tp.ID != nil {
				patchID = strings.TrimSpace(*tp.ID)
			}
			patchName := ""
			if tp.Name != nil {
				patchName = strings.TrimSpace(*tp.Name)
			}
			for i, existing := range cfg.Templates {
				if (patchID != "" && existing.ID == patchID) || (patchID == "" && patchName != "" && strings.EqualFold(existing.Name, patchName)) {
					existingIdx = i
					break
				}
			}
			if existingIdx >= 0 {
				if cfg.Templates[existingIdx].IsBuiltin {
					return nil, nil, fmt.Errorf("cannot modify system default template %q", cfg.Templates[existingIdx].ID)
				}
				diffs, err := mergeTemplate(&cfg.Templates[existingIdx], tp, now)
				if err != nil {
					return nil, nil, err
				}
				if len(diffs) > 0 {
					result.Templates = append(result.Templates, ApplyResourceDiff{
						ID:      cfg.Templates[existingIdx].ID,
						Name:    cfg.Templates[existingIdx].Name,
						Action:  "update",
						Changes: diffs,
					})
					result.Summary.ModifiedTemplates++
					result.Summary.TotalChanges += len(diffs)
				}
			} else {
				newTmpl, diffs, err := createTemplateFromPatch(tp, now)
				if err != nil {
					return nil, nil, err
				}
				cfg.Templates = append(cfg.Templates, *newTmpl)
				result.Templates = append(result.Templates, ApplyResourceDiff{
					ID:      newTmpl.ID,
					Name:    newTmpl.Name,
					Action:  "create",
					Changes: diffs,
				})
				result.Summary.AddedTemplates++
				result.Summary.TotalChanges += len(diffs)
			}
		}
	}

	// 10. Apply ServerTasks
	if patch.ServerTasks != nil {
		cfg.ServerTasks = config.NormalizeServerTasks(cfg.ServerTasks)
		for _, stp := range *patch.ServerTasks {
			existingIdx := -1
			for i, existing := range cfg.ServerTasks {
				if existing.ID == stp.ID {
					existingIdx = i
					break
				}
			}
			if existingIdx == -1 {
				return nil, nil, fmt.Errorf("server task %q not found", stp.ID)
			}
			diffs := mergeServerTask(&cfg.ServerTasks[existingIdx], stp)
			if len(diffs) > 0 {
				result.ServerTasks = append(result.ServerTasks, ApplyResourceDiff{
					ID:      cfg.ServerTasks[existingIdx].ID,
					Action:  "update",
					Changes: diffs,
				})
				result.Summary.ModifiedServerTasks++
				result.Summary.TotalChanges += len(diffs)
			}
		}
	}

	// 11. Apply CronTasks
	if patch.CronTasks != nil {
		for _, ctp := range *patch.CronTasks {
			existingIdx := -1
			patchID := ""
			if ctp.ID != nil {
				patchID = strings.TrimSpace(*ctp.ID)
			}
			patchName := ""
			if ctp.Name != nil {
				patchName = strings.TrimSpace(*ctp.Name)
			}
			for i, existing := range cfg.CronTasks {
				if (patchID != "" && existing.ID == patchID) || (patchID == "" && patchName != "" && strings.EqualFold(existing.Name, patchName)) {
					existingIdx = i
					break
				}
			}
			if existingIdx >= 0 {
				diffs, err := mergeCronTask(&cfg.CronTasks[existingIdx], ctp, cfg.Nodes, now)
				if err != nil {
					return nil, nil, err
				}
				if len(diffs) > 0 {
					result.CronTasks = append(result.CronTasks, ApplyResourceDiff{
						ID:      cfg.CronTasks[existingIdx].ID,
						Name:    cfg.CronTasks[existingIdx].Name,
						Action:  "update",
						Changes: diffs,
					})
					result.Summary.ModifiedCronTasks++
					result.Summary.TotalChanges += len(diffs)
				}
			} else {
				newTask, diffs, err := createCronTaskFromPatch(ctp, cfg.Nodes, now)
				if err != nil {
					return nil, nil, err
				}
				cfg.CronTasks = append(cfg.CronTasks, *newTask)
				result.CronTasks = append(result.CronTasks, ApplyResourceDiff{
					ID:      newTask.ID,
					Name:    newTask.Name,
					Action:  "create",
					Changes: diffs,
				})
				result.Summary.AddedCronTasks++
				result.Summary.TotalChanges += len(diffs)
			}
		}
	}

	// 12. Apply SSHKeyRules
	if patch.SSHKeyRules != nil {
		for _, rp := range *patch.SSHKeyRules {
			existingIdx := -1
			patchID := ""
			if rp.ID != nil {
				patchID = strings.TrimSpace(*rp.ID)
			}
			if patchID != "" {
				for i, existing := range cfg.SSHKeyRules {
					if existing.ID == patchID {
						existingIdx = i
						break
					}
				}
			}
			if existingIdx >= 0 {
				diffs, err := mergeSSHKeyRule(&cfg.SSHKeyRules[existingIdx], rp, now)
				if err != nil {
					return nil, nil, err
				}
				if len(diffs) > 0 {
					result.SSHKeyRules = append(result.SSHKeyRules, ApplyResourceDiff{
						ID:      cfg.SSHKeyRules[existingIdx].ID,
						Action:  "update",
						Changes: diffs,
					})
					result.Summary.ModifiedSSHKeyRules++
					result.Summary.TotalChanges += len(diffs)
				}
			} else {
				newRule, diffs, err := createSSHKeyRuleFromPatch(rp, now)
				if err != nil {
					return nil, nil, err
				}
				cfg.SSHKeyRules = append(cfg.SSHKeyRules, *newRule)
				result.SSHKeyRules = append(result.SSHKeyRules, ApplyResourceDiff{
					ID:      newRule.ID,
					Action:  "create",
					Changes: diffs,
				})
				result.Summary.AddedSSHKeyRules++
				result.Summary.TotalChanges += len(diffs)
			}
		}
	}

	// Check post-patch integrity
	if err := m.normalizeObfuscatorsInConfig(cfg); err != nil {
		return nil, nil, fmt.Errorf("obfuscator validation failed: %w", err)
	}
	if err := validateMeshIntegrity(cfg); err != nil {
		return nil, nil, fmt.Errorf("mesh integrity validation failed: %w", err)
	}

	return result, affectedNodes, nil
}

func routeEqual(a, b string) bool {
	aTrim := strings.TrimSpace(a)
	bTrim := strings.TrimSpace(b)
	if strings.EqualFold(aTrim, bTrim) {
		return true
	}
	hasAtA := strings.HasPrefix(aTrim, "@")
	hasAtB := strings.HasPrefix(bTrim, "@")
	if hasAtA != hasAtB {
		return false
	}
	cleanA := strings.TrimPrefix(aTrim, "@")
	cleanB := strings.TrimPrefix(bTrim, "@")
	_, ipNetA, errA := net.ParseCIDR(cleanA)
	_, ipNetB, errB := net.ParseCIDR(cleanB)
	if errA == nil && errB == nil {
		return ipNetA.String() == ipNetB.String()
	}
	ipA := net.ParseIP(cleanA)
	ipB := net.ParseIP(cleanB)
	if ipA != nil && ipB != nil {
		return ipA.Equal(ipB)
	}
	return false
}

func tagEqual(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func applyStringSliceDelta(base []string, toDel, toAdd *[]string, equalFn func(string, string) bool) []string {
	res := make([]string, 0, len(base))
	res = append(res, base...)

	// Apply deletions first
	if toDel != nil {
		for _, itemToDel := range *toDel {
			trimmedDel := strings.TrimSpace(itemToDel)
			if trimmedDel == "" {
				continue
			}
			var filtered []string
			for _, existing := range res {
				if !equalFn(existing, trimmedDel) {
					filtered = append(filtered, existing)
				}
			}
			res = filtered
		}
	}

	// Apply additions
	if toAdd != nil {
		for _, itemToAdd := range *toAdd {
			trimmedAdd := strings.TrimSpace(itemToAdd)
			if trimmedAdd == "" {
				continue
			}
			alreadyExists := false
			for _, existing := range res {
				if equalFn(existing, trimmedAdd) {
					alreadyExists = true
					break
				}
			}
			if !alreadyExists {
				res = append(res, trimmedAdd)
			}
		}
	}

	return res
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
	if patch.IsClient != nil && *patch.IsClient != target.IsClient {
		checkField("is_client", target.IsClient, *patch.IsClient)
		target.IsClient = *patch.IsClient
	}
	if patch.ClientParent != nil && *patch.ClientParent != target.ClientParent {
		checkField("client_parent", target.ClientParent, *patch.ClientParent)
		target.ClientParent = *patch.ClientParent
	}
	if patch.ClientAllowedIPs != nil && !slices.Equal(target.ClientAllowedIPs, *patch.ClientAllowedIPs) {
		checkField("client_allowed_ips", target.ClientAllowedIPs, *patch.ClientAllowedIPs)
		target.ClientAllowedIPs = append([]string(nil), (*patch.ClientAllowedIPs)...)
	}
	if patch.ClientAdvertiseMainIP != nil && *patch.ClientAdvertiseMainIP != target.ClientAdvertiseMainIP {
		checkField("client_advertise_main_ip", target.ClientAdvertiseMainIP, *patch.ClientAdvertiseMainIP)
		target.ClientAdvertiseMainIP = *patch.ClientAdvertiseMainIP
	}
	if patch.Caption != nil && *patch.Caption != target.Caption {
		checkField("caption", target.Caption, *patch.Caption)
		target.Caption = *patch.Caption
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
	if patch.Tags != nil || patch.TagsAdd != nil || patch.TagsDel != nil {
		base := target.Tags
		if patch.Tags != nil {
			base = *patch.Tags
		}
		newTags := applyStringSliceDelta(base, patch.TagsDel, patch.TagsAdd, tagEqual)
		if !sliceEqual(target.Tags, newTags) {
			diffs = append(diffs, ApplyFieldDiff{
				Field:    "tags",
				OldValue: target.Tags,
				NewValue: newTags,
			})
			target.Tags = newTags
		}
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
	if patch.StaticRoutes != nil || patch.StaticRoutesAdd != nil || patch.StaticRoutesDel != nil {
		base := target.StaticRoutes
		if patch.StaticRoutes != nil {
			base = *patch.StaticRoutes
		}
		newRoutes := applyStringSliceDelta(base, patch.StaticRoutesDel, patch.StaticRoutesAdd, routeEqual)
		if !sliceEqual(target.StaticRoutes, newRoutes) {
			diffs = append(diffs, ApplyFieldDiff{
				Field:    "static_routes",
				OldValue: target.StaticRoutes,
				NewValue: newRoutes,
			})
			target.StaticRoutes = newRoutes
		}
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
	if patch.ExtraHosts != nil && *patch.ExtraHosts != target.ExtraHosts {
		checkField("extra_hosts", target.ExtraHosts, *patch.ExtraHosts)
		target.ExtraHosts = *patch.ExtraHosts
	}
	if patch.CustomNetworkScript != nil && *patch.CustomNetworkScript != target.CustomNetworkScript {
		checkField("custom_network_script", target.CustomNetworkScript, *patch.CustomNetworkScript)
		target.CustomNetworkScript = *patch.CustomNetworkScript
	}
	if patch.RouteReflector != nil && *patch.RouteReflector != target.RouteReflector {
		checkField("route_reflector", target.RouteReflector, *patch.RouteReflector)
		target.RouteReflector = *patch.RouteReflector
	}
	if patch.EnableDynamicBirdConfig != nil && *patch.EnableDynamicBirdConfig != target.EnableDynamicBirdConfig {
		checkField("enable_dynamic_bird_config", target.EnableDynamicBirdConfig, *patch.EnableDynamicBirdConfig)
		target.EnableDynamicBirdConfig = *patch.EnableDynamicBirdConfig
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
	if patch.IsClient != nil {
		node.IsClient = *patch.IsClient
	}
	if patch.ClientParent != nil {
		node.ClientParent = *patch.ClientParent
	}
	if patch.ClientAllowedIPs != nil {
		node.ClientAllowedIPs = append([]string(nil), (*patch.ClientAllowedIPs)...)
	}
	if patch.ClientAdvertiseMainIP != nil {
		node.ClientAdvertiseMainIP = *patch.ClientAdvertiseMainIP
	}
	if patch.Caption != nil {
		node.Caption = *patch.Caption
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
	var baseTags []string
	if patch.Tags != nil {
		baseTags = *patch.Tags
	}
	if patch.TagsAdd != nil || patch.TagsDel != nil {
		baseTags = applyStringSliceDelta(baseTags, patch.TagsDel, patch.TagsAdd, tagEqual)
	}
	node.Tags = baseTags
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
	var baseRoutes []string
	if patch.StaticRoutes != nil {
		baseRoutes = *patch.StaticRoutes
	}
	if patch.StaticRoutesAdd != nil || patch.StaticRoutesDel != nil {
		baseRoutes = applyStringSliceDelta(baseRoutes, patch.StaticRoutesDel, patch.StaticRoutesAdd, routeEqual)
	}
	node.StaticRoutes = baseRoutes
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
	if patch.ExtraHosts != nil {
		node.ExtraHosts = *patch.ExtraHosts
	}
	if patch.CustomNetworkScript != nil {
		node.CustomNetworkScript = *patch.CustomNetworkScript
	}
	if patch.RouteReflector != nil {
		node.RouteReflector = *patch.RouteReflector
	}
	if patch.EnableDynamicBirdConfig != nil {
		node.EnableDynamicBirdConfig = *patch.EnableDynamicBirdConfig
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
	if patch.Tags != nil || patch.TagsAdd != nil || patch.TagsDel != nil {
		base := target.Tags
		if patch.Tags != nil {
			base = *patch.Tags
		}
		newTags := applyStringSliceDelta(base, patch.TagsDel, patch.TagsAdd, tagEqual)
		if !sliceEqual(target.Tags, newTags) {
			diffs = append(diffs, ApplyFieldDiff{
				Field:    "tags",
				OldValue: target.Tags,
				NewValue: newTags,
			})
			target.Tags = newTags
		}
	}
	if patch.AssignIPv4 != nil && *patch.AssignIPv4 != target.AssignIPv4 {
		checkField("assign_ipv4", target.AssignIPv4, *patch.AssignIPv4)
		target.AssignIPv4 = *patch.AssignIPv4
	}
	if patch.DisableBFD != nil && *patch.DisableBFD != target.DisableBFD {
		checkField("disable_bfd", target.DisableBFD, *patch.DisableBFD)
		target.DisableBFD = *patch.DisableBFD
	}
	if patch.Disabled != nil && *patch.Disabled != target.Disabled {
		checkField("disabled", target.Disabled, *patch.Disabled)
		target.Disabled = *patch.Disabled
	}
	if patch.Psk != nil && *patch.Psk != target.Psk {
		checkField("psk", "***", "***")
		target.Psk = *patch.Psk
	}
	if patch.Note != nil && *patch.Note != target.Note {
		checkField("note", target.Note, *patch.Note)
		target.Note = *patch.Note
	}

	mergeLinkEnd(targetFrom, patchFrom, "from", &diffs)
	mergeLinkEnd(targetTo, patchTo, "to", &diffs)

	explicitObfuscatorDisable := (patchFrom != nil && patchFrom.Obfuscator != nil && config.NormalizeObfuscatorRole(*patchFrom.Obfuscator) == config.ObfuscatorNone) ||
		(patchTo != nil && patchTo.Obfuscator != nil && config.NormalizeObfuscatorRole(*patchTo.Obfuscator) == config.ObfuscatorNone)
	if explicitObfuscatorDisable {
		if targetFrom.ObfuscatorRole() != config.ObfuscatorNone || targetTo.ObfuscatorRole() != config.ObfuscatorNone {
			diffs = append(diffs, ApplyFieldDiff{Field: "obfuscator", OldValue: "enabled", NewValue: config.ObfuscatorNone})
		}
		targetFrom.Obfuscator = ""
		targetFrom.ObfuscatorParameters = nil
		targetTo.Obfuscator = ""
		targetTo.ObfuscatorParameters = nil
	}

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
	// patch.NeighborAddress is intentionally ignored: neighbor_address is a system-managed
	// derivative of the peer's address and is not manually editable
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
	if patch.AddPaths != nil && *patch.AddPaths != target.AddPaths {
		checkField("add_paths", target.AddPaths, *patch.AddPaths)
		target.AddPaths = *patch.AddPaths
	}
	if patch.Note != nil && *patch.Note != target.Note {
		checkField("note", target.Note, *patch.Note)
		target.Note = *patch.Note
	}
	if patch.Obfuscator != nil {
		role := config.NormalizeObfuscatorRole(*patch.Obfuscator)
		if role != target.ObfuscatorRole() {
			checkField("obfuscator", target.ObfuscatorRole(), role)
		}
		target.Obfuscator = role
		if role == config.ObfuscatorNone {
			target.ObfuscatorParameters = nil
		}
	}
	if patch.ObfuscatorParameters != nil {
		if !reflect.DeepEqual(target.ObfuscatorParameters, patch.ObfuscatorParameters) {
			checkField("obfuscator_parameters", target.ObfuscatorParameters, patch.ObfuscatorParameters)
		}
		target.ObfuscatorParameters = patch.ObfuscatorParameters.Clone()
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
	var baseTags []string
	if patch.Tags != nil {
		baseTags = *patch.Tags
	}
	if patch.TagsAdd != nil || patch.TagsDel != nil {
		baseTags = applyStringSliceDelta(baseTags, patch.TagsDel, patch.TagsAdd, tagEqual)
	}
	link.Tags = baseTags
	if patch.AssignIPv4 != nil {
		link.AssignIPv4 = *patch.AssignIPv4
	}
	if patch.DisableBFD != nil {
		link.DisableBFD = *patch.DisableBFD
	}
	if patch.Disabled != nil {
		link.Disabled = *patch.Disabled
	}
	if patch.Psk != nil {
		link.Psk = *patch.Psk
	}
	if patch.Note != nil {
		link.Note = *patch.Note
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

func mergeApp(target *config.App, patch config.AppPatch, now time.Time) ([]ApplyFieldDiff, error) {
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

	if patch.Description != nil && *patch.Description != target.Description {
		checkField("description", target.Description, *patch.Description)
		target.Description = *patch.Description
	}
	if patch.Group != nil && *patch.Group != target.Group {
		checkField("group", target.Group, *patch.Group)
		target.Group = *patch.Group
	}
	if patch.Note != nil && *patch.Note != target.Note {
		checkField("note", target.Note, *patch.Note)
		target.Note = *patch.Note
	}
	if patch.URL != nil && *patch.URL != target.URL {
		checkField("url", target.URL, *patch.URL)
		target.URL = *patch.URL
	}
	if patch.Version != nil && *patch.Version != target.Version {
		checkField("version", target.Version, *patch.Version)
		target.Version = *patch.Version
	}
	if patch.AppType != nil && *patch.AppType != target.AppType {
		checkField("app_type", target.AppType, *patch.AppType)
		target.AppType = *patch.AppType
	}
	if patch.MainContent != nil && *patch.MainContent != target.MainContent {
		checkField("main_content", target.MainContent, *patch.MainContent)
		target.MainContent = *patch.MainContent
	}
	if patch.EnvContent != nil && *patch.EnvContent != target.EnvContent {
		checkField("env_content", target.EnvContent, *patch.EnvContent)
		target.EnvContent = *patch.EnvContent
	}
	if patch.Files != nil && !reflect.DeepEqual(*patch.Files, target.Files) {
		checkField("files", target.Files, *patch.Files)
		target.Files = *patch.Files
	}
	if patch.Artifact != nil && !reflect.DeepEqual(patch.Artifact, target.Artifact) {
		checkField("artifact", target.Artifact, patch.Artifact)
		target.Artifact = patch.Artifact
	}
	if patch.Selector != nil && !reflect.DeepEqual(*patch.Selector, target.Selector) {
		checkField("selector", target.Selector, *patch.Selector)
		target.Selector = *patch.Selector
	}
	if patch.Published != nil && *patch.Published != target.Published {
		checkField("published", target.Published, *patch.Published)
		target.Published = *patch.Published
	}
	if patch.Enabled != nil && *patch.Enabled != target.Enabled {
		checkField("enabled", target.Enabled, *patch.Enabled)
		target.Enabled = *patch.Enabled
	}

	if strings.ToLower(target.AppType) == "systemd" && target.MainContent != "" {
		target.MainContent = apps.EnsureSystemdEnvironmentFile(target.Name, target.MainContent)
	}

	if err := config.ValidateApp(*target); err != nil {
		return nil, err
	}

	if len(diffs) > 0 {
		target.UpdatedAt = now
	}
	return diffs, nil
}

func createAppFromPatch(patch config.AppPatch, now time.Time) (*config.App, []ApplyFieldDiff, error) {
	appID := ""
	if patch.ID != nil && *patch.ID != "" {
		appID = *patch.ID
	} else {
		appID = "app_" + uuid.New().String()[:8]
	}

	app := config.App{
		ID:        appID,
		Name:      patch.Name,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if patch.Description != nil {
		app.Description = *patch.Description
	}
	if patch.Group != nil {
		app.Group = *patch.Group
	}
	if patch.Note != nil {
		app.Note = *patch.Note
	}
	if patch.URL != nil {
		app.URL = *patch.URL
	}
	if patch.Version != nil {
		app.Version = *patch.Version
	}
	if patch.AppType != nil {
		app.AppType = *patch.AppType
	}
	if patch.MainContent != nil {
		app.MainContent = *patch.MainContent
	}
	if patch.EnvContent != nil {
		app.EnvContent = *patch.EnvContent
	}
	if patch.Files != nil {
		app.Files = *patch.Files
	}
	if patch.Artifact != nil {
		app.Artifact = patch.Artifact
	}
	if patch.Selector != nil {
		app.Selector = *patch.Selector
	}
	if patch.Published != nil {
		app.Published = *patch.Published
	}
	if patch.Enabled != nil {
		app.Enabled = *patch.Enabled
	}

	if strings.ToLower(app.AppType) == "systemd" && app.MainContent != "" {
		app.MainContent = apps.EnsureSystemdEnvironmentFile(app.Name, app.MainContent)
	}

	if err := config.ValidateApp(app); err != nil {
		return nil, nil, err
	}

	var diffs []ApplyFieldDiff
	diffs = append(diffs, ApplyFieldDiff{
		Field:    "app",
		OldValue: nil,
		NewValue: app.Name,
	})

	return &app, diffs, nil
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

	if patch.Name != nil && *patch.Name != target.Name {
		checkField("name", target.Name, *patch.Name)
		target.Name = *patch.Name
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
	if patch.ExtraHosts != nil && *patch.ExtraHosts != target.ExtraHosts {
		checkField("extra_hosts", target.ExtraHosts, *patch.ExtraHosts)
		target.ExtraHosts = *patch.ExtraHosts
	}
	if patch.BirdImportFilter != nil && *patch.BirdImportFilter != target.BirdImportFilter {
		checkField("bird_import_filter", target.BirdImportFilter, *patch.BirdImportFilter)
		target.BirdImportFilter = *patch.BirdImportFilter
	}
	if patch.BirdExportFilter != nil && *patch.BirdExportFilter != target.BirdExportFilter {
		checkField("bird_export_filter", target.BirdExportFilter, *patch.BirdExportFilter)
		target.BirdExportFilter = *patch.BirdExportFilter
	}
	if patch.BirdPreSnippet != nil && *patch.BirdPreSnippet != target.BirdPreSnippet {
		checkField("bird_pre_snippet", target.BirdPreSnippet, *patch.BirdPreSnippet)
		target.BirdPreSnippet = *patch.BirdPreSnippet
	}
	if patch.BirdPostSnippet != nil && *patch.BirdPostSnippet != target.BirdPostSnippet {
		checkField("bird_post_snippet", target.BirdPostSnippet, *patch.BirdPostSnippet)
		target.BirdPostSnippet = *patch.BirdPostSnippet
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

func mergeNetworkPolicy(target *config.NetworkPolicy, patch config.NetworkPolicyPatch) ([]ApplyFieldDiff, error) {
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

	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name == "" {
			return nil, errors.New("policy name cannot be empty")
		}
		if name != target.Name {
			checkField("name", target.Name, name)
			target.Name = name
		}
	}
	if patch.Description != nil {
		desc := strings.TrimSpace(*patch.Description)
		if desc != target.Description {
			checkField("description", target.Description, desc)
			target.Description = desc
		}
	}
	if patch.Cost != nil {
		cost := *patch.Cost
		if cost <= 0 {
			cost = 100
		}
		if cost != target.Cost {
			checkField("cost", target.Cost, cost)
			target.Cost = cost
		}
	}
	if patch.LocalNetworks != nil {
		cleaned := config.CleanPrefixes(*patch.LocalNetworks)
		if !sliceEqual(target.LocalNetworks, cleaned) {
			checkField("local_networks", target.LocalNetworks, cleaned)
			target.LocalNetworks = cleaned
		}
	}
	if patch.AllowedDstCIDRs != nil {
		cleaned := config.CleanPrefixes(*patch.AllowedDstCIDRs)
		if !sliceEqual(target.AllowedDstCIDRs, cleaned) {
			checkField("allowed_dst_cidrs", target.AllowedDstCIDRs, cleaned)
			target.AllowedDstCIDRs = cleaned
		}
	}
	if patch.AllowedSrcCIDRs != nil {
		cleaned := config.CleanPrefixes(*patch.AllowedSrcCIDRs)
		if !sliceEqual(target.AllowedSrcCIDRs, cleaned) {
			checkField("allowed_src_cidrs", target.AllowedSrcCIDRs, cleaned)
			target.AllowedSrcCIDRs = cleaned
		}
	}
	if patch.DisallowedDstCIDRs != nil {
		cleaned := config.CleanPrefixes(*patch.DisallowedDstCIDRs)
		if !sliceEqual(target.DisallowedDstCIDRs, cleaned) {
			checkField("disallowed_dst_cidrs", target.DisallowedDstCIDRs, cleaned)
			target.DisallowedDstCIDRs = cleaned
		}
	}
	if patch.DisallowedSrcCIDRs != nil {
		cleaned := config.CleanPrefixes(*patch.DisallowedSrcCIDRs)
		if !sliceEqual(target.DisallowedSrcCIDRs, cleaned) {
			checkField("disallowed_src_cidrs", target.DisallowedSrcCIDRs, cleaned)
			target.DisallowedSrcCIDRs = cleaned
		}
	}
	if patch.RejectInternet != nil && *patch.RejectInternet != target.RejectInternet {
		checkField("reject_internet", target.RejectInternet, *patch.RejectInternet)
		target.RejectInternet = *patch.RejectInternet
	}
	if patch.FilterForward != nil && *patch.FilterForward != target.FilterForward {
		checkField("filter_forward", target.FilterForward, *patch.FilterForward)
		target.FilterForward = *patch.FilterForward
	}
	if patch.FilterInput != nil && *patch.FilterInput != target.FilterInput {
		checkField("filter_input", target.FilterInput, *patch.FilterInput)
		target.FilterInput = *patch.FilterInput
	}
	if patch.InputAllowICMP != nil && *patch.InputAllowICMP != target.InputAllowICMP {
		checkField("input_allow_icmp", target.InputAllowICMP, *patch.InputAllowICMP)
		target.InputAllowICMP = *patch.InputAllowICMP
	}
	if patch.InputAllowICMP6 != nil && *patch.InputAllowICMP6 != target.InputAllowICMP6 {
		checkField("input_allow_icmp6", target.InputAllowICMP6, *patch.InputAllowICMP6)
		target.InputAllowICMP6 = *patch.InputAllowICMP6
	}
	if patch.InputTCPPorts != nil {
		cleaned := config.CleanPortList(*patch.InputTCPPorts)
		if !sliceEqual(target.InputTCPPorts, cleaned) {
			checkField("input_tcp_ports", target.InputTCPPorts, cleaned)
			target.InputTCPPorts = cleaned
		}
	}
	if patch.InputUDPPorts != nil {
		cleaned := config.CleanPortList(*patch.InputUDPPorts)
		if !sliceEqual(target.InputUDPPorts, cleaned) {
			checkField("input_udp_ports", target.InputUDPPorts, cleaned)
			target.InputUDPPorts = cleaned
		}
	}
	if patch.SNAT != nil && !reflect.DeepEqual(target.SNAT, patch.SNAT) {
		checkField("snat", target.SNAT, patch.SNAT)
		snatCopy := *patch.SNAT
		target.SNAT = &snatCopy
	}
	if patch.ForwardSNAT != nil && *patch.ForwardSNAT != target.ForwardSNAT {
		checkField("forward_snat", target.ForwardSNAT, *patch.ForwardSNAT)
		target.ForwardSNAT = *patch.ForwardSNAT
	}
	if patch.ForwardSNATTarget != nil {
		val := strings.TrimSpace(*patch.ForwardSNATTarget)
		if val != target.ForwardSNATTarget {
			checkField("forward_snat_target", target.ForwardSNATTarget, val)
			target.ForwardSNATTarget = val
		}
	}
	if patch.DSCPIngress != nil {
		val := config.ValidateDSCP(*patch.DSCPIngress)
		if val != target.DSCPIngress {
			checkField("dscp_ingress", target.DSCPIngress, val)
			target.DSCPIngress = val
		}
	}
	if patch.DSCPEgress != nil {
		val := config.ValidateDSCP(*patch.DSCPEgress)
		if val != target.DSCPEgress {
			checkField("dscp_egress", target.DSCPEgress, val)
			target.DSCPEgress = val
		}
	}
	if patch.BlockIngressNew != nil {
		val := config.NormalizeBlockIngressNew(*patch.BlockIngressNew)
		if val != target.BlockIngressNew {
			checkField("block_ingress_new", target.BlockIngressNew, val)
			target.BlockIngressNew = val
		}
	}
	if patch.IngressNewWhitelist != nil {
		cleaned := config.CleanIngressNewWhitelist(*patch.IngressNewWhitelist)
		if !sliceEqual(target.IngressNewWhitelist, cleaned) {
			checkField("ingress_new_whitelist", target.IngressNewWhitelist, cleaned)
			target.IngressNewWhitelist = cleaned
		}
	}
	if patch.ROA4 != nil {
		val := strings.TrimSpace(*patch.ROA4)
		if err := roa.ValidateROAURL(val); err != nil {
			return nil, fmt.Errorf("invalid roa4: %w", err)
		}
		if val != target.ROA4 {
			checkField("roa4", target.ROA4, val)
			target.ROA4 = val
		}
	}
	if patch.ROA6 != nil {
		val := strings.TrimSpace(*patch.ROA6)
		if err := roa.ValidateROAURL(val); err != nil {
			return nil, fmt.Errorf("invalid roa6: %w", err)
		}
		if val != target.ROA6 {
			checkField("roa6", target.ROA6, val)
			target.ROA6 = val
		}
	}
	if patch.ROAStrict != nil && *patch.ROAStrict != target.ROAStrict {
		checkField("roa_strict", target.ROAStrict, *patch.ROAStrict)
		target.ROAStrict = *patch.ROAStrict
	}
	if patch.Fwmark != nil {
		val := strings.TrimSpace(*patch.Fwmark)
		if val != target.Fwmark {
			checkField("fwmark", target.Fwmark, val)
			target.Fwmark = val
		}
	}
	if patch.Preference != nil && !reflect.DeepEqual(target.Preference, patch.Preference) {
		checkField("preference", target.Preference, patch.Preference)
		prefCopy := *patch.Preference
		target.Preference = &prefCopy
	}
	if patch.Mark != nil {
		val := strings.TrimSpace(*patch.Mark)
		if val != target.Mark {
			checkField("mark", target.Mark, val)
			target.Mark = val
		}
	}
	if patch.RoutingPolicy != nil {
		val := config.NormalizeRoutingPolicy(*patch.RoutingPolicy)
		if val != target.RoutingPolicy {
			checkField("routing_policy", target.RoutingPolicy, val)
			target.RoutingPolicy = val
		}
	}
	if patch.AddPaths != nil {
		val := config.NormalizeAddPaths(*patch.AddPaths)
		if val != target.AddPaths {
			checkField("add_paths", target.AddPaths, val)
			target.AddPaths = val
		}
	}
	if patch.BirdImportFilter != nil {
		val := strings.TrimSpace(*patch.BirdImportFilter)
		if val != target.BirdImportFilter {
			checkField("bird_import_filter", target.BirdImportFilter, val)
			target.BirdImportFilter = val
		}
	}
	if patch.BirdExportFilter != nil {
		val := strings.TrimSpace(*patch.BirdExportFilter)
		if val != target.BirdExportFilter {
			checkField("bird_export_filter", target.BirdExportFilter, val)
			target.BirdExportFilter = val
		}
	}
	if patch.BirdPreSnippet != nil {
		val := strings.TrimSpace(*patch.BirdPreSnippet)
		if val != target.BirdPreSnippet {
			checkField("bird_pre_snippet", target.BirdPreSnippet, val)
			target.BirdPreSnippet = val
		}
	}
	if patch.BirdPostSnippet != nil {
		val := strings.TrimSpace(*patch.BirdPostSnippet)
		if val != target.BirdPostSnippet {
			checkField("bird_post_snippet", target.BirdPostSnippet, val)
			target.BirdPostSnippet = val
		}
	}

	return diffs, nil
}

func createNetworkPolicyFromPatch(patch config.NetworkPolicyPatch) (*config.NetworkPolicy, []ApplyFieldDiff, error) {
	id := ""
	if patch.ID != nil {
		id = strings.TrimSpace(*patch.ID)
	}
	if id == "" {
		id = "policy_" + uuid.New().String()[:8]
	}
	name := ""
	if patch.Name != nil {
		name = strings.TrimSpace(*patch.Name)
	}
	if name == "" {
		return nil, nil, fmt.Errorf("policy name is required when creating network policy %q", id)
	}

	pol := config.NetworkPolicy{
		ID:   id,
		Name: name,
		Cost: 100,
	}
	if _, err := mergeNetworkPolicy(&pol, patch); err != nil {
		return nil, nil, err
	}

	diffs := []ApplyFieldDiff{
		{
			Field:    "network_policy",
			OldValue: nil,
			NewValue: pol.Name,
		},
	}
	return &pol, diffs, nil
}

func mergeBlock(target *config.Block, patch config.BlockPatch) []ApplyFieldDiff {
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

	if patch.Name != nil && strings.TrimSpace(*patch.Name) != target.Name {
		name := strings.TrimSpace(*patch.Name)
		checkField("name", target.Name, name)
		target.Name = name
	}
	if patch.Color != nil && *patch.Color != target.Color {
		checkField("color", target.Color, *patch.Color)
		target.Color = *patch.Color
	}
	if patch.X != nil && *patch.X != target.X {
		checkField("x", target.X, *patch.X)
		target.X = *patch.X
	}
	if patch.Y != nil && *patch.Y != target.Y {
		checkField("y", target.Y, *patch.Y)
		target.Y = *patch.Y
	}
	if patch.Width != nil && *patch.Width != target.Width {
		checkField("width", target.Width, *patch.Width)
		target.Width = *patch.Width
	}
	if patch.Height != nil && *patch.Height != target.Height {
		checkField("height", target.Height, *patch.Height)
		target.Height = *patch.Height
	}
	if patch.Nodes != nil && !sliceEqual(target.Nodes, *patch.Nodes) {
		checkField("nodes", target.Nodes, *patch.Nodes)
		target.Nodes = append([]string(nil), (*patch.Nodes)...)
	}

	return diffs
}

func createBlockFromPatch(patch config.BlockPatch) (*config.Block, []ApplyFieldDiff, error) {
	id := ""
	if patch.ID != nil {
		id = strings.TrimSpace(*patch.ID)
	}
	if id == "" {
		id = "block_" + uuid.New().String()[:8]
	}
	name := ""
	if patch.Name != nil {
		name = strings.TrimSpace(*patch.Name)
	}
	if name == "" {
		return nil, nil, fmt.Errorf("block name is required when creating block %q", id)
	}

	block := config.Block{
		ID:   id,
		Name: name,
	}
	mergeBlock(&block, patch)

	diffs := []ApplyFieldDiff{
		{
			Field:    "block",
			OldValue: nil,
			NewValue: block.Name,
		},
	}
	return &block, diffs, nil
}

func mergeTemplate(target *config.ConfigTemplate, patch config.ConfigTemplatePatch, now time.Time) ([]ApplyFieldDiff, error) {
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

	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name == "" {
			return nil, errors.New("template name cannot be empty")
		}
		if name != target.Name {
			checkField("name", target.Name, name)
			target.Name = name
		}
	}
	if patch.Type != nil && strings.TrimSpace(*patch.Type) != "" {
		tmplType := strings.ToLower(strings.TrimSpace(*patch.Type))
		if !config.IsValidTemplateType(tmplType) {
			return nil, fmt.Errorf("invalid template type %q: must be wg, bird, or nft", tmplType)
		}
		if tmplType != target.Type {
			checkField("type", target.Type, tmplType)
			target.Type = tmplType
		}
	}
	if patch.Description != nil {
		desc := strings.TrimSpace(*patch.Description)
		if desc != target.Description {
			checkField("description", target.Description, desc)
			target.Description = desc
		}
	}
	if patch.Content != nil && *patch.Content != target.Content {
		checkField("content", target.Content, *patch.Content)
		target.Content = *patch.Content
	}

	if err := compiler.ValidateTemplate(target.Type, target.Content); err != nil {
		return nil, err
	}

	if len(diffs) > 0 {
		target.ModifiedAt = now
	}
	return diffs, nil
}

func createTemplateFromPatch(patch config.ConfigTemplatePatch, now time.Time) (*config.ConfigTemplate, []ApplyFieldDiff, error) {
	id := ""
	if patch.ID != nil {
		id = strings.TrimSpace(*patch.ID)
	}
	if id == "" {
		id = "tmpl_" + uuid.New().String()[:8]
	}
	name := ""
	if patch.Name != nil {
		name = strings.TrimSpace(*patch.Name)
	}
	if name == "" {
		return nil, nil, fmt.Errorf("template name is required when creating template %q", id)
	}
	tmplType := ""
	if patch.Type != nil {
		tmplType = strings.ToLower(strings.TrimSpace(*patch.Type))
	}
	if !config.IsValidTemplateType(tmplType) {
		return nil, nil, fmt.Errorf("valid template type (wg, bird, or nft) is required when creating template %q", id)
	}
	content := ""
	if patch.Content != nil {
		content = *patch.Content
	}
	if err := compiler.ValidateTemplate(tmplType, content); err != nil {
		return nil, nil, err
	}

	tmpl := config.ConfigTemplate{
		ID:         id,
		Name:       name,
		Type:       tmplType,
		Content:    content,
		ModifiedAt: now,
	}
	if patch.Description != nil {
		tmpl.Description = strings.TrimSpace(*patch.Description)
	}

	diffs := []ApplyFieldDiff{
		{
			Field:    "template",
			OldValue: nil,
			NewValue: tmpl.Name,
		},
	}
	return &tmpl, diffs, nil
}

func mergeServerTask(target *config.ServerTaskConfig, patch config.ServerTaskPatch) []ApplyFieldDiff {
	var diffs []ApplyFieldDiff
	if patch.CronExpr != nil {
		expr := strings.TrimSpace(*patch.CronExpr)
		if expr != target.CronExpr {
			diffs = append(diffs, ApplyFieldDiff{
				Field:    "cron_expr",
				OldValue: target.CronExpr,
				NewValue: expr,
			})
			target.CronExpr = expr
		}
	}
	if patch.Enabled != nil && *patch.Enabled != target.Enabled {
		diffs = append(diffs, ApplyFieldDiff{
			Field:    "enabled",
			OldValue: target.Enabled,
			NewValue: *patch.Enabled,
		})
		target.Enabled = *patch.Enabled
	}
	return diffs
}

func validateCronTaskFields(task config.CronTask, nodes []config.Node) error {
	if strings.TrimSpace(task.Name) == "" {
		return fmt.Errorf("cron task name cannot be empty")
	}
	if task.TaskType != "ping" && task.TaskType != "tcp_ping" && task.TaskType != "http" {
		return fmt.Errorf("only 'ping', 'tcp_ping' and 'http' task types are supported for cron tasks")
	}
	if err := config.ValidateCronExpr(task.CronExpr); err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}
	if err := config.ValidateCronTask(task); err != nil {
		return err
	}
	if task.Params == nil || strings.TrimSpace(task.Params["target"]) == "" {
		return fmt.Errorf("task parameter 'target' is required")
	}
	if task.TaskType == "http" {
		if err := lookingglass.ValidateHTTPURL(task.Params["target"]); err != nil {
			return fmt.Errorf("invalid http target URL: %w", err)
		}
	}
	matched := false
	for i := range nodes {
		n := &nodes[i]
		if !n.IsUnmanaged() && config.CronTaskTargetsNode(task, n, n.Name) {
			matched = true
			break
		}
	}
	if !matched {
		return fmt.Errorf("no managed nodes match the cron task selector")
	}
	return nil
}

func normalizeCronTaskParams(task *config.CronTask) {
	defaultCount, maxCount := 4, 20
	if task.TaskType == "http" {
		defaultCount, maxCount = 1, config.HTTPProbeMaxCount
	}
	if task.Params == nil {
		task.Params = make(map[string]string)
	}
	if countStr, ok := task.Params["count"]; ok && countStr != "" {
		c, err := strconv.Atoi(countStr)
		if err != nil || c < 1 || c > maxCount {
			task.Params["count"] = strconv.Itoa(defaultCount)
		}
	} else {
		task.Params["count"] = strconv.Itoa(defaultCount)
	}
	if task.TimeoutSec <= 0 {
		task.TimeoutSec = 15
	}
	if task.TimeoutSec > 60 {
		task.TimeoutSec = 60
	}
}

func mergeCronTask(target *config.CronTask, patch config.CronTaskPatch, nodes []config.Node, now time.Time) ([]ApplyFieldDiff, error) {
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

	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name != target.Name {
			checkField("name", target.Name, name)
			target.Name = name
		}
	}
	if patch.Description != nil {
		desc := strings.TrimSpace(*patch.Description)
		if desc != target.Description {
			checkField("description", target.Description, desc)
			target.Description = desc
		}
	}
	if patch.TaskType != nil {
		tt := strings.TrimSpace(*patch.TaskType)
		if tt != target.TaskType {
			checkField("task_type", target.TaskType, tt)
			target.TaskType = tt
		}
	}
	if patch.CronExpr != nil {
		expr := strings.TrimSpace(*patch.CronExpr)
		if expr != target.CronExpr {
			checkField("cron_expr", target.CronExpr, expr)
			target.CronExpr = expr
		}
	}
	if patch.Selector != nil && !reflect.DeepEqual(*patch.Selector, target.Selector) {
		checkField("selector", target.Selector, *patch.Selector)
		target.Selector = normalizeSelectorMap(*patch.Selector)
	}
	if patch.Params != nil && !reflect.DeepEqual(*patch.Params, target.Params) {
		checkField("params", target.Params, *patch.Params)
		paramsCopy := make(map[string]string, len(*patch.Params))
		for k, v := range *patch.Params {
			paramsCopy[k] = v
		}
		target.Params = paramsCopy
	}
	if patch.TimeoutSec != nil && *patch.TimeoutSec != target.TimeoutSec {
		checkField("timeout_sec", target.TimeoutSec, *patch.TimeoutSec)
		target.TimeoutSec = *patch.TimeoutSec
	}
	if patch.Enabled != nil && *patch.Enabled != target.Enabled {
		checkField("enabled", target.Enabled, *patch.Enabled)
		target.Enabled = *patch.Enabled
	}

	normalizeCronTaskParams(target)
	if err := validateCronTaskFields(*target, nodes); err != nil {
		return nil, err
	}

	if len(diffs) > 0 {
		target.UpdatedAt = now
	}
	return diffs, nil
}

func createCronTaskFromPatch(patch config.CronTaskPatch, nodes []config.Node, now time.Time) (*config.CronTask, []ApplyFieldDiff, error) {
	id := ""
	if patch.ID != nil {
		id = strings.TrimSpace(*patch.ID)
	}
	if id == "" {
		id = "cron_" + uuid.New().String()[:8]
	}

	task := config.CronTask{
		ID:        id,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if patch.Name != nil {
		task.Name = strings.TrimSpace(*patch.Name)
	}
	if patch.Description != nil {
		task.Description = strings.TrimSpace(*patch.Description)
	}
	if patch.TaskType != nil {
		task.TaskType = strings.TrimSpace(*patch.TaskType)
	}
	if patch.CronExpr != nil {
		task.CronExpr = strings.TrimSpace(*patch.CronExpr)
	}
	if patch.Selector != nil {
		task.Selector = normalizeSelectorMap(*patch.Selector)
	}
	if patch.Params != nil {
		task.Params = make(map[string]string, len(*patch.Params))
		for k, v := range *patch.Params {
			task.Params[k] = v
		}
	}
	if patch.TimeoutSec != nil {
		task.TimeoutSec = *patch.TimeoutSec
	}
	if patch.Enabled != nil {
		task.Enabled = *patch.Enabled
	}

	normalizeCronTaskParams(&task)
	if err := validateCronTaskFields(task, nodes); err != nil {
		return nil, nil, err
	}

	diffs := []ApplyFieldDiff{
		{
			Field:    "cron_task",
			OldValue: nil,
			NewValue: task.Name,
		},
	}
	return &task, diffs, nil
}

func mergeSSHKeyRule(target *config.SSHKeyRule, patch config.SSHKeyRulePatch, now time.Time) ([]ApplyFieldDiff, error) {
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

	if patch.Type != nil {
		t := strings.TrimSpace(*patch.Type)
		if t == "" {
			t = config.SSHKeyRuleTypeAll
		}
		if t != target.Type {
			checkField("type", target.Type, t)
			target.Type = t
		}
	}
	if patch.SrcSelector != nil {
		norm := normalizeSelectorMap(*patch.SrcSelector)
		if !reflect.DeepEqual(norm, target.SrcSelector) {
			checkField("src_selector", target.SrcSelector, norm)
			target.SrcSelector = norm
		}
	}
	if patch.SourceKeys != nil {
		norm := config.NormalizeSourceKeys(*patch.SourceKeys)
		if norm != target.SourceKeys {
			checkField("source_keys", target.SourceKeys, norm)
			target.SourceKeys = norm
		}
	}
	if patch.DstSelector != nil {
		norm := normalizeSelectorMap(*patch.DstSelector)
		if !reflect.DeepEqual(norm, target.DstSelector) {
			checkField("dst_selector", target.DstSelector, norm)
			target.DstSelector = norm
		}
	}
	if patch.Enabled != nil && *patch.Enabled != target.Enabled {
		checkField("enabled", target.Enabled, *patch.Enabled)
		target.Enabled = *patch.Enabled
	}

	if err := config.ValidateSSHKeyRule(*target); err != nil {
		return nil, err
	}
	target.Type = target.NormalizedType()

	if len(diffs) > 0 {
		target.UpdatedAt = now
	}
	return diffs, nil
}

func createSSHKeyRuleFromPatch(patch config.SSHKeyRulePatch, now time.Time) (*config.SSHKeyRule, []ApplyFieldDiff, error) {
	id := ""
	if patch.ID != nil {
		id = strings.TrimSpace(*patch.ID)
	}
	if id == "" {
		id = "sshrule_" + uuid.New().String()[:8]
	}

	rule := config.SSHKeyRule{
		ID:        id,
		Type:      config.SSHKeyRuleTypeAll,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if patch.Type != nil && strings.TrimSpace(*patch.Type) != "" {
		rule.Type = strings.TrimSpace(*patch.Type)
	}
	if patch.SrcSelector != nil {
		rule.SrcSelector = normalizeSelectorMap(*patch.SrcSelector)
	}
	if patch.SourceKeys != nil {
		rule.SourceKeys = config.NormalizeSourceKeys(*patch.SourceKeys)
	}
	if patch.DstSelector != nil {
		rule.DstSelector = normalizeSelectorMap(*patch.DstSelector)
	}
	if patch.Enabled != nil {
		rule.Enabled = *patch.Enabled
	}

	if err := config.ValidateSSHKeyRule(rule); err != nil {
		return nil, nil, err
	}
	rule.Type = rule.NormalizedType()

	diffs := []ApplyFieldDiff{
		{
			Field:    "ssh_key_rule",
			OldValue: nil,
			NewValue: rule.ID,
		},
	}
	return &rule, diffs, nil
}

func (m *Manager) recomputeLinksAndEndpoints(cfg *config.Config, now time.Time) {
	for i := range cfg.Links {
		l := &cfg.Links[i]
		if l.IsVirtual() {
			continue
		}
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
		if !l.IsManual() {
			l.From.ResolvedEndpoint = compiler.ResolveLinkEndpoint(fromN, toN, &l.From, &l.To)
			l.To.ResolvedEndpoint = compiler.ResolveLinkEndpoint(toN, fromN, &l.To, &l.From)
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
	if c.NetworkSettings.PublicASN > 0 || len(c.NetworkSettings.Prefixes) > 0 || c.NetworkSettings.Name != "" {
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
	if len(c.Apps) > 0 {
		appsList := make([]map[string]interface{}, len(c.Apps))
		for i, a := range c.Apps {
			appsList[i] = map[string]interface{}{
				"name":        a.Name,
				"app_type":    a.AppType,
				"enabled":     a.Enabled,
				"description": a.Description,
			}
		}
		m["apps"] = appsList
	}
	if len(c.NetworkPolicies) > 0 {
		m["network_policies"] = c.NetworkPolicies
	}
	if len(c.Blocks) > 0 {
		m["blocks"] = c.Blocks
	}
	if len(c.Templates) > 0 {
		m["templates"] = c.Templates
	}
	if len(c.ServerTasks) > 0 {
		m["server_tasks"] = c.ServerTasks
	}
	if len(c.CronTasks) > 0 {
		m["cron_tasks"] = c.CronTasks
	}
	if len(c.SSHKeyRules) > 0 {
		m["ssh_key_rules"] = c.SSHKeyRules
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

// formatAppArtifactDiff renders a human-readable diff for an artifact whose
// server-side cache contents changed after the last node sync.
func formatAppArtifactDiff(appName, oldHash, newHash string) string {
	shortHash := func(hash string) string {
		if hash == "" {
			return "(not synced)"
		}
		if len(hash) > 16 {
			return hash[:16]
		}
		return hash
	}

	return fmt.Sprintf("--- artifact:%s (last synced)\n+++ artifact:%s (server cache)\n@@ artifact content changed @@\n-%s\n+%s",
		appName, appName, shortHash(oldHash), shortHash(newHash))
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
	cc.Name = strings.TrimSpace(cc.Name)
	if cc.Name == "" {
		cc.Name = "Snippet " + now.Format("2006-01-02 15:04:05")
	}

	matchIdx := -1
	// Priority 1: Match by ID if ID is provided
	if cc.ID != "" {
		for i := range cfg.ConfigChanges {
			if cfg.ConfigChanges[i].ID == cc.ID {
				matchIdx = i
				break
			}
		}
	}

	// Priority 2: Match by name (case-insensitive) if not already matched
	if matchIdx == -1 {
		for i := range cfg.ConfigChanges {
			if strings.EqualFold(strings.TrimSpace(cfg.ConfigChanges[i].Name), cc.Name) {
				matchIdx = i
				break
			}
		}
	}

	if matchIdx != -1 {
		existing := cfg.ConfigChanges[matchIdx]
		if cc.ID == "" {
			cc.ID = existing.ID
		}
		if cc.CreatedAt.IsZero() {
			cc.CreatedAt = existing.CreatedAt
		}
		cc.ModifiedAt = now
		cfg.ConfigChanges[matchIdx] = cc

		// Remove any other duplicates with the same name
		for i := len(cfg.ConfigChanges) - 1; i >= 0; i-- {
			if i != matchIdx && strings.EqualFold(strings.TrimSpace(cfg.ConfigChanges[i].Name), cc.Name) {
				cfg.ConfigChanges = append(cfg.ConfigChanges[:i], cfg.ConfigChanges[i+1:]...)
			}
		}
	} else {
		if cc.ID == "" {
			cc.ID = uuid.New().String()
		}
		if cc.CreatedAt.IsZero() {
			cc.CreatedAt = now
		}
		cc.ModifiedAt = now
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
