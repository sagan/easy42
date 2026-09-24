package engine

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"easy42/internal/compiler"
	"easy42/internal/config"
	"easy42/internal/crypto"
	"easy42/internal/dns"
	"easy42/internal/roa"
	"easy42/internal/ssh"
)

var (
	ErrNodeNotFound     = errors.New("node not found")
	ErrNodeAlreadyExist = errors.New("node with this name or IP already exists")
	ErrLinkAlreadyExist = errors.New("link between these nodes already exists")
	ErrLinkNotFound     = errors.New("link not found")
)

// Manager orchestrates all operations across config, crypto, ssh, and sync
type Manager struct {
	mu          sync.RWMutex
	store       *config.Store
	stateStore  *config.StateStore
	vault       *crypto.KeyVault
	pool        *ssh.ClientPool
	roaManager  *roa.Manager
	statuses    map[string]*config.NodeStatus
	lastSync    time.Time
	lastResults []config.SyncResult
}

// NewManager creates a new Manager instance
func NewManager(store *config.Store) *Manager {
	stateStore := config.NewStateStore(store.DataDir())
	_, _ = stateStore.Load()
	roaManager := roa.NewManager(filepath.Join(store.DataDir(), "cache", "roa"))
	return &Manager{
		store:       store,
		stateStore:  stateStore,
		vault:       crypto.NewKeyVault(),
		pool:        ssh.NewClientPool(),
		roaManager:  roaManager,
		statuses:    make(map[string]*config.NodeStatus),
		lastResults: make([]config.SyncResult, 0),
	}
}

// ROAManager returns the underlying ROA cache manager
func (m *Manager) ROAManager() *roa.Manager {
	return m.roaManager
}

// Store returns the underlying store
func (m *Manager) Store() *config.Store {
	return m.store
}

// StateStore returns the underlying state store
func (m *Manager) StateStore() *config.StateStore {
	return m.stateStore
}

// GetNetworkState returns the current recorded state
func (m *Manager) GetNetworkState() *config.NetworkState {
	return m.stateStore.Get()
}

// Vault returns the KeyVault
func (m *Manager) Vault() *crypto.KeyVault {
	return m.vault
}

// Unlock unlocks the master key vault using the user password
func (m *Manager) Unlock(password string) error {
	cfg := m.store.Get()
	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			return err
		}
	}

	valid, err := crypto.VerifyPassword(password, cfg.PasswordHash)
	if err != nil || !valid {
		return errors.New("invalid password")
	}

	dek, err := crypto.DecryptDEK(password, cfg.EncryptedDEK)
	if err != nil {
		return fmt.Errorf("failed to decrypt DEK: %w", err)
	}

	return m.vault.Unlock(dek)
}

// Lock locks the vault
func (m *Manager) Lock() {
	m.vault.Lock()
}

// ChangePassword verifies oldPassword, re-encrypts the DEK with newPassword, updates config.json, and unlocks the vault
func (m *Manager) ChangePassword(oldPassword, newPassword string) error {
	newPassword = strings.TrimSpace(newPassword)
	if len(newPassword) < 6 {
		return errors.New("new password must be at least 6 characters")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			return err
		}
	}

	valid, err := crypto.VerifyPassword(oldPassword, cfg.PasswordHash)
	if err != nil || !valid {
		return errors.New("current password is incorrect")
	}

	dek, err := crypto.DecryptDEK(oldPassword, cfg.EncryptedDEK)
	if err != nil {
		return fmt.Errorf("failed to decrypt DEK: %w", err)
	}

	newHash, err := crypto.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	newEncryptedDEK, err := crypto.EncryptDEK(newPassword, dek)
	if err != nil {
		return fmt.Errorf("failed to re-encrypt DEK: %w", err)
	}

	cfgCopy := *cfg
	cfgCopy.PasswordHash = newHash
	cfgCopy.EncryptedDEK = newEncryptedDEK

	if err := m.store.Save(&cfgCopy); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Update vault with DEK
	_ = m.vault.Unlock(dek)

	return nil
}

// ResetSessionSecret regenerates session_secret in config.json and locks the vault, terminating all sessions
func (m *Manager) ResetSessionSecret() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			return err
		}
	}

	newSecret, err := crypto.GenerateSessionSecret()
	if err != nil {
		return fmt.Errorf("failed to generate session secret: %w", err)
	}

	cfgCopy := *cfg
	cfgCopy.SessionSecret = newSecret

	if err := m.store.Save(&cfgCopy); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	m.vault.Lock()
	return nil
}

// IsUnlocked checks if the vault is unlocked
func (m *Manager) IsUnlocked() bool {
	return m.vault.IsUnlocked()
}

// GetNodes returns all configured nodes
func (m *Manager) GetNodes() []config.Node {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.store.Get()
	if cfg == nil {
		return nil
	}
	res := make([]config.Node, len(cfg.Nodes))
	copy(res, cfg.Nodes)
	return res
}

// FindNode finds a node by name
func (m *Manager) FindNode(name string) *config.Node {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.store.Get()
	if cfg == nil {
		return nil
	}
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == name {
			cp := cfg.Nodes[i]
			return &cp
		}
	}
	return nil
}

// GetNode finds a node by name (alias to FindNode)
func (m *Manager) GetNode(name string) *config.Node {
	return m.FindNode(name)
}

// AddNode adds a new node to the topology
func (m *Manager) AddNode(node config.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	node.Name = strings.TrimSpace(node.Name)
	if node.IsExternal {
		if len(node.Name) == 0 || len(node.Name) > 10 {
			return errors.New("external peer name must be between 1 and 10 characters")
		}
	} else {
		if len(node.Name) == 0 || len(node.Name) > 11 {
			return errors.New("node name must be between 1 and 11 characters")
		}
	}
	if !node.IsExternal && (node.Host == "" || node.IP == "") {
		return errors.New("host and IP are required for managed nodes")
	}

	node.IP = strings.TrimSpace(node.IP)
	node.IP6 = strings.TrimSpace(node.IP6)
	if idx := strings.Index(node.IP6, "/"); idx != -1 {
		node.IP6 = strings.TrimSpace(node.IP6[:idx])
	}

	cfg := m.store.Get()
	for _, existing := range cfg.Nodes {
		if strings.EqualFold(existing.Name, node.Name) {
			return fmt.Errorf("node with name %s already exists", node.Name)
		}
		if node.IP != "" && !node.IsExternal && !existing.IsExternal && existing.IP == node.IP {
			return fmt.Errorf("node with IP %s already exists (%s)", node.IP, existing.Name)
		}
		if node.IP6 != "" && !node.IsExternal && !existing.IsExternal && existing.IP6 != "" && existing.IP6 == node.IP6 {
			return fmt.Errorf("node with IPv6 %s already exists (%s)", node.IP6, existing.Name)
		}
	}

	if !node.IsExternal {
		// Ensure "none" entrypoint exists at the end
		var nones []config.Entrypoint
		var normals []config.Entrypoint
		for _, ep := range node.Entrypoints {
			if ep.IsNone() {
				nones = append(nones, ep)
			} else {
				normals = append(normals, ep)
			}
		}
		if len(nones) == 0 {
			nones = append(nones, config.Entrypoint{
				IP:   "",
				Tags: []string{"nat"},
			})
		}
		node.Entrypoints = append(normals, nones...)
	}

	if node.Table <= 0 {
		node.Table = 254
	}

	node.ModifiedAt = time.Now().UTC()
	cfg.Nodes = append(cfg.Nodes, node)
	if err := m.store.Save(cfg); err != nil {
		return err
	}
	m.triggerDNSUpdateForNode(cfg.DNS, node, "")
	return nil
}

// UpdateNode updates an existing node
func (m *Manager) UpdateNode(name string, updated config.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	idx := -1
	for i, n := range cfg.Nodes {
		if n.Name == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ErrNodeNotFound
	}

	updated.Name = strings.TrimSpace(updated.Name)
	if updated.IsExternal {
		if len(updated.Name) == 0 || len(updated.Name) > 10 {
			return errors.New("external peer name must be between 1 and 10 characters")
		}
	} else {
		if len(updated.Name) == 0 || len(updated.Name) > 11 {
			return errors.New("node name must be between 1 and 11 characters")
		}
	}

	// Check name uniqueness if changed
	if updated.Name != name {
		for _, n := range cfg.Nodes {
			if n.Name == updated.Name {
				return ErrNodeAlreadyExist
			}
		}
	}

	updated.IP = strings.TrimSpace(updated.IP)
	updated.IP6 = strings.TrimSpace(updated.IP6)
	if idx := strings.Index(updated.IP6, "/"); idx != -1 {
		updated.IP6 = strings.TrimSpace(updated.IP6[:idx])
	}
	if updated.IP6 != "" && !updated.IsExternal {
		for _, existing := range cfg.Nodes {
			if existing.Name != name && !existing.IsExternal && existing.IP6 != "" && existing.IP6 == updated.IP6 {
				return fmt.Errorf("node with IPv6 %s already exists (%s)", updated.IP6, existing.Name)
			}
		}
	}

	if !updated.IsExternal {
		// Ensure "none" endpoint exists at the end
		var nones []config.Entrypoint
		var normals []config.Entrypoint
		for _, ep := range updated.Entrypoints {
			if ep.IsNone() {
				nones = append(nones, ep)
			} else {
				normals = append(normals, ep)
			}
		}
		if len(nones) == 0 {
			nones = append(nones, config.Entrypoint{
				IP:   "",
				Tags: []string{"nat"},
			})
		}
		updated.Entrypoints = append(normals, nones...)
	}

	if updated.Table <= 0 {
		updated.Table = 254
	}

	now := time.Now().UTC()
	updated.ModifiedAt = now

	oldNode := cfg.Nodes[idx]
	nameChanged := updated.Name != name
	ipChanged := updated.IP != oldNode.IP || updated.IP6 != oldNode.IP6
	if updated.Name != name {
		m.applyNodeRenameLocked(cfg, name, updated.Name, updated.IsExternal, now)
	}

	if updated.IP != oldNode.IP {
		if newAddr, err := compiler.DeriveIPv6LinkLocal(updated.IP); err == nil {
			for i := range cfg.Links {
				if cfg.Links[i].From.Name == updated.Name {
					cfg.Links[i].From.Address = newAddr
					cfg.Links[i].ModifiedAt = now
				}
				if cfg.Links[i].To.Name == updated.Name {
					cfg.Links[i].To.Address = newAddr
					cfg.Links[i].ModifiedAt = now
				}
			}
		}
		for i := range cfg.Links {
			if cfg.Links[i].AssignIPv4 {
				isFrom := cfg.Links[i].From.Name == updated.Name
				isTo := cfg.Links[i].To.Name == updated.Name
				if isFrom || isTo {
					otherName := cfg.Links[i].To.Name
					if isTo {
						otherName = cfg.Links[i].From.Name
					}
					var otherNode *config.Node
					for _, n := range cfg.Nodes {
						if n.Name == otherName {
							otherNode = &n
							break
						}
					}
					fromMainIP := updated.IP
					if fromMainIP == "" {
						fromMainIP = updated.ExternalIP
					}
					toMainIP := ""
					if otherNode != nil {
						toMainIP = otherNode.IP
						if toMainIP == "" {
							toMainIP = otherNode.ExternalIP
						}
					}
					if isTo {
						fromMainIP, toMainIP = toMainIP, fromMainIP
					}

					s := compiler.ExtractInterfaceSuffix(cfg.Links[i].From.Interface, cfg.Links[i].To.Name, otherNode != nil && otherNode.IsExternal)
					if s == "" && otherNode != nil {
						s = compiler.ExtractInterfaceSuffix(cfg.Links[i].To.Interface, cfg.Links[i].From.Name, updated.IsExternal)
					}
					idx := compiler.LinkIndexFromSuffix(s)

					if fromMainIP != "" && toMainIP != "" {
						if a1, a2, err := compiler.DeriveIPv4LinkLocal(fromMainIP, toMainIP, idx); err == nil {
							cfg.Links[i].From.Address4 = a1
							cfg.Links[i].To.Address4 = a2
							cfg.Links[i].ModifiedAt = now
						}
					}
				}
			}
		}
	}

	// Preserve coordinates if not specified in updated node
	if updated.X == nil && oldNode.X != nil {
		updated.X = oldNode.X
	}
	if updated.Y == nil && oldNode.Y != nil {
		updated.Y = oldNode.Y
	}

	cfg.Nodes[idx] = updated

	// Re-resolve endpoints for connected links
	for i := range cfg.Links {
		if cfg.Links[i].From.Name == updated.Name || cfg.Links[i].To.Name == updated.Name {
			var fromN, toN *config.Node
			if cfg.Links[i].From.Name == updated.Name {
				fromN = &cfg.Nodes[idx]
				for j := range cfg.Nodes {
					if cfg.Nodes[j].Name == cfg.Links[i].To.Name {
						toN = &cfg.Nodes[j]
						break
					}
				}
			} else {
				toN = &cfg.Nodes[idx]
				for j := range cfg.Nodes {
					if cfg.Nodes[j].Name == cfg.Links[i].From.Name {
						fromN = &cfg.Nodes[j]
						break
					}
				}
			}
			if fromN != nil && toN != nil {
				fromEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(fromN, toN, nil, cfg.Links[i].To.ListenPort)
				toEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(toN, fromN, nil, cfg.Links[i].From.ListenPort)
				cfg.Links[i].From.Endpoint = fromEP
				cfg.Links[i].To.Endpoint = toEP
				if fromEP != "" && !fromN.IsExternal {
					cfg.Links[i].From.PersistentKeepalive = 25
				} else {
					cfg.Links[i].From.PersistentKeepalive = 0
				}
				if toEP != "" && !toN.IsExternal {
					cfg.Links[i].To.PersistentKeepalive = 25
				} else {
					cfg.Links[i].To.PersistentKeepalive = 0
				}
				cfg.Links[i].ModifiedAt = now
			}
		}
	}

	if err := m.store.Save(cfg); err != nil {
		return err
	}

	if nameChanged {
		m.triggerDNSUpdateForNode(cfg.DNS, updated, name)
	} else if ipChanged {
		m.triggerDNSUpdateForNode(cfg.DNS, updated, "")
	}

	return nil
}

