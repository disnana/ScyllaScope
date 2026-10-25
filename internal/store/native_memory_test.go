//go:build memory.counters

package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"scyllascope/internal/model"
	"strconv"
	"strings"
	"testing"
	"time"

	"modernc.org/libc"
)

// Diagnostic only: regular builds do not enable modernc allocator counters.
// MemStat.Bytes counts OS mappings, including retained allocator regions; it
// is not resident memory and must not be subtracted directly from RSS/GoSys.
func TestNativeMemoryDiagnostic(t *testing.T) {
	if os.Getenv("SCYLLASCOPE_NATIVE_MEMORY_DIAGNOSTIC") != "1" {
		t.Skip("opt in with memory.counters tag and SCYLLASCOPE_NATIVE_MEMORY_DIAGNOSTIC=1")
	}
	var store *Store
	record := func(phase string, payloadBytes, operations int) {
		runtime.GC()
		var g runtime.MemStats
		runtime.ReadMemStats(&g)
		native := libc.MemStat()
		out := map[string]any{"phase": phase, "payload_bytes": payloadBytes, "operations": operations,
			"go_version": runtime.Version(), "go_sys_bytes": g.Sys, "heap_alloc_bytes": g.HeapAlloc,
			"heap_inuse_bytes": g.HeapInuse, "heap_released_bytes": g.HeapReleased,
			"allocator_live_allocations": native.Allocs, "allocator_mapped_bytes": native.Bytes, "allocator_mappings": native.Mmaps,
			"allocator_counters_enabled": true}
		if raw, err := os.ReadFile("/proc/self/statm"); err == nil {
			f := strings.Fields(string(raw))
			if len(f) > 1 {
				pages, _ := strconv.ParseUint(f[1], 10, 64)
				out["rss_bytes"] = pages * uint64(os.Getpagesize())
			}
		}
		b, _ := json.Marshal(out)
		t.Logf("NATIVE %s", b)
	}
	record("before_open", 0, 0)
	var err error
	store, err = Open(filepath.Join(t.TempDir(), "native-diagnostic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if store != nil {
			store.Close()
		}
	}()
	record("after_open", 0, 0)
	if libc.MemStat().Bytes == 0 {
		t.Fatal("expected nonzero counters after real SQLite allocation; memory.counters instrumentation unavailable")
	}
	operations := 0
	for _, size := range []int{64 << 10, 96 << 10, 128 << 10, 192 << 10, 256 << 10, 384 << 10, 512 << 10} {
		payload := strings.Repeat("x", size)
		snapshot := model.Snapshot{ClusterName: "synthetic-native-diagnostic", GeneratedAt: time.Now(), Warnings: []string{payload}}
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if err = store.SaveTopology(snapshot); err != nil {
				t.Fatal(err)
			}
			result, readErr := store.LoadTopology()
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(result.Warnings) != 1 || len(result.Warnings[0]) != size {
				t.Fatal("snapshot payload size changed")
			}
			operations++
		}
		record("growing_payload", size, operations)
	}
	// Shrink the live SQL value again. This separates the retained allocation
	// high-water mark from the size of the currently stored JSON document.
	snapshot := model.Snapshot{ClusterName: "synthetic-native-diagnostic", Warnings: []string{strings.Repeat("x", 64<<10)}}
	if err = store.SaveTopology(snapshot); err != nil {
		t.Fatal(err)
	}
	record("payload_shrunk", 64<<10, operations)
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store = nil
	record("after_close_gc", 0, operations)
}
