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
- **Datastore**: Stores and caches collected data with automatic cleanup
- **Frontend**: HTTP server serving the dashboard UI

## Quick Start

```bash
# Run the application
go run main.go

# Configuration via environment variables
FLUFFY_PORT=8845                    # HTTP server port (default: 8845)
FLUFFY_STORAGE_PATH=./data          # Data storage directory
FLUFFY_CACHE_DURATION=5m            # Cache lifetime
FLUFFY_GATE_BUCKET_SIZE=20          # Rate limit bucket size
FLUFFY_STATIC_DEV=yes               # Use external static files (dev mode)
```

## Data Storage

The datastore saves data in two formats:
- **Compressed Gob** (`.gob.zst`) - Primary format, memory efficient
- **JSON** (`.json`) - Optional, enabled via `FLUFFY_WRITE_JSON=yes`

Data is organized by reset date with automatic cleanup of stale data.

