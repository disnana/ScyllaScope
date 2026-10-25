package api

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"scyllascope/internal/core"
	"scyllascope/internal/model"
	"scyllascope/internal/repository"
	"scyllascope/internal/security"
	"scyllascope/internal/settings"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed all:static
var staticFS embed.FS

type Monitor interface {
	Snapshot() model.Snapshot
	Subscribe() chan struct{}
	Unsubscribe(chan struct{})
	RefreshNow(context.Context) bool
	Query(context.Context, string, int) (model.QueryResult, error)
}
type AlertControl interface {
	Test(context.Context) error
	Action(string, bool, *time.Time) error
}

// Observer permits deterministic API tests without replacing production DB libraries.
type Observer interface {
	Snapshot() model.Snapshot
	RefreshNow(context.Context) bool
	Query(context.Context, string, int) (model.QueryResult, error)
	Subscribe() chan struct{}
	Unsubscribe(chan struct{})
}
type Server struct {
	cfg        *settings.Config
	mon        Monitor
	store      repository.Repository
	alerts     AlertControl
	guard      *security.Guard
	sseSlots   chan struct{}
	querySlot  chan struct{}
	workSlots  chan struct{}
	notifyMu   sync.Mutex
	lastNotify time.Time
}

func New(c *settings.Config, m Monitor, s repository.Repository, a AlertControl) *Server {
	return &Server{cfg: c, mon: m, store: s, alerts: a, guard: security.New(c.Security.UIToken, c.Listen, c.Security.AllowedHosts), sseSlots: make(chan struct{}, 32), querySlot: make(chan struct{}, 2), workSlots: make(chan struct{}, 16)}
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth", s.auth)
	mux.HandleFunc("/api/v1/login", s.login)
	mux.HandleFunc("/api/v1/logout", s.logout)
	mux.HandleFunc("/api/v1/snapshot", s.snapshot)
	mux.HandleFunc("/api/v1/stream", s.stream)
	mux.HandleFunc("/api/v1/events", s.events)
	mux.HandleFunc("/api/v1/incidents", s.incidents)
	mux.HandleFunc("/api/v1/incidents/action", s.incidentAction)
	mux.HandleFunc("/api/v1/history", s.history)
	mux.HandleFunc("/api/v1/refresh", s.refresh)
	mux.HandleFunc("/api/v1/query", s.query)
	mux.HandleFunc("/api/v1/config", s.publicConfig)
	mux.HandleFunc("/api/v1/doctor", s.doctor)
	mux.HandleFunc("/api/v1/export", s.export)
	mux.HandleFunc("/api/v1/runtime", s.runtimeInfo)
	mux.HandleFunc("/api/v1/notifications/test", s.testNotification)
	mux.HandleFunc("/api/v1/peer/state", s.peer)
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/readyz", s.ready)
	mux.HandleFunc("/api/v1/health", s.health)
	mux.Handle("/", newAssets(staticFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		security.Headers(w)
		if !s.guard.HostAllowed(r) {
			http.Error(w, "Host not allowed; configure security.allowed_hosts for your reverse proxy", 403)
			return
		}
		p := r.URL.Path
		if strings.HasPrefix(p, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
			public := p == "/api/v1/auth" || p == "/api/v1/login" || p == "/api/v1/peer/state" || p == "/api/v1/health"
			if !public && s.cfg.Security.UIToken != "" && !s.guard.Authenticated(r) {
				http.Error(w, "authentication required", 401)
				return
			}
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if !security.MutationAllowed(r) {
				http.Error(w, "same-origin request header required", 403)
				return
			}
			if p != "/api/v1/login" && !s.guard.Authenticated(r) {
				http.Error(w, "read-only mode; configure UI token or sign in", 401)
				return
			}
		}
		// SSE owns separate slots and per-write deadlines; ordinary requests must
		// not create an unbounded queue behind the single SQLite connection.
		if p != "/api/v1/stream" {
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(20 * time.Second))
		}
		if strings.HasPrefix(p, "/api/") && p != "/api/v1/stream" && p != "/api/v1/health" && p != "/api/v1/auth" {
			select {
			case s.workSlots <- struct{}{}:
				defer func() { <-s.workSlots }()
			default:
				w.Header().Set("Retry-After", "1")
				http.Error(w, "request concurrency limit reached", 503)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func method(w http.ResponseWriter, r *http.Request, m string) bool {
	if r.Method != m {
		w.Header().Set("Allow", m)
		http.Error(w, "method not allowed", 405)
		return false
	}
	return true
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		http.Error(w, "invalid JSON request", 400)
		return false
	}
	if e := d.Decode(&struct{}{}); e != io.EOF {
		http.Error(w, "trailing JSON not permitted", 400)
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, v any) {
	b, e := json.Marshal(v)
	if e != nil {
		http.Error(w, "response encoding failed", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(b)
}
func (s *Server) auth(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") {
		return
	}
	writeJSON(w, map[string]any{"required": s.cfg.Security.UIToken != "", "authenticated": s.guard.Authenticated(r), "read_only": s.cfg.Security.UIToken == "", "version": model.Version})
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &in) {
		return
	}
	if s.guard.Login(w, r, in.Token) {
		writeJSON(w, map[string]bool{"ok": true})
	}
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	s.guard.Logout(w, r)
	writeJSON(w, map[string]bool{"ok": true})
}
func (s *Server) enriched() model.Snapshot {
	snap := s.mon.Snapshot()
	if s.store != nil {
		if v, e := s.store.Recent(20); e == nil {
			snap.Events = v
		}
		if v, e := s.store.Incidents(true); e == nil {
			snap.Incidents = []model.Incident{}
			resolved := 0
			for _, incident := range v {
				if incident.ResolvedAt == nil || resolved < 30 {
					snap.Incidents = append(snap.Incidents, incident)
					if incident.ResolvedAt != nil {
						resolved++
					}
				}
			}
		}
	}
	return snap
}
func (s *Server) snapshot(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") {
		return
	}
	writeJSON(w, s.enriched())
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") {
		return
	}
	writeJSON(w, map[string]any{"ok": true, "version": model.Version, "service": "scyllascope", "meaning": "process liveness, not database health"})
}
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") {
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	v := s.mon.Snapshot()
	ok := v.Discovery.OK && !v.GeneratedAt.IsZero() && time.Since(v.GeneratedAt) <= s.cfg.Monitoring.StaleAfter && v.GeneratedAt.Sub(time.Now()) < 5*time.Second
	if !ok {
		w.WriteHeader(503)
	}
	writeJSON(w, map[string]any{"ok": ok, "health": v.Health, "generated_at": v.GeneratedAt})
}
func (s *Server) peer(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") {
		return
	}
	if !security.Bearer(r, s.cfg.Token) {
		http.Error(w, "peer authentication required (cluster_token)", 401)
		return
	}
	v := s.mon.Snapshot()
	v.Events = nil
	v.Incidents = nil
	v.Alerts = nil
	own := []model.Edge{}
	for _, e := range v.Edges {
		if e.From == v.LocalNode {
			own = append(own, e)
		}
	}
	v.Edges = own
	writeJSON(w, v)
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") {
		return
	}
	n := 100
	if x, e := strconv.Atoi(r.URL.Query().Get("limit")); e == nil {
		n = max(1, min(x, 500))
	}
	v, e := s.store.Recent(n)
	if e != nil {
		http.Error(w, "event storage unavailable", 500)
		return
	}
	writeJSON(w, v)
}
func (s *Server) incidents(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") {
		return
	}
	v, e := s.store.Incidents(r.URL.Query().Get("all") == "1")
	if e != nil {
		http.Error(w, "incident storage unavailable", 500)
		return
	}
	writeJSON(w, v)
}
func (s *Server) incidentAction(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	var in struct {
		ID      string `json:"id"`
		Action  string `json:"action"`
		Minutes int    `json:"minutes"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.ID) != 24 {
		http.Error(w, "invalid incident id", 400)
		return
	}
	var until *time.Time
	ack := false
	switch in.Action {
	case "ack":
		ack = true
	case "resume":
	case "snooze":
		if in.Minutes != 15 && in.Minutes != 60 && in.Minutes != 1440 {
			http.Error(w, "minutes must be 15, 60, or 1440", 400)
			return
		}
		t := time.Now().Add(time.Duration(in.Minutes) * time.Minute)
		until = &t
	default:
		http.Error(w, "invalid action", 400)
		return
	}
	if e := s.alerts.Action(in.ID, ack, until); e != nil {
		http.Error(w, "incident not found / storage error", 404)
		return
	}
	_ = s.store.Add(model.Event{Time: time.Now(), Severity: "info", Type: "incident_" + in.Action, Node: s.cfg.NodeID, Message: in.ID + ": " + in.Action, Resolved: true})
	writeJSON(w, map[string]bool{"ok": true})
}
func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") {
		return
	}
	hours := min(6, s.cfg.Monitoring.HistoryHours)
	if raw := r.URL.Query().Get("hours"); raw != "" {
		v, e := strconv.Atoi(raw)
		if e != nil || v < 1 || v > s.cfg.Monitoring.HistoryHours {
			http.Error(w, "invalid history range", 400)
			return
		}
		hours = v
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	if node := r.URL.Query().Get("node"); node != "" {
		if len(node) > 128 {
			http.Error(w, "invalid node", 400)
			return
		}
		v, e := s.store.NodeHistory(node, since, 480)
		if e != nil {
			http.Error(w, "node history unavailable", 500)
			return
		}
		writeJSON(w, v)
		return
	}
	v, e := s.store.History(since, 480)
	if e != nil {
		http.Error(w, "history unavailable", 500)
		return
	}
	writeJSON(w, v)
}
func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	if !s.mon.RefreshNow(r.Context()) {
		w.WriteHeader(202)
		writeJSON(w, map[string]any{"busy": true, "message": "refresh already running"})
		return
	}
	writeJSON(w, s.enriched())
}
func (s *Server) doctor(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	s.mon.RefreshNow(r.Context())
	v := s.enriched()
	writeJSON(w, map[string]any{"discovery": v.Discovery, "warnings": v.Warnings, "nodes": v.Nodes, "coverage": v.Coverage, "checked_at": v.GeneratedAt, "read_only": true})
}
func (s *Server) publicConfig(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") {
		return
	}
	writeJSON(w, s.publicValues())
}
func (s *Server) publicValues() map[string]any {
	return map[string]any{"version": model.Version, "config_source": s.cfg.Source, "scylla_config": s.cfg.Scylla.ConfigPath, "cluster_name": s.cfg.ClusterName, "node_id": s.cfg.NodeID, "node_name": s.cfg.NodeName, "listen": s.cfg.Listen, "local_address": s.cfg.LocalAddress, "cql_hosts": s.cfg.Scylla.CQLHosts, "cql_port": s.cfg.Scylla.CQLPort, "internode_port": s.cfg.Scylla.InternodePort, "prometheus_port": s.cfg.Scylla.PrometheusPort, "rest_url": s.cfg.Scylla.RESTURL, "interval": s.cfg.Monitoring.Interval.String(), "timeout": s.cfg.Monitoring.Timeout.String(), "stale_after_seconds": s.cfg.Monitoring.StaleAfter.Seconds(), "history_hours": s.cfg.Monitoring.HistoryHours, "sample_interval": s.cfg.Monitoring.SampleInterval.String(), "failure_window": s.cfg.Monitoring.FailureWindow, "recovery_window": s.cfg.Monitoring.RecoveryWindow, "expected_nodes": s.cfg.Monitoring.ExpectedNodes, "peer_auth_configured": s.cfg.Token != "", "ui_auth_configured": s.cfg.Security.UIToken != "", "console_enabled": s.cfg.Console.Enabled, "allow_user_tables": s.cfg.Console.AllowUserTables, "max_rows": s.cfg.Console.MaxRows, "notifications": map[string]any{"discord": s.cfg.Notifications.DiscordWebhook != "", "generic": s.cfg.Notifications.GenericWebhook != "", "sender_node": s.cfg.Notifications.SenderNode}, "detection_issues": s.cfg.DetectionIssues}
}
func (s *Server) query(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	if !s.cfg.Console.Enabled {
		http.Error(w, "CQL console disabled; explicitly enable console.enabled with a database read-only role", 403)
		return
	}
	var in struct {
		CQL string `json:"cql"`
	}
	if !decode(w, r, &in) {
		return
	}
	q, e := core.ValidateQuery(in.CQL, s.cfg.Console.AllowUserTables)
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	select {
	case s.querySlot <- struct{}{}:
		defer func() { <-s.querySlot }()
	default:
		http.Error(w, "query limit reached", 429)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	v, e := s.mon.Query(ctx, q, s.cfg.Console.MaxRows)
	if e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	_ = s.store.Add(model.Event{Time: time.Now(), Severity: "info", Type: "console_query", Node: s.cfg.NodeID, Message: fmt.Sprintf("Read-only CQL: %d rows (%d ms); query text not logged", v.Count, v.TookMS), Resolved: true})
	writeJSON(w, v)
}
func (s *Server) testNotification(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	s.notifyMu.Lock()
	if time.Since(s.lastNotify) < 30*time.Second {
		s.notifyMu.Unlock()
		http.Error(w, "retry after 30 seconds", 429)
		return
	}
	s.lastNotify = time.Now()
	s.notifyMu.Unlock()
	if e := s.alerts.Test(r.Context()); e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") {
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="scyllascope-diagnostics.json"`)
	data := map[string]any{"version": model.Version, "config": s.publicValues(), "snapshot": s.enriched(), "note": "IP addresses included; passwords and tokens omitted. No DB user rows included."}
	if s.guard.Authenticated(r) {
		data["runtime"] = runtimeStats()
	}
	writeJSON(w, data)
}
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") {
		return
	}
	select {
	case s.sseSlots <- struct{}{}:
		defer func() { <-s.sseSlots }()
	default:
		http.Error(w, "stream limit reached", 429)
		return
	}
	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, "stream unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	ch := s.mon.Subscribe()
	defer s.mon.Unsubscribe(ch)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	rc := http.NewResponseController(w)
	write := func(data string) error {
		_ = rc.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, e := io.WriteString(w, data); e != nil {
			return e
		}
		e := rc.Flush()
		_ = rc.SetWriteDeadline(time.Time{})
		return e
	}
	send := func() error {
		// The collector has already read events/incidents once. Do not read and
		// decode the same SQLite rows again for every viewer on every tick.
		snap := s.mon.Snapshot()
		b, e := json.Marshal(snap)
		if e != nil {
			return e
		}
		return write(fmt.Sprintf("id: %d\nevent: snapshot\ndata: %s\n\n", snap.GeneratedAt.UnixMilli(), b))
	}
	if write("retry: 3000\n\n") != nil || send() != nil {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			if s.cfg.Security.UIToken != "" && !s.guard.Authenticated(r) {
				_ = write("event: auth_expired\ndata: {}\n\n")
				return
			}
			if send() != nil {
				return
			}
		case <-ticker.C:
			if s.cfg.Security.UIToken != "" && !s.guard.Authenticated(r) {
				_ = write("event: auth_expired\ndata: {}\n\n")
				return
			}
			if write(": keepalive\n\n") != nil {
				return
			}
		}
	}
}
