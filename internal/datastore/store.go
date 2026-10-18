package datastore

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"github.com/papaburgs/fluffy-robot/internal/metrics"
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

// retryBackoff is the base backoff between failed cycle-write attempts
// (1s, 2s, 4s, … up to 64s; 7 attempts ≈ 2 min worst case, then the
// cycle is dropped loudly). Tests shrink it.
var retryBackoff = time.Second

// inTx runs fn in a single transaction, committing on nil error and
// rolling back otherwise. A failed cycle write (transaction or commit
// failure) is retried with exponential backoff — the data was fetched
// from the API once and the spec treats silently dropping a cycle as the
// worst outcome; retries coalesce because writes are INSERT OR REPLACE on
// natural keys. After 7 attempts the last error is returned to the caller,
// which logs it; counters make the failure visible in /debug/vars.
func inTx(fn func(tx *sql.Tx) error) error {
	var err error
	for attempt := 0; attempt < 7; attempt++ {
		if attempt > 0 {
			metrics.DatastoreWriteRetries.Add(1)
			time.Sleep(retryBackoff << (attempt - 1))
		}
		err = func() error {
			tx, terr := db.Begin()
			if terr != nil {
				return terr
			}
			if err := fn(tx); err != nil {
				_ = tx.Rollback()
				return err
			}
			return tx.Commit()
		}()
		if err == nil {
			return nil
		}
	}
	metrics.DatastoreWriteErrors.Add(1)
	return err
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
