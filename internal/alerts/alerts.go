package alerts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"scyllascope/internal/model"
	"scyllascope/internal/probe"
	"scyllascope/internal/repository"
	"scyllascope/internal/settings"
	"strings"
	"sync"
	"time"
)

type signal struct{ key, node, kind, severity, message string }
type track struct{ bad, good int }
type job struct {
	incident model.Incident
	recovery bool
}
type Engine struct {
	cfg          *settings.Config
	store        repository.Repository
	mu           sync.Mutex
	tracks       map[string]*track
	lastSample   time.Time
	notifyCursor string
	inflight     map[string]bool
	lastAttempt  map[string]time.Time
	queue        chan job
	http         *http.Client
}

func New(c *settings.Config, s repository.Repository) *Engine {
	return &Engine{cfg: c, store: s, tracks: map[string]*track{}, inflight: map[string]bool{}, lastAttempt: map[string]time.Time{}, queue: make(chan job, 128), http: probe.Client(5 * time.Second)}
}
func fingerprint(observer, kind, node string) string {
	sum := sha256.Sum256([]byte(observer + "|" + kind + "|" + node))
	return hex.EncodeToString(sum[:12])
}
func (e *Engine) Evaluate(ctx context.Context, s model.Snapshot) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.lastAttempt) >= 2048 {
		for id, at := range e.lastAttempt {
			if now := s.GeneratedAt; now.Sub(at) >= e.cfg.Notifications.RetryInterval && !e.inflight[id] {
				delete(e.lastAttempt, id)
			}
		}
	}
	if s.GeneratedAt.IsZero() || !s.GeneratedAt.After(e.lastSample) {
		return
	}
	e.lastSample = s.GeneratedAt
	now := s.GeneratedAt
	signals := map[string]signal{}
	add := func(node, kind, sev, msg string) {
		id := fingerprint(e.cfg.NodeID, kind, node)
		signals[id] = signal{id, node, kind, sev, msg}
	}
	if !s.Discovery.Fresh(now, e.cfg.Monitoring.StaleAfter) || !s.Discovery.OK {
		add(e.cfg.LocalAddress, "discovery", "warning", "クラスタ一覧を取得できません: "+s.Discovery.Message)
	}
	if s.Discovery.OK && s.Coverage.DiscoveredNodes < s.Coverage.ExpectedNodes {
		add(e.cfg.LocalAddress, "membership", "warning", fmt.Sprintf("Scyllaメンバー %d / 期待 %d", s.Coverage.DiscoveredNodes, s.Coverage.ExpectedNodes))
	}
	for _, n := range s.Nodes {
		if e.cfg.Token != "" && n.Address != e.cfg.LocalAddress && n.WatchAPI.Fresh(now, e.cfg.Monitoring.StaleAfter) && !n.WatchAPI.OK {
			add(n.Address, "peer_unavailable", "warning", n.Name+": 観測エージェントに接続できません: "+n.WatchAPI.Message)
		}
		if n.State == model.Critical || n.State == model.Warning {
			if len(e.cfg.Notifications.AlertmanagerURLs) == 0 {
				add(n.Address, "node_health", string(n.State), n.Name+": "+strings.Join(n.Reasons, " / "))
			} else {
				for _, condition := range healthConditions(n, now, e.cfg.Monitoring.StaleAfter) {
					add(n.Address, "node_health/"+condition, string(n.State), n.Name+": "+condition)
				}
			}
		}
	}
	for _, edge := range s.Edges {
		if edge.From == s.LocalNode && edge.Asymmetric && !edge.Stale {
			add(edge.To, "asymmetric", "warning", "方向別の接続結果が不一致: "+edge.From+" → "+edge.To)
		}
	}
	existing := map[string]model.Incident{}
	var err error
	boundary := e.notifyCursor
	for pass := 0; pass < 2; pass++ {
		cursor := ""
		if pass == 0 {
			cursor = boundary
		}
		for {
			page, readErr := e.store.WorkIncidents(cursor, 256)
			if readErr != nil {
				return
			}
			if len(page) == 0 {
				break
			}
			if pass == 1 && (boundary == "" || page[0].ID > boundary) {
				break
			}
			for _, v := range page {
				cursor = v.ID
				if pass == 1 && (boundary == "" || v.ID > boundary) {
					break
				}
				if v.Observer != e.cfg.NodeID {
					continue
				}
				if v.ResolvedAt == nil {
					existing[fingerprint(v.Observer, v.Kind, v.Node)] = v
				} else if v.NotificationPending && v.LastNotifiedAt == nil {
					e.enqueue(v, false, now)
				} else if v.RecoveryPending {
					e.enqueue(v, true, now)
				}
			}
		}
	}
	keys := map[string]bool{}
	for k := range signals {
		keys[k] = true
	}
	for k := range existing {
		keys[k] = true
	}
	for k := range keys {
		t := e.tracks[k]
		if t == nil {
			t = &track{}
			e.tracks[k] = t
		}
		v, open := existing[k]
		sig, bad := signals[k]
		if bad {
			t.good = 0
			t.bad = min(t.bad+1, e.cfg.Monitoring.FailureWindow)
			if !open && t.bad < e.cfg.Monitoring.FailureWindow {
				continue
			}
			if !open {
				v = model.Incident{ID: fingerprint(e.cfg.NodeID+"@"+now.UTC().Format(time.RFC3339Nano), sig.kind, sig.node), Observer: e.cfg.NodeID, Node: sig.node, Kind: sig.kind, Severity: sig.severity, Message: sig.message, OpenedAt: now, UpdatedAt: now, NotificationPending: e.enabled()}
				if err = e.store.PutIncident(v); err != nil {
					continue
				}
				_ = e.store.Add(model.Event{Time: now, Severity: sig.severity, Type: "incident_opened", Node: sig.node, Message: "[" + e.cfg.NodeName + "] " + sig.message})
			} else {
				v.UpdatedAt = now
				v.Message = sig.message
				v.Severity = sig.severity
				_ = e.store.PutIncident(v)
			}
			e.enqueue(v, false, now)
		} else if open {
			// Loss of evidence is not recovery. Keep an incident open until the
			// same node / directed edge is demonstrably healthy again.
			knownGood := s.Discovery.Fresh(now, e.cfg.Monitoring.StaleAfter) && s.Discovery.OK
			if strings.HasPrefix(v.Kind, "node_health/") {
				knownGood = false
				for _, n := range s.Nodes {
					if n.Address == v.Node {
						knownGood = conditionRecovered(n, strings.TrimPrefix(v.Kind, "node_health/"), now, e.cfg.Monitoring.StaleAfter)
					}
				}
			}
			if v.Kind == "node_health" {
				knownGood = false
				for _, n := range s.Nodes {
					if n.Address == v.Node && n.State == model.Healthy {
						knownGood = true
					}
				}
			}
			if v.Kind == "peer_unavailable" {
				knownGood = false
				for _, n := range s.Nodes {
					if n.Address == v.Node && n.WatchAPI.Fresh(now, e.cfg.Monitoring.StaleAfter) && n.WatchAPI.OK {
						knownGood = true
					}
				}
			}
			if v.Kind == "membership" {
				knownGood = s.Discovery.Fresh(now, e.cfg.Monitoring.StaleAfter) && s.Discovery.OK && s.Coverage.DiscoveredNodes >= s.Coverage.ExpectedNodes
			}
			if v.Kind == "asymmetric" {
				knownGood = asymmetricRecovered(s, v.Node, now, e.cfg.Monitoring.StaleAfter)
			}
			if !knownGood {
				// Unavailable/retired identities must not accumulate counters forever.
				// The incident remains durable; only consecutive evidence is reset.
				delete(e.tracks, k)
				t.good = 0
				continue
			}
			t.bad = 0
			t.good = min(t.good+1, e.cfg.Monitoring.RecoveryWindow)
			if t.good < e.cfg.Monitoring.RecoveryWindow {
				continue
			}
			v.ResolvedAt = &now
			v.RecoveryPending = v.LastNotifiedAt != nil && !v.Acknowledged
			v.UpdatedAt = now
			if e.store.PutIncident(v) != nil {
				continue
			}
			_ = e.store.Add(model.Event{Time: now, Severity: "info", Type: "incident_resolved", Node: v.Node, Message: "[" + e.cfg.NodeName + "] 復旧: " + v.Message, Resolved: true})
			e.enqueue(v, true, now)
			delete(e.tracks, k)
		}
	}
	// Retire pending signals that recovered before their confirmation window.
	for k := range e.tracks {
		if !keys[k] {
			delete(e.tracks, k)
		}
	}
}

