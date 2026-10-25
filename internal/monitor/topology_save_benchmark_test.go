//go:build memory.counters

package monitor

import (
	"encoding/json"
	"path/filepath"
	"scyllascope/internal/store"
	"testing"
	"time"
)

// BenchmarkTopologySaveOnly models the production persistence cadence: load
// cached topology once at startup, then only save on subsequent collections.
// It measures per-save allocation and stored payload size, not RSS or a
// week-scale leak. Both variants retain the same fixed 3-node/500-incident
// input; compact saves include topologyMetadata's caller allocation each time.
// There is no explicit GC, trim, cache clear, or database reopen in this code.
// The standard Go benchmark framework still controls its own GC/calibration.
func BenchmarkTopologySaveOnly(b *testing.B) {
	for _, variant := range []struct {
		name    string
		compact bool
	}{{"full", false}, {"compact", true}} {
		b.Run(variant.name, func(b *testing.B) {
			snapshot, _ := isolationFixture(time.Now())
			if len(snapshot.Nodes) != 3 || len(snapshot.Incidents) != 500 {
				b.Fatal("fixed topology benchmark fixture changed")
			}
			inputJSON, err := json.Marshal(snapshot)
			if err != nil {
				b.Fatal(err)
			}
			inputBytes := len(inputJSON)
			initial := snapshot
			if variant.compact {
				initial = topologyMetadata(snapshot)
			}
			storedJSON, err := json.Marshal(initial)
			if err != nil {
				b.Fatal(err)
			}
			storedBytes := len(storedJSON)
			st, err := store.Open(filepath.Join(b.TempDir(), "topology.db"))
			if err != nil {
				b.Fatal(err)
			}
			defer st.Close()
			// Provision a real stored cache, then exercise the startup read once.
			// Setup, this first write, and the sole LoadTopology are unmeasured.
			if err = st.SaveTopology(initial); err != nil {
				b.Fatal(err)
			}
			startup, err := st.LoadTopology()
			if err != nil || startup.ClusterName != snapshot.ClusterName || len(startup.Nodes) != 3 {
				b.Fatal("startup topology load failed", err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(storedBytes))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				persisted := snapshot
				if variant.compact {
					// This allocation is deliberately inside the timed loop.
					persisted = topologyMetadata(snapshot)
				}
				// Production loads only at startup; never read back in this loop.
				if err := st.SaveTopology(persisted); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(inputBytes), "input-B")
			b.ReportMetric(float64(storedBytes), "payload-B")
			b.ReportMetric(1, "startup-loads")
		})
	}
}
