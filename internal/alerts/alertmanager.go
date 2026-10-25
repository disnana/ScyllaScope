package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"scyllascope/internal/model"
	"strings"
	"sync"
	"time"
)

// Labels identify the failure, not its reporter or sample time. Directional
// observations deliberately retain the reporter so partitions are not hidden.
func alertLabels(cluster string, v model.Incident) map[string]string {
	labels := map[string]string{"alertname": "ScyllaScopeIncident", "cluster": cluster, "node": v.Node, "kind": v.Kind}
	labels["observer"] = v.Observer
	labels["incident_id"] = v.ID
	return labels
}
func (e *Engine) sendAlertmanager(ctx context.Context, v model.Incident, recovery bool) error {
	end := time.Now().Add(max(5*time.Minute, 4*e.cfg.Notifications.RetryInterval))
	if recovery {
		if v.ResolvedAt == nil {
			return fmt.Errorf("recovery without timestamp")
		}
		end = *v.ResolvedAt
	}
	body, err := json.Marshal([]any{map[string]any{"labels": alertLabels(e.cfg.ClusterName, v), "annotations": map[string]string{"summary": v.Message, "reporter": v.Observer, "severity": v.Severity}, "startsAt": v.OpenedAt, "endsAt": end}})
	if err != nil {
		return err
	}
	// Send to every HA member. Local persisted incidents retry after outages.
	// At least one durable notification service must accept before marking sent.
	// Posting to each endpoint concurrently keeps a timed-out member from
	// delaying requests to a healthy member. Configuration permits at most 8;
	// keep that bound even for internal callers using an unvalidated config.
	urls := e.cfg.Notifications.AlertmanagerURLs
	urls = urls[:min(len(urls), 8)]
	results := make(chan bool, len(urls))
	var requests sync.WaitGroup
	for _, base := range urls {
		requests.Add(1)
		go func(base string) {
			defer requests.Done()
			results <- e.post(ctx, strings.TrimRight(base, "/")+"/api/v2/alerts", body) == nil
		}(base)
	}
	requests.Wait()
	close(results)
	accepted := false
	for ok := range results {
		accepted = accepted || ok
	}
	if !accepted {
		return fmt.Errorf("all Alertmanager endpoints failed")
	}
	return nil
}

func healthConditions(n model.Node, now time.Time, ttl time.Duration) []string {
	out := []string{}
	for _, v := range []struct {
		name string
		c    model.Check
	}{{"cql", n.CQL}, {"internode", n.Internode}, {"service", n.LocalService}} {
		if v.c.Fresh(now, ttl) && !v.c.OK {
			out = append(out, v.name)
		}
	}
	if n.MetadataKnown && !n.MetadataAt.IsZero() && now.Sub(n.MetadataAt) <= ttl {
		if !n.Up {
			out = append(out, "down")
		}
		if n.Status != "NORMAL" && n.Status != "UN" {
			out = append(out, "status")
		}
	}
	if n.Missing {
		out = append(out, "missing")
	}
	return out
}
func conditionRecovered(n model.Node, condition string, now time.Time, ttl time.Duration) bool {
	var c model.Check
	switch condition {
	case "cql":
		c = n.CQL
	case "internode":
		c = n.Internode
	case "service":
		c = n.LocalService
	case "down", "status", "missing":
		if !n.MetadataKnown || n.MetadataAt.IsZero() || now.Sub(n.MetadataAt) > ttl || n.MetadataAt.Sub(now) > 5*time.Second {
			return false
		}
		if condition == "down" {
			return n.Up
		}
		if condition == "status" {
			return n.Status == "NORMAL" || n.Status == "UN"
		}
		return !n.Missing
	default:
		return false
	}
	return c.Fresh(now, ttl) && c.OK
}
