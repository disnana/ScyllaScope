package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"scyllascope/internal/model"
	"scyllascope/internal/store"
	"scyllascope/internal/testkit"
	"testing"
	"time"
)

func TestDeliveryBeyondUIAndResolvedBeforeFirstSuccess(t *testing.T) {
	e, _ := fixture()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	e.store = st
	now := time.Now()
	old := now.Add(-time.Hour)
	for i := 0; i < 700; i++ {
		if err = st.PutIncident(model.Incident{ID: fmt.Sprintf("%06d", i), Observer: e.cfg.NodeID, Kind: "node_health", Node: "retired", OpenedAt: old, UpdatedAt: old, ResolvedAt: &old, NotificationPending: true}); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }))
	defer srv.Close()
	e.cfg.Notifications.GenericWebhook = srv.URL
	target, _ := st.Incident("000000")
	e.deliver(context.Background(), job{target, false})
	e = New(e.cfg, st) // restart after accepting the opening
	e.Evaluate(context.Background(), testkit.Snapshot(now))
	for len(e.queue) > 0 {
		e.deliver(context.Background(), <-e.queue)
	}
	target, _ = st.Incident(target.ID)
	if target.LastNotifiedAt == nil || target.RecoveryPending || target.RecoveryNotifiedAt == nil || calls < 2 {
		t.Fatal(target, calls)
	}
}
func TestRetiredIdentitiesDoNotRetainCounters(t *testing.T) {
	e, m := fixture()
	now := time.Now()
	for i := 0; i < 1000; i++ {
		m.PutIncident(model.Incident{ID: fmt.Sprint(i), Observer: e.cfg.NodeID, Kind: "node_health", Node: fmt.Sprint(i), OpenedAt: now, UpdatedAt: now})
	}
	e.Evaluate(context.Background(), testkit.Snapshot(now))
	if len(e.tracks) != 0 {
		t.Fatal("retired counters retained", len(e.tracks))
	}
}
func TestAlertmanagerHeartbeatFailoverAndConditions(t *testing.T) {
	e, m := fixture()
	var received []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/alerts" {
			t.Error(r.URL)
		}
		var a []map[string]any
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
			t.Error(err)
		}
		received = append(received, a...)
		w.WriteHeader(200)
	}))
	defer srv.Close()
	e.cfg.Notifications.AlertmanagerURLs = []string{"http://127.0.0.1:1", srv.URL}
	e.cfg.Notifications.SenderNode = "offline-sender"
	now := time.Now()
	v := model.Incident{ID: "persisted", Observer: e.cfg.NodeID, Kind: "node_health/cql", Node: "target", OpenedAt: now, UpdatedAt: now, LastNotifiedAt: &now}
	m.PutIncident(v)
	e.enqueue(v, false, now.Add(time.Hour))
	if len(e.queue) != 1 {
		t.Fatal("heartbeat suppressed")
	}
	e.deliver(context.Background(), <-e.queue)
	e = New(e.cfg, m)
	e.enqueue(v, false, now.Add(2*time.Hour))
	e.deliver(context.Background(), <-e.queue)
	if len(received) != 2 {
		t.Fatal(len(received))
	}
	a := alertLabels("cluster", v)
	v.Observer = "other"
	b := alertLabels("cluster", v)
	if a["observer"] == b["observer"] || a["kind"] != b["kind"] {
		t.Fatal(a, b)
	}
	v.Kind = "node_health/internode"
	if alertLabels("cluster", v)["kind"] == a["kind"] {
		t.Fatal("distinct failure collapsed")
	}
	n := testkit.Snapshot(now).Nodes[0]
	n.CQL.OK = false
	n.Internode.OK = false
	if len(healthConditions(n, now, time.Minute)) != 2 {
		t.Fatal(n)
	}
	n.CQL.OK = true
	if !conditionRecovered(n, "cql", now, time.Minute) || conditionRecovered(n, "internode", now, time.Minute) {
		t.Fatal("separate recovery lost")
	}
}

// A full bounded queue must not indefinitely starve older pending work when
// evaluation cadence equals retry_interval.
func TestPendingQueueRotatesAcrossPages(t *testing.T) {
	e, m := fixture()
	e.cfg.Notifications.GenericWebhook = "http://127.0.0.1:1"
	now := time.Now()
	for i := 0; i < 700; i++ {
		m.PutIncident(model.Incident{ID: fmt.Sprintf("%06d", i), Observer: e.cfg.NodeID, Node: "retired", Kind: "node_health", OpenedAt: now, UpdatedAt: now, ResolvedAt: &now, NotificationPending: true})
	}
	seen := map[string]bool{}
	for cycle := 0; cycle < 6; cycle++ {
		e.Evaluate(context.Background(), testkit.Snapshot(now.Add(time.Duration(cycle)*e.cfg.Notifications.RetryInterval)))
		for len(e.queue) > 0 {
			j := <-e.queue
			seen[j.incident.ID] = true
			delete(e.inflight, j.incident.ID)
		}
	}
	if len(seen) != 700 {
		t.Fatal("starved work", len(seen))
	}
}

func TestAlertmanagerOutagePersistsOpeningAndRecoveryAcrossRestart(t *testing.T) {
	e, _ := fixture()
	path := filepath.Join(t.TempDir(), "pending.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	available := false
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if !available {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	e.cfg.Notifications.AlertmanagerURLs = []string{srv.URL}
	e.store = st
	now := time.Now()
	v := model.Incident{ID: "pending", Observer: e.cfg.NodeID, Kind: "node_health/cql", Node: "target", OpenedAt: now.Add(-time.Minute), UpdatedAt: now, ResolvedAt: &now, NotificationPending: true}
	if err = st.PutIncident(v); err != nil {
		t.Fatal(err)
	}
	e.deliver(context.Background(), job{v, false})
	v, _ = st.Incident(v.ID)
	if !v.NotificationPending || v.LastNotifiedAt != nil || v.DeliveryError == "" {
		t.Fatal("failed delivery lost work", v)
	}
	st.Close()
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	e = New(e.cfg, st)
	available = true
	e.Evaluate(context.Background(), testkit.Snapshot(now.Add(time.Second)))
	if len(e.queue) != 1 {
		t.Fatal("opening lost on restart")
	}
	e.deliver(context.Background(), <-e.queue)
	v, _ = st.Incident(v.ID)
	if v.NotificationPending || !v.RecoveryPending {
		t.Fatal("recovery not durable", v)
	}
	e.Evaluate(context.Background(), testkit.Snapshot(now.Add(2*e.cfg.Notifications.RetryInterval)))
	if len(e.queue) != 1 {
		t.Fatal("recovery lost")
	}
	e.deliver(context.Background(), <-e.queue)
	v, _ = st.Incident(v.ID)
	if v.RecoveryPending || v.RecoveryNotifiedAt == nil || calls != 3 {
		t.Fatal(v, calls)
	}
}