// applyNodeRenameLocked updates all references to a node name across config, links, interface names, and state.
// Caller must hold m.mu Lock.
func (m *Manager) applyNodeRenameLocked(cfg *config.Config, oldName, newName string, isExternal bool, now time.Time) {
	oldIfaceInternal := compiler.GetInterfaceName(oldName, false)
	oldIfaceExternal := compiler.GetInterfaceName(oldName, true)
	newIface := compiler.GetInterfaceName(newName, isExternal)
	newIfaceInternal := compiler.GetInterfaceName(newName, false)
	newIfaceExternal := compiler.GetInterfaceName(newName, true)

	replaceIface := func(iface string) string {
		if iface == oldIfaceInternal || iface == "wg42"+oldName {
			return newIfaceInternal
		}
		if iface == oldIfaceExternal || iface == "wg42-"+oldName {
			return newIfaceExternal
		}
		if strings.HasPrefix(iface, "wg42-") && strings.TrimPrefix(iface, "wg42-") == oldName {
			return newIfaceExternal
		}
		if strings.HasPrefix(iface, "wg42") && strings.TrimPrefix(iface, "wg42") == oldName {
			return newIfaceInternal
		}
		if strings.Contains(iface, oldName) {
			return strings.ReplaceAll(iface, oldName, newName)
		}
		return iface
	}

	// Update node name and interface in Nodes list
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == oldName {
			cfg.Nodes[i].Name = newName
			cfg.Nodes[i].ModifiedAt = now
		}
		if cfg.Nodes[i].Interface != "" {
			replaced := replaceIface(cfg.Nodes[i].Interface)
			if replaced != cfg.Nodes[i].Interface {
				cfg.Nodes[i].Interface = replaced
				cfg.Nodes[i].ModifiedAt = now
			}
		}
		for tIdx := range cfg.Nodes[i].Tags {
			if cfg.Nodes[i].Tags[tIdx] == oldName {
				cfg.Nodes[i].Tags[tIdx] = newName
				cfg.Nodes[i].ModifiedAt = now
			}
		}
	}

	// Update all links: From.Name, To.Name, From.Interface, To.Interface, and Tags
	for i := range cfg.Links {
		// Update endpoint names
		if cfg.Links[i].From.Name == oldName {
			cfg.Links[i].From.Name = newName
			cfg.Links[i].ModifiedAt = now
		}
		if cfg.Links[i].To.Name == oldName {
			cfg.Links[i].To.Name = newName
			cfg.Links[i].ModifiedAt = now
		}

		// Update From.Interface: if To is the renamed node, use new peer interface name
		if cfg.Links[i].To.Name == newName {
			s := compiler.ExtractInterfaceSuffix(cfg.Links[i].From.Interface, oldName, isExternal)
			cfg.Links[i].From.Interface = compiler.GetInterfaceNameWithSuffix(newName, s, isExternal)
			cfg.Links[i].ModifiedAt = now
		} else {
			newFromIface := replaceIface(cfg.Links[i].From.Interface)
			if newFromIface != cfg.Links[i].From.Interface {
				cfg.Links[i].From.Interface = newFromIface
				cfg.Links[i].ModifiedAt = now
			}
		}

		// Update To.Interface: if From is the renamed node, use new peer interface name
		if cfg.Links[i].From.Name == newName {
			s := compiler.ExtractInterfaceSuffix(cfg.Links[i].To.Interface, oldName, isExternal)
			cfg.Links[i].To.Interface = compiler.GetInterfaceNameWithSuffix(newName, s, isExternal)
			cfg.Links[i].ModifiedAt = now
		} else {
			newToIface := replaceIface(cfg.Links[i].To.Interface)
			if newToIface != cfg.Links[i].To.Interface {
				cfg.Links[i].To.Interface = newToIface
				cfg.Links[i].ModifiedAt = now
			}
		}

		// Update link tags if matching oldName
		for tIdx := range cfg.Links[i].Tags {
			if cfg.Links[i].Tags[tIdx] == oldName {
				cfg.Links[i].Tags[tIdx] = newName
				cfg.Links[i].ModifiedAt = now
			}
		}
	}

	// Update block nodes if matching oldName
	for bIdx := range cfg.Blocks {
		for nIdx := range cfg.Blocks[bIdx].Nodes {
			if cfg.Blocks[bIdx].Nodes[nIdx] == oldName {
				cfg.Blocks[bIdx].Nodes[nIdx] = newName
			}
		}
	}

	// Update runtime statuses map
	if st, ok := m.statuses[oldName]; ok {
		st.Name = newName
		m.statuses[newName] = st
		delete(m.statuses, oldName)
	}

	// Update state store if present
	if m.stateStore != nil {
		oldIfaces := []string{
			oldIfaceInternal,
			oldIfaceExternal,
			"wg42" + oldName,
			"wg42-" + oldName,
		}
		_ = m.stateStore.RenameNode(oldName, newName, oldIfaces, newIface)
	}
}

// RenameNode renames an existing node across configuration, links, interfaces, and state
func (m *Manager) RenameNode(oldName, newName string) (*config.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)

	if oldName == "" {
		return nil, errors.New("current node name cannot be empty")
	}
	if newName == "" {
		return nil, errors.New("new node name cannot be empty")
	}

	cfg := m.store.Get()
	if cfg == nil {
		return nil, ErrNodeNotFound
	}

	idx := -1
	for i, n := range cfg.Nodes {
		if n.Name == oldName {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, ErrNodeNotFound
	}

	targetNode := cfg.Nodes[idx]

	if newName == oldName {
		cp := targetNode
		return &cp, nil
	}

	if targetNode.IsExternal {
		if len(newName) == 0 || len(newName) > 10 {
			return nil, errors.New("external peer name must be between 1 and 10 characters")
		}
	} else {
		if len(newName) == 0 || len(newName) > 11 {
			return nil, errors.New("node name must be between 1 and 11 characters")
		}
	}

	// Validate allowed characters (letters, digits, hyphen, underscore)
	for _, ch := range newName {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return nil, errors.New("node name may only contain alphanumeric characters, hyphens, and underscores")
		}
	}

	// Check if new name already exists
	for _, n := range cfg.Nodes {
		if strings.EqualFold(n.Name, newName) {
			return nil, fmt.Errorf("node with name %s already exists", newName)
		}
	}

	now := time.Now().UTC()
	m.applyNodeRenameLocked(cfg, oldName, newName, targetNode.IsExternal, now)

	// Re-resolve endpoints for connected links
	for i := range cfg.Links {
		if cfg.Links[i].From.Name == newName || cfg.Links[i].To.Name == newName {
			var fromN, toN *config.Node
			for j := range cfg.Nodes {
				if cfg.Nodes[j].Name == cfg.Links[i].From.Name {
					fromN = &cfg.Nodes[j]
				}
				if cfg.Nodes[j].Name == cfg.Links[i].To.Name {
					toN = &cfg.Nodes[j]
				}
			}
			if fromN != nil && toN != nil {
				fromEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(fromN, toN, nil, cfg.Links[i].To.ListenPort)
				toEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(toN, fromN, nil, cfg.Links[i].From.ListenPort)
				cfg.Links[i].From.Endpoint = fromEP
				cfg.Links[i].To.Endpoint = toEP
				if fromEP != "" && !fromN.IsExternal {
					cfg.Links[i].From.PersistentKeepalive = 25
				} else {
					cfg.Links[i].From.PersistentKeepalive = 0
				}
				if toEP != "" && !toN.IsExternal {
					cfg.Links[i].To.PersistentKeepalive = 25
				} else {
					cfg.Links[i].To.PersistentKeepalive = 0
				}
				cfg.Links[i].ModifiedAt = now
			}
		}
	}

	if err := m.store.Save(cfg); err != nil {
		return nil, err
	}

	for _, n := range cfg.Nodes {
		if n.Name == newName {
			cp := n
			m.triggerDNSUpdateForNode(cfg.DNS, cp, oldName)
			return &cp, nil
		}
	}
	retNode := cfg.Nodes[idx]
	m.triggerDNSUpdateForNode(cfg.DNS, retNode, oldName)
	return &retNode, nil
}

// UpdateNodePosition updates the graph coordinates (x, y) of a node
func (m *Manager) UpdateNodePosition(name string, x float64, y float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	if cfg == nil {
		return ErrNodeNotFound
	}
	idx := -1
	for i, n := range cfg.Nodes {
		if n.Name == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ErrNodeNotFound
	}

	cfg.Nodes[idx].X = &x
	cfg.Nodes[idx].Y = &y
	cfg.Nodes[idx].ModifiedAt = time.Now().UTC()

	return m.store.Save(cfg)
}

// DeleteNode removes a node and all its connected links
func (m *Manager) DeleteNode(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	var deletedHost string
	newNodes := make([]config.Node, 0)
	for _, n := range cfg.Nodes {
		if n.Name != name {
			newNodes = append(newNodes, n)
		} else {
			deletedHost = n.Host
		}
	}
	cfg.Nodes = newNodes

	// Remove links connected to this node
	newLinks := make([]config.Link, 0)
	for _, l := range cfg.Links {
		if l.From.Name != name && l.To.Name != name {
			newLinks = append(newLinks, l)
		}
	}
	cfg.Links = newLinks

	// Remove deleted node from any blocks
	for bIdx := range cfg.Blocks {
		var updatedBlockNodes []string
		for _, nodeName := range cfg.Blocks[bIdx].Nodes {
			if nodeName != name {
				updatedBlockNodes = append(updatedBlockNodes, nodeName)
			}
		}
		cfg.Blocks[bIdx].Nodes = updatedBlockNodes
	}

	// Purge in-memory statuses and sync results
	delete(m.statuses, name)

	newResults := make([]config.SyncResult, 0, len(m.lastResults))
	for _, res := range m.lastResults {
		if res.NodeName != name {
			newResults = append(newResults, res)
		}
	}
	m.lastResults = newResults

	// Close SSH connections if host is no longer used by any other node
	if deletedHost != "" {
		hostStillUsed := false
		for _, n := range newNodes {
			if n.Host == deletedHost {
				hostStillUsed = true
				break
			}
		}
		if !hostStillUsed && m.pool != nil {
			m.pool.CloseHost(deletedHost)
		}
	}

	_ = m.stateStore.RemoveNode(name)

	if err := m.store.Save(cfg); err != nil {
		return err
	}
	m.triggerDNSDeleteForNode(cfg.DNS, name)
	return nil
}

// GetLinks returns all links with resolved endpoints populated
func (m *Manager) GetLinks() []config.Link {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.store.Get()
	if cfg == nil {
		return nil
	}
	nodeMap := make(map[string]*config.Node, len(cfg.Nodes))
	for i := range cfg.Nodes {
		nodeMap[cfg.Nodes[i].Name] = &cfg.Nodes[i]
	}

	res := make([]config.Link, len(cfg.Links))
	for i := range cfg.Links {
		res[i] = cfg.Links[i]
		fromNode := nodeMap[res[i].From.Name]
		toNode := nodeMap[res[i].To.Name]
		res[i].From.ResolvedEndpoint = compiler.ResolveLinkEndpoint(fromNode, toNode, &res[i].From, &res[i].To)
		res[i].To.ResolvedEndpoint = compiler.ResolveLinkEndpoint(toNode, fromNode, &res[i].To, &res[i].From)
		if toNode != nil && toNode.IsExternal {
			res[i].To.Endpoint = res[i].To.ResolvedEndpoint
		}
		if fromNode != nil && fromNode.IsExternal {
			res[i].From.Endpoint = res[i].From.ResolvedEndpoint
		}
	}
	return res
}

