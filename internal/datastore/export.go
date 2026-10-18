package datastore

import (
	"fmt"
	"os"

	"github.com/papaburgs/fluffy-robot/internal/logging"
)

// ExportDB writes a consistent snapshot of the database to dst using
// VACUUM INTO (safe against WAL state — the snapshot is a compacted,
// standalone file). Refuses when free space is short: an ENOSPC mid-copy
// could starve the collector's writes, which is the worst outcome.
func ExportDB(dst string) error {
	size, _ := dbSize() // 0 on error only tightens the guard
	free, err := freeSpace(path)
	if err != nil {
		return fmt.Errorf("export: free-space probe: %w", err)
	}
	if free < int64(float64(size)*1.25)+vacuumMargin {
		return fmt.Errorf("export skipped: not enough free space (free=%d, db=%d)", free, size)
	}
	logging.Info("export: VACUUM INTO snapshot", "dst", dst, "dbBytes", size)
	if _, err := db.Exec(`VACUUM INTO ?`, dst); err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("export: %w", err)
	}
	return nil
}
