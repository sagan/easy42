package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	agentpb "easy42/internal/agent/proto"
	"easy42/internal/config"

	"google.golang.org/protobuf/proto"
)

// TelemetryListener handles telemetry reports received by the hub
type TelemetryListener func(nodeName string, t *agentpb.TelemetryReport)

// RegisterListener handles new agent registrations
type RegisterListener func(conn *AgentConnection)

// CronTaskReportListener handles cron task reports from agents
type CronTaskReportListener func(nodeName string, r *agentpb.CronTaskReport)

// ProbeConfigFile represents a probed remote configuration file with path, content, and hash
type ProbeConfigFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Hash    string `json:"hash"`
}

// NodeMeta holds agent-reported registration metadata (OS info, init system,
// hostname, hosts hash, ...). It is persisted in the cache store so the values
// remain available while the agent is offline — e.g. right after a server
// restart, before the agent reconnects and re-registers. The sync plan needs
// os_info / init_system to build app specs identical to the deployed ones;
// without them every app would look pending after a restart.
type NodeMeta struct {
	AgentVersion string `json:"agent_version,omitempty"`
	OSInfo       string `json:"os_info,omitempty"`
	InitSystem   string `json:"init_system,omitempty"`
	Hostname     string `json:"hostname,omitempty"`
	HostsHash    string `json:"hosts_hash,omitempty"`
	DisableApps  bool   `json:"disable_apps,omitempty"`
	// SSHKeys are the root user SSH public keys collected by the agent.
	SSHKeys []config.SSHPublicKey `json:"ssh_keys,omitempty"`
	// SSHKeyFileHashes maps managed SSH key file paths (known_hosts /
	// authorized_keys) to the hash of their Easy42 block on the node.
	SSHKeyFileHashes map[string]string `json:"ssh_key_file_hashes,omitempty"`
}

// Hub manages active agent connections and cached telemetry
type Hub struct {
	mu               sync.RWMutex
	connections      map[string]*AgentConnection
	telemetry        map[string]*agentpb.TelemetryReport
	lastSeen         map[string]time.Time
	appHashes        map[string]map[string]string          // nodeName -> appName -> sha256
	appFiles         map[string]map[string]ProbeConfigFile // nodeName -> filePath -> ProbeConfigFile
	appEnabled       map[string]map[string]bool            // nodeName -> appName -> last applied enabled state
	probeConfigs     map[string]map[string]ProbeConfigFile // nodeName -> interface -> last probed WireGuard config
	lastLiveReport   map[string]time.Time                  // nodeName -> last agent-initiated report (register/telemetry)
	nodeMeta         map[string]*NodeMeta                  // nodeName -> persisted agent registration metadata
	store            *CacheStore                           // optional persistent cache (survives restarts)
	onTelemetry      []TelemetryListener
	onRegister       []RegisterListener
	onCronTaskReport []CronTaskReportListener
}

// Cache key prefixes used by the persistent hub cache.
const (
	cacheKeyAppHashPrefix     = "app_hash:"
	cacheKeyAppFilePrefix     = "app_file:"
	cacheKeyAppEnabledPrefix  = "app_enabled:"
	cacheKeyProbeConfigPrefix = "probe_config:"
	cacheKeyLastLiveReport    = "last_live_report"
	cacheKeyTelemetry         = "telemetry"
	cacheKeyNodeMeta          = "meta"
)

// NewHub creates a new Hub instance
func NewHub() *Hub {
	return &Hub{
		connections:    make(map[string]*AgentConnection),
		telemetry:      make(map[string]*agentpb.TelemetryReport),
		lastSeen:       make(map[string]time.Time),
		appHashes:      make(map[string]map[string]string),
		appFiles:       make(map[string]map[string]ProbeConfigFile),
		appEnabled:     make(map[string]map[string]bool),
		probeConfigs:   make(map[string]map[string]ProbeConfigFile),
		lastLiveReport: make(map[string]time.Time),
		nodeMeta:       make(map[string]*NodeMeta),
	}
}

