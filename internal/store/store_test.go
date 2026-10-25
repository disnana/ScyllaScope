package store

// These tests require the REAL modernc.org/sqlite dependency; no replacement driver.
import (
	"path/filepath"
	"scyllascope/internal/model"
	"scyllascope/internal/testkit"
	"testing"
	"time"
)

func TestSQLitePersistenceAndReopen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "scope.db")
	s, e := Open(p)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	snap := testkit.Snapshot(now)
	if e = s.AddSnapshot(snap); e != nil {
		t.Fatal(e)
	}
	if e = s.SaveTopology(snap); e != nil {
		t.Fatal(e)
	}
	s.Add(model.Event{Time: now, Message: "persisted", Type: "test", Severity: "info"})
	s.PutIncident(model.Incident{ID: "123456789012345678901234", OpenedAt: now, UpdatedAt: now, RecoveryPending: true})
	s.Close()
	s, e = Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	v, e := s.LoadTopology()
	if e != nil || len(v.Nodes) != 3 {
		t.Fatal(v, e)
	}
	samples, e := s.History(now.Add(-time.Minute), 480)
	if e != nil || len(samples) != 1 {
		t.Fatal(samples, e)
	}
	events, e := s.Recent(20)
	if e != nil || len(events) != 1 || events[0].Message != "persisted" {
		t.Fatal(events, e)
	}
	inc, e := s.Incidents(true)
	if e != nil || len(inc) != 1 || !inc[0].RecoveryPending {
		t.Fatal(inc, e)
	}
}
func TestSQLiteActionsAndRetention(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "scope.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	now := time.Now()
	old := now.Add(-200 * time.Hour)
	s.AddSnapshot(testkit.Snapshot(old))
	s.AddSnapshot(testkit.Snapshot(now))
	id := "123456789012345678901234"
	s.PutIncident(model.Incident{ID: id, OpenedAt: old, UpdatedAt: old})
	if e = s.UpdateIncidentAction(id, true, nil); e != nil {
		t.Fatal(e)
	}
	if e = s.Prune(168); e != nil {
		t.Fatal(e)
	}
	v, _ := s.Incidents(false)
	if len(v) != 1 || !v[0].Acknowledged {
		t.Fatal(v)
	}
	h, _ := s.History(old.Add(-time.Hour), 480)
	if len(h) != 1 {
		t.Fatal(h)
	}
}
