# SQLite Datastore Migration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the gob.zst file datastore with an embedded SQLite store (modernc.org/sqlite) so per-request RAM is flat, cycles are consistent, and retention is automatic.

**Architecture:** One `*sql.DB` over `{FLUFFY_STORAGE_PATH}/fluffy.db`, WAL + `synchronous=FULL`, one transaction per collection cycle. The `internal/datastore` package keeps its name and collector-facing surface; frontend getters gain agent-set filters and aggregate forms. Old gob code, the decoder tool, and the vestigial deps are deleted at the end.

**Tech Stack:** Go 1.24.5, `modernc.org/sqlite` (pure Go — `CGO_ENABLED=0` build unchanged), `database/sql`, expvar metrics, `html/template` + go-echarts (untouched except call sites).

**Spec:** `docs/superpowers/specs/2026-10-17-sqlite-datastore-design.md` — the plan argues from the spec; read both. All schema DDL, pragma values, and policy text below are copied from the spec.

## Global Constraints

- `CGO_ENABLED=0` build; the only new dependency is `modernc.org/sqlite`; `libsql-client-go` and `klauspost/compress` are removed when the gob path dies (Task 11).
- DB file: `{FLUFFY_STORAGE_PATH}/fluffy.db` (default `./`), one process-wide `*sql.DB`.
- Pragmas (set at open): `journal_mode=WAL`, `synchronous=FULL`, `busy_timeout=10000`, `foreign_keys=ON`, `cache_size=-4000`; mmap stays off. `PRAGMA user_version = 1`.
- One transaction per collection cycle. Natural keys are minute-rounded unix timestamps; writes use `INSERT OR REPLACE` so retries and re-collection are idempotent.
- No request path may allocate proportional to total stored data. `/chart` queries take the chart's agent list; empty agent list ⇒ empty result (never an all-agents fallback).
- Bounded retry on failed cycle writes: exponential backoff 1s→64s, at most 7 attempts (~2 min); after that, the cycle is dropped with an escalating log and `datastore_write_errors_total` increments. Rationale (spec): loud drops beat unbounded retry, which would block collection.
- Retention: 5 full game weeks + the current partial week. Prune and VACUUM fire only at the Sunday 13:00 UTC reset transition, behind a free-space guard: skip when free < 1.25×DB size + 32 MB. `FLUFFY_RETENTION_WEEKS` (default 5). Never prune mid-week; never touch the current reset.
- Fresh start: the code never imports old gob data and never deletes old `.gob.zst` directories.
- Collector schedules and reset-detection logic do not change.
- Repo rules: work in a worktree, commit often, push only to `origin` (forgejo), never `main` directly, never the `deploy` remote.

## Review Focus