// SetCacheStore attaches a persistent cache store and loads previously cached
// entries into memory. Every subsequent cache mutation is written through to
// the store so the caches survive a server restart.
func (h *Hub) SetCacheStore(store *CacheStore) error {
	if store == nil {
		return nil
	}
	all, err := store.LoadAll()
	if err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.store = store

	for nodeName, entries := range all {
		for key, value := range entries {
			switch {
			case strings.HasPrefix(key, cacheKeyAppHashPrefix):
				if h.appHashes[nodeName] == nil {
					h.appHashes[nodeName] = make(map[string]string)
				}
				h.appHashes[nodeName][strings.TrimPrefix(key, cacheKeyAppHashPrefix)] = value
			case strings.HasPrefix(key, cacheKeyAppEnabledPrefix):
				if h.appEnabled[nodeName] == nil {
					h.appEnabled[nodeName] = make(map[string]bool)
				}
				h.appEnabled[nodeName][strings.TrimPrefix(key, cacheKeyAppEnabledPrefix)] = value == "true"
			case strings.HasPrefix(key, cacheKeyAppFilePrefix):
				var f ProbeConfigFile
				if json.Unmarshal([]byte(value), &f) == nil {
					if h.appFiles[nodeName] == nil {
						h.appFiles[nodeName] = make(map[string]ProbeConfigFile)
					}
					h.appFiles[nodeName][strings.TrimPrefix(key, cacheKeyAppFilePrefix)] = f
				}
			case strings.HasPrefix(key, cacheKeyProbeConfigPrefix):
				var f ProbeConfigFile
				if json.Unmarshal([]byte(value), &f) == nil {
					if h.probeConfigs[nodeName] == nil {
						h.probeConfigs[nodeName] = make(map[string]ProbeConfigFile)
					}
					h.probeConfigs[nodeName][strings.TrimPrefix(key, cacheKeyProbeConfigPrefix)] = f
				}
			case key == cacheKeyLastLiveReport:
				if unix, err := strconv.ParseInt(value, 10, 64); err == nil && unix > 0 {
					h.lastLiveReport[nodeName] = time.Unix(unix, 0)
				}
			case key == cacheKeyTelemetry:
				var t agentpb.TelemetryReport
				if proto.Unmarshal([]byte(value), &t) == nil {
					h.telemetry[nodeName] = &t
				}
			case key == cacheKeyNodeMeta:
				var meta NodeMeta
				if json.Unmarshal([]byte(value), &meta) == nil {
					h.nodeMeta[nodeName] = &meta
				}
			}
		}
	}
	return nil
}

// persistLocked writes a cache entry to the persistent store. Caller holds mu.
func (h *Hub) persistLocked(nodeName, key, value string) {
	if h.store == nil {
		return
	}
	_ = h.store.Set(nodeName, key, value)
}

// deletePersistedLocked removes a cache entry from the persistent store. Caller holds mu.
func (h *Hub) deletePersistedLocked(nodeName, key string) {
	if h.store == nil {
		return
	}
	_ = h.store.Delete(nodeName, key)
}

// deletePersistedPrefixLocked removes cache entries with a key prefix. Caller holds mu.
func (h *Hub) deletePersistedPrefixLocked(nodeName, prefix string) {
	if h.store == nil {
		return
	}
	_ = h.store.DeletePrefix(nodeName, prefix)
}

// deletePersistedNodeLocked removes every cache entry of a node. Caller holds mu.
func (h *Hub) deletePersistedNodeLocked(nodeName string) {
	if h.store == nil {
		return
	}
	_ = h.store.DeleteNode(nodeName)
}

// persistMetaLocked writes the node metadata entry to the persistent store. Caller holds mu.
func (h *Hub) persistMetaLocked(nodeName string, meta *NodeMeta) {
	if data, err := json.Marshal(meta); err == nil {
		h.persistLocked(nodeName, cacheKeyNodeMeta, string(data))
	}
}

// SetNodeMeta replaces the registration metadata of a node (called on agent
// registration) and persists it so it survives a server restart.
func (h *Hub) SetNodeMeta(nodeName string, meta NodeMeta) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.nodeMeta == nil {
		h.nodeMeta = make(map[string]*NodeMeta)
	}
	cp := meta
	h.nodeMeta[nodeName] = &cp
	h.persistMetaLocked(nodeName, &cp)
}

// GetNodeMeta returns a copy of the persisted registration metadata of a node,
// or nil if the agent never registered (even before a restart).
func (h *Hub) GetNodeMeta(nodeName string) *NodeMeta {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if meta := h.nodeMeta[nodeName]; meta != nil {
		cp := *meta
		return &cp
	}
	return nil
}

