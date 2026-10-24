package datastore

import (
	"reflect"
	"testing"
)

func testChain() map[string][]string {
	return map[string][]string{
		"EXPORT_A": {"IN1", "IN2"},
		"EXPORT_B": {"IN1"},
	}
}

func TestSupplyChainSchema_Table(t *testing.T) {
	seedTestDB(t)
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='supply_chain'`).Scan(&name)
	if err != nil {
		t.Fatalf("supply_chain table missing: %v", err)
	}
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != 2 {
		t.Fatalf("user_version = %d, want 2", version)
	}
}

func TestSupplyChainExists_Empty(t *testing.T) {
	seedTestDB(t)
	if SupplyChainExists() {
		t.Fatal("SupplyChainExists on empty table = true, want false")
	}
}

func TestStoreSupplyChain_RoundTrip(t *testing.T) {
	seedTestDB(t)
	if err := StoreSupplyChain(testChain(), 1700000000); err != nil {
		t.Fatalf("StoreSupplyChain: %v", err)
	}
	if !SupplyChainExists() {
		t.Fatal("SupplyChainExists after store = false, want true")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM supply_chain`).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 3 {
		t.Fatalf("row count = %d, want 3", count)
	}
	var fetchedAt int64
	if err := db.QueryRow(`SELECT fetched_at FROM supply_chain WHERE export='EXPORT_A' AND import='IN1'`).Scan(&fetchedAt); err != nil {
		t.Fatalf("read fetched_at: %v", err)
	}
	if fetchedAt != 1700000000 {
		t.Fatalf("fetched_at = %d, want 1700000000", fetchedAt)
	}
}

func TestStoreSupplyChain_Idempotent(t *testing.T) {
	seedTestDB(t)
	if err := StoreSupplyChain(testChain(), 1700000000); err != nil {
		t.Fatalf("first store: %v", err)
	}
	if err := StoreSupplyChain(testChain(), 1700000000); err != nil {
		t.Fatalf("second store: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM supply_chain`).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 3 {
		t.Fatalf("row count after re-store = %d, want 3", count)
	}
}

func TestGetSupplyChain_RoundTrip(t *testing.T) {
	seedTestDB(t)
	want := map[string][]string{
		"A": {"B", "C"},
		"X": {"Y"},
	}
	if err := StoreSupplyChain(want, 1700000000); err != nil {
		t.Fatalf("StoreSupplyChain: %v", err)
	}
	got, err := GetSupplyChain()
	if err != nil {
		t.Fatalf("GetSupplyChain: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetSupplyChain = %v, want %v", got, want)
	}
}

func TestGetSupplyChain_Empty(t *testing.T) {
	seedTestDB(t)
	got, err := GetSupplyChain()
	if err != nil {
		t.Fatalf("GetSupplyChain: %v", err)
	}
	if got == nil {
		t.Fatal("GetSupplyChain on empty table returned nil map, want non-nil empty")
	}
	if len(got) != 0 {
		t.Fatalf("GetSupplyChain on empty table = %v, want empty map", got)
	}
}
