package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/render"
)

type App struct {
	Current   map[string][]AgentRecord
	LastReset map[string][]AgentRecord
	Reset     string
	Accounts  int
	Agents    int
	Ships     int
}

func (a *App) RootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, indexPartial)
}

func (a *App) AgentsHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, agentsPartial)
}

func (a *App) HeaderHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, headerPartial)
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

func (a *App) Last7dCreditChart(agents []string) *charts.Line {
	line := charts.NewLine()
	weekAgoMs := int(time.Now().Add(-7 * 24 * time.Hour).UnixMilli())

	line.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{Theme: "dark"}),
		charts.WithTitleOpts(opts.Title{
			Title:    "Credits - last 7 days",
			Subtitle: "Adaptive down-sampling",
		}),
		charts.WithYAxisOpts(opts.YAxis{Min: 0}),
		charts.WithXAxisOpts(opts.XAxis{Type: "time", Min: weekAgoMs}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true), Trigger: "axis"}),
	)

	// Adaptive stride to keep point count reasonable
	totalPerHour := collectPointsPerHour
	if totalPerHour == 0 {
		totalPerHour = 12 // assume 5-min cadence
	}
	estimatedTotal := 7 * 24 * totalPerHour
	targetPoints := 200
	stride := 1
	if estimatedTotal > targetPoints {
		stride = estimatedTotal / targetPoints
		if stride < 1 {
			stride = 1
		}
	}

	mapLock.Lock()
	defer mapLock.Unlock()
	for _, p := range agents {
		hist := a.Current[p]
		items := make([]opts.LineData, 0, len(hist)/stride+1)
		for i, r := range hist {
			if i%stride == 0 {
				items = append(items, opts.LineData{Value: []interface{}{r.Timestamp, r.Credits}})
			}
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

func RenderLineChart(w http.ResponseWriter) {
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{Title: "Simple Line Chart"}),
	)

	line.AddSeries("Category A", []opts.LineData{
		{Value: 10}, {Value: 20}, {Value: 30}, {Value: 40},
	})

	// Render only the chart div and script
	RenderChartFragment(w, line)
}

// RenderChartFragment renders a go-echarts chart as a fragment (div + script) to the ResponseWriter.
func RenderChartFragment(w io.Writer, chart render.Renderer) error {
	snippet := chart.RenderSnippet()

	tmpl := template.Must(template.New("chart").Parse(chartPartial))
	data := struct {
		Element template.HTML
		Script  template.HTML
	}{
		Element: template.HTML(snippet.Element),
		Script:  template.HTML(snippet.Script),
	}

	return tmpl.Execute(w, data)
}

// mergeAgents merges the global base agent list with any additional agents
// provided via the URL query values. It supports repeated keys
// (?agents=A&agents=B) and comma-separated lists (?agents=A,B), trims
// whitespace, ignores empties, and de-duplicates while preserving order.
// The base slice is not mutated.
func mergeAgents(base []string, q url.Values) []string {
	// Copy base to avoid mutating the global slice.
	outBase := make([]string, len(base))
	copy(outBase, base)

	// Collect extras from query params.
	var extras []string
	if vals, ok := q["agents"]; ok {
		for _, v := range vals {
			for _, part := range strings.Split(v, ",") {
				s := strings.TrimSpace(part)
				if s != "" {
					extras = append(extras, s)
				}
			}
		}
	}

	if len(extras) == 0 {
		return outBase
	}

	// De-duplicate while preserving order: globals first, then extras.
	seen := make(map[string]struct{}, len(outBase)+len(extras))
	merged := make([]string, 0, len(outBase)+len(extras))
	for _, a := range outBase {
		if _, ok := seen[a]; !ok {
			seen[a] = struct{}{}
			merged = append(merged, a)
		}
	}
	for _, e := range extras {
		if _, ok := seen[e]; !ok {
			seen[e] = struct{}{}
			merged = append(merged, e)
		}
	}
	return merged
}

func (a *App) LoadChartHandler(w http.ResponseWriter, r *http.Request) {
	// Read query params
	q := r.URL.Query()

	// Build effective list of agents from globals plus any provided via query.
	effectiveAgents := mergeAgents(agents, q)

	var line *charts.Line

	// Read period from query and select chart.
	period := q.Get("period")
	switch period {
	case "24h":
		line = a.Last24CreditChart(effectiveAgents)
	case "4h":
		line = a.Last4CreditChart(effectiveAgents)
	case "7d":
		line = a.Last7dCreditChart(effectiveAgents)
	default:
		line = a.Last1CreditChart(effectiveAgents)

	}

	w.Header().Set("Content-Type", "text/html")
	RenderChartFragment(w, line)
}