// UpdateHostsHash records the latest hosts hash reported by a node (via
// heartbeat or telemetry) and persists it so it is available while offline.
func (h *Hub) UpdateHostsHash(nodeName, hash string) {
	if hash == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.nodeMeta == nil {
		h.nodeMeta = make(map[string]*NodeMeta)
	}
	meta := h.nodeMeta[nodeName]
	if meta == nil {
		meta = &NodeMeta{}
		h.nodeMeta[nodeName] = meta
	}
	if meta.HostsHash == hash {
		return
	}
	meta.HostsHash = hash
	h.persistMetaLocked(nodeName, meta)
}

// OnTelemetry registers a listener invoked when telemetry is received
func (h *Hub) OnTelemetry(l TelemetryListener) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onTelemetry = append(h.onTelemetry, l)
}

// OnRegister registers a listener invoked when an agent registers
func (h *Hub) OnRegister(l RegisterListener) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onRegister = append(h.onRegister, l)
}

// OnCronTaskReport registers a listener invoked when a cron task result report is received
func (h *Hub) OnCronTaskReport(l CronTaskReportListener) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onCronTaskReport = append(h.onCronTaskReport, l)
}

// RecordCronTaskReport notifies listeners of an incoming cron task result
func (h *Hub) RecordCronTaskReport(nodeName string, r *agentpb.CronTaskReport) {
	h.mu.RLock()
	listeners := append([]CronTaskReportListener(nil), h.onCronTaskReport...)
	h.mu.RUnlock()

	for _, l := range listeners {
		go l(nodeName, r)
	}
}

// SyncCronTasks sends the latest cron tasks to a specific node agent if connected
func (h *Hub) SyncCronTasks(nodeName string, tasks []*agentpb.CronTaskSpec) error {
	conn, ok := h.GetConnection(nodeName)
	if !ok {
		return nil // Agent not connected; will receive on next connect
	}
	return conn.SyncCronTasks(tasks)
}

// SyncHosts sends the latest cluster hosts to a specific node agent if connected
func (h *Hub) SyncHosts(nodeName string, hosts []*agentpb.HostRecord) error {
	conn, ok := h.GetConnection(nodeName)
	if !ok {
		return nil // Agent not connected; will receive on next connect
	}
	return conn.SyncHosts(hosts)
}

// SyncApps sends the latest apps to a specific node agent if connected
func (h *Hub) SyncApps(nodeName string, apps []*agentpb.AppSpec) error {
	conn, ok := h.GetConnection(nodeName)
	if !ok {
		return nil // Agent not connected; will receive on next connect
	}
	return conn.SyncApps(apps)
}

// SyncObfuscators sends the latest WireGuard obfuscator desired state to a specific node agent if connected
func (h *Hub) SyncObfuscators(nodeName string, specs []*agentpb.ObfuscatorSpec) error {
	conn, ok := h.GetConnection(nodeName)
	if !ok {
		return nil // Agent not connected; will receive on next connect
	}
	return conn.SyncObfuscators(specs)
}

