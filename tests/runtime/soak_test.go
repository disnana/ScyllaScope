package runtime_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"scyllascope/internal/alerts"
	"scyllascope/internal/model"
	"scyllascope/internal/settings"
	"scyllascope/internal/store"
	"scyllascope/internal/testkit"
	"strconv"
	"testing"
	"time"
)

// This is a real SQLite + alert state churn soak. It deliberately does not
// claim to emulate the Scylla driver, live topology, or systemd environment.
func TestSQLiteIncidentSoak(t *testing.T) {
	seconds, _ := strconv.Atoi(os.Getenv("SCYLLASCOPE_SOAK_SECONDS"))
	if seconds < 30 {
		t.Skip("set SCYLLASCOPE_SOAK_SECONDS>=30")
	}
	path := filepath.Join(t.TempDir(), "soak.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { st.Close() }()
	c := &settings.Config{NodeID: "observer", LocalAddress: "127.0.0.1"}
	settings.Defaults(c)
	c.Monitoring.RecoveryWindow = 1
	engine := alerts.New(c, st)
	report := func(phase string, cycles int) {
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		fds, _ := os.ReadDir("/proc/self/fd")
		rss, _ := os.ReadFile("/proc/self/statm")
		v, _ := json.Marshal(map[string]any{"phase": phase, "cycles": cycles, "heap_alloc": m.HeapAlloc, "heap_inuse": m.HeapInuse, "sys": m.Sys, "goroutines": runtime.NumGoroutine(), "fds": len(fds), "statm_pages": string(rss)})
		t.Log(string(v))
	}
	report("baseline", 0)
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	last := time.Now()
	cycles := 0
	for time.Now().Before(deadline) {
		now := time.Now()
		s := testkit.Snapshot(now)
		// Many retired identities used to retain per-incident counters indefinitely.
		v := model.Incident{ID: fmt.Sprintf("%09d", cycles), Observer: c.NodeID, Kind: "node_health", Node: fmt.Sprintf("retired-%d", cycles), OpenedAt: now, UpdatedAt: now}
		if err = st.PutIncident(v); err != nil {
			t.Fatal(err)
		}
		engine.Evaluate(context.Background(), s)
		v.ResolvedAt = &now
		v.Acknowledged = true
		v.UpdatedAt = now.Add(-2 * time.Hour)
		if err = st.PutIncident(v); err != nil {
			t.Fatal(err)
		}
		if err = st.AddSnapshot(s); err != nil {
			t.Fatal(err)
		}
		cycles++
		if cycles%20 == 0 {
			if err = st.Prune(1); err != nil {
				t.Fatal(err)
			}
		}
		if cycles%200 == 0 {
			st.Close()
			st, err = store.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			engine = alerts.New(c, st)
		}
		if time.Since(last) >= 30*time.Second {
			report("sample", cycles)
			last = time.Now()
		}
		time.Sleep(10 * time.Millisecond)
	}
	report("final", cycles)
}
