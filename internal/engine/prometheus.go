package engine

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"easy42/internal/agent"
	agentpb "easy42/internal/agent/proto"
	"easy42/internal/config"
	"easy42/internal/version"
)

// PrometheusExporter generates and caches Prometheus text exposition metrics for the easy42 cluster
type PrometheusExporter struct {
	mgr        *Manager
	mu         sync.RWMutex
	cachedData []byte
	cachedAt   time.Time
	cacheTTL   time.Duration
}

// NewPrometheusExporter creates a new PrometheusExporter instance with a 2-second in-memory state cache
func NewPrometheusExporter(mgr *Manager) *PrometheusExporter {
	return &PrometheusExporter{
		mgr:      mgr,
		cacheTTL: 2 * time.Second,
	}
}

// Invalidate clears the in-memory cache, forcing next scrape to re-render fresh metrics
func (e *PrometheusExporter) Invalidate() {
	e.mu.Lock()
	e.cachedData = nil
	e.mu.Unlock()
}

// Render returns cached Prometheus exposition bytes or renders them from live in-memory state
func (e *PrometheusExporter) Render() ([]byte, error) {
	e.mu.RLock()
	if e.cachedData != nil && time.Since(e.cachedAt) < e.cacheTTL {
		data := e.cachedData
		e.mu.RUnlock()
		return data, nil
	}
	e.mu.RUnlock()

	e.mu.Lock()
	defer e.mu.Unlock()

	// Double-checked locking
	if e.cachedData != nil && time.Since(e.cachedAt) < e.cacheTTL {
		return e.cachedData, nil
	}

	start := time.Now()
	data := e.generateMetrics(start)
	e.cachedData = data
	e.cachedAt = time.Now()
	return data, nil
}

type metricSample struct {
	labels string
	value  string
}

type metricFamily struct {
	name    string
	help    string
	mType   string // "gauge", "counter"
	samples []metricSample
}

type metricCollector struct {
	families map[string]*metricFamily
	order    []string
}

func newMetricCollector() *metricCollector {
	return &metricCollector{
		families: make(map[string]*metricFamily),
	}
}

func (c *metricCollector) register(name, help, mType string) {
	if _, exists := c.families[name]; !exists {
		c.families[name] = &metricFamily{
			name:  name,
			help:  help,
			mType: mType,
		}
		c.order = append(c.order, name)
	}
}

func (c *metricCollector) addSample(name string, labels [][2]string, value float64) {
	f, exists := c.families[name]
	if !exists {
		return
	}
	f.samples = append(f.samples, metricSample{
		labels: formatLabels(labels),
		value:  strconv.FormatFloat(value, 'f', -1, 64),
	})
}

func (c *metricCollector) addSampleInt(name string, labels [][2]string, value int64) {
	f, exists := c.families[name]
	if !exists {
		return
	}
	f.samples = append(f.samples, metricSample{
		labels: formatLabels(labels),
		value:  strconv.FormatInt(value, 10),
	})
}

func (c *metricCollector) addSampleUint(name string, labels [][2]string, value uint64) {
	f, exists := c.families[name]
	if !exists {
		return
	}
	f.samples = append(f.samples, metricSample{
		labels: formatLabels(labels),
		value:  strconv.FormatUint(value, 10),
	})
}

func escapeLabelValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