// AddLink creates a new WireGuard link between two nodes with keypairs and parameters.
// buildLink creates and configures a Link between two nodes with keypairs, addresses, ports, endpoints, and MTU.
// Must be called with m.mu held and vault unlocked.
func (m *Manager) buildLink(cfg *config.Config, n1, n2 *config.Node, listenPort1, listenPort2 int, tags []string, customMTU1, customMTU2 int, linkParams ...*config.Link) (*config.Link, error) {
	// Lexicographical ordering: from.Name < to.Name
	fromNode, toNode := n1, n2
	fromPort, toPort := listenPort1, listenPort2
	fromCustomMTU, toCustomMTU := customMTU1, customMTU2
	if strings.Compare(fromNode.Name, toNode.Name) > 0 {
		fromNode, toNode = toNode, fromNode
		fromPort, toPort = toPort, fromPort
		fromCustomMTU, toCustomMTU = toCustomMTU, fromCustomMTU
	}

	if fromNode.IsExternal && toNode.IsExternal {
		return nil, errors.New("cannot link two external nodes")
	}

	var customFromEnd, customToEnd *config.LinkEnd
	linkType := config.LinkTypeWireGuard
	assignIPv4 := false
	if len(linkParams) > 0 && linkParams[0] != nil {
		lp := linkParams[0]
		assignIPv4 = lp.AssignIPv4
		if lp.Type != "" {
			linkType = lp.Type
		}
		if lp.From.Name == fromNode.Name {
			customFromEnd = &lp.From
			customToEnd = &lp.To
		} else if lp.To.Name == fromNode.Name {
			customFromEnd = &lp.To
			customToEnd = &lp.From
		}
	}
	if customFromEnd != nil && customFromEnd.Type != "" {
		linkType = customFromEnd.Type
	} else if customToEnd != nil && customToEnd.Type != "" {
		linkType = customToEnd.Type
	}
	isManual := (linkType == config.LinkTypeManual)

	var encPrivFrom, pubKeyFrom string
	var encPrivTo, pubKeyTo string

	if !isManual {
		if fromNode.IsExternal {
			if customFromEnd != nil && customFromEnd.PublicKey != "" {
				pubKeyFrom = customFromEnd.PublicKey
			}
		} else {
			kpFrom, err := crypto.GenerateWgKeyPair()
			if err != nil {
				return nil, fmt.Errorf("failed to generate keypair for %s: %w", fromNode.Name, err)
			}
			encPriv, err := m.vault.EncryptField(kpFrom.PrivateKey)
			if err != nil {
				return nil, err
			}
			encPrivFrom = encPriv
			pubKeyFrom = kpFrom.PublicKey
		}

		if toNode.IsExternal {
			if customToEnd != nil && customToEnd.PublicKey != "" {
				pubKeyTo = customToEnd.PublicKey
			}
		} else {
			kpTo, err := crypto.GenerateWgKeyPair()
			if err != nil {
				return nil, fmt.Errorf("failed to generate keypair for %s: %w", toNode.Name, err)
			}
			encPriv, err := m.vault.EncryptField(kpTo.PrivateKey)
			if err != nil {
				return nil, err
			}
			encPrivTo = encPriv
			pubKeyTo = kpTo.PublicKey
		}
	}

	fromAddr := ""
	toAddr := ""
	fromNeighborAddr := ""
	toNeighborAddr := ""
	fromIface := ""
	toIface := ""

	if customFromEnd != nil {
		fromIface = strings.TrimSpace(customFromEnd.Interface)
		fromAddr = strings.TrimSpace(customFromEnd.Address)
		fromNeighborAddr = strings.TrimSpace(customFromEnd.NeighborAddress)
	}
	if customToEnd != nil {
		toIface = strings.TrimSpace(customToEnd.Interface)
		toAddr = strings.TrimSpace(customToEnd.Address)
		toNeighborAddr = strings.TrimSpace(customToEnd.NeighborAddress)
	}

	if isManual {
		if fromIface == "" {
			return nil, fmt.Errorf("interface name is required for node %s in manual link", fromNode.Name)
		}
		if toIface == "" {
			return nil, fmt.Errorf("interface name is required for node %s in manual link", toNode.Name)
		}
		// Cross-fill local and neighbor IPs if one is supplied
		if fromAddr == "" && toNeighborAddr != "" {
			fromAddr = toNeighborAddr
		}
		if toAddr == "" && fromNeighborAddr != "" {
			toAddr = fromNeighborAddr
		}
		if fromNeighborAddr == "" && toAddr != "" {
			fromNeighborAddr = toAddr
		}
		if toNeighborAddr == "" && fromAddr != "" {
			toNeighborAddr = fromAddr
		}
		if fromAddr == "" {
			return nil, fmt.Errorf("local IP address is required for node %s in manual link", fromNode.Name)
		}
		if toAddr == "" {
			return nil, fmt.Errorf("local IP address is required for node %s in manual link", toNode.Name)
		}
	} else {
		if fromAddr == "" && fromNode.IP != "" {
			fromAddr, _ = compiler.DeriveIPv6LinkLocal(fromNode.IP)
		}
		if fromAddr == "" {
			fromAddr = "fe80::1/64"
		}

		if toAddr == "" && toNode.IP != "" {
			toAddr, _ = compiler.DeriveIPv6LinkLocal(toNode.IP)
		}
		if toAddr == "" {
			toAddr = "fe80::2/64"
		}
	}

	usedIfacesFrom := make(map[string]bool)
	usedIfacesTo := make(map[string]bool)
	for _, l := range cfg.Links {
		if l.From.Name == fromNode.Name {
			usedIfacesFrom[l.From.Interface] = true
		}
		if l.To.Name == fromNode.Name {
			usedIfacesFrom[l.To.Interface] = true
		}
		if l.From.Name == toNode.Name {
			usedIfacesTo[l.From.Interface] = true
		}
		if l.To.Name == toNode.Name {
			usedIfacesTo[l.To.Interface] = true
		}
	}

	var linkIndex int
	if !isManual {
		// Find the lowest deterministic index k (0, 1, 2...) such that default interface names don't collide
		var candFromIface, candToIface string
		for k := 0; ; k++ {
			s := ""
			if k > 0 {
				s = fmt.Sprintf("%d", k)
			}
			candFrom := compiler.GetInterfaceNameWithSuffix(toNode.Name, s, toNode.IsExternal)
			candTo := compiler.GetInterfaceNameWithSuffix(fromNode.Name, s, fromNode.IsExternal)
			if !usedIfacesFrom[candFrom] && !usedIfacesTo[candTo] {
				candFromIface = candFrom
				candToIface = candTo
				linkIndex = k
				break
			}
		}

		if fromIface == "" {
			fromIface = candFromIface
		}
		if toIface == "" {
			toIface = candToIface
		}
	}

	if usedIfacesFrom[fromIface] {
		return nil, fmt.Errorf("interface %s already exists on node %s", fromIface, fromNode.Name)
	}
	if usedIfacesTo[toIface] {
		return nil, fmt.Errorf("interface %s already exists on node %s", toIface, toNode.Name)
	}

	fromEP := ""
	toEP := ""
	fromKeepalive := 0
	toKeepalive := 0
	var epTo, epFrom *config.Entrypoint

	if isManual {
		if customFromEnd != nil {
			fromPort = customFromEnd.ListenPort
			fromEP = customFromEnd.Endpoint
			fromKeepalive = customFromEnd.PersistentKeepalive
		} else {
			fromPort = 0
		}
		if customToEnd != nil {
			toPort = customToEnd.ListenPort
			toEP = customToEnd.Endpoint
			toKeepalive = customToEnd.PersistentKeepalive
		} else {
			toPort = 0
		}
	} else {
		baseFromPort := 0
		if toNode.IP != "" {
			baseFromPort = compiler.DerivePortFromIP(toNode.IP)
		} else {
			baseFromPort = 51820
		}
		baseToPort := 0
		if fromNode.IP != "" {
			baseToPort = compiler.DerivePortFromIP(fromNode.IP)
		} else {
			baseToPort = 51820
		}

		if customFromEnd != nil && customFromEnd.ListenPort > 0 {
			fromPort = customFromEnd.ListenPort
		}
		if customToEnd != nil && customToEnd.ListenPort > 0 {
			toPort = customToEnd.ListenPort
		}
		if fromPort == 0 && !fromNode.IsExternal {
			fromPort = baseFromPort + linkIndex
		}
		if toPort == 0 && !toNode.IsExternal {
			toPort = baseToPort + linkIndex
		}

		if customFromEnd != nil && customFromEnd.Endpoint != "" {
			fromEP = customFromEnd.Endpoint
		}
		if customToEnd != nil && customToEnd.Endpoint != "" {
			toEP = customToEnd.Endpoint
		}

		if fromNode.IsExternal {
			// fromNode is external, connecting to toNode (managed internal)
			fromEP, _, epTo = compiler.ResolvePeerEndpointWithEntrypoint(fromNode, toNode, nil, toPort)
			if toEP == "" && customFromEnd != nil && customFromEnd.Endpoint != "" {
				toEP = customFromEnd.Endpoint
			}
		} else if toNode.IsExternal {
			// toNode is external, connecting to fromNode (managed internal)
			toEP, _, epFrom = compiler.ResolvePeerEndpointWithEntrypoint(toNode, fromNode, nil, fromPort)
			if fromEP == "" && customToEnd != nil && customToEnd.Endpoint != "" {
				fromEP = customToEnd.Endpoint
			}
		} else {
			fromEP, _, epTo = compiler.ResolvePeerEndpointWithEntrypoint(fromNode, toNode, nil, toPort)
			toEP, _, epFrom = compiler.ResolvePeerEndpointWithEntrypoint(toNode, fromNode, nil, fromPort)
		}

		if fromEP != "" && !fromNode.IsExternal {
			fromKeepalive = 25
		}
		if toEP != "" && !toNode.IsExternal {
			toKeepalive = 25
		}
		if customFromEnd != nil && customFromEnd.PersistentKeepalive > 0 {
			fromKeepalive = customFromEnd.PersistentKeepalive
		}
		if customToEnd != nil && customToEnd.PersistentKeepalive > 0 {
			toKeepalive = customToEnd.PersistentKeepalive
		}
	}

	// Determine LinkEnd MTU: used entrypoint mtu minus 80 (wg overhead)
	resolveUsedMTU := func(primaryEP, fallbackEP *config.Entrypoint, selfNode, peerNode *config.Node) int {
		var ep *config.Entrypoint
		if primaryEP != nil && !primaryEP.IsNone() {
			ep = primaryEP
		} else if fallbackEP != nil && !fallbackEP.IsNone() {
			ep = fallbackEP
		} else {
			if selfNode != nil {
				for i := range selfNode.Entrypoints {
					if !selfNode.Entrypoints[i].IsNone() {
						ep = &selfNode.Entrypoints[i]
						break
					}
				}
			}
			if ep == nil && peerNode != nil {
				for i := range peerNode.Entrypoints {
					if !peerNode.Entrypoints[i].IsNone() {
						ep = &peerNode.Entrypoints[i]
						break
					}
				}
			}
		}

		baseMTU := 1500
		if ep != nil && ep.MTU > 0 {
			baseMTU = ep.MTU
		}
		return baseMTU - 80
	}

	var fromMTU, toMTU int
	if isManual {
		fromMTU = 1500
		toMTU = 1500
	} else {
		fromMTU = resolveUsedMTU(epTo, epFrom, fromNode, toNode)
		toMTU = resolveUsedMTU(epFrom, epTo, toNode, fromNode)
	}
	if fromCustomMTU > 0 {
		fromMTU = fromCustomMTU
	}
	if toCustomMTU > 0 {
		toMTU = toCustomMTU
	}
	if customFromEnd != nil && customFromEnd.MTU > 0 {
		fromMTU = customFromEnd.MTU
	}
	if customToEnd != nil && customToEnd.MTU > 0 {
		toMTU = customToEnd.MTU
	}

	fromUseIP := false
	if customFromEnd != nil {
		fromUseIP = customFromEnd.UseIp
	}
	toUseIP := false
	if customToEnd != nil {
		toUseIP = customToEnd.UseIp
	}

	fromPolicy := ""
	if customFromEnd != nil && customFromEnd.Policy != "" {
		fromPolicy = customFromEnd.Policy
	} else if toNode.IsExternal {
		fromPolicy = config.PolicyDN42
	} else {
		fromPolicy = config.PolicyDefault
	}

	toPolicy := ""
	if customToEnd != nil && customToEnd.Policy != "" {
		toPolicy = customToEnd.Policy
	} else if fromNode.IsExternal {
		toPolicy = config.PolicyDN42
	} else {
		toPolicy = config.PolicyDefault
	}

	fromCost := 0
	if customFromEnd != nil && customFromEnd.Cost != 0 {
		fromCost = customFromEnd.Cost
	}
	toCost := 0
	if customToEnd != nil && customToEnd.Cost != 0 {
		toCost = customToEnd.Cost
	}

	fromFwmark := ""
	if customFromEnd != nil && strings.TrimSpace(customFromEnd.Fwmark) != "" {
		fromFwmark = strings.TrimSpace(customFromEnd.Fwmark)
	}
	toFwmark := ""
	if customToEnd != nil && strings.TrimSpace(customToEnd.Fwmark) != "" {
		toFwmark = strings.TrimSpace(customToEnd.Fwmark)
	}

	fromMark := ""
	if customFromEnd != nil && strings.TrimSpace(customFromEnd.Mark) != "" {
		fromMark = strings.TrimSpace(customFromEnd.Mark)
	}
	toMark := ""
	if customToEnd != nil && strings.TrimSpace(customToEnd.Mark) != "" {
		toMark = strings.TrimSpace(customToEnd.Mark)
	}

	var fromPref, toPref *int
	if customFromEnd != nil && customFromEnd.Preference != nil {
		fromPref = customFromEnd.Preference
	}
	if customToEnd != nil && customToEnd.Preference != nil {
		toPref = customToEnd.Preference
	}

	fromRoutingPolicy := ""
	if customFromEnd != nil && customFromEnd.RoutingPolicy != "" {
		fromRoutingPolicy = config.NormalizeRoutingPolicy(customFromEnd.RoutingPolicy)
	}
	toRoutingPolicy := ""
	if customToEnd != nil && customToEnd.RoutingPolicy != "" {
		toRoutingPolicy = config.NormalizeRoutingPolicy(customToEnd.RoutingPolicy)
	}

	fromNote := ""
	if customFromEnd != nil {
		fromNote = customFromEnd.Note
	}
	toNote := ""
	if customToEnd != nil {
		toNote = customToEnd.Note
	}

	var fromAddr4, toAddr4 string
	if assignIPv4 {
		fromMainIP := fromNode.IP
		if fromMainIP == "" {
			fromMainIP = fromNode.ExternalIP
		}
		toMainIP := toNode.IP
		if toMainIP == "" {
			toMainIP = toNode.ExternalIP
		}
		if fromMainIP != "" && toMainIP != "" {
			if a1, a2, err := compiler.DeriveIPv4LinkLocal(fromMainIP, toMainIP, linkIndex); err == nil {
				fromAddr4 = a1
				toAddr4 = a2
			}
		}
	}
	if customFromEnd != nil && customFromEnd.Address4 != "" {
		fromAddr4 = customFromEnd.Address4
	}
	if customToEnd != nil && customToEnd.Address4 != "" {
		toAddr4 = customToEnd.Address4
	}

	link := &config.Link{
		Type:       linkType,
		AssignIPv4: assignIPv4,
		From: config.LinkEnd{
			Name:                fromNode.Name,
			Type:                linkType,
			Interface:           fromIface,
			Address:             fromAddr,
			Address4:            fromAddr4,
			NeighborAddress:     fromNeighborAddr,
			ListenPort:          fromPort,
			Endpoint:            fromEP,
			PrivateKey:          encPrivFrom,
			PublicKey:           pubKeyFrom,
			PersistentKeepalive: fromKeepalive,
			MTU:                 fromMTU,
			UseIp:               fromUseIP,
			Policy:              fromPolicy,
			RoutingPolicy:       fromRoutingPolicy,
			Cost:                fromCost,
			Fwmark:              fromFwmark,
			Preference:          fromPref,
			Mark:                fromMark,
			Note:                fromNote,
		},
		To: config.LinkEnd{
			Name:                toNode.Name,
			Type:                linkType,
			Interface:           toIface,
			Address:             toAddr,
			Address4:            toAddr4,
			NeighborAddress:     toNeighborAddr,
			ListenPort:          toPort,
			Endpoint:            toEP,
			PrivateKey:          encPrivTo,
			PublicKey:           pubKeyTo,
			PersistentKeepalive: toKeepalive,
			MTU:                 toMTU,
			UseIp:               toUseIP,
			Policy:              toPolicy,
			RoutingPolicy:       toRoutingPolicy,
			Cost:                toCost,
			Fwmark:              toFwmark,
			Preference:          toPref,
			Mark:                toMark,
			Note:                toNote,
		},
		Tags:       tags,
		ModifiedAt: time.Now().UTC(),
	}

	if !isManual {
		link.From.ResolvedEndpoint = compiler.ResolveLinkEndpoint(fromNode, toNode, &link.From, &link.To)
		link.To.ResolvedEndpoint = compiler.ResolveLinkEndpoint(toNode, fromNode, &link.To, &link.From)
	}

	return link, nil
}

// AddLink adds a new WireGuard link between two nodes.
// Optional customMTU can specify [fromMTU, toMTU] (relative to node1Name, node2Name).
func (m *Manager) AddLink(node1Name, node2Name string, listenPort1, listenPort2 int, tags []string, customMTU ...int) (*config.Link, error) {
	var fromEnd, toEnd *config.LinkEnd
	if listenPort1 > 0 {
		fromEnd = &config.LinkEnd{Name: node1Name, ListenPort: listenPort1}
	}
	if listenPort2 > 0 {
		toEnd = &config.LinkEnd{Name: node2Name, ListenPort: listenPort2}
	}
	return m.AddLinkAdvanced(node1Name, node2Name, fromEnd, toEnd, tags, customMTU...)
}

// AddLinkAdvanced creates a link with full custom LinkEnd properties (useful for external peering)
func (m *Manager) AddLinkAdvanced(node1Name, node2Name string, fromEnd, toEnd *config.LinkEnd, tags []string, customMTU ...int) (*config.Link, error) {
	assignIPv4 := false
	if fromEnd != nil && fromEnd.Address4 != "" {
		assignIPv4 = true
	}
	if toEnd != nil && toEnd.Address4 != "" {
		assignIPv4 = true
	}
	return m.AddLinkWithOptions(node1Name, node2Name, fromEnd, toEnd, tags, assignIPv4, customMTU...)
}

// AddLinkWithOptions creates a link with full custom LinkEnd properties and link-level options (such as assignIPv4)
func (m *Manager) AddLinkWithOptions(node1Name, node2Name string, fromEnd, toEnd *config.LinkEnd, tags []string, assignIPv4 bool, customMTU ...int) (*config.Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	isManual := (fromEnd != nil && fromEnd.IsManual()) || (toEnd != nil && toEnd.IsManual())
	if !isManual && !m.vault.IsUnlocked() {
		return nil, crypto.ErrVaultLocked
	}

	cfg := m.store.Get()
	var n1, n2 *config.Node
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == node1Name {
			n1 = &cfg.Nodes[i]
		}
		if cfg.Nodes[i].Name == node2Name {
			n2 = &cfg.Nodes[i]
		}
	}
	if n1 == nil || n2 == nil {
		return nil, errors.New("one or both nodes not found")
	}

	customMTU1 := 0
	customMTU2 := 0
	if len(customMTU) >= 1 && customMTU[0] > 0 {
		customMTU1 = customMTU[0]
	}
	if len(customMTU) >= 2 && customMTU[1] > 0 {
		customMTU2 = customMTU[1]
	}

	// Lexicographical ordering: from.Name < to.Name
	fromName, toName := n1.Name, n2.Name
	if strings.Compare(fromName, toName) > 0 {
		fromName, toName = toName, fromName
	}

	var lp *config.Link
	if fromEnd != nil || toEnd != nil || assignIPv4 {
		lp = &config.Link{
			AssignIPv4: assignIPv4,
		}
		if isManual {
			lp.Type = config.LinkTypeManual
		}
		if fromEnd != nil {
			lp.From = *fromEnd
			if lp.From.Name == "" {
				lp.From.Name = node1Name
			}
			if fromEnd.Type != "" {
				lp.Type = fromEnd.Type
			}
		}
		if toEnd != nil {
			lp.To = *toEnd
			if lp.To.Name == "" {
				lp.To.Name = node2Name
			}
			if toEnd.Type != "" {
				lp.Type = toEnd.Type
			}
		}
	}

	listenPort1 := 0
	listenPort2 := 0
	if fromEnd != nil {
		listenPort1 = fromEnd.ListenPort
	}
	if toEnd != nil {
		listenPort2 = toEnd.ListenPort
	}

	link, err := m.buildLink(cfg, n1, n2, listenPort1, listenPort2, tags, customMTU1, customMTU2, lp)
	if err != nil {
		return nil, err
	}

	cfg.Links = append(cfg.Links, *link)
	if err := m.store.Save(cfg); err != nil {
		return nil, err
	}

	return link, nil
}

