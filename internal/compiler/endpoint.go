package compiler

import (
	"easy42/internal/config"
	"strings"
)

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

	// 1. Try to find matching tags between entrypoints
	var selectedEP *config.Entrypoint
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

	// If no valid endpoint found (e.g. nodeTo is strictly behind NAT/none)
	if selectedEP == nil || selectedEP.IsNone() {
		return "", 0, nil
	}

	// 3. Resolve port
	port := 0
	if len(selectedEP.Ports) > 0 {
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
		} else if nodeFrom != nil {
			// By default, nodeTo listens on port derived from nodeFrom.IP
			port = DerivePortFromIP(nodeFrom.IP)
		} else {
			port = DerivePortFromIP(nodeTo.IP)
		}
	}

	endpointStr := FormatHostPort(selectedEP.IP, port)
	return endpointStr, port, selectedEP
}