func formatLabels(labels [][2]string) string {
	var filtered [][2]string
	for _, l := range labels {
		if l[1] == "" {
			continue
		}
		filtered = append(filtered, l)
	}
	if len(filtered) == 0 {
		return ""
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i][0] < filtered[j][0]
	})
	var parts []string
	for _, l := range filtered {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, l[0], escapeLabelValue(l[1])))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func (e *PrometheusExporter) generateMetrics(startTime time.Time) []byte {
	c := newMetricCollector()

	// 1. easy42 Exporter Metadata
	c.register("easy42_exporter_build_info", "easy42 Prometheus metrics exporter build and version information.", "gauge")
	c.register("easy42_scrape_duration_seconds", "Duration of the easy42 Prometheus scrape in seconds.", "gauge")
	c.register("easy42_total_nodes", "Total number of nodes configured in easy42.", "gauge")
	c.register("easy42_connected_nodes", "Number of nodes with active connected agent.", "gauge")
	c.register("easy42_total_links", "Total number of configured links in easy42.", "gauge")
	c.register("easy42_node_info", "Static metadata information for cluster node.", "gauge")
	c.register("easy42_node_up", "Status of node agent connection (1 = connected and active, 0 = offline).", "gauge")
	c.register("easy42_node_last_seen_timestamp_seconds", "Unix timestamp in seconds when node agent was last seen.", "gauge")
	c.register("easy42_link_up", "Operational status of configured node-to-node link (1 = up, 0 = down).", "gauge")

	// 2. Linux System (node_exporter compatible)
	c.register("node_load1", "1 minute load average.", "gauge")
	c.register("node_load5", "5 minute load average.", "gauge")
	c.register("node_load15", "15 minute load average.", "gauge")
	c.register("node_cpu_utilization_ratio", "CPU utilization ratio (0.0 to 1.0).", "gauge")
	c.register("node_cpu_percent", "CPU utilization percentage (0.0 to 100.0).", "gauge")
	c.register("node_memory_MemTotal_bytes", "Memory information field MemTotal_bytes.", "gauge")
	c.register("node_memory_MemFree_bytes", "Memory information field MemFree_bytes.", "gauge")
	c.register("node_memory_MemAvailable_bytes", "Memory information field MemAvailable_bytes.", "gauge")
	c.register("node_memory_used_bytes", "Memory used in bytes.", "gauge")
	c.register("node_uptime_seconds", "System uptime in seconds.", "gauge")
	c.register("node_boot_time_seconds", "Node boot time in Unix epoch seconds.", "gauge")
	c.register("node_filesystem_size_bytes", "Filesystem size in bytes.", "gauge")
	c.register("node_filesystem_free_bytes", "Filesystem free space in bytes.", "gauge")
	c.register("node_filesystem_avail_bytes", "Filesystem space available to non-root users in bytes.", "gauge")
	c.register("node_filesystem_used_bytes", "Filesystem used space in bytes.", "gauge")
	c.register("node_network_up", "Value is 1 if device is up, 0 if down.", "gauge")
	c.register("node_network_receive_bytes_total", "Network device statistic receive_bytes.", "counter")
	c.register("node_network_transmit_bytes_total", "Network device statistic transmit_bytes.", "counter")
	c.register("node_network_receive_packets_total", "Network device statistic receive_packets.", "counter")
	c.register("node_network_transmit_packets_total", "Network device statistic transmit_packets.", "counter")
	c.register("node_network_receive_errs_total", "Network device statistic receive_errs.", "counter")
	c.register("node_network_transmit_errs_total", "Network device statistic transmit_errs.", "counter")

	// 3. WireGuard (MindFlavor/prometheus_wireguard_exporter compatible)
	c.register("wireguard_interface_up", "1 if WireGuard interface is up, 0 otherwise.", "gauge")
	c.register("wireguard_latest_handshake_seconds", "UNIX timestamp seconds of the last completed WireGuard handshake.", "gauge")
	c.register("wireguard_latest_handshake_delay_seconds", "Elapsed time in seconds since the last handshake.", "gauge")
	c.register("wireguard_received_bytes_total", "Total bytes received from the WireGuard peer.", "counter")
	c.register("wireguard_sent_bytes_total", "Total bytes sent to the WireGuard peer.", "counter")
	c.register("wireguard_peer_persistent_keepalive_interval_seconds", "Persistent keepalive interval in seconds.", "gauge")
	c.register("wireguard_peer_connected", "1 if peer completed handshake within the last 180 seconds, 0 otherwise.", "gauge")

	// 4. BIRD / BGP (bird_exporter compatible)
	c.register("bird_protocol_up", "1 if BIRD protocol is up, 0 otherwise.", "gauge")
	c.register("bird_protocol_state", "Current state of BIRD protocol (value 1 on matching state label).", "gauge")
	c.register("bird_bgp_session_established", "1 if BGP session is in Established state, 0 otherwise.", "gauge")

	// Populate metadata
	c.addSample("easy42_exporter_build_info", [][2]string{
		{"version", version.Version},
		{"commit", version.Commit},
	}, 1)

	// Fetch in-memory cluster configuration
	var cfg *config.Config
	if e.mgr != nil && e.mgr.store != nil {
		cfg = e.mgr.store.Get()
	}

	if cfg == nil {
		return c.formatOutput(startTime)
	}

	// Fetch in-memory live agent snapshots
	var hub = e.mgr.agentHub
	var telemetryMap map[string]*agentpb.TelemetryReport
	var connMap map[string]*agent.AgentConnection
	if hub != nil {
		telemetryMap = hub.GetAllTelemetry()
		connMap = hub.GetAllConnections()
	}

	totalNodes := len(cfg.Nodes)
	c.addSampleInt("easy42_total_nodes", nil, int64(totalNodes))
	c.addSampleInt("easy42_total_links", nil, int64(len(cfg.Links)))

	// Build peer lookup maps from configured links for enriched labeling
	type peerLookupKey struct {
		node          string
		interfaceName string
	}
	type peerPubKeyKey struct {
		node   string
		pubKey string
	}
	linkPeerByIface := make(map[peerLookupKey]string)
	linkPeerByPubKey := make(map[peerPubKeyKey]string)

	for _, l := range cfg.Links {
		if l.From.Name != "" && l.To.Name != "" {
			if l.From.Interface != "" {
				linkPeerByIface[peerLookupKey{node: l.From.Name, interfaceName: l.From.Interface}] = l.To.Name
			}
			if l.To.Interface != "" {
				linkPeerByIface[peerLookupKey{node: l.To.Name, interfaceName: l.To.Interface}] = l.From.Name
			}
			if l.To.PublicKey != "" {
				linkPeerByPubKey[peerPubKeyKey{node: l.From.Name, pubKey: l.To.PublicKey}] = l.To.Name
			}
			if l.From.PublicKey != "" {
				linkPeerByPubKey[peerPubKeyKey{node: l.To.Name, pubKey: l.From.PublicKey}] = l.From.Name
			}
		}
	}

	// Sort nodes alphabetically for deterministic metric output
	sortedNodes := append([]config.Node(nil), cfg.Nodes...)
	sort.Slice(sortedNodes, func(i, j int) bool {
		return sortedNodes[i].Name < sortedNodes[j].Name
	})

	var connectedCount int64
	nowUnix := time.Now().Unix()

	// Track link health: linkKey -> bool
	activeLinks := make(map[string]bool)

	for _, node := range sortedNodes {
		nodeName := node.Name
		isConnected := false
		var conn *agent.AgentConnection
		var lastSeen time.Time

		if hub != nil {
			isConnected = hub.IsConnected(nodeName)
			conn = connMap[nodeName]
			lastSeen = hub.GetLastSeen(nodeName)
		}

		if isConnected {
			connectedCount++
		}

		// Node metadata & connection status
		hostname := ""
		agentVer := ""
		osInfo := ""
		if conn != nil {
			hostname = conn.Hostname()
			agentVer = conn.AgentVersion()
			osInfo = conn.OSInfo()
		}

		asnStr := ""
		if node.ASN > 0 {
			asnStr = strconv.FormatUint(node.ASN, 10)
		}

		isExtStr := "0"
		if node.IsExternal {
			isExtStr = "1"
		}

		c.addSample("easy42_node_info", [][2]string{
			{"node", nodeName},
			{"hostname", hostname},
			{"agent_version", agentVer},
			{"os", osInfo},
			{"asn", asnStr},
			{"ip", node.IP},
			{"ip6", node.IP6},
			{"mode", node.Mode},
			{"is_external", isExtStr},
		}, 1)

		if isConnected {
			c.addSampleInt("easy42_node_up", [][2]string{{"node", nodeName}}, 1)
		} else {
			c.addSampleInt("easy42_node_up", [][2]string{{"node", nodeName}}, 0)
		}

		if !lastSeen.IsZero() {
			c.addSampleInt("easy42_node_last_seen_timestamp_seconds", [][2]string{{"node", nodeName}}, lastSeen.Unix())
		}

		// Process node telemetry
		telemetry := telemetryMap[nodeName]
		if telemetry == nil {
			continue
		}

		// 2. System Metrics
		if sys := telemetry.System; sys != nil {
			if len(sys.LoadAvg) > 0 {
				c.addSample("node_load1", [][2]string{{"node", nodeName}}, float64(sys.LoadAvg[0]))
			}
			if len(sys.LoadAvg) > 1 {
				c.addSample("node_load5", [][2]string{{"node", nodeName}}, float64(sys.LoadAvg[1]))
			}
			if len(sys.LoadAvg) > 2 {
				c.addSample("node_load15", [][2]string{{"node", nodeName}}, float64(sys.LoadAvg[2]))
			}

			c.addSample("node_cpu_utilization_ratio", [][2]string{{"node", nodeName}}, float64(sys.CpuPercent)/100.0)
			c.addSample("node_cpu_percent", [][2]string{{"node", nodeName}}, float64(sys.CpuPercent))

			c.addSampleUint("node_memory_MemTotal_bytes", [][2]string{{"node", nodeName}}, sys.MemoryTotalBytes)
			c.addSampleUint("node_memory_used_bytes", [][2]string{{"node", nodeName}}, sys.MemoryUsedBytes)
			if sys.MemoryTotalBytes >= sys.MemoryUsedBytes {
				memFree := sys.MemoryTotalBytes - sys.MemoryUsedBytes
				c.addSampleUint("node_memory_MemFree_bytes", [][2]string{{"node", nodeName}}, memFree)
				c.addSampleUint("node_memory_MemAvailable_bytes", [][2]string{{"node", nodeName}}, memFree)
			}

			if sys.UptimeSeconds > 0 {
				c.addSampleUint("node_uptime_seconds", [][2]string{{"node", nodeName}}, sys.UptimeSeconds)
				bootTime := nowUnix - int64(sys.UptimeSeconds)
				c.addSampleInt("node_boot_time_seconds", [][2]string{{"node", nodeName}}, bootTime)
			}
		}

		// Disks
		for _, disk := range telemetry.Disks {
			if disk.Path == "" {
				continue
			}
			labels := [][2]string{
				{"node", nodeName},
				{"mountpoint", disk.Path},
			}
			c.addSampleUint("node_filesystem_size_bytes", labels, disk.TotalBytes)
			c.addSampleUint("node_filesystem_free_bytes", labels, disk.FreeBytes)
			c.addSampleUint("node_filesystem_avail_bytes", labels, disk.FreeBytes)
			c.addSampleUint("node_filesystem_used_bytes", labels, disk.UsedBytes)
		}

		// Interfaces
		seenIfaces := make(map[string]bool)
		for _, iface := range telemetry.Interfaces {
			if iface.Name == "" {
				continue
			}
			seenIfaces[iface.Name] = true
			labels := [][2]string{
				{"node", nodeName},
				{"device", iface.Name},
			}
			if iface.IsUp {
				c.addSampleInt("node_network_up", labels, 1)
			} else {
				c.addSampleInt("node_network_up", labels, 0)
			}
			c.addSampleUint("node_network_receive_bytes_total", labels, iface.RxBytes)
			c.addSampleUint("node_network_transmit_bytes_total", labels, iface.TxBytes)
			c.addSampleUint("node_network_receive_packets_total", labels, iface.RxPackets)
			c.addSampleUint("node_network_transmit_packets_total", labels, iface.TxPackets)
			c.addSampleUint("node_network_receive_errs_total", labels, iface.RxErrors)
			c.addSampleUint("node_network_transmit_errs_total", labels, iface.TxErrors)

			// If it's a wireguard interface (e.g. wg*), record wireguard_interface_up
			if strings.HasPrefix(iface.Name, "wg") {
				wgLabels := [][2]string{
					{"node", nodeName},
					{"interface", iface.Name},
				}
				if iface.IsUp {
					c.addSampleInt("wireguard_interface_up", wgLabels, 1)
				} else {
					c.addSampleInt("wireguard_interface_up", wgLabels, 0)
				}
			}
		}

		// WireGuard Peers
		for _, peer := range telemetry.WgPeers {
			if peer.InterfaceName == "" || peer.PublicKey == "" {
				continue
			}

			// Resolve peer node name if part of internal cluster links
			peerNode := linkPeerByPubKey[peerPubKeyKey{node: nodeName, pubKey: peer.PublicKey}]
			if peerNode == "" {
				peerNode = linkPeerByIface[peerLookupKey{node: nodeName, interfaceName: peer.InterfaceName}]
			}

			peerLabels := [][2]string{
				{"node", nodeName},
				{"interface", peer.InterfaceName},
				{"public_key", peer.PublicKey},
				{"endpoint", peer.Endpoint},
				{"peer_node", peerNode},
				{"friendly_name", peerNode},
			}

			if peer.LastHandshakeTime > 0 {
				c.addSampleInt("wireguard_latest_handshake_seconds", peerLabels, peer.LastHandshakeTime)
				delaySeconds := nowUnix - peer.LastHandshakeTime
				if delaySeconds < 0 {
					delaySeconds = 0
				}
				c.addSampleInt("wireguard_latest_handshake_delay_seconds", peerLabels, delaySeconds)
			}

			c.addSampleUint("wireguard_received_bytes_total", peerLabels, peer.RxBytes)
			c.addSampleUint("wireguard_sent_bytes_total", peerLabels, peer.TxBytes)

			connLabels := [][2]string{
				{"node", nodeName},
				{"interface", peer.InterfaceName},
				{"public_key", peer.PublicKey},
				{"peer_node", peerNode},
				{"friendly_name", peerNode},
			}

			if peer.PersistentKeepalive > 0 {
				c.addSampleInt("wireguard_peer_persistent_keepalive_interval_seconds", connLabels, peer.PersistentKeepalive)
			}

			// Connected if handshake happened within last 180 seconds
			isPeerConnected := peer.LastHandshakeTime > 0 && (nowUnix-peer.LastHandshakeTime) <= 180
			if isPeerConnected {
				c.addSampleInt("wireguard_peer_connected", connLabels, 1)
				if peerNode != "" {
					activeLinks[nodeName+"->"+peerNode] = true
				}
			} else {
				c.addSampleInt("wireguard_peer_connected", connLabels, 0)
			}
		}

		// BIRD Protocols
		for _, proto := range telemetry.BirdProtocols {
			if proto.Name == "" {
				continue
			}

			// Resolve peer node name if protocol corresponds to peer
			peerNode := ""
			cleanProto := strings.ToLower(proto.Name)
			cleanProto = strings.TrimPrefix(cleanProto, "dn42_")
			cleanProto = strings.TrimPrefix(cleanProto, "bgp_")
			cleanProtoNorm := strings.ReplaceAll(cleanProto, "_", "-")
			for _, n := range cfg.Nodes {
				nLower := strings.ToLower(n.Name)
				if nLower == cleanProto || nLower == cleanProtoNorm || strings.ReplaceAll(nLower, "-", "_") == cleanProto {
					peerNode = n.Name
					break
				}
			}

			protoLabels := [][2]string{
				{"node", nodeName},
				{"name", proto.Name},
				{"proto", proto.Proto},
				{"peer_node", peerNode},
			}

			isUp := strings.EqualFold(proto.State, "up")
			if isUp {
				c.addSampleInt("bird_protocol_up", protoLabels, 1)
			} else {
				c.addSampleInt("bird_protocol_up", protoLabels, 0)
			}

			// Protocol state gauge
			stateLabels := [][2]string{
				{"node", nodeName},
				{"name", proto.Name},
				{"proto", proto.Proto},
				{"state", strings.ToLower(proto.State)},
			}
			c.addSampleInt("bird_protocol_state", stateLabels, 1)

			// BGP Session Established
			if strings.EqualFold(proto.Proto, "BGP") {
				bgpLabels := [][2]string{
					{"node", nodeName},
					{"name", proto.Name},
					{"peer_node", peerNode},
				}
				if strings.Contains(strings.ToLower(proto.Info), "established") {
					c.addSampleInt("bird_bgp_session_established", bgpLabels, 1)
				} else {
					c.addSampleInt("bird_bgp_session_established", bgpLabels, 0)
				}
			}
		}
	}

	c.addSampleInt("easy42_connected_nodes", nil, connectedCount)

	// Link Health Status
	for _, l := range cfg.Links {
		linkLabels := [][2]string{
			{"from_node", l.From.Name},
			{"to_node", l.To.Name},
			{"from_interface", l.From.Interface},
			{"to_interface", l.To.Interface},
		}

		forwardActive := activeLinks[l.From.Name+"->"+l.To.Name]
		reverseActive := activeLinks[l.To.Name+"->"+l.From.Name]
		if forwardActive || reverseActive {
			c.addSampleInt("easy42_link_up", linkLabels, 1)
		} else {
			c.addSampleInt("easy42_link_up", linkLabels, 0)
		}
	}

	return c.formatOutput(startTime)
}

func (c *metricCollector) formatOutput(startTime time.Time) []byte {
	// Add scrape duration
	duration := time.Since(startTime).Seconds()
	c.addSample("easy42_scrape_duration_seconds", nil, duration)

	var buf bytes.Buffer
	for _, name := range c.order {
		f := c.families[name]
		if f == nil || len(f.samples) == 0 {
			continue
		}

		// Sort samples by labels for determinism
		sort.Slice(f.samples, func(i, j int) bool {
			return f.samples[i].labels < f.samples[j].labels
		})

		buf.WriteString("# HELP ")
		buf.WriteString(f.name)
		buf.WriteString(" ")
		buf.WriteString(f.help)
		buf.WriteString("\n")

		buf.WriteString("# TYPE ")
		buf.WriteString(f.name)
		buf.WriteString(" ")
		buf.WriteString(f.mType)
		buf.WriteString("\n")

		for _, s := range f.samples {
			buf.WriteString(f.name)
			buf.WriteString(s.labels)
			buf.WriteString(" ")
			buf.WriteString(s.value)
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
	}

	return buf.Bytes()
}
