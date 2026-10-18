package datastore

import "testing"

func statusResp(reset string, agents int, accounts *int) ResponseStatus {
	var r ResponseStatus
	r.ResetDate = reset
	r.Stats.Agents = agents
	r.Stats.Accounts = accounts
	r.Stats.Ships = 10
	r.Stats.Systems = 5
	r.Stats.Waypoints = 100
	r.Status = "ACTIVE"
	r.Version = "v1"
	// r.ServerResets.Next stays zero for these round-trips
	return r
}

func TestStoreStats_NilAccountsIsZero(t *testing.T) { // #5 fix
	seedTestDB(t)
	r := statusResp("2026-10-05", 3, nil) // accounts omitted
	if err := StoreStats(r); err != nil {
		t.Fatalf("StoreStats: %v", err)
	}
	got, err := GetStats(currentReset)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if got.Accounts != 0 {
		t.Errorf("accounts = %d, want 0 when API omits stats.accounts", got.Accounts)
	}
}

func TestStats_RoundTripLatest(t *testing.T) {
	seedTestDB(t) // current = 2026-10-05
	five := 5
	if err := StoreStats(statusResp("2026-10-05", 10, &five)); err != nil {
		t.Fatalf("StoreStats w1: %v", err)
	}
	UpdateReset("2026-10-12")
	if err := StoreStats(statusResp("2026-10-12", 20, &five)); err != nil {
		t.Fatalf("StoreStats w2: %v", err)
	}
	got, err := GetStats("") // current-reset convention
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if got.Reset != "2026-10-12" || got.Agents != 20 {
		t.Errorf("latest stats = %+v, want reset 2026-10-12 agents 20", got)
	}
	if got.Accounts != 5 {
		t.Errorf("accounts = %d, want 5", got.Accounts)
	}
}

func TestGetStats_OlderReset(t *testing.T) { // cross-reset fix (#4)
	seedTestDB(t)                            // current = 2026-10-05
	five := 5
	if err := StoreStats(statusResp("2026-10-05", 10, &five)); err != nil {
		t.Fatalf("StoreStats A: %v", err)
	}
	UpdateReset("2026-10-12")
	if err := StoreStats(statusResp("2026-10-12", 20, &five)); err != nil {
		t.Fatalf("StoreStats B: %v", err)
	}
	got, err := GetStats("2026-10-05")
	if err != nil {
		t.Fatalf("GetStats older: %v", err)
	}
	if got.Agents != 10 {
		t.Errorf("older reset agents = %d, want 10", got.Agents)
	}
}

func TestLeaderboards_RoundTrip(t *testing.T) {
	seedTestDB(t)
	var r ResponseStatus
	r.ResetDate = "2026-10-05"
	r.Leaderboards.MostCredits = append(r.Leaderboards.MostCredits,
		struct {
			AgentSymbol string `json:"agentSymbol"`
			Credits     int64  `json:"credits"`
		}{"A1", 100}, struct {
			AgentSymbol string `json:"agentSymbol"`
			Credits     int64  `json:"credits"`
		}{"B1", 200})
	r.Leaderboards.MostSubmittedCharts = append(r.Leaderboards.MostSubmittedCharts,
		struct {
			AgentSymbol string `json:"agentSymbol"`
			ChartCount  int    `json:"chartCount"`
		}{"C1", 7})
	if err := StoreLeaderboards(r); err != nil {
		t.Fatalf("StoreLeaderboards A: %v", err)
	}
	UpdateReset("2026-10-12")
	r.ResetDate = "2026-10-12"
	r.Leaderboards.MostCredits[0].Credits = 999
	if err := StoreLeaderboards(r); err != nil {
		t.Fatalf("StoreLeaderboards B: %v", err)
	}

	cred, charts, err := GetLeaderboard("") // latest
	if err != nil {
		t.Fatalf("GetLeaderboard: %v", err)
	}
	if len(cred) != 2 || cred[0].Symbol != "A1" || cred[0].Value != 999 {
		t.Errorf("credits list = %+v, want A1=999 then B1=200", cred)
	}
	if len(charts) != 1 || charts[0].Symbol != "C1" || charts[0].Value != 7 {
		t.Errorf("charts list = %+v, want C1=7", charts)
	}

	oc, _, err := GetLeaderboard("2026-10-05")
	if err != nil {
		t.Fatalf("GetLeaderboard older: %v", err)
	}
	if oc[0].Value != 100 {
		t.Errorf("older reset credits[0].Value = %d, want 100", oc[0].Value)
	}
}

func TestAllResets_OrderedDesc(t *testing.T) {
	seedTestDB(t)
	for _, r := range []string{"2026-09-28", "2026-10-05", "2026-10-12"} {
		five := 5
		if err := StoreStats(statusResp(r, 1, &five)); err != nil {
			t.Fatalf("StoreStats %s: %v", r, err)
		}
	}
	resets := AllResets()
	if len(resets) != 3 {
		t.Fatalf("resets = %v, want 3", resets)
	}
	if resets[0] != "2026-10-12" || resets[2] != "2026-09-28" {
		t.Errorf("resets = %v, want newest first", resets)
	}
}
