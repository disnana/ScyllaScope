package store

import (
	"path/filepath"
	"scyllascope/internal/testkit"
	"testing"
	"time"
)

func TestSQLiteHistoryExcludesFutureAndHonorsLimit(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "scope.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	now := time.Now()
	for i := 0; i < 120; i++ {
		if e = s.AddSnapshot(testkit.Snapshot(now.Add(time.Duration(i-60) * time.Minute))); e != nil {
			t.Fatal(e)
		}
	}
	for _, limit := range []int{2, 3, 17, 50} {
		v, e := s.History(now.Add(-time.Hour), limit)
		if e != nil || len(v) > limit || len(v) == 0 {
			t.Fatal(e, len(v), limit)
		}
		for _, a := range v {
			if a.Time.After(time.Now()) {
				t.Fatal("future history returned")
			}
		}
		nodes, e := s.NodeHistory("100.64.0.1", now.Add(-time.Hour), limit)
		if e != nil || len(nodes) > limit || len(nodes) == 0 {
			t.Fatal(e, len(nodes), limit)
		}
		for _, n := range nodes {
			if n.LastUpdatedAt.After(time.Now()) {
				t.Fatal("future node returned")
			}
		}
	}
}
func TestSQLiteExplicitMemoryPragmasAndIndexes(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "scope.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	var cache int
	if e = s.db.QueryRow("PRAGMA cache_size").Scan(&cache); e != nil || cache != -2048 {
		t.Fatal(cache, e)
	}
	var count int
	if e = s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='node_samples_v5_ts_idx'").Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
}
