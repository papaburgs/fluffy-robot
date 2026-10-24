# Supply Chain — Design

**Issue:** #20 (supersedes #19, which over-scoped into a universe-wide market crawl — rejected by owner: "all I want is the supply chain endpoint, do not touch markets, marketgood, tradegoods")
**Date:** 2026-10-24
**Status:** draft for review

## Intent

Store the global supply chain: the map of **each export tradegood to the import goods it is built from** (`FAB_MATS ← IRON, QUARTZ_SAND`). Static game-design data — not reset-dependent, does not change — so it is fetched **once** from the API and never again. A later page will render it; this work is backend only.

## Data source (verified live)

`GET /market/supply-chain` →

```json
{"data": {"exportToImportMap": {"LIQUID_HYDROGEN": ["MACHINERY"], "FAB_MATS": ["IRON","QUARTZ_SAND"], ...}}}
```

Called **anonymously on 2026-10-24: HTTP 200** with ~126 exports — despite the API spec (`local/spacetradersapi.json`) marking it `AgentToken`-required. Safe for this tokenless app. One fixed endpoint, no parameters, no pagination.

## Storage

Schema v2 (bump `user_version` 1 → 2; `CREATE TABLE IF NOT EXISTS` keeps the bump idempotent):

```sql
CREATE TABLE IF NOT EXISTS supply_chain (
    export     TEXT NOT NULL,
    import     TEXT NOT NULL,
    fetched_at INTEGER NOT NULL,
    PRIMARY KEY (export, import)
) WITHOUT ROWID;
```

- One row per (export, import) pair — ~250 short rows today. RAM impact: nil.
- Not reset-scoped; `Prune` works from an explicit table list and will not touch it.
- "Already collected?" = `SELECT EXISTS(SELECT 1 FROM supply_chain LIMIT 1)` (`SupplyChainExists()`).

## Collector

`updateSupplyChain(ctx)` in `internal/collector/supplychain.go`:

1. `ds.SupplyChainExists()` → true ⇒ **return nil** (the core owner rule: never call again once collected).
2. `GET {baseURL}/market/supply-chain`, unmarshal `data.exportToImportMap`.
3. Empty/absent map ⇒ log, return nil without storing (defensive: a weird payload must not mark the chain collected; retried from scratch next tick).
4. `ds.StoreSupplyChain(chain, now)` — one `inTx` transaction (`INSERT OR REPLACE` per row).

Wiring (no new goroutine — a single-call job runs on the main collector loop):

- Once at collector start (`Run()`, right after the first `updateStatus`) so a fresh DB populates within seconds of boot.
- A `FLUFFY_SUPPLY_TICK` ticker (default `24h`, parsed by the existing `tickDuration`), firing into the main `select` alongside the other three tickers; stopped/recreated in the reset-pause branch like the rest. Daily re-runs are no-ops once data exists.

Getter: `GetSupplyChain() (map[string][]string, error)` — full map, ordered keys, for a future frontend page.

## Non-goals

- No per-waypoint market data, no tradegood names/descriptions, no prices, no `EXCHANGE`/waypoint traits (that was the rejected #19 scope).
- No pruning, no history — the map is stored once and treated as immutable.

## Testing

- Datastore: schema presence + `user_version=2`; store→get round trip; `SupplyChainExists` false→true; idempotent re-store.
- Collector (httptest): `updateSupplyChain` fetches and stores; a second run produces **zero** additional HTTP requests (the never-refetch rule); an empty-map payload stores nothing.
