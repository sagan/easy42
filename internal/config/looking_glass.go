package config

// ParameterType defines the UI input type and validation rule for task parameters
type ParameterType string

const (
	ParamTypeString   ParameterType = "string"
	ParamTypeIPOrCIDR ParameterType = "ip_or_cidr"
	ParamTypeNumber   ParameterType = "number"
	ParamTypeSelect   ParameterType = "select"
	ParamTypeBoolean  ParameterType = "boolean"
)

// TaskParam defines a user-configurable parameter in a Looking Glass task
type TaskParam struct {
	Key          string        `json:"key"`
	Label        string        `json:"label"`
	Type         ParameterType `json:"type"`
	Description  string        `json:"description,omitempty"`
	DefaultValue string        `json:"default_value,omitempty"`
	Required     bool          `json:"required"`
	Options      []string      `json:"options,omitempty"`
	Regex        string        `json:"regex,omitempty"`
}

// LookingGlassTask defines a built-in or custom Looking Glass task
type LookingGlassTask struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Category    string      `json:"category"` // "BGP / Routing", "Connectivity", "Diagnostics", "Custom"
	CommandTmpl string      `json:"command_tmpl"`
	Parser      string      `json:"parser"` // "bird_route", "bird_protocols", "ping", "traceroute", "mtr", "raw"
	Params      []TaskParam `json:"params"`
	IsBuiltin   bool        `json:"is_builtin,omitempty"`
	TimeoutSec  int         `json:"timeout_sec,omitempty"`
}

