//go:build memory.counters

package monitor

import (
	"fmt"
	"scyllascope/internal/model"
	"strings"
	"testing"
	"time"
)

// Six fixed slots avoid retaining a growing failure history in a memory test.
// The original error text is intentionally absent from this public schema.
type isolationProbeFailure struct {
	Phase          string    `json:"phase"`
	AttemptedAtUTC time.Time `json:"attempted_at_utc"`
	CheckedAtUTC   time.Time `json:"checked_at_utc"`
	LatencyNS      int64     `json:"latency_ns"`
}
type isolationProbeHealthSlot struct {
	NodeIndex    int                    `json:"node_index"`
	Protocol     string                 `json:"protocol"`
	Observations int64                  `json:"observations"`
	Failures     int64                  `json:"failures"`
	LastFailure  *isolationProbeFailure `json:"last_failure,omitempty"`
}
type isolationProbeHealth struct {
	slots                                  [6]isolationProbeHealthSlot
	observations, cqlFailures, tcpFailures int64
}

func isolationProbeFailurePhase(message string) string {
	m := strings.ToLower(strings.TrimSpace(message))
	switch {
	case strings.HasPrefix(m, "dial "):
		return "dial"
	case strings.HasPrefix(m, "read "):
		return "read"
	case strings.HasPrefix(m, "write "):
		return "write"
	case strings.HasPrefix(m, "tls:") || strings.HasPrefix(m, "x509:"):
		return "tls"
	case strings.HasPrefix(m, "unexpected cql reply") || strings.HasPrefix(m, "invalid cql supported map"):
		return "protocol"
	default:
		return "unknown" // e.g. plain EOF/deadline has no trustworthy stage.
	}
}
func (h *isolationProbeHealth) observe(nodeIndex int, protocol string, c model.Check) bool {
	offset := 0
	if protocol == "tcp" {
		offset = 1
	}
	slot := &h.slots[nodeIndex*2+offset]
	slot.NodeIndex = nodeIndex
	slot.Protocol = protocol
	slot.Observations++
	h.observations++
	if c.OK {
		return false
	}
	firstFailure := slot.Failures == 0
	slot.Failures++
	if protocol == "cql" {
		h.cqlFailures++
	} else {
		h.tcpFailures++
	}
	attempted := c.CheckedAt
	if !attempted.IsZero() {
		attempted = attempted.Add(-c.Latency).UTC()
	}
	slot.LastFailure = &isolationProbeFailure{Phase: isolationProbeFailurePhase(c.Message), AttemptedAtUTC: attempted, CheckedAtUTC: c.CheckedAt.UTC(), LatencyNS: int64(c.Latency)}
	return firstFailure
}
func (h *isolationProbeHealth) failed() bool { return h.cqlFailures+h.tcpFailures > 0 }
func (h *isolationProbeHealth) snapshot() [6]isolationProbeHealthSlot {
	slots := h.slots
	for i := range slots {
		slots[i].NodeIndex = i / 2
		if i%2 == 0 {
			slots[i].Protocol = "cql"
		} else {
			slots[i].Protocol = "tcp"
		}
	}
	return slots
}

// A health NG remains a health NG, but it is not a resource/fixture-shape
// failure. The runner separately degrades the case when the persistent health
// flag is true. The memory workload continues, preserving resource evidence.
func isolationObserveCollectorSnapshot(s model.Snapshot, h *isolationProbeHealth, report func(int, string, model.Check)) error {
	if len(s.Nodes) != 3 || len(s.Incidents) != 500 || !s.Discovery.OK {
		return fmt.Errorf("collector fixture counts/discovery changed")
	}
	for i, n := range s.Nodes {
		for _, v := range []struct {
			protocol string
			check    model.Check
		}{{"cql", n.CQL}, {"tcp", n.Internode}} {
			firstFailure := h.observe(i, v.protocol, v.check)
			if firstFailure && report != nil {
				report(i, v.protocol, v.check)
			}
		}
	}
	return nil
}

func TestIsolationProbeFailurePersistsAfterLaterSuccess(t *testing.T) {
	now := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	s, _ := isolationFixture(now)
	for i := range s.Nodes {
		s.Nodes[i].CQL = model.Check{OK: true, CheckedAt: now}
		s.Nodes[i].Internode = model.Check{OK: true, CheckedAt: now}
	}
	var tracker isolationProbeHealth
	s.Nodes[2].CQL = model.Check{OK: false, Message: "dial tcp synthetic: i/o timeout", Latency: 200 * time.Millisecond, CheckedAt: now}
	logs := 0
	if err := isolationObserveCollectorSnapshot(s, &tracker, func(int, string, model.Check) { logs++ }); err != nil {
		t.Fatal("probe NG incorrectly failed resource trial", err)
	}
	if !tracker.failed() || tracker.cqlFailures != 1 || tracker.tcpFailures != 0 || tracker.observations != 6 || logs != 1 {
		t.Fatal(tracker, logs)
	}
	first := *tracker.slots[4].LastFailure
	// Repeated failures update cumulative/last metadata without retaining an
	// unbounded testing.T log history. Each fixed node/protocol slot logs once.
	for i := 1; i <= 10; i++ {
		s.Nodes[2].CQL.CheckedAt = now.Add(time.Duration(i) * time.Second)
		if err := isolationObserveCollectorSnapshot(s, &tracker, func(int, string, model.Check) { logs++ }); err != nil {
			t.Fatal(err)
		}
	}
	if logs != 1 || tracker.cqlFailures != 11 || tracker.observations != 66 {
		t.Fatal("repeated failures grew logs or lost counts", logs, tracker)
	}
	last := *tracker.slots[4].LastFailure
	if !last.CheckedAtUTC.Equal(now.Add(10 * time.Second)) {
		t.Fatal("last failure was not updated", last)
	}
	s.Nodes[2].CQL = model.Check{OK: true, CheckedAt: now.Add(11 * time.Second)}
	if err := isolationObserveCollectorSnapshot(s, &tracker, nil); err != nil {
		t.Fatal(err)
	}
	if !tracker.failed() || tracker.cqlFailures != 11 || tracker.observations != 72 || *tracker.slots[4].LastFailure != last {
		t.Fatal("later success erased health failure", tracker)
	}
	if first.Phase != "dial" || !first.AttemptedAtUTC.Equal(now.Add(-200*time.Millisecond)) || first.LatencyNS != 200000000 {
		t.Fatal(first)
	}
	s.Discovery.OK = false
	if err := isolationObserveCollectorSnapshot(s, &tracker, nil); err == nil {
		t.Fatal("resource/discovery failure was not fatal-capable")
	}
	if tracker.observations != 72 {
		t.Fatal("invalid resource snapshot counted as valid observations")
	}
}
func TestIsolationProbePhaseClassification(t *testing.T) {
	for message, want := range map[string]string{
		"dial tcp synthetic: i/o timeout": "dial", "read tcp synthetic: i/o timeout": "read", "write tcp synthetic: broken pipe": "write",
		"tls: handshake failure": "tls", "x509: invalid certificate": "tls", "unexpected CQL reply (opcode=00)": "protocol", "invalid CQL SUPPORTED map": "protocol",
		"EOF": "unknown", "unexpected EOF": "unknown", "context deadline exceeded": "unknown", "i/o timeout": "unknown", "": "unknown",
	} {
		if got := isolationProbeFailurePhase(message); got != want {
			t.Errorf("%q: got %s, want %s", message, got, want)
		}
	}
}
