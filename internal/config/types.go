package config

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Config represents the top-level configuration stored in config.json
type Config struct {
	PasswordHash  string `json:"password_hash"`
	EncryptedDEK  string `json:"encrypted_dek"`
	SessionSecret string `json:"session_secret"`
	Nodes         []Node `json:"nodes"`
	Links         []Link `json:"links"`
}

// PortSpec handles single ports (51820), port ranges ("2000-2999"), or object ({port, external_port})
type PortSpec struct {
	Port         int    `json:"port,omitempty"`
	ExternalPort int    `json:"external_port,omitempty"`
	Range        string `json:"range,omitempty"`
}

func (p PortSpec) MarshalJSON() ([]byte, error) {
	if p.Range != "" {
		return json.Marshal(p.Range)
	}
	if p.ExternalPort != 0 && p.ExternalPort != p.Port {
		return json.Marshal(map[string]int{
			"port":          p.Port,
			"external_port": p.ExternalPort,
		})
	}
	if p.Port != 0 {
		return json.Marshal(p.Port)
	}
	return json.Marshal(map[string]int{
		"port":          p.Port,
		"external_port": p.ExternalPort,
	})
}

func (p *PortSpec) UnmarshalJSON(data []byte) error {
	// Try single integer
	var portNum int
	if err := json.Unmarshal(data, &portNum); err == nil {
		p.Port = portNum
		p.ExternalPort = portNum
		return nil
	}

	// Try string (range or single port as string)
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		if strings.Contains(str, "-") {
			p.Range = str
			return nil
		}
		if num, err := strconv.Atoi(str); err == nil {
			p.Port = num
			p.ExternalPort = num
			return nil
		}
		p.Range = str
		return nil
	}

	// Try object
	type portObj struct {
		Port         int `json:"port"`
		ExternalPort int `json:"external_port"`
	}
	var po portObj
	if err := json.Unmarshal(data, &po); err == nil {
		p.Port = po.Port
		if po.ExternalPort != 0 {
			p.ExternalPort = po.ExternalPort
		} else {
			p.ExternalPort = po.Port
		}
		return nil
	}

	return fmt.Errorf("invalid port spec: %s", string(data))
}

// Entrypoint represents an external entrypoint of a node
type Entrypoint struct {
	IP    string     `json:"ip,omitempty"`
	Ports []PortSpec `json:"ports,omitempty"`
	Tags  []string   `json:"tags,omitempty"`
	MTU   int        `json:"mtu,omitempty"`
}

// IsNone checks if this is the "none" endpoint (strictly behind NAT/firewall)
func (e *Entrypoint) IsNone() bool {
	return strings.TrimSpace(e.IP) == ""
}

// Node represents a device/node in the network
type Node struct {
	Name        string       `json:"name"`                 // Max 11 chars hostname
	Host        string       `json:"host"`                 // SSH host / alias / IP
	IP          string       `json:"ip"`                   // Main IPv4 (e.g. 192.168.100.1)
	Interface   string       `json:"interface"`            // Main IP interface name (e.g. lo, dn42, eth0)
	ASN         uint64       `json:"asn"`                  // AS Number (default in 4299420000..4299429999)
	Entrypoints []Entrypoint `json:"entrypoints,omitempty"`// External entrypoints
	Tags        []string     `json:"tags,omitempty"`
	X           *float64     `json:"x,omitempty"`          // Graph X coordinate
	Y           *float64     `json:"y,omitempty"`          // Graph Y coordinate
}

// LinkEnd represents one endpoint of a WireGuard link
type LinkEnd struct {
	Name                string `json:"name"`
	Interface           string `json:"interface"`            // e.g. wg42<peer>
	Address             string `json:"address"`              // e.g. fe80::192:168:100:10/64
	ListenPort          int    `json:"listen_port"`          // Local device wg listening port
	Endpoint            string `json:"endpoint,omitempty"`   // External access endpoint (optional)
	PrivateKey          string `json:"private_key,omitempty"`// Encrypted base64 wireguard private key
	PublicKey           string `json:"public_key"`           // Wireguard public key
	PersistentKeepalive int    `json:"persistent_keepalive"` // Keepalive interval (25 or 0)
	MTU                 int    `json:"mtu,omitempty"`
}

// Link represents a WireGuard link between two nodes
type Link struct {
	From LinkEnd  `json:"from"`
	To   LinkEnd  `json:"to"`
	Tags []string `json:"tags,omitempty"`
}

// InterfaceInfo represents an interface on a remote machine
type InterfaceInfo struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
	Up        bool     `json:"up"`
	Type      string   `json:"type,omitempty"`
	MTU       int      `json:"mtu,omitempty"`
}

// WgPeerStatus represents runtime WireGuard peer info
type WgPeerStatus struct {
	PublicKey           string    `json:"public_key"`
	Endpoint            string    `json:"endpoint"`
	AllowedIPs          []string  `json:"allowed_ips"`
	LatestHandshake     time.Time `json:"latest_handshake"`
	TransferRxBytes     int64     `json:"transfer_rx_bytes"`
	TransferTxBytes     int64     `json:"transfer_tx_bytes"`
	PersistentKeepalive int       `json:"persistent_keepalive"`
}

// WgInterfaceStatus represents runtime WireGuard interface status on device
type WgInterfaceStatus struct {
	Name       string         `json:"name"`
	PublicKey  string         `json:"public_key"`
	ListenPort int            `json:"listen_port"`
	Peers      []WgPeerStatus `json:"peers"`
}

// NodeStatus represents cached runtime status of a node
type NodeStatus struct {
	Name         string              `json:"name"`
	Host         string              `json:"host"`
	Connected    bool                `json:"connected"`
	LastSeen     time.Time           `json:"last_seen"`
	Hostname     string              `json:"hostname"`
	Interfaces   []InterfaceInfo     `json:"interfaces"`
	WgInterfaces []WgInterfaceStatus `json:"wg_interfaces"`
	Error        string              `json:"error,omitempty"`
}

// ActionType represents an action to execute on a node during sync
type ActionType string

const (
	ActionCreateConfig ActionType = "create_config"
	ActionUpdateConfig ActionType = "update_config"
	ActionDeleteConfig ActionType = "delete_config"
	ActionUpInterface  ActionType = "up_interface"
	ActionSyncConfig   ActionType = "sync_config"
	ActionDownInterface ActionType = "down_interface"
)

// SyncAction represents a planned action on a target node
type SyncAction struct {
	NodeName    string     `json:"node_name"`
	Host        string     `json:"host"`
	Type        ActionType `json:"type"`
	Interface   string     `json:"interface"`
	TargetFile  string     `json:"target_file"`
	FileContent string     `json:"file_content,omitempty"`
	Diff        string     `json:"diff,omitempty"`
	Command     string     `json:"command,omitempty"`
	Description string     `json:"description"`
}

// SyncResult represents the execution result of sync actions
type SyncResult struct {
	NodeName string    `json:"node_name"`
	Action   string    `json:"action"`
	Success  bool      `json:"success"`
	Error    string    `json:"error,omitempty"`
	Output   string    `json:"output,omitempty"`
	Duration float64   `json:"duration_ms"`
}
