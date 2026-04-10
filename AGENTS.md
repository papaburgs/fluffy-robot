# Fluffy Robot

Go dashboard for the SpaceTraders public API. No auth token needed.

## Commands

- **Run:** `go run main.go`
- **Build:** `go build -o fluffy-robot main.go`
- **Tests:** `go test ./...` — only `internal/gate` has tests; long-running gate tests need `GO_TEST_LONG=true go test ./internal/gate/...`
- **Format:** `go fmt ./...`
- **Vet:** `go vet ./...`
- **Decode data files:** `go run tools/decoder/main.go <file.gob.zst>`

No CI workflows exist in this repo.

## Config (env vars)

| Variable | Default | Description |
|----------|---------|-------------|
| `FLUFFY_PORT` | `8845` | HTTP server port (`:` prefix auto-added if missing) |
| `FLUFFY_STORAGE_PATH` | `./` | Data storage directory |
| `FLUFFY_GATE_BUCKET_SIZE` | `20` | Rate limit bucket size (t1 limit is hardcoded to 2 in `main.go`) |
| `FLUFFY_WRITE_JSON` | no | Accepts `yes`, `y`, or `true` (case-insensitive) to enable JSON alongside `.gob.zst` |
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
- **Datastore** (`internal/datastore/`): File-based storage under `{FLUFFY_STORAGE_PATH}/{reset_date}/`. Exports raw `Get*` functions that load from disk on demand. Time-series loaders (`GetAgentHistory`, `GetConstructions`) accept `start, end int64` params to filter records; pass `0, 0` for all data. `LatestReset()` and `NextReset()` busy-wait (sleep loop) until `currentReset` is set by the collector.
- **Frontend** (`internal/frontend/`): HTTP dashboard using `html/template` and `go-echarts/v2`. Handlers load raw data from datastore once per request, then call pure transform functions (`agentsMap`, `latestShips`, `jumpgatesMap`, `latestConstructionRecords`, etc.) defined in `charts.go`. Transform functions never touch the filesystem — they only reshape in-memory data. Intermediate lists are nil'd out for GC.
- **Gate** (`internal/gate/`): Rate limiter for SpaceTraders API calls. Two-tier: t1 (per-second) and t60 (per-minute) with bucket capacity.
- **Metrics** (`internal/metrics/`): `expvar` counters at `/debug/vars`.
- **Logging** (`internal/logging/`): `fmt.Println`-based. No `slog`.

Startup in `main.go`: `logging.InitLogger()` → `datastore.Init()` → 1s sleep → collector goroutine → 2s sleep → `frontend.StartServer()` (blocking). The sleeps are race-condition guards; the collector and datastore need time to settle before the frontend serves requests.

## Data Format

Primary: compressed Gob (`.gob.zst`). JSON opt-in via `FLUFFY_WRITE_JSON`.

`readData()` dispatches on file prefixes: `agents`, `agentsStatus`, `stats`, `leaderboard`, `jumpgates`, `construction`, `factions`.

## Conventions

- Game resets weekly; collector pauses tickers ~3 min before reset and polls until new reset appears.
- Intermediate lists set to `nil` after use to ease GC pressure.
- `.json` and `.gob.zst` files are gitignored.
- Static files always served from disk (no embed).
- `libsql-client-go` is in `go.mod` but not referenced in code — likely vestigial, do not add new uses.
- `stretchr/testify` is an indirect dependency; only `internal/gate` has tests.