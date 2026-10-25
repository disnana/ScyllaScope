package monitor

import (
	"scyllascope/internal/settings"
	"scyllascope/internal/testkit"
	"testing"
	"time"
)

// Identical test file is also run against the unchanged v0.6.2 release.
func BenchmarkPublish(b *testing.B) {
	for _, tc := range []struct {
		name    string
		viewers int
	}{{"no_viewers", 0}, {"32_slow_viewers", 32}} {
		b.Run(tc.name, func(b *testing.B) {
			c := &settings.Config{ClusterName: "bench", NodeID: "bench", LocalAddress: "127.0.0.1"}
			settings.Defaults(c)
			m := New(c, nil, nil, nil)
			defer m.http.CloseIdleConnections()
			for i := 0; i < tc.viewers; i++ {
				ch := m.Subscribe()
				defer m.Unsubscribe(ch)
			}
			s := testkit.Snapshot(time.Now())
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m.Publish(s)
			}
		})
	}
}
func BenchmarkSnapshot(b *testing.B) {
	c := &settings.Config{ClusterName: "bench", NodeID: "bench", LocalAddress: "127.0.0.1"}
	settings.Defaults(c)
	m := New(c, nil, nil, nil)
	defer m.http.CloseIdleConnections()
	m.Publish(testkit.Snapshot(time.Now()))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Snapshot()
	}
}
