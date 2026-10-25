# Developer Guide

## Overview

Fluffy Robot is a dashboard application that collects and displays public data from the SpaceTraders API. It operates without authentication, polling public endpoints for leaderboards, agent statistics, and jumpgate information.

**Primary design goal: lean on memory.** The app runs as a single container on Railway, where RAM is the dominant cost (CPU is light except during ingestion bursts). Every change should assume memory is the constraint. The datastore is an embedded SQLite database (see [Datastore](#datastore-internaldatastore) — one bounded file, SQL-windowed reads, no load-then-filter); it replaced the file-based store whose whole-reset loads were the main RAM driver.

## Architecture Overview

```
main.go
├── datastore.Init()            # storage dir setup
├── collector.NewCollector(gate.New(2, FLUFFY_GATE_BUCKET_SIZE), baseURL)  # poller goroutine
└── frontend.StartServer()      # blocking HTTP server
```

Startup sequence in `main.go`: `logging.InitLogger()` → `datastore.Init()` → 1s sleep → collector goroutine (`c.Run`) → 2s sleep → `frontend.StartServer()` (blocking). The sleeps are race-condition guards; the collector and datastore need time to settle before the frontend serves requests.

The collector and the frontend share no in-memory state: the SQLite database is the channel — the collector writes one transaction per cycle, the frontend reads narrow windows per request. There is **no in-memory cache layer** (an older cached implementation was removed long before the SQLite migration).

## Collector (`internal/collector/`)

Scheduled intervals (`collector.go`):

| Ticker | Interval | Work |
|--------|----------|------|
| Agents | 5 min | `updateStatus` + `updateAgents` (re-aims reset timer) |
| Jumpgates | 30 min | `updateJumpgates` (systems under construction) |
| Construction | 4 h | `updateInactiveJumpgates` (systems not yet started) |
| Supply chain | 24 h | `updateSupplyChain` (no-op once the static chain is collected) |

**Key files:**

- `collector.go` — main loop, tickers, reset detection (`loopAtReset`)
- `api.go` — `doGET` HTTP client with retries: up to 5 retries on 429 (also calls `gate.Lock()` to block the rate limiter for a window), up to 3 retries on other errors; a new `http.Client` per call, but they share `http.DefaultTransport`, so connection pooling still applies
- `agents.go` — `updateStatus` (GET `/`: stores stats + leaderboards, sets current reset), `updateAgents` (paginates `/agents?limit=20`, stores agent snapshot + status history, then `updateJumpgatesFromAgents`). Also `updateFactionss` — **dead code, never called**; no faction data is ever collected
- `jumpgates.go` — discovers jumpgate waypoint per agent system (`findJumpgateSymbol`), tracks construction progress (`fetchConstructionStatus`), marks systems `Active`/`Const`/`Complete`, and writes synthetic 1600 FAB_MATS / 400 ADVANCED_CIRCUITRY records for completed gates so charts keep updating
- `supplychain.go` — `updateSupplyChain`: fetches the global supply chain (export→imports map) from `GET /market/supply-chain` exactly once. Gated on `SupplyChainExists()`; an empty/absent `exportToImportMap` is logged and not stored (next tick retries from scratch). Runs once at startup (non-fatal on error) and on its own ticker; success bumps `collector_supply_chain_updates_total`

Reset detection (`loopAtReset`): stops all tickers, sleeps 3 min, then polls the status endpoint once a minute until `resetDate` is today AND the most-submitted-charts leaderboard is empty. Note: the trailing most-credits check in that loop is unreachable (the charts-empty branch returns first) — dead code kept for reference.

## Rate Limiting (`internal/gate/`)

Token-bucket-ish two-tier limiter driven by tickers:

- **t1**: per ~1s window, limit **2** (hardcoded in `main.go`: `gate.New(2, gateBucketSize)`)
- **t60**: per minute, limit `FLUFFY_GATE_BUCKET_SIZE` (default 20)

`Gate.Latch(ctx)` queues a channel and blocks until the `loop()` goroutine grants a slot (t1 bucket first, then t60). `Gate.Lock(ctx)` sets both counters far above their limits, blocking everything until the next window tick (used on 429s). Blocked grants back off in 100 ms steps. Only `internal/gate` has tests; long ones need `GO_TEST_LONG=true`.

## Datastore (`internal/datastore/`)

Embedded **SQLite** — one file, `{FLUFFY_STORAGE_PATH}/fluffy.db`, opened once per process
(`modernc.org/sqlite`, pure Go; works with the existing `CGO_ENABLED=0` alpine build).
Pragmas at open: `journal_mode=WAL` (concurrent readers while the collector writes),
`synchronous=FULL` (a hard kill cannot lose the last cycle), `busy_timeout=10000`,
`foreign_keys=ON`, `cache_size=-4000` (4 MiB page-cache cap — keeps RSS flat; mmap stays off).
Schema version lives in `PRAGMA user_version` (currently 2).

**Tables** (see `schema.go` for the DDL):

- `agents` — global symbol registry (id ↔ symbol); series rows store the id.
- `agent_snapshots` — per-reset latest attributes (replaced wholesale per agent tick, inside the cycle tx).
- `agent_status` — time series `(agent_id, timestamp, credits, ships)`, `WITHOUT ROWID`, clustered on the key.
- `stats` — one row per reset (upsert).
- `leaderboards` — `(reset_date, kind, rank)` rows; kind is `credits` or `charts`.
- `jumpgates` — per-reset gate list with `status` (NoActivity/Active/Const/Complete) and `completed_at`.
- `construction` — time series `(jumpgate, timestamp, fabmat, advcct)`, `WITHOUT ROWID`.
- `factions` — per-reset rows; traits stored as a JSON array.
- `supply_chain` — static export→imports map, one row per `(export, import)` pair plus `fetched_at`,
  `WITHOUT ROWID`. Not reset-scoped and not pruned: stored once via `StoreSupplyChain`
  (`INSERT OR REPLACE`, one tx), read via `GetSupplyChain`, existence check `SupplyChainExists`.

**Writes:** every collection cycle is **one transaction** (`inTx`) — a partial cycle can never exist.
Natural keys (minute-rounded timestamps, reset_date + symbol) use `INSERT OR REPLACE`, so retries are
idempotent. A failed cycle write is retried with exponential backoff (1s, 2s, 4s … 64s, at most 7
attempts ≈ 2 min) then dropped loudly: `datastore_write_errors_total` increments and the caller logs.

**Reads:** getters take time windows (and agent sets) and filter in SQL — nothing on any request path
allocates proportional to total stored data:

- `GetAgentHistory(reset, start, end, agents)` — `WHERE a.symbol IN (…) AND s.timestamp BETWEEN …`;
  an empty (non-nil) agent slice returns zero rows (never an all-agents fallback).
- `GetConstructions(reset, start, end)` — window on `timestamp`.
- `GetAgentList(reset)` — snapshot join for that reset; empty `reset` means the current one
  (same convention on all reset-scoped getters).
- Aggregates: `LatestShips()`, `LatestConstructions()` — `GROUP BY … MAX(timestamp)` instead of
  a full-window scan.

**Reset handling:** `Reset` is a date key, not a directory. `UpdateReset(r)` flips an in-memory var only.
`LatestReset()` busy-waits until the collector's first status fetch (unchanged behavior on fresh boot).

**Retention:** keep 5 full game weeks + the current partial week; on the Sunday 13:00 UTC reset
transition (inside the collector's quiet window) `Prune()` deletes the week that just became
6th-oldest (`DELETE … WHERE timestamp < newResetStart − 5×7d` on series tables; `reset_date <` that
date on per-reset tables), then VACUUMs behind a free-space guard (skip when free < 1.25×DB + 32 MiB —
the ~220 MB of gob leftovers can push the 500 MB PVC short; ENOSPC must never break collection).
`FLUFFY_RETENTION_WEEKS` (default 5, floor 4) tunes the count. Nothing is ever deleted mid-week.

**Export:** `/export` streams a `VACUUM INTO` snapshot (consistent, WAL-safe) behind the same
free-space guard; 503 with a message when the guard trips.

## Frontend (`internal/frontend/`)

HTTP dashboard on the default mux using `html/template` + `go-echarts/v2`.

**Key files:**

- `frontend.go` — server setup, template parsing (`{FLUFFY_TEMPLATE_DIR}/templates/*.html`), static file serving (`{FLUFFY_STATIC_DIR}/static`, always from disk), and `mergeAgents` (reflect-based helper for the agent-selection query params)
- `handlers.go` — request handlers; each loads raw data from the datastore once per request, transforms, and nils intermediates for GC
- `charts.go` — pure transform functions and chart builders; never touch the filesystem

**Routes:**

| Route | Handler | Description |
|-------|---------|-------------|
| `/` | RootHandler | Main dashboard |
| `/agents` | AgentsHandler | Faction/system filter options |
| `/agents-grid` | AgentsGridHandler | Agent table (search/sort/filters) |
| `/permissions` | PermissionsHandler | Agent permissions |
| `/permissions-grid` | PermissionsGridHandler | Grid view of permissions |
| `/leaderboard` | LeaderboardHandler | Credit and chart rankings (`?type=credits\|charts`) |
| `/stats` | StatsHandler | Server statistics |
| `/jumpgates` | JumpgatesHandler | Jumpgate construction parallel chart |
| `/chart` | LoadChartHandler | Credit/ship/construction charts (`?period=24h\|4h\|7d\|1h`, agent lists via `storageAgents`/`paramAgents`) |
| `/supply-chain` | SupplyChainHandler | Supply chain tree (`?mode=needs\|products`, `?good=SYMBOL`) |
| `/status` | HeaderHandler | Status header |
| `/export` | ExportHandler | Streams a consistent `fluffy.db` snapshot (`VACUUM INTO`, free-space guarded; 503 when short) |
| `/static/*` | http.FileServer | Static assets |
| `/debug/vars` | expvar | Metrics |

**Template functions:** `add` (int addition), `unixTime` (unix → `2006-01-02 15:04`), `constructionStatus` (code → name).

**Charts pipeline:** handlers pull raw slices from the datastore → pure transforms in `charts.go` (`agentsMap`, `jumpgatesMap`, `latestConstructionRecords`, `constructionRecords`, `agentRecordsCredits/Ships` with downsampling to ~50 points/series) → echarts objects → `RenderSnippet()` → injected as `template.HTML` fragments. Inactive agents are identified by `credits == 175000` (starting balance).

## Data Flow

### Collection flow

1. `updateStatus` gets the status document, stores stats + leaderboards, sets the current reset.
2. Tickers fire `updateAgents` / `updateJumpgates` / `updateInactiveJumpgates` at their intervals.
3. `Gate.Latch()` serializes all API calls through the two-tier limiter.
4. Responses are unmarshaled into `datastore` types and written in **one transaction per cycle** (`Store*` → `inTx`), then read back by the frontend in narrow SQL windows.

### Request flow

1. A handler calls datastore getters, which **read from disk on every request** (no cache).
2. Pure transforms reshape the data; intermediates are nil'd for GC.
3. Output is rendered as HTML (or injected echarts fragments).

## Reset Handling

The game server resets weekly (from the API's `serverResets.frequency`):

1. The reset timer is initially 7 days; every agent tick re-aims it to `nextReset - 3 min` (from the API's `serverResets.next`).
2. When it fires: tickers stop, one last status fetch runs, then a 3-minute sleep.
3. `loopAtReset` polls once a minute until the status shows `resetDate` == today and the charts leaderboard is empty.
4. Tickers restart for the new reset; reset is a data column — nothing new is created, and the weekly retention prune + guarded VACUUM just ran inside `loopAtReset`.

## Deployment

- **Dockerfile**: two-stage build — `golang:1` builder with `CGO_ENABLED=0` (statically linked binary), minimal `alpine` runtime with the binary, `templates/`, `static/`, and CA certs. The build being CGO-free constrains driver choices (see the SQLite migration issue).
- **Railway**: single container; RAM is the dominant cost, CPU is light except during ingestion and history-heavy page loads. Keep the resident set and per-request spikes low.

### SQLite cutover runbook (one-time)

1. Merge + deploy at the Sunday 13:00 UTC reset boundary — the fresh DB starts with the new game.
2. The first status fetch of the new reset also triggers the factions fetch; expect a `factions` write in the first cycle logs.
3. Delete the old `*.gob.zst` reset directories from the PVC in week one (frees ~220 MB). Until then, VACUUM and `/export` may skip with a logged free-space guard — that is intentional (ENOSPC must never starve the collector's writes).
4. Watch `/debug/vars` for a full reset week: `datastore_write_errors_total` and `datastore_write_retries_total` should be flat zero; `datastore_prune_last_run` advances once per reset. Railway RAM graph: success = flat at the post-bounce floor (~50 MB), no weekly climb.
5. If the DB exceeds ~250 MB after the first full week, consult the owner about dropping retention to 4 weeks or decimating older weeks (see the design spec). Do not silently reduce retention below 4 weeks.

- **Metrics**: `expvar` counters/durations at `/debug/vars` — collector (`collector_*`), gate (`gate_*`), datastore (`datastore_*`), and per-handler durations (`handler_<name>` with `count`/`total_ms`/`latest_ms`).

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FLUFFY_PORT` | `8845` | HTTP server port (`:` prefix auto-added if missing) |
| `FLUFFY_STORAGE_PATH` | `./` | Data directory; the store is `fluffy.db` inside it |
| `FLUFFY_RETENTION_WEEKS` | `5` | Full game weeks kept (floor 4); pruned at the Sunday reset boundary |
| `FLUFFY_GATE_BUCKET_SIZE` | `20` | t60 (per-minute) rate limit; t1 limit is hardcoded to 2 in `main.go` |
| `FLUFFY_AGENT_TICK` | `5m` | Agent + status collection interval (Go duration syntax, e.g. `30s`) |
| `FLUFFY_JUMPGATE_TICK` | `30m` | Jumpgate (under construction) progress interval |
| `FLUFFY_CONSTRUCTION_TICK` | `4h` | Scan for newly-started jumpgate construction |
| `FLUFFY_SUPPLY_TICK` | `24h` | Supply chain refresh check (no-op once collected) |
| `FLUFFY_TEMPLATE_DIR` | `internal/frontend` | Directory containing `templates/` |
| `FLUFFY_STATIC_DIR` | `internal/frontend` | Directory containing `static/` |
| `FLUFFY_LOG_LEVEL` | info | Set `debug` or `dbg` to enable `logging.Debug()` output |

> Removed: `FLUFFY_CACHE_DURATION` and `FLUFFY_STATIC_DEV` (old cache layer). `FLUFFY_WRITE_JSON` was retired with the gob.zst store — query JSON with the `sqlite3` CLI instead.

## Known Issues

The SQLite migration (forgejo #3/#4/#5/#6/#7) resolved the former datastore issues:

1. ~~Memory pressure on retrieval~~ — **fixed**: history getters query by window (+ agent set) in SQL.
2. ~~Cross-reset reads broken~~ — **fixed**: reset is a column; `GetStats`/`GetLeaderboard`/`GetAgentList`(etc.) take a reset arg, empty ⇒ latest.
3. ~~FD pressure~~ — **gone**: there are no per-record files; the store is one DB file.
4. ~~Panic in `StoreStats` on null `stats.accounts`~~ — **fixed**: nil ⇒ 0.
5. ~~Ignored write errors~~ — **fixed**: writes return errors, log loudly, count expvar, and retry with backoff.
6. ~~`resets` data race / `resets[0]` panic~~ — **fixed**: the `resets` global and its goroutine are gone; handlers use `LatestReset()`.

Remaining (non-datastore): none known. Report new ones via forgejo issues.

Historical (pre-migration) notes live in git history.

# Deployment runbook lives below in Deployment.

## Development

### Commands

```bash
go run main.go                      # run
go build -o fluffy-robot main.go    # build
go test ./...                       # tests (internal/gate + internal/datastore)
go test -race ./...                 # race-check (frontend reads while collector writes)
GO_TEST_LONG=true go test ./internal/gate/...   # long-running gate tests
go fmt ./...
go vet ./...
sqlite3 "$FLUFFY_STORAGE_PATH/fluffy.db"            # inspect the store (e.g. SELECT * FROM stats)
```

No CI workflows exist in this repo.

### Adding a New Data Type

1. Define the type in `internal/datastore/types.go` and a table in `internal/datastore/schema.go` (bump `user_version` when existing tables change — `applySchema` runs idempotently at open).
2. Add a `Store*` function inside `inTx`; natural keys should make retries idempotent (`INSERT OR REPLACE` on minute-rounded timestamps / reset-date keys).
3. Add a `Get*` function with SQL-side filtering (time window / reset / agent set) — never load-then-filter in memory.
4. Add collector logic in `internal/collector/` to fetch and store (log write errors; `inTx` retries are already counted).
5. Add frontend handler(s) and route(s) in `internal/frontend/`.
6. Inspect data with `sqlite3 "$FLUFFY_STORAGE_PATH/fluffy.db"`.

### Project Structure

```
fluffy-robot/
├── main.go                 # Entry point: init datastore, collector goroutine, frontend
├── Dockerfile              # CGO_ENABLED=0 multi-stage build (alpine runtime)
├── internal/
│   ├── collector/          # Data collection
│   │   ├── collector.go    # Tickers, reset detection
│   │   ├── api.go          # HTTP client, retries, response types
│   │   ├── agents.go       # Status, agents, factions (once per reset)
│   │   └── jumpgates.go    # Jumpgate discovery + construction tracking
│   ├── datastore/          # Embedded SQLite store (modernc.org/sqlite)
│   │   ├── datastore.go    # Init, UpdateReset, path helpers
│   │   ├── store.go        # openDB + pragmas, inTx (bounded retry), freeSpace/dbSize
│   │   ├── schema.go       # schema v1 DDL (PRAGMA user_version)
│   │   ├── agent.go        # Agent registry, snapshot, time-windowed history
│   │   ├── status.go       # Stats, leaderboards, LatestReset
│   │   ├── jumpgates.go    # Jumpgate list + construction series
│   │   ├── factions.go     # Per-reset factions (collected once per reset)
│   │   ├── prune.go        # Weekly 5-week retention prune + guarded VACUUM
│   │   ├── export.go       # VACUUM INTO snapshot behind free-space guard
│   │   └── types.go        # Type definitions
│   ├── frontend/           # HTTP frontend
│   │   ├── frontend.go     # Server, routes, templates, static
│   │   ├── handlers.go     # Request handlers
│   │   ├── charts.go      # Pure transforms + echarts builders
│   │   ├── templates/      # HTML templates (from disk)
│   │   └── static/        # CSS, SVG (from disk)
│   ├── gate/               # Rate limiting (t1/t60) + tests
│   ├── logging/            # fmt.Println-based logging
│   └── metrics/            # expvar counters
└── docs/                   # INTERNAL.md + superpowers spec/plan
```