// GetHostsHash returns the latest reported hosts hash for a node, falling back
// to the persisted metadata (and telemetry) when the agent is offline.
func (h *Hub) GetHostsHash(nodeName string) string {
	if conn, ok := h.GetConnection(nodeName); ok {
		if hash := conn.HostsHash(); hash != "" {
			return hash
		}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if meta := h.nodeMeta[nodeName]; meta != nil && meta.HostsHash != "" {
		return meta.HostsHash
	}
	if t := h.telemetry[nodeName]; t != nil {
		return t.HostsHash
	}
	return ""
}

// GetOSInfo returns the reported OS info for a node agent, falling back to the
// persisted registration metadata when the agent is offline.
func (h *Hub) GetOSInfo(nodeName string) string {
	if conn, ok := h.GetConnection(nodeName); ok {
		if v := conn.OSInfo(); v != "" {
			return v
		}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if meta := h.nodeMeta[nodeName]; meta != nil {
		return meta.OSInfo
	}
	return ""
}

// GetInitSystem returns the init system reported by a node agent
// ("systemd", "procd" or "openrc"), falling back to the persisted registration
// metadata when the agent is offline. It returns "" for nodes that never
// reported and for agents that predate the field.
func (h *Hub) GetInitSystem(nodeName string) string {
	if conn, ok := h.GetConnection(nodeName); ok {
		if v := conn.InitSystem(); v != "" {
			return v
		}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if meta := h.nodeMeta[nodeName]; meta != nil {
		return meta.InitSystem
	}
	return ""
}

// GetHostname returns the reported hostname of a node agent, falling back to
// the persisted registration metadata when the agent is offline.
func (h *Hub) GetHostname(nodeName string) string {
	if conn, ok := h.GetConnection(nodeName); ok {
		if v := conn.Hostname(); v != "" {
			return v
		}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if meta := h.nodeMeta[nodeName]; meta != nil {
		return meta.Hostname
	}
	return ""
}

// GetAgentVersion returns the reported agent version of a node, falling back to
// the persisted registration metadata when the agent is offline.
func (h *Hub) GetAgentVersion(nodeName string) string {
	if conn, ok := h.GetConnection(nodeName); ok {
		if v := conn.AgentVersion(); v != "" {
			return v
		}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if meta := h.nodeMeta[nodeName]; meta != nil {
		return meta.AgentVersion
	}
	return ""
}

// IsAppsDisabled returns true if the node agent has disabled apps in its configuration
func (h *Hub) IsAppsDisabled(nodeName string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if conn, ok := h.connections[nodeName]; ok {
		return conn.DisableApps()
	}
	if meta := h.nodeMeta[nodeName]; meta != nil {
		return meta.DisableApps
	}
	if t, ok := h.telemetry[nodeName]; ok {
		return t.DisableApps
	}
	return false
}

// SetAppHashes updates the cached deployed app hashes for a node
func (h *Hub) SetAppHashes(nodeName string, hashes map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.appHashes == nil {
		h.appHashes = make(map[string]map[string]string)
	}
	cp := make(map[string]string, len(hashes))
	persist := make(map[string]string, len(hashes))
	for k, v := range hashes {
		cp[k] = v
		persist[cacheKeyAppHashPrefix+k] = v
	}
	h.appHashes[nodeName] = cp
	h.deletePersistedPrefixLocked(nodeName, cacheKeyAppHashPrefix)
	if h.store != nil && len(persist) > 0 {
		_ = h.store.SetMany(nodeName, persist)
	}
}

// GetAppHashes returns a copy of the cached deployed app hashes for a node
func (h *Hub) GetAppHashes(nodeName string) map[string]string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.appHashes == nil {
		return nil
	}
	res := make(map[string]string)
	for k, v := range h.appHashes[nodeName] {
		res[k] = v
	}
	return res
}

// SetAppHash updates the cached deployed hash of a specific app on a node
func (h *Hub) SetAppHash(nodeName, appName, hash string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.appHashes == nil {
		h.appHashes = make(map[string]map[string]string)
	}
	if h.appHashes[nodeName] == nil {
		h.appHashes[nodeName] = make(map[string]string)
	}
	h.appHashes[nodeName][appName] = hash
	h.persistLocked(nodeName, cacheKeyAppHashPrefix+appName, hash)
}

// RemoveAppHash removes the cached deployed hash of a specific app on a node
func (h *Hub) RemoveAppHash(nodeName, appName string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.appHashes == nil || h.appHashes[nodeName] == nil {
		return
	}
	delete(h.appHashes[nodeName], appName)
	h.deletePersistedLocked(nodeName, cacheKeyAppHashPrefix+appName)
}

// GetAppHash returns the cached deployed hash of a specific app on a node
func (h *Hub) GetAppHash(nodeName, appName string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.appHashes == nil {
		return ""
	}
	return h.appHashes[nodeName][appName]
}

// SetAppEnabled records the enabled state that was last applied for an app on a node.
// The agent probes cannot infer this from files alone, so it is tracked separately to
// detect enable/disable drift during sync planning.
func (h *Hub) SetAppEnabled(nodeName, appName string, enabled bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.appEnabled == nil {
		h.appEnabled = make(map[string]map[string]bool)
	}
	if h.appEnabled[nodeName] == nil {
		h.appEnabled[nodeName] = make(map[string]bool)
	}
	h.appEnabled[nodeName][appName] = enabled
	h.persistLocked(nodeName, cacheKeyAppEnabledPrefix+appName, strconv.FormatBool(enabled))
}

// GetAppEnabled returns the last applied enabled state and whether it is known.
func (h *Hub) GetAppEnabled(nodeName, appName string) (bool, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.appEnabled == nil || h.appEnabled[nodeName] == nil {
		return false, false
	}
	enabled, ok := h.appEnabled[nodeName][appName]
	return enabled, ok
}

// GetAppEnabledMap returns a copy of the last applied enabled states for a node.
func (h *Hub) GetAppEnabledMap(nodeName string) map[string]bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make(map[string]bool)
	for k, v := range h.appEnabled[nodeName] {
		res[k] = v
	}
	return res
}

// RemoveAppEnabled clears the recorded enabled state for an app on a node.
func (h *Hub) RemoveAppEnabled(nodeName, appName string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.appEnabled == nil || h.appEnabled[nodeName] == nil {
		return
	}
	delete(h.appEnabled[nodeName], appName)
	h.deletePersistedLocked(nodeName, cacheKeyAppEnabledPrefix+appName)
}

// SetAppFiles updates the cached deployed app files for a node
func (h *Hub) SetAppFiles(nodeName string, files map[string]ProbeConfigFile) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.appFiles == nil {
		h.appFiles = make(map[string]map[string]ProbeConfigFile)
	}
	cp := make(map[string]ProbeConfigFile, len(files))
	persist := make(map[string]string, len(files))
	for k, v := range files {
		cp[k] = v
		if data, err := json.Marshal(v); err == nil {
			persist[cacheKeyAppFilePrefix+k] = string(data)
		}
	}
	h.appFiles[nodeName] = cp
	h.deletePersistedPrefixLocked(nodeName, cacheKeyAppFilePrefix)
	if h.store != nil && len(persist) > 0 {
		_ = h.store.SetMany(nodeName, persist)
	}
}

// SetAppFile updates or adds a single cached deployed app file for a node
func (h *Hub) SetAppFile(nodeName string, filePath string, file ProbeConfigFile) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.appFiles == nil {
		h.appFiles = make(map[string]map[string]ProbeConfigFile)
	}
	if h.appFiles[nodeName] == nil {
		h.appFiles[nodeName] = make(map[string]ProbeConfigFile)
	}
	h.appFiles[nodeName][filePath] = file
	if data, err := json.Marshal(file); err == nil {
		h.persistLocked(nodeName, cacheKeyAppFilePrefix+filePath, string(data))
	}
}

// RemoveAppFile removes a cached deployed app file for a node
func (h *Hub) RemoveAppFile(nodeName string, filePath string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.appFiles == nil || h.appFiles[nodeName] == nil {
		return
	}
	delete(h.appFiles[nodeName], filePath)
	h.deletePersistedLocked(nodeName, cacheKeyAppFilePrefix+filePath)
}

// GetAppFiles returns a copy of the cached deployed app files for a node
func (h *Hub) GetAppFiles(nodeName string) map[string]ProbeConfigFile {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.appFiles == nil {
		return nil
	}
	orig := h.appFiles[nodeName]
	if orig == nil {
		return nil
	}
	cp := make(map[string]ProbeConfigFile, len(orig))
	for k, v := range orig {
		cp[k] = v
	}
	return cp
}

// RecordLiveReport marks that the agent for nodeName has pushed live data
// (registration or telemetry) at the current time.
func (h *Hub) RecordLiveReport(nodeName string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.lastLiveReport == nil {
		h.lastLiveReport = make(map[string]time.Time)
	}
	now := time.Now()
	h.lastLiveReport[nodeName] = now
	h.persistLocked(nodeName, cacheKeyLastLiveReport, strconv.FormatInt(now.Unix(), 10))
}

// LastLiveReport returns when the agent for nodeName last pushed live data.
// The zero time means the agent has not reported since the server started.
func (h *Hub) LastLiveReport(nodeName string) time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastLiveReport[nodeName]
}