// CreateFullMesh creates missing links between specified nodes (or all nodes if nil/empty)
// using default ports and MTU.
func (m *Manager) CreateFullMesh(nodeNames []string) ([]*config.Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.vault.IsUnlocked() {
		return nil, crypto.ErrVaultLocked
	}

	cfg := m.store.Get()
	var targetNodes []*config.Node

	if len(nodeNames) == 0 {
		for i := range cfg.Nodes {
			if !cfg.Nodes[i].IsExternal {
				targetNodes = append(targetNodes, &cfg.Nodes[i])
			}
		}
	} else {
		nodeMap := make(map[string]*config.Node)
		for i := range cfg.Nodes {
			nodeMap[cfg.Nodes[i].Name] = &cfg.Nodes[i]
		}
		for _, name := range nodeNames {
			if n, ok := nodeMap[name]; ok {
				if !n.IsExternal {
					targetNodes = append(targetNodes, n)
				}
			}
		}
	}

	if len(targetNodes) < 2 {
		return nil, errors.New("at least two nodes are required to create a mesh")
	}

	var addedLinks []*config.Link
	now := time.Now().UTC()

	for i := 0; i < len(targetNodes); i++ {
		for j := i + 1; j < len(targetNodes); j++ {
			n1 := targetNodes[i]
			n2 := targetNodes[j]

			// Lexicographical ordering
			fromName, toName := n1.Name, n2.Name
			if strings.Compare(fromName, toName) > 0 {
				fromName, toName = toName, fromName
			}

			// Check if link already exists
			exists := false
			for _, l := range cfg.Links {
				if l.From.Name == fromName && l.To.Name == toName {
					exists = true
					break
				}
			}
			if exists {
				continue
			}

			// Generate default ports
			fromPort := compiler.DerivePortFromIP(n2.IP)
			toPort := compiler.DerivePortFromIP(n1.IP)

			link, err := m.buildLink(cfg, n1, n2, fromPort, toPort, nil, 0, 0)
			if err != nil {
				return nil, err
			}
			link.ModifiedAt = now
			cfg.Links = append(cfg.Links, *link)
			addedLinks = append(addedLinks, link)
		}
	}

	if len(addedLinks) > 0 {
		if err := m.store.Save(cfg); err != nil {
			return nil, err
		}
	}

	return addedLinks, nil
}

// UpdateLink updates parameters (listen ports, custom MTUs, tags) of an existing WireGuard link
func (m *Manager) UpdateLink(node1Name, node2Name string, listenPort1, listenPort2 int, tags []string, customMTU ...int) (*config.Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	var n1, n2 *config.Node
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == node1Name {
			n1 = &cfg.Nodes[i]
		}
		if cfg.Nodes[i].Name == node2Name {
			n2 = &cfg.Nodes[i]
		}
	}
	if n1 == nil || n2 == nil {
		return nil, errors.New("one or both nodes not found")
	}

	customMTU1 := 0
	customMTU2 := 0
	if len(customMTU) >= 1 && customMTU[0] > 0 {
		customMTU1 = customMTU[0]
	}
	if len(customMTU) >= 2 && customMTU[1] > 0 {
		customMTU2 = customMTU[1]
	}

	// Lexicographical ordering: from.Name < to.Name
	fromNode, toNode := n1, n2
	fromPort, toPort := listenPort1, listenPort2
	fromCustomMTU, toCustomMTU := customMTU1, customMTU2
	if strings.Compare(fromNode.Name, toNode.Name) > 0 {
		fromNode, toNode = toNode, fromNode
		fromPort, toPort = toPort, fromPort
		fromCustomMTU, toCustomMTU = toCustomMTU, fromCustomMTU
	}

	linkIdx := -1
	for i, l := range cfg.Links {
		if l.From.Name == fromNode.Name && l.To.Name == toNode.Name {
			linkIdx = i
			break
		}
	}
	if linkIdx == -1 {
		return nil, ErrLinkNotFound
	}

	link := &cfg.Links[linkIdx]

	if fromPort > 0 {
		link.From.ListenPort = fromPort
	}
	if toPort > 0 {
		link.To.ListenPort = toPort
	}
	if fromCustomMTU > 0 {
		link.From.MTU = fromCustomMTU
	}
	if toCustomMTU > 0 {
		link.To.MTU = toCustomMTU
	}
	if tags != nil {
		link.Tags = tags
	}

	isExternalLink := fromNode.IsExternal || toNode.IsExternal
	if !isExternalLink {
		// Re-resolve endpoints and keepalives with updated ports
		fromEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(fromNode, toNode, nil, link.To.ListenPort)
		toEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(toNode, fromNode, nil, link.From.ListenPort)
		link.From.Endpoint = fromEP
		link.To.Endpoint = toEP
		if fromEP != "" {
			link.From.PersistentKeepalive = 25
		} else {
			link.From.PersistentKeepalive = 0
		}
		if toEP != "" {
			link.To.PersistentKeepalive = 25
		} else {
			link.To.PersistentKeepalive = 0
		}
	} else if toNode.IsExternal {
		toEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(toNode, fromNode, nil, link.From.ListenPort)
		if toEP != "" {
			link.To.Endpoint = toEP
		}
	} else if fromNode.IsExternal {
		fromEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(fromNode, toNode, nil, link.To.ListenPort)
		if fromEP != "" {
			link.From.Endpoint = fromEP
		}
	}
	link.ModifiedAt = time.Now().UTC()
	link.From.ResolvedEndpoint = compiler.ResolveLinkEndpoint(fromNode, toNode, &link.From, &link.To)
	link.To.ResolvedEndpoint = compiler.ResolveLinkEndpoint(toNode, fromNode, &link.To, &link.From)

	if err := m.store.Save(cfg); err != nil {
		return nil, err
	}

	return link, nil
}

// UpdateLinkAdvanced updates any parameters of an existing link, including addresses, endpoints, and public keys
func (m *Manager) UpdateLinkAdvanced(node1Name, node2Name string, customFrom, customTo *config.LinkEnd, tags []string) (*config.Link, error) {
	return m.UpdateLinkWithOptions(node1Name, node2Name, customFrom, customTo, tags, nil)
}

// UpdateLinkWithOptions updates parameters of an existing link, including assignIPv4 option
func (m *Manager) UpdateLinkWithOptions(node1Name, node2Name string, customFrom, customTo *config.LinkEnd, tags []string, assignIPv4 *bool) (*config.Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	var n1, n2 *config.Node
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == node1Name {
			n1 = &cfg.Nodes[i]
		}
		if cfg.Nodes[i].Name == node2Name {
			n2 = &cfg.Nodes[i]
		}
	}
	if n1 == nil || n2 == nil {
		return nil, errors.New("one or both nodes not found")
	}

	fromNode, toNode := n1, n2
	fromEnd, toEnd := customFrom, customTo
	if strings.Compare(fromNode.Name, toNode.Name) > 0 {
		fromNode, toNode = toNode, fromNode
		fromEnd, toEnd = toEnd, fromEnd
	}

	target1 := ""
	target2 := ""
	if customFrom != nil && customFrom.Interface != "" {
		target1 = customFrom.Interface
	}
	if customTo != nil && customTo.Interface != "" {
		target2 = customTo.Interface
	}

	linkIdx := -1
	for i, l := range cfg.Links {
		if l.From.Name == fromNode.Name && l.To.Name == toNode.Name {
			if target1 != "" || target2 != "" {
				match := (target1 == "" || l.From.Interface == target1 || l.To.Interface == target1) &&
					(target2 == "" || l.From.Interface == target2 || l.To.Interface == target2)
				if !match {
					continue
				}
			}
			linkIdx = i
			break
		}
	}
	if linkIdx == -1 {
		return nil, ErrLinkNotFound
	}

	link := &cfg.Links[linkIdx]

	if fromEnd != nil {
		if fromEnd.Type != "" {
			link.From.Type = fromEnd.Type
		}
		if fromEnd.Interface != "" {
			link.From.Interface = fromEnd.Interface
		}
		if fromEnd.NeighborAddress != "" {
			link.From.NeighborAddress = fromEnd.NeighborAddress
		}
		if fromEnd.ListenPort > 0 {
			link.From.ListenPort = fromEnd.ListenPort
		}
		if fromEnd.Address != "" {
			link.From.Address = fromEnd.Address
		}
		if fromEnd.Endpoint != "" {
			link.From.Endpoint = fromEnd.Endpoint
		}
		if fromEnd.MTU > 0 {
			link.From.MTU = fromEnd.MTU
		}
		if fromEnd.PersistentKeepalive >= 0 {
			link.From.PersistentKeepalive = fromEnd.PersistentKeepalive
		}
		if fromNode.IsExternal && fromEnd.PublicKey != "" {
			link.From.PublicKey = fromEnd.PublicKey
		}
		link.From.UseIp = fromEnd.UseIp
		if fromEnd.Policy != "" {
			link.From.Policy = fromEnd.Policy
		}
		if fromEnd.Cost != 0 {
			link.From.Cost = fromEnd.Cost
		}
		if fromEnd.Fwmark != "" {
			link.From.Fwmark = strings.TrimSpace(fromEnd.Fwmark)
		}
		if fromEnd.Preference != nil {
			link.From.Preference = fromEnd.Preference
		}
		if fromEnd.Mark != "" {
			link.From.Mark = strings.TrimSpace(fromEnd.Mark)
		}
		if fromEnd.RoutingPolicy != "" {
			if strings.EqualFold(fromEnd.RoutingPolicy, "inherit") {
				link.From.RoutingPolicy = ""
			} else {
				link.From.RoutingPolicy = config.NormalizeRoutingPolicy(fromEnd.RoutingPolicy)
			}
		}
		if fromEnd.Note != "" {
			if fromEnd.Note == "__CLEAR__" || fromEnd.Note == "<clear>" {
				link.From.Note = ""
			} else {
				link.From.Note = fromEnd.Note
			}
		}
	}

	if toEnd != nil {
		if toEnd.Type != "" {
			link.To.Type = toEnd.Type
		}
		if toEnd.Interface != "" {
			link.To.Interface = toEnd.Interface
		}
		if toEnd.NeighborAddress != "" {
			link.To.NeighborAddress = toEnd.NeighborAddress
		}
		if toEnd.ListenPort > 0 {
			link.To.ListenPort = toEnd.ListenPort
		}
		if toEnd.Address != "" {
			link.To.Address = toEnd.Address
		}
		if toEnd.Endpoint != "" {
			link.To.Endpoint = toEnd.Endpoint
		}
		if toEnd.MTU > 0 {
			link.To.MTU = toEnd.MTU
		}
		if toEnd.PersistentKeepalive >= 0 {
			link.To.PersistentKeepalive = toEnd.PersistentKeepalive
		}
		if toNode.IsExternal && toEnd.PublicKey != "" {
			link.To.PublicKey = toEnd.PublicKey
		}
		link.To.UseIp = toEnd.UseIp
		if toEnd.Policy != "" {
			link.To.Policy = toEnd.Policy
		}
		if toEnd.Cost != 0 {
			link.To.Cost = toEnd.Cost
		}
		if toEnd.Fwmark != "" {
			link.To.Fwmark = strings.TrimSpace(toEnd.Fwmark)
		}
		if toEnd.Preference != nil {
			link.To.Preference = toEnd.Preference
		}
		if toEnd.Mark != "" {
			link.To.Mark = strings.TrimSpace(toEnd.Mark)
		}
		if toEnd.RoutingPolicy != "" {
			if strings.EqualFold(toEnd.RoutingPolicy, "inherit") {
				link.To.RoutingPolicy = ""
			} else {
				link.To.RoutingPolicy = config.NormalizeRoutingPolicy(toEnd.RoutingPolicy)
			}
		}
		if toEnd.Note != "" {
			if toEnd.Note == "__CLEAR__" || toEnd.Note == "<clear>" {
				link.To.Note = ""
			} else {
				link.To.Note = toEnd.Note
			}
		}
	}

	if link.From.IsManual() || link.To.IsManual() {
		link.Type = config.LinkTypeManual
		link.From.Type = config.LinkTypeManual
		link.To.Type = config.LinkTypeManual

		// Cross-fill missing local IP with neighbor IP if provided
		if link.From.Address == "" && link.To.NeighborAddress != "" {
			link.From.Address = link.To.NeighborAddress
		}
		if link.To.Address == "" && link.From.NeighborAddress != "" {
			link.To.Address = link.From.NeighborAddress
		}
		if link.From.NeighborAddress == "" && link.To.Address != "" {
			link.From.NeighborAddress = link.To.Address
		}
		if link.To.NeighborAddress == "" && link.From.Address != "" {
			link.To.NeighborAddress = link.From.Address
		}

		if strings.TrimSpace(link.From.Interface) == "" {
			return nil, fmt.Errorf("interface name is required for node %s in manual link", fromNode.Name)
		}
		if strings.TrimSpace(link.To.Interface) == "" {
			return nil, fmt.Errorf("interface name is required for node %s in manual link", toNode.Name)
		}
		if strings.TrimSpace(link.From.Address) == "" {
			return nil, fmt.Errorf("local IP address is required for node %s in manual link", fromNode.Name)
		}
		if strings.TrimSpace(link.To.Address) == "" {
			return nil, fmt.Errorf("local IP address is required for node %s in manual link", toNode.Name)
		}
	}

	if tags != nil {
		link.Tags = tags
	}

	if fromEnd != nil && fromEnd.Address4 != "" {
		link.From.Address4 = fromEnd.Address4
	}
	if toEnd != nil && toEnd.Address4 != "" {
		link.To.Address4 = toEnd.Address4
	}

	if assignIPv4 != nil {
		link.AssignIPv4 = *assignIPv4
		if *assignIPv4 {
			linkIndex := 0
			s := compiler.ExtractInterfaceSuffix(link.From.Interface, toNode.Name, toNode.IsExternal)
			if s == "" {
				s = compiler.ExtractInterfaceSuffix(link.To.Interface, fromNode.Name, fromNode.IsExternal)
			}
			linkIndex = compiler.LinkIndexFromSuffix(s)

			fromMainIP := fromNode.IP
			if fromMainIP == "" {
				fromMainIP = fromNode.ExternalIP
			}
			toMainIP := toNode.IP
			if toMainIP == "" {
				toMainIP = toNode.ExternalIP
			}
			if fromMainIP != "" && toMainIP != "" {
				if a1, a2, err := compiler.DeriveIPv4LinkLocal(fromMainIP, toMainIP, linkIndex); err == nil {
					if fromEnd == nil || fromEnd.Address4 == "" {
						link.From.Address4 = a1
					}
					if toEnd == nil || toEnd.Address4 == "" {
						link.To.Address4 = a2
					}
				}
			}
		} else {
			link.From.Address4 = ""
			link.To.Address4 = ""
		}
	} else if link.AssignIPv4 && (link.From.Address4 == "" || link.To.Address4 == "") {
		linkIndex := 0
		s := compiler.ExtractInterfaceSuffix(link.From.Interface, toNode.Name, toNode.IsExternal)
		if s == "" {
			s = compiler.ExtractInterfaceSuffix(link.To.Interface, fromNode.Name, fromNode.IsExternal)
		}
		linkIndex = compiler.LinkIndexFromSuffix(s)

		fromMainIP := fromNode.IP
		if fromMainIP == "" {
			fromMainIP = fromNode.ExternalIP
		}
		toMainIP := toNode.IP
		if toMainIP == "" {
			toMainIP = toNode.ExternalIP
		}
		if fromMainIP != "" && toMainIP != "" {
			if a1, a2, err := compiler.DeriveIPv4LinkLocal(fromMainIP, toMainIP, linkIndex); err == nil {
				if link.From.Address4 == "" {
					link.From.Address4 = a1
				}
				if link.To.Address4 == "" {
					link.To.Address4 = a2
				}
			}
		}
	}

	isExternalLink := fromNode.IsExternal || toNode.IsExternal
	if !isExternalLink && !link.IsManual() {
		fromEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(fromNode, toNode, nil, link.To.ListenPort)
		toEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(toNode, fromNode, nil, link.From.ListenPort)
		link.From.Endpoint = fromEP
		link.To.Endpoint = toEP
		if fromEP != "" {
			if fromEnd != nil && fromEnd.PersistentKeepalive > 0 {
				link.From.PersistentKeepalive = fromEnd.PersistentKeepalive
			} else {
				link.From.PersistentKeepalive = 25
			}
		} else {
			link.From.PersistentKeepalive = 0
		}
		if toEP != "" {
			if toEnd != nil && toEnd.PersistentKeepalive > 0 {
				link.To.PersistentKeepalive = toEnd.PersistentKeepalive
			} else {
				link.To.PersistentKeepalive = 25
			}
		} else {
			link.To.PersistentKeepalive = 0
		}
	} else if toNode.IsExternal && !link.IsManual() {
		if link.From.Endpoint == "" && toEnd != nil && toEnd.Endpoint != "" {
			link.From.Endpoint = toEnd.Endpoint
		}
		if link.From.Endpoint != "" && link.From.PersistentKeepalive == 0 {
			link.From.PersistentKeepalive = 25
		}
		toEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(toNode, fromNode, nil, link.From.ListenPort)
		if toEP != "" {
			link.To.Endpoint = toEP
		}
	} else if fromNode.IsExternal && !link.IsManual() {
		if link.To.Endpoint == "" && fromEnd != nil && fromEnd.Endpoint != "" {
			link.To.Endpoint = fromEnd.Endpoint
		}
		if link.To.Endpoint != "" && link.To.PersistentKeepalive == 0 {
			link.To.PersistentKeepalive = 25
		}
		fromEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(fromNode, toNode, nil, link.To.ListenPort)
		if fromEP != "" {
			link.From.Endpoint = fromEP
		}
	}

	link.ModifiedAt = time.Now().UTC()
	if !link.IsManual() {
		link.From.ResolvedEndpoint = compiler.ResolveLinkEndpoint(fromNode, toNode, &link.From, &link.To)
		link.To.ResolvedEndpoint = compiler.ResolveLinkEndpoint(toNode, fromNode, &link.To, &link.From)
	}
	if err := m.store.Save(cfg); err != nil {
		return nil, err
	}

	return link, nil
}

