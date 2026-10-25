// Package model defines the wire format. v0.4 fields remain for rolling upgrades;
// new clients must use Known/State/CheckedAt rather than interpreting zero as OK.
package model

import "time"

const Version = "0.6.3-rc.2"

type NodeState string

const (
	Healthy  NodeState = "healthy"
	Warning  NodeState = "warning"
	Critical NodeState = "critical"
	Unknown  NodeState = "unknown"
)

type Check struct {
	OK        bool          `json:"ok"`
	State     string        `json:"state,omitempty"` // ok, error, unknown, disabled, stale
	Kind      string        `json:"kind,omitempty"`  // tcp, cql_options, http, query, systemd
	Latency   time.Duration `json:"latency"`         // nanoseconds, kept compatible with v0.4
	Message   string        `json:"message,omitempty"`
	CheckedAt time.Time     `json:"checked_at"`
}

func (c Check) Known() bool {
	return !c.CheckedAt.IsZero() && c.State != "unknown" && c.State != "disabled" && c.State != "stale"
}
func (c Check) Fresh(now time.Time, ttl time.Duration) bool {
	return c.Known() && now.Sub(c.CheckedAt) <= ttl && c.CheckedAt.Sub(now) < 5*time.Second
}
func UnknownCheck(message string) Check { return Check{State: "unknown", Message: message} }

type Node struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Address       string             `json:"address"`
	DC            string             `json:"dc,omitempty"`
	Rack          string             `json:"rack,omitempty"`
	Status        string             `json:"status,omitempty"`
	Up            bool               `json:"up"`
	MetadataKnown bool               `json:"metadata_known"`
	MetadataAt    time.Time          `json:"metadata_at"`
	Missing       bool               `json:"missing_from_topology"`
	LoadBytes     int64              `json:"load_bytes"`
	LoadKnown     bool               `json:"load_known"`
	HostID        string             `json:"host_id,omitempty"`
	Shards        int                `json:"shards,omitempty"`
	Version       string             `json:"version,omitempty"`
	CQL           Check              `json:"cql"`
	Internode     Check              `json:"internode"`
	WatchAPI      Check              `json:"watch_api"`
	Metrics       Check              `json:"metrics"`
	REST          Check              `json:"rest"`
	LocalService  Check              `json:"local_service"`
	MetricValues  map[string]float64 `json:"metric_values,omitempty"`
	// Deprecated v0.4 scalar fields retained only for display compatibility.
	Compactions        float64           `json:"compactions,omitempty"`
	PendingCompactions float64           `json:"pending_compactions,omitempty"`
	CompactionBacklog  float64           `json:"compaction_backlog,omitempty"`
	ReadQueueOverloads float64           `json:"read_queue_overloads,omitempty"`
	RaftWaiterDropped  float64           `json:"raft_waiter_dropped,omitempty"`
	ObservedBy         map[string]Check  `json:"observed_by,omitempty"`
	Labels             map[string]string `json:"labels,omitempty"`
	State              NodeState         `json:"state"`
	Reasons            []string          `json:"reasons"`
	AgentVersion       string            `json:"agent_version,omitempty"`
	AgentAt            time.Time         `json:"agent_at"`
	LastUpdatedAt      time.Time         `json:"last_updated_at"`
}
type Edge struct {
	From       string    `json:"from"`
	To         string    `json:"to"`
	CQL        Check     `json:"cql"`
	Internode  Check     `json:"internode"`
	WatchAPI   Check     `json:"watch_api"`
	Asymmetric bool      `json:"asymmetric"`
	ObservedAt time.Time `json:"observed_at"`
	Stale      bool      `json:"stale"`
}
type Event struct {
	ID       int64     `json:"id"`
	Time     time.Time `json:"time"`
	Severity string    `json:"severity"`
	Type     string    `json:"type"`
	Node     string    `json:"node,omitempty"`
	Message  string    `json:"message"`
	Resolved bool      `json:"resolved"`
}
type Incident struct {
	ID                  string     `json:"id"`
	Observer            string     `json:"observer"`
	Node                string     `json:"node,omitempty"`
	Kind                string     `json:"kind"`
	Severity            string     `json:"severity"`
	Message             string     `json:"message"`
	OpenedAt            time.Time  `json:"opened_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	ResolvedAt          *time.Time `json:"resolved_at,omitempty"`
	Acknowledged        bool       `json:"acknowledged"`
	SilencedUntil       *time.Time `json:"silenced_until,omitempty"`
	LastNotifiedAt      *time.Time `json:"last_notified_at,omitempty"`
	NotificationPending bool       `json:"notification_pending,omitempty"`
	RecoveryPending     bool       `json:"recovery_pending,omitempty"`
	RecoveryNotifiedAt  *time.Time `json:"recovery_notified_at,omitempty"`
	DeliveryError       string     `json:"delivery_error,omitempty"`
}
type Coverage struct {
	ExpectedNodes      int `json:"expected_nodes"`
	DiscoveredNodes    int `json:"discovered_nodes"`
	ReachableNodes     int `json:"reachable_nodes"`
	AgentsReady        int `json:"agents_ready"`
	ExpectedDirections int `json:"expected_directions"`
	ObservedDirections int `json:"observed_directions"`
}
type Snapshot struct {
	SchemaVersion     int        `json:"schema_version"`
	Version           string     `json:"version"`
	Demo              bool       `json:"demo,omitempty"`
	ClusterName       string     `json:"cluster_name"`
	LocalNode         string     `json:"local_node"`
	LocalNodeName     string     `json:"local_node_name,omitempty"`
	LocalAddress      string     `json:"local_address"`
	Health            NodeState  `json:"health"`
	Discovery         Check      `json:"discovery"`
	Coverage          Coverage   `json:"coverage"`
	Warnings          []string   `json:"warnings"`
	Nodes             []Node     `json:"nodes"`
	Edges             []Edge     `json:"edges"`
	Alerts            []Event    `json:"alerts,omitempty"`
	Incidents         []Incident `json:"incidents"`
	Events            []Event    `json:"events,omitempty"`
	GeneratedAt       time.Time  `json:"generated_at"`
	StaleAfterSeconds float64    `json:"stale_after_seconds"`
	CollectDurationMS float64    `json:"collect_duration_ms"`
}
type Sample struct {
	Time             time.Time `json:"time"`
	ClusterHealth    NodeState `json:"cluster_health"`
	NodesTotal       int       `json:"nodes_total"`
	NodesHealthy     int       `json:"nodes_healthy"`
	CQLHealthy       int       `json:"cql_healthy"`
	InternodeHealthy int       `json:"internode_healthy"`
	AverageLatencyMS *float64  `json:"average_latency_ms"`
	TotalLoadBytes   *int64    `json:"total_load_bytes"`
	DiscoveryOK      bool      `json:"discovery_ok"`
	AgentsReady      int       `json:"agents_ready"`
}
type QueryResult struct {
	Columns   []string         `json:"columns"`
	Rows      []map[string]any `json:"rows"`
	Count     int              `json:"count"`
	TookMS    int64            `json:"took_ms"`
	Truncated bool             `json:"truncated"`
}
