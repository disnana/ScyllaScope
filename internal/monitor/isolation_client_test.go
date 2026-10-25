//go:build memory.counters

package monitor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

const isolationToken = "synthetic-isolation-token-00000000000000000000000000000000"

type isolationClientResult struct {
	SSE        [2]int64 `json:"sse_snapshots"`
	Gets       int64    `json:"snapshot_gets"`
	Bytes      int64    `json:"received_bytes"`
	Errors     int64    `json:"errors"`
	PayloadMin int64    `json:"payload_min"`
	PayloadMax int64    `json:"payload_max"`
}

func TestIsolationExternalClient(t *testing.T) {
	if os.Getenv("SCYLLASCOPE_ISOLATION_HELPER") != "1" {
		t.Skip("isolated external client helper only")
	}
	raw := os.Getenv("SCYLLASCOPE_ISOLATION_CLIENT_URL")
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Path != "" || !net.ParseIP(u.Hostname()).IsLoopback() {
		t.Fatal("helper requires literal loopback HTTP origin")
	}
	seconds, err := strconv.Atoi(os.Getenv("SCYLLASCOPE_ISOLATION_CLIENT_SECONDS"))
	if err != nil || seconds < 5 || seconds > 1800 {
		t.Fatal("invalid helper duration")
	}
	out := os.Getenv("SCYLLASCOPE_ISOLATION_CLIENT_RESULT")
	if out == "" {
		t.Fatal("missing helper result path")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gate := os.Getenv("SCYLLASCOPE_ISOLATION_CLIENT_GATE")
	if gate == "" {
		t.Fatal("client measurement gate missing")
	}
	go func() {
		hard := time.NewTimer(time.Duration(seconds+10) * time.Second)
		defer hard.Stop()
		poll := time.NewTicker(10 * time.Millisecond)
		defer poll.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-hard.C:
				cancel()
				return
			case <-poll.C:
				if _, err := os.Stat(gate); err == nil {
					timer := time.NewTimer(time.Duration(seconds) * time.Second)
					defer timer.Stop()
					select {
					case <-timer.C:
						cancel()
					case <-hard.C:
						cancel()
					case <-ctx.Done():
					}
					return
				}
			}
		}
	}()
	client := &http.Client{Transport: &http.Transport{Proxy: nil, MaxConnsPerHost: 4, MaxIdleConns: 4, MaxIdleConnsPerHost: 4, IdleConnTimeout: time.Minute}}
	var counts [2]atomic.Int64
	var gets, bytesRead, errors, payloadMin, payloadMax atomic.Int64
	countPayload := func(n int64) {
		for {
			old := payloadMin.Load()
			if old != 0 && old <= n {
				break
			}
			if payloadMin.CompareAndSwap(old, n) {
				break
			}
		}
		for {
			old := payloadMax.Load()
			if old >= n || payloadMax.CompareAndSwap(old, n) {
				break
			}
		}
	}
	var wg sync.WaitGroup
	request := func(path string) (*http.Response, error) {
		req, _ := http.NewRequestWithContext(ctx, "GET", raw+path, nil)
		req.Header.Set("Authorization", "Bearer "+isolationToken)
		return client.Do(req)
	}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			response, err := request("/api/v1/stream")
			if err != nil {
				if ctx.Err() == nil {
					errors.Add(1)
				}
				return
			}
			defer response.Body.Close()
			if response.StatusCode != 200 {
				errors.Add(1)
				return
			}
			scanner := bufio.NewScanner(response.Body)
			scanner.Buffer(make([]byte, 64<<10), 1<<20)
			for scanner.Scan() {
				if ctx.Err() != nil {
					return
				}
				line := scanner.Bytes()
				if strings.HasPrefix(string(line), "data:") {
					var v struct {
						Incidents []json.RawMessage `json:"incidents"`
					}
					if err := json.Unmarshal(line[len("data:"):], &v); err != nil || len(v.Incidents) != 500 {
						errors.Add(1)
						return
					}
					countPayload(int64(len(bytes.TrimSpace(line[len("data:"):]))))
					bytesRead.Add(int64(len(line)))
					counts[i].Add(1)
				}
			}
			if scanner.Err() != nil && ctx.Err() == nil {
				errors.Add(1)
			}
		}(i)
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
				response, err := request("/api/v1/snapshot")
				if err != nil {
					if ctx.Err() == nil {
						errors.Add(1)
					}
					continue
				}
				if response.StatusCode != 200 {
					errors.Add(1)
					response.Body.Close()
					continue
				}
				data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
				response.Body.Close()
				if err != nil {
					if ctx.Err() == nil {
						errors.Add(1)
					}
					continue
				}
				var v struct {
					Incidents []json.RawMessage `json:"incidents"`
				}
				if json.Unmarshal(data, &v) != nil || len(v.Incidents) != 500 {
					if ctx.Err() == nil {
						errors.Add(1)
					}
					continue
				}
				countPayload(int64(len(bytes.TrimSpace(data))))
				gets.Add(1)
				bytesRead.Add(int64(len(data)))
			}
		}
	}()
	progressDone := make(chan struct{})
	go func() {
		defer close(progressDone)
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				v := isolationClientResult{SSE: [2]int64{counts[0].Load(), counts[1].Load()}, Gets: gets.Load(), Bytes: bytesRead.Load(), Errors: errors.Load(), PayloadMin: payloadMin.Load(), PayloadMax: payloadMax.Load()}
				data, _ := json.Marshal(v)
				temp := out + ".tmp"
				if err := os.WriteFile(temp, data, 0600); err != nil {
					errors.Add(1)
					continue
				}
				if err := os.Rename(temp, out); err != nil {
					errors.Add(1)
				}
			}
		}
	}()
	wg.Wait()
	<-progressDone
	client.CloseIdleConnections()
	result := isolationClientResult{SSE: [2]int64{counts[0].Load(), counts[1].Load()}, Gets: gets.Load(), Bytes: bytesRead.Load(), Errors: errors.Load(), PayloadMin: payloadMin.Load(), PayloadMax: payloadMax.Load()}
	data, _ := json.Marshal(result)
	if err := os.WriteFile(out+".tmp", append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(out+".tmp", out); err != nil {
		t.Fatal(err)
	}
	if result.Errors != 0 || result.SSE[0] < 1 || result.SSE[1] < 1 || result.Gets < 1 {
		t.Fatal(fmt.Sprintf("external client failed: %+v", result))
	}
}

