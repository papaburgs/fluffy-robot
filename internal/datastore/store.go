package datastore

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	_ "modernc.org/sqlite"
)

// db is the single process-wide connection. WAL lets concurrent readers
// proceed while the collector (the only writer) holds the write lock.
var db *sql.DB

func openDB(dbPath string) (*sql.DB, error) {
	d, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	// WAL needs concurrent readers + one writer; the pool stays small.
	d.SetMaxOpenConns(4)
	pragmas := []string{
		"journal_mode = WAL",
		"synchronous = FULL",
		"busy_timeout = 10000",
		"foreign_keys = ON",
		"cache_size = -4000",
	}
	for _, p := range pragmas {
		if _, err := d.Exec("PRAGMA " + p); err != nil {
			return nil, fmt.Errorf("pragma %s: %w", p, err)
		}
	}
	if err := applySchema(d); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return d, nil
}

// inTx runs fn in a single transaction, committing on nil error or
// rolling back otherwise. Retry/backoff live in Task 6.
func inTx(fn func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func freeSpace(path string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, fmt.Errorf("statfs: %w", err)
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}

func dbSize() (int64, error) {
	// main db + WAL; when the WAL is empty statfs on it just ignores.
	mainInfo, err := os.Stat(filepath.Join(path, "fluffy.db"))
	if err != nil {
		return 0, err
	}
	walInfo, err := os.Stat(filepath.Join(path, "fluffy.db-wal"))
	wal := int64(0)
	if err == nil {
		wal = walInfo.Size()
	}
	return mainInfo.Size() + wal, nil
}
