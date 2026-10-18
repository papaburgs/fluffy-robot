# Fluffy Robot

Go dashboard for the SpaceTraders public API. No auth token needed.

**Primary goal: lean on memory.** The app runs as a single container on Railway, where RAM is the dominant cost (CPU is light except during ingestion). Optimize for RAM before anything else.

## Commands

- **Run:** `go run main.go`
- **Build:** `go build -o fluffy-robot main.go`
- **Tests:** `go test ./...` — `internal/gate` and `internal/datastore` have tests; long-running gate tests need `GO_TEST_LONG=true go test ./internal/gate/...`; race-check with `go test -race ./...`
- **Format:** `go fmt ./...`
- **Vet:** `go vet ./...`
- **Inspect the database:** `sqlite3 {FLUFFY_STORAGE_PATH}/fluffy.db` (queries, JSON via `json_group_array` etc.; no in-repo decoder needed)

No CI workflows exist in this repo.

## Config (env vars)

| Variable | Default | Description |
|----------|---------|-------------|
| `FLUFFY_PORT` | `8845` | HTTP server port (`:` prefix auto-added if missing) |
| `FLUFFY_STORAGE_PATH` | `./` | Data directory; the store is a single `fluffy.db` SQLite file inside it |
| `FLUFFY_GATE_BUCKET_SIZE` | `20` | Rate limit bucket size (t1 limit is hardcoded to 2 in `main.go`) |
| `FLUFFY_AGENT_TICK` | `5m` | Agent + status collection interval (Go duration syntax, e.g. `30s`) |
| `FLUFFY_JUMPGATE_TICK` | `30m` | Jumpgate (under construction) progress interval |
| `FLUFFY_CONSTRUCTION_TICK` | `4h` | Scan for newly-started jumpgate construction |
| `FLUFFY_RETENTION_WEEKS` | `5` | Full game weeks kept in the DB (floor 4); older weeks pruned at the Sunday reset boundary |
| `FLUFFY_TEMPLATE_DIR` | `internal/frontend` | Directory containing `templates/` |
| `FLUFFY_STATIC_DIR` | `internal/frontend` | Directory containing `static/` |
| `FLUFFY_LOG_LEVEL` | info | Set `debug` or `dbg` to enable `logging.Debug()` output |

## Architecture

```
Collector ──▶ Datastore ◀── Frontend (HTTP)
    │                           │
    └─────── SpaceTraders API ──┘
```

- **Collector** (`internal/collector/`): Gathers data on scheduled intervals (agents: 5m, jumpgates: 30m, construction: 4h). No auth token needed — all public API. Base URL is hardcoded in `main.go`: `https://api.spacetraders.io/v2`.
- **Datastore** (`internal/datastore/`): Embedded **SQLite** (driver `modernc.org/sqlite` — pure Go; the build stays `CGO_ENABLED=0`). One file: `{FLUFFY_STORAGE_PATH}/fluffy.db`, one process-wide `*sql.DB`, WAL + `synchronous=FULL`. Reset is a column, not a directory. Every collection cycle is a single transaction; failed cycle writes retry with bounded backoff (1s, 2s … 32s on the six retries, ≈1 min total, 7 attempts) before being dropped loudly (`datastore_write_errors_total`). Time-series getters take `start, end` (plus an agent-set filter for `GetAgentHistory`); pass `0, 0` for all data. `LatestReset()` busy-waits until the collector's first status fetch.
- **Frontend** (`internal/frontend/`): HTTP dashboard using `html/template` and `go-echarts/v2`. Handlers load raw data from datastore once per request, then call pure transform functions (`agentsMap`, `jumpgatesMap`, `latestConstructionRecords`, etc.) defined in `charts.go`. Transform functions never touch the filesystem — they only reshape in-memory data. Intermediate lists are nil'd out for GC.
- **Gate** (`internal/gate/`): Rate limiter for SpaceTraders API calls. Two-tier: t1 (per-second) and t60 (per-minute) with bucket capacity.
- **Metrics** (`internal/metrics/`): `expvar` counters at `/debug/vars`.
- **Logging** (`internal/logging/`): `fmt.Println`-based. No `slog`.

Startup in `main.go`: `logging.InitLogger()` → `datastore.Init()` → 1s sleep → collector goroutine → 2s sleep → `frontend.StartServer()` (blocking). The sleeps are race-condition guards; the collector and datastore need time to settle before the frontend serves requests.

## Data Format

Primary: one SQLite database (`fluffy.db`; `modernc.org/sqlite`, WAL mode). Tables: `agents` (symbol registry), `agent_snapshots` (per-reset latest), `agent_status` / `construction` (time series, `WITHOUT ROWID`), `stats`, `leaderboards`, `jumpgates`, `factions`. The weekly retention prune (=5 weeks) runs at the Sunday 13:00 UTC reset boundary.

## Conventions

- Game resets weekly (Sunday ~13:00 UTC); collector pauses tickers ~3 min before reset and polls until new reset appears; the datastore prune and VACUUM run in that quiet window.
- Factions are fetched once per reset (first status fetch of a new game).
- Intermediate lists set to `nil` after use to ease GC pressure (frontend paths).
- `.db` and any `.gob.zst` leftovers are gitignored.
- Static files always served from disk (no embed).

## Git
- when making changes, only apply to worktrees
- origin remote is an internal forgejo repo, use this one exclusively
  - github repo is for deployments only, you are not allowed to use that
- worktrees are made in `.worktrees/` which is in gitignore
- you can commit in worktrees, push to 'origin', and make pull requests there
- do not push, pull, or make PRs against the 'deploy' remote
- do not commit directly against main, that is a protected branch

## Issues
- the forgejo repo ('origin') is the source of truth for issue tracking
- the forgejo-mcp server is available and you are allowed to use it to create and update issues there