// DeleteLink removes a link between two nodes, optionally matching a specific interface
func (m *Manager) DeleteLink(node1Name, node2Name string, ifaces ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	from, to := node1Name, node2Name
	if strings.Compare(from, to) > 0 {
		from, to = to, from
	}

	matchIface := ""
	if len(ifaces) > 0 && ifaces[0] != "" {
		matchIface = strings.TrimSpace(ifaces[0])
	}

	cfg := m.store.Get()
	newLinks := make([]config.Link, 0)
	found := false
	for _, l := range cfg.Links {
		if l.From.Name == from && l.To.Name == to {
			if matchIface != "" {
				if l.From.Interface == matchIface || l.To.Interface == matchIface {
					found = true
					continue
				}
			} else {
				found = true
				continue
			}
		}
		newLinks = append(newLinks, l)
	}

	if !found {
		return ErrLinkNotFound
	}

	cfg.Links = newLinks
	return m.store.Save(cfg)
}

// ProbeHost probes a remote node via SSH
func (m *Manager) ProbeHost(host string, excludeNode ...string) (*ssh.ProbeResult, error) {
	sshClient, _, err := m.pool.GetClient(host)
	if err != nil {
		return nil, err
	}

	exclude := ""
	if len(excludeNode) > 0 && excludeNode[0] != "" {
		exclude = excludeNode[0]
	} else {
		for _, n := range m.GetNodes() {
			if strings.EqualFold(n.Host, host) {
				exclude = n.Name
				break
			}
		}
	}

	nodes := m.GetNodes()
	if exclude != "" {
		var filtered []config.Node
		for _, n := range nodes {
			if n.Name != exclude {
				filtered = append(filtered, n)
			}
		}
		nodes = filtered
	}

	return ssh.ProbeHost(sshClient, host, nodes)
}

// RefreshNodeStatus refreshes a node's live status via SSH
func (m *Manager) RefreshNodeStatus(nodeName string) (*config.NodeStatus, error) {
	node := m.FindNode(nodeName)
	if node == nil {
		return nil, ErrNodeNotFound
	}

	if node.IsExternal {
		status := &config.NodeStatus{
			Name:      node.Name,
			Host:      "external",
			LastSeen:  time.Now(),
			Connected: true,
			Hostname:  node.Name,
		}
		m.mu.Lock()
		m.statuses[nodeName] = status
		m.mu.Unlock()
		return status, nil
	}

	status := &config.NodeStatus{
		Name:      node.Name,
		Host:      node.Host,
		LastSeen:  time.Now(),
		Connected: false,
	}

	sshClient, _, err := m.pool.GetClientWithTimeout(node.Host, 4*time.Second)
	if err != nil {
		status.Error = err.Error()
		m.mu.Lock()
		m.statuses[nodeName] = status
		m.mu.Unlock()
		return status, nil
	}

	status.Connected = true
	hostname, _ := ssh.RunCommand(sshClient, "hostname")
	status.Hostname = strings.TrimSpace(hostname)

	wgStatus, err := ssh.QueryWireGuardStatus(sshClient)
	if err == nil {
		status.WgInterfaces = wgStatus
	}

	m.mu.Lock()
	m.statuses[nodeName] = status
	m.mu.Unlock()

	return status, nil
}

// RestartNodeWireGuardInterfaces restarts all WireGuard interfaces on a managed node
func (m *Manager) RestartNodeWireGuardInterfaces(nodeName string) (string, error) {
	node := m.FindNode(nodeName)
	if node == nil {
		return "", ErrNodeNotFound
	}
	if node.IsExternal {
		return "", fmt.Errorf("node %s is external and cannot be managed via SSH", nodeName)
	}
	if node.Host == "" {
		return "", fmt.Errorf("node %s has no host configured", nodeName)
	}

	sshClient, _, err := m.pool.GetClient(node.Host)
	if err != nil {
		return "", fmt.Errorf("failed to connect to host %s: %w", node.Host, err)
	}

	cmd := `sh -l -c '
for i in $( { for c in /etc/wireguard/*.conf; do [ -f "$c" ] && basename "$c" .conf; done; wg show interfaces 2>/dev/null | tr " " "\n"; } | sort -u ); do
  [ -n "$i" ] || continue
  wg-quick down "$i" 2>/dev/null || ip link del dev "$i" 2>/dev/null
  [ -f "/etc/wireguard/$i.conf" ] && wg-quick up "$i"
done
'`
	out, err := ssh.RunCommandWithTimeout(sshClient, cmd, 30*time.Second)
	if err != nil {
		return out, fmt.Errorf("failed to restart WireGuard interfaces: %w", err)
	}

	go func() {
		_, _ = m.RefreshNodeStatus(nodeName)
	}()

	return out, nil
}

// RestartNodeBird restarts the BIRD routing service on a managed node
func (m *Manager) RestartNodeBird(nodeName string) (string, error) {
	node := m.FindNode(nodeName)
	if node == nil {
		return "", ErrNodeNotFound
	}
	if node.IsExternal {
		return "", fmt.Errorf("node %s is external and cannot be managed via SSH", nodeName)
	}
	if node.Host == "" {
		return "", fmt.Errorf("node %s has no host configured", nodeName)
	}

	sshClient, _, err := m.pool.GetClient(node.Host)
	if err != nil {
		return "", fmt.Errorf("failed to connect to host %s: %w", node.Host, err)
	}

	out, err := ssh.RunCommandWithTimeout(sshClient, "service bird restart", 20*time.Second)
	if err != nil {
		return out, fmt.Errorf("failed to restart bird: %w", err)
	}

	return out, nil
}

// RestartNodeInterface restarts a specific WireGuard interface on a managed node
func (m *Manager) RestartNodeInterface(nodeName string, iface string) (string, error) {
	if iface == "" {
		return "", fmt.Errorf("interface name is required")
	}
	node := m.FindNode(nodeName)
	if node == nil {
		return "", ErrNodeNotFound
	}
	if node.IsExternal {
		return "", fmt.Errorf("node %s is external and cannot be managed via SSH", nodeName)
	}
	if node.Host == "" {
		return "", fmt.Errorf("node %s has no host configured", nodeName)
	}

	sshClient, _, err := m.pool.GetClient(node.Host)
	if err != nil {
		return "", fmt.Errorf("failed to connect to host %s: %w", node.Host, err)
	}

	cmd := fmt.Sprintf("sh -l -c 'wg-quick down %s 2>/dev/null || ip link del dev %s 2>/dev/null ; wg-quick up %s'", iface, iface, iface)
	out, err := ssh.RunCommandWithTimeout(sshClient, cmd, 20*time.Second)
	if err != nil {
		return out, fmt.Errorf("failed to restart WireGuard interface %s: %w", iface, err)
	}

	go func() {
		_, _ = m.RefreshNodeStatus(nodeName)
	}()

	return out, nil
}

// GetNodeStatuses returns cached node statuses, purging any stale entries for deleted nodes
func (m *Manager) GetNodeStatuses() map[string]config.NodeStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	activeNodes := make(map[string]bool)
	if cfg != nil {
		for _, n := range cfg.Nodes {
			activeNodes[n.Name] = true
		}
	}

	for k := range m.statuses {
		if !activeNodes[k] {
			delete(m.statuses, k)
		}
	}

	res := make(map[string]config.NodeStatus)
	for k, v := range m.statuses {
		res[k] = *v
	}
	return res
}

