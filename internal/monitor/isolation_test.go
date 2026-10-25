//go:build memory.counters

package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
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
	"sync/atomic"
	"testing"
	"time"

	"modernc.org/libc"
)

func isolationEnvInt(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return -1
	}
	return n
}
func isolationProcessRSS(pid int) uint64 {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 2 {
		return 0
	}
	pages, _ := strconv.ParseUint(fields[1], 10, 64)
	return pages * uint64(os.Getpagesize())
}
func isolationProfile(root, name string) error {
	f, err := os.OpenFile(filepath.Join(root, name+".heap.pprof"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	err = pprof.Lookup("heap").WriteTo(f, 0)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// One case per OS process: the parent orchestrator must run cases sequentially.
// During measurement there is no runtime.GC, FreeOSMemory, shrink_memory, DB
// reopen, node replacement, history growth, or fixture identity growth.
func TestIsolationMemory(t *testing.T) {
	if os.Getenv("SCYLLASCOPE_ISOLATION_ENABLE") != "1" {
		t.Skip("opt in with memory.counters and SCYLLASCOPE_ISOLATION_ENABLE=1")
	}
	mode := os.Getenv("SCYLLASCOPE_ISOLATION_MODE")
	allowed := map[string]bool{"idle": true, "sqlite_fixed": true, "sqlite_compact": true, "collector_fixed": true, "json_only": true, "json_sse": true, "notification": true}
	seconds := isolationEnvInt("SCYLLASCOPE_ISOLATION_SECONDS", -1)
	warmup := isolationEnvInt("SCYLLASCOPE_ISOLATION_WARMUP", 10)
	sampleEvery := isolationEnvInt("SCYLLASCOPE_ISOLATION_SAMPLE_SECONDS", 5)
	if !allowed[mode] || seconds < 5 || seconds > 1800 || warmup < 1 || warmup >= seconds || sampleEvery < 5 || sampleEvery > 10 {
		t.Fatal("invalid isolation mode/duration/warmup/sample settings")
	}
	root := os.Getenv("SCYLLASCOPE_ISOLATION_OUTPUT")
	if root == "" {
		t.Fatal("output directory required")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	output, err := os.OpenFile(filepath.Join(root, "samples.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	encoder := json.NewEncoder(output)
	snapshot, repository := isolationFixture(time.Now())
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	fixedBytes := len(encoded)
	encoded = nil
	cfg := &settings.Config{ClusterName: snapshot.ClusterName, NodeID: "isolation-observer", NodeName: "fixed-0", LocalAddress: "127.0.0.1", Listen: "127.0.0.1:0"}
	settings.Defaults(cfg)
	cfg.Monitoring.Interval = 250 * time.Millisecond
	cfg.Monitoring.Timeout = 200 * time.Millisecond
	cfg.Monitoring.MaxNodes = 3
	cfg.Monitoring.ExpectedNodes = 3
	cfg.Monitoring.Concurrency = 3
	cfg.Monitoring.SampleInterval = time.Hour
	cfg.Security.UIToken = isolationToken
	cfg.Notifications.RetryInterval = time.Second
	var mon *Monitor
	var engine *alerts.Engine
	var sqlite *store.Store
	var server *httptest.Server
	var probeCleanup func()
	var probeCount *atomic.Int64
	var receiver *exec.Cmd
	var receiverDone chan error
	receiverWaited := false
	var receiverResult isolationReceiverResult
	var source *isolationSource
	var client *exec.Cmd
	var clientDone chan error
	var clientWaited bool
	var clientExit error
	var progressLast int64
	var progressAt time.Time
	var receiverLast int64
	var receiverAt = time.Now()
	engineCtx, engineCancel := context.WithCancel(context.Background())
	defer engineCancel()
	var engineDone chan struct{}
	cleanupDone := false
	cleanup := func() {
		if cleanupDone {
			return
		}
		cleanupDone = true
		if client != nil && !clientWaited {
			client.Process.Kill()
			<-clientDone
			clientWaited = true
		}
		engineCancel()
		if engineDone != nil {
			<-engineDone
		}
		if server != nil {
			server.Close()
		}
		if receiver != nil && !receiverWaited {
			receiver.Process.Signal(os.Interrupt)
			select {
			case <-receiverDone:
				receiverWaited = true
			case <-time.After(5 * time.Second):
				receiver.Process.Kill()
				<-receiverDone
				receiverWaited = true
			}
		}
		if mon != nil {
			mon.http.CloseIdleConnections()
		}
		if sqlite != nil {
			sqlite.Close()
		}
		if probeCleanup != nil {
			probeCleanup()
		}
	}
	defer cleanup()
	switch mode {
	case "sqlite_fixed", "sqlite_compact":
		sqlite, err = store.Open(filepath.Join(root, "fixed.db"))
		if err != nil {
			t.Fatal(err)
		}
		setupSnapshot := snapshot
		if mode == "sqlite_compact" {
			setupSnapshot = topologyMetadata(snapshot)
		}
		if err = sqlite.SaveTopology(setupSnapshot); err != nil {
			t.Fatal(err)
		}
	case "collector_fixed":
		cfg.DetectionIssues = append([]string{}, snapshot.Warnings...)
		probeCleanup, probeCount = isolationProbeFixtures(t, cfg)
		source = &isolationSource{nodes: append([]model.Node{}, snapshot.Nodes...)}
		mon = New(cfg, repository, source, nil)
	case "json_only":
		mon = New(cfg, nil, &isolationSource{}, nil)
		mon.Publish(snapshot)
	case "json_sse":
		mon = New(cfg, nil, &isolationSource{}, nil)
		mon.Publish(snapshot)
		server = httptest.NewUnstartedServer(nil)
		cfg.Listen = server.Listener.Addr().String()
		server.Config.Handler = api.New(cfg, mon, nil, isolationAlertControl{}).Handler()
		server.Start()
		helperOutput := filepath.Join(root, "client-results.json")
		client = exec.Command(os.Args[0], "-test.run=^TestIsolationExternalClient$", "-test.count=1")
		client.Env = append(os.Environ(), "SCYLLASCOPE_ISOLATION_HELPER=1", "SCYLLASCOPE_ISOLATION_CLIENT_URL="+server.URL, "SCYLLASCOPE_ISOLATION_CLIENT_SECONDS="+strconv.Itoa(seconds), "SCYLLASCOPE_ISOLATION_CLIENT_RESULT="+helperOutput, "SCYLLASCOPE_ISOLATION_CLIENT_GATE="+filepath.Join(root, "client-start"))
		client.Stdout = io.Discard
		client.Stderr = io.Discard
		if err = client.Start(); err != nil {
			t.Fatal(err)
		}
		clientDone = make(chan error, 1)
		go func() { clientDone <- client.Wait() }()
		readyDeadline := time.Now().Add(6 * time.Second)
		for {
			data, readErr := os.ReadFile(helperOutput)
			var ready isolationClientResult
			if readErr == nil && json.Unmarshal(data, &ready) == nil && ready.SSE[0] > 0 && ready.SSE[1] > 0 && ready.Gets > 0 {
				break
			}
			if time.Now().After(readyDeadline) {
				t.Fatal("external client workload not ready")
			}
			time.Sleep(10 * time.Millisecond)
		}
		if err = os.WriteFile(filepath.Join(root, "client-start"), []byte("start\n"), 0600); err != nil {
			t.Fatal(err)
		}
	case "notification":
		receiver = exec.Command(os.Args[0], "-test.run=^TestIsolationReceiverProcess$", "-test.count=1")
		receiver.Env = append(os.Environ(), "SCYLLASCOPE_ISOLATION_RECEIVER_HELPER=1", "SCYLLASCOPE_ISOLATION_RECEIVER_ROOT="+root, "SCYLLASCOPE_ISOLATION_RECEIVER_SECONDS="+strconv.Itoa(seconds+10))
		receiver.Stdout = io.Discard
		receiver.Stderr = io.Discard
		if err = receiver.Start(); err != nil {
			t.Fatal(err)
		}
		receiverDone = make(chan error, 1)
		go func() { receiverDone <- receiver.Wait() }()
		ready := filepath.Join(root, "receiver-ready.json")
		until := time.Now().Add(5 * time.Second)
		for {
			data, readErr := os.ReadFile(ready)
			if readErr == nil {
				cfg.Notifications.AlertmanagerURLs = []string{string(data)}
				break
			}
			if time.Now().After(until) {
				t.Fatal("external receiver not ready")
			}
			time.Sleep(10 * time.Millisecond)
		}

		engine = alerts.New(cfg, repository)
		engineDone = make(chan struct{})
		go func() { defer close(engineDone); engine.Run(engineCtx) }()
	}
	started := time.Now()
	var operations, codecOperations, codecBytes int64
	payloadMin, payloadMax := fixedBytes, fixedBytes
	if mode == "sqlite_compact" {
		data, encodeErr := json.Marshal(topologyMetadata(snapshot))
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		payloadMin = len(data)
		payloadMax = len(data)
	}
	record := func(phase string) error {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		native := libc.MemStat()
		rss := isolationProcessRSS(os.Getpid())
		fds, fdErr := os.ReadDir("/proc/self/fd")
		if fdErr != nil {
			return fdErr
		}
		out := map[string]any{"phase": phase, "mode": mode, "elapsed_seconds": time.Since(started).Seconds(), "warmup_seconds": warmup, "duration_seconds": seconds, "sample_interval_seconds": sampleEvery,
			"go_version": runtime.Version(), "go_sys_bytes": m.Sys, "heap_alloc_bytes": m.HeapAlloc, "heap_inuse_bytes": m.HeapInuse, "heap_idle_bytes": m.HeapIdle, "heap_released_bytes": m.HeapReleased, "stack_inuse_bytes": m.StackInuse, "stack_sys_bytes": m.StackSys, "heap_objects": m.HeapObjects, "num_gc": m.NumGC, "last_gc_unix_nano": m.LastGC, "next_gc_bytes": m.NextGC,
			"allocator_counters_enabled": true, "allocator_live_allocations": native.Allocs, "allocator_mapped_bytes": native.Bytes, "allocator_mappings": native.Mmaps, "rss_bytes": rss, "open_fds": len(fds), "goroutines": runtime.NumGoroutine(),
			"operations": operations, "codec_operations": codecOperations, "codec_bytes": codecBytes, "json_sse_get_hz": 1, "json_sse_sse_clients": 2, "nominal_snapshot_encodes_hz": 9, "configured_hz": 4, "fixed_fixture_nodes": 3, "fixed_fixture_incidents": 500, "fixture_message_bytes": 256, "input_snapshot_json_bytes": fixedBytes, "operation_payload_bytes_min": payloadMin, "operation_payload_bytes_max": payloadMax, "operation_payload_bytes_measured": mode == "sqlite_fixed" || mode == "sqlite_compact" || mode == "json_only", "parent_process_only": true, "heap_alloc_is_live_heap": false, "sampler_allocations_included": true, "measurement_gc_forced": false}
		if m.LastGC > 0 {
			out["natural_gc_age_seconds"] = float64(time.Now().UnixNano()-int64(m.LastGC)) / 1e9
		}
		if phase == "post_stop_gc" {
			out["measurement_gc_forced"] = true
			out["slope_eligible"] = false
		} else {
			out["slope_eligible"] = phase == "sample" && time.Since(started) >= time.Duration(warmup)*time.Second
		}
		if client != nil {
			select {
			case clientExit = <-clientDone:
				clientWaited = true
				if clientExit != nil || time.Since(started) < time.Duration(seconds-1)*time.Second {
					return fmt.Errorf("external client exited early")
				}
			default:
			}
			childRSS := isolationProcessRSS(client.Process.Pid)
			out["external_client_rss_bytes"] = childRSS
			if childRSS > 256<<20 {
				return fmt.Errorf("external client RSS limit exceeded")
			}
			if phase != "post_stop_gc" && !clientWaited && childRSS == 0 && time.Since(started) < time.Duration(seconds-1)*time.Second {
				return fmt.Errorf("external client no longer measurable")
			}
			if data, readErr := os.ReadFile(filepath.Join(root, "client-results.json")); readErr == nil {
				var progress isolationClientResult
				if err := json.Unmarshal(data, &progress); err != nil {
					return err
				}
				out["external_client"] = progress
				total := progress.Gets + progress.SSE[0] + progress.SSE[1]
				if total > progressLast {
					progressAt = time.Now()
					progressLast = total
				}
				if progress.Errors != 0 {
					return fmt.Errorf("external client reported error")
				}
				if phase == "sample" && time.Since(started) > 10*time.Second && time.Since(progressAt) > 10*time.Second {
					return fmt.Errorf("external client workload stalled")
				}
			} else if time.Since(started) > 5*time.Second && phase != "post_stop_gc" {
				return fmt.Errorf("external client progress missing")
			}
		}
		if receiver != nil {
			childRSS := isolationProcessRSS(receiver.Process.Pid)
			out["external_receiver_rss_bytes"] = childRSS
			if childRSS > 256<<20 {
				return fmt.Errorf("external receiver RSS limit exceeded")
			}
			if !cleanupDone {
				select {
				case <-receiverDone:
					receiverWaited = true
					return fmt.Errorf("external receiver exited early")
				default:
				}
			}
		}
		for k, v := range repository.counts() {
			out[k] = v
		}
		if mon != nil {
			mon.mu.RLock()
			out["monitor_subscriptions"] = len(mon.subs)
			mon.mu.RUnlock()
			out["known_nodes"] = len(mon.known)
		}
		if probeCount != nil {
			out["successful_cql_probes"] = probeCount.Load()
		}
		if mode == "sqlite_fixed" || mode == "sqlite_compact" {
			out["sqlite_topology_rows"] = 1
			out["sqlite_history_rows"] = 0
			out["sqlite_incident_rows"] = 0
		}
		if engine != nil {
			stats := engine.Stats()
			out["engine"] = stats
			data, readErr := os.ReadFile(filepath.Join(root, "receiver-status.json"))
			if readErr != nil {
				return readErr
			}
			if err := json.Unmarshal(data, &receiverResult); err != nil {
				return err
			}
			out["notification_external_receiver"] = receiverResult
			if receiverResult.Requests > receiverLast {
				receiverLast = receiverResult.Requests
				receiverAt = time.Now()
			}
			if phase == "sample" && time.Since(started) > 10*time.Second && time.Since(receiverAt) > 10*time.Second {
				return fmt.Errorf("notification work stalled")
			}
			if receiverResult.Errors != 0 {
				return fmt.Errorf("external receiver errors")
			}

			if stats.Queued > 128 || stats.Inflight > 160 || stats.Attempts > 2048 || stats.Tracks > 512 {
				return fmt.Errorf("notification resource bound exceeded: %+v", stats)
			}
		}
		if rss == 0 {
			return fmt.Errorf("RSS measurement unavailable")
		}
		if rss > 256<<20 || native.Bytes > 128<<20 || len(fds) > 128 || runtime.NumGoroutine() > 512 {
			return fmt.Errorf("isolation resource bound exceeded")
		}
		return encoder.Encode(out)
	}
	if err = record("baseline"); err != nil {
		t.Fatal(err)
	}
	var codecTicker *time.Ticker
	var codecChannel <-chan time.Time
	if mode == "json_only" {
		codecTicker = time.NewTicker(time.Second / 9)
		codecChannel = codecTicker.C
		defer codecTicker.Stop()
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	samples := time.NewTicker(time.Duration(sampleEvery) * time.Second)
	defer samples.Stop()
	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	defer timer.Stop()
	warm := time.NewTimer(time.Duration(warmup) * time.Second)
	defer warm.Stop()
	running := true
	for running {
		select {
		case <-timer.C:
			running = false
		case <-warm.C:
			if err = isolationProfile(root, "warmup"); err != nil {
				t.Fatal(err)
			}
			if err = record("warmup_complete"); err != nil {
				t.Fatal(err)
			}
		case <-samples.C:
			if err = record("sample"); err != nil {
				t.Fatal(err)
			}
		case <-codecChannel:
			data, codecErr := json.Marshal(mon.Snapshot())
			if codecErr != nil {
				t.Fatal(codecErr)
			}
			if len(data) < payloadMin {
				payloadMin = len(data)
			}
			if len(data) > payloadMax {
				payloadMax = len(data)
			}
			io.Discard.Write(data)
			codecOperations++
			codecBytes += int64(len(data))
		case <-ticker.C:
			switch mode {
			case "idle":
			case "sqlite_fixed", "sqlite_compact":
				persisted := snapshot
				if mode == "sqlite_compact" {
					persisted = topologyMetadata(snapshot)
				}
				if err = sqlite.SaveTopology(persisted); err == nil {
					var read model.Snapshot
					read, err = sqlite.LoadTopology()
					expectedIncidents := 500
					if mode == "sqlite_compact" {
						expectedIncidents = 0
					}
					if err == nil && (len(read.Nodes) != 3 || len(read.Incidents) != expectedIncidents || read.ClusterName != snapshot.ClusterName) {
						err = fmt.Errorf("fixed SQLite payload counts changed")
					}
				}
			case "collector_fixed":
				if !mon.RefreshNow(context.Background()) {
					err = fmt.Errorf("collector refresh did not execute")
				} else {
					mon.mu.RLock()
					current := mon.snapshot
					mon.mu.RUnlock()
					if len(current.Nodes) != 3 || len(current.Incidents) != 500 || !current.Discovery.OK {
						err = fmt.Errorf("collector fixture counts/discovery changed")
					}
					for _, node := range current.Nodes {
						if !node.CQL.OK || !node.Internode.OK {
							err = fmt.Errorf("loopback probe fixture failed node=%s CQL=%+v Internode=%+v collect_ms=%f", node.Address, node.CQL, node.Internode, current.CollectDurationMS)
						}
					}
				}
			case "json_only":
				snapshot.GeneratedAt = time.Now()
				mon.Publish(snapshot)
			case "json_sse":
				snapshot.GeneratedAt = time.Now()
				mon.Publish(snapshot)
			case "notification":
				snapshot.GeneratedAt = time.Now()
				snapshot.Discovery.CheckedAt = snapshot.GeneratedAt
				engine.Evaluate(engineCtx, snapshot)
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode != "idle" {
				operations++
			}
		}
	}
	if err = record("end"); err != nil {
		t.Fatal(err)
	}
	if err = isolationProfile(root, "end"); err != nil {
		t.Fatal(err)
	}
	if client != nil {
		if !clientWaited {
			select {
			case err = <-clientDone:
				clientWaited = true
				if err != nil {
					t.Fatal("external client failed", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("external client did not finish")
			}
		} else if clientExit != nil {
			t.Fatal(clientExit)
		}
		data, readErr := os.ReadFile(filepath.Join(root, "client-results.json"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		var result isolationClientResult
		if json.Unmarshal(data, &result) != nil || result.Errors != 0 || result.SSE[0] < int64(seconds)*2 || result.SSE[1] < int64(seconds)*2 || result.Gets < int64(seconds/2) {
			t.Fatal("external client did not establish positive workload")
		}
	}
	if mode == "notification" && (receiverResult.Accepted < int64(max(1, seconds/10)*100) || receiverResult.Rejected < int64(max(1, seconds/10)*100)) {
		t.Fatal("notification fixture did not exercise both 503 and 204 phases")
	}
	if mode != "idle" && operations < int64(seconds)*2 {
		t.Fatal("no workload operations")
	}
	if mode == "json_only" && codecOperations < int64(seconds)*5 {
		t.Fatal("codec work fell below minimum rate")
	}
	cleanup()
	runtime.GC() // Diagnostic only, excluded from the steady-state slope.
	if err = record("post_stop_gc"); err != nil {
		t.Fatal(err)
	}
	runtime.KeepAlive(snapshot)
	runtime.KeepAlive(repository)
	runtime.KeepAlive(mon)
	runtime.KeepAlive(engine)
	runtime.KeepAlive(sqlite)
	runtime.KeepAlive(cfg)
	runtime.KeepAlive(source)
	t.Logf("PASS isolation mode=%s measured_seconds=%d operations=%d input_json_bytes=%d", mode, seconds, operations, fixedBytes)
}
