package lookingglass

import "easy42/internal/config"

// RunRequest encapsulates an ad-hoc or templated Looking Glass execution request
type RunRequest struct {
	Nodes         []string          `json:"nodes"`                   // List of node names to execute on
	TaskID        string            `json:"task_id,omitempty"`       // If executing a known/saved task template
	AdHoc         bool              `json:"ad_hoc,omitempty"`         // If true, custom ad-hoc command
	CustomCommand string            `json:"custom_command,omitempty"`// Directly provided CLI command (sanitized)
	CustomParser  string            `json:"custom_parser,omitempty"` // Parser to use for ad-hoc command ("raw", "ping", etc.)
	Params        map[string]string `json:"params,omitempty"`        // Parameter values for interpolation
	TimeoutSec    int               `json:"timeout_sec,omitempty"`   // Execution timeout in seconds (capped at 60s)
}

// NodeResult contains the execution result on a single node
type NodeResult struct {
	NodeName   string      `json:"node_name"`
	TaskID     string      `json:"task_id,omitempty"`
	Command    string      `json:"command"`
	RawOutput  string      `json:"raw_output"`
	ExitCode   int         `json:"exit_code"`
	DurationMs int64       `json:"duration_ms"`
	Error      string      `json:"error,omitempty"`
	Parser     string      `json:"parser"`
	Parsed     interface{} `json:"parsed,omitempty"`
}

// RunResponse is the response returned to the client
type RunResponse struct {
	TaskID  string                `json:"task_id,omitempty"`
	Command string                `json:"command"`
	Results map[string]NodeResult `json:"results"` // Keyed by node name
}

// BirdProtocolEntry represents a row in `birdc show protocols`
type BirdProtocolEntry struct {
	Name      string `json:"name"`
	Proto     string `json:"proto"`
	Table     string `json:"table"`
	State     string `json:"state"` // "up", "down", "start", "stop"
	Since     string `json:"since"`
	Info      string `json:"info"` // "Established", "Active", "Connect", etc.
	Connected bool   `json:"connected"`
}

// BirdRouteEntry represents a route candidate in `birdc show route`
type BirdRouteEntry struct {
	Network     string   `json:"network"`
	Best        bool     `json:"best"`
	FromProto   string   `json:"from_proto"`
	Since       string   `json:"since"`
	Metric      string   `json:"metric,omitempty"`
	Via         string   `json:"via,omitempty"`
	Interface   string   `json:"interface,omitempty"`
	NextHop     string   `json:"next_hop,omitempty"`
	ASPath      []string `json:"as_path,omitempty"`
	Communities []string `json:"communities,omitempty"`
	LocalPref   string   `json:"local_pref,omitempty"`
	Origin      string   `json:"origin,omitempty"`
}

// BirdRouteResult wraps route list with target
type BirdRouteResult struct {
	Target string           `json:"target,omitempty"`
	Routes []BirdRouteEntry `json:"routes"`
}

// PingPacket represents a single ping reply line
type PingPacket struct {
	Seq    int     `json:"seq"`
	TTL    int     `json:"ttl"`
	TimeMs float64 `json:"time_ms"`
	Host   string  `json:"host,omitempty"`
}

// PingResult represents parsed ping statistics
type PingResult struct {
	Host             string       `json:"host"`
	IP               string       `json:"ip,omitempty"`
	PacketsSent      int          `json:"packets_sent"`
	PacketsReceived  int          `json:"packets_received"`
	PacketLossPct    float64      `json:"packet_loss_pct"`
	MinRTT           float64      `json:"min_rtt_ms"`
	AvgRTT           float64      `json:"avg_rtt_ms"`
	MaxRTT           float64      `json:"max_rtt_ms"`
	MdevRTT          float64      `json:"mdev_rtt_ms"`
	Packets          []PingPacket `json:"packets,omitempty"`
}

// TracerouteHop represents a hop in traceroute
type TracerouteHop struct {
	Hop      int       `json:"hop"`
	Host     string    `json:"host"`
	IP       string    `json:"ip,omitempty"`
	ASN      string    `json:"asn,omitempty"`
	Times    []float64 `json:"times_ms"`
	LossPct  float64   `json:"loss_pct,omitempty"`
	TimedOut bool      `json:"timed_out"`
}

// TracerouteResult represents parsed traceroute hops
type TracerouteResult struct {
	Target string          `json:"target"`
	Hops   []TracerouteHop `json:"hops"`
}

// MTRHop represents a row in MTR report
type MTRHop struct {
	Hop     int     `json:"hop"`
	Host    string  `json:"host"`
	IP      string  `json:"ip,omitempty"`
	LossPct float64 `json:"loss_pct"`
	Sent    int     `json:"sent"`
	Last    float64 `json:"last_ms"`
	Avg     float64 `json:"avg_ms"`
	Best    float64 `json:"best_ms"`
	Worst   float64 `json:"worst_ms"`
	StDev   float64 `json:"stdev_ms"`
}

// MTRResult wraps MTR report rows
type MTRResult struct {
	Target string   `json:"target"`
	Hops   []MTRHop `json:"hops"`
}

// TasksResponse represents the list of all available tasks
type TasksResponse struct {
	Builtin []config.LookingGlassTask `json:"builtin"`
	Custom  []config.LookingGlassTask `json:"custom"`
}
