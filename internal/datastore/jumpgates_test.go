package datastore

import "testing"

func TestJumpgateList_LatestWins(t *testing.T) {
	seedTestDB(t)
	if err := UpdateJumpGates([]JGInfo{
		{Jumpgate: "X1-YZ-J1", System: "X1-YZ", Headquarters: "X1-YZ-Q1", Status: Active},
		{Jumpgate: "X1-AA-J1", System: "X1-AA", Headquarters: "X1-AA-Q1", Status: NoActivity},
	}); err != nil {
		t.Fatalf("UpdateJumpGates 1: %v", err)
	}
	// second call replaces the whole reset list — the dropped gate is gone
	if err := UpdateJumpGates([]JGInfo{
		{Jumpgate: "X1-BB-J1", System: "X1-BB", Headquarters: "X1-BB-Q1", Status: Const},
	}); err != nil {
		t.Fatalf("UpdateJumpGates 2: %v", err)
	}
	got, err := GetJumpgateList(currentReset)
	if err != nil {
		t.Fatalf("GetJumpgateList: %v", err)
	}
	if len(got) != 1 || got[0].System != "X1-BB" || got[0].Status != Const {
		t.Errorf("latest list = %+v, want only X1-BB Const", got)
	}
}

func TestJumpgates_StatusFilters(t *testing.T) {
	seedTestDB(t)
	if err := UpdateJumpGates([]JGInfo{
		{Jumpgate: "X1-A-J1", System: "X1-A", Headquarters: "X1-A-Q1", Status: Active},
		{Jumpgate: "X1-C-J1", System: "X1-C", Headquarters: "X1-C-Q1", Status: Const},
		{Jumpgate: "X1-D-J1", System: "X1-D", Headquarters: "X1-D-Q1", Status: Complete, Complete: 42},
	}); err != nil {
		t.Fatalf("UpdateJumpGates: %v", err)
	}

	all := GetJumpgates(currentReset)
	if len(all) != 3 {
		t.Fatalf("GetJumpgates = %d, want 3", len(all))
	}
	if all["X1-A"].Jumpgate != "X1-A-J1" {
		t.Errorf("GetJumpgates not keyed by System: %+v", all)
	}
	underConst := GetJumpgatesUnderConst(currentReset)
	if len(underConst) != 1 || underConst["X1-C"].Jumpgate != "X1-C-J1" {
		t.Errorf("UnderConst = %+v, want X1-C", underConst)
	}
	notStarted := GetJumpgatesNotStarted(currentReset)
	if len(notStarted) != 1 || notStarted["X1-A"].Jumpgate != "X1-A-J1" {
		t.Errorf("NotStarted = %+v, want X1-A", notStarted)
	}
	complete := GetJumpgatesComplete(currentReset)
	if len(complete) != 1 || complete[0].System != "X1-D" || complete[0].Complete != 42 {
		t.Errorf("Complete = %+v, want X1-D ts 42", complete)
	}
}

func TestMarkComplete_Started(t *testing.T) {
	seedTestDB(t)
	if err := UpdateJumpGates([]JGInfo{
		{Jumpgate: "X1-A-J1", System: "X1-A", Headquarters: "X1-A-Q1", Status: Active},
		{Jumpgate: "X1-B-J1", System: "X1-B", Headquarters: "X1-B-Q1", Status: Active},
	}); err != nil {
		t.Fatalf("UpdateJumpGates: %v", err)
	}
	if err := MarkJumpgatesStarted([]string{"X1-A"}); err != nil {
		t.Fatalf("MarkStarted: %v", err)
	}
	got, _ := GetJumpgateList(currentReset)
	bySys := map[string]JGInfo{}
	for _, j := range got {
		bySys[j.System] = j
	}
	if bySys["X1-A"].Status != Const {
		t.Errorf("X1-A status = %d, want Const", bySys["X1-A"].Status)
	}
	if bySys["X1-B"].Status != Active {
		t.Errorf("X1-B status = %d, want Active (untouched)", bySys["X1-B"].Status)
	}
	if err := MarkJumpgatesComplete([]string{"X1-B"}, 1234); err != nil {
		t.Fatalf("MarkComplete: %v", err)
	}
	got, _ = GetJumpgateList(currentReset)
	for _, j := range got {
		if j.System == "X1-B" && (j.Status != Complete || j.Complete != 1234) {
			t.Errorf("X1-B = %+v, want Complete at 1234", j)
		}
	}
}

func TestConstructions_Window(t *testing.T) {
	seedTestDB(t)
	base := int64(9_000_000)
	var recs []JGConstruction
	for i := 0; i < 5; i++ {
		recs = append(recs, JGConstruction{Timestamp: base + int64(i*300), Jumpgate: "X1-A-J1", Fabmat: i * 100, Advcct: i * 10})
	}
	if err := AddConstructions(recs, base); err != nil {
		t.Fatalf("AddConstructions: %v", err)
	}
	got, err := GetConstructions(currentReset, base+300, base+900)
	if err != nil {
		t.Fatalf("GetConstructions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("window rows = %d, want 3", len(got))
	}
	for _, r := range got {
		if r.Timestamp < base+300 || r.Timestamp > base+900 {
			t.Errorf("timestamp %d outside window", r.Timestamp)
		}
	}
	// sorted ascending by timestamp
	for i := 1; i < len(got); i++ {
		if got[i].Timestamp < got[i-1].Timestamp {
			t.Errorf("rows not time-ordered: %+v", got)
		}
	}
}

func TestLatestConstructions(t *testing.T) {
	seedTestDB(t)
	if err := AddConstructions([]JGConstruction{
		{Timestamp: 100, Jumpgate: "X1-A-J1", Fabmat: 10, Advcct: 1},
		{Timestamp: 200, Jumpgate: "X1-A-J1", Fabmat: 20, Advcct: 2},
		{Timestamp: 150, Jumpgate: "X1-B-J1", Fabmat: 99, Advcct: 9},
	}, 200); err != nil {
		t.Fatalf("AddConstructions: %v", err)
	}
	got := LatestConstructions()
	if len(got) != 2 {
		t.Fatalf("latest rows = %d, want 2 (one per gate)", len(got))
	}
	for _, r := range got {
		if r.Jumpgate == "X1-A-J1" && (r.Timestamp != 200 || r.Fabmat != 20) {
			t.Errorf("X1-A latest = %+v, want ts 200 fabmat 20", r)
		}
		if r.Jumpgate == "X1-B-J1" && r.Fabmat != 99 {
			t.Errorf("X1-B latest = %+v, want fabmat 99", r)
		}
	}
}
