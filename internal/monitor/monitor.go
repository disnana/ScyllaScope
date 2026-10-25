package monitor

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"scyllascope/internal/core"
	"scyllascope/internal/model"
	"scyllascope/internal/probe"
	"scyllascope/internal/process"
	"scyllascope/internal/repository"
	"scyllascope/internal/settings"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Source interface {
	Discover(context.Context) ([]model.Node, error)
	Query(context.Context, string, int) (model.QueryResult, error)
	Close()
}
type Monitor struct {
	cfg                                *settings.Config
	store                              repository.Repository
	source                             Source
	http                               *http.Client
	tls                                *tls.Config
	mu                                 sync.RWMutex
	refreshMu                          sync.Mutex
	snapshot                           model.Snapshot
	subs                               map[chan struct{}]struct{}
	names                              map[string]string
	lastPrune, lastSample, lastRuntime time.Time
	version                            string
	shards                             int
	memory                             string
	known                              map[string]model.Node
	AfterCollect                       func(context.Context, model.Snapshot)
}

func New(c *settings.Config, st repository.Repository, source Source, tlsCfg *tls.Config) *Monitor {
	m := &Monitor{cfg: c, store: st, source: source, tls: tlsCfg, http: probe.Client(c.Monitoring.Timeout), subs: map[chan struct{}]struct{}{}, known: map[string]model.Node{}, names: map[string]string{}, snapshot: model.Snapshot{Version: model.Version, SchemaVersion: 2, ClusterName: c.ClusterName, LocalNode: c.NodeID, LocalNodeName: c.NodeName, LocalAddress: c.LocalAddress, Health: model.Unknown, Nodes: []model.Node{}, Edges: []model.Edge{}, Incidents: []model.Incident{}, StaleAfterSeconds: c.Monitoring.StaleAfter.Seconds()}}
	if st != nil {
		if s, e := st.LoadTopology(); e == nil && s.ClusterName == c.ClusterName {
			for _, n := range s.Nodes {
				m.known[n.Address] = metadataOnly(n)
			}
		}
	}
	m.trimKnown(nil, false, time.Now())
	return m
}
func (m *Monitor) Snapshot() model.Snapshot {
	m.mu.RLock()
	s := m.snapshot
	m.mu.RUnlock()
	// Published state is never mutated; do expensive copying outside the lock.
	return model.CloneSnapshot(s)
}
func (m *Monitor) Subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	m.mu.Lock()
	m.subs[ch] = struct{}{}
	m.mu.Unlock()
	return ch
}
func (m *Monitor) Unsubscribe(ch chan struct{}) {
	m.mu.Lock()
	delete(m.subs, ch)
	m.mu.Unlock()
}
func (m *Monitor) Publish(s model.Snapshot) {
	owned := model.CloneSnapshot(s)
	m.mu.Lock()
	m.snapshot = owned
	for ch := range m.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	m.mu.Unlock()
}
func (m *Monitor) Run(ctx context.Context) {
	defer m.source.Close()
	defer m.http.CloseIdleConnections()
	m.RefreshNow(ctx)
	t := time.NewTicker(m.cfg.Monitoring.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.RefreshNow(ctx)
		}
	}
}
func (m *Monitor) RefreshNow(ctx context.Context) bool {
	if ctx.Err() != nil || !m.refreshMu.TryLock() {
		return false
	}
	defer m.refreshMu.Unlock()
	m.refresh(ctx)
	return true
}
func (m *Monitor) refresh(ctx context.Context) {
	parent := ctx
	started := time.Now()
	budget := max(15*time.Second, time.Duration((m.cfg.Monitoring.MaxNodes+m.cfg.Monitoring.Concurrency-1)/m.cfg.Monitoring.Concurrency)*m.cfg.Monitoring.Timeout*3)
	budget = min(budget, 90*time.Second)
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	if time.Since(m.lastRuntime) > time.Minute {
		m.names = tailscaleNames(ctx)
		m.shards, m.memory, m.version = localRuntimeInfo(ctx)
		m.lastRuntime = time.Now()
	}
	qStart := time.Now()
	nodes, err := m.source.Discover(ctx)
	discovery := model.Check{OK: err == nil, State: "ok", Kind: "query", CheckedAt: time.Now(), Latency: time.Since(qStart), Message: "system.cluster_status"}
	warnings := append([]string{}, m.cfg.DetectionIssues...)
	if err != nil {
		discovery.OK = false
		discovery.State = "error"
		discovery.Message = err.Error()
		warnings = append(warnings, "クラスタ一覧の取得失敗: "+err.Error())
		nodes = nil
	}
	if len(nodes) > m.cfg.Monitoring.MaxNodes {
		nodes = nodes[:m.cfg.Monitoring.MaxNodes]
		discovery.OK = false
		discovery.State = "error"
		discovery.Message = "max_nodes を超えました"
	}
	if err == nil && len(nodes) == 0 {
		err = fmt.Errorf("system.cluster_status returned no members")
		discovery.OK = false
		discovery.State = "error"
		discovery.Message = err.Error()
	}
	cleaned := make([]model.Node, 0, len(nodes))
	validSeen := map[string]bool{}
	for _, n := range nodes {
		if net.ParseIP(n.Address) == nil || validSeen[n.Address] {
			discovery.OK = false
			discovery.State = "error"
			discovery.Message = "invalid or duplicate peer address"
			continue
		}
		validSeen[n.Address] = true
		cleaned = append(cleaned, n)
	}
	nodes = cleaned
	seen := map[string]bool{}
	now := time.Now()
	for i := range nodes {
		n := &nodes[i]
		if net.ParseIP(n.Address) == nil || seen[n.Address] {
			continue
		}
		seen[n.Address] = true
		n.MetadataKnown = true
		n.MetadataAt = now
		n.Missing = false
		if n.ID == "" {
			n.ID = n.Address
		}
		m.known[n.Address] = metadataOnly(*n)
	}
	m.trimKnown(seen, discovery.OK, now)
	// Retain known/explicit nodes during outages, but do not claim cached metadata
	// is current. Fresh TCP reachability alone cannot establish cluster health.
	for _, n := range m.fallbackNodes() {
		if !seen[n.Address] && len(nodes) < m.cfg.Monitoring.MaxNodes {
			n.Missing = discovery.OK
			n.MetadataKnown = false
			n.LoadKnown = false
			nodes = append(nodes, n)
			seen[n.Address] = true
		}
	}
	if len(nodes) > m.cfg.Monitoring.MaxNodes {
		nodes = nodes[:m.cfg.Monitoring.MaxNodes]
	}
	localID := m.cfg.LocalAddress
	for i := range nodes {
		n := &nodes[i]
		n.CQL = model.Check{}
		n.Internode = model.Check{}
		n.WatchAPI = model.UnknownCheck("ScyllaScope peer 未確認")
		n.LocalService = model.UnknownCheck("リモートノードのエージェントが必要")
		n.REST = model.UnknownCheck("ローカルAPIのみ")
		n.AgentVersion = ""
		n.AgentAt = time.Time{}
		n.ObservedBy = map[string]model.Check{}
		n.MetricValues = nil
		n.Metrics = model.UnknownCheck("未収集")
		n.Labels = map[string]string{}
		if n.ID == "" {
			n.ID = n.Address
		}
		if name := m.names[n.Address]; name != "" {
			n.Name = name
		}
		for _, p := range m.cfg.Peers {
			if p.Address == n.Address && p.Name != "" && p.Name != p.Address {
				n.Name = p.Name
			}
		}
		if n.Name == "" {
			n.Name = n.Address
		}
		if n.Address == m.cfg.LocalAddress {
			localID = n.ID
			n.Name = m.cfg.NodeName
		}
	}
	sem := make(chan struct{}, m.cfg.Monitoring.Concurrency)
	var wg sync.WaitGroup
	for i := range nodes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			n := &nodes[i]
			probeHost := n.Address
			if n.Address == m.cfg.LocalAddress && len(m.cfg.Scylla.CQLHosts) > 0 {
				probeHost = m.cfg.Scylla.CQLHosts[0]
			}
			n.CQL = probe.CQL(ctx, probeHost, m.cfg.Scylla.CQLPort, m.cfg.Monitoring.Timeout, m.tls)
			n.Internode = probe.TCP(ctx, n.Address, m.cfg.Scylla.InternodePort, m.cfg.Monitoring.Timeout)
			if n.Address == m.cfg.LocalAddress {
				n.WatchAPI = model.Check{OK: true, State: "ok", Kind: "http", Message: "この観測ノード", CheckedAt: time.Now()}
				n.AgentVersion = model.Version
				n.AgentAt = time.Now()
				n.LocalService = probe.Service(ctx, m.cfg.Scylla.SystemdService, m.cfg.Monitoring.Timeout)
				n.REST = probe.HTTP(ctx, m.http, strings.TrimRight(m.cfg.Scylla.RESTURL, "/")+"/storage_service/host_id")
				n.Shards = m.shards
				n.Version = m.version
				if m.memory != "" {
					n.Labels["memory"] = m.memory
				}
				u := "http://" + net.JoinHostPort(m.cfg.Scylla.PrometheusAddress, strconv.Itoa(m.cfg.Scylla.PrometheusPort)) + "/metrics"
				n.Metrics, n.MetricValues = probe.Metrics(ctx, m.http, u)
			}
			n.State, n.Reasons = core.NodeHealth(*n, time.Now(), m.cfg.Monitoring.StaleAfter)
			n.LastUpdatedAt = time.Now()
		}(i)
	}
	wg.Wait()
	edges := []model.Edge{}
	for _, n := range nodes {
		if n.Address != m.cfg.LocalAddress {
			edges = append(edges, model.Edge{From: localID, To: n.ID, CQL: n.CQL, Internode: n.Internode, WatchAPI: n.WatchAPI, ObservedAt: n.LastUpdatedAt})
		}
	}
	type peerResult struct {
		address string
		snap    model.Snapshot
		err     error
	}
	results := make(chan peerResult, m.cfg.Monitoring.Concurrency)
	for _, n := range nodes {
		if n.Address == m.cfg.LocalAddress {
			continue
		}
		wg.Add(1)
		go func(address string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			s, e := m.fetchPeer(ctx, address)
			select {
			case results <- peerResult{address, s, e}:
			case <-ctx.Done():
			}
		}(n.Address)
	}
	go func() { wg.Wait(); close(results) }()
	// Consume while fetching. Do not retain a decoded snapshot for every peer.
	for r := range results {
		if r.err != nil {
			for i := range nodes {
				if nodes[i].Address == r.address {
					state := "error"
					if m.cfg.Token == "" {
						state = "disabled"
					}
					nodes[i].WatchAPI = model.Check{State: state, Kind: "http", Message: r.err.Error(), CheckedAt: time.Now()}
				}
			}
			continue
		}
		nodes, edges = core.MergePeer(nodes, edges, r.snap, r.address, time.Now(), m.cfg.Monitoring.StaleAfter)
	}
	// Refresh local edge WatchAPI result; remote edges retain the peer's view.
	for i := range edges {
		if edges[i].From == localID {
			for _, n := range nodes {
				if edges[i].To == n.ID {
					edges[i].WatchAPI = n.WatchAPI
				}
			}
		}
	}
	core.SortNodes(nodes)
	s := model.Snapshot{SchemaVersion: 2, Version: model.Version, ClusterName: m.cfg.ClusterName, LocalNode: localID, LocalNodeName: m.cfg.NodeName, LocalAddress: m.cfg.LocalAddress, Nodes: nodes, Edges: edges, Discovery: discovery, Warnings: warnings, GeneratedAt: time.Now(), StaleAfterSeconds: m.cfg.Monitoring.StaleAfter.Seconds(), CollectDurationMS: float64(time.Since(started).Microseconds()) / 1000}
	core.Summarize(&s, m.cfg.Monitoring.ExpectedNodes, m.cfg.Monitoring.StaleAfter)
	if s.Coverage.AgentsReady < len(nodes) {
		s.Warnings = append(s.Warnings, "全方向の観測には、各ノードへのScyllaScope導入と共通cluster_tokenが必要です")
	}
	if m.cfg.Token == "" {
		s.Warnings = append(s.Warnings, "cluster_token未設定: 認証済みpeer状態交換は無効です")
	}
	// Cancelling a manual request or shutdown must not publish partial evidence.
	if parent.Err() != nil {
		return
	}
	if m.AfterCollect != nil {
		m.AfterCollect(ctx, s)
	}
	if m.store != nil {
		if v, e := m.store.Incidents(true); e == nil {
			resolved := 0
			for _, incident := range v {
				if incident.ResolvedAt == nil || resolved < 30 {
					s.Incidents = append(s.Incidents, incident)
					if incident.ResolvedAt != nil {
						resolved++
					}
				}
			}
		}
		if v, e := m.store.Recent(15); e == nil {
			s.Events = v
		}
		if time.Since(m.lastSample) >= m.cfg.Monitoring.SampleInterval {
			if e := m.store.AddSnapshot(s); e != nil {
				s.Warnings = append(s.Warnings, "履歴保存失敗: "+e.Error())
			} else {
				m.lastSample = time.Now()
			}
		}
		if discovery.OK {
			if e := m.store.SaveTopology(topologyMetadata(s)); e != nil {
				s.Warnings = append(s.Warnings, "トポロジー保存失敗: "+e.Error())
			}
		}
		if time.Since(m.lastPrune) > 10*time.Minute {
			if e := m.store.Prune(m.cfg.Monitoring.HistoryHours); e == nil {
				m.lastPrune = time.Now()
			} else {
				s.Warnings = append(s.Warnings, "履歴の期限切れ削除に失敗: "+e.Error())
			}
		}
	}
	m.Publish(s)
}
func (m *Monitor) fetchPeer(ctx context.Context, address string) (model.Snapshot, error) {
	var s model.Snapshot
	if m.cfg.Token == "" {
		return s, fmt.Errorf("cluster_token not configured")
	}
	if !settings.PrivateAddress(address) {
		return s, fmt.Errorf("public peer auto-discovery refused")
	}
	_, port, _ := net.SplitHostPort(m.cfg.Listen)
	base := "http://" + net.JoinHostPort(address, port)
	for _, p := range m.cfg.Peers {
		if p.Address == address && p.API != "" {
			base = p.API
		}
	}
	u, e := url.Parse(base)
	if e != nil || u.Hostname() != address {
		return s, fmt.Errorf("peer endpoint does not match address")
	}
	req, e := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(base, "/")+"/api/v1/peer/state", nil)
	if e != nil {
		return s, e
	}
	req.Header.Set("Authorization", "Bearer "+m.cfg.Token)
	resp, e := m.http.Do(req)
	if e != nil {
		return s, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return s, fmt.Errorf("peer HTTP %d (token / version / install)", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if e != nil || len(b) > 2*1024*1024 {
		return s, fmt.Errorf("peer response too large or incomplete")
	}
	if e = json.Unmarshal(b, &s); e != nil {
		return s, e
	}
	e = core.ValidatePeer(s, m.cfg.ClusterName, address, time.Now(), m.cfg.Monitoring.StaleAfter)
	return s, e
}
func (m *Monitor) Query(ctx context.Context, q string, limit int) (model.QueryResult, error) {
	return m.source.Query(ctx, q, limit)
}
func localRuntimeInfo(parent context.Context) (int, string, string) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	out, e := process.Output(ctx, 4096, "pgrep", "-xo", "scylla")
	if e != nil {
		return 0, "", ""
	}
	pid := strings.TrimSpace(string(out))
	if _, e = strconv.Atoi(pid); e != nil {
		return 0, "", ""
	}
	b, _ := os.ReadFile("/proc/" + pid + "/cmdline")
	args := strings.Split(string(b), "\x00")
	shards := 0
	memory := ""
	for i, a := range args {
		if a == "--smp" && i+1 < len(args) {
			shards, _ = strconv.Atoi(args[i+1])
		}
		if strings.HasPrefix(a, "--smp=") {
			shards, _ = strconv.Atoi(strings.TrimPrefix(a, "--smp="))
		}
		if a == "--memory" && i+1 < len(args) {
			memory = args[i+1]
		}
		if strings.HasPrefix(a, "--memory=") {
			memory = strings.TrimPrefix(a, "--memory=")
		}
	}
	version := ""
	if b, e := process.Output(ctx, 4096, "scylla", "--version"); e == nil {
		version = strings.TrimSpace(string(b))
	}
	return shards, memory, version
}
func tailscaleNames(parent context.Context) map[string]string {
	out := map[string]string{}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	b, e := process.Output(ctx, 2<<20, "tailscale", "status", "--json")
	if e != nil {
		return out
	}
	type peer struct {
		HostName     string
		TailscaleIPs []string
	}
	var raw struct {
		Self peer
		Peer map[string]peer
	}
	if json.Unmarshal(b, &raw) != nil {
		return out
	}
	for _, ip := range raw.Self.TailscaleIPs {
		out[ip] = raw.Self.HostName
	}
	for _, p := range raw.Peer {
		for _, ip := range p.TailscaleIPs {
			out[ip] = p.HostName
		}
	}
	return out
}
