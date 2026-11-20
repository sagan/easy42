package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"easy42/internal/agent"
	agentpb "easy42/internal/agent/proto"
	"easy42/internal/config"

	"github.com/google/uuid"
)

// sshKeysSyncTimeout bounds the on-demand SSH key collection command.
const sshKeysSyncTimeout = 10 * time.Second

// setupAgentSSHKeysListeners registers SSH key related hooks on the agent hub.
//
// Key collection is driven by telemetry (agents report their public keys on
// every report, like the /etc/hosts block hash). On registration the server
// fetches the keys immediately and re-pushes the desired key files, so a node
// that just came online converges without waiting for a sync.
func (m *Manager) setupAgentSSHKeysListeners() {
	if m.agentHub == nil {
		return
	}

	m.agentHub.OnRegister(func(conn *agent.AgentConnection) {
		go m.handleAgentSSHKeysRegister(conn)
	})

	m.agentHub.OnTelemetry(func(nodeName string, t *agentpb.TelemetryReport) {
		if t == nil {
			return
		}
		if len(t.SshKeys) == 0 && !config.SSHKeySupportedByAgent(m.agentHub.GetAgentVersion(nodeName)) {
			return
		}
		keys := sshPublicKeysFromProto(t.SshKeys)
		m.agentHub.SetSSHKeys(nodeName, keys)
		if len(keys) > 0 {
			m.syncSSHKeysForNode(nodeName)
		}
	})
}

// sshPublicKeysFromProto converts protobuf keys into config keys.
func sshPublicKeysFromProto(keys []*agentpb.SshPublicKey) []config.SSHPublicKey {
	res := make([]config.SSHPublicKey, 0, len(keys))
	for _, k := range keys {
		if k == nil {
			continue
		}
		res = append(res, config.SSHPublicKey{
			Algorithm: k.Algorithm,
			Key:       k.Key,
			Comment:   k.Comment,
			Path:      k.Path,
			Kind:      k.Kind,
		})
	}
	return res
}

// sshKeysToProto converts a list of keys into the protobuf representation.
func sshKeysToProto(keys []config.SSHPublicKey) []*agentpb.SshPublicKey {
	res := make([]*agentpb.SshPublicKey, 0, len(keys))
	for _, k := range keys {
		k = k.Normalize()
		if !k.IsValid() {
			continue
		}
		res = append(res, &agentpb.SshPublicKey{
			Algorithm: k.Algorithm,
			Key:       k.Key,
			Comment:   k.Comment,
			Path:      k.Path,
			Kind:      k.Kind,
		})
	}
	return res
}

func (m *Manager) handleAgentSSHKeysRegister(conn *agent.AgentConnection) {
	nodeName := conn.NodeName()
	if !config.SSHKeySupportedByAgent(conn.AgentVersion()) {
		return
	}
	// Older keys are kept if the fetch fails (e.g. agent busy), so an existing
	// desired state is not dropped because of a transient error.
	if keys, hashes, err := m.FetchNodeSSHKeys(nodeName); err == nil {
		m.agentHub.SetSSHKeys(nodeName, keys)
		m.agentHub.SetSSHKeyFileHashes(nodeName, hashes)
	}
	m.syncSSHKeysForNode(nodeName)
}

