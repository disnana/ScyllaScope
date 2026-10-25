package model

import (
	"maps"
	"slices"
	"time"
)

// CloneNode copies every mutable child. Strings and time.Time values are immutable.
func CloneNode(n Node) Node {
	n.MetricValues = maps.Clone(n.MetricValues)
	n.ObservedBy = maps.Clone(n.ObservedBy)
	n.Labels = maps.Clone(n.Labels)
	n.Reasons = slices.Clone(n.Reasons)
	return n
}
func copyTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := *t
	return &v
}
func CloneIncident(v Incident) Incident {
	v.ResolvedAt = copyTime(v.ResolvedAt)
	v.SilencedUntil = copyTime(v.SilencedUntil)
	v.LastNotifiedAt = copyTime(v.LastNotifiedAt)
	v.RecoveryNotifiedAt = copyTime(v.RecoveryNotifiedAt)
	return v
}

// CloneSnapshot is the immutable publication boundary. It cannot silently replace
// a snapshot with its zero value when a field is not JSON-encodable.
func CloneSnapshot(s Snapshot) Snapshot {
	s.Nodes = slices.Clone(s.Nodes)
	for i := range s.Nodes {
		s.Nodes[i] = CloneNode(s.Nodes[i])
	}
	s.Edges = slices.Clone(s.Edges)
	s.Warnings = slices.Clone(s.Warnings)
	s.Alerts = slices.Clone(s.Alerts)
	s.Events = slices.Clone(s.Events)
	s.Incidents = slices.Clone(s.Incidents)
	for i := range s.Incidents {
		s.Incidents[i] = CloneIncident(s.Incidents[i])
	}
	return s
}