// SetProbeConfigs caches the WireGuard config files reported by a node agent
func (h *Hub) SetProbeConfigs(nodeName string, configs map[string]ProbeConfigFile) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.probeConfigs == nil {
		h.probeConfigs = make(map[string]map[string]ProbeConfigFile)
	}
	h.deletePersistedPrefixLocked(nodeName, cacheKeyProbeConfigPrefix)
	if len(configs) == 0 {
		delete(h.probeConfigs, nodeName)
		return
	}
	cp := make(map[string]ProbeConfigFile, len(configs))
	persist := make(map[string]string, len(configs))
	for k, v := range configs {
		cp[k] = v
		if data, err := json.Marshal(v); err == nil {
			persist[cacheKeyProbeConfigPrefix+k] = string(data)
		}
	}
	h.probeConfigs[nodeName] = cp
	if h.store != nil && len(persist) > 0 {
		_ = h.store.SetMany(nodeName, persist)
	}
}

// GetProbeConfigs returns a copy of the cached WireGuard config files for a node
func (h *Hub) GetProbeConfigs(nodeName string) map[string]ProbeConfigFile {
	h.mu.RLock()
	defer h.mu.RUnlock()
	orig := h.probeConfigs[nodeName]
	if orig == nil {
		return nil
	}
	cp := make(map[string]ProbeConfigFile, len(orig))
	for k, v := range orig {
		cp[k] = v
	}
	return cp
}

