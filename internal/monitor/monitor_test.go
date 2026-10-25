package monitor

import (
	"context"
	"errors"
	"scyllascope/internal/model"
	"scyllascope/internal/settings"
	"scyllascope/internal/testkit"
	"sync"
	"testing"
	"time"
)

func monitorFixture() *Monitor {
	c := &settings.Config{ClusterName: "Example Cluster", NodeID: "test", Listen: "127.0.0.1:8091", LocalAddress: "127.0.0.1"}
	settings.Defaults(c)
	c.Monitoring.Timeout = 100 * time.Millisecond
	c.Monitoring.MaxNodes = 3
	c.Scylla.SystemdService = "disabled"
	m := New(c, testkit.NewMemory(), &testkit.Source{Err: errors.New("offline")}, nil)
	m.lastRuntime = time.Now()
	return m
}
func TestPublicationImmutability(t *testing.T) {
	m := monitorFixture()
	s := testkit.Snapshot(time.Now())
	m.Publish(s)
	s.Nodes[0].Name = "external mutation"
	v := m.Snapshot()
	if v.Nodes[0].Name == s.Nodes[0].Name {
		t.Fatal("mutable publication")
	}
	v.Nodes[0].Labels["memory"] = "mutated"
	if m.Snapshot().Nodes[0].Labels["memory"] == "mutated" {
		t.Fatal("mutable nested state")
	}
}
func TestConcurrentSubscribersGetNewestAndDoNotBlock(t *testing.T) {
	m := monitorFixture()
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 15; j++ {
				ch := m.Subscribe()
				_ = m.Snapshot()
				m.Unsubscribe(ch)
			}
		}()
	}
	ch := m.Subscribe()
	defer m.Unsubscribe(ch)
	for i := 0; i < 30; i++ {
		s := testkit.Snapshot(time.Now())
		s.Version = "last"
		m.Publish(s)
	}
	wg.Wait()
	select {
	case <-ch:
		v := m.Snapshot()
		if v.Version != "last" {
			t.Fatal(v.Version)
		}
	case <-time.After(time.Second):
		t.Fatal("no snapshot")
	}
}
func TestDiscoveryFailureNeverCreatesHealthyFallback(t *testing.T) {
	m := monitorFixture()
	if !m.RefreshNow(context.Background()) {
		t.Fatal("refresh")
	}
	s := m.Snapshot()
	if s.Discovery.OK || s.Health == model.Healthy {
		t.Fatal(s)
	}
	if len(s.Nodes) != 1 || s.Nodes[0].MetadataKnown {
		t.Fatal(s.Nodes)
	}
}
func TestRefreshSingleFlight(t *testing.T) {
	m := monitorFixture()
	wait := make(chan struct{})
	src := &testkit.Source{Wait: wait}
	m.source = src
	done := make(chan bool)
	go func() { done <- m.RefreshNow(context.Background()) }()
	time.Sleep(20 * time.Millisecond)
	if m.RefreshNow(context.Background()) {
		t.Fatal("concurrent refresh accepted")
	}
	close(wait)
	if !<-done {
		t.Fatal("first rejected")
	}
}