// PlanSync computes the sync actions required to bring remote devices up to date.
// If nodeNames are provided, only changes for those target nodes are computed.
func (m *Manager) PlanSync(nodeNames ...string) ([]config.SyncAction, error) {
	m.mu.RLock()
	if !m.vault.IsUnlocked() {
		m.mu.RUnlock()
		return nil, crypto.ErrVaultLocked
	}

	cfg := m.store.Get()
	nodeMap := make(map[string]*config.Node)
	for i := range cfg.Nodes {
		nodeMap[cfg.Nodes[i].Name] = &cfg.Nodes[i]
	}
	nodes := make([]config.Node, len(cfg.Nodes))
	copy(nodes, cfg.Nodes)
	links := make([]config.Link, len(cfg.Links))
	copy(links, cfg.Links)
	m.mu.RUnlock()

	targetMap := make(map[string]bool)
	for _, name := range nodeNames {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			targetMap[trimmed] = true
		}
	}
	isPartial := len(targetMap) > 0

	currentState := m.stateStore.Get()
	actions := make([]config.SyncAction, 0)

	// 1. Detect unused/deleted wg42* interfaces on remote devices by running `wg`
	expectedIfacesPerNode := make(map[string]map[string]bool)
	manualIfacesPerNode := make(map[string]map[string]bool)
	for _, n := range nodes {
		expectedIfacesPerNode[n.Name] = make(map[string]bool)
		manualIfacesPerNode[n.Name] = make(map[string]bool)
	}
	for _, link := range links {
		if link.IsManual() {
			manualIfacesPerNode[link.From.Name][link.From.Interface] = true
			manualIfacesPerNode[link.To.Name][link.To.Interface] = true
			continue
		}
		if em, ok := expectedIfacesPerNode[link.From.Name]; ok {
			em[link.From.Interface] = true
		}
		if em, ok := expectedIfacesPerNode[link.To.Name]; ok {
			em[link.To.Interface] = true
		}
	}

	var wg sync.WaitGroup
	var cleanActionsMu sync.Mutex
	var cleanActions []config.SyncAction

	for _, n := range nodes {
		if n.IsExternal {
			continue
		}
		if isPartial && !targetMap[n.Name] {
			continue
		}
		node := n
		wg.Add(1)
		go func(targetNode config.Node) {
			defer wg.Done()
			sshClient, _, err := m.pool.GetClientWithTimeout(targetNode.Host, 3*time.Second)
			if err != nil {
				m.mu.Lock()
				m.statuses[targetNode.Name] = &config.NodeStatus{
					Name:      targetNode.Name,
					Host:      targetNode.Host,
					LastSeen:  time.Now(),
					Connected: false,
					Error:     err.Error(),
				}
				m.mu.Unlock()
				return
			}
			runningIfaces, err := ssh.GetRunningWgInterfaces(sshClient)
			if err != nil {
				return
			}
			expected := expectedIfacesPerNode[targetNode.Name]
			manualIfs := manualIfacesPerNode[targetNode.Name]
			for _, iface := range runningIfaces {
				// We only manage wg42* prefix wireguard interfaces
				if strings.HasPrefix(iface, "wg42") {
					if !expected[iface] && !manualIfs[iface] {
						targetFile := fmt.Sprintf("/etc/wireguard/%s.conf", iface)
						cleanActionsMu.Lock()
						cleanActions = append(cleanActions, config.SyncAction{
							NodeName:    targetNode.Name,
							Host:        targetNode.Host,
							Type:        config.ActionDeleteConfig,
							Interface:   iface,
							TargetFile:  targetFile,
							Command:     fmt.Sprintf("wg-quick down %s && rm -f %s", iface, targetFile),
							Description: fmt.Sprintf("Remove deleted interface %s on %s", iface, targetNode.Name),
							FileContent: fmt.Sprintf("# Deleted interface %s is no longer in graph.\n# Action: wg-quick down %s && rm -f %s\n", iface, iface, targetFile),
							NeedsApply:  true,
							Status:      "pending",
							DiffStatus:  "delete",
						})
						cleanActionsMu.Unlock()
					}
				}
			}
		}(node)
	}
	wg.Wait()

	// Also check stateStore for recorded interfaces that are no longer expected
	for _, n := range nodes {
		if n.IsExternal {
			continue
		}
		if isPartial && !targetMap[n.Name] {
			continue
		}
		expected := expectedIfacesPerNode[n.Name]
		manualIfs := manualIfacesPerNode[n.Name]
		if stNode, ok := currentState.Nodes[n.Name]; ok {
			for ifaceName := range stNode.Interfaces {
				if strings.HasPrefix(ifaceName, "wg42") && !expected[ifaceName] && !manualIfs[ifaceName] {
					alreadyAdded := false
					for _, ca := range cleanActions {
						if ca.NodeName == n.Name && ca.Interface == ifaceName {
							alreadyAdded = true
							break
						}
					}
					if !alreadyAdded {
						targetFile := fmt.Sprintf("/etc/wireguard/%s.conf", ifaceName)
						cleanActions = append(cleanActions, config.SyncAction{
							NodeName:    n.Name,
							Host:        n.Host,
							Type:        config.ActionDeleteConfig,
							Interface:   ifaceName,
							TargetFile:  targetFile,
							Command:     fmt.Sprintf("wg-quick down %s && rm -f %s", ifaceName, targetFile),
							Description: fmt.Sprintf("Remove deleted interface %s on %s", ifaceName, n.Name),
							FileContent: fmt.Sprintf("# Deleted interface %s is no longer in graph.\n# Action: wg-quick down %s && rm -f %s\n", ifaceName, ifaceName, targetFile),
							NeedsApply:  true,
							Status:      "pending",
							DiffStatus:  "delete",
						})
					}
				}
			}
		}
	}

	// Sort clean actions for determinism
	sort.Slice(cleanActions, func(i, j int) bool {
		if cleanActions[i].NodeName != cleanActions[j].NodeName {
			return cleanActions[i].NodeName < cleanActions[j].NodeName
		}
		return cleanActions[i].Interface < cleanActions[j].Interface
	})
	actions = append(actions, cleanActions...)

	// 2. Active links configuration
	for _, link := range links {
		if link.IsManual() {
			continue
		}
		fromNode := nodeMap[link.From.Name]
		toNode := nodeMap[link.To.Name]
		if fromNode == nil || toNode == nil {
			continue
		}

		// 1. From node end (only if fromNode is managed)
		if !fromNode.IsExternal && (!isPartial || targetMap[fromNode.Name]) {
			fromConf, err := compiler.GenerateWgConfigContent(fromNode, toNode, &link.From, &link.To, m.vault, cfg, &link)
			if err == nil {
				targetFile := fmt.Sprintf("/etc/wireguard/%s.conf", link.From.Interface)
				desiredHash := config.HashConfig(compiler.NormalizeConfig(fromConf))

				needsApply := true
				status := "pending"
				diffStatus := "create"

				if stNode, ok := currentState.Nodes[fromNode.Name]; ok {
					if stIface, ok := stNode.Interfaces[link.From.Interface]; ok {
						if stIface.ConfigHash == desiredHash {
							needsApply = false
							status = "synced"
							diffStatus = "synced"
						} else {
							diffStatus = "update"
						}
					}
				}

				actions = append(actions, config.SyncAction{
					NodeName:    fromNode.Name,
					Host:        fromNode.Host,
					Type:        config.ActionSyncConfig,
					Interface:   link.From.Interface,
					TargetFile:  targetFile,
					FileContent: fromConf,
					Description: fmt.Sprintf("Configure %s on %s (peer %s)", link.From.Interface, fromNode.Name, toNode.Name),
					NeedsApply:  needsApply,
					Status:      status,
					DiffStatus:  diffStatus,
				})
			}
		}

		// 2. To node end (only if toNode is managed)
		if !toNode.IsExternal && (!isPartial || targetMap[toNode.Name]) {
			toConf, err := compiler.GenerateWgConfigContent(toNode, fromNode, &link.To, &link.From, m.vault, cfg, &link)
			if err == nil {
				targetFile := fmt.Sprintf("/etc/wireguard/%s.conf", link.To.Interface)
				desiredHash := config.HashConfig(compiler.NormalizeConfig(toConf))

				needsApply := true
				status := "pending"
				diffStatus := "create"

				if stNode, ok := currentState.Nodes[toNode.Name]; ok {
					if stIface, ok := stNode.Interfaces[link.To.Interface]; ok {
						if stIface.ConfigHash == desiredHash {
							needsApply = false
							status = "synced"
							diffStatus = "synced"
						} else {
							diffStatus = "update"
						}
					}
				}

				actions = append(actions, config.SyncAction{
					NodeName:    toNode.Name,
					Host:        toNode.Host,
					Type:        config.ActionSyncConfig,
					Interface:   link.To.Interface,
					TargetFile:  targetFile,
					FileContent: toConf,
					Description: fmt.Sprintf("Configure %s on %s (peer %s)", link.To.Interface, toNode.Name, fromNode.Name),
					NeedsApply:  needsApply,
					Status:      status,
					DiffStatus:  diffStatus,
				})
			}
		}
	}

	// Build map of all policies and nodes for policy/ROA planning
	allPolicies := cfg.GetAllPolicies()
	policyMap := make(map[string]config.NetworkPolicy)
	for _, p := range allPolicies {
		policyMap[p.ID] = p
	}

	nodeByName := make(map[string]*config.Node)
	for i := range nodes {
		nodeByName[nodes[i].Name] = &nodes[i]
	}

	// 3. Node ROA & BIRD configurations
	for _, n := range nodes {
		if n.IsExternal {
			continue
		}
		if isPartial && !targetMap[n.Name] {
			continue
		}
		node := n

		// Determine policies used by links on this node
		usedPolicies := make(map[string]config.NetworkPolicy)
		for _, l := range links {
			var localEnd *config.LinkEnd
			var remoteNode *config.Node
			if l.From.Name == node.Name {
				localEnd = &l.From
				remoteNode = nodeByName[l.To.Name]
			} else if l.To.Name == node.Name {
				localEnd = &l.To
				remoteNode = nodeByName[l.From.Name]
			} else {
				continue
			}
			isRemoteExternal := remoteNode != nil && remoteNode.IsExternal
			pID := localEnd.EffectivePolicy(isRemoteExternal)
			if pol, ok := policyMap[pID]; ok {
				usedPolicies[pID] = pol
			} else if isRemoteExternal {
				usedPolicies[config.PolicyDN42] = policyMap[config.PolicyDN42]
			} else {
				usedPolicies[config.PolicyDefault] = policyMap[config.PolicyDefault]
			}
		}

		var usedPolicyIDs []string
		for pID := range usedPolicies {
			usedPolicyIDs = append(usedPolicyIDs, pID)
		}
		sort.Strings(usedPolicyIDs)

		nodeHasRoaUpdates := false
		for _, pID := range usedPolicyIDs {
			pol := usedPolicies[pID]
			cleanID := compiler.SanitizeIdentifier(pol.ID)

			if strings.TrimSpace(pol.ROA4) != "" {
				targetFile := fmt.Sprintf("/etc/easy42_%s_roa4.conf", cleanID)
				content, _ := m.roaManager.GetROAContent(context.Background(), pol.ROA4, false)
				desiredHash := config.HashConfig(compiler.NormalizeConfig(content))

				needsApply := true
				status := "pending"
				diffStatus := "create"

				if stNode, ok := currentState.Nodes[node.Name]; ok && stNode.RoaConfigHashes != nil {
					if h, exists := stNode.RoaConfigHashes[targetFile]; exists && h != "" {
						if h == desiredHash {
							needsApply = false
							status = "synced"
							diffStatus = "synced"
						} else {
							diffStatus = "update"
						}
					}
				}

				if needsApply {
					nodeHasRoaUpdates = true
				}

				actions = append(actions, config.SyncAction{
					NodeName:    node.Name,
					Host:        node.Host,
					Type:        config.ActionSyncRoaConfig,
					Interface:   cleanID + "_roa4",
					TargetFile:  targetFile,
					FileContent: content,
					Description: fmt.Sprintf("Deploy ROA4 table (%s) for policy '%s' on %s", targetFile, pol.Name, node.Name),
					NeedsApply:  needsApply,
					Status:      status,
					DiffStatus:  diffStatus,
				})
			}

			if strings.TrimSpace(pol.ROA6) != "" {
				targetFile := fmt.Sprintf("/etc/easy42_%s_roa6.conf", cleanID)
				content, _ := m.roaManager.GetROAContent(context.Background(), pol.ROA6, false)
				desiredHash := config.HashConfig(compiler.NormalizeConfig(content))

				needsApply := true
				status := "pending"
				diffStatus := "create"

				if stNode, ok := currentState.Nodes[node.Name]; ok && stNode.RoaConfigHashes != nil {
					if h, exists := stNode.RoaConfigHashes[targetFile]; exists && h != "" {
						if h == desiredHash {
							needsApply = false
							status = "synced"
							diffStatus = "synced"
						} else {
							diffStatus = "update"
						}
					}
				}

				if needsApply {
					nodeHasRoaUpdates = true
				}

				actions = append(actions, config.SyncAction{
					NodeName:    node.Name,
					Host:        node.Host,
					Type:        config.ActionSyncRoaConfig,
					Interface:   cleanID + "_roa6",
					TargetFile:  targetFile,
					FileContent: content,
					Description: fmt.Sprintf("Deploy ROA6 table (%s) for policy '%s' on %s", targetFile, pol.Name, node.Name),
					NeedsApply:  needsApply,
					Status:      status,
					DiffStatus:  diffStatus,
				})
			}
		}

		birdConf, err := compiler.GenerateBirdConfig(&node, nodes, links, &cfg.NetworkSettings, cfg.NetworkPolicies)
		if err != nil {
			continue
		}

		targetFile := compiler.DefaultBirdConfigPath
		desiredHash := config.HashConfig(compiler.NormalizeConfig(birdConf))

		needsApply := true
		status := "pending"
		diffStatus := "create"

		if stNode, ok := currentState.Nodes[node.Name]; ok {
			if stNode.BirdConfigHash != "" {
				if stNode.BirdConfigHash == desiredHash {
					needsApply = false
					status = "synced"
					diffStatus = "synced"
				} else {
					diffStatus = "update"
				}
			}
		}

		if nodeHasRoaUpdates {
			needsApply = true
			if diffStatus == "synced" {
				diffStatus = "update"
				status = "pending"
			}
		}

		actions = append(actions, config.SyncAction{
			NodeName:    node.Name,
			Host:        node.Host,
			Type:        config.ActionSyncBirdConfig,
			Interface:   "bird",
			TargetFile:  targetFile,
			FileContent: birdConf,
			Command:     "birdc configure",
			Description: fmt.Sprintf("Configure BIRD routing (%s) on %s", targetFile, node.Name),
			NeedsApply:  needsApply,
			Status:      status,
			DiffStatus:  diffStatus,
		})
	}

	// 4. Node nftables configurations
	for _, n := range nodes {
		if n.IsExternal {
			continue
		}
		if isPartial && !targetMap[n.Name] {
			continue
		}
		node := n
		nftConf, err := compiler.GenerateNftablesConfig(&node, nodes, links, &cfg.NetworkSettings, cfg.NetworkPolicies)
		if err != nil {
			continue
		}

		targetFile := compiler.DefaultNftablesConfigPath
		desiredHash := config.HashConfig(compiler.NormalizeConfig(nftConf))

		needsApply := true
		status := "pending"
		diffStatus := "create"

		if stNode, ok := currentState.Nodes[node.Name]; ok {
			if stNode.NftablesConfigHash != "" {
				if stNode.NftablesConfigHash == desiredHash {
					needsApply = false
					status = "synced"
					diffStatus = "synced"
				} else {
					diffStatus = "update"
				}
			}
		}

		actions = append(actions, config.SyncAction{
			NodeName:    node.Name,
			Host:        node.Host,
			Type:        config.ActionSyncNftablesConfig,
			Interface:   "nftables",
			TargetFile:  targetFile,
			FileContent: nftConf,
			Command:     targetFile,
			Description: fmt.Sprintf("Configure nftables firewall (%s) on %s", targetFile, node.Name),
			NeedsApply:  needsApply,
			Status:      status,
			DiffStatus:  diffStatus,
		})
	}

	// Sort actions: pending (NeedsApply == true) first, then synced.
	// Within each group, order by actionPriority (clean/delete -> wireguard -> bird), then NodeName and Interface
	sort.Slice(actions, func(i, j int) bool {
		if actions[i].NeedsApply != actions[j].NeedsApply {
			return actions[i].NeedsApply // true comes before false
		}
		pI := actionPriority(actions[i].Type)
		pJ := actionPriority(actions[j].Type)
		if pI != pJ {
			return pI < pJ
		}
		if actions[i].NodeName != actions[j].NodeName {
			return actions[i].NodeName < actions[j].NodeName
		}
		return actions[i].Interface < actions[j].Interface
	})

	if actions == nil {
		actions = []config.SyncAction{}
	}
	return actions, nil
}

// ExecuteSync executes planned actions across nodes
func (m *Manager) ExecuteSync(force ...bool) ([]config.SyncResult, error) {
	isForce := len(force) > 0 && force[0]
	return m.ExecuteSyncNodes(isForce)
}

// ExecuteSyncNodes executes planned actions across specified nodes (or all nodes if none specified)
func (m *Manager) ExecuteSyncNodes(isForce bool, nodeNames ...string) ([]config.SyncResult, error) {
	actions, err := m.PlanSync(nodeNames...)
	if err != nil {
		return nil, err
	}

	var actionsToRun []config.SyncAction
	for _, act := range actions {
		if isForce || act.NeedsApply {
			actionsToRun = append(actionsToRun, act)
		}
	}

	if len(actionsToRun) == 0 {
		return []config.SyncResult{}, nil
	}

	results := make([]config.SyncResult, 0)
	for _, act := range actionsToRun {
		start := time.Now()
		res := config.SyncResult{
			NodeName: act.NodeName,
			Action:   act.Description,
		}

		sshClient, sftpClient, err := m.pool.GetClient(act.Host)
		if err != nil {
			res.Success = false
			res.Error = fmt.Sprintf("SSH connect failed: %v", err)
			res.Duration = float64(time.Since(start).Milliseconds())
			results = append(results, res)
			continue
		}

		// Handle deleted interface cleanup
		if act.Type == config.ActionDeleteConfig || act.Type == config.ActionDownInterface {
			if err := ssh.CleanWireGuardInterface(sshClient, sftpClient, act.Interface); err != nil {
				res.Success = false
				res.Error = fmt.Sprintf("Failed to clean interface %s: %v", act.Interface, err)
			} else {
				res.Success = true
				_ = m.stateStore.RemoveInterface(act.NodeName, act.Interface)
			}
			res.Duration = float64(time.Since(start).Milliseconds())
			results = append(results, res)
			continue
		}

		// Check current remote file
		currentContent, _ := ssh.ReadRemoteFile(sftpClient, act.TargetFile)
		needsUpdate := isForce || compiler.NeedsUpdate(currentContent, act.FileContent)

		// Handle ROA configuration update
		if act.Type == config.ActionSyncRoaConfig {
			if needsUpdate {
				if err := ssh.AtomicWriteFile(sftpClient, act.TargetFile, []byte(act.FileContent), 0644); err != nil {
					res.Success = false
					res.Error = fmt.Sprintf("Failed to write ROA config (%s): %v", act.TargetFile, err)
					res.Duration = float64(time.Since(start).Milliseconds())
					results = append(results, res)
					continue
				}
			}

			res.Success = true
			res.Duration = float64(time.Since(start).Milliseconds())
			results = append(results, res)

			// Record successful application in stateStore
			hash := config.HashConfig(compiler.NormalizeConfig(act.FileContent))
			_ = m.stateStore.UpdateRoaState(act.NodeName, act.Host, act.TargetFile, hash, time.Now())
			continue
		}

		// Handle BIRD configuration update
		if act.Type == config.ActionSyncBirdConfig {
			if needsUpdate {
				if err := ssh.AtomicWriteFile(sftpClient, act.TargetFile, []byte(act.FileContent), 0644); err != nil {
					res.Success = false
					res.Error = fmt.Sprintf("Failed to write BIRD config: %v", err)
					res.Duration = float64(time.Since(start).Milliseconds())
					results = append(results, res)
					continue
				}

				// Apply BIRD configuration on remote device
				out, err := ssh.RunCommand(sshClient, "birdc configure")
				res.Output = strings.TrimSpace(out)
				if err != nil {
					res.Success = false
					res.Error = fmt.Sprintf("Failed to apply BIRD config (birdc configure): %v", err)
					res.Duration = float64(time.Since(start).Milliseconds())
					results = append(results, res)
					continue
				}
			}

			res.Success = true
			res.Duration = float64(time.Since(start).Milliseconds())
			results = append(results, res)

			// Record successful application in stateStore
			hash := config.HashConfig(compiler.NormalizeConfig(act.FileContent))
			_ = m.stateStore.UpdateBirdState(act.NodeName, act.Host, hash, time.Now())
			continue
		}

		// Handle nftables configuration update
		if act.Type == config.ActionSyncNftablesConfig {
			if needsUpdate {
				if err := ssh.AtomicWriteFile(sftpClient, act.TargetFile, []byte(act.FileContent), 0755); err != nil {
					res.Success = false
					res.Error = fmt.Sprintf("Failed to write nftables config: %v", err)
					res.Duration = float64(time.Since(start).Milliseconds())
					results = append(results, res)
					continue
				}

				// Apply nftables rules on remote device
				cmd := act.Command
				if cmd == "" {
					cmd = "/etc/easy42.nft"
				}
				out, err := ssh.RunCommand(sshClient, fmt.Sprintf("chmod +x %s 2>/dev/null; %s", act.TargetFile, cmd))
				res.Output = strings.TrimSpace(out)
				if err != nil {
					res.Success = false
					res.Error = fmt.Sprintf("Failed to apply nftables rules (%s): %v", cmd, err)
					res.Duration = float64(time.Since(start).Milliseconds())
					results = append(results, res)
					continue
				}
			}

			res.Success = true
			res.Duration = float64(time.Since(start).Milliseconds())
			results = append(results, res)

			// Record successful application in stateStore
			hash := config.HashConfig(compiler.NormalizeConfig(act.FileContent))
			_ = m.stateStore.UpdateNftablesState(act.NodeName, act.Host, hash, time.Now())
			continue
		}

		if needsUpdate {
			if err := ssh.AtomicWriteFile(sftpClient, act.TargetFile, []byte(act.FileContent), 0600); err != nil {
				res.Success = false
				res.Error = fmt.Sprintf("Failed to write config: %v", err)
				res.Duration = float64(time.Since(start).Milliseconds())
				results = append(results, res)
				continue
			}
		}

		// Start device wg42* interface if not in started state, or sync dynamically if already running
		if !ssh.IsInterfaceStarted(sshClient, act.Interface) {
			if err := ssh.StartWireGuardInterface(sshClient, act.Interface); err != nil {
				res.Success = false
				res.Error = fmt.Sprintf("Failed to start WireGuard interface %s: %v", act.Interface, err)
				res.Duration = float64(time.Since(start).Milliseconds())
				results = append(results, res)
				continue
			}
		} else if needsUpdate {
			// Check if interface requires restart (e.g. MTU or Address changed)
			needsRestart := compiler.RequiresWgRestart(currentContent, act.FileContent)
			if !needsRestart {
				// Also check if live interface MTU differs from desired MTU in config
				desiredMTU := compiler.ExtractWgMTU(act.FileContent)
				if desiredMTU > 0 {
					if liveMTU, err := ssh.GetInterfaceMTU(sshClient, act.Interface); err == nil && liveMTU > 0 && liveMTU != desiredMTU {
						needsRestart = true
					}
				}
			}

			if needsRestart {
				if err := ssh.RestartWireGuard(sshClient, act.Interface); err != nil {
					res.Success = false
					res.Error = fmt.Sprintf("Failed to restart WireGuard interface %s: %v", act.Interface, err)
					res.Duration = float64(time.Since(start).Milliseconds())
					results = append(results, res)
					continue
				}
			} else {
				// Interface is already started; sync/reload WireGuard dynamically
				if err := ssh.SyncWireGuard(sshClient, act.Interface, act.TargetFile); err != nil {
					res.Success = false
					res.Error = fmt.Sprintf("Failed to reload WireGuard interface %s: %v", act.Interface, err)
					res.Duration = float64(time.Since(start).Milliseconds())
					results = append(results, res)
					continue
				}
			}
		}

		res.Success = true
		res.Duration = float64(time.Since(start).Milliseconds())
		results = append(results, res)

		// Record successful application in stateStore
		hash := config.HashConfig(compiler.NormalizeConfig(act.FileContent))
		existingState := m.stateStore.Get()
		var existingHandshake *time.Time
		workingState := config.WorkingStateUnknown
		var existingRx, existingTx int64
		if stNode, ok := existingState.Nodes[act.NodeName]; ok {
			if stIface, ok := stNode.Interfaces[act.Interface]; ok {
				existingHandshake = stIface.LatestHandshake
				workingState = stIface.WorkingState
				existingRx = stIface.TransferRxBytes
				existingTx = stIface.TransferTxBytes
			}
		}
		_ = m.stateStore.UpdateInterface(act.NodeName, act.Host, config.StateInterface{
			Name:            act.Interface,
			TargetFile:      act.TargetFile,
			ConfigHash:      hash,
			Status:          "active",
			LatestHandshake: existingHandshake,
			WorkingState:    workingState,
			TransferRxBytes: existingRx,
			TransferTxBytes: existingTx,
			AppliedAt:       time.Now(),
		})
	}

	if results == nil {
		results = []config.SyncResult{}
	}

	m.mu.Lock()
	m.lastSync = time.Now()
	m.lastResults = results
	m.mu.Unlock()

	return results, nil
}

