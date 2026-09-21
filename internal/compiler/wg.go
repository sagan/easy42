package compiler

import (
	"fmt"
	"strings"

	"easy42/internal/config"
	"easy42/internal/crypto"
	"easy42/util/tplutil"
)

// GetDefaultWgTemplate returns the content of the embedded easy42_wg.conf.tmpl template
func GetDefaultWgTemplate() (string, error) {
	data, err := templatesFS.ReadFile("templates/easy42_wg.conf.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to read embedded wg template: %w", err)
	}
	return string(data), nil
}

// BuildWgLinkContext prepares the template context map for a WireGuard link configuration.
func BuildWgLinkContext(
	selfNode *config.Node,
	peerNode *config.Node,
	selfEnd *config.LinkEnd,
	peerEnd *config.LinkEnd,
	vault *crypto.KeyVault,
	args ...any,
) (map[string]any, error) {
	var privateKey string
	if selfEnd != nil && selfEnd.PrivateKey != "" {
		if vault != nil {
			decrypted, err := vault.DecryptField(selfEnd.PrivateKey)
			if err != nil {
				nodeName := ""
				if selfNode != nil {
					nodeName = selfNode.Name
				}
				return nil, fmt.Errorf("failed to decrypt private key for node %s: %w", nodeName, err)
			}
			privateKey = decrypted
		} else {
			privateKey = selfEnd.PrivateKey
		}
	}

	address := ""
	if selfEnd != nil {
		address = selfEnd.Address
	}
	if address == "" && selfNode != nil && selfNode.IP != "" {
		derived, err := DeriveIPv6LinkLocal(selfNode.IP)
		if err != nil {
			return nil, fmt.Errorf("failed to derive link-local address for node %s: %w", selfNode.Name, err)
		}
		address = derived
	}

	var listenPort int
	if selfEnd != nil {
		listenPort = selfEnd.ListenPort
	}
	if listenPort == 0 && peerNode != nil && !peerNode.IsExternal && peerNode.IP != "" {
		listenPort = DerivePortFromIP(peerNode.IP)
	}

	peerAddrOnly := ""
	if peerEnd != nil && peerEnd.Address != "" {
		peerAddrOnly = strings.TrimSpace(peerEnd.Address)
		if idx := strings.Index(peerAddrOnly, "/"); idx != -1 {
			peerAddrOnly = peerAddrOnly[:idx]
		}
	} else if peerNode != nil && peerNode.IP != "" {
		derived, err := DeriveIPv6LinkLocalAddressOnly(peerNode.IP)
		if err == nil {
			peerAddrOnly = derived
		}
	}
	if peerAddrOnly == "" {
		peerAddrOnly = "fe80::1"
	}

	mtu := 1420
	if selfEnd != nil && selfEnd.MTU > 0 {
		mtu = selfEnd.MTU
	}

	peerPublicKey := ""
	if peerEnd != nil {
		peerPublicKey = peerEnd.PublicKey
	}

	endpoint := ResolveLinkEndpoint(selfNode, peerNode, selfEnd, peerEnd)

	var keepalive int
	if selfEnd != nil {
		keepalive = selfEnd.PersistentKeepalive
	}
	if keepalive == 0 && endpoint != "" {
		keepalive = 25
	}
	if endpoint == "" {
		keepalive = 0
	}

	allowedIPs := fmt.Sprintf("%s/128, 0.0.0.0/0, ::/0", peerAddrOnly)

	var policyMap map[string]config.NetworkPolicy
	for _, arg := range args {
		switch v := arg.(type) {
		case *config.Config:
			if v != nil {
				allPolicies := v.GetAllPolicies()
				policyMap = make(map[string]config.NetworkPolicy, len(allPolicies))
				for _, p := range allPolicies {
					policyMap[p.ID] = p
				}
			}
		case config.Config:
			allPolicies := v.GetAllPolicies()
			policyMap = make(map[string]config.NetworkPolicy, len(allPolicies))
			for _, p := range allPolicies {
				policyMap[p.ID] = p
			}
		case []config.NetworkPolicy:
			cfgPolicies := &config.Config{NetworkPolicies: v}
			allPolicies := cfgPolicies.GetAllPolicies()
			policyMap = make(map[string]config.NetworkPolicy, len(allPolicies))
			for _, p := range allPolicies {
				policyMap[p.ID] = p
			}
		case *config.NetworkPolicy:
			if v != nil {
				if policyMap == nil {
					policyMap = make(map[string]config.NetworkPolicy)
				}
				policyMap[v.ID] = *v
			}
		case config.NetworkPolicy:
			if policyMap == nil {
				policyMap = make(map[string]config.NetworkPolicy)
			}
			policyMap[v.ID] = v
		case map[string]config.NetworkPolicy:
			policyMap = v
		}
	}
	if policyMap == nil {
		allPolicies := (&config.Config{}).GetAllPolicies()
		policyMap = make(map[string]config.NetworkPolicy, len(allPolicies))
		for _, p := range allPolicies {
			policyMap[p.ID] = p
		}
	}

	isRemoteExternal := peerNode != nil && peerNode.IsExternal
	policyID := selfEnd.EffectivePolicy(isRemoteExternal)
	var activePolicy *config.NetworkPolicy
	if pol, ok := policyMap[policyID]; ok {
		activePolicy = &pol
	}
	fwmark := selfEnd.EffectiveFwmark(activePolicy)

	ctx := map[string]any{
		"self_node":            selfNode,
		"peer_node":            peerNode,
		"self_end":             selfEnd,
		"peer_end":             peerEnd,
		"address":              address,
		"listen_port":          listenPort,
		"private_key":          privateKey,
		"mtu":                  mtu,
		"peer_public_key":      peerPublicKey,
		"peer_addr_only":       peerAddrOnly,
		"allowed_ips":          allowedIPs,
		"endpoint":             endpoint,
		"persistent_keepalive": keepalive,
	}

	if fwmark != "" {
		ctx["fwmark"] = fwmark
		ctx["FwMark"] = fwmark
	}

	ifaceName := ""
	if selfEnd != nil && selfEnd.Interface != "" {
		ifaceName = selfEnd.Interface
	} else if peerNode != nil {
		ifaceName = GetInterfaceName(peerNode.Name, peerNode.IsExternal)
	}
	peerName := ""
	if peerNode != nil {
		peerName = peerNode.Name
	}

	var wgHooksInterface []string
	var wgHooksPeer []string
	var wgHooksPost []string
	if selfNode != nil {
		for _, h := range selfNode.ConfigHooks {
			content := strings.TrimSpace(h.Content)
			if content == "" {
				continue
			}
			if !MatchWgHookTarget(h.Target, ifaceName, peerName) {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(h.Type)) {
			case "wg.interface":
				wgHooksInterface = append(wgHooksInterface, content)
			case "wg.peer":
				wgHooksPeer = append(wgHooksPeer, content)
			case "wg.post", "wg":
				wgHooksPost = append(wgHooksPost, content)
			}
		}
	}
	if len(wgHooksInterface) > 0 {
		ctx["wg_hooks_interface"] = wgHooksInterface
	}
	if len(wgHooksPeer) > 0 {
		ctx["wg_hooks_peer"] = wgHooksPeer
	}
	if len(wgHooksPost) > 0 {
		ctx["wg_hooks_post"] = wgHooksPost
	}

	return ctx, nil
}

