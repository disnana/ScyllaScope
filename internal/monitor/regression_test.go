package monitor

import (
	"context"
	"fmt"
	"scyllascope/internal/model"
	"scyllascope/internal/settings"
	"scyllascope/internal/testkit"
	"testing"
	"time"
)

func TestKnownCacheHardBoundDuring50000Replacements(t *testing.T) {
	m := monitorFixture()
	now := time.Now()
	for i := 0; i < 50000; i++ {
		addr := fmt.Sprintf("10.%d.%d.%d", (i/65536)%256, (i/256)%256, i%256)
		m.known[addr] = model.Node{Address: addr, MetadataAt: now.Add(time.Duration(i) * time.Second)}
		m.trimKnown(map[string]bool{addr: true}, true, now.Add(time.Duration(i)*time.Second))
		if len(m.known) > 2*m.cfg.Monitoring.MaxNodes {
			t.Fatalf("unbounded: %d", len(m.known))
		}
		if _, ok := m.known[addr]; !ok {
			t.Fatal("current node evicted")
		}
	}
	t.Logf("50000 replacements; retained=%d cap=%d", len(m.known), 2*m.cfg.Monitoring.MaxNodes)
}
func TestRetirementRequiresDiscoveryEvidence(t *testing.T) {
	m := monitorFixture()
	now := time.Now()
	m.known["10.1.1.1"] = model.Node{MetadataAt: now.Add(-48 * time.Hour)}
	m.known[m.cfg.LocalAddress] = model.Node{MetadataAt: now.Add(-48 * time.Hour)}
	m.trimKnown(nil, false, now)
	if len(m.known) != 2 {
		t.Fatal("outage lost remembered topology")
	}
	m.trimKnown(nil, true, now)
	if _, ok := m.known["10.1.1.1"]; ok {
		t.Fatal("retired member not expired")
	}
	if _, ok := m.known[m.cfg.LocalAddress]; !ok {
		t.Fatal("local node expired")
	}
}
func TestCacheStoresMetadataOnly(t *testing.T) {
	n := testkit.Snapshot(time.Now()).Nodes[0]
	n.ObservedBy = map[string]model.Check{"large": {}}
	n.Reasons = []string{"old"}
	v := metadataOnly(n)
	if v.ID != n.ID || v.Address != n.Address || v.MetricValues != nil || v.Labels != nil || v.ObservedBy != nil || v.Reasons != nil {
		t.Fatal(v)
	}
}
func TestCanceledRefreshDoesNotPublishFalseFailure(t *testing.T) {
	m := monitorFixture()
	m.Publish(testkit.Snapshot(time.Now()))
	before := m.Snapshot()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	m.AfterCollect = func(context.Context, model.Snapshot) { calls++ }
	if m.RefreshNow(ctx) {
		t.Fatal("canceled refresh accepted")
	}
	if !m.Snapshot().GeneratedAt.Equal(before.GeneratedAt) || calls != 0 {
		t.Fatal("canceled request changed health")
	}
}
func TestFullNotificationSlotKeepsNewestSnapshot(t *testing.T) {
	m := monitorFixture()
	ch := m.Subscribe()
	defer m.Unsubscribe(ch)
	s := testkit.Snapshot(time.Now())
	for i := 0; i < 1000; i++ {
		s.Version = fmt.Sprint(i)
		m.Publish(s)
	}
	if len(ch) != 1 || m.Snapshot().Version != "999" {
		t.Fatal("queue grew or latest snapshot lost")
	}
}

func TestFallbackPrioritizesLocalAndExplicitPeerDeterministically(t *testing.T) {
	m := monitorFixture()
	m.cfg.Peers = []settings.Peer{{Address: "192.0.2.10", Name: "explicit"}}
	for i := 0; i < 6; i++ {
		addr := fmt.Sprintf("10.0.0.%d", i+1)
		m.known[addr] = model.Node{Address: addr, MetadataAt: time.Now()}
	}
	for i := 0; i < 100; i++ {
		v := m.fallbackNodes()
		if v[0].Address != m.cfg.LocalAddress || v[1].Address != "192.0.2.10" {
			t.Fatal("local or configured peer hidden by cache ordering", v)
		}
	}
}
