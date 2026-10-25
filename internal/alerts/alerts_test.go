package alerts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"scyllascope/internal/model"
	"scyllascope/internal/settings"
	"scyllascope/internal/testkit"
	"sync/atomic"
	"testing"
	"time"
)

func fixture() (*Engine, *testkit.Memory) {
	c := &settings.Config{ClusterName: "Example Cluster", NodeID: "demo-a", NodeName: "Demo A", LocalAddress: "100.64.0.1"}
	settings.Defaults(c)
	m := testkit.NewMemory()
	return New(c, m), m
}
func bad(now time.Time) model.Snapshot {
	s := testkit.Snapshot(now)
	s.Nodes[2].State = model.Critical
	s.Nodes[2].Reasons = []string{"CQL failed"}
	return s
}
func TestColdStartUnhealthyOpensAfterWindow(t *testing.T) {
	e, m := fixture()
	now := time.Now()
	for i := 0; i < 2; i++ {
		e.Evaluate(context.Background(), bad(now.Add(time.Duration(i)*time.Second)))
	}
	v, _ := m.Incidents(false)
	if len(v) != 0 {
		t.Fatal(v)
	}
	e.Evaluate(context.Background(), bad(now.Add(2*time.Second)))
	v, _ = m.Incidents(false)
	if len(v) != 1 {
		t.Fatal(v)
	}
}
func TestRepeatedSnapshotNotCounted(t *testing.T) {
	e, m := fixture()
	s := bad(time.Now())
	for i := 0; i < 10; i++ {
		e.Evaluate(context.Background(), s)
	}
	v, _ := m.Incidents(false)
	if len(v) != 0 {
		t.Fatal(v)
	}
}
func TestUnknownDoesNotResolveIncident(t *testing.T) {
	e, m := fixture()
	now := time.Now()
	for i := 0; i < 3; i++ {
		e.Evaluate(context.Background(), bad(now.Add(time.Duration(i)*time.Second)))
	}
	for i := 3; i < 8; i++ {
		s := testkit.Snapshot(now.Add(time.Duration(i) * time.Second))
		s.Nodes[2].State = model.Unknown
		e.Evaluate(context.Background(), s)
	}
	v, _ := m.Incidents(false)
	if len(v) != 1 {
		t.Fatal("unknown resolved failure", v)
	}
	for i := 8; i < 10; i++ {
		e.Evaluate(context.Background(), testkit.Snapshot(now.Add(time.Duration(i)*time.Second)))
	}
	v, _ = m.Incidents(false)
	if len(v) != 0 {
		t.Fatal("recovery failed", v)
	}
}
func TestEngineRestartRetainsIncident(t *testing.T) {
	e, m := fixture()
	now := time.Now()
	for i := 0; i < 3; i++ {
		e.Evaluate(context.Background(), bad(now.Add(time.Duration(i)*time.Second)))
	}
	e = New(e.cfg, m)
	e.Evaluate(context.Background(), bad(now.Add(3*time.Second)))
	v, _ := m.Incidents(false)
	events, _ := m.Recent(20)
	if len(v) != 1 || len(events) != 1 {
		t.Fatal(v, events)
	}
}
func TestNotificationFailureRecordedAndNoMention(t *testing.T) {
	e, m := fixture()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer srv.Close()
	e.cfg.Notifications.DiscordWebhook = srv.URL
	now := time.Now()
	for i := 0; i < 3; i++ {
		e.Evaluate(context.Background(), bad(now.Add(time.Duration(i)*time.Second)))
	}
	select {
	case j := <-e.queue:
		e.deliver(context.Background(), j)
	default:
		t.Fatal("no job")
	}
	v, _ := m.Incidents(false)
	if calls.Load() != 1 || len(v) != 1 || v[0].DeliveryError == "" || v[0].LastNotifiedAt != nil {
		t.Fatal(v, calls.Load())
	}
}
func TestAcknowledgedIncidentNotEnqueued(t *testing.T) {
	e, m := fixture()
	e.cfg.Notifications.GenericWebhook = "http://127.0.0.1/fixture"
	now := time.Now()
	for i := 0; i < 3; i++ {
		e.Evaluate(context.Background(), bad(now.Add(time.Duration(i)*time.Second)))
	}
	j := <-e.queue
	delete(e.inflight, j.incident.ID)
	if err := e.Action(j.incident.ID, true, nil); err != nil {
		t.Fatal(err)
	}
	e.Evaluate(context.Background(), bad(now.Add(2*time.Minute)))
	if len(e.queue) != 0 {
		t.Fatal("ack not respected")
	}
	v, _ := m.Incidents(false)
	if !v[0].Acknowledged {
		t.Fatal(v)
	}
}

