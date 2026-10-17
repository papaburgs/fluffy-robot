# Developer Guide

## Overview

Fluffy Robot is a dashboard application that collects and displays public data from the SpaceTraders API. It operates without authentication, polling public endpoints for leaderboards, agent statistics, and jumpgate information.

**Primary design goal: lean on memory.** The app runs as a single container on Railway, where RAM is the dominant cost (CPU is light except during ingestion bursts). Every change should assume memory is the constraint. The current file-based datastore has known memory-pressure problems in retrieval (see [Known issues](#known-issues)); replacing it with a SQLite backend is the planned structural fix, tracked on the internal Forgejo.

## Architecture Overview

```
main.go
├── datastore.Init()            # storage dir setup
├── collector.NewCollector(gate.New(2, FLUFFY_GATE_BUCKET_SIZE), baseURL)  # poller goroutine
└── frontend.StartServer()      # blocking HTTP server
```

Startup sequence in `main.go`: `logging.InitLogger()` → `datastore.Init()` → 1s sleep → collector goroutine (`c.Run`) → 2s sleep → `frontend.StartServer()` (blocking). The sleeps are race-condition guards; the collector and datastore need time to settle before the frontend serves requests.

The collector and the frontend do not share in-memory data structures. The filesystem is the only channel between them: the collector writes files, the frontend re-reads them on every request. There is **no in-memory cache layer** (an older cached implementation was removed; some leftovers like the `datastore_cache_resets_total` metric and `datastore.consolidate()` are dormant).

## Collector (`internal/collector/`)

Scheduled intervals (`collector.go`):

| Ticker | Interval | Work |
|--------|----------|------|
| Agents | 5 min | `updateStatus` + `updateAgents` (re-aims reset timer) |
| Jumpgates | 30 min | `updateJumpgates` (systems under construction) |
| Construction | 4 h | `updateInactiveJumpgates` (systems not yet started) |

**Key files:**

- `collector.go` — main loop, tickers, reset detection (`loopAtReset`)
- `api.go` — `doGET` HTTP client with retries: up to 5 retries on 429 (also calls `gate.Lock()` to block the rate limiter for a window), up to 3 retries on other errors; a new `http.Client` per call, but they share `http.DefaultTransport`, so connection pooling still applies
- `agents.go` — `updateStatus` (GET `/`: stores stats + leaderboards, sets current reset), `updateAgents` (paginates `/agents?limit=20`, stores agent snapshot + status history, then `updateJumpgatesFromAgents`). Also `updateFactionss` — **dead code, never called**; no faction data is ever collected
- `jumpgates.go` — discovers jumpgate waypoint per agent system (`findJumpgateSymbol`), tracks construction progress (`fetchConstructionStatus`), marks systems `Active`/`Const`/`Complete`, and writes synthetic 1600 FAB_MATS / 400 ADVANCED_CIRCUITRY records for completed gates so charts keep updating

Reset detection (`loopAtReset`): stops all tickers, sleeps 3 min, then polls the status endpoint once a minute until `resetDate` is today AND the most-submitted-charts leaderboard is empty. Note: the trailing most-credits check in that loop is unreachable (the charts-empty branch returns first) — dead code kept for reference.

## Rate Limiting (`internal/gate/`)

Token-bucket-ish two-tier limiter driven by tickers:

- **t1**: per ~1s window, limit **2** (hardcoded in `main.go`: `gate.New(2, gateBucketSize)`)
- **t60**: per minute, limit `FLUFFY_GATE_BUCKET_SIZE` (default 20)

`Gate.Latch(ctx)` queues a channel and blocks until the `loop()` goroutine grants a slot (t1 bucket first, then t60). `Gate.Lock(ctx)` sets both counters far above their limits, blocking everything until the next window tick (used on 429s). Blocked grants back off in 100 ms steps. Only `internal/gate` has tests; long ones need `GO_TEST_LONG=true`.

## Datastore (`internal/datastore/`)

File-based storage under `{FLUFFY_STORAGE_PATH}/{reset_date}/`. Two formats per write: compressed Gob (`.gob.zst`, always) and pretty-printed JSON (`.json`, only when `FLUFFY_WRITE_JSON=yes|y|true`).

**File naming:** "latest" snapshots are written with timestamp `0` — `agents-0.gob.zst`, `jumpgates-0.gob.zst`, `stats-0.gob.zst`, `leaderboard-0.gob.zst` — and are overwritten every cycle. Time series get real timestamps: `agentsStatus-{unixts}.gob.zst`, `construction-{unixts}.gob.zst` (appended every cycle).

**Read path (hot spot):** `readData(prefix, thisReset)` scans the reset directory and loads *every* matching file, fully decompressed, into `map[string]*bytes.Buffer`. Getters then gob-decode *all* of it and filter in memory:

- `GetAgentHistory(reset, start, end)` / `GetConstructions(reset, start, end)` decode the **entire history** for the reset, then drop records outside `[start, end]`. A week of 5-minute agent snapshots (~2000 files) is held in RAM at once per request — this is the main memory-pressure suspect.
- Single-file getters (`GetAgentList`, `GetJumpgateList`, `GetStats`, `GetLeaderboard`, `GetFactions`) expect exactly one file to match their prefix.
- All files opened during a `readData` call stay open until the function returns (`defer` inside the loop).

**Reset handling:** `UpdateReset(r)` sets `currentReset`, builds `resetPath`, creates the directory. `LatestReset()` and `NextReset()` busy-wait (sleep loop) until the collector has set `currentReset` — calling them before the first status fetch blocks.

**Consolidation is disabled:** `consolidate.go` exists and would rewrite N history files as one, but its call sites in `GetAgentHistory`/`GetConstructions` are commented out. The `consolidating` flag is still set/cleared pointlessly, and getters busy-wait on it.

**Known sharp edges** (see also [Known issues](#known-issues)):

- `readData` lists the requested reset's directory but opens files from the *current* reset's path — reads for a non-current reset fail.
- `GetStats(r)` and `GetJumpgates(r)` ignore their `r` argument and always read the current reset.
- `StoreStats` dereferences `r.Stats.Accounts` (a `*int`) without a nil check — panics if the API omits `stats.accounts`.
- `StoreAgents`, `StoreStats`, `StoreLeaderboards` ignore `writeData` errors.
- The `factions` write/read path is fully dead (never collected).

**Useful helpers:** `SystemFromWaypoint("X1-Y-Z")` → `"X1-Y"`, `DataPath()` → storage root, `AllResets()` → reset-date directory names, newest first.

## Frontend (`internal/frontend/`)

HTTP dashboard on the default mux using `html/template` + `go-echarts/v2`.

**Key files:**

- `frontend.go` — server setup, template parsing (`{FLUFFY_TEMPLATE_DIR}/templates/*.html`), static file serving (`{FLUFFY_STATIC_DIR}/static`, always from disk), the `resets` global refreshed by the `updateResetLoop` goroutine, and `mergeAgents` (reflect-based helper for the agent-selection query params)
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
| `/status` | HeaderHandler | Status header |
| `/export` | ExportHandler | `tar.gz` of the whole storage directory |
| `/static/*` | http.FileServer | Static assets |
| `/debug/vars` | expvar | Metrics |

**Template functions:** `add` (int addition), `unixTime` (unix → `2006-01-02 15:04`), `constructionStatus` (code → name).

**Charts pipeline:** handlers pull raw slices from the datastore → pure transforms in `charts.go` (`agentsMap`, `jumpgatesMap`, `latestShips`, `latestConstructionRecords`, `constructionRecords`, `agentRecordsCredits/Ships` with downsampling to ~50 points/series) → echarts objects → `RenderSnippet()` → injected as `template.HTML` fragments. Inactive agents are identified by `credits == 175000` (starting balance).

The `resets` slice is written by a goroutine and read by handlers without synchronization (data race, benign in practice), and `LoadChartHandler` indexes `resets[0]`, which panics if no reset data exists yet.

## Data Flow

### Collection flow

1. `updateStatus` gets the status document, stores stats + leaderboards, sets the current reset.
2. Tickers fire `updateAgents` / `updateJumpgates` / `updateInactiveJumpgates` at their intervals.
3. `Gate.Latch()` serializes all API calls through the two-tier limiter.
4. Responses are unmarshaled into `datastore` types and written via `writeData` (gob.zst, optional JSON).

### Request flow

1. A handler calls datastore getters, which **read from disk on every request** (no cache).
2. Pure transforms reshape the data; intermediates are nil'd for GC.
3. Output is rendered as HTML (or injected echarts fragments).

## Reset Handling

The game server resets weekly (from the API's `serverResets.frequency`):

1. The reset timer is initially 7 days; every agent tick re-aims it to `nextReset - 3 min` (from the API's `serverResets.next`).
2. When it fires: tickers stop, one last status fetch runs, then a 3-minute sleep.
3. `loopAtReset` polls once a minute until the status shows `resetDate` == today and the charts leaderboard is empty.
4. Tickers restart for the new reset; a new reset-date directory is created on the next status fetch.

## Deployment

- **Dockerfile**: two-stage build — `golang:1` builder with `CGO_ENABLED=0` (statically linked binary), minimal `alpine` runtime with the binary, `templates/`, `static/`, and CA certs. The build being CGO-free constrains driver choices (see the SQLite migration issue).
- **Railway**: single container; RAM is the dominant cost, CPU is light except during ingestion and history-heavy page loads. Keep the resident set and per-request spikes low.
- **Metrics**: `expvar` counters/durations at `/debug/vars` — collector (`collector_*`), gate (`gate_*`), datastore (`datastore_*`), and per-handler durations (`handler_<name>` with `count`/`total_ms`/`latest_ms`).

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FLUFFY_PORT` | `8845` | HTTP server port (`:` prefix auto-added if missing) |
| `FLUFFY_STORAGE_PATH` | `./` | Data storage directory |
| `FLUFFY_GATE_BUCKET_SIZE` | `20` | t60 (per-minute) rate limit; t1 limit is hardcoded to 2 in `main.go` |
| `FLUFFY_WRITE_JSON` | no | Accepts `yes`, `y`, or `true` (case-insensitive) to write JSON alongside `.gob.zst` |
| `FLUFFY_TEMPLATE_DIR` | `internal/frontend` | Directory containing `templates/` |
| `FLUFFY_STATIC_DIR` | `internal/frontend` | Directory containing `static/` |
| `FLUFFY_LOG_LEVEL` | info | Set `debug` or `dbg` to enable `logging.Debug()` output |

> Historical: `FLUFFY_CACHE_DURATION` and `FLUFFY_STATIC_DEV` no longer exist (the cache layer was removed; static serving uses `FLUFFY_STATIC_DIR`).

## Known Issues

Documented so the SQLite migration can address them; also tracked on the internal Forgejo.

1. **Memory pressure on retrieval** — history getters decompress and decode the *entire* reset history into RAM per request, then filter. Biggest RAM driver and the primary motivation for the SQLite backend.
2. **Cross-reset reads broken** — `readData` opens files from the current reset path even when asked for another reset.
3. **FD pressure** — every file opened in `readData`'s loop stays open until the function returns.
4. **Potential panic in `StoreStats`** — `*r.Stats.Accounts` deref without nil check.
5. **Ignored write errors** — `StoreAgents`/`StoreStats`/`StoreLeaderboards` return success regardless of `writeData` outcome.
6. **`resets` data race / `resets[0]` panic risk** in the frontend.
7. **Dead code** — `updateFactionss` (factions never collected), unreachable most-credits branch in `loopAtReset`, disabled `consolidate()`, unused `TimedAgentRecord`/`resetAgentKey` types and `datastore_cache_resets_total` metric, `GetFactions`/decoder `factions` path.
8. **Stale Dockerfile comment** — says "scratch", actually uses alpine (fixed in this repo).

## Development

### Commands

```bash
go run main.go                      # run
go build -o fluffy-robot main.go    # build
go test ./...                       # tests (only internal/gate has them)
GO_TEST_LONG=true go test ./internal/gate/...   # long-running gate tests
go fmt ./...
go vet ./...
go run tools/decoder/main.go <file.gob.zst>     # decode a data file to JSON
```

No CI workflows exist in this repo.

### Adding a New Data Type

1. Define the type in `internal/datastore/types.go`.
2. Add a `Store*` function (call `writeData` with a basename; use timestamp `0` for a latest-snapshot file, real timestamps for time series).
3. Add a `Get*` function (use `readData` + gob decode).
4. Add collector logic in `internal/collector/` to fetch and store.
5. Add frontend handler(s) and route(s) in `internal/frontend/`.
6. Add a prefix case in `tools/decoder/main.go` so the file type can be inspected.

### Project Structure

```
fluffy-robot/
├── main.go                 # Entry point: init datastore, collector goroutine, frontend
├── Dockerfile              # CGO_ENABLED=0 multi-stage build (alpine runtime)
├── internal/
│   ├── collector/          # Data collection
│   │   ├── collector.go    # Tickers, reset detection
│   │   ├── api.go          # HTTP client, retries, response types
│   │   ├── agents.go       # Status, agents, factions (dead)
│   │   └── jumpgates.go    # Jumpgate discovery + construction tracking
│   ├── datastore/          # File-based storage (gob.zst / json)
│   │   ├── datastore.go    # Init, writeData/readData
│   │   ├── agent.go        # Agent snapshot + history
│   │   ├── status.go       # Stats, leaderboards, reset helpers
│   │   ├── jumpgates.go    # Jumpgate list + construction history
│   │   ├── factions.go      # Dead path (never collected)
│   │   ├── consolidate.go  # Disabled compaction of history files
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
├── tools/decoder/           # gob.zst → JSON decoder for data files
└── docs/                   # INTERNAL.md (this guide)
```