// sshKeysEqual reports whether two collected key sets are identical.
func sshKeysEqual(a, b []config.SSHPublicKey) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Normalize() != b[i].Normalize() {
			return false
		}
	}
	return true
}

// NotifyRegister triggers registered registration listeners
func (h *Hub) NotifyRegister(conn *AgentConnection) {
	h.mu.RLock()
	listeners := append([]RegisterListener(nil), h.onRegister...)
	h.mu.RUnlock()

	for _, l := range listeners {
		go l(conn)
	}
}

// Register registers an active connection for a node
func (h *Hub) Register(conn *AgentConnection) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// If old connection exists, close it
	if old, exists := h.connections[conn.NodeName()]; exists && old != conn {
		go old.Close()
	}

	h.connections[conn.NodeName()] = conn
	h.lastSeen[conn.NodeName()] = time.Now()
}

// Unregister removes a connection for a node if it matches the active connection
func (h *Hub) Unregister(conn *AgentConnection) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if current, exists := h.connections[conn.NodeName()]; exists && current == conn {
		delete(h.connections, conn.NodeName())
	}
}

// UnregisterNode removes the active connection for a node by name
func (h *Hub) UnregisterNode(nodeName string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.connections, nodeName)
}

// RemoveNode closes any active connection, removes the node from connections, telemetry, and lastSeen
func (h *Hub) RemoveNode(nodeName string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conn, exists := h.connections[nodeName]; exists {
		go conn.Close()
		delete(h.connections, nodeName)
	}
	delete(h.telemetry, nodeName)
	delete(h.lastSeen, nodeName)
	delete(h.appHashes, nodeName)
	delete(h.appFiles, nodeName)
	delete(h.appEnabled, nodeName)
	delete(h.probeConfigs, nodeName)
	delete(h.lastLiveReport, nodeName)
	delete(h.nodeMeta, nodeName)
	h.deletePersistedNodeLocked(nodeName)
}

// IsConnected returns whether the node agent is currently connected and active
func (h *Hub) IsConnected(nodeName string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, exists := h.connections[nodeName]
	if !exists {
		return false
	}
	if lastSeen, ok := h.lastSeen[nodeName]; ok && !lastSeen.IsZero() {
		if time.Since(lastSeen) > 45*time.Second {
			return false
		}
	}
	return true
}

// GetConnection retrieves the active connection for a node
func (h *Hub) GetConnection(nodeName string) (*AgentConnection, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conn, exists := h.connections[nodeName]
	if !exists {
		return nil, false
	}
	if lastSeen, ok := h.lastSeen[nodeName]; ok && !lastSeen.IsZero() {
		if time.Since(lastSeen) > 45*time.Second {
			return nil, false
		}
	}
	return conn, true
}

// RecordHeartbeat records the last heartbeat time for a node
func (h *Hub) RecordHeartbeat(nodeName string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastSeen[nodeName] = time.Now()
}

// UpdateTelemetry stores the latest telemetry report for a node
func (h *Hub) UpdateTelemetry(nodeName string, t *agentpb.TelemetryReport) {
	h.mu.Lock()
	h.telemetry[nodeName] = t
	h.lastSeen[nodeName] = time.Now()
	if data, err := proto.Marshal(t); err == nil {
		h.persistLocked(nodeName, cacheKeyTelemetry, string(data))
	}
	// Keep the persisted registration metadata in sync with the fields carried
	// by telemetry so they remain available while the agent is offline.
	if h.nodeMeta == nil {
		h.nodeMeta = make(map[string]*NodeMeta)
	}
	meta := h.nodeMeta[nodeName]
	if meta == nil && (t.HostsHash != "" || t.DisableApps || len(t.SshKeys) > 0) {
		meta = &NodeMeta{}
		h.nodeMeta[nodeName] = meta
	}
	if meta != nil {
		metaChanged := false
		if t.HostsHash != "" && meta.HostsHash != t.HostsHash {
			meta.HostsHash = t.HostsHash
			metaChanged = true
		}
		if meta.DisableApps != t.DisableApps {
			meta.DisableApps = t.DisableApps
			metaChanged = true
		}
		// Agents that support SSH key collection always report the complete
		// key set, so an empty list means "no keys".
		var reportedKeys []config.SSHPublicKey
		for _, k := range t.SshKeys {
			reportedKeys = append(reportedKeys, config.SSHPublicKey{
				Algorithm: k.Algorithm,
				Key:       k.Key,
				Comment:   k.Comment,
				Path:      k.Path,
			})
		}
		if len(reportedKeys) == 0 {
			reportedKeys = nil
		}
		if !sshKeysEqual(meta.SSHKeys, reportedKeys) {
			meta.SSHKeys = reportedKeys
			metaChanged = true
		}
		if metaChanged {
			h.persistMetaLocked(nodeName, meta)
		}
	}
	listeners := append([]TelemetryListener(nil), h.onTelemetry...)
	h.mu.Unlock()

	for _, l := range listeners {
		go l(nodeName, t)
	}
}

