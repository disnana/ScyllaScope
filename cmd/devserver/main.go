// Devserver serves the REAL HTTP API and Svelte UI with clearly labelled fixture data.
// It never connects to a database, is loopback-only, and is NOT the production binary.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"scyllascope/internal/alerts"
	"scyllascope/internal/api"
	"scyllascope/internal/core"
	"scyllascope/internal/model"
	"scyllascope/internal/monitor"
	"scyllascope/internal/settings"
	"scyllascope/internal/testkit"
	"strings"
	"sync"
	"time"
)

const devToken = "scyllascope-local-test-token-0000000000000000000000"

type fixture struct {
	mu       sync.Mutex
	mon      *monitor.Monitor
	store    *testkit.Memory
	scenario string
	seq      int
}

func (f *fixture) Snapshot() model.Snapshot        { return f.mon.Snapshot() }
func (f *fixture) Subscribe() chan struct{}        { return f.mon.Subscribe() }
func (f *fixture) Unsubscribe(ch chan struct{})    { f.mon.Unsubscribe(ch) }
func (f *fixture) RefreshNow(context.Context) bool { f.tick(); return true }
func (f *fixture) Query(ctx context.Context, q string, l int) (model.QueryResult, error) {
	select {
	case <-ctx.Done():
		return model.QueryResult{}, ctx.Err()
	case <-time.After(150 * time.Millisecond):
	}
	s := f.mon.Snapshot()
	rows := []map[string]any{}
	for _, n := range s.Nodes {
		rows = append(rows, map[string]any{"peer": n.Address, "dc": n.DC, "up": n.Up, "status": n.Status, "load": n.LoadBytes})
	}
	return model.QueryResult{Columns: []string{"peer", "dc", "up", "status", "load"}, Rows: rows, Count: len(rows), TookMS: 150}, nil
}
func (f *fixture) tick() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.scenario == "frozen" {
		return
	}
	f.seq++
	now := time.Now()
	s := testkit.Snapshot(now)
	s.Discovery.Message = "system.cluster_status (DEMO fixture, not a database connection)"
	s.Nodes[0].CQL.Latency = time.Duration(1100+(f.seq%7)*140) * time.Microsecond
	s.Nodes[0].MetricValues["scylla_compaction_manager_backlog"] = float64(f.seq%5) / 100
	switch f.scenario {
	case "partial":
		s.Edges = s.Edges[:2]
		for i := range s.Nodes {
			if i > 0 {
				s.Nodes[i].AgentAt = time.Time{}
				s.Nodes[i].AgentVersion = ""
				s.Nodes[i].WatchAPI = model.Check{State: "disabled", Kind: "http", Message: "observer not installed"}
			}
		}
		s.Warnings = []string{"観測ノード 1/3。監視エージェント未接続のため、逆方向は未確認です"}
	case "failure":
		fail := model.Check{State: "timeout", Kind: "tcp", Message: "connection timed out", CheckedAt: now}
		for i := range s.Edges {
			if s.Edges[i].From == s.Nodes[2].ID {
				s.Edges[i].Internode = fail
				s.Edges[i].CQL = fail
			}
		}
	case "unknown":
		s.Discovery = model.Check{State: "error", Kind: "query", Message: "DEMO: no CQL metadata response", CheckedAt: now}
		for i := range s.Nodes {
			s.Nodes[i].MetadataKnown = false
			s.Nodes[i].LoadKnown = false
		}
	case "reorder":
		s.Nodes[0], s.Nodes[2] = s.Nodes[2], s.Nodes[0]
	case "xss":
		s.Nodes[2].Name = `<img src=x onerror="window.INJECTED=true">`
	}
	core.Summarize(&s, 3, 30*time.Second)
	f.store.AddSnapshot(s)
	s.Events, _ = f.store.Recent(20)
	s.Incidents, _ = f.store.Incidents(true)
	f.mon.Publish(s)
}
func main() {
	port := flag.Int("port", 8765, "loopback dev port")
	flag.Parse()
	address := net.JoinHostPort("127.0.0.1", fmt.Sprint(*port))
	c := &settings.Config{ClusterName: "Example Cluster", NodeID: "demo-a", NodeName: "Demo A", LocalAddress: "100.64.0.1", Listen: address, Token: strings.Repeat("p", 64), Source: "DEMO fixture", Security: settings.SecurityConfig{UIToken: devToken, AllowedHosts: []string{"localhost"}}, Console: settings.ConsoleConfig{Enabled: true}}
	settings.Defaults(c)
	c.Monitoring.ExpectedNodes = 3
	c.Scylla.CQLHosts = []string{c.LocalAddress}
	st := testkit.NewMemory()
	m := monitor.New(c, st, &testkit.Source{}, nil)
	f := &fixture{mon: m, store: st}
	for i := 119; i >= 0; i-- {
		s := testkit.Snapshot(time.Now().Add(-time.Duration(i) * time.Minute))
		s.Nodes[0].CQL.Latency = time.Duration(1200+i%9*300) * time.Microsecond
		st.AddSnapshot(s)
	}
	for i := 0; i < 14; i++ {
		st.Add(model.Event{Time: time.Now().Add(-time.Duration(14-i) * 5 * time.Minute), Severity: []string{"info", "warning", "info"}[i%3], Type: "demo", Node: []string{"Demo A", "Demo B", "demo-c"}[i%3], Message: []string{"観測を完了しました", "接続の再試行を検出しました", "観測が復旧しました"}[i%3], Resolved: true})
	}
	st.PutIncident(model.Incident{ID: "123456789012345678901234", Observer: c.NodeID, Node: "100.64.0.3", Kind: "demo", Severity: "warning", Message: "デモアラート: 未確認の観測方向があります", OpenedAt: time.Now().Add(-3 * time.Minute), UpdatedAt: time.Now()})
	f.tick()
	engine := alerts.New(c, st)
	server := api.New(c, f, st, engine)
	real := server.Handler()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_test/scenario" {
			if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer "+devToken {
				http.Error(w, "denied", 403)
				return
			}
			var in struct {
				Scenario string `json:"scenario"`
			}
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in) != nil {
				http.Error(w, "bad request", 400)
				return
			}
			f.mu.Lock()
			f.scenario = in.Scenario
			f.mu.Unlock()
			f.tick()
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		real.ServeHTTP(w, r)
	})
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			f.tick()
		}
	}()
	log.Printf("DEMO ONLY http://%s · synthetic test token is defined in cmd/devserver", address)
	log.Fatal((&http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}).ListenAndServe())
}
