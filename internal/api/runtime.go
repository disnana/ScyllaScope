package api

import (
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Values are bytes, not MiB. RSS is current resident memory, not virtual size.
// Missing OS-specific values are omitted instead of fabricated as zero.
func runtimeStats() map[string]any {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	out := map[string]any{"sampled_at": time.Now().UTC(), "go_version": runtime.Version(),
		"goroutines": runtime.NumGoroutine(), "heap_alloc_bytes": m.HeapAlloc,
		"heap_inuse_bytes": m.HeapInuse, "heap_idle_bytes": m.HeapIdle,
		"heap_released_bytes": m.HeapReleased, "go_sys_bytes": m.Sys,
		"gc_cycles": m.NumGC, "gc_pause_total_ns": m.PauseTotalNs}
	if raw, e := os.ReadFile("/proc/self/status"); e == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			f := strings.Fields(line)
			if len(f) == 3 && (f[0] == "VmRSS:" || f[0] == "VmHWM:") && f[2] == "kB" {
				if n, e := strconv.ParseUint(f[1], 10, 64); e == nil {
					key := "rss_bytes"
					if f[0] == "VmHWM:" {
						key = "peak_rss_bytes"
					}
					out[key] = n * 1024
				}
			}
		}
	}
	if f, e := os.ReadDir("/proc/self/fd"); e == nil {
		out["open_fds"] = len(f)
	}
	return out
}
func (s *Server) runtimeInfo(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") {
		return
	}
	// Runtime diagnostics require a UI token even in anonymous read-only mode.
	if !s.guard.Authenticated(r) {
		http.Error(w, "authentication required", 401)
		return
	}
	writeJSON(w, runtimeStats())
}
