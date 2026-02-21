package main

import (
	"embed"
	"net/http"
	"os"
	"strconv"

	"log/slog"

	_ "embed"

	"github.com/papaburgs/fluffy-robot/internal/app"
	"github.com/papaburgs/fluffy-robot/internal/db"
)

//go:embed static
var staticFiles embed.FS // This variable now holds the entire 'static' directory

func collectionsEnabled() bool {
	enabled := true
	collectionsDisabledEnv := os.Getenv("COLLECTIONS_DISABLED")
	if collectionsDisabledEnv != "" {
		disabled, err := strconv.ParseBool(os.Getenv("COLLECTIONS_DISABLED"))
		if err != nil {
			slog.Error("error parsing boolean value from env COLLECTIONS_DISABLED", "error", err.Error())
		}
		if disabled {
			enabled = false
		}
	}
	slog.Info("collections enabled", "value", enabled)
	return enabled
}

func main() {

	database, err := db.Connect()
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := db.InitSchema(database); err != nil {
		slog.Error("failed to initialize schema", "error", err)
		os.Exit(1)
	}

	////////// Static file handling \\\\\\\\\\
	// define this value to be something in order to use the 'external' css to make it easier to work on
	// leaving it unset will embed the directory instead to make it easier for a server
	if _, ok := os.LookupEnv("SPACETRADER_LEADERBOARD_STATIC_DEV"); ok {
		// Dev case
		fs := http.FileServer(http.Dir("./cmd/gui/static"))
		http.Handle("/static/", http.StripPrefix("/static/", fs))
	} else {
		// production case
		fsHandler := http.FileServer(http.FS(staticFiles))
		http.Handle("/static/", fsHandler)
	}

	storageLocation := "."
	if loc, ok := os.LookupEnv("SPACETRADER_LEADERBOARD_BACKUP_PATH"); ok {
		storageLocation = loc
	}
	slog.Debug("storage location:", "base", storageLocation)

	a := app.NewApp(storageLocation, collectionsEnabled(), database)
	slog.Info("starting fluffy robot", "version", "3.0.0")

	http.HandleFunc("/", a.RootHandler)
	http.HandleFunc("/export", a.ExportHandler)
	http.HandleFunc("/agents", a.AgentsHandler)
	http.HandleFunc("/status", a.HeaderHandler)
	http.HandleFunc("/chart", a.LoadChartHandler)
	http.HandleFunc("/agentlist", a.AgentListHandler)

	// Start the web server and listen on port 8845.
	slog.Info("Starting server on http://localhost:8845")
	slog.Warn("Server Done", "Error", http.ListenAndServe(":8845", nil))
}
