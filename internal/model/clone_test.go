package model

import (
	"math"
	"testing"
	"time"
)

func TestCloneSnapshotOwnsAllMutableFields(t *testing.T) {
	now := time.Now()
	s := Snapshot{Warnings: []string{"x"}, Nodes: []Node{{Labels: map[string]string{"a": "b"}, MetricValues: map[string]float64{"m": 2}, ObservedBy: map[string]Check{"o": {OK: true}}, Reasons: []string{"why"}}}, Edges: []Edge{{From: "a"}}, Events: []Event{{Message: "x"}}, Alerts: []Event{{Message: "x"}}, Incidents: []Incident{{ResolvedAt: &now, SilencedUntil: &now, LastNotifiedAt: &now, RecoveryNotifiedAt: &now}}}
	c := CloneSnapshot(s)
	c.Nodes[0].Labels["a"] = "changed"
	c.Nodes[0].MetricValues["m"] = 3
	c.Nodes[0].ObservedBy["o"] = Check{}
	c.Nodes[0].Reasons[0] = "changed"
	c.Edges[0].From = "changed"
	c.Events[0].Message = "changed"
	c.Alerts[0].Message = "changed"
	c.Warnings[0] = "changed"
	*c.Incidents[0].ResolvedAt = time.Time{}
	*c.Incidents[0].SilencedUntil = time.Time{}
	*c.Incidents[0].LastNotifiedAt = time.Time{}
	*c.Incidents[0].RecoveryNotifiedAt = time.Time{}
	if s.Nodes[0].Labels["a"] != "b" || s.Nodes[0].MetricValues["m"] != 2 || !s.Nodes[0].ObservedBy["o"].OK || s.Nodes[0].Reasons[0] != "why" || s.Edges[0].From != "a" || s.Events[0].Message != "x" || s.Alerts[0].Message != "x" || s.Warnings[0] != "x" || now.IsZero() {
		t.Fatal("alias survived deep clone")
	}
}
func TestCloneDoesNotSilentlyEraseNonJSONState(t *testing.T) {
	s := Snapshot{Version: "present", Nodes: []Node{{MetricValues: map[string]float64{"bad": math.NaN()}}}}
	c := CloneSnapshot(s)
	if c.Version != "present" || len(c.Nodes) != 1 || !math.IsNaN(c.Nodes[0].MetricValues["bad"]) {
		t.Fatal("state erased")
	}
}
func TestClonePreservesNilAndEmpty(t *testing.T) {
	if CloneSnapshot(Snapshot{}).Nodes != nil {
		t.Fatal("nil changed")
	}
	if CloneSnapshot(Snapshot{Nodes: []Node{}}).Nodes == nil {
		t.Fatal("empty changed")
	}
}
