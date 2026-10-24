# Supply Chain Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Store the SpaceTraders global supply chain (export → imports map) in SQLite, fetched exactly once.

**Architecture:** One new `supply_chain` table (schema v2); a collector function `updateSupplyChain` that no-ops when the table is populated and otherwise does a single `GET /market/supply-chain` + one transaction. Wired once at collector start plus a daily ticker. No new dependencies, no new goroutines.

**Tech Stack:** Go stdlib + `modernc.org/sqlite` (already vendored), `httptest` for the collector test.

**Spec:** `docs/superpowers/specs/2026-10-24-supply-chain-design.md` (issue #20)

## Global Constraints

- CGO-free build; **no new external dependencies** (tests use stdlib `net/http/httptest`).
- Every DB write is one transaction via `inTx` (`internal/datastore/store.go`) — bounded retry, drop-loudly.
- Data is static and NOT reset-scoped: no `reset_date` column; `Prune` untouched.
- Never refetch: `SupplyChainExists()` gates the API call.
- Before merge: `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./... -count=1`, `go test -race ./...` all clean.

## Review Focus

1. **Weird payload** (missing `exportToImportMap` key or empty map): must not be treated as "collected" — store nothing, so the next tick retries. Pinned by collector fixture test (Task 3).
2. **Reset pause / VACUUM contention**: single-row write inside `inTx` retry — a drop is loud (metric) and self-heals next tick. No dedicated test; behavior inherits the tested `inTx` path.
3. **Test isolation**: datastore package globals change per test — collector tests must set `FLUFFY_STORAGE_PATH` + `datastore.Init()` per test and must NOT use `t.Parallel()`.

---

### Task 1: Schema v2 + store/exists datastore functions

**Files:**
- Modify: `internal/datastore/schema.go` (append DDL; `schemaVersion` 1 → 2; `PRAGMA user_version = 2`)
- Create: `internal/datastore/supplychain.go`
- Test: `internal/datastore/supplychain_test.go` (new)

**Interfaces:**
- Consumes: `openDB`/`applySchema`, `inTx`, `DatastoreReads`/`DatastoreWrites` metrics conventions.
- Produces:
  - table `supply_chain (export TEXT NOT NULL, import TEXT NOT NULL, fetched_at INTEGER NOT NULL, PRIMARY KEY (export, import)) WITHOUT ROWID`
  - `func StoreSupplyChain(chain map[string][]string, now int64) error` — one `inTx`, `INSERT OR REPLACE` per (export, import)
  - `func SupplyChainExists() bool` — `SELECT EXISTS(SELECT 1 FROM supply_chain LIMIT 1)`; false on error (safe retry)

- [ ] **Step 1: Write the failing tests**

In `supplychain_test.go`:
- `TestSupplyChainSchema_Table`: `seedTestDB(t)`; `sqlite_master` (`type='table'`) contains `supply_chain`; `PRAGMA user_version` = 2.
- `TestSupplyChainExists_Empty`: `seedTestDB(t)`; `SupplyChainExists()` is false.
- `TestStoreSupplyChain_RoundTrip`: store `{"EXPORT_A": ["IN1","IN2"], "EXPORT_B": ["IN1"]}` with `now=1700000000` → `SupplyChainExists()` true; raw SQL `SELECT COUNT(*)` = 3 rows; `SELECT fetched_at FROM supply_chain WHERE export='EXPORT_A' AND import='IN1'` = 1700000000.
- `TestStoreSupplyChain_Idempotent`: store the same map twice → still 3 rows, no error.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/datastore/ -run 'TestSupplyChain|TestStoreSupplyChain' -v`
Expected: FAIL — `undefined: StoreSupplyChain` etc.

- [ ] **Step 3: Implement**

Append the table DDL (verbatim above) to `schemaDDL`; bump version + pragma to 2. Write `StoreSupplyChain`/`SupplyChainExists` in `supplychain.go` following `agent.go`/`store.go` patterns (`DatastoreReads.Add(1)` on the exists check).

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/datastore/ -run 'TestSupplyChain|TestStoreSupplyChain|TestOpenDB' -v`
Expected: PASS (incl. existing `TestOpenDB_PrivilegedState`).

- [ ] **Step 5: Commit**

```
git add internal/datastore/schema.go internal/datastore/supplychain.go internal/datastore/supplychain_test.go
git commit -m "feat(datastore): supply_chain table + StoreSupplyChain/SupplyChainExists"
```

### Task 2: GetSupplyChain getter

**Files:**
- Modify: `internal/datastore/supplychain.go`
- Test: `internal/datastore/supplychain_test.go`

**Interfaces:**
- Consumes: Task 1 table.
- Produces: `func GetSupplyChain() (map[string][]string, error)` — full map export → imports list; imports within an export sorted (`ORDER BY export, import`); empty table ⇒ empty non-nil map, nil error.

- [ ] **Step 1: Write the failing tests**

- `TestGetSupplyChain_RoundTrip`: store `{"A": ["B","C"], "X": ["Y"]}` → map equals exactly; `A` list is `["B","C"]` (sorted).
- `TestGetSupplyChain_Empty`: `seedTestDB(t)` → empty map, nil error.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/datastore/ -run TestGetSupplyChain -v`; expected FAIL (undefined).

- [ ] **Step 3: Implement** — `SELECT export, import FROM supply_chain ORDER BY export, import`, fold rows into the map; `make(map[string][]string)` up front so empty stays non-nil.

- [ ] **Step 4: Run to verify pass** — same `-run`; PASS. Then `go test ./internal/datastore/ -count=1` all green.

- [ ] **Step 5: Commit**

```
git add internal/datastore/supplychain.go internal/datastore/supplychain_test.go
git commit -m "feat(datastore): GetSupplyChain getter"
```

### Task 3: Collector fetch + wiring + metrics + docs

**Files:**
- Create: `internal/collector/supplychain.go`
- Test: `internal/collector/supplychain_test.go` (new)
- Modify: `internal/collector/collector.go` (struct + `NewCollector` + `Run`)
- Modify: `internal/metrics/metrics.go`
- Modify: `internal/collector/tick_test.go`
- Modify: `AGENTS.md`, `docs/INTERNAL.md`

**Interfaces:**
- Consumes: `doGET` (`api.go:14`), `tickDuration` (`collector.go:39`), Task 1–2 datastore funcs.
- Produces:
  - `func (c *Collector) updateSupplyChain(ctx context.Context) error` — `SupplyChainExists()` ⇒ nil; else GET `%s/market/supply-chain`, parse `data.exportToImportMap` (`map[string][]string`); empty map ⇒ log + nil (no store); else `StoreSupplyChain(m, now)`
  - Collector fields `supplyTicker *time.Ticker`, `supplyInterval time.Duration`
  - env `FLUFFY_SUPPLY_TICK` (default `24h`)
  - metric `collector_supply_chain_updates_total`

- [ ] **Step 1: Write the failing tests**

- In `tick_test.go`: `FLUFFY_SUPPLY_TICK` parse cases mirroring the existing pattern — unset ⇒ `24*time.Hour`; `30s` honored; invalid ⇒ `24*time.Hour` fallback.
- In `supplychain_test.go`, helper-first: temp datastore via `os.Setenv("FLUFFY_STORAGE_PATH", t.TempDir())` + `datastore.Init()` (restore env in `t.Cleanup`; **no `t.Parallel()`**), plus `httptest.NewServer` with a request-hit counter on `/market/supply-chain` and a collector with `gate.New(2, 20)`, `baseURL = srv.URL`.
  - `TestUpdateSupplyChain_FetchesOnce`: payload with 2 exports → first run stores it (`ds.GetSupplyChain()` matches); second run ⇒ server hit count still 1.
  - `TestUpdateSupplyChain_EmptyPayloadSkipped`: server returns `{}` → run ⇒ no error, `SupplyChainExists()` false (next tick would retry).

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/collector/ -run 'TestTickDuration|TestUpdateSupplyChain' -v`
Expected: FAIL (undefined).

- [ ] **Step 3: Implement**

- `metrics.go`: `CollectorSupplyChainUpdates = expvar.NewInt("collector_supply_chain_updates_total")`.
- `collector.go`: `NewCollector` adds `supplyInterval = tickDuration("FLUFFY_SUPPLY_TICK", 24*time.Hour)`; in `Run()`, after the first `updateStatus`, call `c.updateSupplyChain(ctx)` once (non-fatal on error); create `c.supplyTicker`; add a `select` case calling it on tick; stop/recreate the ticker in the reset-pause branch with the other three.
- `supplychain.go`: `updateSupplyChain` per Interfaces; on successful store `metrics.CollectorSupplyChainUpdates.Add(1)`; log via `logging.Info` like other collectors (`apiCalls`, duration).
- Docs: `AGENTS.md` — env-table row (`FLUFFY_SUPPLY_TICK`, `24h`, "Supply chain refresh check (no-op once collected)") + `supply_chain` in the Data Format table list + collector intervals mention; `docs/INTERNAL.md` — schema/collector notes.

- [ ] **Step 4: Verify**

Run: `go build ./...`; `go vet ./...`; `gofmt -l .` (empty); `go test ./... -count=1`; `go test -race ./...`
Expected: all clean, new tests green.

- [ ] **Step 5: Commit**

```
git add internal/collector/ internal/metrics/metrics.go AGENTS.md docs/INTERNAL.md
git commit -m "feat(collector): updateSupplyChain — fetch-once supply chain + FLUFFY_SUPPLY_TICK + docs"
```

---

## Self-Review Notes

- Spec coverage: table + version bump (T1), exists-gated single fetch (T3), once-at-start + daily ticker (T3), getter (T2), defensive empty-payload rule (T3 fixture), tests per spec (T1–T3).
- Name consistency: `StoreSupplyChain`, `SupplyChainExists`, `GetSupplyChain`, `updateSupplyChain`, `FLUFFY_SUPPLY_TICK`, `collector_supply_chain_updates_total` — identical in all tasks.
- Proportion: 3 tasks for 2 new files + 4 small edits; each task independently green and commit-able.
