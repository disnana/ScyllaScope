package monitor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"scyllascope/internal/alerts"
	"scyllascope/internal/api"
	"scyllascope/internal/model"
	"scyllascope/internal/settings"
	"scyllascope/internal/store"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type churnSource struct{ started time.Time }

func (s *churnSource) Discover(ctx context.Context) ([]model.Node, error) {
	elapsed := time.Since(s.started)
	generation := int(elapsed / (5 * time.Second))
	if int(elapsed/time.Second)%60 < 10 {
		return nil, fmt.Errorf("synthetic discovery outage")
	}
	return []model.Node{{ID: "local", Address: "127.0.0.1", Name: "local", Up: true, Status: "NORMAL"}, {ID: fmt.Sprintf("replacement-%d", generation), Address: fmt.Sprintf("127.1.%d.%d", generation/250, generation%250+1), Name: "synthetic replacement", Up: true, Status: "NORMAL"}}, nil
}
func (*churnSource) Query(context.Context, string, int) (model.QueryResult, error) {
	return model.QueryResult{}, nil
}
func (*churnSource) Close() {}

// The primary engine/store/monitor are never restarted during this experiment.
// This exercises the real collector and SQLite with synthetic discovery data,
// local failing probes, notification outage/recovery, and authenticated HTTP/SSE.
func TestLongCollectorSoak(t *testing.T) {
	seconds, _ := strconv.Atoi(os.Getenv("SCYLLASCOPE_LONG_SECONDS"))
	if seconds < 60 {
		t.Skip("set SCYLLASCOPE_LONG_SECONDS>=60")
	}
	root := t.TempDir()
	if p := os.Getenv("SCYLLASCOPE_LONG_OUTPUT"); p != "" {
		root = p
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
	}
	out, err := os.OpenFile(filepath.Join(root, "samples.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	encoder := json.NewEncoder(out)
	st, err := store.Open(filepath.Join(root, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	started := time.Now()
	var attempts, accepted, failures, sseConnections, sseSnapshots, gets atomic.Int64
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		phase := int(time.Since(started)/time.Second) % 120
		if phase < 45 {
			w.WriteHeader(503)
			return
		}
		accepted.Add(1)
		w.WriteHeader(204)
	}))
	defer receiver.Close()
	metrics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "scylla_storage_proxy_coordinator_read_timeouts 0\n")
	}))
	defer metrics.Close()
	metricsHost, metricsPort, _ := net.SplitHostPort(strings.TrimPrefix(metrics.URL, "http://"))
	port, _ := strconv.Atoi(metricsPort)
	c := &settings.Config{ClusterName: "soak", NodeID: "observer", NodeName: "synthetic", LocalAddress: "127.0.0.1", Listen: "127.0.0.1:0"}
	settings.Defaults(c)
	c.Monitoring.Timeout = 20 * time.Millisecond
	c.Monitoring.Interval = 250 * time.Millisecond
	c.Monitoring.SampleInterval = time.Second
	c.Monitoring.MaxNodes = 4
	c.Monitoring.Concurrency = 2
	c.Monitoring.KnownNodeRetention = time.Minute
	c.Monitoring.FailureWindow = 1
	c.Monitoring.RecoveryWindow = 1
	c.Security.UIToken = strings.Repeat("s", 64)
	c.Notifications.GenericWebhook = receiver.URL
	c.Notifications.RetryInterval = time.Second
	c.Scylla.CQLPort = 1
	c.Scylla.InternodePort = 1
	c.Scylla.SystemdService = "synthetic-soak-no-service"
	c.Scylla.RESTURL = metrics.URL
	c.Scylla.PrometheusAddress = metricsHost
	c.Scylla.PrometheusPort = port
	// Seed more work than both the UI limit and the bounded queue; it remains
	// durable during receiver outages and is drained when the receiver recovers.
	old := time.Now().Add(-time.Hour)
	for i := 0; i < 4096; i++ {
		if err := st.PutIncident(model.Incident{ID: fmt.Sprintf("seed-%06d", i), Observer: c.NodeID, Node: fmt.Sprintf("retired-%d", i), Kind: "node_health", OpenedAt: old, UpdatedAt: old, ResolvedAt: &old, NotificationPending: true}); err != nil {
			t.Fatal(err)
		}
	}
	// Saturate the bounded 500-row display working set before warmup, so later
	// active-incident growth on disk is not mistaken for a retained-heap leak.
	for i := 0; i < 600; i++ {
		if err := st.PutIncident(model.Incident{ID: fmt.Sprintf("open-%06d", i), Observer: c.NodeID, Node: fmt.Sprintf("retired-open-%d", i), Kind: "node_health", OpenedAt: old, UpdatedAt: old, LastNotifiedAt: &old}); err != nil {
			t.Fatal(err)
		}
	}
	engine := alerts.New(c, st)
	m := New(c, st, &churnSource{started}, nil)
	m.AfterCollect = engine.Evaluate
	// Host process introspection is unrelated to retention and would spawn local
	// commands. The collector's network, repository and notification paths run.
	m.lastRuntime = time.Now().Add(2 * time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); engine.Run(ctx) }()
	server := httptest.NewServer(api.New(c, m, st, engine).Handler())
	defer server.Close()
	client := &http.Client{Transport: &http.Transport{Proxy: nil, MaxIdleConns: 8, IdleConnTimeout: time.Second}}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/v1/stream", nil)
				req.Header.Set("Authorization", "Bearer "+c.Security.UIToken)
				resp, e := client.Do(req)
				if e != nil {
					if ctx.Err() == nil {
						failures.Add(1)
					}
					return
				}
				if resp.StatusCode != 200 {
					failures.Add(1)
					resp.Body.Close()
					return
				}
				sseConnections.Add(1)
				reader := bufio.NewScanner(resp.Body)
				reader.Buffer(make([]byte, 4096), 4<<20)
				n := 0
				for reader.Scan() {
					if strings.HasPrefix(reader.Text(), "data:") {
						sseSnapshots.Add(1)
						n++
					}
					if n >= 20 {
						break
					}
				}
				resp.Body.Close()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/v1/snapshot", nil)
				req.Header.Set("Authorization", "Bearer "+c.Security.UIToken)
				resp, e := client.Do(req)
				if e != nil {
					if ctx.Err() == nil {
						failures.Add(1)
					}
					continue
				}
				var snap model.Snapshot
				e = json.NewDecoder(resp.Body).Decode(&snap)
				resp.Body.Close()
				if resp.StatusCode != 200 || e != nil {
					failures.Add(1)
				}
				gets.Add(1)
			}
		}
	}()
	cycles := 0
	last := time.Now()
	warmup := min(300, seconds/5)
	nextProfile := warmup
	sample := func(phase string) {
		runtime.GC()
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		rssRaw, _ := os.ReadFile("/proc/self/statm")
		fields := strings.Fields(string(rssRaw))
		rss := int64(0)
		if len(fields) > 1 {
			pages, _ := strconv.ParseInt(fields[1], 10, 64)
			rss = pages * int64(os.Getpagesize())
		}
		fds, _ := os.ReadDir("/proc/self/fd")
		stats := engine.Stats()
		if len(m.known) > 2*c.Monitoring.MaxNodes || stats.Queued > stats.QueueCapacity || stats.Attempts > 2048 {
			t.Fatalf("resource bound exceeded known=%d stats=%+v", len(m.known), stats)
		}
		m.mu.RLock()
		subs := len(m.subs)
		m.mu.RUnlock()
		durableWork, seedPending := 0, 0
		cursor := ""
		for {
			page, e := st.WorkIncidents(cursor, 256)
			if e != nil {
				t.Fatal(e)
			}
			if len(page) == 0 {
				break
			}
			for _, v := range page {
				cursor = v.ID
				durableWork++
				if strings.HasPrefix(v.ID, "seed-") {
					seedPending++
				}
			}
		}
		if phase == "final" && seedPending != 0 {
			t.Errorf("initial 4096 pending jobs not drained: %d", seedPending)
		}
		row := map[string]any{"durable_work": durableWork, "seed_pending": seedPending, "elapsed_seconds": time.Since(started).Seconds(), "phase": phase, "cycles": cycles, "heap_alloc_bytes": mem.HeapAlloc, "heap_objects": mem.HeapObjects, "heap_inuse_bytes": mem.HeapInuse, "rss_bytes": rss, "goroutines": runtime.NumGoroutine(), "fds": len(fds), "known": len(m.known), "subscribers": subs, "alerts": stats, "attempts": attempts.Load(), "accepted": accepted.Load(), "http_failures": failures.Load(), "sse_connections": sseConnections.Load(), "sse_snapshots": sseSnapshots.Load(), "snapshot_gets": gets.Load()}
		if err := encoder.Encode(row); err != nil {
			t.Fatal(err)
		}
		out.Sync()
	}
	profile := func(label string) {
		f, e := os.OpenFile(filepath.Join(root, label+".heap"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			t.Fatal(e)
		}
		if e = pprof.WriteHeapProfile(f); e != nil {
			t.Fatal(e)
		}
		f.Close()
	}
	sample("baseline")
	ticker := time.NewTicker(c.Monitoring.Interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
		case <-ticker.C:
			m.RefreshNow(ctx)
			cycles++
			elapsed := int(time.Since(started) / time.Second)
			if time.Since(last) >= 30*time.Second {
				sample("sample")
				last = time.Now()
			}
			if elapsed >= nextProfile && nextProfile < seconds {
				runtime.GC()
				profile(fmt.Sprintf("at-%d-seconds", nextProfile))
				if nextProfile == warmup {
					nextProfile = seconds / 2
				} else {
					nextProfile = seconds
				}
			}
		}
	}
	cancel()
	wg.Wait()
	client.CloseIdleConnections()
	server.Close()
	m.http.CloseIdleConnections()
	sample("final")
	profile("final")
	if failures.Load() != 0 || accepted.Load() == 0 || attempts.Load() <= accepted.Load() {
		t.Fatalf("workload failure http=%d accepted=%d attempts=%d", failures.Load(), accepted.Load(), attempts.Load())
	}
	t.Logf("completed %ds cycles=%d results=%s", seconds, cycles, root)
}
