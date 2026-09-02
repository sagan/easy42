package ssh

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"time"

	"easy42/internal/config"

	"golang.org/x/crypto/ssh"
)

// ProbeResult contains discovered information about a remote node
type ProbeResult struct {
	Hostname            string                 `json:"hostname"`
	SuggestedName       string                 `json:"suggested_name"`
	SuggestedIP         string                 `json:"suggested_ip"`
	SuggestedInterface  string                 `json:"suggested_interface"`
	SuggestedASN        uint64                 `json:"suggested_asn"`
	Interfaces          []config.InterfaceInfo `json:"interfaces"`
	DetectedEntrypoints []config.Entrypoint    `json:"detected_entrypoints"`
}

type ipJSONAddr struct {
	Family string `json:"family"`
	Local  string `json:"local"`
	Prefix int    `json:"prefixlen"`
}

type ipJSONInterface struct {
	IfName    string       `json:"ifname"`
	OperState string       `json:"operstate"`
	MTU       int          `json:"mtu"`
	AddrInfo  []ipJSONAddr `json:"addr_info"`
}

// ProbeHost connects to a remote host and discovers its network configuration
func ProbeHost(client *ssh.Client, host string, existingNodes []config.Node) (*ProbeResult, error) {
	// 1. Get Hostname
	hostnameOut, err := RunCommand(client, "hostname")
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}
	hostname := strings.TrimSpace(hostnameOut)

	// 2. Discover Interfaces using ip -j addr or ip addr
	var ifaces []config.InterfaceInfo
	jsonOut, err := RunCommand(client, "ip -j addr show")
	if err == nil {
		var rawIfaces []ipJSONInterface
		if jsonErr := json.Unmarshal([]byte(jsonOut), &rawIfaces); jsonErr == nil {
			for _, raw := range rawIfaces {
				info := config.InterfaceInfo{
					Name:      raw.IfName,
					Up:        strings.EqualFold(raw.OperState, "UP") || strings.EqualFold(raw.OperState, "UNKNOWN"),
					Addresses: make([]string, 0),
					MTU:       raw.MTU,
				}
				for _, addr := range raw.AddrInfo {
					info.Addresses = append(info.Addresses, fmt.Sprintf("%s/%d", addr.Local, addr.Prefix))
				}
				ifaces = append(ifaces, info)
			}
		}
	}

	// Fallback to text parsing if ip -j wasn't supported
	if len(ifaces) == 0 {
		txtOut, _ := RunCommand(client, "ip -o addr show")
		lines := strings.Split(strings.TrimSpace(txtOut), "\n")
		ifMap := make(map[string]*config.InterfaceInfo)
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				ifName := fields[1]
				if _, exists := ifMap[ifName]; !exists {
					ifMap[ifName] = &config.InterfaceInfo{
						Name:      ifName,
						Up:        true,
						Addresses: make([]string, 0),
					}
				}
				for i, f := range fields {
					if (f == "inet" || f == "inet6") && i+1 < len(fields) {
						ifMap[ifName].Addresses = append(ifMap[ifName].Addresses, fields[i+1])
					}
				}
			}
		}

		// Try to read MTU from ip -o link show
		linkOut, _ := RunCommand(client, "ip -o link show")
		for line := range strings.SplitSeq(strings.TrimSpace(linkOut), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				name := strings.TrimSuffix(fields[1], ":")
				if atIdx := strings.Index(name, "@"); atIdx != -1 {
					name = name[:atIdx]
				}
				for i, f := range fields {
					if f == "mtu" && i+1 < len(fields) {
						var mtuVal int
						if _, err := fmt.Sscanf(fields[i+1], "%d", &mtuVal); err == nil && mtuVal > 0 {
							if ifInfo, exists := ifMap[name]; exists {
								ifInfo.MTU = mtuVal
							}
						}
					}
				}
			}
		}

		for _, v := range ifMap {
			ifaces = append(ifaces, *v)
		}
	}

	// 3. Compute Suggested Node Name (max 11 chars, valid hostname)
	suggestedName := sanitizeNodeName(hostname)
	suggestedName = makeUniqueNodeName(suggestedName, existingNodes)

	// 4. Determine Suggested Main IPv4 and Interface
	var suggestedIP, suggestedIface string
	for _, iface := range ifaces {
		// Prefer lo or dummy or private interfaces
		for _, addr := range iface.Addresses {
			if strings.Contains(addr, ".") && !strings.HasPrefix(addr, "127.") {
				ipOnly := strings.Split(addr, "/")[0]
				if suggestedIP == "" || iface.Name == "lo" || strings.HasPrefix(iface.Name, "dummy") || strings.HasPrefix(iface.Name, "dn42") {
					suggestedIP = ipOnly
					suggestedIface = iface.Name
				}
			}
		}
	}

	// 5. Generate unique ASN in 4299420000-4299429999
	usedASNs := make(map[uint64]bool)
	for _, n := range existingNodes {
		usedASNs[n.ASN] = true
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	var suggestedASN uint64 = 4299420001
	for {
		candidate := uint64(4299420000 + r.Intn(9999) + 1)
		if !usedASNs[candidate] {
			suggestedASN = candidate
			break
		}
	}

	// Determine target interface's MTU (fallbacks to 1500)
	targetMTU := determineTargetMTU(client, host, ifaces, suggestedIface)

	// 6. Detected entrypoints
	var entrypoints []config.Entrypoint
	// If host is a direct IP or hostname, add it as entrypoint
	if host != "" && !strings.Contains(host, "/") {
		entrypoints = append(entrypoints, config.Entrypoint{
			IP:   host,
			Tags: []string{"default"},
			MTU:  targetMTU,
		})
	}
	// Always append the "none" endpoint
	entrypoints = append(entrypoints, config.Entrypoint{
		IP:   "",
		Tags: []string{"nat"},
	})

	return &ProbeResult{
		Hostname:            hostname,
		SuggestedName:       suggestedName,
		SuggestedIP:         suggestedIP,
		SuggestedInterface:  suggestedIface,
		SuggestedASN:        suggestedASN,
		Interfaces:          ifaces,
		DetectedEntrypoints: entrypoints,
	}, nil
}

