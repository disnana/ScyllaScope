package core_test

import (
	"scyllascope/internal/core"
	"scyllascope/internal/model"
	"scyllascope/internal/testkit"
	"testing"
	"time"
)

func TestHealthyRequiresFreshMetadata(t *testing.T) {
	now := time.Now()
	n := testkit.Snapshot(now).Nodes[0]
	s, _ := core.NodeHealth(n, now, 30*time.Second)
	if s != model.Healthy {
		t.Fatal(s)
	}
	n.MetadataKnown = false
	s, _ = core.NodeHealth(n, now, 30*time.Second)
	if s != model.Unknown {
		t.Fatalf("TCP alone must not mean healthy: %s", s)
	}
}
func TestStaleChecksAndOldServiceFailure(t *testing.T) {
	now := time.Now()
	n := testkit.Snapshot(now).Nodes[0]
	n.LocalService = model.Check{OK: false, State: "error", CheckedAt: now.Add(-time.Minute)}
	s, _ := core.NodeHealth(n, now, 30*time.Second)
	if s != model.Healthy {
		t.Fatal(s)
	}
	n.CQL.CheckedAt = now.Add(-time.Minute)
	s, _ = core.NodeHealth(n, now, 30*time.Second)
	if s != model.Unknown {
		t.Fatal(s)
	}
}
func TestFutureMetadataUnknown(t *testing.T) {
	now := time.Now()
	n := testkit.Snapshot(now).Nodes[0]
	n.MetadataAt = now.Add(time.Hour)
	s, _ := core.NodeHealth(n, now, 30*time.Second)
	if s != model.Unknown {
		t.Fatal(s)
	}
}
func TestDiscoveryFailureNotGreen(t *testing.T) {
	s := testkit.Snapshot(time.Now())
	s.Discovery.OK = false
	core.Summarize(&s, 3, 30*time.Second)
	if s.Health != model.Unknown {
		t.Fatal(s.Health)
	}
}
func TestMissingNodeNotVanishing(t *testing.T) {
	s := testkit.Snapshot(time.Now())
	s.Nodes = s.Nodes[:2]
	s.Edges = nil
	core.Summarize(&s, 3, 30*time.Second)
	if s.Coverage.ExpectedNodes != 3 || s.Health != model.Warning {
		t.Fatalf("%+v", s)
	}
}
func TestAgentCoverageSeparateFromNodes(t *testing.T) {
	s := testkit.Snapshot(time.Now())
	s.Nodes[1].WatchAPI = model.UnknownCheck("missing")
	s.Nodes[2].WatchAPI = model.UnknownCheck("missing")
	s.Edges = s.Edges[:2]
	core.Summarize(&s, 3, 30*time.Second)
	if s.Coverage.AgentsReady != 1 || s.Coverage.DiscoveredNodes != 3 || s.Coverage.ObservedDirections != 2 {
		t.Fatal(s.Coverage)
	}
}
func TestAsymmetricRequiresFreshReverse(t *testing.T) {
	s := testkit.Snapshot(time.Now())
	s.Edges[0].Internode.OK = false
	core.Summarize(&s, 3, 30*time.Second)
	if !s.Edges[0].Asymmetric {
		t.Fatal("asymmetry missing")
	}
	for i := range s.Edges {
		if s.Edges[i].From == s.Edges[0].To && s.Edges[i].To == s.Edges[0].From {
			s.Edges[i].ObservedAt = s.GeneratedAt.Add(-time.Minute)
		}
	}
	core.Summarize(&s, 3, 30*time.Second)
	if s.Edges[0].Asymmetric {
		t.Fatal("stale edge must not prove asymmetry")
	}
}
func TestSampleNullUnknownNoZeroAndNoFloor(t *testing.T) {
	s := testkit.Snapshot(time.Now())
	for i := range s.Nodes {
		s.Nodes[i].LoadKnown = false
		s.Nodes[i].CQL.Latency = 100 * time.Microsecond
	}
	a := core.Sample(s)
	if a.TotalLoadBytes != nil || a.AverageLatencyMS == nil || *a.AverageLatencyMS != 0.1 {
		t.Fatal(a)
	}
	for i := range s.Nodes {
		s.Nodes[i].CQL = model.Check{}
	}
	if core.Sample(s).AverageLatencyMS != nil {
		t.Fatal("unknown != zero")
	}
}
func TestDownsampleIncludesNewestAndOldest(t *testing.T) {
	v := make([]int, 10000)
	for i := range v {
		v[i] = i
	}
	r := core.Downsample(v, 480)
	if len(r) != 480 || r[0] != 0 || r[479] != 9999 {
		t.Fatal(r)
	}
}
func TestClusterRowNumericAndTextLoad(t *testing.T) {
	for _, load := range []any{int64(1024), "1 KiB"} {
		n, e := core.ClusterRow(map[string]any{"peer": "100.64.0.1", "dc": "datacenter1", "status": "NORMAL", "up": true, "load": load})
		if e != nil || !n.LoadKnown || n.LoadBytes != 1024 {
			t.Fatalf("%+v %v", n, e)
		}
	}
}
func TestClusterRowInvalidRejected(t *testing.T) {
	_, e := core.ClusterRow(map[string]any{"peer": "bad", "status": "NORMAL", "up": true})
	if e == nil {
		t.Fatal("bad address accepted")
	}
	n, e := core.ClusterRow(map[string]any{"peer": "100.64.0.1", "status": "NORMAL", "up": true})
	if e != nil || n.LoadKnown {
		t.Fatalf("%+v %v", n, e)
	}
}
func TestQueryAllowlistAndLimits(t *testing.T) {
	for _, q := range []string{"SELECT * FROM system.cluster_status;", "SELECT peer, up FROM system.cluster_status LIMIT 10"} {
		if _, e := core.ValidateQuery(q, false); e != nil {
			t.Fatal(q, e)
		}
	}
	for _, q := range []string{"DELETE FROM x.y", "SELECT * FROM system.local; DROP TABLE x.y", "SELECT count(*) FROM system.local", "SELECT * FROM x.y", "SELECT * FROM system_auth.roles", "SELECT * FROM system.local ALLOW FILTERING", "SELECT * FROM system.local -- comment"} {
		if _, e := core.ValidateQuery(q, false); e == nil {
			t.Fatal("unsafe query accepted:", q)
		}
	}
}
func TestPeerValidationRejectsForeignStaleAndIdentity(t *testing.T) {
	now := time.Now()
	s := testkit.Snapshot(now)
	if e := core.ValidatePeer(s, "Example Cluster", s.LocalAddress, now, 30*time.Second); e != nil {
		t.Fatal(e)
	}
	for _, mutation := range []func(*model.Snapshot){func(s *model.Snapshot) { s.ClusterName = "other" }, func(s *model.Snapshot) { s.GeneratedAt = now.Add(-time.Hour) }, func(s *model.Snapshot) { s.GeneratedAt = now.Add(time.Hour) }, func(s *model.Snapshot) { s.LocalNode = "forged" }} {
		v := testkit.Snapshot(now)
		mutation(&v)
		if e := core.ValidatePeer(v, "Example Cluster", s.LocalAddress, now, 30*time.Second); e == nil {
			t.Fatal("bad peer accepted")
		}
	}
}
func TestPeerDoesNotReGossipOrDuplicateEdges(t *testing.T) {
	now := time.Now()
	s := testkit.Snapshot(now)
	p := testkit.Snapshot(now)
	p.LocalNode = p.Nodes[1].ID
	p.LocalAddress = p.Nodes[1].Address
	p.Version = "0.5.0"
	_, edges := core.MergePeer(s.Nodes, nil, p, p.LocalAddress, now, 30*time.Second)
	if len(edges) != 2 {
		t.Fatal(len(edges))
	}
	_, edges = core.MergePeer(s.Nodes, edges, p, p.LocalAddress, now, 30*time.Second)
	if len(edges) != 2 {
		t.Fatal("duplicate edges", len(edges))
	}
	for _, e := range edges {
		if e.From != p.LocalNode {
			t.Fatal("relayed edge")
		}
	}
}
