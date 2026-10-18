package datastore

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenDB_PrivilegedState(t *testing.T) {
	d, err := openDB(filepath.Join(t.TempDir(), "fluffy.db"))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer d.Close()

	var s string
	var i int
	if err := d.QueryRow("PRAGMA journal_mode").Scan(&s); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if strings.ToLower(s) != "wal" {
		t.Errorf("journal_mode = %q, want wal", s)
	}
	if err := d.QueryRow("PRAGMA synchronous").Scan(&i); err != nil {
		t.Fatalf("synchronous: %v", err)
	}
	if i != 2 {
		t.Errorf("synchronous = %d, want 2 (FULL)", i)
	}
	if err := d.QueryRow("PRAGMA foreign_keys").Scan(&i); err != nil {
		t.Fatalf("foreign_keys: %v", err)
	}
	if i != 1 {
		t.Errorf("foreign_keys = %d, want 1", i)
	}
	if err := d.QueryRow("PRAGMA cache_size").Scan(&i); err != nil {
		t.Fatalf("cache_size: %v", err)
	}
	if i != -4000 {
		t.Errorf("cache_size = %d, want -4000 (4 MiB)", i)
	}
	var uv int
	if err := d.QueryRow("PRAGMA user_version").Scan(&uv); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	if uv != schemaVersion {
		t.Errorf("user_version = %d, want %d", uv, schemaVersion)
	}

	want := map[string]bool{
		"agents": true, "agent_snapshots": true, "agent_status": true,
		"stats": true, "leaderboards": true, "jumpgates": true,
		"construction": true, "factions": true,
	}
	rows, err := d.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		t.Fatalf("sqlite_master: %v", err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		got[name] = true
	}
	for tbl := range want {
		if !got[tbl] {
			t.Errorf("missing table %q", tbl)
		}
	}

	// WITHOUT ROWID tables must be index-b-tree (clustered on primary key).
	var sqlText string
	if err := d.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='agent_status'`).Scan(&sqlText); err != nil {
		t.Fatalf("agent_status sql: %v", err)
	}
	if !strings.Contains(strings.ToUpper(sqlText), "WITHOUT ROWID") {
		t.Errorf("agent_status not WITHOUT ROWID: %s", sqlText)
	}
}

func TestInTx_CommitsAndRollsBack(t *testing.T) {
	d, err := openDB(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer d.Close()
	db = d // package-global used by inTx

	err = inTx(func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO agents(symbol, first_seen) VALUES('A1', 1)`)
		return err
	})
	if err != nil {
		t.Fatalf("committing inTx: %v", err)
	}
	var n int
	if err := d.QueryRow(`SELECT count(*) FROM agents`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("after commit, rows = %d, want 1", n)
	}

	err = inTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO agents(symbol, first_seen) VALUES('A2', 1)`); err != nil {
			return err
		}
		return errRollback // sentinel forces rollback
	})
	if err != errRollback {
		t.Errorf("inTx err = %v, want errRollback", err)
	}
	if err := d.QueryRow(`SELECT count(*) FROM agents`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("after rollback, rows = %d, want 1 (A2 must not be visible)", n)
	}
}

func TestFreeSpace_Positive(t *testing.T) {
	dir := t.TempDir()
	b, err := freeSpace(dir)
	if err != nil {
		t.Fatalf("freeSpace: %v", err)
	}
	if b <= 0 {
		t.Errorf("freeSpace = %d, want > 0", b)
	}
}

var errRollback = errors.New("force rollback")
