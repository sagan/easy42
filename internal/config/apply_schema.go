package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ValidNodeNameRegex enforces alphanumeric characters, hyphens, and underscores
var ValidNodeNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ConfigChangeJSONSchema provides the official JSON Schema (Draft-07) for easy42 config changes
const ConfigChangeJSONSchema = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "Easy42ConfigChange",
  "description": "Strict JSON Schema for easy42 configuration change objects",
  "$ref": "#/definitions/ConfigPatch",
  "definitions": {
    "ConfigPatch": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "network_settings": { "$ref": "#/definitions/NetworkSettingsPatch" },
        "network_policies": {
          "type": "array",
          "items": { "$ref": "#/definitions/NetworkPolicy" }
        },
        "nodes": {
          "type": "array",
          "items": { "$ref": "#/definitions/NodePatch" }
        },
        "links": {
          "type": "array",
          "items": { "$ref": "#/definitions/LinkPatch" }
        },
        "blocks": {
          "type": "array",
          "items": { "$ref": "#/definitions/Block" }
        },
        "dns": { "$ref": "#/definitions/DNSPatch" },
        "templates": {
          "type": "array",
          "items": { "$ref": "#/definitions/ConfigTemplate" }
        },
        "server_tasks": {
          "type": "array",
          "items": { "$ref": "#/definitions/ServerTaskConfig" }
        },
        "cron_tasks": {
          "type": "array",
          "items": { "$ref": "#/definitions/CronTask" }
        },
        "database": { "$ref": "#/definitions/DatabaseSettingsPatch" },
        "integrations": { "$ref": "#/definitions/IntegrationsSettingsPatch" }
      }
    },
    "NetworkSettingsPatch": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "public_asn": { "type": "integer", "minimum": 1, "maximum": 4294967295 },
        "prefixes": { "type": "array", "items": { "type": "string" } },
        "disallowed_dn42_networks": { "type": "array", "items": { "type": "string" } },
        "disallowed_dn42_cidrs": { "type": "array", "items": { "type": "string" } },
        "flapalerted_address": { "type": "string" },
        "flapalerted_port": { "type": "integer", "minimum": 1, "maximum": 65535 },
        "flapalerted_asn": { "type": "integer", "minimum": 1, "maximum": 4294967295 },
        "flapalerted_url": { "type": "string" }
      }
    },
    "NodePatch": {
      "type": "object",
      "required": ["name"],
      "additionalProperties": false,
      "properties": {
        "name": { "type": "string", "pattern": "^[a-zA-Z0-9_-]+$", "minLength": 1, "maxLength": 64 },
        "host": { "type": "string" },
        "is_external": { "type": "boolean" },
        "description": { "type": "string" },
        "note": { "type": "string" },
        "ip": { "type": "string" },
        "ip6": { "type": "string" },
        "external_ip": { "type": "string" },
        "external_ip6": { "type": "string" },
        "interface": { "type": "string" },
        "asn": { "type": "integer", "minimum": 1, "maximum": 4294967295 },
        "entrypoints": {
          "type": "array",
          "items": { "$ref": "#/definitions/EntrypointPatch" }
        },
        "tags": { "type": "array", "items": { "type": "string" } },
        "table": { "type": "integer", "minimum": 0, "maximum": 65535 },
        "external_table": { "type": "integer", "minimum": 0, "maximum": 65535 },
        "internet_table": { "type": "integer", "minimum": 0, "maximum": 65535 },
        "metric": { "type": ["integer", "null"] },
        "static_routes": { "type": "array", "items": { "type": "string" } },
        "routes": {
          "type": "array",
          "items": { "$ref": "#/definitions/KernelRouteRule" }
        },
        "config_hooks": {
          "type": "array",
          "items": { "$ref": "#/definitions/ConfigHook" }
        },
        "wg_template": { "type": "string" },
        "bird_template": { "type": "string" },
        "nft_template": { "type": "string" },
        "x": { "type": ["number", "null"] },
        "y": { "type": ["number", "null"] },
        "mode": { "type": "string", "enum": ["ssh", "agent", ""] },
        "enable_fa": { "type": "boolean" },
        "update_hosts": { "type": "boolean" }
      }
    },
    "EntrypointPatch": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "ip": { "type": "string" },
        "ports": { "type": "array" },
        "tags": { "type": "array", "items": { "type": "string" } },
        "mtu": { "type": "integer", "minimum": 0, "maximum": 9000 }
      }
    },
    "LinkPatch": {
      "type": "object",
      "required": ["from", "to"],
      "additionalProperties": false,
      "properties": {
        "type": { "type": "string", "enum": ["wireguard", "manual", ""] },
        "from": { "$ref": "#/definitions/LinkEndPatch" },
        "to": { "$ref": "#/definitions/LinkEndPatch" },
        "tags": { "type": "array", "items": { "type": "string" } },
        "assign_ipv4": { "type": "boolean" },
        "psk": { "type": "string" }
      }
    },
    "LinkEndPatch": {
      "type": "object",
      "required": ["name"],
      "additionalProperties": false,
      "properties": {
        "name": { "type": "string", "pattern": "^[a-zA-Z0-9_-]+$", "minLength": 1 },
        "type": { "type": "string", "enum": ["wireguard", "manual", ""] },
        "interface": { "type": "string" },
        "address": { "type": "string" },
        "address4": { "type": "string" },
        "neighbor_address": { "type": "string" },
        "listen_port": { "type": "integer", "minimum": 0, "maximum": 65535 },
        "endpoint": { "type": "string" },
        "persistent_keepalive": { "type": "integer", "minimum": 0, "maximum": 3600 },
        "mtu": { "type": "integer", "minimum": 0, "maximum": 9000 },
        "use_ip": { "type": "boolean" },
        "policy": { "type": "string" },
        "cost": { "type": "integer", "minimum": 0 },
        "fwmark": { "type": "string" },
        "preference": { "type": ["integer", "null"] },
        "mark": { "type": "string" },
        "routing_policy": { "type": "string" },
        "note": { "type": "string" }
      }
    },
    "DNSPatch": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "zone_id": { "type": "string" },
        "api_token": { "type": "string" },
        "base_domain": { "type": "string" },
        "publish_ipv6_own_name": { "type": "boolean" }
      }
    },
    "DatabaseSettingsPatch": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "retention_days": { "type": "integer", "minimum": 1 },
        "raw_retention_days": { "type": "integer", "minimum": 1 }
      }
    },
    "IntegrationsSettingsPatch": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "prometheus_enabled": { "type": "boolean" },
        "prometheus_auth_token": { "type": "string" }
      }
    },
    "NetworkPolicy": { "type": "object" },
    "Block": { "type": "object" },
    "ConfigTemplate": { "type": "object" },
    "ServerTaskConfig": { "type": "object" },
    "CronTask": { "type": "object" },
    "KernelRouteRule": {
      "type": "object",
      "additionalProperties": false,
      "required": ["table", "prefixes"],
      "properties": {
        "table": { "type": "integer", "minimum": 0 },
        "prefixes": { "type": "array", "items": { "type": "string" } }
      }
    },
    "ConfigHook": {
      "type": "object",
      "additionalProperties": false,
      "required": ["type", "content"],
      "properties": {
        "type": { "type": "string" },
        "target": { "type": "string" },
        "content": { "type": "string" }
      }
    }
  }
}`

// ConfigPatch represents the validated top-level patch object
type ConfigPatch struct {
	NetworkSettings *NetworkSettingsPatch  `json:"network_settings,omitempty" yaml:"network_settings,omitempty"`
	NetworkPolicies *[]NetworkPolicy       `json:"network_policies,omitempty" yaml:"network_policies,omitempty"`
	Nodes           *[]NodePatch           `json:"nodes,omitempty" yaml:"nodes,omitempty"`
	Links           *[]LinkPatch           `json:"links,omitempty" yaml:"links,omitempty"`
	Blocks          *[]Block               `json:"blocks,omitempty" yaml:"blocks,omitempty"`
	DNS             *DNSPatch              `json:"dns,omitempty" yaml:"dns,omitempty"`
	Templates       *[]ConfigTemplate      `json:"templates,omitempty" yaml:"templates,omitempty"`
	ServerTasks     *[]ServerTaskConfig    `json:"server_tasks,omitempty" yaml:"server_tasks,omitempty"`
	CronTasks       *[]CronTask            `json:"cron_tasks,omitempty" yaml:"cron_tasks,omitempty"`
	Database        *DatabaseSettingsPatch `json:"database,omitempty" yaml:"database,omitempty"`
	Integrations    *IntegrationsSettingsPatch `json:"integrations,omitempty" yaml:"integrations,omitempty"`
}

// NetworkSettingsPatch holds partial changes to global network settings
type NetworkSettingsPatch struct {
	PublicASN              *uint64   `json:"public_asn,omitempty" yaml:"public_asn,omitempty"`
	Prefixes               *[]string `json:"prefixes,omitempty" yaml:"prefixes,omitempty"`
	DisallowedDN42Networks *[]string `json:"disallowed_dn42_networks,omitempty" yaml:"disallowed_dn42_networks,omitempty"`
	DisallowedDN42CIDRs    *[]string `json:"disallowed_dn42_cidrs,omitempty" yaml:"disallowed_dn42_cidrs,omitempty"`
	FlapAlertedAddress     *string   `json:"flapalerted_address,omitempty" yaml:"flapalerted_address,omitempty"`
	FlapAlertedPort        *int      `json:"flapalerted_port,omitempty" yaml:"flapalerted_port,omitempty"`
	FlapAlertedASN         *uint64   `json:"flapalerted_asn,omitempty" yaml:"flapalerted_asn,omitempty"`
	FlapAlertedURL         *string   `json:"flapalerted_url,omitempty" yaml:"flapalerted_url,omitempty"`
}

// NodePatch holds partial changes to a node
type NodePatch struct {
	Name          string             `json:"name" yaml:"name"`
	Host          *string            `json:"host,omitempty" yaml:"host,omitempty"`
	IsExternal    *bool              `json:"is_external,omitempty" yaml:"is_external,omitempty"`
	Description   *string            `json:"description,omitempty" yaml:"description,omitempty"`
	Note          *string            `json:"note,omitempty" yaml:"note,omitempty"`
	IP            *string            `json:"ip,omitempty" yaml:"ip,omitempty"`
	IP6           *string            `json:"ip6,omitempty" yaml:"ip6,omitempty"`
	ExternalIP    *string            `json:"external_ip,omitempty" yaml:"external_ip,omitempty"`
	ExternalIP6   *string            `json:"external_ip6,omitempty" yaml:"external_ip6,omitempty"`
	Interface     *string            `json:"interface,omitempty" yaml:"interface,omitempty"`
	ASN           *uint64            `json:"asn,omitempty" yaml:"asn,omitempty"`
	Entrypoints   *[]Entrypoint      `json:"entrypoints,omitempty" yaml:"entrypoints,omitempty"`
	Tags          *[]string          `json:"tags,omitempty" yaml:"tags,omitempty"`
	Table         *int               `json:"table,omitempty" yaml:"table,omitempty"`
	ExternalTable *int               `json:"external_table,omitempty" yaml:"external_table,omitempty"`
	InternetTable *int               `json:"internet_table,omitempty" yaml:"internet_table,omitempty"`
	Metric        *int               `json:"metric,omitempty" yaml:"metric,omitempty"`
	StaticRoutes  *[]string          `json:"static_routes,omitempty" yaml:"static_routes,omitempty"`
	Routes        *[]KernelRouteRule `json:"routes,omitempty" yaml:"routes,omitempty"`
	ConfigHooks   *[]ConfigHook      `json:"config_hooks,omitempty" yaml:"config_hooks,omitempty"`
	WgTemplate    *string            `json:"wg_template,omitempty" yaml:"wg_template,omitempty"`
	BirdTemplate  *string            `json:"bird_template,omitempty" yaml:"bird_template,omitempty"`
	NftTemplate   *string            `json:"nft_template,omitempty" yaml:"nft_template,omitempty"`
	X             *float64           `json:"x,omitempty" yaml:"x,omitempty"`
	Y             *float64           `json:"y,omitempty" yaml:"y,omitempty"`
	Mode          *string            `json:"mode,omitempty" yaml:"mode,omitempty"`
	EnableFA      *bool              `json:"enable_fa,omitempty" yaml:"enable_fa,omitempty"`
	UpdateHosts   *bool              `json:"update_hosts,omitempty" yaml:"update_hosts,omitempty"`
}

// LinkEndPatch holds partial changes to a link endpoint
type LinkEndPatch struct {
	Name                string  `json:"name" yaml:"name"`
	Type                *string `json:"type,omitempty" yaml:"type,omitempty"`
	Interface           *string `json:"interface,omitempty" yaml:"interface,omitempty"`
	Address             *string `json:"address,omitempty" yaml:"address,omitempty"`
	Address4            *string `json:"address4,omitempty" yaml:"address4,omitempty"`
	NeighborAddress     *string `json:"neighbor_address,omitempty" yaml:"neighbor_address,omitempty"`
	ListenPort          *int    `json:"listen_port,omitempty" yaml:"listen_port,omitempty"`
	Endpoint            *string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	PersistentKeepalive *int    `json:"persistent_keepalive,omitempty" yaml:"persistent_keepalive,omitempty"`
	MTU                 *int    `json:"mtu,omitempty" yaml:"mtu,omitempty"`
	UseIp               *bool   `json:"use_ip,omitempty" yaml:"use_ip,omitempty"`
	Policy              *string `json:"policy,omitempty" yaml:"policy,omitempty"`
	Cost                *int    `json:"cost,omitempty" yaml:"cost,omitempty"`
	Fwmark              *string `json:"fwmark,omitempty" yaml:"fwmark,omitempty"`
	Preference          *int    `json:"preference,omitempty" yaml:"preference,omitempty"`
	Mark                *string `json:"mark,omitempty" yaml:"mark,omitempty"`
	RoutingPolicy       *string `json:"routing_policy,omitempty" yaml:"routing_policy,omitempty"`
	Note                *string `json:"note,omitempty" yaml:"note,omitempty"`
}

// LinkPatch holds partial changes to a link
type LinkPatch struct {
	Type       *string       `json:"type,omitempty" yaml:"type,omitempty"`
	From       *LinkEndPatch `json:"from,omitempty" yaml:"from,omitempty"`
	To         *LinkEndPatch `json:"to,omitempty" yaml:"to,omitempty"`
	Tags       *[]string     `json:"tags,omitempty" yaml:"tags,omitempty"`
	AssignIPv4 *bool         `json:"assign_ipv4,omitempty" yaml:"assign_ipv4,omitempty"`
	Psk        *string       `json:"psk,omitempty" yaml:"psk,omitempty"`
}

// DNSPatch holds partial changes to DNS settings
type DNSPatch struct {
	ZoneID             *string `json:"zone_id,omitempty" yaml:"zone_id,omitempty"`
	APIToken           *string `json:"api_token,omitempty" yaml:"api_token,omitempty"`
	BaseDomain         *string `json:"base_domain,omitempty" yaml:"base_domain,omitempty"`
	PublishIPv6OwnName *bool   `json:"publish_ipv6_own_name,omitempty" yaml:"publish_ipv6_own_name,omitempty"`
}

// DatabaseSettingsPatch holds partial changes to database settings
type DatabaseSettingsPatch struct {
	RetentionDays    *int `json:"retention_days,omitempty" yaml:"retention_days,omitempty"`
	RawRetentionDays *int `json:"raw_retention_days,omitempty" yaml:"raw_retention_days,omitempty"`
}

// IntegrationsSettingsPatch holds partial changes to integrations
type IntegrationsSettingsPatch struct {
	PrometheusEnabled   *bool   `json:"prometheus_enabled,omitempty" yaml:"prometheus_enabled,omitempty"`
	PrometheusAuthToken *string `json:"prometheus_auth_token,omitempty" yaml:"prometheus_auth_token,omitempty"`
}

// ForbiddenFieldErrors checks for prohibited or internal-only fields
var forbiddenRootKeys = map[string]string{
	"password_hash":  "sensitive authentication secret",
	"encrypted_dek":  "sensitive encryption key",
	"session_secret": "session secret",
}

var forbiddenNodeKeys = map[string]string{
	"modified_at":     "system-managed timestamp",
	"agent_version":   "runtime daemon reported telemetry",
	"agent_last_seen": "runtime daemon reported telemetry",
}

var forbiddenLinkKeys = map[string]string{
	"modified_at":       "system-managed timestamp",
	"resolved_endpoint": "system-derived peer endpoint",
}

// ParseAndValidateConfigChange parses a JSON or YAML config snippet and performs strict schema verification
func ParseAndValidateConfigChange(raw []byte) (*ConfigPatch, string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, "", errors.New("config change snippet is empty")
	}

	// Determine format (JSON vs YAML)
	format := "json"
	var jsonData []byte
	if trimmed[0] == '{' || trimmed[0] == '[' {
		jsonData = trimmed
	} else {
		// Parse YAML to generic map, then encode to JSON for unified strict schema decoding
		var yamlObj interface{}
		if err := yaml.Unmarshal(trimmed, &yamlObj); err != nil {
			return nil, "", fmt.Errorf("invalid YAML syntax: %w", err)
		}
		var err error
		jsonData, err = json.Marshal(yamlObj)
		if err != nil {
			return nil, "", fmt.Errorf("failed to convert YAML to JSON: %w", err)
		}
		format = "yaml"
	}

	// 1. Raw JSON structural inspection for forbidden fields
	var genericMap map[string]interface{}
	if err := json.Unmarshal(jsonData, &genericMap); err != nil {
		return nil, format, fmt.Errorf("invalid JSON object: %w", err)
	}

	for k, reason := range forbiddenRootKeys {
		if _, exists := genericMap[k]; exists {
			return nil, format, fmt.Errorf("forbidden field '%s': %s cannot be modified via config change", k, reason)
		}
	}

	// 2. Standard Multi-Resource ConfigPatch decoding with DisallowUnknownFields
	dec := json.NewDecoder(bytes.NewReader(jsonData))
	dec.DisallowUnknownFields()
	var patch ConfigPatch
	if err := dec.Decode(&patch); err != nil {
		return nil, format, fmt.Errorf("strict schema validation failed: %w", err)
	}

	// Check forbidden fields inside nodes
	if patch.Nodes != nil {
		if rawNodes, ok := genericMap["nodes"].([]interface{}); ok {
			for i, rn := range rawNodes {
				if rnm, ok := rn.(map[string]interface{}); ok {
					for k, reason := range forbiddenNodeKeys {
						if _, exists := rnm[k]; exists {
							return nil, format, fmt.Errorf("nodes[%d]: forbidden field '%s': %s cannot be modified via config change", i, k, reason)
						}
					}
				}
			}
		}
		for i := range *patch.Nodes {
			if err := validateNodePatch(&(*patch.Nodes)[i]); err != nil {
				return nil, format, fmt.Errorf("nodes[%d] (%s): %w", i, (*patch.Nodes)[i].Name, err)
			}
		}
	}

	// Check forbidden fields inside links
	if patch.Links != nil {
		if rawLinks, ok := genericMap["links"].([]interface{}); ok {
			for i, rl := range rawLinks {
				if rlm, ok := rl.(map[string]interface{}); ok {
					for k, reason := range forbiddenLinkKeys {
						if _, exists := rlm[k]; exists {
							return nil, format, fmt.Errorf("links[%d]: forbidden field '%s': %s cannot be modified via config change", i, k, reason)
						}
					}
				}
			}
		}
		for i := range *patch.Links {
			if err := validateLinkPatch(&(*patch.Links)[i]); err != nil {
				return nil, format, fmt.Errorf("links[%d]: %w", i, err)
			}
		}
	}

	// Validate NetworkSettings
	if patch.NetworkSettings != nil {
		if err := validateNetworkSettingsPatch(patch.NetworkSettings); err != nil {
			return nil, format, fmt.Errorf("network_settings: %w", err)
		}
	}

	// Validate Database
	if patch.Database != nil {
		if patch.Database.RetentionDays != nil && *patch.Database.RetentionDays <= 0 {
			return nil, format, errors.New("database.retention_days must be > 0")
		}
		if patch.Database.RawRetentionDays != nil && *patch.Database.RawRetentionDays <= 0 {
			return nil, format, errors.New("database.raw_retention_days must be > 0")
		}
	}

	return &patch, format, nil
}

func validateNodePatch(n *NodePatch) error {
	n.Name = strings.TrimSpace(n.Name)
	if n.Name == "" {
		return errors.New("node name is required")
	}
	if !ValidNodeNameRegex.MatchString(n.Name) {
		return fmt.Errorf("invalid node name '%s': must contain only alphanumeric characters, hyphens, and underscores", n.Name)
	}

	if n.IP != nil {
		clean := strings.TrimSpace(*n.IP)
		if clean != "" {
			parsed := net.ParseIP(clean)
			if parsed == nil || parsed.To4() == nil {
				return fmt.Errorf("invalid IPv4 address '%s' for node %s", clean, n.Name)
			}
		}
	}

	if n.ExternalIP != nil {
		clean := strings.TrimSpace(*n.ExternalIP)
		if clean != "" {
			parsed := net.ParseIP(clean)
			if parsed == nil || parsed.To4() == nil {
				return fmt.Errorf("invalid external IPv4 address '%s' for node %s", clean, n.Name)
			}
		}
	}

	if n.IP6 != nil {
		clean := strings.TrimSpace(*n.IP6)
		if idx := strings.Index(clean, "/"); idx != -1 {
			clean = strings.TrimSpace(clean[:idx])
		}
		if clean != "" {
			parsed := net.ParseIP(clean)
			if parsed == nil || parsed.To4() != nil {
				return fmt.Errorf("invalid IPv6 address '%s' for node %s", clean, n.Name)
			}
		}
	}

	if n.ExternalIP6 != nil {
		clean := strings.TrimSpace(*n.ExternalIP6)
		if idx := strings.Index(clean, "/"); idx != -1 {
			clean = strings.TrimSpace(clean[:idx])
		}
		if clean != "" {
			parsed := net.ParseIP(clean)
			if parsed == nil || parsed.To4() != nil {
				return fmt.Errorf("invalid external IPv6 address '%s' for node %s", clean, n.Name)
			}
		}
	}

	if n.ASN != nil && *n.ASN == 0 {
		return fmt.Errorf("ASN must be > 0 for node %s", n.Name)
	}

	if n.StaticRoutes != nil {
		for _, r := range *n.StaticRoutes {
			trimmed := strings.TrimSpace(r)
			if trimmed == "" {
				continue
			}
			if _, _, err := net.ParseCIDR(trimmed); err != nil {
				if parsed := net.ParseIP(trimmed); parsed == nil {
					return fmt.Errorf("invalid static route '%s' for node %s: must be a valid CIDR or IP", r, n.Name)
				}
			}
		}
	}

	if n.Table != nil && *n.Table < 0 {
		return fmt.Errorf("routing table must be >= 0 for node %s", n.Name)
	}
	if n.ExternalTable != nil && *n.ExternalTable < 0 {
		return fmt.Errorf("external routing table must be >= 0 for node %s", n.Name)
	}
	if n.InternetTable != nil && *n.InternetTable < 0 {
		return fmt.Errorf("internet routing table must be >= 0 for node %s", n.Name)
	}

	if n.Entrypoints != nil {
		for i, ep := range *n.Entrypoints {
			if ep.MTU < 0 || ep.MTU > 9000 {
				return fmt.Errorf("entrypoints[%d]: invalid MTU %d (must be between 0 and 9000)", i, ep.MTU)
			}
		}
	}

	if n.Mode != nil {
		mode := strings.ToLower(strings.TrimSpace(*n.Mode))
		if mode != "" && mode != "ssh" && mode != "agent" {
			return fmt.Errorf("invalid mode '%s': must be 'ssh' or 'agent'", *n.Mode)
		}
	}

	return nil
}

func validateLinkPatch(l *LinkPatch) error {
	if l.From == nil || strings.TrimSpace(l.From.Name) == "" {
		return errors.New("link 'from.name' is required")
	}
	if l.To == nil || strings.TrimSpace(l.To.Name) == "" {
		return errors.New("link 'to.name' is required")
	}

	l.From.Name = strings.TrimSpace(l.From.Name)
	l.To.Name = strings.TrimSpace(l.To.Name)

	if strings.EqualFold(l.From.Name, l.To.Name) {
		return fmt.Errorf("link endpoints cannot be the same node ('%s')", l.From.Name)
	}

	if !ValidNodeNameRegex.MatchString(l.From.Name) {
		return fmt.Errorf("invalid node name '%s' in link from", l.From.Name)
	}
	if !ValidNodeNameRegex.MatchString(l.To.Name) {
		return fmt.Errorf("invalid node name '%s' in link to", l.To.Name)
	}

	if l.From.ListenPort != nil && (*l.From.ListenPort < 0 || *l.From.ListenPort > 65535) {
		return fmt.Errorf("invalid from.listen_port %d: must be 0-65535", *l.From.ListenPort)
	}
	if l.To.ListenPort != nil && (*l.To.ListenPort < 0 || *l.To.ListenPort > 65535) {
		return fmt.Errorf("invalid to.listen_port %d: must be 0-65535", *l.To.ListenPort)
	}

	if l.From.MTU != nil && (*l.From.MTU < 0 || *l.From.MTU > 9000) {
		return fmt.Errorf("invalid from.mtu %d: must be 0-9000", *l.From.MTU)
	}
	if l.To.MTU != nil && (*l.To.MTU < 0 || *l.To.MTU > 9000) {
		return fmt.Errorf("invalid to.mtu %d: must be 0-9000", *l.To.MTU)
	}

	return nil
}

func validateNetworkSettingsPatch(s *NetworkSettingsPatch) error {
	if s.PublicASN != nil && *s.PublicASN == 0 {
		return errors.New("public_asn must be > 0")
	}
	if s.FlapAlertedPort != nil && (*s.FlapAlertedPort < 1 || *s.FlapAlertedPort > 65535) {
		return fmt.Errorf("invalid flapalerted_port %d: must be between 1 and 65535", *s.FlapAlertedPort)
	}
	return nil
}
