package app

// this file contains all functions related to charting

import (
	"html/template"
	"io"
	"log/slog"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/render"
)

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
	for _, p := range agents {
		hist, err := a.GetAgentRecordsFromCSV(p, 24*time.Hour)
		if err != nil {
			slog.Error("error getting agent records", "error", err)
			continue
		}
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
	for _, p := range agents {
		hist, err := a.GetAgentRecordsFromCSV(p, 4*time.Hour)
		if err != nil {
			slog.Error("error getting agent records", "error", err)
			continue
		}
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
	for _, p := range agents {
		hist, err := a.GetAgentRecordsFromCSV(p, 1*time.Hour)
		if err != nil {
			slog.Error("error getting agent records", "error", err)
			continue
		}
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
	totalPerHour := a.collectPointsPerHour
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

	for _, p := range agents {
		hist, err := a.GetAgentRecordsFromCSV(p, 7*24*time.Hour)
		if err != nil {
			slog.Error("error getting agent records", "error", err)
			continue
		}
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

// func RenderLineChart(w http.ResponseWriter) {
// 	line := charts.NewLine()
// 	line.SetGlobalOptions(
// 		charts.WithTitleOpts(opts.Title{Title: "Simple Line Chart"}),
// 	)
//
// 	line.AddSeries("Category A", []opts.LineData{
// 		{Value: 10}, {Value: 20}, {Value: 30}, {Value: 40},
// 	})
//
// 	// Render only the chart div and script
// 	RenderChartFragment(w, line)
// }

// RenderChartFragment renders a go-echarts chart as a fragment (div + script) to the ResponseWriter.
func (a *App) RenderChartFragment(w io.Writer, chart render.Renderer) error {
	snippet := chart.RenderSnippet()

	data := struct {
		Element template.HTML
		Script  template.HTML
	}{
		Element: template.HTML(snippet.Element),
		Script:  template.HTML(snippet.Script),
	}

	return a.t.ExecuteTemplate(w, "chart.html", data)
}