// DefaultLookingGlassTasks returns the built-in Looking Glass task templates
func DefaultLookingGlassTasks() []LookingGlassTask {
	return []LookingGlassTask{
		{
			ID:          "ping",
			Name:        "Ping (ICMP)",
			Description: "Send ICMP ECHO_REQUEST packets to destination host or IP",
			Category:    "Connectivity",
			CommandTmpl: "ping -c {{count}} -W 2 {{target}}",
			Parser:      "ping",
			TimeoutSec:  20,
			IsBuiltin:   true,
			Params: []TaskParam{
				{
					Key:          "target",
					Label:        "Target Host or IP",
					Type:         ParamTypeIPOrCIDR,
					Description:  "IPv4 / IPv6 address or hostname",
					Required:     true,
					DefaultValue: "172.20.0.1",
				},
				{
					Key:          "count",
					Label:        "Packet Count",
					Type:         ParamTypeNumber,
					Description:  "Number of echo requests to send (1-20)",
					Required:     false,
					DefaultValue: "4",
				},
			},
		},
		{
			ID:          "traceroute",
			Name:        "Traceroute",
			Description: "Print the route packets trace to network host",
			Category:    "Connectivity",
			CommandTmpl: "traceroute -m {{max_ttl}} -w 2 {{target}}",
			Parser:      "traceroute",
			TimeoutSec:  35,
			IsBuiltin:   true,
			Params: []TaskParam{
				{
					Key:          "target",
					Label:        "Target Host or IP",
					Type:         ParamTypeIPOrCIDR,
					Description:  "IPv4 / IPv6 address or hostname",
					Required:     true,
					DefaultValue: "172.20.0.1",
				},
				{
					Key:          "max_ttl",
					Label:        "Max Hops (TTL)",
					Type:         ParamTypeNumber,
					Description:  "Max number of hops to probe (1-64)",
					Required:     false,
					DefaultValue: "30",
				},
			},
		},
		{
			ID:          "mtr",
			Name:        "MTR Report",
			Description: "Run My Traceroute in report mode for detailed hop statistics",
			Category:    "Connectivity",
			CommandTmpl: "mtr --report --report-cycles {{cycles}} --no-dns {{target}}",
			Parser:      "mtr",
			TimeoutSec:  45,
			IsBuiltin:   true,
			Params: []TaskParam{
				{
					Key:          "target",
					Label:        "Target Host or IP",
					Type:         ParamTypeIPOrCIDR,
					Description:  "IPv4 / IPv6 address or hostname",
					Required:     true,
					DefaultValue: "172.20.0.1",
				},
				{
					Key:          "cycles",
					Label:        "Report Cycles",
					Type:         ParamTypeNumber,
					Description:  "Number of pings per hop (1-10)",
					Required:     false,
					DefaultValue: "5",
				},
			},
		},
		{
			ID:          "bird_show_protocols",
			Name:        "BIRD Show Protocols",
			Description: "Display status of all BGP, OSPF, kernel, and device protocols",
			Category:    "BGP / Routing",
			CommandTmpl: "birdc show protocols {{protocol}}",
			Parser:      "bird_protocols",
			TimeoutSec:  15,
			IsBuiltin:   true,
			Params: []TaskParam{
				{
					Key:          "protocol",
					Label:        "Protocol Filter",
					Type:         ParamTypeString,
					Description:  "Optional protocol name (leave blank for all protocols)",
					Required:     false,
					DefaultValue: "",
				},
			},
		},
		{
			ID:          "bird_show_route",
			Name:        "BIRD Show Route",
			Description: "Query routing table entries for a specific destination IP or subnet",
			Category:    "BGP / Routing",
			CommandTmpl: "birdc show route for {{target}} {{scope}}",
			Parser:      "bird_route",
			TimeoutSec:  20,
			IsBuiltin:   true,
			Params: []TaskParam{
				{
					Key:          "target",
					Label:        "Target IP or Prefix",
					Type:         ParamTypeIPOrCIDR,
					Description:  "IPv4 or IPv6 address/prefix (e.g. 172.20.0.1 or 172.20.0.0/14)",
					Required:     true,
					DefaultValue: "172.20.0.1",
				},
				{
					Key:          "scope",
					Label:        "Route Scope",
					Type:         ParamTypeSelect,
					Description:  "all (include non-preferred routes), primary (best route only), or filtered",
					Required:     false,
					DefaultValue: "all",
					Options:      []string{"all", "primary", "filtered"},
				},
			},
		},
		{
			ID:          "bird_show_route_where",
			Name:        "BIRD Show Route Where",
			Description: "Query routing table filtered by custom BIRD condition",
			Category:    "BGP / Routing",
			CommandTmpl: "birdc show route where {{condition}}",
			Parser:      "bird_route",
			TimeoutSec:  25,
			IsBuiltin:   true,
			Params: []TaskParam{
				{
					Key:          "condition",
					Label:        "BIRD Filter Expression",
					Type:         ParamTypeString,
					Description:  "e.g. bgp_path ~ [= 4242421234 =] or net ~ [ 172.20.0.0/14+ ]",
					Required:     true,
					DefaultValue: "bgp_path ~ [= 4242420000 =]",
				},
			},
		},
		{
			ID:          "bird_show_status",
			Name:        "BIRD Status & Memory",
			Description: "Show BIRD daemon status, uptime, router ID, and memory usage",
			Category:    "BGP / Routing",
			CommandTmpl: "birdc show status && echo '---' && birdc show memory",
			Parser:      "raw",
			TimeoutSec:  10,
			IsBuiltin:   true,
			Params:      []TaskParam{},
		},
		{
			ID:          "ip_route_get",
			Name:        "Kernel Route (ip route get)",
			Description: "Query Linux kernel FIB routing decision for an IP",
			Category:    "Diagnostics",
			CommandTmpl: "ip route get {{target}}",
			Parser:      "raw",
			TimeoutSec:  10,
			IsBuiltin:   true,
			Params: []TaskParam{
				{
					Key:          "target",
					Label:        "Destination IP",
					Type:         ParamTypeIPOrCIDR,
					Description:  "IP address to query kernel route table",
					Required:     true,
					DefaultValue: "1.1.1.1",
				},
			},
		},
		{
			ID:          "whois_dn42",
			Name:        "DN42 WHOIS",
			Description: "Query DN42 WHOIS database for an ASN, subnet, or person handle",
			Category:    "Diagnostics",
			CommandTmpl: "whois -h whois.dn42 {{target}}",
			Parser:      "raw",
			TimeoutSec:  15,
			IsBuiltin:   true,
			Params: []TaskParam{
				{
					Key:          "target",
					Label:        "Query Target",
					Type:         ParamTypeString,
					Description:  "ASN (AS424242xxxx), CIDR, or maintainer handle",
					Required:     true,
					DefaultValue: "AS4242420000",
				},
			},
		},
	}
}
