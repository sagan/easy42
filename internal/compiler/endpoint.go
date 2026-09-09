package compiler

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"easy42/internal/config"
)

// LookupIPFunc defines the signature for IP lookup functions
type LookupIPFunc func(host string) ([]net.IP, error)

// DefaultLookupIP is the DNS IP resolver function used by ResolveEndpointToIP, swappable for testing
var DefaultLookupIP LookupIPFunc = net.LookupIP

// ResolveEndpointToIP resolves a domain/hostname inside an endpoint string (e.g. "vpn.example.com:51820")
// to an IP address (e.g. "1.2.3.4:51820" or "[2001:db8::1]:51820").
// If the endpoint already contains a numeric IP, or if resolution fails, it returns the endpoint unchanged.
func ResolveEndpointToIP(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}

	host, portStr, err := net.SplitHostPort(endpoint)
	if err != nil {
		// Might not have a port or might be malformed
		host = endpoint
		portStr = ""
	}

	// Remove brackets around IPv6 if any
	cleanHost := strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")

	// If already a valid IP address, return original endpoint
	if ip := net.ParseIP(cleanHost); ip != nil {
		return endpoint
	}

	// Perform DNS lookup
	ips, err := DefaultLookupIP(cleanHost)
	if err != nil || len(ips) == 0 {
		return endpoint
	}

	// Prefer IPv4 for compatibility, fallback to first available IP
	var selectedIP string
	for _, ip := range ips {
		if ip4 := ip.To4(); ip4 != nil {
			selectedIP = ip4.String()
			break
		}
	}
	if selectedIP == "" {
		selectedIP = ips[0].String()
	}

	if portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err == nil {
			return FormatHostPort(selectedIP, port)
		}
		if strings.Contains(selectedIP, ":") && !strings.HasPrefix(selectedIP, "[") {
			return fmt.Sprintf("[%s]:%s", selectedIP, portStr)
		}
		return fmt.Sprintf("%s:%s", selectedIP, portStr)
	}

	return selectedIP
}

// ResolveLinkEndpoint determines the remote peer endpoint for selfNode's connection to peerNode.
// If selfNode is external, the remote endpoint is always derived from the managed peerNode's entrypoint and listen port.
// If selfEnd.UseIp is true, any hostname in the endpoint will be resolved to an IP in easy42 server.
func ResolveLinkEndpoint(
	selfNode *config.Node,
	peerNode *config.Node,
	selfEnd *config.LinkEnd,
	peerEnd *config.LinkEnd,
) string {
	if selfEnd == nil {
		return ""
	}

	endpoint := ""

	// 1. If selfNode is an external peer, its remote endpoint is ALWAYS the managed peerNode's endpoint
	if selfNode != nil && selfNode.IsExternal && peerNode != nil && !peerNode.IsExternal {
		peerListenPort := 0
		if peerEnd != nil && peerEnd.ListenPort > 0 {
			peerListenPort = peerEnd.ListenPort
		} else if peerNode.IP != "" {
			peerListenPort = DerivePortFromIP(peerNode.IP)
		}
		derivedEP, _ := ResolvePeerEndpoint(selfNode, peerNode, nil, peerListenPort)
		endpoint = derivedEP
	}

	// 2. Otherwise, check if selfEnd.Endpoint is explicitly set
	if endpoint == "" && selfEnd.Endpoint != "" {
		endpoint = selfEnd.Endpoint
	}

	// 3. If empty, derive from peerNode's entrypoint and peerEnd's ListenPort
	if endpoint == "" && peerNode != nil {
		peerListenPort := 0
		if peerEnd != nil && peerEnd.ListenPort > 0 {
			peerListenPort = peerEnd.ListenPort
		} else if selfNode != nil && selfNode.IP != "" {
			peerListenPort = DerivePortFromIP(selfNode.IP)
		} else if peerNode.IP != "" {
			peerListenPort = DerivePortFromIP(peerNode.IP)
		}
		derivedEP, _ := ResolvePeerEndpoint(selfNode, peerNode, nil, peerListenPort)
		endpoint = derivedEP
	}

	if endpoint != "" && selfEnd.UseIp {
		endpoint = ResolveEndpointToIP(endpoint)
	}

	return endpoint
}

