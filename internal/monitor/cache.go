package monitor

import (
	"scyllascope/internal/model"
	"sort"
	"time"
)

// Discovery cache retains metadata, never the previous metrics/observation graph.
func metadataOnly(n model.Node) model.Node {
	return model.Node{ID: n.ID, Name: n.Name, Address: n.Address, DC: n.DC, Rack: n.Rack,
		Status: n.Status, Up: n.Up, MetadataKnown: n.MetadataKnown, MetadataAt: n.MetadataAt,
		LoadBytes: n.LoadBytes, LoadKnown: n.LoadKnown, HostID: n.HostID}
}

// topologyMetadata is the restart discovery cache, not a copy of the live UI
// snapshot. Preserve the same node set/order and original metadata timestamps;
// observations, history and incidents have their own live/persistent owners.
// Keep Store.SaveTopology generic for other callers and legacy round-trips.
func topologyMetadata(s model.Snapshot) model.Snapshot {
	cached := model.Snapshot{SchemaVersion: s.SchemaVersion, Version: s.Version,
		ClusterName: s.ClusterName, LocalNode: s.LocalNode, LocalNodeName: s.LocalNodeName,
		LocalAddress: s.LocalAddress, GeneratedAt: s.GeneratedAt}
	if s.Nodes != nil {
		cached.Nodes = make([]model.Node, len(s.Nodes))
		for i, n := range s.Nodes {
			cached.Nodes[i] = metadataOnly(n)
		}
	}
	return cached
}

// A failed discovery never proves membership removal. Expire retired members
// only with a successful complete discovery, and keep configured peers explicitly.
// At all times bound the cache to twice max_nodes, preserving current identities.
func (m *Monitor) trimKnown(current map[string]bool, complete bool, now time.Time) {
	explicit := map[string]bool{m.cfg.LocalAddress: true}
	for _, p := range m.cfg.Peers {
		explicit[p.Address] = true
	}
	if complete {
		for addr, n := range m.known {
			if !current[addr] && !explicit[addr] && now.Sub(n.MetadataAt) > m.cfg.Monitoring.KnownNodeRetention {
				delete(m.known, addr)
			}
		}
	}
	limit := max(2, 2*m.cfg.Monitoring.MaxNodes)
	if len(m.known) <= limit {
		return
	}
	keys := make([]string, 0, len(m.known))
	for k := range m.known {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if current[a] != current[b] {
			return !current[a]
		}
		if explicit[a] != explicit[b] {
			return !explicit[a]
		}
		if !m.known[a].MetadataAt.Equal(m.known[b].MetadataAt) {
			return m.known[a].MetadataAt.Before(m.known[b].MetadataAt)
		}
		return a < b
	})
	for _, k := range keys[:len(keys)-limit] {
		delete(m.known, k)
	}
}

// Deterministic fallback prevents cache map iteration from dropping the local
// observer or arbitrarily changing the displayed subset during an outage.
func (m *Monitor) fallbackNodes() []model.Node {
	candidates := map[string]model.Node{}
	for addr, n := range m.known {
		candidates[addr] = n
	}
	explicit := map[string]bool{m.cfg.LocalAddress: true}
	for _, p := range m.cfg.Peers {
		explicit[p.Address] = true
		if _, ok := candidates[p.Address]; !ok {
			candidates[p.Address] = model.Node{ID: p.Address, Address: p.Address, Name: p.Name}
		}
	}
	if _, ok := candidates[m.cfg.LocalAddress]; !ok {
		candidates[m.cfg.LocalAddress] = model.Node{ID: m.cfg.LocalAddress, Address: m.cfg.LocalAddress, Name: m.cfg.NodeName}
	}
	keys := make([]string, 0, len(candidates))
	for key := range candidates {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a == m.cfg.LocalAddress || b == m.cfg.LocalAddress {
			return a == m.cfg.LocalAddress
		}
		if explicit[a] != explicit[b] {
			return explicit[a]
		}
		if !candidates[a].MetadataAt.Equal(candidates[b].MetadataAt) {
			return candidates[a].MetadataAt.After(candidates[b].MetadataAt)
		}
		return a < b
	})
	out := make([]model.Node, 0, len(keys))
	for _, key := range keys {
		out = append(out, candidates[key])
	}
	return out
}
