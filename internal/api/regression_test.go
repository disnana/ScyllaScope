package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"scyllascope/internal/model"
	"scyllascope/internal/repository"
	"scyllascope/internal/security"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type countingRepo struct {
	repository.Repository
	calls atomic.Int64
}

func (c *countingRepo) Recent(n int) ([]model.Event, error) {
	c.calls.Add(1)
	return c.Repository.Recent(n)
}
func (c *countingRepo) Incidents(all bool) ([]model.Incident, error) {
	c.calls.Add(1)
	return c.Repository.Incidents(all)
}
func TestSSEDoesNotRequeryHistoryPerViewer(t *testing.T) {
	s, m, st := setupAPI(t)
	r := &countingRepo{Repository: st}
	s.store = r
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/v1/stream", nil)
	req.Header.Set("Authorization", "Bearer "+s.cfg.Security.UIToken)
	resp, e := ts.Client().Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	for round := 0; round < 10; round++ {
		for {
			line, e := reader.ReadString('\n')
			if e != nil {
				t.Fatal(e)
			}
			if strings.HasPrefix(line, "data:") {
				break
			}
		}
		v := m.Snapshot()
		v.GeneratedAt = time.Now()
		m.Publish(v)
	}
	if r.calls.Load() != 0 {
		t.Fatalf("SSE repeated %d history reads", r.calls.Load())
	}
	cancel()
	io.Copy(io.Discard, reader)
}
func TestGeneralRequestSlotsRecoverAndDoNotBlockHealth(t *testing.T) {
	s, _, _ := setupAPI(t)
	h := s.Handler()
	for i := 0; i < cap(s.workSlots); i++ {
		s.workSlots <- struct{}{}
	}
	w := req(h, "GET", "/api/v1/config", "", s.cfg.Security.UIToken)
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	if req(h, "GET", "/healthz", "", "").Code != 200 {
		t.Fatal("health blocked")
	}
	<-s.workSlots
	if req(h, "GET", "/api/v1/config", "", s.cfg.Security.UIToken).Code != 200 {
		t.Fatal("slot not reusable")
	}
}
func TestRuntimeTelemetryAuthenticatedAndFinite(t *testing.T) {
	s, _, _ := setupAPI(t)
	h := s.Handler()
	if req(h, "GET", "/api/v1/runtime", "", "").Code != 401 {
		t.Fatal("runtime leaked")
	}
	w := req(h, "GET", "/api/v1/runtime", "", s.cfg.Security.UIToken)
	var data map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &data) != nil || data["goroutines"].(float64) < 1 {
		t.Fatal(w.Code, w.Body)
	}
}

func TestReadOnlyExportDoesNotBypassRuntimeAuthentication(t *testing.T) {
	s, _, _ := setupAPI(t)
	s.cfg.Security.UIToken = ""
	s.guard = security.New("", s.cfg.Listen, nil)
	h := s.Handler()
	w := req(h, "GET", "/api/v1/export", "", "")
	var data map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &data) != nil {
		t.Fatal(w.Code, w.Body)
	}
	if _, ok := data["runtime"]; ok {
		t.Fatal("anonymous export bypassed runtime authentication")
	}
	if req(h, "GET", "/api/v1/runtime", "", "").Code != 401 {
		t.Fatal("anonymous runtime admitted")
	}
}
