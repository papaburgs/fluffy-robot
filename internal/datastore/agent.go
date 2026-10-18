package datastore

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/logging"
)

// StoreAgents writes one cycle of agent observations as a single
// transaction. The snapshot is replaced wholesale for the current reset
// (latest fetch wins); the series is appended with INSERT OR REPLACE so
// minute-truncated/rounded timestamps make retries idempotent.
func StoreAgents(apiAgents []PublicAgent, now int64) error {
	if db == nil {
		return fmt.Errorf("datastore: db not initialized")
	}
	return inTx(func(tx *sql.Tx) error {
		// registry: map every symbol to an id (INSERT OR IGNORE, then read).
		for _, a := range apiAgents {
			if _, err := tx.Exec(
				`INSERT OR IGNORE INTO agents (symbol, first_seen) VALUES (?, ?)`,
				a.Symbol, now,
			); err != nil {
				return fmt.Errorf("registry upsert %q: %w", a.Symbol, err)
			}
		}
		rows, err := tx.Query(`SELECT id, symbol FROM agents`)
		if err != nil {
			return err
		}
		idOf := make(map[string]int64)
		for rows.Next() {
			var id int64
			var sym string
			if err := rows.Scan(&id, &sym); err != nil {
				return err
			}
			idOf[sym] = id
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		snapshotsIns, err := tx.Prepare(
			`INSERT OR REPLACE INTO agent_snapshots
			 (reset_date, agent_id, credits, faction, headquarters, system, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer snapshotsIns.Close()

		// Replace the whole snapshot for this reset (deleted then re-inserted
		// inside the same tx — readers never observe the gap because it
		// never leaves the transaction).
		if _, err := tx.Exec(`DELETE FROM agent_snapshots WHERE reset_date = ?`, string(currentReset)); err != nil {
			return err
		}

		seriesIns, err := tx.Prepare(
			`INSERT OR REPLACE INTO agent_status (agent_id, timestamp, credits, ships)
			 VALUES (?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer seriesIns.Close()

		for _, a := range apiAgents {
			id, ok := idOf[a.Symbol]
			if !ok {
				return fmt.Errorf("agents registry missing symbol %q", a.Symbol)
			}
			if _, err := snapshotsIns.Exec(
				string(currentReset), id, a.Credits, a.StartingFaction, a.Headquarters,
				SystemFromWaypoint(a.Headquarters), now,
			); err != nil {
				return err
			}
			if _, err := seriesIns.Exec(id, now, a.Credits, a.ShipCount); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetAgentList returns the most recent snapshot for the given reset. An
// empty reset string means the current one.
func GetAgentList(reset Reset) ([]Agent, error) {
	resetArg := string(reset)
	if resetArg == "" {
		resetArg = string(currentReset)
	}
	rows, err := db.Query(
		`SELECT a.symbol, s.credits, s.faction, s.headquarters, s.system
		   FROM agent_snapshots s JOIN agents a ON a.id = s.agent_id
		  WHERE s.reset_date = ?`, resetArg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := []Agent{}
	for rows.Next() {
		var sym, faction, hq, system string
		var credits int64
		if err := rows.Scan(&sym, &credits, &faction, &hq, &system); err != nil {
			return nil, err
		}
		res = append(res, Agent{Symbol: sym, Credits: credits, Faction: faction, Headquarters: hq, System: system})
	}
	return res, rows.Err()
}

// GetAgentHistory returns series rows within [start,end] (end 0 ⇒ now) for
// the given agent symbols. An empty slice returns zero rows — the caller
// must pass the requested set explicitly, or the query would load the
// whole reset into RAM (the gob-path bug this backend exists to fix).
// Passing nil means "no filter" for internal callers that genuinely want
// the whole window.
func GetAgentHistory(reset Reset, start, end int64, agents []string) ([]AgentStatus, error) {
	if end == 0 {
		end = time.Now().Unix()
	}
	res := []AgentStatus{}
	if agents != nil && len(agents) == 0 {
		return res, nil
	}

	where := `WHERE s.timestamp >= ? AND s.timestamp <= ?`
	args := []any{start, end}
	if len(agents) > 0 {
		marks := strings.Repeat("?,", len(agents))
		where = `WHERE a.symbol IN (` + strings.TrimSuffix(marks, ",") + `) AND s.timestamp BETWEEN ? AND ?`
		prepended := make([]any, 0, len(agents)+2)
		for _, a := range agents {
			prepended = append(prepended, a)
		}
		prepended = append(prepended, start, end)
		args = prepended
	}

	rows, err := db.Query(
		`SELECT a.symbol, s.timestamp, s.credits, s.ships
		   FROM agent_status s JOIN agents a ON a.id = s.agent_id `+where+
			` ORDER BY s.timestamp`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r AgentStatus
		if err := rows.Scan(&r.Symbol, &r.Timestamp, &r.Credits, &r.Ships); err != nil {
			return nil, err
		}
		res = append(res, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// old callers relied on the slice being time-ordered; ORDER BY already
	// delivers that, but keep the sort as a safety for IN()-query plans
	// (cheap on a bounded window, and the old path cost 2000-file scans).
	sort.Slice(res, func(i, j int) bool { return res[i].Timestamp < res[j].Timestamp })
	return res, nil
}

// LatestShips returns each agent's ships at its max timestamp — the
// aggregate version of the old in-memory latestShips() transform.
func LatestShips() map[string]int64 {
	rows, err := db.Query(
		`SELECT a.symbol, s.ships, MAX(s.timestamp)
		   FROM agent_status s JOIN agents a ON a.id = s.agent_id
		  GROUP BY a.symbol`)
	if err != nil {
		logging.Error("LatestShips:", err)
		return map[string]int64{}
	}
	defer rows.Close()
	res := make(map[string]int64)
	for rows.Next() {
		var sym string
		var ships int64
		var maxTs int64
		if err := rows.Scan(&sym, &ships, &maxTs); err != nil {
			logging.Error("LatestShips scan:", err)
			return map[string]int64{}
		}
		res[sym] = ships
	}
	return res
}
