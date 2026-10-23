# Fluffy Robot

A dashboard project that displays public leaderboards and public agent data from the SpaceTraders API. Data is retrieved without requiring a token, making it a read-only public-facing interface.

## Project Overview

This application collects and displays:
- **Public Leaderboards** - Credit and chart submission rankings
- **Public Agent Data** - Agent statistics, ship counts, and credits
- **Jumpgate Information** - Jumpgate statuses and construction progress
- **Server Statistics** - Game server stats and reset information
- **Faction Summary** - Duplicate home systems and count by faction

## Current limitations
- Only Display current reset, no history, although a month of resets is stored, future goal is to have it there
- Construction of jumpgate might be up to 4 hours behind, only currently known _builders_ are tracked, inactive agents and non building agents are not updated frequently to save api calls

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  Collector  │────▶│  Datastore  │◀────│   Frontend  │
│             │     │             │     │   (HTTP)    │
└─────────────┘     └─────────────┘     └─────────────┘
      │                                        │
      └───────────── SpaceTraders API ─────────┘
```

- **Collector**: Gathers data from the SpaceTraders public API on scheduled intervals
- **Datastore**: Embedded SQLite (`fluffy.db`); every cycle is one transaction, reads are SQL-windowed. No cache layer, no outside service.
- **Frontend**: HTTP server serving the dashboard UI

See [docs/INTERNAL.md](docs/INTERNAL.md) for the full developer guide. The app is deployed as a single container on Railway; **the primary design goal is low memory**.

## Quick Start

```bash
# Run the application
go run main.go

# Configuration via environment variables
FLUFFY_PORT=8845                    # HTTP server port (default: 8845)
FLUFFY_STORAGE_PATH=./data          # Data directory; the store is fluffy.db inside it
FLUFFY_GATE_BUCKET_SIZE=20          # Rate limit bucket size (per-minute)
FLUFFY_RETENTION_WEEKS=5            # Full game weeks kept (default 5, floor 4)
FLUFFY_TEMPLATE_DIR=internal/frontend  # Directory containing templates/ (optional)
FLUFFY_STATIC_DIR=internal/frontend    # Directory containing static/ (optional)
```

## Data Storage

The datastore is a single embedded SQLite database (`{FLUFFY_STORAGE_PATH}/fluffy.db`, driver `modernc.org/sqlite`, WAL mode). Reset week is a column, not a directory. Collection cycles commit as one transaction; failed writes retry with bounded backoff. Retention prunes to the last 5 full game weeks (plus the current one) automatically at the Sunday reset boundary. Inspect with `sqlite3`; `FLUFFY_WRITE_JSON` no longer exists.