type isolationReceiverResult struct {
	Requests   int64 `json:"requests"`
	Accepted   int64 `json:"accepted"`
	Rejected   int64 `json:"rejected"`
	Bytes      int64 `json:"bytes"`
	PayloadMin int64 `json:"payload_min"`
	PayloadMax int64 `json:"payload_max"`
	Errors     int64 `json:"errors"`
}

func TestIsolationReceiverProcess(t *testing.T) {
	if os.Getenv("SCYLLASCOPE_ISOLATION_RECEIVER_HELPER") != "1" {
		t.Skip("external receiver helper only")
	}
	root := os.Getenv("SCYLLASCOPE_ISOLATION_RECEIVER_ROOT")
	seconds := isolationEnvInt("SCYLLASCOPE_ISOLATION_RECEIVER_SECONDS", -1)
	if root == "" || seconds < 5 || seconds > 1810 {
		t.Fatal("invalid external receiver settings")
	}
	var mu sync.Mutex
	var result isolationReceiverResult
	started := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, err := io.Copy(io.Discard, r.Body)
		mu.Lock()
		result.Requests++
		result.Bytes += n
		if result.PayloadMin == 0 || n < result.PayloadMin {
			result.PayloadMin = n
		}
		if n > result.PayloadMax {
			result.PayloadMax = n
		}
		if err != nil && r.Context().Err() == nil {
			result.Errors++
		}
		failed := int(time.Since(started)/time.Second)%10 < 5
		if failed {
			result.Rejected++
		} else {
			result.Accepted++
		}
		mu.Unlock()
		if failed {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(204)
		}
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	if !net.ParseIP(parsed.Hostname()).IsLoopback() {
		t.Fatal("receiver must bind loopback")
	}
	write := func() error {
		mu.Lock()
		data, err := json.Marshal(result)
		mu.Unlock()
		if err != nil {
			return err
		}
		temp := filepath.Join(root, "receiver-status.tmp")
		if err = os.WriteFile(temp, data, 0600); err != nil {
			return err
		}
		return os.Rename(temp, filepath.Join(root, "receiver-status.json"))
	}
	if err := write(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "receiver-ready.json"), []byte(server.URL), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	timeout := time.NewTimer(time.Duration(seconds) * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if err := write(); err != nil {
				t.Fatal(err)
			}
			return
		case <-timeout.C:
			if err := write(); err != nil {
				t.Fatal(err)
			}
			return
		case <-ticker.C:
			if err := write(); err != nil {
				t.Fatal(err)
			}
		}
	}
}
