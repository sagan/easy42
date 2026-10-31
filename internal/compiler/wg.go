package compiler

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
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

	var policyMap map[string]config.NetworkPolicy
	var assignIPv4 bool
	if selfEnd != nil && selfEnd.Address4 != "" {
		assignIPv4 = true
	}

	var rawPsk string
	if selfEnd != nil && selfEnd.Psk != "" {
		rawPsk = selfEnd.Psk
	} else if peerEnd != nil && peerEnd.Psk != "" {
		rawPsk = peerEnd.Psk
	}

	// The link this config is generated for. An explicitly passed link always takes
	// precedence over a lookup in the full config, regardless of argument order.
	var explicitLink *config.Link
	var fullCfg *config.Config

	for _, arg := range args {
		switch v := arg.(type) {
		case *config.Link:
			if v != nil {
				explicitLink = v
			}
		case config.Link:
			l := v
			explicitLink = &l
		case bool:
			assignIPv4 = v
		case *config.Config:
			if v != nil {
				fullCfg = v
				allPolicies := v.GetAllPolicies()
				policyMap = make(map[string]config.NetworkPolicy, len(allPolicies))
				for _, p := range allPolicies {
					policyMap[p.ID] = p
				}
			}
		case config.Config:
			c := v
			fullCfg = &c
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

	linkForEnd := explicitLink
	if linkForEnd == nil && fullCfg != nil {
		linkForEnd = findLinkForEnd(fullCfg.Links, selfNode, peerNode, selfEnd, peerEnd)
	}
	if linkForEnd != nil {
		if linkForEnd.AssignIPv4 {
			assignIPv4 = true
		}
		if rawPsk == "" {
			rawPsk = linkPsk(linkForEnd)
		}
	}
	if policyMap == nil {
		allPolicies := (&config.Config{}).GetAllPolicies()
		policyMap = make(map[string]config.NetworkPolicy, len(allPolicies))
		for _, p := range allPolicies {
			policyMap[p.ID] = p
		}
	}

	address4 := ""
	peerAddr4Only := ""
	if selfEnd != nil && selfEnd.Address4 != "" {
		address4 = selfEnd.Address4
	}
	if peerEnd != nil && peerEnd.Address4 != "" {
		peerAddr4Only = strings.TrimSpace(peerEnd.Address4)
		if idx := strings.Index(peerAddr4Only, "/"); idx != -1 {
			peerAddr4Only = peerAddr4Only[:idx]
		}
	}

	if assignIPv4 && (address4 == "" || peerAddr4Only == "") && selfNode != nil && peerNode != nil {
		selfMainIP := getNodeMainIP(selfNode)
		peerMainIP := getNodeMainIP(peerNode)
		if selfMainIP != "" && peerMainIP != "" {
			linkIdx := 0
			if selfEnd != nil && selfEnd.Interface != "" {
				s := ExtractInterfaceSuffix(selfEnd.Interface, peerNode.Name, peerNode.IsExternal)
				linkIdx = LinkIndexFromSuffix(s)
			} else if peerEnd != nil && peerEnd.Interface != "" {
				s := ExtractInterfaceSuffix(peerEnd.Interface, selfNode.Name, selfNode.IsExternal)
				linkIdx = LinkIndexFromSuffix(s)
			}
			if a1, a2, err := DeriveIPv4LinkLocal(selfMainIP, peerMainIP, linkIdx); err == nil {
				if address4 == "" {
					address4 = a1
				}
				if peerAddr4Only == "" {
					p := strings.TrimSpace(a2)
					if idx := strings.Index(p, "/"); idx != -1 {
						p = p[:idx]
					}
					peerAddr4Only = p
				}
			}
		}
	}

	var allowedIPs string
	peerV4 := ""
	if peerAddr4Only != "" {
		peerV4 = peerAddr4Only + "/32"
	} else if peerEnd != nil && peerEnd.Address4 != "" {
		peerV4 = peerEnd.Address4
		if !strings.Contains(peerV4, "/") {
			peerV4 = peerV4 + "/32"
		}
	}
	if peerV4 == "" {
		allowedIPs = fmt.Sprintf("%s/128, 0.0.0.0/0, ::/0", peerAddrOnly)
	} else {
		allowedIPs = fmt.Sprintf("%s/128, %s, 0.0.0.0/0, ::/0", peerAddrOnly, peerV4)
	}

	isRemoteExternal := peerNode != nil && peerNode.IsExternal
	policyID := selfEnd.EffectivePolicy(isRemoteExternal)
	var activePolicy *config.NetworkPolicy
	if pol, ok := policyMap[policyID]; ok {
		activePolicy = &pol
	}
	fwmark := selfEnd.EffectiveFwmark(activePolicy)

	var psk string
	if rawPsk != "" {
		if vault != nil {
			decrypted, err := vault.DecryptField(rawPsk)
			if err != nil {
				nodeName := ""
				if selfNode != nil {
					nodeName = selfNode.Name
				}
				return nil, fmt.Errorf("failed to decrypt PSK for node %s: %w", nodeName, err)
			}
			psk = decrypted
		} else {
			psk = rawPsk
		}
	}

	selfNote := ""
	if selfNode != nil {
		selfNote = selfNode.Note
	}
	nodeData := config.ExtractNoteData(selfNote)

	peerNote := ""
	if peerNode != nil {
		peerNote = peerNode.Note
	}
	peerNodeData := config.ExtractNoteData(peerNote)

	selfEndNote := ""
	if selfEnd != nil {
		selfEndNote = selfEnd.Note
	}
	selfEndData := config.ExtractNoteData(selfEndNote)

	peerEndNote := ""
	if peerEnd != nil {
		peerEndNote = peerEnd.Note
	}
	peerEndData := config.ExtractNoteData(peerEndNote)

	linkNote := ""
	if linkForEnd != nil {
		linkNote = linkForEnd.Note
	}
	linkData := config.ExtractNoteData(linkNote)

	ctx := map[string]any{
		"self_node":            toWgTemplateMap(selfNode, selfNote),
		"peer_node":            toWgTemplateMap(peerNode, peerNote),
		"self_end":             toWgTemplateMap(selfEnd, selfEndNote),
		"peer_end":             toWgTemplateMap(peerEnd, peerEndNote),
		"data":                 nodeData,
		"Data":                 nodeData,
		"note":                 selfNote,
		"Note":                 selfNote,
		"node_data":            nodeData,
		"peer_node_data":       peerNodeData,
		"link_data":            linkData,
		"self_end_data":        selfEndData,
		"peer_end_data":        peerEndData,
		"address":              address,
		"listen_port":          listenPort,
		"private_key":          privateKey,
		"mtu":                  mtu,
		"peer_public_key":      peerPublicKey,
		"peer_addr_only":       peerAddrOnly,
		"allowed_ips":          allowedIPs,
		"endpoint":             endpoint,
		"persistent_keepalive": keepalive,
		"assign_ipv4":          assignIPv4,
	}

	if linkForEnd != nil {
		ctx["link"] = toWgTemplateMap(linkForEnd, linkNote)
	}

	if psk != "" {
		ctx["psk"] = psk
		ctx["Psk"] = psk
	}

	if address4 != "" {
		ctx["address4"] = address4
	}
	if peerAddr4Only != "" {
		ctx["peer_addr4_only"] = peerAddr4Only
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

// toWgTemplateMap converts a struct pointer (Node, Link, LinkEnd) to a map containing
// json-tagged names, exported PascalCase struct field names, 0-arg methods, and note / data fields.
func toWgTemplateMap(v any, note string) map[string]any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer && rv.IsNil() {
		return nil
	}
	res := make(map[string]any)

	// 1. JSON tags (e.g. name, is_external, etc.)
	if data, err := json.Marshal(v); err == nil {
		_ = json.Unmarshal(data, &res)
	}

	// 2. Struct field names (e.g. Name, IsExternal, Address4, etc.)
	elem := rv
	if elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}
	if elem.Kind() == reflect.Struct {
		typ := elem.Type()
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.PkgPath == "" { // exported field
				fv := elem.Field(i)
				if fv.CanInterface() {
					res[f.Name] = fv.Interface()
				}
			}
		}
	}

	// 3. Methods without arguments (e.g. LinkType(), IsManual(), ExternalIp(), Data())
	for i := 0; i < rv.NumMethod(); i++ {
		m := rv.Type().Method(i)
		if m.PkgPath == "" && m.Type.NumIn() == 1 && m.Type.NumOut() == 1 {
			out := m.Func.Call([]reflect.Value{rv})[0]
			res[m.Name] = out.Interface()
		}
	}

	noteData := config.ExtractNoteData(note)
	res["note"] = note
	res["Note"] = note
	res["data"] = noteData
	res["Data"] = noteData
	return res
}