var invalidCharRegex = regexp.MustCompile(`[^a-zA-Z0-9\-]`)

func sanitizeNodeName(raw string) string {
	cleaned := invalidCharRegex.ReplaceAllString(strings.ToLower(raw), "-")
	cleaned = strings.Trim(cleaned, "-")
	if len(cleaned) > 11 {
		cleaned = cleaned[:11]
	}
	if cleaned == "" {
		cleaned = "node"
	}
	return cleaned
}

func makeUniqueNodeName(base string, existing []config.Node) string {
	names := make(map[string]bool)
	for _, n := range existing {
		names[n.Name] = true
	}

	if !names[base] {
		return base
	}

	// Append suffix
	for i := 1; i <= 99; i++ {
		suffix := fmt.Sprintf("-%d", i)
		maxBaseLen := 11 - len(suffix)
		var candidate string
		if len(base) > maxBaseLen {
			candidate = base[:maxBaseLen] + suffix
		} else {
			candidate = base + suffix
		}
		if !names[candidate] {
			return candidate
		}
	}
	return base
}

func determineTargetMTU(client *ssh.Client, host string, ifaces []config.InterfaceInfo, suggestedIface string) int {
	targetMTU := 1500
	var targetIface *config.InterfaceInfo

	// 1. Try to match host directly with interface addresses
	cleanHost := strings.TrimSpace(host)
	if cleanHost != "" {
		for i := range ifaces {
			for _, addr := range ifaces[i].Addresses {
				ipOnly := strings.Split(addr, "/")[0]
				if ipOnly == cleanHost {
					targetIface = &ifaces[i]
					break
				}
			}
			if targetIface != nil {
				break
			}
		}
	}

	// 2. Try to match SSH_CONNECTION server IP with interface addresses
	if targetIface == nil && client != nil {
		sshConnOut, _ := RunCommand(client, "echo $SSH_CONNECTION")
		fields := strings.Fields(sshConnOut)
		if len(fields) >= 3 {
			serverIP := fields[2]
			for i := range ifaces {
				for _, addr := range ifaces[i].Addresses {
					ipOnly := strings.Split(addr, "/")[0]
					if ipOnly == serverIP {
						targetIface = &ifaces[i]
						break
					}
				}
				if targetIface != nil {
					break
				}
			}
		}
	}

	// 3. Try to get default route interface
	if targetIface == nil && client != nil {
		routeOut, _ := RunCommand(client, "ip -j route show default")
		type ipRouteJSON struct {
			Dev string `json:"dev"`
		}
		var routes []ipRouteJSON
		if json.Unmarshal([]byte(routeOut), &routes) == nil && len(routes) > 0 && routes[0].Dev != "" {
			for i := range ifaces {
				if ifaces[i].Name == routes[0].Dev {
					targetIface = &ifaces[i]
					break
				}
			}
		}
	}

	// 4. Fallback: text ip route show default
	if targetIface == nil && client != nil {
		routeTxt, _ := RunCommand(client, "ip route show default")
		fields := strings.Fields(routeTxt)
		for i, f := range fields {
			if f == "dev" && i+1 < len(fields) {
				devName := fields[i+1]
				for j := range ifaces {
					if ifaces[j].Name == devName {
						targetIface = &ifaces[j]
						break
					}
				}
				break
			}
		}
	}

	// 5. Fallback to suggested interface or first non-loopback UP interface
	if targetIface == nil && suggestedIface != "" && suggestedIface != "lo" {
		for i := range ifaces {
			if ifaces[i].Name == suggestedIface {
				targetIface = &ifaces[i]
				break
			}
		}
	}
	if targetIface == nil {
		for i := range ifaces {
			if ifaces[i].Name != "lo" && ifaces[i].Up {
				targetIface = &ifaces[i]
				break
			}
		}
	}

	if targetIface != nil && targetIface.MTU > 0 {
		targetMTU = targetIface.MTU
	}

	return targetMTU
}
