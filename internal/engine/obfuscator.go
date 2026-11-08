package engine

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"easy42/internal/agent"
	agentpb "easy42/internal/agent/proto"
	"easy42/internal/compiler"
	"easy42/internal/config"
)

const (
	obfuscatorPortStart     = 30000
	obfuscatorTunAddrPrefix = "198.51.100" // The A.B.C ipv4 prefix. Use 198.51.100.0/24 (RFC 6890 TEST-NET-2)
	obfuscatorTunBlockStart = 1            // first usable /30 block in <obfuscatorTunAddrPrefix>/24
	obfuscatorTunBlockEnd   = 63           // last usable /30 block
)

func isSameLink(a, b *config.Link) bool {
	if a == nil || b == nil {
		return false
	}
	return a.From.Name == b.From.Name && a.To.Name == b.To.Name &&
		a.From.Interface == b.From.Interface && a.To.Interface == b.To.Interface
}

func usedObfuscatorPorts(cfg *config.Config, nodeName string, skip *config.Link) map[int]bool {
	used := make(map[int]bool)
	if cfg == nil {
		return used
	}
	add := func(port int) {
		if port > 0 {
			used[port] = true
		}
	}
	for i := range cfg.Links {
		l := &cfg.Links[i]
		if skip != nil && isSameLink(l, skip) {
			continue
		}
		for _, end := range []*config.LinkEnd{&l.From, &l.To} {
			if end.Name != nodeName {
				continue
			}
			add(end.ListenPort)
			if p := end.ObfuscatorParameters; p != nil {
				add(p.LocalPort)
				add(p.ServerPort)
				add(p.WireGuardPort)
			}
		}
	}
	return used
}

