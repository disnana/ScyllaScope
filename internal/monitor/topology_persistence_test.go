package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"scyllascope/internal/model"
	"scyllascope/internal/settings"
	"scyllascope/internal/store"
	"scyllascope/internal/testkit"
	"strings"
	"testing"
	"time"
)

func topologyFixture() model.Snapshot {
	now := time.Date(2026, 1, 2, 3, 4, 5, 123456789, time.UTC)
	s := testkit.Snapshot(now)
	s.LocalAddress = "127.0.0.1"
	for i := range s.Nodes {
		s.Nodes[i].Address = fmt.Sprintf("127.0.0.%d", i+1)
	}
	return s
}
func topologyConfig(cluster string) *settings.Config {
	c := &settings.Config{ClusterName: cluster, NodeID: "observer", NodeName: "synthetic", LocalAddress: "127.0.0.1", Listen: "127.0.0.1:8091"}
	settings.Defaults(c)
	c.Monitoring.Timeout = 20 * time.Millisecond
	c.Monitoring.MaxNodes = 4
	c.Monitoring.Concurrency = 2
	c.Scylla.CQLPort = 1
	c.Scylla.InternodePort = 1
	c.Scylla.RESTURL = "http://127.0.0.1:1"
	c.Scylla.PrometheusPort = 1
	c.Scylla.SystemdService = "disabled"
	c.Peers = []settings.Peer{{Address: "127.0.0.9", Name: "explicit"}}
	return c
}

func TestTopologyMetadataPayloadIndependentOfLiveData(t *testing.T) {
	base := topologyFixture()
	compact := topologyMetadata(base)
	expected, err := json.Marshal(compact)
	if err != nil {
		t.Fatal(err)
	}
	noisy := model.CloneSnapshot(base)
	for i := 0; i < 500; i++ {
		noisy.Incidents = append(noisy.Incidents, model.Incident{ID: fmt.Sprint(i), Message: strings.Repeat("i", 512)})
		noisy.Events = append(noisy.Events, model.Event{Message: strings.Repeat("e", 512)})
		noisy.Edges = append(noisy.Edges, model.Edge{From: fmt.Sprint(i), To: strings.Repeat("edge", 128)})
		noisy.Nodes[0].MetricValues[fmt.Sprint(i)] = float64(i)
	}
	noisy.Warnings = []string{strings.Repeat("w", 65536)}
	noisy.Nodes[0].Labels = map[string]string{"large": strings.Repeat("label", 20000)}
	noisy.Nodes[0].ObservedBy = map[string]model.Check{"stale": {Message: strings.Repeat("observation", 5000)}}
	noisy.Nodes[0].Reasons = []string{strings.Repeat("reason", 10000)}
	actual, err := json.Marshal(topologyMetadata(noisy))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatal("live payload leaked into restart cache")
	}
	for i, n := range base.Nodes {
		if !reflect.DeepEqual(compact.Nodes[i], metadataOnly(n)) {
			t.Fatal("metadata/order changed", i)
		}
		if !compact.Nodes[i].MetadataAt.Equal(n.MetadataAt) {
			t.Fatal("metadata timestamp extended")
		}
	}
	if compact.ClusterName != base.ClusterName || compact.SchemaVersion != base.SchemaVersion || !compact.GeneratedAt.Equal(base.GeneratedAt) {
		t.Fatal("cache identity/time changed")
	}
	full, _ := json.Marshal(noisy)
	t.Logf("synthetic full snapshot=%d B; compact=%d B; compact unchanged after 500 incidents/events/edges and expanded metrics", len(full), len(actual))
	compact.Nodes[0].Name = "changed copy"
	if base.Nodes[0].Name == compact.Nodes[0].Name {
		t.Fatal("metadata slice aliases live snapshot")
	}
}

