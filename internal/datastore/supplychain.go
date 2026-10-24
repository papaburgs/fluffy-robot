package datastore

import (
	"database/sql"
	"fmt"

	"github.com/papaburgs/fluffy-robot/internal/logging"
	"github.com/papaburgs/fluffy-robot/internal/metrics"
)

// StoreSupplyChain writes the export→imports map as one (export, import)
// row per pair in a single transaction. INSERT OR REPLACE on the natural
// key makes retries idempotent. The data is static game data and not
// reset-scoped; Prune does not touch this table.
func StoreSupplyChain(chain map[string][]string, now int64) error {
	err := inTx(func(tx *sql.Tx) error {
		ins, err := tx.Prepare(
			`INSERT OR REPLACE INTO supply_chain (export, import, fetched_at)
			 VALUES (?, ?, ?)`)
		if err != nil {
			return err
		}
		defer ins.Close()
		for export, imports := range chain {
			for _, imp := range imports {
				if _, err := ins.Exec(export, imp, now); err != nil {
					return fmt.Errorf("supply_chain upsert %q→%q: %w", export, imp, err)
				}
			}
		}
		return nil
	})
	if err == nil {
		metrics.DatastoreWrites.Add(1)
	}
	return err
}

// SupplyChainExists reports whether the chain has ever been collected.
// False on error so the collector retries the fetch (safe direction).
func SupplyChainExists() bool {
	metrics.DatastoreReads.Add(1)
	var exists bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM supply_chain LIMIT 1)`).Scan(&exists); err != nil {
		logging.Error("SupplyChainExists:", err)
		return false
	}
	return exists
}
