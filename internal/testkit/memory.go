// Package testkit is used only by tests and cmd/preview, never by the production CLI.
package testkit

import (
	"context"
	"encoding/json"
	"fmt"
	"scyllascope/internal/core"
	"scyllascope/internal/model"
	"sort"
	"sync"
	"time"
)

func clone[T any](x T) T { b, _ := json.Marshal(x); var v T; json.Unmarshal(b, &v); return v }

type Memory struct {
	mu        sync.Mutex
	events    []model.Event
	incidents map[string]model.Incident
	samples   []model.Sample
	nodes     map[string][]model.Node
	top       model.Snapshot
}

func NewMemory() *Memory {
	return &Memory{incidents: map[string]model.Incident{}, nodes: map[string][]model.Node{}}
}
func (m *Memory) Add(e model.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e.ID = int64(len(m.events) + 1)
	m.events = append(m.events, e)
	return nil
}
func (m *Memory) Recent(n int) ([]model.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := []model.Event{}
	for i := len(m.events) - 1; i >= 0 && len(v) < n; i-- {
		v = append(v, m.events[i])
	}
	return clone(v), nil
}
func (m *Memory) Incidents(all bool) ([]model.Incident, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := []model.Incident{}
	for _, i := range m.incidents {
		if all || i.ResolvedAt == nil {
			v = append(v, i)
		}
	}
	sort.Slice(v, func(i, j int) bool { return v[i].UpdatedAt.After(v[j].UpdatedAt) })
	return clone(v), nil
}
func (m *Memory) WorkIncidents(after string, limit int) ([]model.Incident, error) {
	all, _ := m.Incidents(true)
	out := []model.Incident{}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	for _, v := range all {
		if v.ID > after && (v.ResolvedAt == nil || v.RecoveryPending || (v.NotificationPending && !v.Acknowledged)) {
			out = append(out, v)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}
func (m *Memory) Incident(id string) (model.Incident, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.incidents[id]
	if !ok {
		return v, fmt.Errorf("not found")
	}
	return clone(v), nil
}
func (m *Memory) PutIncident(i model.Incident) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.incidents[i.ID] = clone(i)
	return nil
}
func (m *Memory) UpdateIncidentAction(id string, a bool, u *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.incidents[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	v.Acknowledged = a
	v.SilencedUntil = u
	m.incidents[id] = clone(v)
	return nil
}
func (m *Memory) AddSnapshot(s model.Snapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.samples = append(m.samples, core.Sample(s))
	for _, n := range s.Nodes {
		m.nodes[n.Address] = append(m.nodes[n.Address], clone(n))
	}
	return nil
}
func (m *Memory) History(since time.Time, limit int) ([]model.Sample, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []model.Sample{}
	for _, v := range m.samples {
		if !v.Time.Before(since) {
			out = append(out, v)
		}
	}
	return core.Downsample(clone(out), limit), nil
}
func (m *Memory) NodeHistory(id string, since time.Time, limit int) ([]model.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []model.Node{}
	for _, v := range m.nodes[id] {
		if !v.LastUpdatedAt.Before(since) {
			out = append(out, v)
		}
	}
	return core.Downsample(clone(out), limit), nil
}
func (m *Memory) SaveTopology(s model.Snapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.top = clone(s)
	return nil
}
func (m *Memory) LoadTopology() (model.Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.top.GeneratedAt.IsZero() {
		return model.Snapshot{}, fmt.Errorf("no topology")
	}
	return clone(m.top), nil
}
func (m *Memory) Prune(int) error { return nil }

type Source struct {
	mu    sync.Mutex
	Nodes []model.Node
	Err   error
	Wait  <-chan struct{}
	Calls int
}

func (s *Source) Set(nodes []model.Node, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Nodes = clone(nodes)
	s.Err = err
}
func (s *Source) Discover(ctx context.Context) ([]model.Node, error) {
	s.mu.Lock()
	s.Calls++
	nodes, e, w := clone(s.Nodes), s.Err, s.Wait
	s.mu.Unlock()
	if w != nil {
		select {
		case <-w:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nodes, e
}
func (s *Source) Query(ctx context.Context, q string, l int) (model.QueryResult, error) {
	return model.QueryResult{Columns: []string{"value"}, Rows: []map[string]any{{"value": "fixture"}}, Count: 1}, nil
}
func (s *Source) Close() {}
func Good(now time.Time, ms int) model.Check {
	return model.Check{OK: true, State: "ok", Kind: "cql_options", Latency: time.Duration(ms) * time.Millisecond, CheckedAt: now}
}
func Snapshot(now time.Time) model.Snapshot {
	names := []string{"Demo A", "Demo B", "demo-c"}
	ips := []string{"100.64.0.1", "100.64.0.2", "100.64.0.3"}
	s := model.Snapshot{Version: model.Version, SchemaVersion: 2, Demo: true, ClusterName: "Example Cluster", LocalNode: ips[0], LocalAddress: ips[0], LocalNodeName: "Demo A", GeneratedAt: now, StaleAfterSeconds: 30, Discovery: Good(now, 2), Warnings: []string{}, Incidents: []model.Incident{}}
	s.Discovery.Kind = "query"
	s.Discovery.Message = "system.cluster_status (preview fixture)"
	for i, name := range names {
		n := model.Node{ID: ips[i], Address: ips[i], Name: name, DC: "datacenter1", Rack: "rack1", Status: "NORMAL", Up: true, MetadataKnown: true, MetadataAt: now, LoadKnown: true, LoadBytes: int64(520+i*90) << 20, HostID: fmt.Sprintf("demo-host-%d", i), Shards: []int{4, 4, 2}[i], Version: "2026.1.13", CQL: Good(now, []int{1, 23, 22}[i]), Internode: Good(now, []int{1, 22, 22}[i]), WatchAPI: Good(now, 2), REST: Good(now, 1), LocalService: Good(now, 1), Metrics: Good(now, 1), LastUpdatedAt: now, AgentVersion: model.Version, AgentAt: now, Labels: map[string]string{"memory": []string{"6G", "4G", "3G"}[i]}, MetricValues: map[string]float64{"scylla_compaction_manager_pending_compactions": 0, "scylla_compaction_manager_compactions": 1, "scylla_compaction_manager_backlog": 0.03, "scylla_storage_proxy_coordinator_read_timeouts": 0}}
		n.Internode.Kind = "tcp"
		n.LocalService.Kind = "systemd"
		n.LocalService.Message = "active"
		s.Nodes = append(s.Nodes, n)
	}
	for i, a := range ips {
		for j, b := range ips {
			if i != j {
				s.Edges = append(s.Edges, model.Edge{From: a, To: b, CQL: Good(now, 22+(i+j)%3), Internode: Good(now, 21+(i+j)%3), WatchAPI: Good(now, 22), ObservedAt: now})
			}
		}
	}
	core.Summarize(&s, 3, 30*time.Second)
	return s
}
