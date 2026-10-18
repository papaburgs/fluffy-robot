package datastore

import (
	"path/filepath"
	"testing"
)

// seedTestDB opens a temp DB named fluffy.db and points the package's
// `path` global at its directory, so dbSize/freeSpace/export (which read
// {path}/fluffy.db) work against the test store.
func seedTestDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path = dir
	d, err := openDB(filepath.Join(dir, "fluffy.db"))
	if err != nil {
		t.Fatalf("seed db: %v", err)
	}
	t.Cleanup(func() { d.Close(); path = "./" })
	db = d
	UpdateReset("2026-10-05")
}

func pubAgents(list ...struct {
	sym, hf, hq string
	cr          int64
	sc          int
},
) []PublicAgent {
	res := make([]PublicAgent, 0, len(list))
	for _, a := range list {
		res = append(res, PublicAgent{
			Symbol: a.sym, StartingFaction: a.hf, Headquarters: a.hq,
			Credits: a.cr, ShipCount: a.sc,
		})
	}
	return res
}

func TestStoreAgents_RoundTrip(t *testing.T) {
	seedTestDB(t)
	now := int64(1_000_000)
	agents := pubAgents(
		struct {
			sym, hf, hq string
			cr          int64
			sc          int
		}{"BORG-1", "COSMIC", "X1-YZ-123", 175000, 2},
		struct {
			sym, hf, hq string
			cr          int64
			sc          int
		}{"BORG-2", "GALACTIC", "X1-YZ-456", 200000, 5},
	)
	if err := StoreAgents(agents, now); err != nil {
		t.Fatalf("StoreAgents: %v", err)
	}
	got, err := GetAgentList(currentReset)
	if err != nil {
		t.Fatalf("GetAgentList: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d agents, want 2", len(got))
	}
	bySym := map[string]Agent{}
	for _, a := range got {
		bySym[a.Symbol] = a
	}
	if bySym["BORG-1"].System != "X1-YZ" || bySym["BORG-2"].System != "X1-YZ" {
		t.Errorf("System not derived from headquarters: %+v", got)
	}
	if bySym["BORG-2"].Credits != 200000 {
		t.Errorf("BORG-2 credits = %d, want 200000", bySym["BORG-2"].Credits)
	}
}