// findLinkForEnd locates the exact link a WireGuard config is generated for.
// A link only matches when it connects the same two nodes AND the self side uses
// the same local interface. Interface names alone are NOT unique across nodes
// (e.g. both A and C have "wg42B" toward B), so they must never be matched in isolation.
func findLinkForEnd(links []config.Link, selfNode, peerNode *config.Node, selfEnd, peerEnd *config.LinkEnd) *config.Link {
	selfName, peerName := "", ""
	if selfNode != nil {
		selfName = selfNode.Name
	} else if selfEnd != nil {
		selfName = selfEnd.Name
	}
	if peerNode != nil {
		peerName = peerNode.Name
	} else if peerEnd != nil {
		peerName = peerEnd.Name
	}
	if selfName == "" || peerName == "" {
		return nil
	}
	selfIface := ""
	if selfEnd != nil {
		selfIface = selfEnd.Interface
	}

	for i := range links {
		l := &links[i]
		var localEnd *config.LinkEnd
		switch {
		case l.From.Name == selfName && l.To.Name == peerName:
			localEnd = &l.From
		case l.To.Name == selfName && l.From.Name == peerName:
			localEnd = &l.To
		default:
			continue
		}
		if selfIface == "" || localEnd.Interface == selfIface {
			return l
		}
	}
	return nil
}

