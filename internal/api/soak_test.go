package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Runs only when explicitly requested. Exercises the real monitor publication,
// HTTP server, authenticated SSE, disconnect/reconnect and snapshot routes.
// Source data is synthetic; no CQL/SQLite adapter or live cluster is exercised.
func TestSoakRuntime(t *testing.T) {
	seconds, _ := strconv.Atoi(os.Getenv("SCYLLASCOPE_SOAK_SECONDS"))
	if seconds < 30 {
		t.Skip("set SCYLLASCOPE_SOAK_SECONDS>=30 for finite runtime test")
	}
	s, m, _ := setupAPI(t)
	handler := s.Handler()
	runtime.GC()
	baseline := runtimeStats()
	t.Logf("BASELINE %s", mustJSON(baseline))
	ts := httptest.NewServer(handler)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	var received, requests, reconnects, published, failures atomic.Int64
	client := &http.Client{Transport: &http.Transport{MaxIdleConns: 32, MaxIdleConnsPerHost: 32}}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/v1/stream", nil)
				req.Header.Set("Authorization", "Bearer "+s.cfg.Security.UIToken)
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
				reconnects.Add(1)
				reader := bufio.NewReader(resp.Body)
				n := 0
				for n < 100 {
					line, e := reader.ReadString('\n')
					if e != nil {
						if ctx.Err() == nil {
							failures.Add(1)
						}
						break
					}
					if strings.HasPrefix(line, "data:") {
						received.Add(1)
						n++
					}
				}
				resp.Body.Close()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/v1/snapshot", nil)
				req.Header.Set("Authorization", "Bearer "+s.cfg.Security.UIToken)
				resp, e := client.Do(req)
				if e != nil {
					if ctx.Err() == nil {
						failures.Add(1)
					}
					continue
				}
				var v map[string]any
				e = json.NewDecoder(resp.Body).Decode(&v)
				resp.Body.Close()
				if resp.StatusCode != 200 || e != nil {
					if ctx.Err() == nil {
						failures.Add(1)
					}
				}
				requests.Add(1)
			}
		}
	}()
	fixture := m.Snapshot()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	samples := time.NewTicker(10 * time.Second)
	defer samples.Stop()
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
		case <-ticker.C:
			fixture.GeneratedAt = time.Now()
			m.Publish(fixture)
			published.Add(1)
		case <-samples.C:
			t.Logf("SAMPLE %s", mustJSON(runtimeStats()))
		}
	}
	wg.Wait()
	client.CloseIdleConnections()
	ts.Close()
	deadline := time.Now().Add(3 * time.Second)
	for len(s.sseSlots) != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	after := runtimeStats()
	t.Logf("AFTER %s", mustJSON(after))
	t.Logf("RESULT seconds=%d published=%d sse_snapshots=%d reconnects=%d snapshot_requests=%d failures=%d active_sse=%d", seconds, published.Load(), received.Load(), reconnects.Load(), requests.Load(), failures.Load(), len(s.sseSlots))
	if failures.Load() != 0 || len(s.sseSlots) != 0 {
		t.Fatal("request failure or SSE slot leak")
	}
	if received.Load() < 1000 || reconnects.Load() < 16 || requests.Load() < 100 {
		t.Fatal("insufficient workload")
	}
	if after["goroutines"].(int) > baseline["goroutines"].(int)+8 {
		t.Fatal("goroutine growth after teardown")
	}
	if b, ok := baseline["open_fds"].(int); ok {
		if after["open_fds"].(int) > b+4 {
			t.Fatal("file descriptor growth after teardown")
		}
	}
	// Small object pool variation is normal; this is not a multi-day leak proof.
	if after["heap_alloc_bytes"].(uint64) > baseline["heap_alloc_bytes"].(uint64)+(8<<20) {
		t.Fatal("large retained heap growth after teardown")
	}
}
func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
