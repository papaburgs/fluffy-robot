# Developer Guide

## Overview

Fluffy Robot is a dashboard application that collects and displays public data from the SpaceTraders API. It operates without authentication, polling public endpoints for leaderboards, agent statistics, and jumpgate information.

## Architecture Overview

```
main.go
├── collector.NewCollector()  # Data collection service
├── datastore.Init()           # Storage initialization
└── frontend.StartServer()    # HTTP server
```

### Collector (`internal/collector/`)

The collector is responsible for fetching data from the SpaceTraders API on scheduled intervals.

**Key Components:**

- `collector.go` - Main collector loop with tickers for different update frequencies:
  - Agent updates: Every 5 minutes
  - Jumpgate updates: Every 30 minutes
  - Construction updates: Every 4 hours
  - Reset detection: Weekly (7 days)

- `api.go` - HTTP client for SpaceTraders API calls

- `agents.go` - Agent data fetching and processing

- `jumpgates.go` - Jumpgate data fetching and construction tracking

**Rate Limiting (`internal/gate/`):**

The collector uses a `Gate` to enforce API rate limits:
- Per-second limit (default: 1 request)
- Per-minute limit (default: 20 requests)

The `Gate.Latch()` method blocks until a request slot is available.

### Datastore (`internal/datastore/`)

The datastore handles all data storage and caching.

**Storage Format:**

Data is stored in two formats:
1. **Compressed Gob** (`.gob.zst`) - Default, using zstd compression
2. **JSON** (`.json`) - Optional, enabled with `FLUFFY_WRITE_JSON=yes`

**Data Organization:**

```
{FLUFFY_STORAGE_PATH}/
└── {reset_date}/
    ├── agents-{timestamp}.gob.zst
    ├── agents-{timestamp}.json
    ├── jumpgates-{timestamp}.gob.zst
    └── ...
```

**Key Types (`types.go`):**

- `Reset` - A string type alias representing a reset date (e.g., "2024-01-02")
- `Agent` - Agent data with symbol, credits, faction, headquarters
- `Stats` - Server statistics per reset
- `LeaderboardEntry` - Symbol and value for rankings
- `JGInfo` - Jumpgate information and construction status
- `JGConstruction` - Construction progress tracking

**Global State Maps:**

```go
agentsList        map[Reset][]Agent
agentHistory      map[Reset][]AgentStatus
stats             map[Reset]Stats
creditLeaders     map[Reset][]LeaderboardEntry
chartLeaders      map[Reset][]LeaderboardEntry
jumpgateLists     map[Reset][]JGInfo
constructionsLists map[Reset][]JGConstruction
```

**Cache Behavior:**

- Cache lifetime defaults to 5 minutes (`FLUFFY_CACHE_DURATION`)
- `watchTimer()` can trigger `zero()` to clear in-memory data after inactivity
- Data is loaded from disk on demand via `readData()`

**Key Functions:**

- `Init()` - Initializes storage path and cache settings
- `UpdateReset(r Reset)` - Sets current reset and creates directory
- `writeData()` - Saves data in both gob.zst and JSON formats
- `readData()` - Reads compressed gob files and returns buffers

### Frontend (`internal/frontend/`)

The frontend is an HTTP server that serves the dashboard UI.

**Key Files:**

- `frontend.go` - Server initialization, route registration, and template setup
- `handlers.go` - HTTP request handlers for all endpoints
- `charts.go` - Chart data processing and display

**Template Functions:**

```go
add              - Integer addition
unixTime         - Unix timestamp to formatted string
constructionStatus - Construction status code to string
```

**Environment Variables:**

- `FLUFFY_PORT` - Server port (default: 8845)
- `FLUFFY_STATIC_DEV` - Use external static files instead of embedded
- `FLUFFY_TEMPLATE_DIR` - Custom template directory path

**Routes:**

| Route | Handler | Description |
|-------|---------|-------------|
| `/` | RootHandler | Main dashboard |
| `/leaderboard` | LeaderboardHandler | Credit and chart rankings |
| `/stats` | StatsHandler | Server statistics |
| `/jumpgates` | JumpgatesHandler | Jumpgate listing |
| `/chart` | LoadChartHandler | Chart details |
| `/permissions` | PermissionsHandler | Agent permissions |
| `/permissions-grid` | PermissionsGridHandler | Grid view of permissions |
| `/status` | HeaderHandler | Status header |
| `/export` | ExportHandler | Data export endpoint |

## Data Flow

### Collection Flow

1. Collector starts and calls `updateStatus()` to get current reset
2. Tickers trigger periodic updates at different intervals
3. `Gate.Latch()` ensures rate limits are respected
4. API responses are unmarshaled into types from datastore
5. Data is saved via `writeData()` to disk
6. In-memory maps are updated for fast access

### Request Flow

1. HTTP request arrives at frontend
2. Handler calls appropriate datastore getter function
3. Getter checks in-memory map, loads from disk if empty
4. Data is processed and served as HTML/JSON

## Reset Handling

The game server has weekly resets. The collector:

1. Sets a timer for 3 minutes before expected reset
2. When timer fires, stops all tickers
3. Polls status endpoint until reset completes
4. Detects reset completion when:
   - Reset date matches today
   - Leaderboards are empty (new reset)
5. Restarts tickers with new reset date

## Development

### Adding a New Data Type

1. Define the type in `internal/datastore/types.go`
2. Add a global map variable in `internal/datastore/datastore.go`
3. Add getter/setter functions in datastore package
4. Add collector logic in `internal/collector/` to fetch and save
5. Add frontend handler if needed in `internal/frontend/`

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FLUFFY_PORT` | 8845 | HTTP server port |
| `FLUFFY_STORAGE_PATH` | ./ | Data storage directory |
| `FLUFFY_CACHE_DURATION` | 5m | In-memory cache lifetime |
| `FLUFFY_GATE_BUCKET_SIZE` | 20 | Rate limit bucket size |
| `FLUFFY_WRITE_JSON` | no | Enable JSON file output |
| `FLUFFY_STATIC_DEV` | no | Use external static files |
| `FLUFFY_TEMPLATE_DIR` | internal/frontend | Template directory |

### Testing

```bash
go test ./...
go build -o fluffy-robot main.go
```

### Project Structure

```
fluffy-robot/
├── main.go                 # Application entry point
├── internal/
│   ├── collector/          # Data collection
│   │   ├── collector.go    # Main loop and scheduling
│   │   ├── api.go          # API client
│   │   ├── agents.go       # Agent data
│   │   └── jumpgates.go    # Jumpgate data
│   ├── datastore/          # Data storage
│   │   ├── datastore.go    # Storage logic
│   │   └── types.go        # Type definitions
│   ├── frontend/           # HTTP frontend
│   │   ├── frontend.go     # Server setup
│   │   ├── handlers.go     # Request handlers
│   │   └── charts.go       # Chart handling
│   ├── gate/               # Rate limiting
│   │   └── gate.go         # Token bucket implementation
│   └── logging/            # Logging setup
└── docs/                   # Documentation
```