// asymmetricRecovered returns true only when a fresh cluster view proves that an
// asymmetric incident is no longer applicable. There are two recovery cases:
//
//  1. Both directed observations still exist and are fresh, but now agree.
//     They may agree on success *or failure*; a symmetric outage is a different
//     problem and must not keep an asymmetry incident open forever.
//  2. The incident points at a node identity that no longer exists in a fresh
//     discovery result. This happens when a node is replaced/re-bootstrapped and
//     receives a new Scylla host_id. A discovery failure never counts as proof.
func asymmetricRecovered(s model.Snapshot, target string, now time.Time, ttl time.Duration) bool {
	if !s.Discovery.Fresh(now, ttl) || !s.Discovery.OK {
		return false
	}
	present := false
	for _, n := range s.Nodes {
		if n.ID == target {
			present = true
			break
		}
	}
	if !present {
		return true
	}
	var forward, reverse *model.Edge
	for i := range s.Edges {
		edge := &s.Edges[i]
		if edge.From == s.LocalNode && edge.To == target {
			forward = edge
		}
		if edge.From == target && edge.To == s.LocalNode {
			reverse = edge
		}
	}
	if forward == nil || reverse == nil || forward.Stale || reverse.Stale {
		return false
	}
	// Require fresh evidence for both protocols in both directions so that a
	// missing/unknown check cannot be mistaken for recovery.
	checks := []model.Check{forward.CQL, forward.Internode, reverse.CQL, reverse.Internode}
	for _, c := range checks {
		if !c.Fresh(now, ttl) {
			return false
		}
	}
	return !forward.Asymmetric && !reverse.Asymmetric
}

