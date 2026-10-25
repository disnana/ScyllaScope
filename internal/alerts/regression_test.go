package alerts

import (
	"context"
	"fmt"
	"scyllascope/internal/model"
	"scyllascope/internal/testkit"
	"testing"
	"time"
)

func TestRetryBookkeepingHardCapAndRecovery(t *testing.T) {
	e, _ := fixture()
	e.cfg.Notifications.GenericWebhook = "http://127.0.0.1:1"
	now := time.Now()
	for i := 0; i < 10000; i++ {
		id := fmt.Sprint(i)
		e.enqueue(model.Incident{ID: id}, false, now)
		select {
		case j := <-e.queue:
			delete(e.inflight, j.incident.ID)
		default:
		}
		if len(e.lastAttempt) > 2048 {
			t.Fatal("retry map unbounded")
		}
	}
	if len(e.lastAttempt) != 2048 {
		t.Fatal(len(e.lastAttempt))
	}
	e.Evaluate(context.Background(), testkit.Snapshot(now.Add(e.cfg.Notifications.RetryInterval+time.Second)))
	if len(e.lastAttempt) != 0 {
		t.Fatal("eligible retry entries not reclaimed")
	}
	e.enqueue(model.Incident{ID: "new"}, false, now.Add(e.cfg.Notifications.RetryInterval+time.Second))
	if len(e.queue) != 1 {
		t.Fatal("queue failed to recover")
	}
}
func TestRetryCleanupPreservesInflight(t *testing.T) {
	e, _ := fixture()
	now := time.Now()
	for i := 0; i < 2048; i++ {
		e.lastAttempt[fmt.Sprint(i)] = now.Add(-time.Hour)
	}
	e.inflight["0"] = true
	e.Evaluate(context.Background(), testkit.Snapshot(now))
	if _, ok := e.lastAttempt["0"]; !ok {
		t.Fatal("inflight retry removed")
	}
}
