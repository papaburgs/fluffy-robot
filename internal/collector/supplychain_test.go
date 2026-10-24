package collector

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	ds "github.com/papaburgs/fluffy-robot/internal/datastore"
	"github.com/papaburgs/fluffy-robot/internal/gate"
)

// seedCollectorDB points the datastore package globals at a temp store.
// The datastore globals are process-wide, so these tests must NOT use
// t.Parallel().
func seedCollectorDB(t *testing.T) {
	t.Helper()
	t.Setenv("FLUFFY_STORAGE_PATH", t.TempDir())
	ds.Init()
}

func TestUpdateSupplyChain_FetchesOnce(t *testing.T) {
	seedCollectorDB(t)
	want := map[string][]string{
		"FAB_MATS":        {"IRON", "QUARTZ_SAND"},
		"LIQUID_HYDROGEN": {"MACHINERY"},
	}
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/market/supply-chain" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		hits++
		fmt.Fprint(w, `{"data":{"exportToImportMap":{"FAB_MATS":["IRON","QUARTZ_SAND"],"LIQUID_HYDROGEN":["MACHINERY"]}}}`)
	}))
	t.Cleanup(srv.Close)

	c := NewCollector(gate.New(2, 20), srv.URL)
	if err := c.updateSupplyChain(context.Background()); err != nil {
		t.Fatalf("first updateSupplyChain: %v", err)
	}
	got, err := ds.GetSupplyChain()
	if err != nil {
		t.Fatalf("GetSupplyChain: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetSupplyChain = %v, want %v", got, want)
	}

	// The never-refetch rule: a second run must not hit the API again.
	if err := c.updateSupplyChain(context.Background()); err != nil {
		t.Fatalf("second updateSupplyChain: %v", err)
	}
	if hits != 1 {
		t.Fatalf("server hits after two runs = %d, want 1", hits)
	}
}

func TestUpdateSupplyChain_EmptyPayloadSkipped(t *testing.T) {
	seedCollectorDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{}`)
	}))
	t.Cleanup(srv.Close)

	c := NewCollector(gate.New(2, 20), srv.URL)
	if err := c.updateSupplyChain(context.Background()); err != nil {
		t.Fatalf("updateSupplyChain: %v", err)
	}
	if ds.SupplyChainExists() {
		t.Fatal("empty payload marked the chain collected; next tick would not retry")
	}
}