func TestStoreAgents_SnapshotRefreshSeriesAppends(t *testing.T) {
	seedTestDB(t)
	a := pubAgents(struct {
		sym, hf, hq string
		cr          int64
		sc          int
	}{"BORG-1", "COSMIC", "X1-YZ-123", 175000, 2})
	if err := StoreAgents(a, 1_000_000); err != nil {
		t.Fatalf("StoreAgents first: %v", err)
	}
	a2 := pubAgents(struct {
		sym, hf, hq string
		cr          int64
		sc          int
	}{"BORG-1", "COSMIC", "X1-YZ-123", 190000, 3})
	if err := StoreAgents(a2, 1_000_300); err != nil {
		t.Fatalf("StoreAgents second: %v", err)
	}

	// snapshot: only latest credits
	gotList, err := GetAgentList(currentReset)
	if err != nil {
		t.Fatalf("GetAgentList: %v", err)
	}
	for _, g := range gotList {
		if g.Symbol == "BORG-1" && g.Credits != 190000 {
			t.Errorf("snapshot credits = %d, want 190000 (latest wins)", g.Credits)
		}
	}

	// series: both rows present
	hist, err := GetAgentHistory(currentReset, 0, 0, nil)
	if err != nil {
		t.Fatalf("GetAgentHistory: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("series rows = %d, want 2", len(hist))
	}
}

func TestGetAgentHistory_WindowAndAgentFilter(t *testing.T) {
	seedTestDB(t)
	a1 := pubAgents(struct {
		sym, hf, hq string
		cr          int64
		sc          int
	}{"A1", "COSMIC", "X1-YZ-123", 175000, 2})
	// ten 5-minute ticks for A1, plus a second agent B1
	ts := int64(2_000_000)
	for i := 0; i < 10; i++ {
		a1[0].Credits = 175000 + int64(i*1000)
		if err := StoreAgents(a1, ts+int64(i*300)); err != nil {
			t.Fatalf("StoreAgents a1 %d: %v", i, err)
		}
		b1 := pubAgents(struct {
			sym, hf, hq string
			cr          int64
			sc          int
		}{"B1", "GALACTIC", "X1-YZ-456", 50000, 1})
		if err := StoreAgents(b1, ts+int64(i*300)); err != nil {
			t.Fatalf("StoreAgents b1 %d: %v", i, err)
		}
	}

	// window covers only the middle 5 ticks of A1, filter only A1
	// a1 ticks at ts+0..ts+2700 (10 ticks, step 300).
	start := ts + 2*300 // skip first two
	end := ts + 6*300   // skip last three
	hist, err := GetAgentHistory(currentReset, start, end, []string{"A1"})
	if err != nil {
		t.Fatalf("GetAgentHistory: %v", err)
	}
	if len(hist) != 5 {
		t.Fatalf("rows = %d, want 5", len(hist))
	}
	for _, r := range hist {
		if r.Symbol != "A1" {
			t.Errorf("expected only A1 rows, got %q", r.Symbol)
		}
		if r.Timestamp < start || r.Timestamp > end {
			t.Errorf("timestamp %d outside window [%d,%d]", r.Timestamp, start, end)
		}
	}
}

func TestGetAgentHistory_EmptyAgentFilter(t *testing.T) {
	seedTestDB(t)
	a := pubAgents(struct {
		sym, hf, hq string
		cr          int64
		sc          int
	}{"A1", "COSMIC", "X1-YZ-123", 175000, 2})
	if err := StoreAgents(a, 1_000_000); err != nil {
		t.Fatalf("StoreAgents: %v", err)
	}
	hist, err := GetAgentHistory(currentReset, 0, 0, []string{})
	if err != nil {
		t.Fatalf("GetAgentHistory: %v", err)
	}
	if len(hist) != 0 {
		t.Errorf("empty filter returned %d rows, want 0", len(hist))
	}
}

func TestGetAgentHistory_OlderReset(t *testing.T) {
	seedTestDB(t)
	a := pubAgents(struct {
		sym, hf, hq string
		cr          int64
		sc          int
	}{"A1", "COSMIC", "X1-YZ-123", 100, 2})
	if err := StoreAgents(a, 1_000_000); err != nil {
		t.Fatalf("StoreAgents A: %v", err)
	}
	UpdateReset("2026-10-12")
	a[0].Credits = 500
	if err := StoreAgents(a, 1_604_000); err != nil {
		t.Fatalf("StoreAgents A2: %v", err)
	}

	older, err := GetAgentList("2026-10-05")
	if err != nil {
		t.Fatalf("GetAgentList older: %v", err)
	}
	if len(older) != 1 || older[0].Credits != 100 {
		t.Errorf("older reset list = %+v, want one row with credits 100", older)
	}
}

func TestLatestShips(t *testing.T) {
	seedTestDB(t)
	a := pubAgents(struct {
		sym, hf, hq string
		cr          int64
		sc          int
	}{"A1", "COSMIC", "X1-YZ-123", 175000, 2})
	if err := StoreAgents(a, 1_000_000); err != nil {
		t.Fatalf("StoreAgents 1: %v", err)
	}
	a[0].ShipCount = 7
	if err := StoreAgents(a, 1_000_300); err != nil {
		t.Fatalf("StoreAgents 2: %v", err)
	}
	a[0].ShipCount = 4
	if err := StoreAgents(a, 1_000_600); err != nil {
		t.Fatalf("StoreAgents 3: %v", err)
	}
	ships := LatestShips()
	if ships["A1"] != 4 {
		t.Errorf("latest ships = %d, want 4 (max timestamp)", ships["A1"])
	}
}
