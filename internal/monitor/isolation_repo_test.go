//go:build memory.counters

package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"scyllascope/internal/model"
	"scyllascope/internal/testkit"
	"strings"
	"sync"
	"time"
)

// Every repository operation retains at most the original 500 identities and
// one topology. Events/history operations count work but never append records.
type isolationRepository struct {
	mu                                                sync.Mutex
	keys                                              []string
	incidents                                         map[string]model.Incident
	topology                                          model.Snapshot
	eventWrites, sampleWrites, topologyWrites, prunes int64
}

func isolationFixture(now time.Time) (model.Snapshot, *isolationRepository) {
	s := testkit.Snapshot(now)
	s.ClusterName = "synthetic-isolation"
	s.LocalNode = "fixed-0"
	s.LocalAddress = "127.0.0.1"
	s.LocalNodeName = "fixed-0"
	for i := range s.Nodes {
		s.Nodes[i].ID = fmt.Sprintf("fixed-%d", i)
		s.Nodes[i].Name = s.Nodes[i].ID
		s.Nodes[i].Address = fmt.Sprintf("127.0.0.%d", i+1)
		s.Nodes[i].State = model.Unknown
		s.Nodes[i].Reasons = nil
	}
	s.Edges = nil
	s.Events = nil
	s.Alerts = nil
	s.Warnings = nil
	r := &isolationRepository{incidents: map[string]model.Incident{}}
	for i := 0; i < 500; i++ {
		v := model.Incident{ID: fmt.Sprintf("fixed-%04d", i), Observer: "isolation-observer", Node: "fixed-unknown", Kind: "node_health/cql", Severity: "warning", Message: strings.Repeat("x", 256), OpenedAt: now.Add(-time.Hour), UpdatedAt: now, NotificationPending: true}
		r.keys = append(r.keys, v.ID)
		r.incidents[v.ID] = v
		s.Incidents = append(s.Incidents, v)
	}
	encoded, _ := json.Marshal(s)
	if len(encoded) < 384<<10 {
		s.Warnings = []string{strings.Repeat("p", (384<<10)-len(encoded))}
	}
	r.topology = topologyMetadata(s)
	return s, r
}
func (r *isolationRepository) Add(model.Event) error {
	r.mu.Lock()
	r.eventWrites++
	r.mu.Unlock()
	return nil
}
func (r *isolationRepository) Recent(int) ([]model.Event, error) { return []model.Event{}, nil }
func (r *isolationRepository) Incidents(all bool) ([]model.Incident, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v := make([]model.Incident, 0, 500)
	for _, k := range r.keys {
		x := r.incidents[k]
		if all || x.ResolvedAt == nil {
			v = append(v, x)
		}
	}
	return v, nil
}
func (r *isolationRepository) WorkIncidents(after string, limit int) ([]model.Incident, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v := make([]model.Incident, 0, min(limit, 500))
	for _, k := range r.keys {
		if k > after {
			v = append(v, r.incidents[k])
			if len(v) >= limit {
				break
			}
		}
	}
	return v, nil
}
func (r *isolationRepository) Incident(id string) (model.Incident, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.incidents[id]
	if !ok {
		return v, fmt.Errorf("unknown synthetic incident")
	}
	return v, nil
}
func (r *isolationRepository) PutIncident(v model.Incident) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.incidents[v.ID]; !ok {
		return fmt.Errorf("isolation repository forbids identity growth")
	}
	r.incidents[v.ID] = v
	return nil
}
func (r *isolationRepository) UpdateIncidentAction(string, bool, *time.Time) error {
	return fmt.Errorf("not used in isolation")
}
func (r *isolationRepository) AddSnapshot(model.Snapshot) error {
	r.mu.Lock()
	r.sampleWrites++
	r.mu.Unlock()
	return nil
}
func (r *isolationRepository) History(time.Time, int) ([]model.Sample, error) {
	return []model.Sample{}, nil
}
func (r *isolationRepository) NodeHistory(string, time.Time, int) ([]model.Node, error) {
	return []model.Node{}, nil
}
func (r *isolationRepository) SaveTopology(s model.Snapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.topology = topologyMetadata(s)
	r.topologyWrites++
	return nil
}
func (r *isolationRepository) LoadTopology() (model.Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return model.CloneSnapshot(r.topology), nil
}
func (r *isolationRepository) Prune(int) error { r.mu.Lock(); r.prunes++; r.mu.Unlock(); return nil }
func (r *isolationRepository) counts() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return map[string]any{"repository_incidents": len(r.incidents), "repository_topology_rows": 1, "repository_topology_nodes": len(r.topology.Nodes), "repository_topology_incidents": len(r.topology.Incidents), "repository_history_rows": 0, "repository_event_rows": 0, "discarded_event_writes": r.eventWrites, "discarded_sample_writes": r.sampleWrites, "topology_writes": r.topologyWrites, "prune_calls": r.prunes}
}

type isolationSource struct{ nodes []model.Node }

func (s *isolationSource) Discover(context.Context) ([]model.Node, error) {
	v := make([]model.Node, len(s.nodes))
	copy(v, s.nodes)
	return v, nil
}
func (*isolationSource) Query(context.Context, string, int) (model.QueryResult, error) {
	return model.QueryResult{}, fmt.Errorf("unused")
}
func (*isolationSource) Close() {}

type isolationAlertControl struct{}

func (isolationAlertControl) Test(context.Context) error            { return fmt.Errorf("unused") }
func (isolationAlertControl) Action(string, bool, *time.Time) error { return fmt.Errorf("unused") }
