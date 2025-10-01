package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "embed"
	"log/slog"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"
)

var collectPointsPerHour int

//go:embed main.css
var mainCss string

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

type App struct {
	Current   map[string][]AgentRecord
	LastReset map[string][]AgentRecord
	Reset     string
	Accounts  int
	Agents    int
	Ships     int
}

var mapLock sync.Mutex
var backupLocation string

func (a *App) RootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	// Generate the chart.
	a.renderPage(w)
}

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

	// Start the web server and listen on port 8845.
	fmt.Println("Starting server on http://localhost:8845")
	log.Fatal(http.ListenAndServe(":8845", nil))
}

func (a *App) renderPage(w io.Writer) {
	page := components.NewPage()
	page.SetPageTitle("Fluffy Robot")
	_, err := w.Write([]byte(`<style>` + mainCss + `</style>`))
	if err != nil {
		slog.Error("Error writing embedded main.css", "error", err)
	}
	page.AddCharts(
		a.Last1CreditChart(agents),
		a.Last4CreditChart(agents),
		a.Last24CreditChart(agents),
	)
	err = page.Render(io.MultiWriter(w))
	if err != nil {
		slog.Error("Error rendering page", "error", err)
	}
}

func (a *App) Last24CreditChart(agents []string) *charts.Line {
	line := charts.NewLine()
	tfha := int(time.Now().Add(-24 * 60 * time.Minute).UnixMilli())
	line.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{
			Theme: "dark",
		}),
		charts.WithTitleOpts(opts.Title{
			Title:    "Credits - last 24 hours",
			Subtitle: "Data point every 15 minutes",
		}),
		charts.WithYAxisOpts(opts.YAxis{
			Min: 0,
			// Max: 200,
		}),
		charts.WithXAxisOpts(opts.XAxis{
			Type: "time",
			Min:  tfha,
		}),
		charts.WithTooltipOpts(opts.Tooltip{ // Potential to string format tooltip here
			Show:    opts.Bool(true),
			Trigger: "axis",
		}),
	)
	mapLock.Lock()
	defer mapLock.Unlock()
	for _, p := range agents {
		hist := a.Current[p]
		items := make([]opts.LineData, 0)
		for i, r := range hist {
			if i%10 == 0 {
				items = append(items, opts.LineData{Value: []interface{}{r.Timestamp, r.Credits}})
			}
		}
		line.AddSeries(p, items)
	}
	return line
}

func (a *App) Last4CreditChart(agents []string) *charts.Line {
	line := charts.NewLine()
	tfha := int(time.Now().Add(-4 * 60 * time.Minute).UnixMilli())
	line.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{
			Theme: "dark",
		}),
		charts.WithTitleOpts(opts.Title{
			Title:    "Credits - last 4 hours",
			Subtitle: "20 Data Points per hour",
		}),
		charts.WithYAxisOpts(opts.YAxis{
			Min: 0,
			// Max: 200,
		}),
		charts.WithXAxisOpts(opts.XAxis{
			Type: "time",
			Min:  tfha,
		}),
		charts.WithTooltipOpts(opts.Tooltip{ // Potential to string format tooltip here
			Show:    opts.Bool(true),
			Trigger: "axis",
		}),
	)
	mapLock.Lock()
	defer mapLock.Unlock()
	for _, p := range agents {
		hist := a.Current[p]
		items := make([]opts.LineData, 0)
		for i, r := range hist {
			if i%2 == 0 {
				items = append(items, opts.LineData{Value: []interface{}{r.Timestamp, r.Credits}})
			}
		}
		line.AddSeries(p, items)
	}

	return line
}

func (a *App) Last1CreditChart(agents []string) *charts.Line {
	line := charts.NewLine()
	tfha := int(time.Now().Add(-1 * 60 * time.Minute).UnixMilli())
	line.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{
			Theme: "dark",
		}),
		charts.WithTitleOpts(opts.Title{
			Title:    "Credits - last hour",
			Subtitle: "All Data points",
		}),
		charts.WithYAxisOpts(opts.YAxis{
			Min: 0,
			// Max: 200,
		}),
		charts.WithXAxisOpts(opts.XAxis{
			Type: "time",
			Min:  tfha,
		}),
		charts.WithTooltipOpts(opts.Tooltip{ // Potential to string format tooltip here
			Show:    opts.Bool(true),
			Trigger: "axis",
		}),
	)
	mapLock.Lock()
	defer mapLock.Unlock()
	for _, p := range agents {
		hist := a.Current[p]
		items := make([]opts.LineData, 0)
		for _, r := range hist {
			items = append(items, opts.LineData{Value: []interface{}{r.Timestamp, r.Credits}})
		}
		line.AddSeries(p, items)
	}

	return line
}

func (a *App) ExportHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="backup.json"`)
	mapLock.Lock()
	defer mapLock.Unlock()
	data, err := json.Marshal(a)
	if err != nil {
		http.Error(w, "failed to marshal export data", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(data)
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