func TestLegacyAndCompactTopologyRestartEquivalent(t *testing.T) {
	fixture := topologyFixture()
	var previousFallback []model.Node
	for _, format := range []string{"legacy-full", "compact"} {
		t.Run(format, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "restart.db")
			st, err := store.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			saved := fixture
			if format == "compact" {
				saved = topologyMetadata(fixture)
			}
			if err = st.SaveTopology(saved); err != nil {
				t.Fatal(err)
			}
			if err = st.Close(); err != nil {
				t.Fatal(err)
			}
			st, err = store.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			c := topologyConfig(fixture.ClusterName)
			m := New(c, st, &testkit.Source{Err: errors.New("synthetic discovery outage")}, nil)
			defer m.http.CloseIdleConnections()
			m.lastRuntime = time.Now().Add(time.Hour)
			for _, n := range fixture.Nodes {
				cached, ok := m.known[n.Address]
				if !ok || !reflect.DeepEqual(cached, metadataOnly(n)) {
					t.Fatal("restart metadata changed", n.Address, cached)
				}
				if cached.CQL.Known() || cached.Internode.Known() || cached.MetricValues != nil {
					t.Fatal("cached live evidence restored")
				}
			}
			fallback := m.fallbackNodes()
			if len(fallback) != 4 || fallback[0].Address != c.LocalAddress || fallback[1].Address != "127.0.0.9" {
				t.Fatal("fallback priority changed", fallback)
			}
			if previousFallback != nil && !reflect.DeepEqual(previousFallback, fallback) {
				t.Fatal("old and compact restart differ")
			}
			previousFallback = fallback
			if !m.RefreshNow(context.Background()) {
				t.Fatal("refresh not run")
			}
			snap := m.Snapshot()
			if snap.Discovery.OK || snap.Health == model.Healthy || len(snap.Nodes) != 4 {
				t.Fatal("outage/fallback semantics changed")
			}
			for _, n := range snap.Nodes {
				if n.MetadataKnown || n.LoadKnown {
					t.Fatal("cached metadata treated as fresh")
				}
			}
			other := New(topologyConfig("different-cluster"), st, &testkit.Source{}, nil)
			defer other.http.CloseIdleConnections()
			if len(other.known) != 0 {
				t.Fatal("other cluster topology loaded")
			}
		})
	}
}

func TestCollectorPersistsOnlyTopologyMetadata(t *testing.T) {
	fixture := topologyFixture()
	c := topologyConfig(fixture.ClusterName)
	c.Peers = nil
	st, err := store.Open(filepath.Join(t.TempDir(), "collector.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Now()
	for i := 0; i < 500; i++ {
		if err = st.PutIncident(model.Incident{ID: fmt.Sprintf("incident-%04d", i), Observer: c.NodeID, Kind: "node_health", Node: "retired", Message: strings.Repeat("incident", 100), OpenedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 20; i++ {
		if err = st.Add(model.Event{Time: now, Type: "synthetic", Message: strings.Repeat("event", 100)}); err != nil {
			t.Fatal(err)
		}
	}
	metricServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "scylla_storage_proxy_coordinator_read_timeouts 0\n")
	}))
	defer metricServer.Close()
	c.Scylla.RESTURL = metricServer.URL
	// Reuse the fixture server for the real metrics HTTP path.
	var port int
	if _, err = fmt.Sscanf(strings.TrimPrefix(metricServer.URL, "http://127.0.0.1:"), "%d", &port); err != nil {
		t.Fatal(err)
	}
	c.Scylla.PrometheusAddress = "127.0.0.1"
	c.Scylla.PrometheusPort = port
	m := New(c, st, &testkit.Source{Nodes: fixture.Nodes}, nil)
	defer m.http.CloseIdleConnections()
	m.lastRuntime = time.Now().Add(time.Hour)
	if !m.RefreshNow(context.Background()) {
		t.Fatal("refresh not run")
	}
	live := m.Snapshot()
	if len(live.Incidents) != 500 || len(live.Events) != 15 || len(live.Edges) == 0 {
		t.Fatal("live snapshot lost data")
	}
	foundMetrics := false
	for _, n := range live.Nodes {
		if len(n.MetricValues) > 0 {
			foundMetrics = true
		}
	}
	if !foundMetrics {
		t.Fatal("metrics fixture not exercised")
	}
	persisted, err := st.LoadTopology()
	if err != nil {
		t.Fatal(err)
	}
	want := topologyMetadata(live)
	gotJSON, _ := json.Marshal(persisted)
	wantJSON, _ := json.Marshal(want)
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatal("collector did not persist metadata-only restart cache")
	}
	if len(persisted.Incidents) != 0 || len(persisted.Events) != 0 || len(persisted.Edges) != 0 {
		t.Fatal("live graph stored twice")
	}
	full, _ := json.Marshal(live)
	t.Logf("collector full snapshot=%d B; persisted restart cache=%d B", len(full), len(gotJSON))
}