// FetchNodeSSHKeys asks a connected agent to collect its root SSH public keys.
// It returns the keys and the hashes of the Easy42 blocks currently present in
// the managed key files.
func (m *Manager) FetchNodeSSHKeys(nodeName string) ([]config.SSHPublicKey, map[string]string, error) {
	if m.agentHub == nil {
		return nil, nil, fmt.Errorf("agent hub not available")
	}
	if !m.agentHub.IsConnected(nodeName) {
		return nil, nil, fmt.Errorf("node agent '%s' is not connected", nodeName)
	}
	ctx, cancel := context.WithTimeout(context.Background(), sshKeysSyncTimeout)
	defer cancel()

	resp, err := m.agentHub.SendCommand(ctx, nodeName, &agentpb.CommandRequest{
		Command: &agentpb.CommandRequest_FetchSshKeys{
			FetchSshKeys: &agentpb.FetchSshKeysCmd{},
		},
	})
	if err != nil {
		return nil, nil, err
	}
	if !resp.Success {
		return nil, nil, fmt.Errorf("%s", firstNonEmpty(resp.Stderr, resp.Stdout, "agent failed to collect SSH keys"))
	}

	var result struct {
		SSHKeys []struct {
			Algorithm string `json:"algorithm"`
			Key       string `json:"key"`
			Comment   string `json:"comment"`
			Path      string `json:"path"`
			Kind      string `json:"kind"`
		} `json:"ssh_keys"`
		SSHKeysEnabled     *bool  `json:"ssh_keys_enabled"`
		KnownHostsHash     string `json:"known_hosts_hash"`
		AuthorizedKeysHash string `json:"authorized_keys_hash"`
	}
	if err := json.Unmarshal([]byte(resp.Stdout), &result); err != nil {
		return nil, nil, fmt.Errorf("invalid SSH keys response: %w", err)
	}

	keys := make([]config.SSHPublicKey, 0, len(result.SSHKeys))
	for _, k := range result.SSHKeys {
		keys = append(keys, config.SSHPublicKey{
			Algorithm: k.Algorithm,
			Key:       k.Key,
			Comment:   k.Comment,
			Path:      k.Path,
			Kind:      k.Kind,
		})
	}
	hashes := map[string]string{
		config.SSHKnownHostsPath:     result.KnownHostsHash,
		config.SSHAuthorizedKeysPath: result.AuthorizedKeysHash,
	}
	return keys, hashes, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// sshKeysEnabled returns whether SSH key management is available for a node:
// agent mode, a supporting agent version and app management not disabled.
func (m *Manager) sshKeysEnabled(node config.Node) bool {
	if node.IsUnmanaged() || !node.IsAgentMode() {
		return false
	}
	if m.agentHub == nil {
		return false
	}
	if m.agentHub.IsAppsDisabled(node.Name) {
		return false
	}
	version := m.agentHub.GetAgentVersion(node.Name)
	if version == "" {
		version = node.AgentVersion
	}
	return config.SSHKeySupportedByAgent(version)
}

// GetNodeSSHKeys returns the collected public keys of a node.
func (m *Manager) GetNodeSSHKeys(nodeName string) []config.SSHPublicKey {
	if m.agentHub == nil {
		return nil
	}
	return m.agentHub.GetSSHKeys(nodeName)
}

// SSHKeyNodeState describes the SSH key collection state of a single node,
// as shown in the SSH tab of the Apps page.
type SSHKeyNodeState struct {
	Name         string                `json:"name"`
	Mode         string                `json:"mode"`
	Connected    bool                  `json:"connected"`
	AgentVersion string                `json:"agent_version,omitempty"`
	Supported    bool                  `json:"supported"`
	Reason       string                `json:"reason,omitempty"`
	Keys         []config.SSHPublicKey `json:"keys"`
	FileHashes   map[string]string     `json:"file_hashes,omitempty"`
}

// GetSSHKeysOverview returns the collection state of every managed node.
func (m *Manager) GetSSHKeysOverview() []SSHKeyNodeState {
	m.mu.RLock()
	cfg := m.store.Get()
	m.mu.RUnlock()
	if cfg == nil {
		return []SSHKeyNodeState{}
	}

	res := make([]SSHKeyNodeState, 0, len(cfg.Nodes))
	for _, node := range cfg.Nodes {
		if node.IsUnmanaged() {
			continue
		}
		// Keys must always be a JSON array (never null) so every client can
		// iterate over it unconditionally, including for unsupported nodes.
		state := SSHKeyNodeState{
			Name:       node.Name,
			Mode:       node.Mode,
			Keys:       []config.SSHPublicKey{},
			FileHashes: map[string]string{},
		}
		if m.agentHub != nil {
			state.Connected = m.agentHub.IsConnected(node.Name)
			state.AgentVersion = m.agentHub.GetAgentVersion(node.Name)
			if state.AgentVersion == "" {
				state.AgentVersion = node.AgentVersion
			}
		}
		if state.AgentVersion == "" {
			state.AgentVersion = node.AgentVersion
		}

		switch {
		case !node.IsAgentMode():
			state.Reason = "SSH key management requires agent mode"
		case m.agentHub != nil && m.agentHub.IsAppsDisabled(node.Name):
			state.Reason = "Disabled on this node (disable_apps = true)"
		case !config.SSHKeySupportedByAgent(state.AgentVersion):
			state.Reason = fmt.Sprintf("Agent version %s does not support SSH key management (requires %s+)", displayAgentVersion(state.AgentVersion), config.MinAgentVersionForSSHKeys)
		default:
			state.Supported = true
			state.Keys = m.GetNodeSSHKeys(node.Name)
			if state.Keys == nil {
				state.Keys = []config.SSHPublicKey{}
			}
			if m.agentHub != nil {
				for path, hash := range m.agentHub.GetSSHKeyFileHashes(node.Name) {
					state.FileHashes[path] = hash
				}
			}
		}
		res = append(res, state)
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Name < res[j].Name })
	return res
}

