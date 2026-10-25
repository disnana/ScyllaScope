package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"scyllascope/internal/core"
	"scyllascope/internal/model"
	"time"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if e := os.MkdirAll(filepath.Dir(path), 0750); e != nil {
		return nil, e
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	fail := func(e error) (*Store, error) { db.Close(); return nil, e }
	if _, e = db.ExecContext(ctx, `PRAGMA journal_mode=WAL;PRAGMA busy_timeout=5000;PRAGMA foreign_keys=ON;PRAGMA cache_size=-2048;PRAGMA mmap_size=0;PRAGMA wal_autocheckpoint=1000;PRAGMA journal_size_limit=8388608;`); e != nil {
		return fail(e)
	}
	// Additive migration: retain v0.4 samples/events for rollback and inspection.
	_, e = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS events (id INTEGER PRIMARY KEY AUTOINCREMENT,ts TEXT NOT NULL,severity TEXT NOT NULL,type TEXT NOT NULL,node TEXT,message TEXT NOT NULL,resolved INTEGER NOT NULL DEFAULT 0);
 CREATE INDEX IF NOT EXISTS events_ts_idx ON events(ts);
 CREATE TABLE IF NOT EXISTS incidents_v5(id TEXT PRIMARY KEY,body TEXT NOT NULL,active INTEGER NOT NULL,updated INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS samples_v5(ts INTEGER PRIMARY KEY,body TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS node_samples_v5(node TEXT NOT NULL,ts INTEGER NOT NULL,body TEXT NOT NULL,PRIMARY KEY(node,ts));
 CREATE INDEX IF NOT EXISTS node_samples_v5_ts_idx ON node_samples_v5(ts);
 CREATE INDEX IF NOT EXISTS incidents_v5_active_updated_idx ON incidents_v5(active,updated);
 CREATE INDEX IF NOT EXISTS incidents_v5_work_idx ON incidents_v5(id) WHERE active=1 OR json_extract(body,'$.recovery_pending')=1 OR (json_extract(body,'$.notification_pending')=1 AND coalesce(json_extract(body,'$.acknowledged'),0)=0);
 CREATE TABLE IF NOT EXISTS scope_state_v5(key TEXT PRIMARY KEY,body TEXT NOT NULL);`)
	if e != nil {
		return fail(e)
	}
	if e = os.Chmod(path, 0600); e != nil {
		return fail(e)
	}
	return &Store{db}, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Add(e model.Event) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `INSERT INTO events(ts,severity,type,node,message,resolved) VALUES(?,?,?,?,?,?)`, e.Time.UTC().Format(time.RFC3339Nano), e.Severity, e.Type, e.Node, e.Message, e.Resolved)
	return err
}
func (s *Store) Recent(limit int) ([]model.Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if limit < 1 || limit > 500 {
		limit = 50
	}
	rows, e := s.db.QueryContext(ctx, `SELECT id,ts,severity,type,coalesce(node,''),message,resolved FROM events ORDER BY id DESC LIMIT ?`, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.Event{}
	for rows.Next() {
		var v model.Event
		var ts string
		if e = rows.Scan(&v.ID, &ts, &v.Severity, &v.Type, &v.Node, &v.Message, &v.Resolved); e != nil {
			return nil, e
		}
		v.Time, e = time.Parse(time.RFC3339Nano, ts)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) Incidents(includeResolved bool) ([]model.Incident, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	query := `SELECT body FROM incidents_v5`
	if !includeResolved {
		query += ` WHERE active=1`
	}
	query += ` ORDER BY active DESC, updated DESC LIMIT 500`
	rows, e := s.db.QueryContext(ctx, query)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.Incident{}
	for rows.Next() {
		var b string
		var v model.Incident
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// WorkIncidents uses a keyset cursor independent of the UI's 500-row limit.
// JSON predicates also cover databases created by older releases, without a
// destructive migration or an unbounded historical allocation.
func (s *Store) WorkIncidents(after string, limit int) ([]model.Incident, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT body FROM incidents_v5 WHERE id>? AND
 (active=1 OR json_extract(body,'$.recovery_pending')=1 OR
 (json_extract(body,'$.notification_pending')=1 AND coalesce(json_extract(body,'$.acknowledged'),0)=0))
 ORDER BY id LIMIT ?`, after, max(1, min(limit, 256)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Incident{}
	for rows.Next() {
		var body string
		var v model.Incident
		if err = rows.Scan(&body); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(body), &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) Incident(id string) (model.Incident, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var body string
	var v model.Incident
	err := s.db.QueryRowContext(ctx, `SELECT body FROM incidents_v5 WHERE id=?`, id).Scan(&body)
	if err != nil {
		return v, err
	}
	err = json.Unmarshal([]byte(body), &v)
	return v, err
}
func (s *Store) PutIncident(v model.Incident) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = s.db.ExecContext(ctx, `INSERT INTO incidents_v5(id,body,active,updated) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body,active=excluded.active,updated=excluded.updated`, v.ID, string(b), v.ResolvedAt == nil, v.UpdatedAt.UnixMilli())
	return e
}
func (s *Store) UpdateIncidentAction(id string, ack bool, silence *time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var b string
	if e = tx.QueryRowContext(ctx, `SELECT body FROM incidents_v5 WHERE id=?`, id).Scan(&b); e != nil {
		return e
	}
	var v model.Incident
	if e = json.Unmarshal([]byte(b), &v); e != nil {
		return e
	}
	v.Acknowledged = ack
	v.SilencedUntil = silence
	b2, e := json.Marshal(v)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE incidents_v5 SET body=? WHERE id=?`, string(b2), id); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) AddSnapshot(v model.Snapshot) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sample := core.Sample(v)
	b, e := json.Marshal(sample)
	if e != nil {
		return e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `INSERT OR REPLACE INTO samples_v5(ts,body) VALUES(?,?)`, v.GeneratedAt.UnixMilli(), string(b)); e != nil {
		return e
	}
	for _, n := range v.Nodes {
		b, e := json.Marshal(n)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT OR REPLACE INTO node_samples_v5(node,ts,body) VALUES(?,?,?)`, n.Address, v.GeneratedAt.UnixMilli(), string(b)); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (s *Store) History(since time.Time, limit int) ([]model.Sample, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	limit = max(2, min(limit, 2000))
	now := time.Now()
	span := max(int64(0), now.Sub(since).Milliseconds())
	step := max(int64(1), (span+int64(limit)-2)/int64(limit-1))
	rows, e := s.db.QueryContext(ctx, `SELECT body FROM samples_v5 WHERE ts IN (SELECT max(ts) FROM samples_v5 WHERE ts>=? AND ts<=? GROUP BY CAST((ts-?)/? AS INTEGER)) ORDER BY ts LIMIT ?`, since.UnixMilli(), now.UnixMilli(), since.UnixMilli(), step, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := []model.Sample{}
	for rows.Next() {
		var b string
		var a model.Sample
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &a); e != nil {
			return nil, e
		}
		v = append(v, a)
	}
	return core.Downsample(v, limit), rows.Err()
}
func (s *Store) NodeHistory(node string, since time.Time, limit int) ([]model.Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	limit = max(2, min(limit, 2000))
	now := time.Now()
	span := max(int64(0), now.Sub(since).Milliseconds())
	step := max(int64(1), (span+int64(limit)-2)/int64(limit-1))
	rows, e := s.db.QueryContext(ctx, `SELECT body FROM node_samples_v5 WHERE node=? AND ts IN (SELECT max(ts) FROM node_samples_v5 WHERE node=? AND ts>=? AND ts<=? GROUP BY CAST((ts-?)/? AS INTEGER)) ORDER BY ts LIMIT ?`, node, node, since.UnixMilli(), now.UnixMilli(), since.UnixMilli(), step, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := []model.Node{}
	for rows.Next() {
		var b string
		var a model.Node
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &a); e != nil {
			return nil, e
		}
		v = append(v, a)
	}
	return core.Downsample(v, limit), rows.Err()
}
func (s *Store) SaveTopology(v model.Snapshot) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO scope_state_v5(key,body) VALUES('topology',?)`, string(b))
	return e
}
func (s *Store) LoadTopology() (model.Snapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var v model.Snapshot
	var b string
	e := s.db.QueryRowContext(ctx, `SELECT body FROM scope_state_v5 WHERE key='topology'`).Scan(&b)
	if e != nil {
		return v, e
	}
	e = json.Unmarshal([]byte(b), &v)
	return v, e
}
func (s *Store) Prune(hours int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM samples_v5 WHERE ts<?`, `DELETE FROM node_samples_v5 WHERE ts<?`, `DELETE FROM incidents_v5 WHERE active=0 AND updated<? AND (json_extract(body,'$.acknowledged')=1 OR (json_extract(body,'$.recovery_pending') IS NOT 1 AND json_extract(body,'$.notification_pending') IS NOT 1))`} {
		if _, e = tx.ExecContext(ctx, q, cutoff.UnixMilli()); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM events WHERE resolved=1 AND ts<?`, cutoff.UTC().Format(time.RFC3339Nano)); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM events WHERE id NOT IN (SELECT id FROM events ORDER BY id DESC LIMIT 20000)`); e != nil {
		return e
	}
	return tx.Commit()
}

var ErrNotFound = errors.New("not found")
