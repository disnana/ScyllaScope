package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"scyllascope/internal/alerts"
	"scyllascope/internal/model"
	"scyllascope/internal/monitor"
	"scyllascope/internal/settings"
	"scyllascope/internal/testkit"
	"strings"
	"testing"
	"time"
)

func setupAPI(t *testing.T) (*Server, *monitor.Monitor, *testkit.Memory) {
	t.Helper()
	c := &settings.Config{ClusterName: "Example Cluster", NodeID: "demo-a", NodeName: "Demo A", Listen: "127.0.0.1:8091", LocalAddress: "100.64.0.1", Token: strings.Repeat("p", 64), Security: settings.SecurityConfig{UIToken: strings.Repeat("u", 64)}, Console: settings.ConsoleConfig{Enabled: true}}
	settings.Defaults(c)
	st := testkit.NewMemory()
	m := monitor.New(c, st, &testkit.Source{}, nil)
	m.Publish(testkit.Snapshot(time.Now()))
	return New(c, m, st, alerts.New(c, st)), m, st
}
func req(h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:8091"+path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	r.Header.Set("X-ScyllaScope-Request", "1")
	r.Header.Set("Origin", "http://127.0.0.1:8091")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestAPIAuthMethodsAndSecrets(t *testing.T) {
	s, _, _ := setupAPI(t)
	s.cfg.Scylla.Password = "TOP-SECRET-CQL"
	s.cfg.Notifications.DiscordWebhook = "https://example.invalid/PRIVATE-WEBHOOK"
	h := s.Handler()
	cases := []struct {
		method, path, token string
		want                int
	}{{"GET", "/api/v1/snapshot", "", 401}, {"GET", "/api/v1/snapshot", s.cfg.Token, 401}, {"GET", "/api/v1/snapshot", s.cfg.Security.UIToken, 200}, {"POST", "/api/v1/snapshot", s.cfg.Security.UIToken, 405}, {"GET", "/api/v1/peer/state", s.cfg.Security.UIToken, 401}, {"GET", "/api/v1/peer/state", s.cfg.Token, 200}, {"GET", "/healthz", "", 200}}
	for _, tc := range cases {
		w := req(h, tc.method, tc.path, "{}", tc.token)
		if w.Code != tc.want {
			t.Fatalf("%s %s got %d want %d: %s", tc.method, tc.path, w.Code, tc.want, w.Body)
		}
	}
	for _, p := range []string{"/api/v1/config", "/api/v1/export"} {
		w := req(h, "GET", p, "", s.cfg.Security.UIToken)
		for _, secret := range []string{s.cfg.Token, s.cfg.Security.UIToken, "TOP-SECRET-CQL", "PRIVATE-WEBHOOK"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("secret leaked", p)
			}
		}
	}
}
func TestAPICSRFAndBodyLimits(t *testing.T) {
	s, _, _ := setupAPI(t)
	h := s.Handler()
	for _, origin := range []string{"https://evil.example", "null"} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:8091/api/v1/login", strings.NewReader(`{"token":"`+s.cfg.Security.UIToken+`"}`))
		r.Header.Set("Origin", origin)
		r.Header.Set("X-ScyllaScope-Request", "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
	for _, body := range []string{`{"cql":"SELECT * FROM system.local;","extra":1}`, `{"cql":"SELECT * FROM system.local;"} {}`, `{"cql":"` + strings.Repeat("a", 40000) + `"}`} {
		w := req(h, "POST", "/api/v1/query", body, s.cfg.Security.UIToken)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body)
		}
	}
}
func TestAPIConsoleIsExplicitAndConstrained(t *testing.T) {
	s, _, _ := setupAPI(t)
	h := s.Handler()
	for _, q := range []string{"DELETE FROM system.local;", "SELECT now() FROM system.local;", "SELECT * FROM private.users;", "SELECT * FROM system.local; TRUNCATE system.local"} {
		b, _ := json.Marshal(map[string]string{"cql": q})
		w := req(h, "POST", "/api/v1/query", string(b), s.cfg.Security.UIToken)
		if w.Code != 400 {
			t.Fatalf("%s: %d", q, w.Code)
		}
	}
	w := req(h, "POST", "/api/v1/query", `{"cql":"SELECT * FROM system.local;"}`, s.cfg.Security.UIToken)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	s.cfg.Console.Enabled = false
	w = req(h, "POST", "/api/v1/query", `{"cql":"SELECT * FROM system.local;"}`, s.cfg.Security.UIToken)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestAPIReadinessAndPeerPrivacy(t *testing.T) {
	s, m, _ := setupAPI(t)
	v := m.Snapshot()
	v.Events = []model.Event{{Message: "private"}}
	v.Incidents = []model.Incident{{Message: "private"}}
	m.Publish(v)
	h := s.Handler()
	w := req(h, "GET", "/api/v1/peer/state", "", s.cfg.Token)
	if strings.Contains(w.Body.String(), "private") {
		t.Fatal("peer leaked event/incident")
	}
	var p model.Snapshot
	json.Unmarshal(w.Body.Bytes(), &p)
	for _, e := range p.Edges {
		if e.From != p.LocalNode {
			t.Fatal("recursive observation forwarding")
		}
	}
	for _, tm := range []time.Time{time.Time{}, time.Now().Add(-time.Hour), time.Now().Add(time.Hour)} {
		v.GeneratedAt = tm
		m.Publish(v)
		if w = req(h, "GET", "/readyz", "", ""); w.Code != 503 {
			t.Fatal(w.Code)
		}
	}
}
func TestAPIAssetsCacheCSPAndNoListing(t *testing.T) {
	s, _, _ := setupAPI(t)
	h := s.Handler()
	w := req(h, "GET", "/", "", "")
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatal(w)
	}
	tag := w.Header().Get("ETag")
	r := httptest.NewRequest("GET", "http://127.0.0.1:8091/", nil)
	r.Header.Set("If-None-Match", tag)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 304 {
		t.Fatal(w.Code)
	}
	for _, p := range []string{"/static/", "/ui/", "/../go.mod", "/unknown.js"} {
		if req(h, "GET", p, "", "").Code == 200 {
			t.Fatal("unexpected static exposure", p)
		}
	}
}
func TestAPIStreamAuthRevocation(t *testing.T) {
	s, m, _ := setupAPI(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	client := &http.Client{Timeout: 5 * time.Second}
	post := func(path string, c *http.Cookie) *http.Response {
		r, _ := http.NewRequest("POST", ts.URL+path, bytes.NewBufferString(`{"token":"`+s.cfg.Security.UIToken+`"}`))
		r.Header.Set("Origin", ts.URL)
		r.Header.Set("X-ScyllaScope-Request", "1")
		if c != nil {
			r.AddCookie(c)
		}
		v, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	login := post("/api/v1/login", nil)
	cookie := login.Cookies()[0]
	login.Body.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/v1/stream", nil)
	r.AddCookie(cookie)
	resp, e := client.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	for {
		line, e := reader.ReadString('\n')
		if e != nil {
			t.Fatal(e)
		}
		if strings.HasPrefix(line, "data:") {
			break
		}
	}
	logout := post("/api/v1/logout", cookie)
	logout.Body.Close()
	v := m.Snapshot()
	v.GeneratedAt = time.Now()
	m.Publish(v)
	b, e := io.ReadAll(reader)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Contains(b, []byte("auth_expired")) {
		t.Fatal(string(b))
	}
}
