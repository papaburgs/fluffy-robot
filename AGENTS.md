# Fluffy Robot

## Run

```bash
go run main.go
```

## Commands

- **Build:** `go build -o fluffy-robot main.go`
- **Tests:** `go test ./...` — only `internal/gate` has tests; long-running gate tests need `GO_TEST_LONG=true go test ./internal/gate/...`
- **Format:** `go fmt ./...`
- **Vet:** `go vet ./...`
- **Decode data files:** `go run tools/decoder/main.go <file.gob.zst>`

## Config (env vars)

| Variable | Default | Description |
|----------|---------|-------------|
| `FLUFFY_PORT` | `8845` | HTTP server port |
| `FLUFFY_STORAGE_PATH` | `./` | Data storage directory |
| `FLUFFY_CACHE_DURATION` | `5m` | Cache lifetime |
| `FLUFFY_GATE_BUCKET_SIZE` | `20` | Rate limit bucket size |
| `FLUFFY_WRITE_JSON` | no | Enable JSON output (default: compressed Gob `.gob.zst`) |
| `FLUFFY_TEMPLATE_DIR` | `internal/frontend` | Directory containing `templates/` |
| `FLUFFY_STATIC_DIR` | `internal/frontend` | Directory containing `static/` |
| `FLUFFY_LOG_LEVEL` | info | Set `debug` or `dbg` to enable `logging.Debug()` output |

## Architecture

```
Collector ──▶ Datastore ◀── Frontend (HTTP)
    │                           │
    └─────── SpaceTraders API ──┘
```

- **Collector** (`internal/collector/`): Gathers data on scheduled intervals (agents: 5m, jumpgates: 30m, construction: 4h). No auth token needed — all public API.
- **Datastore** (`internal/datastore/`): File-based storage under `{FLUFFY_STORAGE_PATH}/{reset_date}/`. Exports `Get*` functions that load from disk on demand and return data. No global in-memory maps.
- **Frontend** (`internal/frontend/`): HTTP dashboard. Handlers call datastore `Get*` functions, transform data into template shapes, nil out intermediates for GC.
- **Metrics** (`internal/metrics/`): `expvar` counters for collector, gate, datastore, and per-handler duration tracking. Published at `/debug/vars`.
- **Logging** (`internal/logging/`): Minimal `fmt.Println`-based. `Debug()`, `Info()`, `Warn()` (prefixes `level=warn`), `Error()` (prefixes `level=Error`). No `slog`.

Entry point: `main.go`

## Data Format

Primary: compressed Gob (`.gob.zst`). JSON opt-in via `FLUFFY_WRITE_JSON=yes`.

Data file prefixes matter — `readData()` and the decoder tool dispatch on these: `agents`, `agentsStatus`, `stats`, `leaderboard`, `jumpgates`, `construction`.

## Conventions

- Game resets weekly; collector pauses tickers ~3 min before reset and polls until new reset appears.
- Datastore `Get*` functions return raw data; frontend handlers build maps/filter as needed.
- Intermediate lists set to `nil` after use to ease GC pressure.
- `.json` and `.gob.zst` files are gitignored.
- Static files always served from disk (no embed).