func allocateObfuscatorPort(used map[int]bool, preferred int) (int, error) {
	if preferred > 0 {
		if used[preferred] {
			return 0, fmt.Errorf("obfuscator port %d is already in use on this node", preferred)
		}
		return preferred, nil
	}
	for port := obfuscatorPortStart; port <= 65535; port++ {
		if !used[port] {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free obfuscator port available")
}

func obfuscatorTunBlock(ip string) int {
	parsed := net.ParseIP(strings.TrimSuffix(strings.TrimSpace(ip), "/32"))
	if parsed == nil || parsed.To4() == nil {
		return -1
	}
	v4 := parsed.To4()
	if v4[0] != 192 || v4[1] != 168 || v4[2] != 200 {
		return -1
	}
	return int(v4[3]) / 4
}

func usedObfuscatorTunBlocks(cfg *config.Config, nodeNames []string, skip *config.Link) map[int]bool {
	used := make(map[int]bool)
	if cfg == nil {
		return used
	}
	want := make(map[string]bool, len(nodeNames))
	for _, name := range nodeNames {
		want[name] = true
	}
	for i := range cfg.Links {
		l := &cfg.Links[i]
		if skip != nil && isSameLink(l, skip) {
			continue
		}
		for _, end := range []*config.LinkEnd{&l.From, &l.To} {
			if !want[end.Name] || end.ObfuscatorParameters == nil {
				continue
			}
			p := end.ObfuscatorParameters
			if b := obfuscatorTunBlock(p.TunLocal); b >= 0 {
				used[b] = true
			}
			if b := obfuscatorTunBlock(p.TunPeer); b >= 0 {
				used[b] = true
			}
		}
	}
	return used
}

func allocateObfuscatorTunBlock(cfg *config.Config, nodeNames []string, skip *config.Link) (string, string, error) {
	used := usedObfuscatorTunBlocks(cfg, nodeNames, skip)
	for block := obfuscatorTunBlockStart; block <= obfuscatorTunBlockEnd; block++ {
		if used[block] {
			continue
		}
		base := block * 4
		return fmt.Sprintf("%s.%d", obfuscatorTunAddrPrefix, base+1), fmt.Sprintf("%s.%d", obfuscatorTunAddrPrefix, base+2), nil
	}
	return "", "", fmt.Errorf("no free obfuscator TUN address block available")
}

func usedObfuscatorTunNames(cfg *config.Config, nodeName string, skip *config.Link) map[string]bool {
	used := make(map[string]bool)
	if cfg == nil {
		return used
	}
	for i := range cfg.Links {
		l := &cfg.Links[i]
		if skip != nil && isSameLink(l, skip) {
			continue
		}
		for _, end := range []*config.LinkEnd{&l.From, &l.To} {
			if end.Name == nodeName && end.ObfuscatorParameters != nil {
				if name := strings.TrimSpace(end.ObfuscatorParameters.TunName); name != "" {
					used[name] = true
				}
			}
		}
	}
	return used
}

func generateObfuscatorTunName(cfg *config.Config, nodeName, peerName string, skip *config.Link) string {
	used := usedObfuscatorTunNames(cfg, nodeName, skip)
	for suffix := 0; suffix < 100; suffix++ {
		name := config.FormatObfuscatorTunName(peerName, suffix)
		if !used[name] {
			return name
		}
	}
	return config.FormatObfuscatorTunName(peerName, 99)
}

func fillObfuscatorDefaults(cfg *config.Config, link *config.Link, clientEnd, serverEnd *config.LinkEnd, clientNode, serverNode *config.Node) error {
	if clientEnd == nil || serverEnd == nil || clientNode == nil || serverNode == nil {
		return fmt.Errorf("obfuscator link endpoints are incomplete")
	}

	clientP := clientEnd.ObfuscatorParameters.Clone()
	if clientP == nil {
		clientP = &config.ObfuscatorParameters{}
	}
	serverP := serverEnd.ObfuscatorParameters.Clone()
	if serverP == nil {
		serverP = &config.ObfuscatorParameters{}
	}

	// Server port is allocated on the server node and mirrored to the client.
	// A client-side value is also accepted as the preferred port when the server
	// side has not supplied one (the common one-sided patch shape).
	serverPortPreferred := serverP.ServerPort
	if serverPortPreferred <= 0 {
		serverPortPreferred = clientP.ServerPort
	}
	serverUsed := usedObfuscatorPorts(cfg, serverNode.Name, link)
	serverPort, err := allocateObfuscatorPort(serverUsed, serverPortPreferred)
	if err != nil {
		return fmt.Errorf("server endpoint: %w", err)
	}
	serverP.ServerPort = serverPort
	clientP.ServerPort = serverPort

	clientUsed := usedObfuscatorPorts(cfg, clientNode.Name, link)
	localPort, err := allocateObfuscatorPort(clientUsed, clientP.LocalPort)
	if err != nil {
		return fmt.Errorf("client endpoint: %w", err)
	}
	clientP.LocalPort = localPort

	// The server forwards to the local WireGuard listener.
	serverP.WireGuardPort = serverEnd.ListenPort
	if serverP.WireGuardPort <= 0 {
		if serverNode.IP != "" {
			serverP.WireGuardPort = compiler.DerivePortFromIP(serverNode.IP)
		} else {
			serverP.WireGuardPort = 51820
		}
	}

	if clientP.TunName == "" {
		clientP.TunName = generateObfuscatorTunName(cfg, clientNode.Name, serverNode.Name, link)
	}
	if serverP.TunName == "" {
		serverP.TunName = generateObfuscatorTunName(cfg, serverNode.Name, clientNode.Name, link)
	}

	// Keep the TUN address pairs complementary. If either side already supplies a
	// complete pair, derive the other side from it; otherwise allocate a fresh /30.
	clientHasPair := clientP.TunLocal != "" && clientP.TunPeer != ""
	serverHasPair := serverP.TunLocal != "" && serverP.TunPeer != ""
	switch {
	case clientHasPair:
		serverP.TunLocal = clientP.TunPeer
		serverP.TunPeer = clientP.TunLocal
	case serverHasPair:
		clientP.TunLocal = serverP.TunPeer
		clientP.TunPeer = serverP.TunLocal
	default:
		clientLocal, clientPeer, err := allocateObfuscatorTunBlock(cfg, []string{clientNode.Name, serverNode.Name}, link)
		if err != nil {
			return err
		}
		clientP.TunLocal = clientLocal
		clientP.TunPeer = clientPeer
		serverP.TunLocal = clientPeer
		serverP.TunPeer = clientLocal
	}

	if clientP.ExtraArgs == nil {
		clientP.ExtraArgs = []string{}
	}
	if serverP.ExtraArgs == nil {
		serverP.ExtraArgs = []string{}
	}

	clientEnd.Obfuscator = config.ObfuscatorPhantunClient
	serverEnd.Obfuscator = config.ObfuscatorPhantunServer
	clientEnd.ObfuscatorParameters = clientP
	serverEnd.ObfuscatorParameters = serverP
	return nil
}

// prepareLinkObfuscator normalizes complementary roles and generates missing
// parameters for a link. It is intentionally idempotent: existing user-provided
// values are preserved. Empty roles are treated as "none".
func (m *Manager) prepareLinkObfuscator(cfg *config.Config, link *config.Link, fromNode, toNode *config.Node, isNew bool) error {
	if link == nil {
		return nil
	}
	fromRole := link.From.ObfuscatorRole()
	toRole := link.To.ObfuscatorRole()

	if fromRole == config.ObfuscatorNone && toRole == config.ObfuscatorNone {
		link.From.Obfuscator = ""
		link.From.ObfuscatorParameters = nil
		link.To.Obfuscator = ""
		link.To.ObfuscatorParameters = nil
		return nil
	}
	if err := config.ValidateObfuscatorRole(fromRole); err != nil {
		return err
	}
	if err := config.ValidateObfuscatorRole(toRole); err != nil {
		return err
	}

	// Auto-complete a missing complementary role. Explicit "none" on either side
	// is handled above by disabling the feature for the whole link.
	if fromRole == config.ObfuscatorNone {
		fromRole = config.ComplementObfuscatorRole(toRole)
	}
	if toRole == config.ObfuscatorNone {
		toRole = config.ComplementObfuscatorRole(fromRole)
	}
	if config.ComplementObfuscatorRole(fromRole) != toRole {
		return fmt.Errorf("obfuscator roles must be complementary (%s <-> %s)", config.ObfuscatorPhantunClient, config.ObfuscatorPhantunServer)
	}

	clientEnd, serverEnd := &link.From, &link.To
	clientNode, serverNode := fromNode, toNode
	if fromRole == config.ObfuscatorPhantunServer {
		clientEnd, serverEnd = &link.To, &link.From
		clientNode, serverNode = toNode, fromNode
	}
	clientEnd.Obfuscator = config.ObfuscatorPhantunClient
	serverEnd.Obfuscator = config.ObfuscatorPhantunServer

	if err := fillObfuscatorDefaults(cfg, link, clientEnd, serverEnd, clientNode, serverNode); err != nil {
		return err
	}
	return config.ValidateLinkObfuscator(link, fromNode, toNode)
}

func replaceEndpointPort(endpoint string, port int) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	if port <= 0 {
		return endpoint
	}
	host := endpoint
	if h, _, err := net.SplitHostPort(endpoint); err == nil {
		host = h
	} else if strings.HasPrefix(endpoint, "[") && strings.HasSuffix(endpoint, "]") {
		host = strings.TrimPrefix(strings.TrimSuffix(endpoint, "]"), "[")
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// BuildObfuscatorSpecsForNode builds the full desired set of Phantun services for a node.
func BuildObfuscatorSpecsForNode(cfg *config.Config, node *config.Node) ([]*agentpb.ObfuscatorSpec, error) {
	if cfg == nil || node == nil {
		return nil, nil
	}
	nodeMap := make(map[string]*config.Node, len(cfg.Nodes))
	for i := range cfg.Nodes {
		nodeMap[cfg.Nodes[i].Name] = &cfg.Nodes[i]
	}
	policyMap := make(map[string]config.NetworkPolicy)
	for _, p := range cfg.GetAllPolicies() {
		policyMap[p.ID] = p
	}

	specs := make([]*agentpb.ObfuscatorSpec, 0)
	for i := range cfg.Links {
		link := &cfg.Links[i]
		if link.IsManual() {
			continue
		}
		var selfEnd, peerEnd *config.LinkEnd
		var peerNode *config.Node
		if link.From.Name == node.Name {
			selfEnd, peerEnd = &link.From, &link.To
			peerNode = nodeMap[link.To.Name]
		} else if link.To.Name == node.Name {
			selfEnd, peerEnd = &link.To, &link.From
			peerNode = nodeMap[link.From.Name]
		} else {
			continue
		}
		role := selfEnd.ObfuscatorRole()
		if role == config.ObfuscatorNone {
			continue
		}
		if !node.IsAgentMode() {
			return nil, fmt.Errorf("link %s <-> %s uses a WireGuard obfuscator but node %s is not in agent mode", link.From.Name, link.To.Name, node.Name)
		}
		if selfEnd.ObfuscatorParameters == nil {
			return nil, fmt.Errorf("link %s <-> %s is missing obfuscator parameters for node %s", link.From.Name, link.To.Name, node.Name)
		}
		p := selfEnd.ObfuscatorParameters
		localPort := p.LocalPort
		if role == config.ObfuscatorPhantunServer {
			localPort = p.ServerPort
		}
		spec := &agentpb.ObfuscatorSpec{
			InterfaceName: selfEnd.Interface,
			ServiceName:   config.ObfuscatorServiceName(selfEnd.Interface),
			Role:          role,
			TunName:       p.TunName,
			TunLocal:      p.TunLocal,
			TunPeer:       p.TunPeer,
			LocalPort:     int32(localPort),
			ServerPort:    int32(p.ServerPort),
			WireguardPort: int32(p.WireGuardPort),
			ExtraArgs:     append([]string(nil), p.ExtraArgs...),
			Enabled:       true,
		}
		switch role {
		case config.ObfuscatorPhantunClient:
			serverPort := p.ServerPort
			if peerEnd != nil && peerEnd.ObfuscatorParameters != nil && peerEnd.ObfuscatorParameters.ServerPort > 0 {
				serverPort = peerEnd.ObfuscatorParameters.ServerPort
			}
			underlying := compiler.ResolveUnderlyingLinkEndpoint(node, peerNode, selfEnd, peerEnd)
			if underlying == "" {
				return nil, fmt.Errorf("cannot resolve underlying endpoint for obfuscated link %s <-> %s", link.From.Name, link.To.Name)
			}
			spec.Remote = replaceEndpointPort(underlying, serverPort)
		case config.ObfuscatorPhantunServer:
			wgPort := p.WireGuardPort
			if wgPort <= 0 {
				wgPort = selfEnd.ListenPort
			}
			spec.Remote = net.JoinHostPort("127.0.0.1", strconv.Itoa(wgPort))
		}

		var policy *config.NetworkPolicy
		if pol, ok := policyMap[selfEnd.EffectivePolicy(peerNode != nil && peerNode.IsExternal)]; ok {
			policy = &pol
		}
		spec.Fwmark = selfEnd.EffectiveFwmark(policy)
		specs = append(specs, spec)
	}
	sort.Slice(specs, func(i, j int) bool {
		if specs[i].InterfaceName != specs[j].InterfaceName {
			return specs[i].InterfaceName < specs[j].InterfaceName
		}
		return specs[i].ServiceName < specs[j].ServiceName
	})
	return specs, nil
}

// ObfuscatorSpecsHash returns a stable hash of a desired obfuscator set.
func ObfuscatorSpecsHash(specs []*agentpb.ObfuscatorSpec) string {
	if len(specs) == 0 {
		return config.HashConfig("[]")
	}
	data, err := json.Marshal(specs)
	if err != nil {
		return ""
	}
	return config.HashConfig(string(data))
}

// validateNodeObfuscatorConfiguration verifies that every obfuscator-enabled link
// touching the node references agent-mode managed endpoints on both sides.
func validateNodeObfuscatorConfiguration(cfg *config.Config, node *config.Node) error {
	if cfg == nil || node == nil {
		return nil
	}
	nodeMap := make(map[string]*config.Node, len(cfg.Nodes))
	for i := range cfg.Nodes {
		nodeMap[cfg.Nodes[i].Name] = &cfg.Nodes[i]
	}
	for i := range cfg.Links {
		link := &cfg.Links[i]
		if link.From.Name != node.Name && link.To.Name != node.Name {
			continue
		}
		if !link.From.ObfuscatorEnabled() && !link.To.ObfuscatorEnabled() {
			continue
		}
		fromNode, toNode := nodeMap[link.From.Name], nodeMap[link.To.Name]
		if fromNode == nil || toNode == nil {
			return fmt.Errorf("link %s <-> %s uses a WireGuard obfuscator but one of its endpoints does not exist", link.From.Name, link.To.Name)
		}
		if fromNode.IsExternal || toNode.IsExternal {
			return fmt.Errorf("link %s <-> %s uses a WireGuard obfuscator but one of its endpoints is external", link.From.Name, link.To.Name)
		}
		if !fromNode.IsAgentMode() || !toNode.IsAgentMode() {
			return fmt.Errorf("link %s <-> %s uses a WireGuard obfuscator but both nodes must be in agent mode", link.From.Name, link.To.Name)
		}
	}
	return nil
}

// normalizeObfuscatorsInConfig fills in missing obfuscator parameters for every
// enabled link in a config. It is used after applying config-change snippets.
func (m *Manager) normalizeObfuscatorsInConfig(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	nodeMap := make(map[string]*config.Node, len(cfg.Nodes))
	for i := range cfg.Nodes {
		nodeMap[cfg.Nodes[i].Name] = &cfg.Nodes[i]
	}
	for i := range cfg.Links {
		link := &cfg.Links[i]
		if link.IsManual() {
			link.From.Obfuscator = ""
			link.From.ObfuscatorParameters = nil
			link.To.Obfuscator = ""
			link.To.ObfuscatorParameters = nil
			continue
		}
		if !link.From.ObfuscatorEnabled() && !link.To.ObfuscatorEnabled() {
			continue
		}
		if err := m.prepareLinkObfuscator(cfg, link, nodeMap[link.From.Name], nodeMap[link.To.Name], false); err != nil {
			return fmt.Errorf("link %s <-> %s: %w", link.From.Name, link.To.Name, err)
		}
	}
	return nil
}

// BuildObfuscatorSpecsForNodeName builds the desired obfuscator set for a node by name.
func (m *Manager) BuildObfuscatorSpecsForNodeName(nodeName string) ([]*agentpb.ObfuscatorSpec, string, error) {
	m.mu.RLock()
	cfg := m.store.Get()
	var node *config.Node
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == nodeName {
			node = &cfg.Nodes[i]
			break
		}
	}
	m.mu.RUnlock()
	if node == nil {
		return nil, "", fmt.Errorf("node %s not found", nodeName)
	}
	specs, err := BuildObfuscatorSpecsForNode(cfg, node)
	if err != nil {
		return nil, "", err
	}
	return specs, ObfuscatorSpecsHash(specs), nil
}

// SyncObfuscatorsToNode pushes the full desired obfuscator service set to a node agent.
func (m *Manager) SyncObfuscatorsToNode(nodeName string) (string, error) {
	if m.agentHub == nil || !m.agentHub.IsConnected(nodeName) {
		return "", fmt.Errorf("node %s agent is not connected; WireGuard obfuscator requires a running agent", nodeName)
	}
	specs, hash, err := m.BuildObfuscatorSpecsForNodeName(nodeName)
	if err != nil {
		return "", err
	}
	if err := m.agentHub.SyncObfuscators(nodeName, specs); err != nil {
		return "", err
	}
	return hash, nil
}

// SyncObfuscatorsToAgents pushes the full desired obfuscator service set to every connected agent node.
func (m *Manager) SyncObfuscatorsToAgents() {
	if m.agentHub == nil {
		return
	}
	m.mu.RLock()
	cfg := m.store.Get()
	m.mu.RUnlock()
	if cfg == nil {
		return
	}
	currentState := m.stateStore.Get()
	for i := range cfg.Nodes {
		node := &cfg.Nodes[i]
		if node.IsExternal || !node.IsAgentMode() {
			continue
		}
		if err := validateNodeObfuscatorConfiguration(cfg, node); err != nil {
			continue
		}
		specs, err := BuildObfuscatorSpecsForNode(cfg, node)
		if err != nil || (len(specs) == 0 && currentState.Nodes[node.Name].ObfuscatorConfigHash == "") {
			continue
		}
		if m.agentHub.IsConnected(node.Name) {
			_ = m.agentHub.SyncObfuscators(node.Name, specs)
		}
	}
}

// setupAgentObfuscatorListeners re-pushes the desired obfuscator service set when an agent registers.
func (m *Manager) setupAgentObfuscatorListeners() {
	if m.agentHub == nil {
		return
	}
	m.agentHub.OnRegister(func(conn *agent.AgentConnection) {
		nodeName := conn.NodeName()
		specs, hash, err := m.BuildObfuscatorSpecsForNodeName(nodeName)
		if err != nil {
			return
		}
		state := m.stateStore.Get()
		prevHash := ""
		if st, ok := state.Nodes[nodeName]; ok {
			prevHash = st.ObfuscatorConfigHash
		}
		if len(specs) == 0 && prevHash == "" {
			return
		}
		if hash == prevHash {
			return
		}
		if pushedHash, err := m.SyncObfuscatorsToNode(nodeName); err == nil && m.stateStore != nil {
			host := ""
			if node := m.FindNode(nodeName); node != nil {
				host = node.Host
			}
			_ = m.stateStore.UpdateObfuscatorState(nodeName, host, pushedHash, time.Now())
		}
	})
}

// planObfuscatorActions computes one full-set reconciliation action per eligible node.
func (m *Manager) planObfuscatorActions(cfg *config.Config, nodes []config.Node, targetMap map[string]bool, isPartial bool, currentState *config.NetworkState) ([]config.SyncAction, error) {
	// Validate every targeted node up-front so a non-agent endpoint always
	// produces a clear agent-mode error rather than a downstream resolution error.
	for i := range nodes {
		node := &nodes[i]
		if node.IsExternal || (isPartial && !targetMap[node.Name]) {
			continue
		}
		if err := validateNodeObfuscatorConfiguration(cfg, node); err != nil {
			return nil, err
		}
	}

	actions := make([]config.SyncAction, 0)
	for i := range nodes {
		node := &nodes[i]
		if node.IsExternal {
			continue
		}
		if isPartial && !targetMap[node.Name] {
			continue
		}
		if err := validateNodeObfuscatorConfiguration(cfg, node); err != nil {
			return nil, err
		}
		specs, err := BuildObfuscatorSpecsForNode(cfg, node)
		if err != nil {
			return nil, err
		}
		prevHash := ""
		if currentState != nil {
			if st, ok := currentState.Nodes[node.Name]; ok {
				prevHash = st.ObfuscatorConfigHash
			}
		}
		if len(specs) == 0 && prevHash == "" {
			continue
		}
		desiredHash := ObfuscatorSpecsHash(specs)
		needsApply := desiredHash != prevHash
		status := "pending"
		diffStatus := "update"
		if !needsApply {
			status = "synced"
			diffStatus = "synced"
		}
		description := fmt.Sprintf("Configure WireGuard obfuscator services on %s (%d service(s))", node.Name, len(specs))
		if len(specs) == 0 {
			description = fmt.Sprintf("Remove WireGuard obfuscator services on %s", node.Name)
		}
		actions = append(actions, config.SyncAction{
			NodeName:    node.Name,
			Host:        node.Host,
			Type:        config.ActionSyncObfuscators,
			Interface:   "obfuscator",
			Description: description,
			NeedsApply:  needsApply,
			Status:      status,
			DiffStatus:  diffStatus,
		})
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i].NodeName < actions[j].NodeName })
	return actions, nil
}