// UpdateState connects to devices via SSH/SFTP to fetch their live state and update state.json.
// If nodeNames are provided, only those specific nodes are refreshed and their links/interfaces updated.
func (m *Manager) UpdateState(nodeNames ...string) (*config.NetworkState, []string, error) {
	m.mu.RLock()
	cfg := m.store.Get()
	nodes := make([]config.Node, len(cfg.Nodes))
	copy(nodes, cfg.Nodes)
	links := make([]config.Link, len(cfg.Links))
	copy(links, cfg.Links)
	m.mu.RUnlock()

	targetMap := make(map[string]bool)
	for _, name := range nodeNames {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			targetMap[trimmed] = true
		}
	}
	isPartialUpdate := len(targetMap) > 0

	type linkMetaInfo struct {
		peerNode            string
		peerPubKey          string
		persistentKeepalive int
	}
	linkMeta := make(map[string]map[string]linkMetaInfo)
	for _, link := range links {
		// End From
		if linkMeta[link.From.Name] == nil {
			linkMeta[link.From.Name] = make(map[string]linkMetaInfo)
		}
		ka := 0
		if link.From.PersistentKeepalive > 0 || link.To.PersistentKeepalive > 0 {
			ka = 25
		}
		linkMeta[link.From.Name][link.From.Interface] = linkMetaInfo{
			peerNode:            link.To.Name,
			peerPubKey:          link.To.PublicKey,
			persistentKeepalive: ka,
		}

		// End To
		if linkMeta[link.To.Name] == nil {
			linkMeta[link.To.Name] = make(map[string]linkMetaInfo)
		}
		linkMeta[link.To.Name][link.To.Interface] = linkMetaInfo{
			peerNode:            link.From.Name,
			peerPubKey:          link.From.PublicKey,
			persistentKeepalive: ka,
		}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var warnings []string

	type nodeIfaceResult struct {
		nodeName string
		host     string
		ifaces   map[string]config.StateInterface
	}
	results := make([]nodeIfaceResult, 0, len(nodes))

	for _, n := range nodes {
		node := n
		if isPartialUpdate && !targetMap[node.Name] {
			continue
		}

		if node.IsExternal {
			m.mu.Lock()
			m.statuses[node.Name] = &config.NodeStatus{
				Name:      node.Name,
				Host:      "external",
				LastSeen:  time.Now(),
				Connected: true,
				Hostname:  node.Name,
			}
			m.mu.Unlock()
			continue
		}

		wg.Add(1)
		go func(targetNode config.Node) {
			defer wg.Done()

			type probeOutput struct {
				ifaces   map[string]config.StateInterface
				hostname string
				wgStatus []config.WgInterfaceStatus
				err      error
			}
			outChan := make(chan probeOutput, 1)

			go func() {
				sshClient, sftpClient, err := m.pool.GetClientWithTimeout(targetNode.Host, 4*time.Second)
				if err != nil {
					outChan <- probeOutput{err: err}
					return
				}

				// Query WireGuard interface and peer statuses
				wgStatus, _ := ssh.QueryWireGuardStatus(sshClient)
				wgMap := make(map[string]*config.WgInterfaceStatus)
				for i := range wgStatus {
					wgMap[wgStatus[i].Name] = &wgStatus[i]
				}

				// 1. Find all wg42*.conf files in /etc/wireguard
				filesOut, _ := ssh.RunCommand(sshClient, "ls -1 /etc/wireguard/wg42*.conf 2>/dev/null")
				lines := strings.Split(strings.TrimSpace(filesOut), "\n")

				nodeIfaces := make(map[string]config.StateInterface)
				for _, line := range lines {
					filePath := strings.TrimSpace(line)
					if filePath == "" {
						continue
					}
					base := filepath.Base(filePath)
					ifaceName := strings.TrimSuffix(base, ".conf")
					if !strings.HasPrefix(ifaceName, "wg42") {
						continue
					}

					// Read file content
					content, readErr := ssh.ReadRemoteFile(sftpClient, filePath)
					if readErr != nil {
						catOut, catErr := ssh.RunCommand(sshClient, fmt.Sprintf("cat %s", filePath))
						if catErr == nil {
							content = catOut
						}
					}

					hash := config.HashConfig(compiler.NormalizeConfig(content))
					status := "down"
					isStarted := ssh.IsInterfaceStarted(sshClient, ifaceName)
					if isStarted {
						status = "active"
					}

					meta := linkMeta[targetNode.Name][ifaceName]
					var latestHandshake time.Time
					var rxBytes, txBytes int64
					keepalive := meta.persistentKeepalive

					if wgInfo, ok := wgMap[ifaceName]; ok && len(wgInfo.Peers) > 0 {
						var matchedPeer *config.WgPeerStatus
						for pIdx := range wgInfo.Peers {
							if meta.peerPubKey != "" && wgInfo.Peers[pIdx].PublicKey == meta.peerPubKey {
								matchedPeer = &wgInfo.Peers[pIdx]
								break
							}
						}
						if matchedPeer == nil {
							matchedPeer = &wgInfo.Peers[0]
						}
						latestHandshake = matchedPeer.LatestHandshake
						rxBytes = matchedPeer.TransferRxBytes
						txBytes = matchedPeer.TransferTxBytes
						if matchedPeer.PersistentKeepalive > 0 {
							keepalive = matchedPeer.PersistentKeepalive
						}
					}

					now := time.Now()
					var handshakePtr *time.Time
					workingState := config.WorkingStateUnknown

					if !isStarted {
						workingState = config.WorkingStateNotWorking
					} else if !latestHandshake.IsZero() {
						handshakePtr = &latestHandshake
						if now.Sub(latestHandshake) <= 3*time.Minute {
							workingState = config.WorkingStateWorking
						} else {
							if keepalive > 0 {
								workingState = config.WorkingStateNotWorking
							} else {
								workingState = config.WorkingStateUnknown
							}
						}
					} else {
						if keepalive > 0 {
							workingState = config.WorkingStateNotWorking
						} else {
							workingState = config.WorkingStateUnknown
						}
					}

					nodeIfaces[ifaceName] = config.StateInterface{
						Name:            ifaceName,
						TargetFile:      filePath,
						ConfigHash:      hash,
						PeerNode:        meta.peerNode,
						PeerPubKey:      meta.peerPubKey,
						Status:          status,
						LatestHandshake: handshakePtr,
						WorkingState:    workingState,
						TransferRxBytes: rxBytes,
						TransferTxBytes: txBytes,
						AppliedAt:       time.Now(),
					}
				}

				hostname, _ := ssh.RunCommand(sshClient, "hostname")
				outChan <- probeOutput{
					ifaces:   nodeIfaces,
					hostname: strings.TrimSpace(hostname),
					wgStatus: wgStatus,
				}
			}()

			select {
			case out := <-outChan:
				if out.err != nil {
					mu.Lock()
					warnings = append(warnings, fmt.Sprintf("%s: failed to connect: %v", targetNode.Name, out.err))
					mu.Unlock()
					m.mu.Lock()
					m.statuses[targetNode.Name] = &config.NodeStatus{
						Name:      targetNode.Name,
						Host:      targetNode.Host,
						LastSeen:  time.Now(),
						Connected: false,
						Error:     out.err.Error(),
					}
					m.mu.Unlock()
					return
				}

				m.mu.Lock()
				m.statuses[targetNode.Name] = &config.NodeStatus{
					Name:         targetNode.Name,
					Host:         targetNode.Host,
					LastSeen:     time.Now(),
					Connected:    true,
					Hostname:     out.hostname,
					WgInterfaces: out.wgStatus,
				}
				m.mu.Unlock()

				mu.Lock()
				results = append(results, nodeIfaceResult{
					nodeName: targetNode.Name,
					host:     targetNode.Host,
					ifaces:   out.ifaces,
				})
				mu.Unlock()

			case <-time.After(30 * time.Second):
				mu.Lock()
				warnings = append(warnings, fmt.Sprintf("%s: probe timed out", targetNode.Name))
				mu.Unlock()
				m.mu.Lock()
				m.statuses[targetNode.Name] = &config.NodeStatus{
					Name:      targetNode.Name,
					Host:      targetNode.Host,
					LastSeen:  time.Now(),
					Connected: false,
					Error:     "probe timed out",
				}
				m.mu.Unlock()
				return
			}
		}(node)
	}
	wg.Wait()

	// Update state store
	currentState := m.stateStore.Get()

	// Remove deleted or external nodes from state ONLY on full cluster updates
	if !isPartialUpdate {
		activeInternalNodes := make(map[string]bool)
		allActiveNodes := make(map[string]bool)
		for _, n := range nodes {
			allActiveNodes[n.Name] = true
			if !n.IsExternal {
				activeInternalNodes[n.Name] = true
			}
		}

		for stName := range currentState.Nodes {
			if !activeInternalNodes[stName] {
				delete(currentState.Nodes, stName)
			}
		}

		m.mu.Lock()
		for stName := range m.statuses {
			if !allActiveNodes[stName] {
				delete(m.statuses, stName)
			}
		}
		m.mu.Unlock()
	}

	// Merge probed node interfaces
	for _, res := range results {
		existingNode := currentState.Nodes[res.nodeName]
		currentState.Nodes[res.nodeName] = config.StateNode{
			Name:               res.nodeName,
			Host:               res.host,
			LastSeen:           time.Now(),
			BirdConfigHash:     existingNode.BirdConfigHash,
			BirdAppliedAt:      existingNode.BirdAppliedAt,
			NftablesConfigHash: existingNode.NftablesConfigHash,
			NftablesAppliedAt:  existingNode.NftablesAppliedAt,
			Interfaces:         res.ifaces,
		}
	}

	if err := m.stateStore.Save(currentState); err != nil {
		return nil, warnings, fmt.Errorf("failed to save updated state: %w", err)
	}

	return currentState, warnings, nil
}

// GetLastSyncResults returns the latest sync execution results
func (m *Manager) GetLastSyncResults() (time.Time, []config.SyncResult) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := m.lastResults
	if res == nil {
		res = []config.SyncResult{}
	}
	return m.lastSync, res
}

// GenerateBirdConfig generates the BIRD configuration for a given node
func (m *Manager) GenerateBirdConfig(nodeName string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := m.store.Get()
	var targetNode *config.Node
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == nodeName {
			targetNode = &cfg.Nodes[i]
			break
		}
	}
	if targetNode == nil {
		return "", fmt.Errorf("node %s not found", nodeName)
	}

	return compiler.GenerateBirdConfig(targetNode, cfg.Nodes, cfg.Links, &cfg.NetworkSettings, cfg.NetworkPolicies)
}

// GenerateBirdConfigWithTemplate generates BIRD config using a custom template
func (m *Manager) GenerateBirdConfigWithTemplate(nodeName string, tmplContent string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := m.store.Get()
	var targetNode *config.Node
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == nodeName {
			targetNode = &cfg.Nodes[i]
			break
		}
	}
	if targetNode == nil {
		return "", fmt.Errorf("node %s not found", nodeName)
	}

	return compiler.GenerateBirdConfigWithTemplate(tmplContent, targetNode, cfg.Nodes, cfg.Links, &cfg.NetworkSettings, cfg.NetworkPolicies)
}

// GenerateNftablesConfig generates the nftables configuration for a given node
func (m *Manager) GenerateNftablesConfig(nodeName string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := m.store.Get()
	var targetNode *config.Node
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == nodeName {
			targetNode = &cfg.Nodes[i]
			break
		}
	}
	if targetNode == nil {
		return "", fmt.Errorf("node %s not found", nodeName)
	}

	return compiler.GenerateNftablesConfig(targetNode, cfg.Nodes, cfg.Links, &cfg.NetworkSettings, cfg.NetworkPolicies)
}

