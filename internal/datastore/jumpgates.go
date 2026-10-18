package datastore

import (
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/logging"
)

// UpdateJumpGates replaces the current reset's jumpgate list (latest wins)
// in one transaction.
func UpdateJumpGates(jgList []JGInfo) error {
	return inTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM jumpgates WHERE reset_date = ?`, string(currentReset)); err != nil {
			return err
		}
		ins, err := tx.Prepare(`INSERT OR REPLACE INTO jumpgates
			(reset_date, system, jumpgate, headquarters, status, completed_at)
			VALUES (?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer ins.Close()
		for _, j := range jgList {
			if _, err := ins.Exec(string(currentReset), j.System, j.Jumpgate, j.Headquarters, int(j.Status), j.Complete); err != nil {
				return err
			}
		}
		return nil
	})
}

func resetOrCurrent(reset Reset) string {
	if reset == "" {
		return string(currentReset)
	}
	return string(reset)
}

func GetJumpgateList(reset Reset) ([]JGInfo, error) {
	rows, err := db.Query(`SELECT system, jumpgate, headquarters, status, completed_at
		FROM jumpgates WHERE reset_date = ?`, resetOrCurrent(reset))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := []JGInfo{}
	for rows.Next() {
		var j JGInfo
		var status int
		if err := rows.Scan(&j.System, &j.Jumpgate, &j.Headquarters, &status, &j.Complete); err != nil {
			return nil, err
		}
		j.Status = ConstructionStatus(status)
		res = append(res, j)
	}
	return res, rows.Err()
}

// updateGateStatus sets the given statuses on the current reset's gates.
func updateGateStatus(systems []string, status ConstructionStatus, completedAt int64) error {
	marks := strings.TrimSuffix(strings.Repeat("?,", len(systems)), ",")
	args := []any{int(status)}
	if status == Complete {
		args = append(args, completedAt)
	}
	args = append(args, string(currentReset))
	for _, s := range systems {
		args = append(args, s)
	}
	setClause := `status = ?`
	if status == Complete {
		setClause = `status = ?, completed_at = ?`
	}
	return inTx(func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE jumpgates SET `+setClause+
			` WHERE reset_date = ? AND system IN (`+marks+`)`, args...)
		return err
	})
}

func MarkJumpgatesComplete(jgs []string, ts int64) error {
	return updateGateStatus(jgs, Complete, ts)
}

func MarkJumpgatesStarted(jgs []string) error {
	return updateGateStatus(jgs, Const, 0)
}

func AddConstructions(cList []JGConstruction, ts int64) error {
	// ts is the cycle's write timestamp; each record carries its own
	// Timestamp (the collector stamps them identically), which is the
	// natural key. INSERT OR REPLACE keeps retries idempotent.
	return inTx(func(tx *sql.Tx) error {
		ins, err := tx.Prepare(`INSERT OR REPLACE INTO construction (jumpgate, timestamp, fabmat, advcct)
			VALUES (?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer ins.Close()
		for _, c := range cList {
			if _, err := ins.Exec(c.Jumpgate, c.Timestamp, c.Fabmat, c.Advcct); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetConstructions returns construction rows within [start,end] (end 0 =>
// now), time-ordered. The window is applied in SQL — no load-then-filter.
func GetConstructions(reset Reset, start, end int64) ([]JGConstruction, error) {
	if end == 0 {
		end = time.Now().Unix()
	}
	rows, err := db.Query(`SELECT timestamp, jumpgate, fabmat, advcct
		FROM construction WHERE timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := []JGConstruction{}
	for rows.Next() {
		var r JGConstruction
		if err := rows.Scan(&r.Timestamp, &r.Jumpgate, &r.Fabmat, &r.Advcct); err != nil {
			return nil, err
		}
		res = append(res, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Timestamp < res[j].Timestamp })
	return res, nil
}

// LatestConstructions returns the most recent construction row per gate,
// bounded to the current reset week (same bound as LatestShips — see its
// doc comment). Empty currentReset (fresh boot) returns nothing.
func LatestConstructions() []JGConstruction {
	bound, err := weekStart()
	if err != nil {
		return []JGConstruction{}
	}
	rows, err := db.Query(`SELECT jumpgate, fabmat, advcct, MAX(timestamp)
		FROM construction WHERE timestamp >= ? GROUP BY jumpgate`, bound)
	if err != nil {
		logging.Error("LatestConstructions:", err)
		return []JGConstruction{}
	}
	defer rows.Close()
	res := []JGConstruction{}
	for rows.Next() {
		var r JGConstruction
		if err := rows.Scan(&r.Jumpgate, &r.Fabmat, &r.Advcct, &r.Timestamp); err != nil {
			logging.Error("LatestConstructions scan:", err)
			return []JGConstruction{}
		}
		res = append(res, r)
	}
	return res
}

func jumpgatesByFilter(reset Reset, filter func(JGInfo) bool) map[string]JGInfo {
	current, err := GetJumpgateList(reset)
	if err != nil {
		logging.Error("error loading jumpgates:", err)
		return nil
	}
	res := make(map[string]JGInfo)
	for _, j := range current {
		if filter == nil || filter(j) {
			res[j.System] = j
		}
	}
	return res
}

func GetJumpgates(reset Reset) map[string]JGInfo {
	return jumpgatesByFilter(reset, nil)
}

func GetJumpgatesUnderConst(reset Reset) map[string]JGInfo {
	return jumpgatesByFilter(reset, func(j JGInfo) bool { return j.Status == Const })
}

func GetJumpgatesNotStarted(reset Reset) map[string]JGInfo {
	return jumpgatesByFilter(reset, func(j JGInfo) bool { return j.Status == Active })
}

func GetJumpgatesComplete(reset Reset) []JGInfo {
	m := jumpgatesByFilter(reset, func(j JGInfo) bool { return j.Status == Complete })
	out := make([]JGInfo, 0, len(m))
	for _, j := range m {
		out = append(out, j)
	}
	return out
}

type ConstructionRecord struct {
	Timestamp int64
	Fabmat    int
	Advcct    int
}