// MatchWgHookTarget checks if a hook's target matches the given interface or peer name.
// An empty target or "*" matches all WireGuard interfaces.
func MatchWgHookTarget(target, ifaceName, peerName string) bool {
	t := strings.TrimSpace(strings.ToLower(target))
	if t == "" || t == "*" {
		return true
	}
	iLower := strings.ToLower(strings.TrimSpace(ifaceName))
	pLower := strings.ToLower(strings.TrimSpace(peerName))
	if iLower != "" && (iLower == t || (strings.HasSuffix(t, "*") && strings.HasPrefix(iLower, strings.TrimSuffix(t, "*")))) {
		return true
	}
	if pLower != "" && (pLower == t || (strings.HasSuffix(t, "*") && strings.HasPrefix(pLower, strings.TrimSuffix(t, "*")))) {
		return true
	}
	return false
}

// GenerateWgConfigContentWithTemplate generates WireGuard config using a custom template
func GenerateWgConfigContentWithTemplate(
	tmplContent string,
	selfNode *config.Node,
	peerNode *config.Node,
	selfEnd *config.LinkEnd,
	peerEnd *config.LinkEnd,
	vault *crypto.KeyVault,
	args ...any,
) (string, error) {
	ctx, err := BuildWgLinkContext(selfNode, peerNode, selfEnd, peerEnd, vault, args...)
	if err != nil {
		return "", err
	}

	return tplutil.RenderTemplate(tmplContent, ctx)
}

// GenerateWgConfigContent generates the standard WireGuard configuration content for a node's end of a link using the default template
func GenerateWgConfigContent(
	selfNode *config.Node,
	peerNode *config.Node,
	selfEnd *config.LinkEnd,
	peerEnd *config.LinkEnd,
	vault *crypto.KeyVault,
	args ...any,
) (string, error) {
	tmplContent, err := GetDefaultWgTemplate()
	if err != nil {
		return "", err
	}
	return GenerateWgConfigContentWithTemplate(tmplContent, selfNode, peerNode, selfEnd, peerEnd, vault, args...)
}

// GetInterfaceNameWithSuffix returns the standard wg42<peer_name><suffix> interface name for internal peers,
// or wg42-<peer_name><suffix> for external peers, truncating peerName if necessary to adhere to Linux's 15-char limit.
func GetInterfaceNameWithSuffix(peerName string, suffix string, isExternal ...bool) string {
	cleanName := strings.TrimSpace(peerName)
	ext := len(isExternal) > 0 && isExternal[0]
	prefix := "wg42"
	maxPeerLen := 11
	if ext {
		prefix = "wg42-"
		maxPeerLen = 10
	}
	if suffix != "" {
		maxPeerLen -= len(suffix)
		if maxPeerLen < 0 {
			maxPeerLen = 0
		}
	}
	if len(cleanName) > maxPeerLen {
		cleanName = cleanName[:maxPeerLen]
	}
	return fmt.Sprintf("%s%s%s", prefix, cleanName, suffix)
}

// GetInterfaceName returns the standard wg42<peer_name> interface name for internal peers,
// or wg42-<peer_name> for external peers.
func GetInterfaceName(peerName string, isExternal ...bool) string {
	return GetInterfaceNameWithSuffix(peerName, "", isExternal...)
}

// GetExternalInterfaceName returns the external wg42-<peer_name> interface name (max 10 chars peer name)
func GetExternalInterfaceName(peerName string) string {
	return GetInterfaceName(peerName, true)
}

// ExtractInterfaceSuffix extracts the deterministic suffix (e.g. "1", "2") from an interface name given a peer name.
// Returns "" if it is the primary interface without suffix or does not follow the deterministic naming scheme.
func ExtractInterfaceSuffix(iface, peerName string, isExternal ...bool) string {
	ext := len(isExternal) > 0 && isExternal[0]
	if iface == GetInterfaceName(peerName, ext) {
		return ""
	}
	for i := 1; i <= 99; i++ {
		s := fmt.Sprintf("%d", i)
		if iface == GetInterfaceNameWithSuffix(peerName, s, ext) {
			return s
		}
	}
	return ""
}