// GenerateNftablesConfigWithTemplate generates nftables config using a custom template
func (m *Manager) GenerateNftablesConfigWithTemplate(nodeName string, tmplContent string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := m.store.Get()
	var targetNode *config.Node
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == nodeName {
			targetNode = &cfg.Nodes[i]
			break
		}
	}
	if targetNode == nil {
		return "", fmt.Errorf("node %s not found", nodeName)
	}

	return compiler.GenerateNftablesConfigWithTemplate(tmplContent, targetNode, cfg.Nodes, cfg.Links, &cfg.NetworkSettings, cfg.NetworkPolicies)
}

// GetNetworkPolicies returns all policies (built-in virtual policies + custom policies)
func (m *Manager) GetNetworkPolicies() []config.NetworkPolicy {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.store.Get()
	if cfg == nil {
		return config.GetBuiltinPolicies(nil)
	}
	return cfg.GetAllPolicies()
}

// GetNetworkPolicy finds a policy by its ID
func (m *Manager) GetNetworkPolicy(id string) (*config.NetworkPolicy, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.store.Get()
	if cfg == nil {
		return nil, errors.New("config not found")
	}
	p := cfg.FindPolicy(id)
	if p == nil {
		return nil, errors.New("network policy not found")
	}
	return p, nil
}

// CreateNetworkPolicy adds a new custom network policy
func (m *Manager) CreateNetworkPolicy(p config.NetworkPolicy) (*config.NetworkPolicy, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.vault.IsUnlocked() {
		return nil, crypto.ErrVaultLocked
	}

	id := strings.TrimSpace(p.ID)
	name := strings.TrimSpace(p.Name)
	if id == "" {
		return nil, errors.New("policy id is required")
	}
	if name == "" {
		return nil, errors.New("policy name is required")
	}
	if id == config.PolicyDefault || id == config.PolicyDN42 || id == config.PolicyNone {
		return nil, fmt.Errorf("cannot create policy with reserved ID %q", id)
	}

	cfg := m.store.Get()
	for _, existing := range cfg.NetworkPolicies {
		if existing.ID == id {
			return nil, fmt.Errorf("network policy with ID %q already exists", id)
		}
	}

	p.ID = id
	p.Name = name
	p.IsInternal = false
	if p.Cost <= 0 {
		p.Cost = 100
	}
	p.LocalNetworks = config.CleanPrefixes(p.LocalNetworks)
	p.AllowedDstCIDRs = config.CleanPrefixes(p.AllowedDstCIDRs)
	p.AllowedSrcCIDRs = config.CleanPrefixes(p.AllowedSrcCIDRs)
	p.DisallowedDstCIDRs = config.CleanPrefixes(p.DisallowedDstCIDRs)
	p.DisallowedSrcCIDRs = config.CleanPrefixes(p.DisallowedSrcCIDRs)
	p.InputTCPPorts = config.CleanPortList(p.InputTCPPorts)
	p.InputUDPPorts = config.CleanPortList(p.InputUDPPorts)
	p.DSCPIngress = config.ValidateDSCP(p.DSCPIngress)
	p.DSCPEgress = config.ValidateDSCP(p.DSCPEgress)
	p.Fwmark = strings.TrimSpace(p.Fwmark)
	p.Mark = strings.TrimSpace(p.Mark)
	p.ForwardSNATTarget = strings.TrimSpace(p.ForwardSNATTarget)
	p.BlockIngressNew = config.NormalizeBlockIngressNew(p.BlockIngressNew)
	p.RoutingPolicy = config.NormalizeRoutingPolicy(p.RoutingPolicy)

	cfg.NetworkPolicies = append(cfg.NetworkPolicies, p)
	if err := m.store.Save(cfg); err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateNetworkPolicy updates an existing custom network policy
func (m *Manager) UpdateNetworkPolicy(id string, p config.NetworkPolicy) (*config.NetworkPolicy, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.vault.IsUnlocked() {
		return nil, crypto.ErrVaultLocked
	}

	if id == config.PolicyDefault || id == config.PolicyDN42 || id == config.PolicyNone {
		return nil, fmt.Errorf("cannot modify built-in policy %q", id)
	}

	name := strings.TrimSpace(p.Name)
	if name == "" {
		return nil, errors.New("policy name is required")
	}

	cfg := m.store.Get()
	idx := -1
	for i, existing := range cfg.NetworkPolicies {
		if existing.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, fmt.Errorf("network policy %q not found", id)
	}

	if p.Cost <= 0 {
		p.Cost = 100
	}

	cfg.NetworkPolicies[idx].Name = name
	cfg.NetworkPolicies[idx].Description = strings.TrimSpace(p.Description)
	cfg.NetworkPolicies[idx].Cost = p.Cost
	cfg.NetworkPolicies[idx].LocalNetworks = config.CleanPrefixes(p.LocalNetworks)
	cfg.NetworkPolicies[idx].AllowedDstCIDRs = config.CleanPrefixes(p.AllowedDstCIDRs)
	cfg.NetworkPolicies[idx].AllowedSrcCIDRs = config.CleanPrefixes(p.AllowedSrcCIDRs)
	cfg.NetworkPolicies[idx].DisallowedDstCIDRs = config.CleanPrefixes(p.DisallowedDstCIDRs)
	cfg.NetworkPolicies[idx].DisallowedSrcCIDRs = config.CleanPrefixes(p.DisallowedSrcCIDRs)
	cfg.NetworkPolicies[idx].RejectInternet = p.RejectInternet
	cfg.NetworkPolicies[idx].FilterForward = p.FilterForward
	cfg.NetworkPolicies[idx].FilterInput = p.FilterInput
	cfg.NetworkPolicies[idx].InputAllowICMP = p.InputAllowICMP
	cfg.NetworkPolicies[idx].InputAllowICMP6 = p.InputAllowICMP6
	cfg.NetworkPolicies[idx].InputTCPPorts = config.CleanPortList(p.InputTCPPorts)
	cfg.NetworkPolicies[idx].InputUDPPorts = config.CleanPortList(p.InputUDPPorts)
	cfg.NetworkPolicies[idx].SNAT = p.SNAT
	cfg.NetworkPolicies[idx].ForwardSNAT = p.ForwardSNAT
	cfg.NetworkPolicies[idx].ForwardSNATTarget = strings.TrimSpace(p.ForwardSNATTarget)
	cfg.NetworkPolicies[idx].DSCPIngress = config.ValidateDSCP(p.DSCPIngress)
	cfg.NetworkPolicies[idx].DSCPEgress = config.ValidateDSCP(p.DSCPEgress)
	cfg.NetworkPolicies[idx].ROA4 = strings.TrimSpace(p.ROA4)
	cfg.NetworkPolicies[idx].ROA6 = strings.TrimSpace(p.ROA6)
	cfg.NetworkPolicies[idx].ROAStrict = p.ROAStrict
	cfg.NetworkPolicies[idx].Fwmark = strings.TrimSpace(p.Fwmark)
	cfg.NetworkPolicies[idx].Preference = p.Preference
	cfg.NetworkPolicies[idx].Mark = strings.TrimSpace(p.Mark)
	cfg.NetworkPolicies[idx].BlockIngressNew = config.NormalizeBlockIngressNew(p.BlockIngressNew)
	cfg.NetworkPolicies[idx].RoutingPolicy = config.NormalizeRoutingPolicy(p.RoutingPolicy)

	if err := m.store.Save(cfg); err != nil {
		return nil, err
	}
	res := cfg.NetworkPolicies[idx]
	return &res, nil
}

// DeleteNetworkPolicy deletes a custom network policy if it's not currently used by any link
func (m *Manager) DeleteNetworkPolicy(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.vault.IsUnlocked() {
		return crypto.ErrVaultLocked
	}

	if id == config.PolicyDefault || id == config.PolicyDN42 || id == config.PolicyNone {
		return fmt.Errorf("cannot delete built-in policy %q", id)
	}

	cfg := m.store.Get()
	// Check if any link uses this policy
	for _, l := range cfg.Links {
		if l.From.Policy == id || l.To.Policy == id {
			return fmt.Errorf("policy %q is currently in use by link %s <-> %s", id, l.From.Name, l.To.Name)
		}
	}

	idx := -1
	for i, existing := range cfg.NetworkPolicies {
		if existing.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("network policy %q not found", id)
	}

	cfg.NetworkPolicies = append(cfg.NetworkPolicies[:idx], cfg.NetworkPolicies[idx+1:]...)
	return m.store.Save(cfg)
}

// GetNetworkSettings returns current network settings from config
func (m *Manager) GetNetworkSettings() config.NetworkSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.store.Get()
	if cfg == nil {
		return config.NetworkSettings{}
	}
	ns := cfg.NetworkSettings
	ns.Prefixes = config.CleanPrefixes(ns.Prefixes)
	ns.LocalDN42Networks = config.CleanPrefixes(ns.LocalDN42Networks)
	if len(ns.DisallowedDN42Networks) == 0 && len(ns.DisallowedDN42CIDRs) > 0 {
		ns.DisallowedDN42Networks = config.CleanPrefixes(ns.DisallowedDN42CIDRs)
	} else {
		ns.DisallowedDN42Networks = config.CleanPrefixes(ns.DisallowedDN42Networks)
	}
	ns.DisallowedDN42CIDRs = ns.DisallowedDN42Networks
	return ns
}

// UpdateNetworkSettings updates the global network settings in config.json
func (m *Manager) UpdateNetworkSettings(settings config.NetworkSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	if cfg == nil {
		return config.ErrConfigNotFound
	}
	settings.Prefixes = config.CleanPrefixes(settings.Prefixes)
	settings.LocalDN42Networks = config.CleanPrefixes(settings.LocalDN42Networks)
	if len(settings.DisallowedDN42Networks) == 0 && len(settings.DisallowedDN42CIDRs) > 0 {
		settings.DisallowedDN42Networks = config.CleanPrefixes(settings.DisallowedDN42CIDRs)
	} else {
		settings.DisallowedDN42Networks = config.CleanPrefixes(settings.DisallowedDN42Networks)
	}
	settings.DisallowedDN42CIDRs = settings.DisallowedDN42Networks
	cfg.NetworkSettings = settings
	return m.store.Save(cfg)
}

// GetBlocks returns current blocks from config
func (m *Manager) GetBlocks() []config.Block {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.store.Get()
	if cfg == nil || cfg.Blocks == nil {
		return []config.Block{}
	}
	res := make([]config.Block, len(cfg.Blocks))
	copy(res, cfg.Blocks)
	return res
}

// UpdateBlocks updates the blocks list in config.json
func (m *Manager) UpdateBlocks(blocks []config.Block) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	if cfg == nil {
		return config.ErrConfigNotFound
	}
	if blocks == nil {
		blocks = []config.Block{}
	}
	cfg.Blocks = blocks
	return m.store.Save(cfg)
}

// RefreshAllROA forces a refresh of all ROA URLs configured across all policies
func (m *Manager) RefreshAllROA(ctx context.Context, force bool) error {
	cfg := m.store.Get()
	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			return err
		}
	}
	allPolicies := cfg.GetAllPolicies()
	for _, p := range allPolicies {
		if strings.TrimSpace(p.ROA4) != "" {
			_, _ = m.roaManager.GetROAContent(ctx, p.ROA4, force)
		}
		if strings.TrimSpace(p.ROA6) != "" {
			_, _ = m.roaManager.GetROAContent(ctx, p.ROA6, force)
		}
	}
	return nil
}

// RefreshPolicyROA refreshes the ROA cache for a specific policy ID
func (m *Manager) RefreshPolicyROA(ctx context.Context, policyID string) error {
	cfg := m.store.Get()
	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			return err
		}
	}
	p := cfg.FindPolicy(policyID)
	if p == nil {
		return fmt.Errorf("policy '%s' not found", policyID)
	}
	if strings.TrimSpace(p.ROA4) != "" {
		if _, err := m.roaManager.GetROAContent(ctx, p.ROA4, true); err != nil {
			return err
		}
	}
	if strings.TrimSpace(p.ROA6) != "" {
		if _, err := m.roaManager.GetROAContent(ctx, p.ROA6, true); err != nil {
			return err
		}
	}
	return nil
}

// StartROABackgroundRefresher runs a periodic background task to refresh ROA URL caches
func (m *Manager) StartROABackgroundRefresher(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = m.RefreshAllROA(ctx, true)
			}
		}
	}()
}

func actionPriority(t config.ActionType) int {
	switch t {
	case config.ActionDeleteConfig, config.ActionDownInterface:
		return 1
	case config.ActionCreateConfig, config.ActionUpdateConfig, config.ActionSyncConfig, config.ActionUpInterface:
		return 2
	case config.ActionSyncRoaConfig:
		return 3
	case config.ActionSyncBirdConfig:
		return 4
	case config.ActionSyncNftablesConfig:
		return 5
	default:
		return 6
	}
}

// GetDNSConfig returns current DNS integration configuration
func (m *Manager) GetDNSConfig() config.DNSConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.store.Get()
	if cfg == nil {
		return config.DNSConfig{}
	}
	return cfg.DNS
}

// UpdateDNSConfig updates the DNS integration configuration
func (m *Manager) UpdateDNSConfig(dnsCfg config.DNSConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	if cfg == nil {
		return config.ErrConfigNotFound
	}
	dnsCfg.ZoneID = strings.TrimSpace(dnsCfg.ZoneID)
	dnsCfg.APIToken = strings.TrimSpace(dnsCfg.APIToken)
	dnsCfg.BaseDomain = strings.TrimSpace(dnsCfg.BaseDomain)
	cfg.DNS = dnsCfg
	return m.store.Save(cfg)
}

// SyncDNS performs manual or force synchronization of all nodes to Cloudflare DNS
func (m *Manager) SyncDNS(ctx context.Context, force bool) (*dns.SyncResult, error) {
	m.mu.RLock()
	cfg := m.store.Get()
	if cfg == nil {
		m.mu.RUnlock()
		return nil, config.ErrConfigNotFound
	}
	dnsCfg := cfg.DNS
	nodes := make([]config.Node, len(cfg.Nodes))
	copy(nodes, cfg.Nodes)
	m.mu.RUnlock()

	if !dnsCfg.IsConfigured() {
		return nil, errors.New("Cloudflare DNS is not configured. Please specify Zone ID, API Token, and Base Domain.")
	}

	client := dns.NewClient(dnsCfg.ZoneID, dnsCfg.APIToken, dnsCfg.BaseDomain)
	return dns.Sync(ctx, client, nodes, dnsCfg, force)
}

func (m *Manager) triggerDNSUpdateForNode(dnsCfg config.DNSConfig, node config.Node, oldName string) {
	if !dnsCfg.IsConfigured() {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		client := dns.NewClient(dnsCfg.ZoneID, dnsCfg.APIToken, dnsCfg.BaseDomain)
		if err := dns.SyncNode(ctx, client, node, oldName, dnsCfg); err != nil {
			log.Printf("[CF DNS] Error updating DNS for node %s: %v", node.Name, err)
		}
	}()
}

func (m *Manager) triggerDNSDeleteForNode(dnsCfg config.DNSConfig, nodeName string) {
	if !dnsCfg.IsConfigured() {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		client := dns.NewClient(dnsCfg.ZoneID, dnsCfg.APIToken, dnsCfg.BaseDomain)
		if err := dns.DeleteNodeRecords(ctx, client, nodeName, dnsCfg); err != nil {
			log.Printf("[CF DNS] Error deleting DNS for node %s: %v", nodeName, err)
		}
	}()
}

