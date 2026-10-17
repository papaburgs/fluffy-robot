# Fluffy Robot

A dashboard project that displays public leaderboards and public agent data from the SpaceTraders API. Data is retrieved without requiring a token, making it a read-only public-facing interface.

## Project Overview

This application collects and displays:
- **Public Leaderboards** - Credit and chart submission rankings
- **Public Agent Data** - Agent statistics, ship counts, and credits
- **Jumpgate Information** - Jumpgate statuses and construction progress
- **Server Statistics** - Game server stats and reset information

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
- **Datastore**: Stores collected data as per-reset files on disk; read from disk on every request (no cache layer)
- **Frontend**: HTTP server serving the dashboard UI

See [docs/INTERNAL.md](docs/INTERNAL.md) for the full developer guide. The app is deployed as a single container on Railway; **the primary design goal is low memory**.

## Quick Start

```bash
# Run the application
go run main.go

# Configuration via environment variables
FLUFFY_PORT=8845                    # HTTP server port (default: 8845)
FLUFFY_STORAGE_PATH=./data          # Data storage directory
FLUFFY_GATE_BUCKET_SIZE=20          # Rate limit bucket size (per-minute)
FLUFFY_TEMPLATE_DIR=internal/frontend  # Directory containing templates/ (optional)
FLUFFY_STATIC_DIR=internal/frontend    # Directory containing static/ (optional)
FLUFFY_WRITE_JSON=yes               # Also write JSON alongside .gob.zst (optional)
```

## Data Storage

The datastore saves data in two formats:
- **Compressed Gob** (`.gob.zst`) - Primary format, memory efficient
- **JSON** (`.json`) - Optional, enabled via `FLUFFY_WRITE_JSON=yes`

Data is organized by reset date: latest snapshots are overwritten each cycle, history files (agent status, construction progress) are timestamped appends. There is no automatic cleanup.


