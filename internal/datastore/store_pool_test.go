package datastore

import (
	"path/filepath"
	"testing"
)

// Every pooled connection must carry the pragmas — not just whichever
// connection happened to run the Exec at open. A second connection is
// forced by holding the first one inside an open transaction.
func TestOpenDB_PragmaOnEveryConnection(t *testing.T) {
	d, err := openDB(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer d.Close()

	tx, err := d.Begin() // holds connection #1
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()

	// The pool must check out a *new* connection for this read.
	var busy, fk, sync, cache int
	row := d.QueryRow(`PRAGMA busy_timeout`)
	if err := row.Scan(&busy); err != nil {
		t.Fatalf("busy_timeout read: %v", err)
	}
	row = d.QueryRow(`PRAGMA foreign_keys`)
	if err := row.Scan(&fk); err != nil {
		t.Fatalf("foreign_keys read: %v", err)
	}
	row = d.QueryRow(`PRAGMA synchronous`)
	if err := row.Scan(&sync); err != nil {
		t.Fatalf("synchronous read: %v", err)
	}
	row = d.QueryRow(`PRAGMA cache_size`)
	if err := row.Scan(&cache); err != nil {
		t.Fatalf("cache_size read: %v", err)
	}
	if busy != 10000 || fk != 1 || sync != 2 || cache != -4000 {
		t.Errorf("second connection pragmas: busy=%d fk=%d sync=%d cache=%d; want 10000/1/2/-4000", busy, fk, sync, cache)
	}
}
