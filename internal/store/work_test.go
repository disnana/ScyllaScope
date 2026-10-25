package store

import (
	"fmt"
	"path/filepath"
	"scyllascope/internal/model"
	"testing"
	"time"
)

func TestWorkPaginationAndPruneRetainUndelivered(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old := now.Add(-48 * time.Hour)
	for i := 0; i < 1201; i++ {
		v := model.Incident{ID: fmt.Sprintf("%06d", i), OpenedAt: old, UpdatedAt: old, ResolvedAt: &old, NotificationPending: true}
		if i%2 == 0 {
			v.LastNotifiedAt = &old
			v.RecoveryPending = true
		}
		if err = s.PutIncident(v); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.Prune(1); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ui, _ := s.Incidents(true)
	if len(ui) != 500 {
		t.Fatal(len(ui))
	}
	count := 0
	cursor := ""
	for {
		page, err := s.WorkIncidents(cursor, 113)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		for _, v := range page {
			if v.ID <= cursor {
				t.Fatal("cursor did not advance")
			}
			cursor = v.ID
			count++
		}
	}
	if count != 1201 {
		t.Fatal(count)
	}
	v, err := s.Incident("000000")
	if err != nil || !v.RecoveryPending {
		t.Fatal(v, err)
	}
}