// SetSSHKeys replaces the collected root SSH public keys of a node and persists
// them so they stay available while the agent is offline.
func (h *Hub) SetSSHKeys(nodeName string, keys []config.SSHPublicKey) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.nodeMeta == nil {
		h.nodeMeta = make(map[string]*NodeMeta)
	}
	meta := h.nodeMeta[nodeName]
	if meta == nil {
		meta = &NodeMeta{}
		h.nodeMeta[nodeName] = meta
	}
	cp := make([]config.SSHPublicKey, len(keys))
	for i, k := range keys {
		cp[i] = k.Normalize()
	}
	if len(cp) == 0 {
		cp = nil
	}
	meta.SSHKeys = cp
	h.persistMetaLocked(nodeName, meta)
}

// GetSSHKeys returns the collected root SSH public keys of a node.
func (h *Hub) GetSSHKeys(nodeName string) []config.SSHPublicKey {
	h.mu.RLock()
	defer h.mu.RUnlock()
	meta := h.nodeMeta[nodeName]
	if meta == nil || len(meta.SSHKeys) == 0 {
		return nil
	}
	res := make([]config.SSHPublicKey, len(meta.SSHKeys))
	copy(res, meta.SSHKeys)
	return res
}

// SetSSHKeyFileHashes records the reported hashes of the Easy42 blocks of the
// managed SSH key files of a node and persists them.
func (h *Hub) SetSSHKeyFileHashes(nodeName string, hashes map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.nodeMeta == nil {
		h.nodeMeta = make(map[string]*NodeMeta)
	}
	meta := h.nodeMeta[nodeName]
	if meta == nil {
		meta = &NodeMeta{}
		h.nodeMeta[nodeName] = meta
	}
	if meta.SSHKeyFileHashes == nil {
		meta.SSHKeyFileHashes = make(map[string]string)
	}
	changed := false
	for path, hash := range hashes {
		if meta.SSHKeyFileHashes[path] != hash {
			meta.SSHKeyFileHashes[path] = hash
			changed = true
		}
	}
	if changed {
		h.persistMetaLocked(nodeName, meta)
	}
}

// SetSSHKeyFileHash records the reported hash of the Easy42 block of a single
// managed SSH key file of a node.
func (h *Hub) SetSSHKeyFileHash(nodeName, path, hash string) {
	h.SetSSHKeyFileHashes(nodeName, map[string]string{path: hash})
}

// GetSSHKeyFileHashes returns the reported hashes of the managed SSH key files
// of a node (falling back to the persisted registration metadata when offline).
func (h *Hub) GetSSHKeyFileHashes(nodeName string) map[string]string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make(map[string]string)
	if meta := h.nodeMeta[nodeName]; meta != nil {
		for k, v := range meta.SSHKeyFileHashes {
			res[k] = v
		}
	}
	return res
}

// GetTelemetry retrieves the latest telemetry report for a node
func (h *Hub) GetTelemetry(nodeName string) *agentpb.TelemetryReport {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.telemetry[nodeName]
}

// GetAllTelemetry returns a snapshot copy of all cached node telemetry reports
func (h *Hub) GetAllTelemetry() map[string]*agentpb.TelemetryReport {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make(map[string]*agentpb.TelemetryReport, len(h.telemetry))
	for k, v := range h.telemetry {
		res[k] = v
	}
	return res
}

// GetAllConnections returns a snapshot copy of all active agent connections
func (h *Hub) GetAllConnections() map[string]*AgentConnection {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make(map[string]*AgentConnection, len(h.connections))
	for k, v := range h.connections {
		res[k] = v
	}
	return res
}

