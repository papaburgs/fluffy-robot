# Fluffy Robot

## Run

```bash
go run main.go
```

## Config (env vars)

| Variable | Default | Description |
|----------|---------|-------------|
| `FLUFFY_PORT` | `8845` | HTTP server port |
| `FLUFFY_STORAGE_PATH` | `./data` | Data storage directory |
| `FLUFFY_CACHE_DURATION` | `5m` | Cache lifetime |
| `FLUFFY_GATE_BUCKET_SIZE` | `20` | Rate limit bucket size |
| `FLUFFY_STATIC_DEV` | no | Use external static files (dev mode) |
| `FLUFFY_WRITE_JSON` | no | Enable JSON output (default: compressed Gob `.gob.zst`) |

## Architecture

```
Collector ──▶ Datastore ◀── Frontend (HTTP)
    │                           │
    └─────── SpaceTraders API ──┘
```

- **Collector** (`internal/collector/`): Gathers data on scheduled intervals
- **Datastore** (`internal/datastore/`): Stores/caches with auto-cleanup, organized by reset date
- **Frontend** (`internal/frontend/`): HTTP dashboard UI

Entry point: `main.go`

## Tests

```bash
go test ./...
```

Long-running gate tests skipped by default; run with `GO_TEST_LONG=true go test ./internal/gate/...`.

## Data Format

Primary: compressed Gob (`.gob.zst`). JSON opt-in via `FLUFFY_WRITE_JSON=yes`.