func displayAgentVersion(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return v
}

// plannedSSHKeyFile is the desired state of one managed SSH key file.
type plannedSSHKeyFile struct {
	Path   string
	Block  string
	Hash   string
	Keys   []config.SSHPublicKey
	RuleID []string
}

// planSSHKeyFilesForNode resolves the enabled SSH key rules for a destination
// node and returns the desired Easy42 block per managed file.
//
// Only source nodes with SSH key management enabled take part, and only nodes
// that match the destination selector are targets. A node is never its own
// source (its own key in its own authorized_keys is useless and a known_hosts
// entry for itself is not needed, the ssh client trusts localhost implicitly).
func (m *Manager) planSSHKeyFilesForNode(cfg *config.Config, dstNode config.Node) map[string]plannedSSHKeyFile {
	result := make(map[string]plannedSSHKeyFile)
	if cfg == nil {
		return result
	}

	type agg struct {
		keys  []config.SSHPublicKey
		rules []string
	}
	collected := make(map[string]*agg)

	for _, rule := range cfg.SSHKeyRules {
		if !rule.Enabled {
			continue
		}
		if !config.AppTargetsNode(config.App{Selector: rule.DstSelector}, &dstNode, dstNode.Name) {
			continue
		}
		for _, src := range cfg.Nodes {
			if src.Name == dstNode.Name {
				continue
			}
			// External peers (DN42/IXP) have no agent and can never publish keys.
			// They match an "all nodes" selector but are not a gap, so they are
			// neither sources nor "skipped" — listing 25 of them on every action
			// buried the sync dialog in noise.
			if src.IsUnmanaged() {
				continue
			}
			if !config.AppTargetsNode(config.App{Selector: rule.SrcSelector}, &src, src.Name) {
				continue
			}
			if !m.sshKeysEnabled(src) {
				// A node that is simply not set up for SSH key management (SSH
				// mode, older agent, apps disabled) is excluded by configuration;
				// it is not a temporary gap worth explaining on every sync.
				continue
			}
			keys := m.GetNodeSSHKeys(src.Name)
			if len(keys) == 0 {
				// Agent-mode node with key management available but no keys yet.
				// It simply contributes nothing; the next sync picks its keys up
				// automatically once the agent reports them.
				continue
			}
			keys = withSSHKeyHostnames(keys, src, cfg.DNS.BaseDomain)
			for _, path := range rule.TargetFiles() {
				a := collected[path]
				if a == nil {
					a = &agg{}
					collected[path] = a
				}
				a.keys = append(a.keys, keys...)
				a.rules = append(a.rules, rule.ID)
			}
		}
	}

	for path, a := range collected {
		// A file with no keys and no reason to manage it stays out of the plan:
		// an empty block only carries skip notes and would otherwise look like a
		// rule that targets the node.
		if len(a.keys) == 0 {
			continue
		}
		authorized := config.IsAuthorizedKeysPath(path)
		block := config.BuildSSHKeysBlock(a.keys, authorized)
		result[path] = plannedSSHKeyFile{
			Path:   path,
			Block:  block,
			Hash:   config.HashSSHKeysBlock(block),
			Keys:   a.keys,
			RuleID: a.rules,
		}
	}
	return result
}

