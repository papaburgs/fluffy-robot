# Fluffy Robot Dashboard

This project is a Go web application designed to track and display statistics for SpaceTraders agents. It provides a dashboard to visualize agent credit trends over various time periods and allows users to manage selected agents.

## Project Overview

*   **Purpose:** To collect, store, and visualize SpaceTraders agent data, offering insights into agent credit trends.
*   **Main Technologies:**
    *   **Backend:** Go (version 1.24.5)
    *   **Web Framework:** Standard Go `net/http`
    *   **Charting:** `go-echarts`
    *   **Frontend Interactivity:** `htmx.org`
    *   **Containerization:** Docker
*   **Architecture:** The application operates as a web server, serving HTML templates, static assets, and dynamic content. It includes internal components for agent data collection, caching, and processing. Data is collected from the SpaceTraders API.
*   **Key Features:**
    *   Agent data collection from SpaceTraders API.
    *   Dashboard with interactive credit charts for 1h, 4h, 24h, and 7d periods.
    *   Agent selection and persistence in browser local storage.
    *   Export of application data (backup.json).

Emphasis is on maintaining a small memory footprint and efficient use of cpu.

## Building and Running

### Prerequisites

*   Go (version 1.24.5 or higher)
*   Docker (if building/running with containers)

### Build Commands

1.  **Build from Source:**
    ```bash
    go mod tidy
    go build -o app -ldflags "-s -w" *.go
    ```
2.  **Build Docker Image:**
    ```bash
    docker build -t fluffy-robot .
    ```

### Run Commands

1.  **Run Executable:**
    ```bash
    ./app
    ```
    The application will listen on `http://localhost:8845`.

2.  **Run Docker Container:**
    ```bash
    docker run -p 8845:8845 fluffy-robot
    ```
    The application will be accessible at `http://localhost:8845`.

### Configuration (Environment Variables)

The application can be configured using the following environment variables:

*   `COLLECTIONS_DISABLED`: Set to `"true"` to disable data collection. (e.g., `COLLECTIONS_DISABLED="true" ./app`)
*   `SPACETRADER_LEADERBOARD_STATIC_DEV`: If set (e.g., to `"1"`), static files will be served directly from the `./static/` directory. Otherwise, embedded static files are used.
*   `SPACETRADER_LEADERBOARD_BACKUP_PATH`: Specifies the local directory for storing collected agent data. Defaults to the current directory (`.`).
*   `SPACETRADER_LEADERBOARD_LOG_LEVEL`: Sets the logging verbosity. Accepted values: `"debug"`, `"warn"`, `"error"`, `"info"` (default).

## Development Conventions

*   **Code Formatting:** Standard Go formatting (`go fmt`).
*   **Testing:** Unit tests are written using Go's built-in testing framework, often augmented with `github.com/stretchr/testify` for assertions. Example: `go test ./internal/app/...`
*   **Logging:** Structured logging is implemented using the `slog` package.
*   **Frontend Development:**
    *   `htmx.org` is used for dynamic content updates, minimizing JavaScript.
    *   `go-echarts` is integrated for data visualization.
    *   Frontend state, such as selected agents, is managed using browser `localStorage`.
*   **Project Structure:** Follows a typical Go project layout with `internal` packages for reusable logic and `templates` and `static` directories for web assets.
