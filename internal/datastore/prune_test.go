package datastore

import (
	"database/sql"
	"fmt"
	"math"
	"testing"
	"time"
)

// weekStarts returns reset-date strings (Sunday 13:00 UTC cadence) for n
// consecutive weeks ending at the given newest date.
func weekStarts(n int, newest string) []string {
	base, _ := time.Parse("2006-01-02", newest)
	res := make([]string, 0, n)
	for i := n - 1; i >= 0; i-- {
		res = append(res, base.AddDate(0, 0, -7*i).Format("2006-01-02"))
	}
	return res
}

func utcTs(date string, hour int) int64 {
	t, _ := time.ParseInLocation(time.DateTime, date+" 13:00:00", time.UTC)
	return t.Add(time.Duration(hour-13) * time.Hour).Unix()
}

func TestPrune_KeepsFiveFullWeeks(t *testing.T) {
	seedTestDB(t)
	weeks := weekStarts(7, "2026-10-12") // W1=2026-08-31 … W7=2026-10-12 (current)
	// one series row per week, mid-week (Wednesday 13:00)
	for i, w := range weeks {
		day, _ := time.Parse("2006-01-02", w)
		ts := day.AddDate(0, 0, 3).Add(13 * time.Hour).Unix()
		if err := inTx(func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT OR IGNORE INTO agents (symbol, first_seen) VALUES ('P-1', 1)`)
			if err != nil {
				return err
			}
			var id int64
			if err := tx.QueryRow(`SELECT id FROM agents WHERE symbol='P-1'`).Scan(&id); err != nil {
				return err
			}
			_, err = tx.Exec(`INSERT OR REPLACE INTO agent_status (agent_id, timestamp, credits, ships) VALUES (?, ?, ?, ?)`,
				id, ts, int64(1000*i+1), 1)
			return err
		}); err != nil {
			t.Fatalf("seed week %d: %v", i, err)
		}
	}

	if err := Prune(5, Reset("2026-10-12")); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM agent_status`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 6 {
		t.Errorf("agent_status rows = %d, want 6 (5 kept weeks + current week)", n)
	}
	// the dropped row is the oldest week's
	var minTs int64
	if err := db.QueryRow(`SELECT MIN(timestamp) FROM agent_status`).Scan(&minTs); err != nil {
		t.Fatalf("min ts: %v", err)
	}
	w2Day, _ := time.Parse("2006-01-02", weeks[1])
	wantMin := w2Day.AddDate(0, 0, 3).Add(13 * time.Hour).Unix()
	if minTs != wantMin {
		t.Errorf("min timestamp = %d, want %d (week W2 Wednesday — W1 dropped)", minTs, wantMin)
	}
}

func TestPrune_BoundaryRowKept(t *testing.T) {
	seedTestDB(t)
	weeks := weekStarts(7, "2026-10-12")
	// cutoff = weekstart(W7) - 5*7d = weekstart(W2). Seed a row exactly there.
	cutoff, _ := time.Parse("2006-01-02", weeks[1])
	cutoffTs := cutoff.Add(13 * time.Hour).Unix()
	if err := inTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO agents (symbol, first_seen) VALUES ('P-1', 1)`); err != nil {
			return err
		}
		var id int64
		if err := tx.QueryRow(`SELECT id FROM agents WHERE symbol='P-1'`).Scan(&id); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT OR REPLACE INTO agent_status (agent_id, timestamp, credits, ships) VALUES (?, ?, ?, ?)`,
			id, cutoffTs, 42, 1)
		return err
	}); err != nil {
		t.Fatalf("seed boundary: %v", err)
	}
	if err := Prune(5, Reset("2026-10-12")); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	var credits int64
	err := db.QueryRow(`SELECT credits FROM agent_status WHERE timestamp = ?`, cutoffTs).Scan(&credits)
	if err != nil {
		t.Fatalf("boundary row must be kept (delete is strictly < cutoff): %v", err)
	}
	if credits != 42 {
		t.Errorf("boundary row credits = %d, want 42", credits)
	}
}

func TestPrune_PerResetTablesCut(t *testing.T) {
	seedTestDB(t)
	weeks := weekStarts(7, "2026-10-12")
	for _, w := range weeks {
		if err := StoreStats(statusResp(w, 7, nil)); err != nil {
			t.Fatalf("StoreStats %s: %v", w, err)
		}
		var r ResponseStatus
		r.ResetDate = w
		r.Leaderboards.MostCredits = append(r.Leaderboards.MostCredits,
			struct {
				AgentSymbol string `json:"agentSymbol"`
				Credits     int64  `json:"credits"`
			}{"A1", 100})
		if err := StoreLeaderboards(r); err != nil {
			t.Fatalf("StoreLeaderboards %s: %v", w, err)
		}
	}
	// StoreStats/StoreLeaderboards use currentReset via UpdateReset in
	// their caller; direct SQL for the remaining per-reset tables here.
	for _, w := range weeks {
		if err := inTx(func(tx *sql.Tx) error {
			if _, err := tx.Exec(`INSERT OR REPLACE INTO jumpgates (reset_date, system, jumpgate, headquarters, status, completed_at) VALUES (?, 'X1-A', 'X1-A-J1', 'X1-A-Q1', 1, 0)`, w); err != nil {
				return err
			}
			_, err := tx.Exec(`INSERT OR REPLACE INTO factions (reset_date, symbol, name, description, headquarters, traits, is_recruiting) VALUES (?, 'COS', 'Cosmic', 'd', 'X1-Q1', '[]', 1)`, w)
			return err
		}); err != nil {
			t.Fatalf("seed %s: %v", w, err)
		}
	}
	if err := Prune(5, Reset("2026-10-12")); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	for _, tbl := range []string{"stats", "leaderboards", "jumpgates", "factions"} {
		var n int
		if err := db.QueryRow(fmt.Sprintf(`SELECT count(*) FROM %s`, tbl)).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", tbl, err)
		}
		if n != 6 {
			t.Errorf("%s rows = %d, want 6 (5 full weeks + current)", tbl, n)
		}
	}
}

func TestPrune_VacuumSkippedWhenSpaceShort(t *testing.T) {
	seedTestDB(t)
	// Force the guard: require more margin than any filesystem has.
	old := vacuumMargin
	vacuumMargin = math.MaxInt64 / 2
	t.Cleanup(func() { vacuumMargin = old })

	weeks := weekStarts(7, "2026-10-12")
	day, _ := time.Parse("2006-01-02", weeks[0])
	ts := day.Add(13 * time.Hour).Unix()
	if err := inTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO agents (symbol, first_seen) VALUES ('P-1', 1)`); err != nil {
			return err
		}
		var id int64
		if err := tx.QueryRow(`SELECT id FROM agents WHERE symbol='P-1'`).Scan(&id); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT OR REPLACE INTO agent_status (agent_id, timestamp, credits, ships) VALUES (?, ?, 1, 1)`, id, ts)
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := Prune(5, Reset("2026-10-12")); err != nil {
		t.Fatalf("Prune with short space must still succeed (only VACUUM is skipped): %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM agent_status`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("agent_status rows = %d, want 0 — prune ran despite VACUUM skip", n)
	}
}

func TestRetentionWeeks_Default(t *testing.T) {
	retentionWeeks = 0
	v := RetentionWeeks()
	if v != 5 {
		t.Errorf("RetentionWeeks with no env = %d, want 5", v)
	}
}
