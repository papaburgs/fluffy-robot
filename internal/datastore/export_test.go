package datastore

import (
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportDB_ValidSnapshot(t *testing.T) {
	seedTestDB(t)
	five := 5
	if err := StoreStats(statusResp("2026-10-05", 42, &five)); err != nil {
		t.Fatalf("seed stats: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "export.db")
	if err := ExportDB(dst); err != nil {
		t.Fatalf("ExportDB: %v", err)
	}

	// The snapshot is a complete, openable sqlite db.
	snap, err := sql.Open("sqlite", dst)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer snap.Close()
	var n int
	if err := snap.QueryRow(`SELECT count(*) FROM stats WHERE reset_date='2026-10-05'`).Scan(&n); err != nil {
		t.Fatalf("query snapshot: %v", err)
	}
	if n != 1 {
		t.Errorf("snapshot stats rows = %d, want 1", n)
	}
	// Source stays usable after export.
	var agents int
	if err := db.QueryRow(`SELECT agents FROM stats`).Scan(&agents); err != nil || agents != 42 {
		t.Errorf("source db unreadable after export: agents=%v err=%v", agents, err)
	}
}

func TestExportDB_SkippedWhenSpaceShort(t *testing.T) {
	seedTestDB(t)
	old := vacuumMargin
	vacuumMargin = math.MaxInt64 / 2
	t.Cleanup(func() { vacuumMargin = old })

	dst := filepath.Join(t.TempDir(), "export.db")
	err := ExportDB(dst)
	if err == nil {
		t.Fatal("ExportDB must fail when the free-space guard trips")
	}
	if got := err.Error(); !strings.Contains(got, "free space") {
		t.Errorf("error %q should mention free space", got)
	}
	if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
		t.Errorf("dst file must not exist after a guarded skip")
	}
}
