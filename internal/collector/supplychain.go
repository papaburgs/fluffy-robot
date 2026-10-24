package collector

import (
	"context"
	"encoding/json"
	"time"

	ds "github.com/papaburgs/fluffy-robot/internal/datastore"
	"github.com/papaburgs/fluffy-robot/internal/logging"
	"github.com/papaburgs/fluffy-robot/internal/metrics"
)

// responseSupplyChain matches GET /market/supply-chain: a static
// export→imports map (~126 exports), fetched without an agent token.
type responseSupplyChain struct {
	Data struct {
		ExportToImportMap map[string][]string `json:"exportToImportMap"`
	} `json:"data"`
}

// updateSupplyChain fetches the global supply chain exactly once. The data
// is static game-design data: if the store already has it, this is a no-op
// (the core rule — never call the endpoint again once collected). A
// missing/empty map is treated as "not collected" and stored as nothing,
// so the next tick retries from scratch. Errors are logged by the caller
// and are never fatal.
func (c *Collector) updateSupplyChain(ctx context.Context) error {
	if ds.SupplyChainExists() {
		return nil
	}
	c.ingestStart = time.Now()

	resp, err := c.doGET(ctx, c.baseURL+"/market/supply-chain")
	if err != nil {
		return err
	}

	var chain responseSupplyChain
	if err := json.Unmarshal(resp.Bytes, &chain); err != nil {
		return err
	}
	if len(chain.Data.ExportToImportMap) == 0 {
		logging.Warn("supply chain payload empty, will retry next tick")
		return nil
	}

	if err := ds.StoreSupplyChain(chain.Data.ExportToImportMap, time.Now().Unix()); err != nil {
		return err
	}
	metrics.CollectorSupplyChainUpdates.Add(1)
	logging.Info("supply chain ingestion completed", "exports", len(chain.Data.ExportToImportMap), "apiCalls", c.apiCalls, "duration", time.Now().Sub(c.ingestStart))
	return nil
}