1. **Empty agent list on `/chart`** (no agents selected) must return an empty series set, not fall back to all agents — a wrong guess silently recreates the 120 MB spike. Pinned by `TestGetAgentHistory_EmptyAgentFilter` (Task 2).
2. **Fresh boot before the first status cycle** (the deploy moment): the old `resets[0]` index panics. All handlers must use `ds.LatestReset()`; the `resets` global and its racing goroutine are deleted. Pinned by grep-zero verification + build (Task 9).
3. **Transient write failure mid-cycle** (locked/busy disk): retries coalesce via `INSERT OR REPLACE`, no partial cycle is ever visible. Pinned by `TestInTx_RetryTransient` and `TestInTx_DropAfterBound` (Task 6).
4. **Prune boundary**: rows exactly at the cutoff timestamp belong to the kept week. Pinned by `TestPrune_BoundaryRowKept` (Task 7).
5. **Cross-reset reads** (a previous reset's stats/leaderboard/agent list) must return that reset's data — broken in the old store (#4). Pinned by `TestGetStats_OlderReset` and `TestGetAgentList_OlderReset` (Tasks 3, 2).

---

### Task 1: Store foundation — driver, open/pragmas, schema, txn helper

**Files:**
- Modify: `go.mod` / `go.sum` (add `modernc.org/sqlite`; drop `tursodatabase/libsql-client-go`)
- Create: `internal/datastore/store.go`
- Create: `internal/datastore/schema.go`
- Modify: `internal/datastore/datastore.go` (rewrite `Init()`; keep `writeData`/`readData` and the `writeJSON` var compiling — old store lives until Task 11)
- Test: `internal/datastore/store_test.go`

**Interfaces:**
- Produces: `var db *sql.DB` (package-private); `openDB(path string) (*sql.DB, error)`; `inTx(fn func(tx *sql.Tx) error) error`; `freeSpace(path string) (int64, error)`; `dbSize() (int64, error)`; `applySchema(d *sql.DB) error`; `const schemaVersion = 1`. `Init()` keeps its no-void-signature + `os.Exit(1)-on-failure semantics and opens the DB.

- [ ] **Step 1: Write the failing test**

```go
func TestOpenDB_PrivilegedState(t *testing.T) {
    d, err := openDB(filepath.Join(t.TempDir(), "fluffy.db"))
    // assert: err nil
    // query journal_mode -> "wal"; synchronous -> "2" (FULL);
    // foreign_keys -> "1"; cache_size -> "-4000"; user_version -> "1"
    // query sqlite_master: tables agents, agent_snapshots, agent_status,
    // stats, leaderboards, jumpgates, construction, factions all exist
}
func TestInTx_CommitsAndRollsBack(t *testing.T) { /* insert in tx -> visible; fn returns error -> rolled back */ }
func TestFreeSpace_Positive(t *testing.T) { /* freeSpace(t.TempDir()) > 0 */ }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/datastore/ -run 'TestOpenDB|TestInTx|TestFreeSpace' -v`
Expected: FAIL (undefined: `openDB`, `inTx`, `freeSpace`)

- [ ] **Step 3: Implement `store.go`, `schema.go`; rewrite `Init()`**

`openDB` opens via `modernc.org/sqlite` driver, applies the pragmas (Global Constraints values), then `applySchema`. `applySchema` executes the spec's §Schema v1 DDL **verbatim** (all 8 tables, `WITHOUT ROWID` where the spec has it) and sets `user_version`, idempotently (create-if-not-exists; check user_version and bump only forward). `inTx` begins a tx, runs `fn`, commits or rolls back. `freeSpace` uses `syscall.Statfs` (linux). `Init()` reads `FLUFFY_STORAGE_PATH` (default `./`), `MkdirAll`, `openDB(filepath.Join(path, "fluffy.db"))`, assigns package `db`; keeps parsing `FLUFFY_WRITE_JSON` into the existing `writeJSON` var (removed in Task 11). Then: `go get modernc.org/sqlite && go mod tidy` (drops libsql-client-go).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/datastore/ -run 'TestOpenDB|TestInTx|TestFreeSpace' -v`
Expected: PASS. Also `go build ./...` green (old gob store still compiles).

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/datastore/
git commit -m "feat(datastore): sqlite foundation — open, pragmas, schema v1, txn helper"
```

### Task 2: Agents — store transaction, list, filtered history, latest ships

**Files:**
- Modify (replace): `internal/datastore/agent.go`
- Modify: `internal/frontend/handlers.go:67` (`GetAgentHistory(thisReset, startTime, 0, nil)` — `nil` keeps today's behavior until Task 9), `handlers.go:456` (same)
- Test: `internal/datastore/agent_test.go`

**Interfaces:**
- Consumes: `inTx`, `db`, `UpdateReset` (existing, still old until Task 3 — tests set `currentReset` via `UpdateReset` which for now keeps its directory side effect; harmless on the new path).
- Produces: `StoreAgents(apiAgents []PublicAgent, now int64) error`; `GetAgentList(reset Reset) ([]Agent, error)`; `GetAgentHistory(reset Reset, start, end int64, agents []string) ([]AgentStatus, error)`; `LatestShips() map[string]int64`.

- [ ] **Step 1: Write the failing tests**

```go
func TestStoreAgents_RoundTrip(t *testing.T)  // two agents stored; GetAgentList(currentReset) returns them with System from SystemFromWaypoint
func TestStoreAgents_SnapshotRefreshSeriesAppends(t *testing.T)
    // StoreAgents(ts=100) then StoreAgents(ts=200) with changed credits:
    // GetAgentList -> latest credits; GetAgentHistory(reset, 0, 0, nil) -> rows at both 100 and 200
func TestGetAgentHistory_WindowAndAgentFilter(t *testing.T)
    // seed 3 agents x 10 rows; query agents=[a1], start/end covering 5 rows
    // assert: exactly 5 rows returned, all Symbol==a1  (proves filter happens in SQL)
func TestGetAgentHistory_EmptyAgentFilter(t *testing.T)  // agents=[] -> zero rows, nil error
func TestGetAgentHistory_OlderReset(t *testing.T)  // two resets stored; list for older reset returns older snapshot
func TestLatestShips(t *testing.T)  // per agent, ships of max(timestamp) row
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/datastore/ -run 'TestStoreAgents|TestGetAgentHistory|TestLatestShips' -v`
Expected: FAIL (old implementations: signature mismatch / wrong semantics)

- [ ] **Step 3: Replace `agent.go`**

`StoreAgents` = one `inTx` transaction: (1) `INSERT OR IGNORE` each symbol into `agents` registry, then `SELECT id, symbol` into a map; (2) `DELETE FROM agent_snapshots WHERE reset_date = ?` + re-insert rows (latest-wins, spec §schema note); (3) `INSERT OR REPLACE INTO agent_status` per row. `GetAgentList` joins `agent_snapshots`/`agents` for the reset; empty `reset` arg means latest (`WHERE reset_date = (SELECT MAX(reset_date) …)`), same convention for all reset-scoped getters. `GetAgentHistory` builds `WHERE a.symbol IN (…)` only when `agents` is non-empty, plus `AND s.timestamp BETWEEN ? AND ?` (end 0 ⇒ now, as today). `LatestShips` = `SELECT a.symbol, s.ships, MAX(s.timestamp) … GROUP BY a.symbol`. Delete the old file's `consolidating` busy-wait lines (the `consolidating` var lives in old jumpgates.go until Task 4 — keep the var if the package needs it to compile; simplest: move `var consolidating bool` deletion into Task 4).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/datastore/ -v && go build ./...`
Expected: PASS, build green (frontend call sites updated to pass `nil`).

- [ ] **Step 5: Commit**

```bash
git add internal/datastore/agent.go internal/datastore/agent_test.go internal/frontend/handlers.go
git commit -m "feat(datastore): agent store in sqlite — txn per cycle, agent-filtered history"
```

### Task 3: Status — stats, leaderboards, reset helpers (fixes #4, #5)

**Files:**
- Modify (replace): `internal/datastore/status.go`
- Test: `internal/datastore/status_test.go`

**Interfaces:**
- Produces: `UpdateReset(r Reset)` (in-memory `currentReset` only — no disk op); `LatestReset() Reset`; `AllResets() []string` (`SELECT DISTINCT reset_date FROM stats ORDER BY reset_date DESC`); `StoreStats(r ResponseStatus) error` (nil `r.Stats.Accounts` ⇒ 0); `GetStats(reset Reset) (Stats, error)`; `StoreLeaderboards(r ResponseStatus) error`; `GetLeaderboard(reset Reset) ([]LeaderboardEntry, []LeaderboardEntry, error)`.

- [ ] **Step 1: Write the failing tests**

```go
func TestStoreStats_NilAccountsIsZero(t *testing.T)  // #5 fix: ResponseStatus{Stats:{Accounts:nil}} -> Stats.Accounts==0, no panic
func TestStats_RoundTripLatest(t *testing.T)         // StoreStats for two resets; GetStats("") -> newest
func TestGetStats_OlderReset(t *testing.T)           // GetStats(olderReset) -> older reset's row (cross-reset fix)
func TestLeaderboards_RoundTrip(t *testing.T)        // credits + charts lists per reset; GetLeaderboard("") -> newest
func TestAllResets_OrderedDesc(t *testing.T)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/datastore/ -run 'TestStoreStats|TestStats|TestGetStats|TestLeaderboards|TestAllResets' -v`
Expected: FAIL

- [ ] **Step 3: Replace `status.go`**

`StoreStats`/`StoreLeaderboards` use `inTx` + `INSERT OR REPLACE` on `reset_date` (leaderboards: delete reset's rows, insert ranked rows — `kind` = 'credits'/'charts'). `NextReset()` keeps existing shape for now (reads `GetStats(currentReset).NextReset`) — it is deleted in Task 11 if unreferenced after Task 9. Busy-wait semantics of `LatestReset` unchanged.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/datastore/ -v && go build ./...`
Expected: PASS, build green.

- [ ] **Step 5: Commit**

```bash
git add internal/datastore/status.go internal/datastore/status_test.go
git commit -m "feat(datastore): stats/leaderboard/reset helpers in sqlite; nil-accounts fix"
```

### Task 4: Jumpgates and constructions

**Files:**
- Modify (replace): `internal/datastore/jumpgates.go`
- Modify: `internal/collector/jumpgates.go` (call sites gain `if err != nil { logging.Error(...) }` for `UpdateJumpGates`, `MarkJumpgatesComplete`, `MarkJumpgatesStarted`, `AddConstructions`)
- Test: `internal/datastore/jumpgates_test.go`

**Interfaces:**
- Produces: `UpdateJumpGates(jgList []JGInfo) error` (tx: delete reset's rows, insert — latest wins); `GetJumpgateList(reset Reset) ([]JGInfo, error)`; `GetJumpgates(reset Reset) map[string]JGInfo` keyed by System; `GetJumpgatesUnderConst`, `GetJumpgatesNotStarted`, `GetJumpgatesComplete` (same filters as today, now respecting the `reset` arg via latest-reset convention); `MarkJumpgatesComplete(jgs []string, ts int64) error` (`UPDATE … SET status=Complete, completed_at=? WHERE reset_date=? AND system IN (…)`); `MarkJumpgatesStarted(jgs []string) error` (status=Const); `AddConstructions(cList []JGConstruction, ts int64) error` (INSERT OR REPLACE); `GetConstructions(reset Reset, start, end int64) ([]JGConstruction, error)`; `LatestConstructions() []JGConstruction` (`GROUP BY jumpgate` with `MAX(timestamp)`).

- [ ] **Step 1: Write the failing tests**

```go
func TestJumpgateList_LatestWins(t *testing.T)      // two UpdateJumpGates calls; GetJumpgateList -> only latest rows
func TestJumpgates_StatusFilters(t *testing.T)      // seed Const/Active/Complete gates; the three filter getters return the right maps/slices, keyed by System
func TestMarkComplete_Started(t *testing.T)          // MarkJumpgatesComplete sets status+completed_at; MarkJumpgatesStarted sets Const
func TestConstructions_Window(t *testing.T)         // rows outside [start,end] excluded; end=0 -> now
func TestLatestConstructions(t *testing.T)           // one row per gate, the max-timestamp one
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/datastore/ -run 'TestJumpgate|TestMark|TestConstructions|TestLatestConstructions' -v`
Expected: FAIL

- [ ] **Step 3: Replace `jumpgates.go`** — delete the `consolidating` var and its busy-waits here; also delete `ConstructionRecord`? No — keep `ConstructionRecord` (charts use it). Keep `GetConstructions` window convention (end 0 ⇒ now) identical to today.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/datastore/ -v && go build ./...`
Expected: PASS, build green.

- [ ] **Step 5: Commit**

```bash
git add internal/datastore/jumpgates.go internal/datastore/jumpgates_test.go internal/collector/jumpgates.go
git commit -m "feat(datastore): jumpgates + construction series in sqlite"
```

### Task 5: Factions store

**Files:**
- Modify (replace): `internal/datastore/factions.go`
- Test: `internal/datastore/factions_test.go`

**Interfaces:**
- Produces: `StoreFactions(fac []Faction) error` (tx: delete current reset's rows, insert with `traits` marshaled to JSON); `GetFactions(reset Reset) ([]Faction, error)` (traits unmarshaled back).

- [ ] **Step 1: Write the failing test**

```go
func TestFactions_RoundTrip(t *testing.T)  // store for reset A (2 factions w/ traits); GetFactions(A) -> same; GetFactions(otherReset) -> empty, no error
```

- [ ] **Step 2: Run to verify it fails** — Run: `go test ./internal/datastore/ -run TestFactions -v` — Expected: FAIL

- [ ] **Step 3: Replace `factions.go`** — keep the existing behavior of stamping `k.Reset = currentReset` before insert.

- [ ] **Step 4: Run to verify it passes** — Run: `go test ./internal/datastore/ -v` — Expected: PASS

- [ ] **Step 5: Commit** — `git commit -m "feat(datastore): factions store in sqlite (per-reset upsert)"`

### Task 6: Bounded retry on cycle writes

**Files:**
- Modify: `internal/datastore/store.go` (`inTx` gains retry)
- Modify: `internal/metrics/metrics.go` (add `DatastoreWriteRetries = expvar.NewInt("datastore_write_retries_total")`, `DatastoreWriteErrors = expvar.NewInt("datastore_write_errors_total")`)
- Test: `internal/datastore/retry_test.go`

**Interfaces:**
- Produces: same `inTx(fn func(tx *sql.Tx) error) error` — internally: up to 7 attempts, backoff 1s→64s (1,2,4,8,16,32,64), each attempt a fresh transaction; `datastore_write_retries_total` increments per failed attempt; after the 7th failure returns the last error (caller logs it, `datastore_write_errors_total` increments inside `inTx`).

- [ ] **Step 1: Write the failing tests**

```go
func TestInTx_RetryTransient(t *testing.T)
    // fn fails on attempts 1-2 (package test hook: a fail counter), succeeds on 3
    // assert: data committed, retry counter +2, no error returned
func TestInTx_DropAfterBound(t *testing.T)
    // fn always fails; assert: error returned, 6 retries counted, error counter +1, nothing committed
```

Backoff in tests: shorten sleeps via a package var `retryBackoff = 1 * time.Second` that tests set to `time.Millisecond`.

- [ ] **Step 2: Run to verify they fail** — Run: `go test ./internal/datastore/ -run TestInTx -v` — Expected: FAIL (no retry today)

- [ ] **Step 3: Implement retry inside `inTx`** — keep the success path allocation-free; metrics via expvar Ints defined in Task 1? No — define the two new counters here in metrics.go (the expvar names go in the Global Constraints list of observability).

- [ ] **Step 4: Run to verify they pass** — Run: `go test ./internal/datastore/ -v && go build ./...` — Expected: PASS

- [ ] **Step 5: Commit** — `git commit -m "feat(datastore): bounded retry with backoff for cycle writes; write metrics"`

### Task 7: Retention prune + VACUUM + free-space guard

**Files:**
- Create: `internal/datastore/prune.go`
- Modify: `internal/datastore/store.go` (export `dbSize` consumer)
- Modify: `internal/metrics/metrics.go` (add `DatastorePruneRows = expvar.NewInt("datastore_prune_rows_total")`, `DatastorePruneLastRun = expvar.NewInt("datastore_prune_last_run")`)
- Test: `internal/datastore/prune_test.go`

**Interfaces:**
- Produces: `RetentionWeeks() int` (env `FLUFFY_RETENTION_WEEKS`, default 5, parsed once in `Init`); `Prune(weeks int, resetDate Reset) error` — cutoff = `resetDate parsed as 2006-01-02 15:04 UTC` (13:00) minus `weeks*7*24h`; deletes series rows `timestamp < cutoff` and per-reset rows `reset_date < cutoff.Format("2006-01-02")`; then VACUUM behind the guard (skip with log when `freeSpace(dbFileDir) < 1.25*dbSize() + 32MB`); sets prune metrics.

- [ ] **Step 1: Write the failing tests**

```go
func TestPrune_KeepsFiveFullWeeks(t *testing.T)
    // seed series rows for week starts W1..W7 (W7 = "current"); Prune(5, W7)
    // assert: rows with timestamp < W7start-35d are gone; W2..W6 full rows + W7 rows all present
func TestPrune_BoundaryRowKept(t *testing.T)   // row exactly at cutoff timestamp survives
func TestPrune_PerResetTablesCut(t *testing.T)  // stats/leaderboard/snapshot/jumpgate/faction rows for dropped weeks gone; current intact
func TestPrune_VacuumSkippedWhenSpaceShort(t *testing.T)
    // force guard by requiring free < 1.25*dbSize+32MB via injected tiny fake threshold
    // (make the margin a package var `vacuumMargin` tests can raise to a huge value)
    // assert: rows still pruned, no VACUUM ran (guard logged), no error
```

- [ ] **Step 2: Run to verify they fail** — Run: `go test ./internal/datastore/ -run TestPrune -v` — Expected: FAIL (no `Prune`)

- [ ] **Step 3: Implement `prune.go`** — one `inTx` for all deletes (so a prune is atomic); VACUUM outside the tx (it can't run inside one), guarded. Parse reset date exactly as `time.Parse("2006-01-02 15:04", string(resetDate)+" 13:00")` in **UTC**.

- [ ] **Step 4: Run to verify they pass** — Run: `go test ./internal/datastore/ -v` — Expected: PASS

- [ ] **Step 5: Commit** — `git commit -m "feat(datastore): weekly retention prune at reset boundary + guarded vacuum"`

### Task 8: Collector wiring — factions fetch, prune hook, error surfacing

**Files:**
- Modify: `internal/collector/agents.go` (rename `updateFactionss`→`updateFactions`; factions trigger in `updateStatus`; `StoreAgents` error logging)
- Modify: `internal/collector/collector.go` (prune call at the end of `loopAtReset`)

**Interfaces:**
- Consumes: `StoreFactions`, `Prune`, `RetentionWeeks`, `StoreAgents` (now returns error).

- [ ] **Step 1: Wire factions once per reset**

In `updateStatus`: compare `status.ResetDate` to the pre-update `c.currentReset`; if different (including the fresh-boot `""` case), after storing stats/leaderboards call `c.updateFactions(ctx)` — errors are logged, never fatal. `updateFactions` paginates `/factions?limit=20` (existing dead function body — reuse, renamed; fix the double `datastore` import alias in this file).

- [ ] **Step 2: Wire the prune hook**

At the end of `loopAtReset`, after the final chart-empty branch returns (new reset date in scope as `status.ResetDate`): `if err := ds.Prune(ds.RetentionWeeks(), ds.Reset(status.ResetDate)); err != nil { logging.Error("prune failed", err) }`. This is the only prune call site — inside the Sunday quiet window, never mid-week.

- [ ] **Step 3: Surface store errors at call sites**

`StoreAgents` call: `if err := datastore.StoreAgents(allAgents, c.currentTimestamp); err != nil { logging.Error("agent cycle write failed", err) }`. Same one-line treatment for the jumpgate store calls (Task 4 added the signatures).

- [ ] **Step 4: Verify**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all green (no unit tests possible without HTTP mocking — behavior verified at Task 12 runbook step: factions row appears on first status of a new reset; prune log line appears at reset).

- [ ] **Step 5: Commit** — `git commit -m "feat(collector): wire factions fetch per reset, weekly prune hook, store error logging"`

### Task 9: Frontend bounded queries; remove the `resets` global race

**Files:**
- Modify: `internal/frontend/handlers.go` (`LoadChartHandler` ~:65-72, `PermissionsHandler` ~:124, `AgentsGridHandler` ~:455-461, `JumpgatesHandler` ~:324-329)
- Modify: `internal/frontend/charts.go` (delete `latestShips` — now SQL-side)
- Modify: `internal/frontend/frontend.go` (delete `resets` global + `updateResetLoop` goroutine + its `StartServer` launch)

**Interfaces:**
- Consumes: `GetAgentHistory(reset, start, end, agents)`, `LatestShips()`, `LatestConstructions()`, `LatestReset()`.
- Produces: no handler touches `resets`; `/chart` queries only the chart's agents.

- [ ] **Step 1: Bound `/chart`**

`thisReset := ds.LatestReset()` (replaces `resets[0]`); `agentHist, _ := ds.GetAgentHistory(thisReset, startTime, 0, chartAgents)` — if `chartAgents` is empty the store returns an empty slice and the charts render empty (no all-agents fallback, Review Focus #1). Keep the `nil`-outs.

- [ ] **Step 2: Aggregate latest-* in the grid/jumpgate handlers**

`AgentsGridHandler`: drop `agentHist, _ := ds.GetAgentHistory(thisReset, 0, 0)` + `latestShips(agentHist)`; use `ships := ds.LatestShips()`. `JumpgatesHandler`: `constrList := ds.LatestConstructions()` (latest row per gate), feed it to `latestConstructionRecords` unchanged (transform stays pure). `PermissionsHandler`: `ds.GetAgentList(ds.LatestReset())` instead of `resets[0]`. Delete `latestShips` from `charts.go`.

- [ ] **Step 3: Delete the race**

Remove `var resets`, `updateResetLoop`, and `go updateResetLoop()` from `frontend.go`. Verify with: `grep -n "resets\[0\]\|updateResetLoop" internal/frontend/` — Expected: zero hits.

- [ ] **Step 4: Verify**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: green. (`NextReset`/`AllResets` now likely unreferenced — confirmed and deleted in Task 11.)

- [ ] **Step 5: Commit** — `git commit -m "feat(frontend): bounded chart/series queries; drop resets global and its data race"`

### Task 10: Export endpoint serves a consistent snapshot

**Files:**
- Create: `internal/datastore/export.go`
- Modify: `internal/frontend/handlers.go` (`ExportHandler`)

**Interfaces:**
- Produces: `ExportDB(dst string) error` — free-space guard (skip-with-error when `free < 1.25*dbSize()+vacuumMargin`), then `VACUUM INTO ?` to `dst` (temp dir), returning the path's file handle streaming left to the handler: handler writes headers, `os.Open`+`io.Copy`, removes temp file. Content-Type `application/octet-stream`, filename `fluffy.db`.

- [ ] **Step 1: Write the failing test**

```go
func TestExportDB_ValidSnapshot(t *testing.T)
    // seed data; ExportDB(tmp/"export.db"); open result with modernc driver
    // assert: query returns seeded rows; source db still usable afterwards
func TestExportDB_SkippedWhenSpaceShort(t *testing.T)
    // raise guard margin to huge; assert error mentions free space; no dst file
```

- [ ] **Step 2: Run to verify it fails** — Run: `go test ./internal/datastore/ -run TestExportDB -v` — Expected: FAIL

- [ ] **Step 3: Implement `export.go` + handler swap** — `VACUUM INTO` cannot run inside a transaction; guard first. The handler streams the temp file, then removes it (never the live db or WAL files).

- [ ] **Step 4: Run to verify it passes** — Run: `go test ./internal/datastore/ -v && go build ./...` — Expected: PASS

- [ ] **Step 5: Commit** — `git commit -m "feat(export): /export streams a VACUUM INTO snapshot with free-space guard"`

### Task 11: Delete the old store path, decoder tool, dead types and deps

**Files:**
- Modify: `internal/datastore/datastore.go` (delete `writeData`, `readData`, `writeJSON`, zstd import), `internal/datastore/types.go`, `internal/datastore/consolidate.go` (delete file), `tools/decoder/main.go` (delete file), `internal/metrics/metrics.go` (delete `DatastoreCacheResets`; delete `NextReset`/`AllResets` if `grep -rn "NextReset()\|AllResets()" --include="*.go" .` shows no callers outside datastore)
- Modify: `go.mod` (`go mod tidy` drops `klauspost/compress`)
- Modify: `internal/datastore/agent.go` etc. only if the compiler demands (it shouldn't — types stay)

**Interfaces:**
- Consumes: nothing new. Dead types to delete from `types.go` (verify each with grep first): `TimedAgentRecord`, `resetAgentKey`, `LeaderboardRecord`, `JumpGateAgentListStruct`. Keep: `Reset`, `ResponseStatus`, `Faction`, `PublicAgent`, `Agent`, `AgentStatus`, `DataPoint`, `Stats`, `JGInfo`, `JGConstruction`, `ConstructionOverview`, `ConstructionRecord`, and the `ConstructionStatus` enum (collector uses it).

- [ ] **Step 1: Delete + tidy + grep-verify**

Delete the files/functions listed above. Run: `go mod tidy && grep -rn "FLUFFY_WRITE_JSON\|libsql\|klauspost\|DatastoreCacheResets" --include="*.go" .`
Expected: zero hits in Go sources.

- [ ] **Step 2: Verify**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: green. `go.sum`/`go.mod` no longer reference libsql-client-go or klauspost/compress.

- [ ] **Step 3: Commit** — `git commit -m "chore: remove gob/zst store, decoder tool, dead types, vestigial deps"`

### Task 12: Race suite, docs, cutover runbook

**Files:**
- Modify: `AGENTS.md` (env table: remove `FLUFFY_WRITE_JSON`, add `FLUFFY_RETENTION_WEEKS` default 5; Conventions: remove the libsql line, note the SQLite store; Data Format: single `.db` file + `sqlite3` CLI)
- Modify: `docs/INTERNAL.md` (rewrite the Datastore section: schema, pragmas, bounded query contract, retention, export, metrics; mark Known Issues 1–7 resolved/obsolete where applicable; project structure: no `tools/decoder`, new store files)
- Modify: `README.md` (Data Storage: SQLite on PVC; env examples updated; remove `FLUFFY_WRITE_JSON`)

- [ ] **Step 1: Full verification**

Run: `go test -race ./... && go vet ./... && go fmt ./...`
Expected: race clean, all green (frontend reads + collector writes now exercise WAL concurrency under `-race`).

- [ ] **Step 2: Write the cutover runbook into INTERNAL.md**

Append to the Deployment section: (1) merge + deploy at the Sunday 13:00 UTC reset boundary — fresh DB starts with the new reset; (2) first status fetch triggers the factions fetch; (3) delete old `.gob.zst` reset dirs in week one (frees ~220 MB; until then VACUUM/export may skip with a logged free-space guard); (4) watch `/debug/vars` (`datastore_write_errors_total`, `datastore_write_retries_total`, `datastore_prune_last_run`) and the Railway RAM graph for one full reset week — success = flat at the post-bounce floor; (5) check DB size after the first full week — if it exceeds ~250 MB, consult the owner about dropping to 4 weeks or decimating older weeks (spec §size math).

- [ ] **Step 3: Commit** — `git commit -m "docs: sqlite datastore docs + cutover runbook; drop gob-path references"`

- [ ] **Step 4: Open the PR** — push the branch to `origin`, open the PR against `main` on the forgejo, description summarizing the spec→plan traceability and the issue mapping (#7 umbrella; resolves #3, #4, #6; #5 fixed in Task 3).