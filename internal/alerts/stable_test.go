package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"scyllascope/internal/model"
	"scyllascope/internal/store"
	"scyllascope/internal/testkit"
	"sync"
	"sync/atomic"
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

func TestLegacyOpeningEnabledBeforeRecoveryRemainsPending(t *testing.T) {
	e, m := fixture()
	now := time.Now()
	s := bad(now)
	v := model.Incident{ID: "legacy", Observer: e.cfg.NodeID, Node: s.Nodes[2].Address, Kind: "node_health", OpenedAt: now.Add(-time.Hour), UpdatedAt: now}
	m.PutIncident(v)
	e.cfg.Notifications.GenericWebhook = "http://127.0.0.1:1"
	e.Evaluate(context.Background(), s)
	v, _ = m.Incident(v.ID)
	if !v.NotificationPending {
		t.Fatal("legacy opening not persisted as pending")
	}
	e.deliver(context.Background(), <-e.queue) // receiver is unavailable
	for i := 1; i <= e.cfg.Monitoring.RecoveryWindow; i++ {
		e.Evaluate(context.Background(), testkit.Snapshot(now.Add(time.Duration(i)*time.Minute)))
	}
	v, _ = m.Incident(v.ID)
	if v.ResolvedAt == nil || !v.NotificationPending {
		t.Fatal("failed opening disappeared on recovery", v)
	}
	e = New(e.cfg, m)
	e.Evaluate(context.Background(), testkit.Snapshot(now.Add(time.Hour)))
	if len(e.queue) != 1 {
		t.Fatal("failed opening not retried after restart")
	}
}
func TestUnknownEvidenceKeepsAlertmanagerHeartbeat(t *testing.T) {
	e, m := fixture()
	now := time.Now()
	s := testkit.Snapshot(now)
	s.Nodes[2].State = model.Unknown
	e.cfg.Notifications.AlertmanagerURLs = []string{"http://127.0.0.1:1"}
	v := model.Incident{ID: "unknown", Observer: e.cfg.NodeID, Node: s.Nodes[2].Address, Kind: "node_health/cql", OpenedAt: now.Add(-time.Hour), UpdatedAt: now, LastNotifiedAt: &now}
	m.PutIncident(v)
	s.Nodes[2].CQL = model.Check{}
	e.Evaluate(context.Background(), s)
	if len(e.queue) != 1 {
		t.Fatal("unresolved alert heartbeat suppressed by unknown evidence")
	}
	if j := <-e.queue; j.recovery {
		t.Fatal("unknown became recovery")
	}
	v, _ = m.Incident(v.ID)
	if v.ResolvedAt != nil {
		t.Fatal("unknown resolved incident")
	}
}
func TestDistinctEpisodesHaveDistinctAlertmanagerIdentities(t *testing.T) {
	v := model.Incident{ID: "old", Observer: "observer", Node: "target", Kind: "node_health/cql"}
	old := alertLabels("cluster", v)
	v.ID = "new"
	current := alertLabels("cluster", v)
	if old["incident_id"] == current["incident_id"] {
		t.Fatal("late recovery may overwrite new firing episode")
	}
	for _, key := range []string{"cluster", "node", "kind"} {
		if old[key] != current[key] {
			t.Fatal("episode changed notification grouping")
		}
	}
}

func TestDuplicateLegacyEpisodesDoNotMultiplyRecoverySamples(t *testing.T) {
	e, m := fixture()
	now := time.Now()
	s := testkit.Snapshot(now)
	for i := 0; i < 3; i++ {
		m.PutIncident(model.Incident{ID: fmt.Sprintf("dup-%d", i), Observer: e.cfg.NodeID, Node: s.Nodes[2].Address, Kind: "node_health", OpenedAt: now.Add(-time.Hour), UpdatedAt: now})
	}
	e.Evaluate(context.Background(), s)
	all, _ := m.Incidents(false)
	if len(all) != 3 {
		t.Fatal("one healthy sample multiplied by duplicate historical rows", len(all))
	}
	for i := 1; i < e.cfg.Monitoring.RecoveryWindow; i++ {
		e.Evaluate(context.Background(), testkit.Snapshot(now.Add(time.Duration(i)*time.Second)))
	}
	all, _ = m.Incidents(false)
	if len(all) != 0 {
		t.Fatal("duplicates did not all resolve after actual recovery window", len(all))
	}
}

