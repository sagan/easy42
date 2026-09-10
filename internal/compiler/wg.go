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
			nodeName := ""
			if selfNode != nil {
				nodeName = selfNode.Name
			}
			return nil, fmt.Errorf("failed to derive link-local address for node %s: %w", nodeName, err)
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

	allowedIPs := fmt.Sprintf("%s/128, 0.0.0.0/0, ::/0", peerAddrOnly)

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
		"peer_address_only":    peerAddrOnly,
		"allowed_ips":          allowedIPs,
		"endpoint":             endpoint,
		"persistent_keepalive": keepalive,

		// PascalCase aliases for template convenience
		"Address":             address,
		"ListenPort":          listenPort,
		"PrivateKey":          privateKey,
		"MTU":                 mtu,
		"PublicKey":           peerPublicKey,
		"PeerPublicKey":       peerPublicKey,
		"AllowedIPs":          allowedIPs,
		"PeerAddrOnly":        peerAddrOnly,
		"PeerAddressOnly":     peerAddrOnly,
		"Endpoint":            endpoint,
		"PersistentKeepalive": keepalive,
	}

	return ctx, nil
}

// GenerateWgConfigContentWithTemplate generates WireGuard config using a custom template
func GenerateWgConfigContentWithTemplate(
	tmplContent string,
	selfNode *config.Node,
	peerNode *config.Node,
	selfEnd *config.LinkEnd,
	peerEnd *config.LinkEnd,
	vault *crypto.KeyVault,
) (string, error) {
	ctx, err := BuildWgLinkContext(selfNode, peerNode, selfEnd, peerEnd, vault)
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
) (string, error) {
	tmplContent, err := GetDefaultWgTemplate()
	if err != nil {
		return "", err
	}
	return GenerateWgConfigContentWithTemplate(tmplContent, selfNode, peerNode, selfEnd, peerEnd, vault)
}

// GetInterfaceName returns the standard wg42<peer_name> interface name for internal peers,
// or wg42-<peer_name> for external peers.
func GetInterfaceName(peerName string, isExternal ...bool) string {
	cleanName := strings.TrimSpace(peerName)
	if len(isExternal) > 0 && isExternal[0] {
		if len(cleanName) > 10 {
			cleanName = cleanName[:10]
		}
		return fmt.Sprintf("wg42-%s", cleanName)
	}
	if len(cleanName) > 11 {
		cleanName = cleanName[:11]
	}
	return fmt.Sprintf("wg42%s", cleanName)
}

// GetExternalInterfaceName returns the external wg42-<peer_name> interface name (max 10 chars peer name)
func GetExternalInterfaceName(peerName string) string {
	return GetInterfaceName(peerName, true)
}
