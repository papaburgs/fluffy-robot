package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"log/slog"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"
)

var agents = []string{
	"BURG",
	"HIVE",
}

type AgentRecord struct {
	TimeStamp time.Time
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

func (a *App) RootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	// Generate the chart.
	renderPage(w)
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

func main() {

	// Create a new text handler that writes to the log file.
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	l := slog.New(h)
	slog.SetDefault(l)

	a := NewApp()
	l.Debug("starting fluffy robot", "version", "0.0.1")

	go
	// Register the handler function for the root URL path ("/").
	http.HandleFunc("/", a.RootHandler)

	// Start the web server and listen on port 8845.
	fmt.Println("Starting server on http://localhost:8845")
	log.Fatal(http.ListenAndServe(":8845", nil))
}

func renderPage(w io.Writer) {
	slog.Info("Starting this", "agents", agents)
	page := components.NewPage()
	page.AddCharts(
		Last4CreditChart(agents),
		Last24CreditChart(agents),
	)
	page.Render(io.MultiWriter(w))
}

func Last24CreditChart(agents []string) *charts.Line {
	line := charts.NewLine()
	tfha := int(time.Now().Add(-24 * 60 * time.Minute).UnixMilli())
	line.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    "Credits - last 4 hours",
			Subtitle: "Data point every 5 minutes",
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
	return line
}

func Last4CreditChart(agents []string) *charts.Line {
	line := charts.NewLine()
	tfha := int(time.Now().Add(-4 * 60 * time.Minute).UnixMilli())
	line.SetGlobalOptions(
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

	return line
}

func (a *App) collector(baseURL string) {
	checkTimerDuration := 5 * time.Minute
	checkTimer := time.NewTicker(checkTimerDuration)
	for {
		select {
		case <-checkTimer.C:
			a.collect(baseURL)
		}
	}
}
