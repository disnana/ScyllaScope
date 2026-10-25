package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt-in synthetic legacy schema migration. No operational database is opened.
// This measures additive index creation and correctness at one finite size;
// it is not a guarantee for arbitrarily large or damaged production databases.
func TestLargeLegacyIncidentMigration(t *testing.T) {
	if os.Getenv("SCYLLASCOPE_LARGE_LEGACY") != "1" {
		t.Skip("set SCYLLASCOPE_LARGE_LEGACY=1 for the 200000-row synthetic SQLite migration")
	}
	const total, activeCount = 200000, 1201
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	legacy.SetMaxOpenConns(1)
	fail := func(err error) {
		if err != nil {
			legacy.Close()
			t.Fatal(err)
		}
	}
	_, err = legacy.Exec(`PRAGMA journal_mode=WAL; CREATE TABLE incidents_v5(id TEXT PRIMARY KEY,body TEXT NOT NULL,active INTEGER NOT NULL,updated INTEGER NOT NULL);`)
	fail(err)
	fixtureStart := time.Now()
	tx, err := legacy.Begin()
	fail(err)
	defer tx.Rollback()
	insert, err := tx.Prepare(`INSERT INTO incidents_v5(id,body,active,updated) VALUES(?,?,?,?)`)
	fail(err)
	old := time.Now().Add(-48 * time.Hour).UTC()
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("legacy-%09d", i)
		// Explicit old-format JSON has neither notification_pending nor recovery_pending.
		body := map[string]any{"id": id, "observer": "synthetic-observer", "node": fmt.Sprintf("synthetic-node-%d", i), "kind": "node_health", "severity": "warning", "message": "synthetic legacy incident", "opened_at": old, "updated_at": old, "acknowledged": false}
		active := i < activeCount
		if !active {
			body["resolved_at"] = old
		}
		raw, e := json.Marshal(body)
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(string(raw), "pending") {
			t.Fatal("fixture accidentally includes new pending fields")
		}
		if _, err = insert.Exec(id, string(raw), active, old.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	fail(insert.Close())
	fail(tx.Commit())
	before, err := legacyDigest(legacy, false)
	fail(err)
	activeBefore, err := legacyDigest(legacy, true)
	fail(err)
	var sqliteVersion string
	fail(legacy.QueryRow(`SELECT sqlite_version()`).Scan(&sqliteVersion))
	fail(legacy.Close())
	fixtureSeconds := time.Since(fixtureStart).Seconds()
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	openStart := time.Now()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if s != nil {
			s.Close()
		}
	}()
	migrationSeconds := time.Since(openStart).Seconds()
	after, err := legacyDigest(s.db, false)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatal("migration changed legacy rows")
	}
	var workIndex int
	if err = s.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='index' AND name='incidents_v5_work_idx'`).Scan(&workIndex); err != nil || workIndex != 1 {
		t.Fatal("work index missing", err)
	}
	uiStart := time.Now()
	ui, err := s.Incidents(true)
	if err != nil || len(ui) != 500 {
		t.Fatal("UI query", len(ui), err)
	}
	uiSeconds := time.Since(uiStart).Seconds()
	countWork := func() int {
		cursor := ""
		count := 0
		for {
			page, e := s.WorkIncidents(cursor, 137)
			if e != nil {
				t.Fatal(e)
			}
			if len(page) == 0 {
				break
			}
			for _, v := range page {
				if v.ID <= cursor || v.ResolvedAt != nil {
					t.Fatal("invalid work pagination", v.ID)
				}
				cursor = v.ID
				count++
			}
		}
		return count
	}
	workStart := time.Now()
	work := countWork()
	workSeconds := time.Since(workStart).Seconds()
	if work != activeCount {
		t.Fatal("legacy active work truncated", work)
	}
	pruneStart := time.Now()
	if err = s.Prune(1); err != nil {
		t.Fatal(err)
	}
	pruneSeconds := time.Since(pruneStart).Seconds()
	var remaining int
	if err = s.db.QueryRow(`SELECT count(*) FROM incidents_v5`).Scan(&remaining); err != nil || remaining != activeCount {
		t.Fatal("prune result", remaining, err)
	}
	activeAfter, err := legacyDigest(s.db, true)
	if err != nil || activeAfter != activeBefore {
		t.Fatal("prune changed active rows", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	reopenStart := time.Now()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	reopenSeconds := time.Since(reopenStart).Seconds()
	if countWork() != activeCount {
		t.Fatal("active work lost after reopen")
	}
	var sqliteVersionCheck string
	if err = s.db.QueryRow(`PRAGMA integrity_check`).Scan(&sqliteVersionCheck); err != nil {
		t.Fatal(err)
	}
	if sqliteVersionCheck != "ok" {
		t.Fatal("SQLite integrity", sqliteVersionCheck)
	}
	summary := map[string]any{"result": "PASS", "rows_before": total, "active_before": activeCount, "rows_after_prune": remaining, "work_rows": work, "ui_rows": len(ui), "original_database_bytes": beforeInfo.Size(), "sqlite_version": sqliteVersion, "fixture_build_seconds": fixtureSeconds, "first_open_additive_migration_seconds": migrationSeconds, "ui_query_seconds": uiSeconds, "work_query_seconds": workSeconds, "prune_seconds": pruneSeconds, "reopen_seconds": reopenSeconds, "all_legacy_rows_unchanged_before_prune": true, "active_rows_unchanged_after_prune": true, "integrity_check": "ok"}
	raw, _ := json.Marshal(summary)
	t.Logf("LEGACY_MIGRATION_METRICS %s", raw)
}

func legacyDigest(db *sql.DB, activeOnly bool) (string, error) {
	query := `SELECT id,body,active,updated FROM incidents_v5`
	if activeOnly {
		query += ` WHERE active=1`
	}
	query += ` ORDER BY id`
	rows, err := db.Query(query)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	h := sha256.New()
	for rows.Next() {
		var id, body string
		var active, updated int64
		if err = rows.Scan(&id, &body, &active, &updated); err != nil {
			return "", err
		}
		// Length prefixes avoid ambiguous concatenations of valid legacy fields.
		fmt.Fprintf(h, "%d:%s%d:", len(id), id, len(body))
		io.WriteString(h, body)
		fmt.Fprintf(h, "/%d/%d\n", active, updated)
	}
	if err = rows.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
