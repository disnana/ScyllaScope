package core

import (
	"fmt"
	"scyllascope/internal/model"
	"time"
)

// ValidatePeer accepts only a configured peer's own fresh observations. It
// never re-gossips merged observations, which would make stale health immortal.
func ValidatePeer(s model.Snapshot, cluster, address string, now time.Time, ttl time.Duration) error {
	if s.SchemaVersion < 2 || s.SchemaVersion > 3 {
		return fmt.Errorf("unsupported peer schema %d; upgrade this observer", s.SchemaVersion)
	}
	if s.LocalAddress != address {
		return fmt.Errorf("peer local_address does not match requested address")
	}
	if s.ClusterName != cluster {
		return fmt.Errorf("peer belongs to a different cluster")
	}
	if s.GeneratedAt.IsZero() || now.Sub(s.GeneratedAt) > ttl || s.GeneratedAt.Sub(now) > 5*time.Second {
		return fmt.Errorf("peer snapshot is stale or its clock is ahead")
	}
	if len(s.Nodes) > 128 || len(s.Edges) > 16384 {
		return fmt.Errorf("peer snapshot exceeds size limits")
	}
	for _, n := range s.Nodes {
		if n.Address == address && n.ID == s.LocalNode {
			return nil
		}
	}
	return fmt.Errorf("peer identity does not match requested address")
}
func MergePeer(nodes []model.Node, edges []model.Edge, ps model.Snapshot, address string, now time.Time, ttl time.Duration) ([]model.Node, []model.Edge) {
	ids := map[string]string{}
	for _, n := range nodes {
		ids[n.Address] = n.ID
	}
	peerID := ids[address]
	remoteToLocal := map[string]string{}
	for _, n := range ps.Nodes {
		if id, ok := ids[n.Address]; ok {
			remoteToLocal[n.ID] = id
		}
	}
	for _, obs := range ps.Nodes {
		for i := range nodes {
			n := &nodes[i]
			if n.Address != obs.Address {
				continue
			}
			if n.ObservedBy == nil {
				n.ObservedBy = map[string]model.Check{}
			}
			n.ObservedBy[peerID] = model.Check{OK: obs.State == model.Healthy, State: "ok", Kind: "peer_observation", Message: string(obs.State), CheckedAt: ps.GeneratedAt}
			if obs.ID == ps.LocalNode && obs.Address == address {
				n.AgentAt = ps.GeneratedAt
				n.AgentVersion = ps.Version
				if n.AgentVersion == "" {
					n.AgentVersion = "0.4.x"
				}
				n.WatchAPI = model.Check{OK: true, State: "ok", Kind: "http", Message: "authenticated peer snapshot", CheckedAt: now}
				if obs.LocalService.Fresh(now, ttl) {
					n.LocalService = obs.LocalService
				}
				if obs.REST.Fresh(now, ttl) {
					n.REST = obs.REST
				}
				if obs.Version != "" {
					n.Version = obs.Version
				}
				if obs.Shards > 0 {
					n.Shards = obs.Shards
				}
				n.Labels = obs.Labels
				if obs.Metrics.Known() && obs.Metrics.Fresh(now, ttl) {
					n.Metrics = obs.Metrics
					n.MetricValues = obs.MetricValues
				}
			}
		}
	}
	seenEdges := map[string]bool{}
	for _, e := range edges {
		seenEdges[e.From+">"+e.To] = true
	}
	for _, e := range ps.Edges {
		if e.From != ps.LocalNode {
			continue
		}
		from, ok1 := remoteToLocal[e.From]
		to, ok2 := remoteToLocal[e.To]
		if !ok1 || !ok2 || from == to {
			continue
		}
		if seenEdges[from+">"+to] {
			continue
		}
		seenEdges[from+">"+to] = true
		e.From = from
		e.To = to
		// Preserve the original observation timestamp. A relay must not refresh stale evidence.
		if e.ObservedAt.IsZero() || now.Sub(e.ObservedAt) > ttl || e.ObservedAt.Sub(now) > 5*time.Second {
			continue
		}
		if !e.CQL.Fresh(now, ttl) && !e.Internode.Fresh(now, ttl) {
			continue
		}
		edges = append(edges, e)
	}
	return nodes, edges
}