// GetLastSeen returns the last time an agent sent telemetry or heartbeat
func (h *Hub) GetLastSeen(nodeName string) time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastSeen[nodeName]
}

// SendCommand dispatches a command to the specified node's agent
func (h *Hub) SendCommand(ctx context.Context, nodeName string, cmd *agentpb.CommandRequest) (*agentpb.CommandResponse, error) {
	conn, ok := h.GetConnection(nodeName)
	if !ok {
		return nil, fmt.Errorf("node %s is not connected via agent", nodeName)
	}
	return conn.SendCommand(ctx, cmd)
}

// NodeStatus converts cached agent telemetry to config.NodeStatus
func (h *Hub) NodeStatus(nodeName string) *config.NodeStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	conn, connected := h.connections[nodeName]
	lastSeen := h.lastSeen[nodeName]
	telemetry := h.telemetry[nodeName]

	status := &config.NodeStatus{
		Name:         nodeName,
		Connected:    connected,
		LastSeen:     lastSeen,
		Mode:         "agent",
		Interfaces:   make([]config.InterfaceInfo, 0),
		WgInterfaces: make([]config.WgInterfaceStatus, 0),
	}

	if conn != nil {
		status.Hostname = conn.Hostname()
		status.AgentVersion = conn.AgentVersion()
	} else if meta := h.nodeMeta[nodeName]; meta != nil {
		// Offline agent: fall back to the persisted registration metadata.
		status.Hostname = meta.Hostname
		status.AgentVersion = meta.AgentVersion
	}

	if telemetry != nil {
		if telemetry.System != nil {
			status.Metrics = &config.SystemMetrics{
				CPUPercent:       telemetry.System.CpuPercent,
				MemoryUsedBytes:  telemetry.System.MemoryUsedBytes,
				MemoryTotalBytes: telemetry.System.MemoryTotalBytes,
				UptimeSeconds:    telemetry.System.UptimeSeconds,
				LoadAvg:          telemetry.System.LoadAvg,
			}
		}

		for _, iface := range telemetry.Interfaces {
			info := config.InterfaceInfo{
				Name:      iface.Name,
				Up:        iface.IsUp,
				Addresses: iface.Addresses,
				Flags:     iface.Flags,
				RxBytes:   iface.RxBytes,
				TxBytes:   iface.TxBytes,
			}
			if (iface.Flags & config.IfaceFlagPrimary) != 0 {
				status.PrimaryInterface = iface.Name
			}
			status.Interfaces = append(status.Interfaces, info)
		}

		for _, disk := range telemetry.Disks {
			dm := config.DiskMetrics{
				Path:       disk.Path,
				TotalBytes: disk.TotalBytes,
				UsedBytes:  disk.UsedBytes,
				FreeBytes:  disk.FreeBytes,
			}
			status.Disks = append(status.Disks, dm)
			if status.Metrics == nil {
				status.Metrics = &config.SystemMetrics{}
			}
			status.Metrics.Disks = append(status.Metrics.Disks, dm)
		}

		// Group WireGuard peers by interface
		wgMap := make(map[string]*config.WgInterfaceStatus)
		for _, peer := range telemetry.WgPeers {
			wg, exists := wgMap[peer.InterfaceName]
			if !exists {
				wg = &config.WgInterfaceStatus{
					Name:  peer.InterfaceName,
					Peers: make([]config.WgPeerStatus, 0),
				}
				wgMap[peer.InterfaceName] = wg
			}

			var lastHandshake time.Time
			if peer.LastHandshakeTime > 0 {
				lastHandshake = time.Unix(peer.LastHandshakeTime, 0)
			}

			wg.Peers = append(wg.Peers, config.WgPeerStatus{
				PublicKey:           peer.PublicKey,
				Endpoint:            peer.Endpoint,
				LatestHandshake:     lastHandshake,
				TransferRxBytes:     int64(peer.RxBytes),
				TransferTxBytes:     int64(peer.TxBytes),
				PersistentKeepalive: int(peer.PersistentKeepalive),
			})
		}

		for _, wg := range wgMap {
			status.WgInterfaces = append(status.WgInterfaces, *wg)
		}
	}

	return status
}

// GenerateToken creates a secure random hex token for agent authorization
func GenerateToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("e42_%d", time.Now().UnixNano())
	}
	return "e42_" + hex.EncodeToString(b)
}