func TestBoundedWorkersProgressPastSlowHAEndpointAndWaitForShutdown(t *testing.T) {
	e, m := fixture()
	e.cfg.Monitoring.RecoveryWindow = 1
	var delivered atomic.Int64
	var identities sync.Map
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.Copy(io.Discard, r.Body); <-r.Context().Done() }))
	defer slow.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload []struct {
			Labels map[string]string `json:"labels"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		for _, alert := range payload {
			if _, loaded := identities.LoadOrStore(alert.Labels["incident_id"], true); !loaded {
				delivered.Add(1)
			}
		}
		w.WriteHeader(200)
	}))
	defer good.Close()
	e.cfg.Notifications.AlertmanagerURLs = []string{slow.URL, good.URL}
	e.http.Timeout = 20 * time.Millisecond
	now := time.Now()
	for i := 0; i < 520; i++ {
		m.PutIncident(model.Incident{ID: fmt.Sprintf("work-%04d", i), Observer: e.cfg.NodeID, Node: "retired", Kind: "node_health/cql", OpenedAt: now, UpdatedAt: now, NotificationPending: true})
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(20 * time.Second)
	cycle := 0
	for delivered.Load() < 520 && time.Now().Before(deadline) {
		cycle++
		e.Evaluate(ctx, testkit.Snapshot(now.Add(time.Duration(cycle)*time.Second)))
		stats := e.Stats()
		if stats.Queued > 128 || stats.Inflight > 128+deliveryWorkers || stats.Attempts > 2048 {
			t.Fatal("unbounded notification bookkeeping", stats)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if delivered.Load() < 520 {
		t.Fatal("slow HA receiver starved healthy receiver", delivered.Load())
	}
	// Refill and cancel during requests. Run must wait for its workers; it must
	// not leave goroutines performing delivery after it reports completion.
	for i := 0; i < 128; i++ {
		e.mu.Lock()
		v, _ := m.Incident(fmt.Sprintf("work-%04d", i))
		e.enqueue(v, false, now.Add(time.Hour))
		e.mu.Unlock()
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("delivery workers did not shut down")
	}
}

func TestSparseRetryBookkeepingExpiresWithoutReachingHardCap(t *testing.T) {
	e, _ := fixture()
	now := time.Now()
	retry := e.cfg.Notifications.RetryInterval
	e.lastAttempt["expired"] = now.Add(-retry)
	e.lastAttempt["young"] = now.Add(-retry / 2)
	e.lastAttempt["inflight"] = now.Add(-2 * retry)
	e.inflight["inflight"] = true
	e.Evaluate(context.Background(), testkit.Snapshot(now))
	if _, ok := e.lastAttempt["expired"]; ok {
		t.Fatal("sparse expired attempt retained")
	}
	if _, ok := e.lastAttempt["young"]; !ok {
		t.Fatal("young throttle removed")
	}
	if _, ok := e.lastAttempt["inflight"]; !ok {
		t.Fatal("inflight throttle removed")
	}
	if len(e.lastAttempt) != 2 {
		t.Fatal(e.Stats())
	}
}

func TestSparseRetryCleanupPreservesIntervalBoundary(t *testing.T) {
	e, _ := fixture()
	e.cfg.Notifications.GenericWebhook = "http://127.0.0.1:1"
	now := time.Now()
	retry := e.cfg.Notifications.RetryInterval
	v := model.Incident{ID: "retry-boundary"}
	e.enqueue(v, false, now)
	if len(e.queue) != 1 {
		t.Fatal("initial attempt was not queued")
	}
	<-e.queue
	delete(e.inflight, v.ID) // simulate a completed unsuccessful attempt
	e.Evaluate(context.Background(), testkit.Snapshot(now.Add(retry-time.Nanosecond)))
	e.enqueue(v, false, now.Add(retry-time.Nanosecond))
	if len(e.queue) != 0 {
		t.Fatal("attempt retried before interval elapsed")
	}
	e.Evaluate(context.Background(), testkit.Snapshot(now.Add(retry)))
	if _, ok := e.lastAttempt[v.ID]; ok {
		t.Fatal("expired throttle retained at interval boundary")
	}
	e.enqueue(v, false, now.Add(retry))
	if len(e.queue) != 1 {
		t.Fatal("attempt not eligible at interval boundary")
	}
}
