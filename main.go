package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "embed"
	"log/slog"
)

var collectPointsPerHour int

var agents = []string{
	"BURG",
	"HIVE",
}

type AgentRecord struct {
	Timestamp time.Time
	ShipCount int
	Credits   int
}

// type History struct {
// 	Records []AgentRecord
// 	Agent   string
// }

var mapLock sync.Mutex
var backupLocation string

func NewApp() *App {
	m := make(map[string][]AgentRecord)
	n := make(map[string][]AgentRecord)
	a := App{
		Current:   m,
		LastReset: n,
		Reset:     "00000",
		Agents:    0,
		Ships:     0,
	}
	return &a
}

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
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	if logl, ok := os.LookupEnv("SPACETRADER_LEADERBOARD_LOG_LEVEL"); !ok {
		h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	} else {
		switch strings.ToLower(logl) {
		case "debug", "dbg":
			h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
		case "warn", "wrn":
			h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn})
		case "error", "err":
			h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})
		default:
			h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
		}
	}

	// Create a new text handler that writes to the log file.
	l := slog.New(h)
	slog.SetDefault(l)

	a := NewApp()
	l.Info("starting fluffy robot", "version", "0.0.5")

	if loc, ok := os.LookupEnv("SPACETRADER_LEADERBOARD_BACKUP_PATH"); ok {
		backupLocation = loc
	} else {
		slog.Debug("no backup location, will use local dir")
		backupLocation = "."
	}

	a.Restore()
	if collectionsEnabled() {
		go a.collector("https://api.spacetraders.io/v2")
	}
	// Register the handler function for the root URL path ("/").
	http.HandleFunc("/", a.RootHandler)
	http.HandleFunc("/export", a.ExportHandler)
	http.HandleFunc("/agents", a.AgentsHandler)
	http.HandleFunc("/status", a.HeaderHandler)
	http.HandleFunc("/chart", a.LoadChartHandler)

	// Start the web server and listen on port 8845.
	fmt.Println("Starting server on http://localhost:8845")
	log.Fatal(http.ListenAndServe(":8845", nil))
}


func (a *App) collector(baseURL string) {
	// do it this way so the render funcs can just look at the points per hour to determine how many points to select
	collectEvery := 5
	checkTimerDuration := time.Duration(collectEvery) * time.Minute
	collectPointsPerHour = 60 / collectEvery

	checkTimer := time.NewTicker(checkTimerDuration)
	for {
		select {
		case <-checkTimer.C:
			a.collect(baseURL)
		}
	}
}