func (e *Engine) enabled() bool {
	if len(e.cfg.Notifications.AlertmanagerURLs) > 0 {
		return true
	}
	return (e.cfg.Notifications.DiscordWebhook != "" || e.cfg.Notifications.GenericWebhook != "") && (e.cfg.Notifications.SenderNode == "" || e.cfg.Notifications.SenderNode == e.cfg.NodeID || e.cfg.Notifications.SenderNode == e.cfg.LocalAddress)
}
func (e *Engine) enqueue(v model.Incident, recovery bool, now time.Time) {
	if !e.enabled() || e.inflight[v.ID] || v.Acknowledged || (v.SilencedUntil != nil && now.Before(*v.SilencedUntil)) {
		return
	}
	if !recovery && v.LastNotifiedAt != nil && len(e.cfg.Notifications.AlertmanagerURLs) == 0 {
		return
	}
	if now.Sub(e.lastAttempt[v.ID]) < e.cfg.Notifications.RetryInterval {
		return
	}
	if recovery && v.LastNotifiedAt == nil {
		return
	}
	if _, exists := e.lastAttempt[v.ID]; !exists && len(e.lastAttempt) >= 2048 {
		return
	}
	select {
	case e.queue <- job{v, recovery}:
		e.notifyCursor = v.ID
		e.inflight[v.ID] = true
		e.lastAttempt[v.ID] = now
	default:
	}
}
func (e *Engine) Run(ctx context.Context) {
	defer e.http.CloseIdleConnections()
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-e.queue:
			e.deliver(ctx, j)
		}
	}
}
func (e *Engine) deliver(ctx context.Context, j job) {
	current, lookupErr := e.store.Incident(j.incident.ID)
	if lookupErr != nil || !current.OpenedAt.Equal(j.incident.OpenedAt) || current.Acknowledged ||
		(current.SilencedUntil != nil && time.Now().Before(*current.SilencedUntil)) ||
		(!j.recovery && current.LastNotifiedAt != nil && len(e.cfg.Notifications.AlertmanagerURLs) == 0) || (j.recovery && !current.RecoveryPending) {
		e.mu.Lock()
		delete(e.inflight, j.incident.ID)
		e.mu.Unlock()
		return
	}
	j.incident = current
	err := e.send(ctx, j.incident, j.recovery)
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.inflight, j.incident.ID)
	v, readErr := e.store.Incident(j.incident.ID)
	if readErr == nil && v.OpenedAt.Equal(j.incident.OpenedAt) {
		if err != nil {
			v.DeliveryError = err.Error()
		} else {
			v.DeliveryError = ""
			now := time.Now()
			if !j.recovery {
				v.LastNotifiedAt = &now
				v.NotificationPending = false
				if v.ResolvedAt != nil {
					v.RecoveryPending = true
				}
			} else {
				v.RecoveryNotifiedAt = &now
				v.RecoveryPending = false
			}
		}
		_ = e.store.PutIncident(v)
	}
	if err != nil {
		_ = e.store.Add(model.Event{Time: time.Now(), Severity: "warning", Type: "notification_failed", Node: j.incident.Node, Message: err.Error(), Resolved: true})
	}
}
func (e *Engine) send(ctx context.Context, v model.Incident, recovery bool) error {
	if len(e.cfg.Notifications.AlertmanagerURLs) > 0 {
		return e.sendAlertmanager(ctx, v, recovery)
	}
	prefix := "⚠️"
	if v.Severity == "critical" {
		prefix = "🔴"
	}
	if recovery {
		prefix = "✅ 復旧"
	}
	msg := prefix + " **ScyllaScope** · " + e.cfg.ClusterName + "\n観測元: " + e.cfg.NodeName + "\n" + v.Message
	if runes := []rune(msg); len(runes) > 1700 {
		msg = string(runes[:1700])
	}
	errs := []string{}
	if u := e.cfg.Notifications.DiscordWebhook; u != "" {
		b, _ := json.Marshal(map[string]any{"content": msg, "allowed_mentions": map[string]any{"parse": []string{}}})
		if err := e.post(ctx, u, b); err != nil {
			errs = append(errs, "Discord: "+err.Error())
		}
	}
	if u := e.cfg.Notifications.GenericWebhook; u != "" {
		b, _ := json.Marshal(map[string]any{"cluster": e.cfg.ClusterName, "observer": e.cfg.NodeID, "recovery": recovery, "delivery_id": v.ID + fmt.Sprintf(":%t", recovery), "incident": v})
		if err := e.post(ctx, u, b); err != nil {
			errs = append(errs, "Webhook: "+err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}
func (e *Engine) post(ctx context.Context, u string, b []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("invalid webhook")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return fmt.Errorf("delivery failed (connection / TLS / timeout)")
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}
func (e *Engine) Test(ctx context.Context) error {
	if len(e.cfg.Notifications.AlertmanagerURLs) == 0 && e.cfg.Notifications.DiscordWebhook == "" && e.cfg.Notifications.GenericWebhook == "" {
		return fmt.Errorf("通知先が設定されていません")
	}
	return e.send(ctx, model.Incident{Message: "テスト通知: この通知は手動操作で送信されました", Observer: e.cfg.NodeID, OpenedAt: time.Now()}, false)
}
func (e *Engine) Action(id string, ack bool, until *time.Time) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.store.UpdateIncidentAction(id, ack, until)
}
