# Data Collector

The Collector is a background service responsible for gathering real-time data from the SpaceTraders API and persisting it for visualization in the dashboard. It manages rate limiting, tracks agent performance, and monitors the construction progress of jumpgates across the galaxy.

## File Layout

- `cmd/collect/main.go`: Application entry point. Handles configuration, database initialization, and the main execution loop (`Run`).
- `cmd/collect/agents.go`: Implements status and agent ingestion logic, including leaderboard updates and agent credit tracking.
- `cmd/collect/jumpgates.go`: Implements jumpgate discovery and construction tracking.
- `internal/db/db.go`: Defines the schema and handles the Turso/libSQL connection.

## The Collection Process

The collector operates on a multi-ticker cycle, balancing high-frequency agent tracking with lower-frequency discovery and construction monitoring. It uses a "Gate" mechanism to respect SpaceTraders' rate limits.

### Execution Flow in `Run`

1.  **Initial Scan**: On startup, it runs `updateJumpgates` once to immediately refresh status for any jumpgates already known to be under construction.
2.  **Ticker Loop**:
    *   **Agent Ticker (5m)**: Updates server status, leaderboards, and agent credits/ships.
    *   **Jumpgate Ticker (30m)**: Checks progress for jumpgates currently in the `jsConst` (Under Construction) state.
    *   **Construction Ticker (4h)**: Runs `updateInactiveJumpgates` to check "Active" systems (those with active agents but no recorded construction) for new building activity.
3.  **Dynamic Reset**: A `resetTimer` is calculated from the API's `nextReset` field. When triggered, it stops all tickers and pauses for 15 minutes to allow the server reset to complete before resuming.

### Key Functions

| Function | Purpose |
| :--- | :--- |
| `updateStatusAgents` | Orchestrates the 5-minute update of server stats, leaderboards, and all agents. |
| `updateStatus` | Fetches `/v2/` to get server health, version, and the current "Most Credits/Charts" leaderboards. |
| `updateAgents` | Iterates through all agents in the API, applying filters and recording current credits/ships. |
| `updateJumpgatesFromAgents` | Uses agent headquarters to discover new systems and their respective jumpgates. |
| `updateJumpgates` | Fetches construction status for jumpgates already marked as "Under Construction" (`jsConst`). |
| `updateInactiveJumpgates` | Checks "Active" jumpgates (`jsActive`) to see if construction has begun; if so, upgrades them to `jsConst`. |

## Database Schema (Tables Written To)

| Table | Description |
| :--- | :--- |
| `stats` | Stores server-wide metadata (version, reset date, next reset time, etc.). |
| `leaderboard` | Stores the top 20 agents by credits and charts for the current reset. |
| `agents` | Metadata for every agent (faction, headquarters, current credit total). |
| `agentstatus` | **Time-series.** Records credits and ship counts for every agent at 5-minute intervals. |
| `jumpgates` | Tracks jumpgate state (0: No Activity, 1: Active, 2: Under Construction, 3: Complete). |
| `construction` | **Time-series.** Records material fulfillment (`FAB_MATS`, `ADVANCED_CIRCUITRY`). |

## Agent Filtering

Filter out specific agents (e.g., test bots or specific factions) using regex to keep the database focused.

*   **Variable**: `FLUFFY_AGENT_IGNORE_FILTERS`
*   **Format**: A semi-colon separated list of regular expressions.
*   **Example**: `^TEST_;^BOT_;^OLD_`

## Frequency Summary

| Task | Frequency |
| :--- | :--- |
| Server Status & Leaderboards | Every 5 Minutes |
| Agent Credits/Ships | Every 5 Minutes |
| Construction Progress (Active) | Every 30 Minutes |
| New Construction Discovery | Every 4 Hours |
| Discovery of New JGs | Every 5 Minutes (via Agent update) |

## Configuration

*   `FLUFFY_TURSO_URL`: Turso/libSQL database URL.
*   `FLUFFY_TURSO_AUTH_TOKEN`: Auth token for Turso.
*   `FLUFFY_GATE_BUCKET_SIZE`: API burst bucket size (default: 20).
*   `FLUFFY_AGENT_IGNORE_FILTERS`: Regex patterns for excluding agents.