func TestRecurrenceKeepsHistoricalIncident(t *testing.T) {
	e, m := fixture()
	now := time.Now()
	for i := 0; i < 3; i++ {
		e.Evaluate(context.Background(), bad(now.Add(time.Duration(i)*time.Second)))
	}
	old, _ := m.Incidents(false)
	for i := 3; i < 5; i++ {
		e.Evaluate(context.Background(), testkit.Snapshot(now.Add(time.Duration(i)*time.Second)))
	}
	for i := 5; i < 8; i++ {
		e.Evaluate(context.Background(), bad(now.Add(time.Duration(i)*time.Second)))
	}
	all, _ := m.Incidents(true)
	current, _ := m.Incidents(false)
	if len(all) != 2 || len(current) != 1 || old[0].ID == current[0].ID {
		t.Fatal(all)
	}
}
func TestRecoveryNotificationFailureRetries(t *testing.T) {
	e, m := fixture()
	var code atomic.Int32
	code.Store(200)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(int(code.Load())) }))
	defer srv.Close()
	e.cfg.Notifications.GenericWebhook = srv.URL
	now := time.Now()
	for i := 0; i < 3; i++ {
		e.Evaluate(context.Background(), bad(now.Add(time.Duration(i)*time.Second)))
	}
	e.deliver(context.Background(), <-e.queue)
	code.Store(500)
	for _, offset := range []int{70, 71} {
		e.Evaluate(context.Background(), testkit.Snapshot(now.Add(time.Duration(offset)*time.Second)))
	}
	select {
	case j := <-e.queue:
		if !j.recovery {
			t.Fatal("not recovery")
		}
		e.deliver(context.Background(), j)
	default:
		t.Fatal("recovery not queued")
	}
	v, _ := m.Incidents(true)
	if !v[0].RecoveryPending || v[0].DeliveryError == "" {
		t.Fatal(v)
	}
	code.Store(200)
	e.Evaluate(context.Background(), testkit.Snapshot(now.Add(140*time.Second)))
	select {
	case j := <-e.queue:
		e.deliver(context.Background(), j)
	default:
		t.Fatal("retry missing")
	}
	v, _ = m.Incidents(true)
	if v[0].RecoveryPending || v[0].RecoveryNotifiedAt == nil || v[0].DeliveryError != "" {
		t.Fatal(v)
	}
}
func TestAcknowledgementCancelsQueuedDelivery(t *testing.T) {
	e, _ := fixture()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	e.cfg.Notifications.GenericWebhook = srv.URL
	now := time.Now()
	for i := 0; i < 3; i++ {
		e.Evaluate(context.Background(), bad(now.Add(time.Duration(i)*time.Second)))
	}
	j := <-e.queue
	e.Action(j.incident.ID, true, nil)
	e.deliver(context.Background(), j)
	if calls.Load() != 0 {
		t.Fatal("acknowledged pending notification delivered")
	}
}

