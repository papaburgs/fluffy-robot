package datastore

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/metrics"
)

func fastBackoff(t *testing.T) {
	t.Helper()
	retryBackoff = time.Millisecond
	t.Cleanup(func() { retryBackoff = time.Second })
}

var errTransient = errors.New("transient write failure")

func TestInTx_RetryTransient(t *testing.T) {
	fastBackoff(t)
	seedTestDB(t)

	attempts := 0
	retryBase := metrics.DatastoreWriteRetries.Value()
	err := inTx(func(tx *sql.Tx) error {
		attempts++
		if attempts < 3 {
			return errTransient // fail attempts 1-2; succeed on 3
		}
		_, execErr := tx.Exec(`INSERT INTO agents (symbol, first_seen) VALUES ('RETRY-1', 7)`)
		return execErr
	})
	if err != nil {
		t.Fatalf("inTx should have succeeded after retries: %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
	if got := metrics.DatastoreWriteRetries.Value() - retryBase; got != 2 {
		t.Errorf("retry counter +%d, want +2", got)
	}
	var sym string
	if err := db.QueryRow(`SELECT symbol FROM agents WHERE symbol = 'RETRY-1'`).Scan(&sym); err != nil {
		t.Errorf("committed row missing: %v", err)
	}
}

func TestInTx_DropAfterBound(t *testing.T) {
	fastBackoff(t)
	seedTestDB(t)

	attempts := 0
	retryBase := metrics.DatastoreWriteRetries.Value()
	errBase := metrics.DatastoreWriteErrors.Value()
	err := inTx(func(tx *sql.Tx) error {
		attempts++
		_, execErr := tx.Exec(`INSERT INTO agents (symbol, first_seen) VALUES ('NEVER-SEE-ME', 1)`)
		if execErr != nil {
			return execErr
		}
		return errTransient
	})
	if err == nil {
		t.Fatal("inTx must return an error after the attempt bound")
	}
	if attempts != 7 {
		t.Errorf("attempts = %d, want 7 (6 retries + final)", attempts)
	}
	if got := metrics.DatastoreWriteRetries.Value() - retryBase; got != 6 {
		t.Errorf("retry counter +%d, want +6", got)
	}
	if got := metrics.DatastoreWriteErrors.Value() - errBase; got != 1 {
		t.Errorf("error counter +%d, want +1", got)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM agents WHERE symbol = 'NEVER-SEE-ME'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("row committed from a failed cycle — partial cycles must not exist")
	}
}
