package datastore

import "testing"

func TestFactions_RoundTrip(t *testing.T) {
	seedTestDB(t)
	mkFaction := func(sym string) Faction {
		var f Faction
		f.Symbol = sym
		f.Name = sym + " Alliance"
		f.Description = "desc for " + sym
		f.Headquarters = "X1-YZ-Q1"
		f.Traits = append(f.Traits,
			struct {
				Symbol      string `json:"symbol"`
				Name        string `json:"name"`
				Description string `json:"description"`
			}{"T1", "Trait One", "first"},
			struct {
				Symbol      string `json:"symbol"`
				Name        string `json:"name"`
				Description string `json:"description"`
			}{"T2", "Trait Two", "second"})
		f.IsRecruiting = true
		return f
	}
	if err := StoreFactions([]Faction{mkFaction("COSMIC"), mkFaction("GALACTIC")}); err != nil {
		t.Fatalf("StoreFactions A: %v", err)
	}
	UpdateReset("2026-10-12")
	if err := StoreFactions([]Faction{mkFaction("COSMIC")}); err != nil {
		t.Fatalf("StoreFactions B: %v", err)
	}

	got, err := GetFactions("2026-10-05")
	if err != nil {
		t.Fatalf("GetFactions A: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("factions A = %d, want 2", len(got))
	}
	bySym := map[string]Faction{}
	for _, f := range got {
		bySym[f.Symbol] = f
	}
	if bySym["COSMIC"].Name != "COSMIC Alliance" || !bySym["COSMIC"].IsRecruiting {
		t.Errorf("COSMIC = %+v", bySym["COSMIC"])
	}
	if len(bySym["COSMIC"].Traits) != 2 || bySym["COSMIC"].Traits[1].Symbol != "T2" {
		t.Errorf("COSMIC traits round-trip failed: %+v", bySym["COSMIC"].Traits)
	}
	if bySym["COSMIC"].Reset != "2026-10-05" {
		t.Errorf("Reset stamp = %q, want 2026-10-05", bySym["COSMIC"].Reset)
	}

	other, err := GetFactions("2026-09-28")
	if err != nil {
		t.Fatalf("GetFactions empty: %v", err)
	}
	if len(other) != 0 {
		t.Errorf("unstored reset returned %d factions, want 0", len(other))
	}
}