func TestAsymmetricIncidentResolvesWhenOldHostIDRetired(t *testing.T) {
	e, m := fixture()
	now := time.Now()
	target := "old-host-id"
	for i := 0; i < 3; i++ {
		s := testkit.Snapshot(now.Add(time.Duration(i) * time.Second))
		s.Nodes[1].ID = target
		s.Edges = []model.Edge{
			{From: s.LocalNode, To: target, CQL: testkit.Good(s.GeneratedAt, 1), Internode: testkit.Good(s.GeneratedAt, 1), ObservedAt: s.GeneratedAt, Asymmetric: true},
			{From: target, To: s.LocalNode, CQL: model.Check{OK: false, State: "error", Kind: "cql_options", CheckedAt: s.GeneratedAt}, Internode: testkit.Good(s.GeneratedAt, 1), ObservedAt: s.GeneratedAt, Asymmetric: true},
		}
		e.Evaluate(context.Background(), s)
	}
	v, _ := m.Incidents(false)
	if len(v) != 1 || v[0].Kind != "asymmetric" {
		t.Fatal("asymmetric incident was not opened", v)
	}
	for i := 3; i < 5; i++ {
		s := testkit.Snapshot(now.Add(time.Duration(i) * time.Second))
		// target is intentionally absent: a fresh discovery now exposes a replacement identity.
		e.Evaluate(context.Background(), s)
	}
	v, _ = m.Incidents(false)
	if len(v) != 0 {
		t.Fatal("retired host_id left an orphan asymmetric incident", v)
	}
}

func TestAsymmetricIncidentDoesNotResolveWhenDiscoveryUnknown(t *testing.T) {
	e, m := fixture()
	now := time.Now()
	target := "old-host-id"
	for i := 0; i < 3; i++ {
		s := testkit.Snapshot(now.Add(time.Duration(i) * time.Second))
		s.Nodes[1].ID = target
		s.Edges = []model.Edge{
			{From: s.LocalNode, To: target, CQL: testkit.Good(s.GeneratedAt, 1), Internode: testkit.Good(s.GeneratedAt, 1), ObservedAt: s.GeneratedAt, Asymmetric: true},
			{From: target, To: s.LocalNode, CQL: model.Check{OK: false, State: "error", Kind: "cql_options", CheckedAt: s.GeneratedAt}, Internode: testkit.Good(s.GeneratedAt, 1), ObservedAt: s.GeneratedAt, Asymmetric: true},
		}
		e.Evaluate(context.Background(), s)
	}
	for i := 3; i < 7; i++ {
		s := testkit.Snapshot(now.Add(time.Duration(i) * time.Second))
		s.Discovery.OK = false
		s.Discovery.State = "error"
		s.Discovery.Message = "temporary discovery failure"
		e.Evaluate(context.Background(), s)
	}
	v, _ := m.Incidents(false)
	found := false
	for _, inc := range v {
		if inc.Kind == "asymmetric" {
			found = true
		}
	}
	if !found {
		t.Fatal("loss of discovery evidence incorrectly resolved asymmetric incident", v)
	}
}

func TestAsymmetricIncidentResolvesWhenDirectionsAgreeOnFailure(t *testing.T) {
	e, m := fixture()
	now := time.Now()
	target := testkit.Snapshot(now).Nodes[1].ID
	for i := 0; i < 3; i++ {
		s := testkit.Snapshot(now.Add(time.Duration(i) * time.Second))
		for j := range s.Edges {
			if (s.Edges[j].From == s.LocalNode && s.Edges[j].To == target) || (s.Edges[j].From == target && s.Edges[j].To == s.LocalNode) {
				s.Edges[j].Asymmetric = true
			}
		}
		e.Evaluate(context.Background(), s)
	}
	v, _ := m.Incidents(false)
	if len(v) != 1 || v[0].Kind != "asymmetric" {
		t.Fatal("asymmetric incident was not opened", v)
	}
	for i := 3; i < 5; i++ {
		s := testkit.Snapshot(now.Add(time.Duration(i) * time.Second))
		for j := range s.Edges {
			if (s.Edges[j].From == s.LocalNode && s.Edges[j].To == target) || (s.Edges[j].From == target && s.Edges[j].To == s.LocalNode) {
				s.Edges[j].Asymmetric = false
				s.Edges[j].CQL = model.Check{OK: false, State: "error", Kind: "cql_options", CheckedAt: s.GeneratedAt}
				s.Edges[j].Internode = model.Check{OK: false, State: "error", Kind: "tcp", CheckedAt: s.GeneratedAt}
				s.Edges[j].ObservedAt = s.GeneratedAt
			}
		}
		e.Evaluate(context.Background(), s)
	}
	v, _ = m.Incidents(false)
	if len(v) != 0 {
		t.Fatal("symmetric failure incorrectly kept asymmetry incident open", v)
	}
}
