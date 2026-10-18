package datastore

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/logging"
	"github.com/papaburgs/fluffy-robot/internal/metrics"
)

var retentionWeeks = 5

// vacuumMargin is the safety margin the VACUUM free-space guard requires
// beyond 1.25*DB size. Tests raise it to force a skip.
var vacuumMargin int64 = 32 << 20

// RetentionWeeks returns the configured number of full game weeks to keep
// (parsed once in Init; default 5).
func RetentionWeeks() int {
	if retentionWeeks == 0 {
		return 5 // defensive — tests may construct the store without Init
	}
	return retentionWeeks
}

func parseRetentionWeeks() {
	env, ok := os.LookupEnv("FLUFFY_RETENTION_WEEKS")
	if !ok {
		return
	}
	if w, err := strconv.Atoi(env); err == nil && w >= 1 {
		if w < 4 {
			logging.Error("FLUFFY_RETENTION_WEEKS below the spec floor of 4, ignoring", "value", w)
			return
		}
		retentionWeeks = w
	} else {
		logging.Error("invalid FLUFFY_RETENTION_WEEKS, defaulting to 5", "value", env)
	}
}

// Prune drops the week that just became the (weeks+1)-th-oldest at a reset
// transition, then VACUUMs behind a free-space guard. resetDate is the
// *new* current reset (e.g. "2026-10-12…"); all data from the current
// reset day onward is kept, as are the previous `weeks` full weeks. Runs
// once per reset transition, while the collector's tickers are stopped.
func Prune(weeks int, resetDate Reset) error {
	day, err := parseResetDay(resetDate)
	if err != nil {
		return err
	}
	cutoffTime := day.AddDate(0, 0, -7*weeks)
	cutoffTs := cutoffTime.Unix()
	cutoffReset := cutoffTime.Format("2006-01-02")

	var rowsDeleted int64
	err = inTx(func(tx *sql.Tx) error {
		for _, tbl := range []string{"agent_status", "construction"} {
			res, derr := tx.Exec(`DELETE FROM `+tbl+` WHERE timestamp < ?`, cutoffTs)
			if derr != nil {
				return fmt.Errorf("prune %s: %w", tbl, derr)
			}
			n, _ := res.RowsAffected()
			rowsDeleted += n
		}
		for _, tbl := range []string{"agent_snapshots", "stats", "leaderboards", "jumpgates", "factions"} {
			res, derr := tx.Exec(`DELETE FROM `+tbl+` WHERE reset_date < ?`, cutoffReset)
			if derr != nil {
				return fmt.Errorf("prune %s: %w", tbl, derr)
			}
			n, _ := res.RowsAffected()
			rowsDeleted += n
		}
		return nil
	})
	if err != nil {
		return err
	}
	metrics.DatastorePruneRows.Add(rowsDeleted)
	metrics.DatastorePruneLastRun.Set(time.Now().Unix())

	size, _ := dbSize() // 0 on error is fine: it only tightens the guard
	free, err := freeSpace(path)
	if err != nil {
		logging.Warn("prune: free-space probe failed, skipping VACUUM", err)
		return nil
	}
	if free < int64(float64(size)*1.25)+vacuumMargin {
		logging.Warn("prune: skipping VACUUM, not enough free space", "free", free, "dbBytes", size)
		return nil
	}
	if _, err := db.Exec(`VACUUM`); err != nil {
		logging.Error("prune: VACUUM failed", err)
	}
	return nil
}

// weekStart returns the unix start (Sunday 13:00 UTC) of the current
// reset week — the lower bound for "latest" aggregates.
func weekStart() (int64, error) {
	if currentReset == "" {
		return 0, fmt.Errorf("no current reset yet")
	}
	day, err := parseResetDay(currentReset)
	if err != nil {
		return 0, err
	}
	return day.Unix(), nil
}

func parseResetDay(r Reset) (time.Time, error) {
	s := string(r)
	if len(s) < 10 {
		return time.Time{}, fmt.Errorf("reset date %q too short", s)
	}
	// API resetDate is ISO8601 with a date prefix; anchor the week at the
	// known Sunday 13:00 UTC reset moment.
	day, err := time.ParseInLocation("2006-01-02 15:04", s[:10]+" 13:00", time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse reset date %q: %w", s, err)
	}
	return day, nil
}