// linkPsk returns the (encrypted) PSK stored on a link, preferring the link-level value.
func linkPsk(l *config.Link) string {
	if l == nil {
		return ""
	}
	if l.Psk != "" {
		return l.Psk
	}
	if l.From.Psk != "" {
		return l.From.Psk
	}
	return l.To.Psk
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

// GenerateWgConfigContent generates the standard WireGuard configuration content for a node's end of a link using the custom template or default template
func GenerateWgConfigContent(
	selfNode *config.Node,
	peerNode *config.Node,
	selfEnd *config.LinkEnd,
	peerEnd *config.LinkEnd,
	vault *crypto.KeyVault,
	args ...any,
) (string, error) {
	var customTemplates []config.ConfigTemplate
	for _, arg := range args {
		switch v := arg.(type) {
		case []config.ConfigTemplate:
			customTemplates = v
		case *config.Config:
			if v != nil {
				customTemplates = v.Templates
			}
		case config.Config:
			customTemplates = v.Templates
		}
	}

	var tmplContent string
	var err error
	if selfNode != nil && strings.TrimSpace(selfNode.WgTemplate) != "" {
		tmplContent, err = FindTemplate(config.TemplateTypeWg, selfNode.WgTemplate, customTemplates)
	} else {
		tmplContent, err = GetDefaultWgTemplate()
	}
	if err != nil {
		return "", err
	}
	return GenerateWgConfigContentWithTemplate(tmplContent, selfNode, peerNode, selfEnd, peerEnd, vault, args...)
}


// GetInterfaceNameWithSuffix returns the standard wg42<peer_name><suffix> interface name for internal peers,
// or wg42-<peer_name><suffix> for external peers, truncating and sanitizing to adhere to Linux's 15-char limit.
func GetInterfaceNameWithSuffix(peerName string, suffix string, isExternal ...bool) string {
	cleanName := strings.TrimSpace(peerName)
	ext := len(isExternal) > 0 && isExternal[0]
	prefix := "wg42"
	if ext {
		prefix = "wg42-"
	}

	maxPeerLen := 15 - len(prefix) - len(suffix)
	if maxPeerLen < 0 {
		maxPeerLen = 0
	}

	if len(cleanName) > maxPeerLen {
		cleanName = cleanName[:maxPeerLen]
	}

	// Truncated interface name must be valid (cannot end with "-" or "_")
	if suffix == "" {
		cleanName = strings.TrimRight(cleanName, "-_")
	}

	if cleanName == "" && suffix == "" {
		if ext {
			cleanName = "peer"
		} else {
			cleanName = "node"
		}
	}

	res := fmt.Sprintf("%s%s%s", prefix, cleanName, suffix)
	if len(res) > 15 {
		res = res[:15]
	}
	for len(res) > len(prefix) && (strings.HasSuffix(res, "-") || strings.HasSuffix(res, "_")) {
		res = strings.TrimRight(res, "-_")
	}
	return res
}

// GetInterfaceName returns the standard wg42<peer_name> interface name for internal peers,
// or wg42-<peer_name> for external peers.
func GetInterfaceName(peerName string, isExternal ...bool) string {
	return GetInterfaceNameWithSuffix(peerName, "", isExternal...)
}

// GetExternalInterfaceName returns the external wg42-<peer_name> interface name
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

// LinkIndexFromSuffix parses an interface suffix string ("" -> 0, "1" -> 1, "2" -> 2) into a link index.
func LinkIndexFromSuffix(suffix string) int {
	if suffix == "" {
		return 0
	}
	if idx, err := strconv.Atoi(suffix); err == nil && idx >= 0 {
		return idx
	}
	return 0
}

// getNodeMainIP extracts the primary IP (v4 or fallback) for a node.
func getNodeMainIP(n *config.Node) string {
	if n == nil {
		return ""
	}
	if strings.TrimSpace(n.IP) != "" {
		return strings.TrimSpace(n.IP)
	}
	if strings.TrimSpace(n.ExternalIP) != "" {
		return strings.TrimSpace(n.ExternalIP)
	}
	if strings.TrimSpace(n.IP6) != "" {
		return strings.TrimSpace(n.IP6)
	}
	return strings.TrimSpace(n.ExternalIP6)
}
