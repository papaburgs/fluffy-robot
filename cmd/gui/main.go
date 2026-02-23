package main

import (
	"embed"
	"net/http"
	"os"

	"log/slog"

	_ "embed"

	"github.com/papaburgs/fluffy-robot/internal/db"
	"github.com/papaburgs/fluffy-robot/internal/logging"
)

//go:embed static
var staticFiles embed.FS // This variable now holds the entire 'static' directory

func main() {
	logging.InitLogger()

	database, err := db.Connect()
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	////////// Static file handling \\\\\\\\\\
	// define this value to be something in order to use the 'external' css to make it easier to work on
	// leaving it unset will embed the directory instead to make it easier for a server
	if _, ok := os.LookupEnv("FLUFFY_STATIC_DEV"); ok {
		// Dev case
		fs := http.FileServer(http.Dir("./static"))
		http.Handle("/static/", http.StripPrefix("/static/", fs))
	} else {
		// production case
		fsHandler := http.FileServer(http.FS(staticFiles))
		http.Handle("/static/", fsHandler)
	}

	a := NewApp(database)
	slog.Info("starting fluffy robot", "version", "3.0.0")

	http.HandleFunc("/", a.RootHandler)
	http.HandleFunc("/agents", a.AgentsHandler)
	http.HandleFunc("/status", a.HeaderHandler)
	http.HandleFunc("/chart", a.LoadChartHandler)
	http.HandleFunc("/agentlist", a.AgentListHandler)

	http.HandleFunc("/leaderboard", a.LeaderboardHandler)
	http.HandleFunc("/stats", a.StatsHandler)
	http.HandleFunc("/jumpgates", a.JumpgatesHandler)

	slog.Info("Starting server on http://localhost:8845")
	slog.Warn("Server Done", "Error", http.ListenAndServe(":8845", nil))
}
