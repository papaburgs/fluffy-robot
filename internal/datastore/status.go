package datastore

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/logging"
)

// StoreStats upserts the status row for the current reset. A missing
// stats.accounts from the API becomes 0 instead of panicking (#5).
func StoreStats(r ResponseStatus) error {
	accounts := 0
	if r.Stats.Accounts != nil {
		accounts = *r.Stats.Accounts
	}
	return inTx(func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT OR REPLACE INTO stats
			(reset_date, market_update, agents, accounts, ships, systems, waypoints,
			 status, version, next_reset, last_update)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.ResetDate, r.Health.LastMarketUpdate.Unix(), r.Stats.Agents, accounts,
			r.Stats.Ships, r.Stats.Systems, r.Stats.Waypoints,
			r.Status, r.Version, r.ServerResets.Next.Unix(), time.Now().Unix())
		return err
	})
}

// StoreLeaderboards replaces the current reset's leaderboard rows
// (latest fetch wins). Credits and charts are separate kinds, keyed by
// rank within the reset.
func StoreLeaderboards(r ResponseStatus) error {
	return inTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM leaderboards WHERE reset_date = ?`, r.ResetDate); err != nil {
			return err
		}
		ins, err := tx.Prepare(`INSERT OR REPLACE INTO leaderboards (reset_date, kind, rank, symbol, value) VALUES (?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer ins.Close()
		for i, x := range r.Leaderboards.MostSubmittedCharts {
			if _, err := ins.Exec(r.ResetDate, "charts", i+1, x.AgentSymbol, int64(x.ChartCount)); err != nil {
				return err
			}
		}
		for i, x := range r.Leaderboards.MostCredits {
			if _, err := ins.Exec(r.ResetDate, "credits", i+1, x.AgentSymbol, x.Credits); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetStats returns the status row for the given reset; empty reset means
// the latest one. Cross-reset reads work (#4 fix: reset is a column).
func GetStats(reset Reset) (Stats, error) {
	res := Stats{}
	resetArg := string(reset)
	if resetArg == "" {
		resetArg = string(currentReset)
		if resetArg == "" {
			// Fresh boot, pre-first-status: no resets yet. Return empty
			// rather than a confusing SQL error.
			return res, nil
		}
	}
	var marketUpdate, nextReset, lastUpdate int64
	err := db.QueryRow(`SELECT reset_date, market_update, agents, accounts, ships,
		systems, waypoints, status, version, next_reset, last_update
		FROM stats WHERE reset_date = ?`, resetArg).Scan(
		&res.Reset, &marketUpdate, &res.Agents, &res.Accounts, &res.Ships,
		&res.Systems, &res.Waypoints, &res.Status, &res.Version, &nextReset, &lastUpdate)
	if err == sql.ErrNoRows {
		return res, fmt.Errorf("no stats for reset %q", resetArg)
	}
	if err != nil {
		return res, err
	}
	res.MarketUpdate = time.Unix(marketUpdate, 0)
	res.NextReset = time.Unix(nextReset, 0)
	res.LastUpdate = time.Unix(lastUpdate, 0)
	return res, nil
}

// GetLeaderboard returns (credits, charts) ranked lists for the given
// reset; empty reset means the latest one.
func GetLeaderboard(reset Reset) ([]LeaderboardEntry, []LeaderboardEntry, error) {
	resetArg := string(reset)
	if resetArg == "" {
		resetArg = string(currentReset)
		if resetArg == "" {
			return nil, nil, nil
		}
	}
	rows, err := db.Query(`SELECT kind, symbol, value FROM leaderboards
		WHERE reset_date = ? ORDER BY kind, rank`, resetArg)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	credits := []LeaderboardEntry{}
	charts := []LeaderboardEntry{}
	for rows.Next() {
		var kind string
		var e LeaderboardEntry
		if err := rows.Scan(&kind, &e.Symbol, &e.Value); err != nil {
			return nil, nil, err
		}
		if kind == "credits" {
			credits = append(credits, e)
		} else {
			charts = append(charts, e)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return credits, charts, nil
}

// AllResets returns every reset date stored, newest first.
func AllResets() []string {
	rows, err := db.Query(`SELECT DISTINCT reset_date FROM stats ORDER BY reset_date DESC`)
	if err != nil {
		logging.Error("AllResets:", err)
		return []string{}
	}
	defer rows.Close()
	res := []string{}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			logging.Error("AllResets scan:", err)
			return []string{}
		}
		res = append(res, d)
	}
	return res
}

// LatestReset busy-waits until the collector has set currentReset — same
// behavior as the old path (blocks on fresh boot until the first status).
func LatestReset() Reset {
	for {
		if currentReset == "" {
			time.Sleep(time.Second)
		} else {
			break
		}
	}
	return currentReset
}

// NextReset reads the next-reset timestamp from the current reset's stats
// row. Preserved for now; deleted if unreferenced at Task 11.
func NextReset() time.Time {
	for {
		if currentReset == "" {
			time.Sleep(time.Second)
		} else {
			break
		}
	}
	st, err := GetStats(currentReset)
	if err != nil {
		logging.Error("error loading stats for NextReset", err)
		return time.Time{}
	}
	return st.NextReset
}