// ResolvePeerEndpoint resolves the endpoint string that nodeFrom should use to connect to nodeTo
func ResolvePeerEndpoint(nodeFrom *config.Node, nodeTo *config.Node, usedPorts map[int]bool, targetListenPort ...int) (string, int) {
	endpointStr, port, _ := ResolvePeerEndpointWithEntrypoint(nodeFrom, nodeTo, usedPorts, targetListenPort...)
	return endpointStr, port
}

// ResolvePeerEndpointWithEntrypoint resolves the endpoint string that nodeFrom should use to connect to nodeTo,
// and returns the resolved endpoint string, port, and the selected Entrypoint pointer.
func ResolvePeerEndpointWithEntrypoint(nodeFrom *config.Node, nodeTo *config.Node, usedPorts map[int]bool, targetListenPort ...int) (string, int, *config.Entrypoint) {
	if nodeTo == nil || len(nodeTo.Entrypoints) == 0 {
		return "", 0, nil
	}

	var selectedEP *config.Entrypoint

	// 1. Try to find matching tags between entrypoints
	if nodeFrom != nil {
		for _, epFrom := range nodeFrom.Entrypoints {
			for _, tagFrom := range epFrom.Tags {
				if strings.TrimSpace(tagFrom) == "" {
					continue
				}
				for i := range nodeTo.Entrypoints {
					epTo := &nodeTo.Entrypoints[i]
					for _, tagTo := range epTo.Tags {
						if strings.EqualFold(tagFrom, tagTo) && !epTo.IsNone() {
							selectedEP = epTo
							break
						}
					}
					if selectedEP != nil {
						break
					}
				}
				if selectedEP != nil {
					break
				}
			}
			if selectedEP != nil {
				break
			}
		}

		// 1b. Check node-level tags if no tag matched from entrypoints
		if selectedEP == nil {
			for _, tagFrom := range nodeFrom.Tags {
				if strings.TrimSpace(tagFrom) == "" {
					continue
				}
				for i := range nodeTo.Entrypoints {
					epTo := &nodeTo.Entrypoints[i]
					for _, tagTo := range epTo.Tags {
						if strings.EqualFold(tagFrom, tagTo) && !epTo.IsNone() {
							selectedEP = epTo
							break
						}
					}
					if selectedEP != nil {
						break
					}
				}
				if selectedEP != nil {
					break
				}
			}
		}
	}

	// 2. Fallback to first non-none endpoint of nodeTo
	if selectedEP == nil {
		for i := range nodeTo.Entrypoints {
			ep := &nodeTo.Entrypoints[i]
			if !ep.IsNone() {
				selectedEP = ep
				break
			}
		}
	}

	targetHost := ""
	if selectedEP != nil && !selectedEP.IsNone() {
		targetHost = selectedEP.IP
	} else if nodeTo.Host != "" {
		targetHost = nodeTo.Host
	}

	if targetHost == "" {
		return "", 0, nil
	}

	// 3. Resolve port
	port := 0
	if selectedEP != nil && len(selectedEP.Ports) > 0 {
		for _, ps := range selectedEP.Ports {
			if ps.Range != "" {
				start, end, err := ParsePortRange(ps.Range)
				if err == nil {
					for p := start; p <= end; p++ {
						if !usedPorts[p] {
							port = p
							break
						}
					}
					if port == 0 {
						port = start
					}
				}
			} else {
				p := ps.Port
				if ps.ExternalPort != 0 {
					p = ps.ExternalPort
				}
				if !usedPorts[p] || port == 0 {
					port = p
				}
			}
			if port != 0 {
				break
			}
		}
	}

	if port == 0 {
		if len(targetListenPort) > 0 && targetListenPort[0] > 0 {
			port = targetListenPort[0]
		} else if nodeFrom != nil && nodeFrom.IP != "" {
			// By default, nodeTo listens on port derived from nodeFrom.IP
			port = DerivePortFromIP(nodeFrom.IP)
		} else if nodeTo.IP != "" {
			port = DerivePortFromIP(nodeTo.IP)
		} else {
			port = 51820
		}
	}

	endpointStr := FormatHostPort(targetHost, port)
	return endpointStr, port, selectedEP
}
