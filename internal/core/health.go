package core

import (
	"math"
	"scyllascope/internal/model"
	"sort"
	"time"
)

func NodeHealth(n model.Node, now time.Time, ttl time.Duration) (model.NodeState, []string) {
	reasons := []string{}
	bad := false
	unknown := false
	warn := false
	for _, v := range []struct {
		name string
		c    model.Check
	}{{"CQL OPTIONS", n.CQL}, {"Internode TCP", n.Internode}} {
		if !v.c.Fresh(now, ttl) {
			unknown = true
			reasons = append(reasons, v.name+" 未確認 / 期限切れ")
		} else if !v.c.OK {
			bad = true
			reasons = append(reasons, v.name+": "+v.c.Message)
		}
	}
	if n.MetadataKnown && !n.MetadataAt.IsZero() && now.Sub(n.MetadataAt) <= ttl && n.MetadataAt.Sub(now) < 5*time.Second {
		if !n.Up {
			bad = true
			reasons = append(reasons, "ScyllaからDOWNと観測")
		}
		if n.Status != "NORMAL" && n.Status != "UN" {
			warn = true
			reasons = append(reasons, "Scylla状態: "+n.Status)
		}
	} else {
		unknown = true
		reasons = append(reasons, "Scyllaメンバー情報が未確認")
	}
	if n.Missing {
		warn = true
		reasons = append(reasons, "直近のクラスタ一覧に存在しません")
	}
	if n.LocalService.Fresh(now, ttl) && !n.LocalService.OK {
		bad = true
		reasons = append(reasons, "scylla-server: "+n.LocalService.Message)
	}
	if bad {
		return model.Critical, reasons
	}
	if warn {
		return model.Warning, reasons
	}
	if unknown {
		return model.Unknown, reasons
	}
	return model.Healthy, reasons
}
func Summarize(s *model.Snapshot, expected int, ttl time.Duration) {
	now := s.GeneratedAt
	cov := model.Coverage{ExpectedNodes: expected}
	if cov.ExpectedNodes < len(s.Nodes) {
		cov.ExpectedNodes = len(s.Nodes)
	}
	health := model.Healthy
	for i := range s.Nodes {
		n := &s.Nodes[i]
		n.State, n.Reasons = NodeHealth(*n, now, ttl)
		if n.MetadataKnown && !n.Missing && now.Sub(n.MetadataAt) <= ttl && n.MetadataAt.Sub(now) < 5*time.Second {
			cov.DiscoveredNodes++
		}
		if n.CQL.Fresh(now, ttl) && n.CQL.OK && n.Internode.Fresh(now, ttl) && n.Internode.OK {
			cov.ReachableNodes++
		}
		if n.WatchAPI.Fresh(now, ttl) && n.WatchAPI.OK {
			cov.AgentsReady++
		}
		if n.State == model.Critical {
			health = model.Critical
		} else if n.State == model.Warning && health != model.Critical {
			health = model.Warning
		} else if n.State == model.Unknown && health == model.Healthy {
			health = model.Unknown
		}
	}
	if len(s.Nodes) == 0 || !s.Discovery.Fresh(now, ttl) || !s.Discovery.OK {
		if health != model.Critical {
			health = model.Unknown
		}
	}
	if cov.ExpectedNodes > cov.DiscoveredNodes && health == model.Healthy {
		health = model.Warning
	}
	cov.ExpectedDirections = len(s.Nodes) * max(0, len(s.Nodes)-1)
	ids := map[string]bool{}
	for _, n := range s.Nodes {
		ids[n.ID] = true
	}
	unique := map[string]bool{}
	filtered := make([]model.Edge, 0, len(s.Edges))
	for _, e := range s.Edges {
		k := e.From + ">" + e.To
		if e.From != e.To && ids[e.From] && ids[e.To] && !unique[k] {
			unique[k] = true
			filtered = append(filtered, e)
		}
	}
	s.Edges = filtered
	dirs := map[string]int{}
	for i := range s.Edges {
		e := &s.Edges[i]
		e.Asymmetric = false
		e.Stale = e.ObservedAt.IsZero() || now.Sub(e.ObservedAt) > ttl || e.ObservedAt.Sub(now) > 5*time.Second || (!e.CQL.Fresh(now, ttl) && !e.Internode.Fresh(now, ttl))
		dirs[e.From+">"+e.To] = i
		if !e.Stale {
			cov.ObservedDirections++
			if ((e.CQL.Fresh(now, ttl) && !e.CQL.OK) || (e.Internode.Fresh(now, ttl) && !e.Internode.OK)) && health == model.Healthy {
				health = model.Warning
			}
		}
	}
	for i := range s.Edges {
		e := &s.Edges[i]
		if j, ok := dirs[e.To+">"+e.From]; ok && !e.Stale && !s.Edges[j].Stale {
			r := &s.Edges[j]
			if (e.CQL.Fresh(now, ttl) && r.CQL.Fresh(now, ttl) && e.CQL.OK != r.CQL.OK) || (e.Internode.Fresh(now, ttl) && r.Internode.Fresh(now, ttl) && e.Internode.OK != r.Internode.OK) {
				e.Asymmetric = true
				r.Asymmetric = true
				if health == model.Healthy {
					health = model.Warning
				}
			}
		}
	}
	s.Health = health
	s.Coverage = cov
}
func Sample(s model.Snapshot) model.Sample {
	v := model.Sample{Time: s.GeneratedAt, ClusterHealth: s.Health, NodesTotal: len(s.Nodes), DiscoveryOK: s.Discovery.OK, AgentsReady: s.Coverage.AgentsReady}
	var sum float64
	count := 0
	var load int64
	known := len(s.Nodes) > 0
	for _, n := range s.Nodes {
		if n.State == model.Healthy {
			v.NodesHealthy++
		}
		if n.CQL.Fresh(s.GeneratedAt, time.Duration(s.StaleAfterSeconds*float64(time.Second))) && n.CQL.OK {
			v.CQLHealthy++
			sum += float64(n.CQL.Latency) / float64(time.Millisecond)
			count++
		}
		if n.Internode.Fresh(s.GeneratedAt, time.Duration(s.StaleAfterSeconds*float64(time.Second))) && n.Internode.OK {
			v.InternodeHealthy++
		}
		if n.LoadKnown && n.MetadataKnown && !n.MetadataAt.IsZero() && s.GeneratedAt.Sub(n.MetadataAt) <= time.Duration(s.StaleAfterSeconds*float64(time.Second)) && n.MetadataAt.Sub(s.GeneratedAt) < 5*time.Second {
			load += n.LoadBytes
		} else {
			known = false
		}
	}
	if count > 0 {
		avg := sum / float64(count)
		avg = math.Round(avg*1000) / 1000
		v.AverageLatencyMS = &avg
	}
	if known {
		v.TotalLoadBytes = &load
	}
	return v
}

// Downsample keeps the entire requested time span and the newest observation.
func Downsample[T any](v []T, limit int) []T {
	if limit < 2 {
		limit = 2
	}
	if len(v) <= limit {
		return v
	}
	out := make([]T, 0, limit)
	for i := 0; i < limit; i++ {
		idx := int(math.Round(float64(i) * float64(len(v)-1) / float64(limit-1)))
		out = append(out, v[idx])
	}
	return out
}
func SortNodes(v []model.Node) {
	sort.Slice(v, func(i, j int) bool {
		if v[i].Name == v[j].Name {
			return v[i].Address < v[j].Address
		}
		return v[i].Name < v[j].Name
	})
}
