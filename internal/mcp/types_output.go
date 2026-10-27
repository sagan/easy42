package mcp

import (
	"time"

	agentpb "easy42/internal/agent/proto"
	"easy42/internal/config"
	"easy42/internal/engine"
	"easy42/tasks"
)

// SimpleMessageOutput represents a standard acknowledgement or confirmation message
type SimpleMessageOutput struct {
	Message string `json:"message"`
}

// ClusterNodeSummary represents a node summary inside ClusterInfoOutput
type ClusterNodeSummary struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	IP6  string `json:"ip6"`
	ASN  uint64 `json:"asn"`
}

// ClusterInfoOutput represents high-level cluster metadata
type ClusterInfoOutput struct {
	NetworkName   string               `json:"network_name"`
	PublicASN     uint64               `json:"public_asn"`
	Version       string               `json:"version"`
	NodeCount     int                  `json:"node_count"`
	LinkCount     int                  `json:"link_count"`
	PolicyCount   int                  `json:"policy_count"`
	TemplateCount int                  `json:"template_count"`
	Role          string               `json:"role"`
	Nodes         []ClusterNodeSummary `json:"nodes"`
	Hint          string               `json:"hint"`
}

// --- Node Tools Outputs ---

type ListNodesOutput struct {
	Nodes []config.Node `json:"nodes"`
}

type GetNodeOutput struct {
	Node           *config.Node `json:"node"`
	BirdConfig     string       `json:"bird_config,omitempty"`
	BirdError      string       `json:"bird_error,omitempty"`
	NftablesConfig string       `json:"nftables_config,omitempty"`
	NftablesError  string       `json:"nftables_error,omitempty"`
}

type NodeMutationOutput struct {
	Message string      `json:"message"`
	Node    config.Node `json:"node"`
}

type CommandOutput struct {
	Message string `json:"message"`
	Output  string `json:"output,omitempty"`
}

type NodeTelemetryOutput struct {
	Node      string                  `json:"node"`
	Connected bool                    `json:"connected"`
	LastSeen  *time.Time              `json:"last_seen,omitempty"`
	Telemetry *agentpb.TelemetryReport `json:"telemetry,omitempty"`
}

type FleetLiveStatusOutput struct {
	Summary config.FleetMetricsSummary `json:"summary"`
	Nodes   []config.NodeLiveStatus    `json:"nodes"`
}

// --- Link Tools Outputs ---

type ListLinksOutput struct {
	Links []config.Link `json:"links"`
}

type LinkMutationOutput struct {
	Message string      `json:"message"`
	Link    config.Link `json:"link"`
}

type CreateMeshLinksOutput struct {
	Message string         `json:"message"`
	Links   []*config.Link `json:"links"`
}

type ResetLinkPskOutput struct {
	Message string `json:"message"`
	From    string `json:"from"`
	To      string `json:"to"`
	Psk     string `json:"psk"`
}

type ResetLinkPrivateKeyOutput struct {
	Message   string `json:"message"`
	Node      string `json:"node"`
	PublicKey string `json:"public_key"`
	Note      string `json:"note"`
	Private   string `json:"private"`
}

// --- Network Tools Outputs ---

type UpdateNetworkSettingsOutput struct {
	Message  string                 `json:"message"`
	Settings config.NetworkSettings `json:"settings"`
}

type GetIPBlocksOutput struct {
	Blocks []config.Block `json:"blocks"`
}

type UpdateIPBlocksOutput struct {
	Message string         `json:"message"`
	Blocks  []config.Block `json:"blocks"`
}

// --- Policy Tools Outputs ---

type ListNetworkPoliciesOutput struct {
	Policies []config.NetworkPolicy `json:"policies"`
}

type NetworkPolicyMutationOutput struct {
	Message string               `json:"message"`
	Policy  config.NetworkPolicy `json:"policy"`
}

// --- Template Tools Outputs ---

type ListTemplatesOutput struct {
	Templates []config.ConfigTemplate `json:"templates"`
}

type TemplateMutationOutput struct {
	Message  string                `json:"message"`
	Template config.ConfigTemplate `json:"template"`
}

// --- Config Apply Tools Outputs ---

type ListConfigChangesOutput struct {
	Changes []config.ConfigChange `json:"changes"`
}

type SaveConfigChangeOutput struct {
	Message string               `json:"message"`
	Change  *config.ConfigChange `json:"change"`
}

// --- Sync Tools Outputs ---

type PreviewSyncOutput struct {
	Actions []config.SyncAction `json:"actions"`
}

type ExecuteSyncOutput struct {
	Results []config.SyncResult `json:"results"`
}

type SyncStatusOutput struct {
	LastSync time.Time           `json:"last_sync"`
	Results  []config.SyncResult `json:"results"`
}

// --- Device Helper Tasks Outputs ---

type ListTasksOutput struct {
	Tasks []tasks.TaskMeta `json:"tasks"`
}

type TaskSourcesOutput struct {
	Sources []tasks.TaskSourceFile `json:"sources"`
}

type CheckTaskStatusOutput struct {
	Results map[string]tasks.TaskStatusResult `json:"results"`
}

type RunTaskOutput struct {
	Results map[string]tasks.TaskRunResult `json:"results"`
}

// --- Looking Glass Outputs ---

type SaveLookingGlassTaskOutput struct {
	Message string                  `json:"message"`
	Task    config.LookingGlassTask `json:"task"`
}

// --- Cron Tasks Outputs ---

type ListCronTasksOutput struct {
	Tasks []config.CronTask `json:"tasks"`
}

type CronTaskMutationOutput struct {
	Message string          `json:"message"`
	Task    config.CronTask `json:"task"`
}

// --- Database & Integrations Outputs ---

type ReclaimDatabaseOutput struct {
	Message  string              `json:"message"`
	Database engine.DatabaseInfo `json:"database"`
}

type ResetPrometheusTokenOutput struct {
	Message             string `json:"message"`
	PrometheusAuthToken string `json:"prometheus_auth_token"`
}
