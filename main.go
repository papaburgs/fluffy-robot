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
	"github.com/goforj/godump"
)

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

func (a *App) RootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	// Generate the chart.
	godump.Dump(a)
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

func main() {

	// Create a new text handler that writes to the log file.
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	l := slog.New(h)
	slog.SetDefault(l)

	a := NewApp()
	l.Info("starting fluffy robot", "version", "0.0.2")

	go a.collector("https://api.spacetraders.io/v2")
	// Register the handler function for the root URL path ("/").
	http.HandleFunc("/", a.RootHandler)

	// Start the web server and listen on port 8845.
	fmt.Println("Starting server on http://localhost:8845")
	log.Fatal(http.ListenAndServe(":8845", nil))
}

func (a *App) renderPage(w io.Writer) {
	slog.Debug("Starting render", "agents", agents)
	page := components.NewPage()
	page.AddCharts(
		a.Last1CreditChart(agents),
		a.Last4CreditChart(agents),
		a.Last24CreditChart(agents),
	)
	page.Render(io.MultiWriter(w))
}

var collectPointsPerHour int

func (a *App) Last24CreditChart(agents []string) *charts.Line {
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
	mapLock.Lock()
	defer mapLock.Unlock()
	for _, p := range agents {
		hist := a.Current[p]
		items := make([]opts.LineData, 0)
		for i, r := range hist {
			// I want 4 points per hour
			if i%(collectPointsPerHour/4) == 0 {
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
			// I want 4 points per hour
			if i%(collectPointsPerHour/20) == 0 {
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
			// I want 4 points per hour
			items = append(items, opts.LineData{Value: []interface{}{r.Timestamp, r.Credits}})
		}
		line.AddSeries(p, items)
	}

	return line
}

func (a *App) collector(baseURL string) {
	// do it this way so the render funcs can just look at the points per hour to determine how many points to select
	collectEvery := 1
	checkTimerDuration := time.Duration(collectEvery) * time.Minute
	collectPointsPerHour = 60 / collectEvery

	checkTimer := time.NewTicker(checkTimerDuration)
	for {
		select {
		case <-checkTimer.C:
			slog.Debug("collecting")
			a.collect(baseURL)

		}
	}
}