// withSSHKeyHostnames returns the keys annotated with the host names the owning
// node is reachable under. known_hosts entries need the host list in front of
// the key (`[marker] hostnames keytype base64-key [comment]`); authorized_keys
// entries ignore it. The input keys are not modified.
func withSSHKeyHostnames(keys []config.SSHPublicKey, node config.Node, baseDomain string) []config.SSHPublicKey {
	hostnames := config.NodeKnownHostsHostnames(node, baseDomain)
	res := make([]config.SSHPublicKey, 0, len(keys))
	for _, k := range keys {
		if k.IsHostKey() {
			k.Hostnames = hostnames
		} else {
			k.Hostnames = nil
		}
		res = append(res, k)
	}
	return res
}

// sshRuleTargetsNode reports whether any enabled SSH key rule targets a node.
func (m *Manager) sshRuleTargetsNode(cfg *config.Config, node config.Node) bool {
	for _, rule := range cfg.SSHKeyRules {
		if !rule.Enabled {
			continue
		}
		if config.AppTargetsNode(config.App{Selector: rule.DstSelector}, &node, node.Name) {
			return true
		}
	}
	return false
}

// planSSHKeyActions appends the SSH key file sync actions for a node.
func (m *Manager) planSSHKeyActions(node config.Node, currentState *config.NetworkState, actions []config.SyncAction) []config.SyncAction {
	m.mu.RLock()
	cfg := m.store.Get()
	m.mu.RUnlock()
	if cfg == nil {
		return actions
	}

	var applied map[string]string
	if currentState != nil {
		if stNode, ok := currentState.Nodes[node.Name]; ok && len(stNode.SSHKeyFileHashes) > 0 {
			applied = stNode.SSHKeyFileHashes
		}
	}

	desired := map[string]plannedSSHKeyFile{}
	if node.IsAgentMode() && m.sshKeysEnabled(node) {
		desired = m.planSSHKeyFilesForNode(cfg, node)
	}

	// A node that is no longer managed (mode switched to SSH, agent too old,
	// app management disabled, node removed from every rule, ...) but still has
	// an Easy42 block recorded from a previous sync must have it removed.
	if len(desired) == 0 {
		if len(applied) == 0 || m.agentHub == nil || !m.agentHub.IsConnected(node.Name) {
			return actions
		}
		if m.agentHub.IsAppsDisabled(node.Name) {
			// The node opted out of SSH key management; nothing was written
			// after that point, so leave the files untouched.
			return actions
		}
		paths := make([]string, 0, len(applied))
		for path := range applied {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			label := "known_hosts"
			if config.IsAuthorizedKeysPath(path) {
				label = "authorized_keys"
			}
			actions = append(actions, config.SyncAction{
				NodeName:    node.Name,
				Host:        node.Host,
				Type:        config.ActionCleanupSSHKeys,
				Interface:   "ssh_" + label,
				TargetFile:  path,
				FileContent: config.BuildSSHKeysBlock(nil, config.IsAuthorizedKeysPath(path)),
				Description: fmt.Sprintf("Remove Easy42 SSH keys from %s on %s", path, node.Name),
				NeedsApply:  true,
				Status:      "pending",
				DiffStatus:  "delete",
			})
		}
		return actions
	}

	paths := make([]string, 0, len(desired))
	for path := range desired {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	appliedHashes := map[string]string{}
	for k, v := range applied {
		appliedHashes[k] = v
	}
	live := map[string]string{}
	if m.agentHub != nil {
		live = m.agentHub.GetSSHKeyFileHashes(node.Name)
	}

	for _, path := range paths {
		d := desired[path]
		needsApply := true
		status := "pending"
		diffStatus := "create"

		appliedHash := appliedHashes[path]
		liveHash := live[path]
		if appliedHash != "" || liveHash != "" {
			diffStatus = "update"
		}
		if appliedHash != "" && appliedHash == d.Hash {
			if liveHash == "" || liveHash == d.Hash {
				needsApply = false
				status = "synced"
				diffStatus = "synced"
			}
		}

		label := "known_hosts"
		if config.IsAuthorizedKeysPath(path) {
			label = "authorized_keys"
		}
		// The description stays short and stable: it is shown on every row of the
		// sync dialog, and appending the list of source nodes that contributed
		// nothing turned every key rotation into an unreadable wall of text. The
		// diff below already shows exactly which keys changed.
		desc := fmt.Sprintf("Update SSH %s (%s) on %s", label, path, node.Name)
		actions = append(actions, config.SyncAction{
			NodeName:    node.Name,
			Host:        node.Host,
			Type:        config.ActionSyncSSHKeys,
			Interface:   "ssh_" + label,
			TargetFile:  path,
			FileContent: d.Block,
			Description: desc,
			NeedsApply:  needsApply,
			Status:      status,
			DiffStatus:  diffStatus,
		})
	}
	return actions
}

// executeSSHKeySyncAction applies one SSH key file sync action on a node agent.
// It returns false when the action type is not handled (it is a no-op then).
func (m *Manager) executeSSHKeySyncAction(act config.SyncAction, res *config.SyncResult) bool {
	isCleanup := act.Type == config.ActionCleanupSSHKeys
	if act.Type != config.ActionSyncSSHKeys && !isCleanup {
		return false
	}

	hash := config.HashSSHKeysBlock(act.FileContent)

	if m.agentHub == nil || !m.agentHub.IsConnected(act.NodeName) {
		res.Success = false
		res.Error = fmt.Sprintf("node agent '%s' is not connected", act.NodeName)
		return true
	}
	if m.agentHub.IsAppsDisabled(act.NodeName) {
		res.Success = false
		res.Error = fmt.Sprintf("SSH key management is disabled on node '%s' (disable_apps = true in agent.json)", act.NodeName)
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var keys []config.SSHPublicKey
	if !isCleanup {
		keys = m.sshSourceKeysFor(act.NodeName)
	}
	cmdResp, err := m.agentHub.SendCommand(ctx, act.NodeName, &agentpb.CommandRequest{
		Command: &agentpb.CommandRequest_SyncAction{
			SyncAction: &agentpb.SyncActionCmd{
				ActionType:    "sync_ssh_keys",
				InterfaceName: act.Interface,
				TargetFile:    act.TargetFile,
				FileContent:   []byte(act.FileContent),
				FileMode:      0644,
				Hosts:         nil,
				Description:   act.Description,
				DiffStatus:    act.DiffStatus,
				SshKeys:       sshKeysToProto(keys),
			},
		},
	})
	if err == nil && cmdResp != nil && (cmdResp.Success || !strings.Contains(cmdResp.Stderr, "unknown command")) {
		if !cmdResp.Success {
			res.Success = false
			res.Error = firstNonEmpty(cmdResp.Stderr, cmdResp.Stdout)
			return true
		}
		res.Success = true
		res.Output = cmdResp.Stdout
		m.recordSSHKeyApplied(act.NodeName, act.Host, act.TargetFile, hash)
		if isCleanup {
			// The node no longer manages this file: forget the record so the
			// cleanup is not planned again.
			m.forgetSSHKeyApplied(act.NodeName, act.TargetFile)
		}
		return true
	}

	if isCleanup {
		res.Success = false
		res.Error = firstNonEmpty(cmdResp.Stderr, "agent does not support SSH key cleanup")
		return true
	}

	// Fallback for agents that predate SyncActionCmd: write the file directly.
	// Older agents cannot create the *.ssh* directory chain, so this only works
	// for nodes that already have the file's parent directory.
	fileResp, ferr := m.agentHub.SendCommand(ctx, act.NodeName, &agentpb.CommandRequest{
		Command: &agentpb.CommandRequest_ApplyConfig{
			ApplyConfig: &agentpb.ApplyConfigFile{
				Path:     act.TargetFile,
				Content:  []byte(act.FileContent),
				FileMode: 0644,
			},
		},
	})
	if ferr != nil {
		res.Success = false
		res.Error = fmt.Sprintf("agent SSH key write failed: %v", ferr)
		return true
	}
	if !fileResp.Success {
		res.Success = false
		res.Error = firstNonEmpty(fileResp.Stderr, fileResp.Stdout)
		return true
	}
	res.Success = true
	res.Output = fileResp.Stdout
	m.recordSSHKeyApplied(act.NodeName, act.Host, act.TargetFile, hash)
	return true
}

// sshSourceKeysFor returns the public keys of every node allowed to authorise
// itself against a destination node, following the enabled SSH key rules.
func (m *Manager) sshSourceKeysFor(nodeName string) []config.SSHPublicKey {
	m.mu.RLock()
	cfg := m.store.Get()
	m.mu.RUnlock()
	if cfg == nil {
		return nil
	}
	var node config.Node
	found := false
	for _, n := range cfg.Nodes {
		if n.Name == nodeName {
			node = n
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	desired := m.planSSHKeyFilesForNode(cfg, node)
	var keys []config.SSHPublicKey
	for _, d := range desired {
		keys = append(keys, d.Keys...)
	}
	return keys
}

// forgetSSHKeyApplied drops the applied record of a managed SSH key file.
func (m *Manager) forgetSSHKeyApplied(nodeName, targetFile string) {
	if m.stateStore == nil {
		return
	}
	_ = m.stateStore.RemoveSSHKeyState(nodeName, targetFile)
	if m.agentHub != nil {
		m.agentHub.SetSSHKeyFileHash(nodeName, targetFile, "")
	}
}

func (m *Manager) recordSSHKeyApplied(nodeName, host, targetFile, hash string) {
	if m.stateStore == nil {
		return
	}
	_ = m.stateStore.UpdateSSHKeyState(nodeName, host, targetFile, hash, time.Now())
	if m.agentHub != nil {
		m.agentHub.SetSSHKeyFileHash(nodeName, targetFile, hash)
	}
}

// syncSSHKeysForNode pushes the desired SSH key files to a node agent when its
// reported state diverges from the applied state. This mirrors the /etc/hosts
// auto-remediation: it only pushes a state that was already applied through a
// sync, so pending (unapplied) changes are never silently deployed.
func (m *Manager) syncSSHKeysForNode(nodeName string) {
	if m.agentHub == nil || !m.agentHub.IsConnected(nodeName) || m.stateStore == nil {
		return
	}
	node := m.FindNode(nodeName)
	if node == nil || !m.sshKeysEnabled(*node) {
		return
	}

	m.mu.RLock()
	cfg := m.store.Get()
	m.mu.RUnlock()
	if cfg == nil {
		return
	}
	desired := m.planSSHKeyFilesForNode(cfg, *node)
	if len(desired) == 0 {
		return
	}

	st := m.stateStore.Get()
	stNode, ok := st.Nodes[nodeName]
	if !ok {
		return
	}
	live := m.agentHub.GetSSHKeyFileHashes(nodeName)

	// Push at most one file per call to keep agent sessions responsive.
	for path, d := range desired {
		appliedHash, applied := stNode.SSHKeyFileHashes[path]
		if !applied || appliedHash != d.Hash {
			continue
		}
		if live[path] == d.Hash {
			continue
		}
		label := "known_hosts"
		if config.IsAuthorizedKeysPath(path) {
			label = "authorized_keys"
		}
		act := config.SyncAction{
			NodeName:    nodeName,
			Host:        node.Host,
			Type:        config.ActionSyncSSHKeys,
			Interface:   "ssh_" + label,
			TargetFile:  path,
			FileContent: d.Block,
			Description: fmt.Sprintf("Update SSH %s (%s) on %s", label, path, nodeName),
			DiffStatus:  "synced",
		}
		res := &config.SyncResult{NodeName: nodeName, Action: act.Description}
		if !m.executeSSHKeySyncAction(act, res) {
			continue
		}
		if !res.Success {
			fmt.Printf("[SSHKeys] Auto-remediation failed on %s: %s\n", nodeName, res.Error)
		}
		return
	}
}

// SyncSSHKeysToNodes pushes up-to-date SSH key files to all eligible nodes.
func (m *Manager) SyncSSHKeysToNodes() {
	if m.agentHub == nil {
		return
	}
	m.mu.RLock()
	cfg := m.store.Get()
	m.mu.RUnlock()
	if cfg == nil {
		return
	}
	var names []string
	for _, n := range cfg.Nodes {
		if m.sshRuleTargetsNode(cfg, n) && m.sshKeysEnabled(n) {
			names = append(names, n.Name)
		}
	}
	for _, name := range names {
		m.syncSSHKeysForNode(name)
	}
}

// ---------------------------------------------------------------- Rule CRUD

// GetSSHKeyRules returns the configured SSH key rules sorted by creation time.
func (m *Manager) GetSSHKeyRules() []config.SSHKeyRule {
	cfg := m.store.Get()
	if cfg == nil {
		return []config.SSHKeyRule{}
	}
	rules := make([]config.SSHKeyRule, len(cfg.SSHKeyRules))
	copy(rules, cfg.SSHKeyRules)
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].CreatedAt.Before(rules[j].CreatedAt) })
	if rules == nil {
		return []config.SSHKeyRule{}
	}
	return rules
}

// GetSSHKeyRule returns a single rule by ID.
func (m *Manager) GetSSHKeyRule(id string) (*config.SSHKeyRule, error) {
	for _, r := range m.GetSSHKeyRules() {
		if r.ID == id {
			cp := r
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("SSH key rule '%s' not found", id)
}

// SaveSSHKeyRule creates or updates an SSH key rule.
func (m *Manager) SaveSSHKeyRule(rule config.SSHKeyRule) (config.SSHKeyRule, error) {
	if err := config.ValidateSSHKeyRule(rule); err != nil {
		return rule, err
	}
	rule.Type = rule.NormalizedType()
	rule.SrcSelector = normalizeSelectorMap(rule.SrcSelector)
	rule.DstSelector = normalizeSelectorMap(rule.DstSelector)

	cfg, err := m.store.Load()
	if err != nil {
		return rule, fmt.Errorf("failed to load config: %w", err)
	}

	now := time.Now()
	found := false
	for i, existing := range cfg.SSHKeyRules {
		if existing.ID == rule.ID {
			rule.CreatedAt = existing.CreatedAt
			rule.UpdatedAt = now
			cfg.SSHKeyRules[i] = rule
			found = true
			break
		}
	}
	if !found {
		rule.ID = "sshrule_" + uuid.New().String()[:8]
		rule.CreatedAt = now
		rule.UpdatedAt = now
		cfg.SSHKeyRules = append(cfg.SSHKeyRules, rule)
	}

	if err := m.store.Save(cfg); err != nil {
		return rule, fmt.Errorf("failed to save config: %w", err)
	}
	return rule, nil
}

// normalizeSelectorMap trims selector keys/values and returns nil for empty
// selectors (which match no node) so the persisted JSON stays clean.
func normalizeSelectorMap(selector map[string]string) map[string]string {
	if len(selector) == 0 {
		return nil
	}
	res := make(map[string]string, len(selector))
	for k, v := range selector {
		res[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return res
}

// DeleteSSHKeyRule removes an SSH key rule by ID.
func (m *Manager) DeleteSSHKeyRule(id string) error {
	cfg, err := m.store.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	updated := make([]config.SSHKeyRule, 0, len(cfg.SSHKeyRules))
	found := false
	for _, r := range cfg.SSHKeyRules {
		if r.ID == id {
			found = true
			continue
		}
		updated = append(updated, r)
	}
	if !found {
		return fmt.Errorf("SSH key rule '%s' not found", id)
	}
	cfg.SSHKeyRules = updated
	if err := m.store.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	return nil
}

// ToggleSSHKeyRule flips the enabled state of an SSH key rule.
func (m *Manager) ToggleSSHKeyRule(id string) (config.SSHKeyRule, error) {
	cfg, err := m.store.Load()
	if err != nil {
		return config.SSHKeyRule{}, fmt.Errorf("failed to load config: %w", err)
	}
	for i, r := range cfg.SSHKeyRules {
		if r.ID == id {
			cfg.SSHKeyRules[i].Enabled = !cfg.SSHKeyRules[i].Enabled
			cfg.SSHKeyRules[i].UpdatedAt = time.Now()
			if err := m.store.Save(cfg); err != nil {
				return config.SSHKeyRule{}, fmt.Errorf("failed to save config: %w", err)
			}
			return cfg.SSHKeyRules[i], nil
		}
	}
	return config.SSHKeyRule{}, fmt.Errorf("SSH key rule '%s' not found", id)
}

// renameSSHKeyRuleRefsLocked rewrites node names inside SSH key rule selectors
// after a node rename. Caller holds m.mu.
func renameSSHKeyRuleRefsLocked(cfg *config.Config, oldName, newName string) {
	for i := range cfg.SSHKeyRules {
		for _, selector := range []map[string]string{cfg.SSHKeyRules[i].SrcSelector, cfg.SSHKeyRules[i].DstSelector} {
			for key, value := range selector {
				if !strings.EqualFold(strings.TrimSpace(key), "name") {
					continue
				}
				parts := strings.Split(value, ",")
				changed := false
				for j, part := range parts {
					if strings.TrimSpace(part) == oldName {
						parts[j] = newName
						changed = true
					}
				}
				if changed {
					selector[key] = strings.Join(parts, ",")
				}
			}
		}
	}
}

// pruneSSHKeyRulesForNodeLocked removes renamed/deleted nodes from rule
// selectors and drops rules whose destination selector becomes empty. Caller
// holds m.mu.
func pruneSSHKeyRulesForNodeLocked(cfg *config.Config, removedName string) {
	kept := make([]config.SSHKeyRule, 0, len(cfg.SSHKeyRules))
	for _, rule := range cfg.SSHKeyRules {
		for _, selector := range []map[string]string{rule.SrcSelector, rule.DstSelector} {
			for key, value := range selector {
				if !strings.EqualFold(strings.TrimSpace(key), "name") {
					continue
				}
				var keptParts []string
				for _, part := range strings.Split(value, ",") {
					trimmed := strings.TrimSpace(part)
					if trimmed == "" || trimmed == removedName {
						continue
					}
					keptParts = append(keptParts, trimmed)
				}
				if len(keptParts) == 0 {
					delete(selector, key)
				} else {
					selector[key] = strings.Join(keptParts, ",")
				}
			}
		}
		if len(rule.DstSelector) == 0 {
			continue
		}
		kept = append(kept, rule)
	}
	cfg.SSHKeyRules = kept
}

// SSHKeysSummary is a small aggregate used by the SSH tab header.
type SSHKeysSummary struct {
	NodesWithKeys int `json:"nodes_with_keys"`
	TotalKeys     int `json:"total_keys"`
	RuleCount     int `json:"rule_count"`
	EnabledRules  int `json:"enabled_rules"`
}

// GetSSHKeysSummary returns aggregate counters for the SSH tab.
func (m *Manager) GetSSHKeysSummary() SSHKeysSummary {
	overview := m.GetSSHKeysOverview()
	var summary SSHKeysSummary
	for _, n := range overview {
		if len(n.Keys) > 0 {
			summary.NodesWithKeys++
		}
		summary.TotalKeys += len(n.Keys)
	}
	rules := m.GetSSHKeyRules()
	summary.RuleCount = len(rules)
	for _, r := range rules {
		if r.Enabled {
			summary.EnabledRules++
		}
	}
	return summary
}
