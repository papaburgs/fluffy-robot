package main

import (
	"html/template"
	"io"
	"log/slog"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/papaburgs/fluffy-robot/internal/types"
)

func (a *App) Last24CreditChart(agents []string) *charts.Line {
	line := charts.NewLine()
	tfha := int(time.Now().Add(-24 * 60 * time.Minute).UnixMilli())
	line.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{
			Theme: "dark",
			Width: "100%",
		}),
		charts.WithTitleOpts(opts.Title{
			Title:    "Credits - last 24 hours",
			Subtitle: "Data point every 15 minutes",
		}),
		charts.WithYAxisOpts(opts.YAxis{
			Min:      0,
			Name:     "Credits",
			Position: "right",
		}),

		charts.WithXAxisOpts(opts.XAxis{
			Type: "time",
			Min:  tfha,
		}),
		charts.WithTooltipOpts(opts.Tooltip{
			Show:    opts.Bool(true),
			Trigger: "axis",
		}),
	)
	for _, p := range agents {
		hist, err := a.GetAgentRecordsFromDB(p, a.Reset, 24*time.Hour)
		if err != nil {
			slog.Error("error getting agent records from DB", "error", err)
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
			Width: "100%",
		}),
		charts.WithTitleOpts(opts.Title{
			Title:    "Credits - last 4 hours",
			Subtitle: "20 Data Points per hour",
		}),
		charts.WithYAxisOpts(opts.YAxis{
			Min:      0,
			Position: "right",
		}),
		charts.WithXAxisOpts(opts.XAxis{
			Type: "time",
			Min:  tfha,
		}),
		charts.WithTooltipOpts(opts.Tooltip{
			Show:    opts.Bool(true),
			Trigger: "axis",
		}),
	)
	for _, p := range agents {
		hist, err := a.GetAgentRecordsFromDB(p, a.Reset, 4*time.Hour)
		if err != nil {
			slog.Error("error getting agent records from DB", "error", err)
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
			Width: "100%",
		}),
		charts.WithTitleOpts(opts.Title{
			Title:    "Credits - last hour",
			Subtitle: "All Data points",
		}),
		charts.WithYAxisOpts(opts.YAxis{
			Min:      0,
			Position: "right",
		}),
		charts.WithXAxisOpts(opts.XAxis{
			Type: "time",
			Min:  tfha,
		}),
		charts.WithTooltipOpts(opts.Tooltip{
			Show:    opts.Bool(true),
			Trigger: "axis",
		}),
	)
	for _, p := range agents {
		hist, err := a.GetAgentRecordsFromDB(p, a.Reset, 1*time.Hour)
		if err != nil {
			slog.Error("error getting agent records from DB", "error", err)
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
		charts.WithInitializationOpts(opts.Initialization{
			Theme: "dark",
			Width: "100%",
		}),
		charts.WithTitleOpts(opts.Title{
			Title:    "Credits - last 7 days",
			Subtitle: "Adaptive down-sampling",
		}),
		charts.WithYAxisOpts(opts.YAxis{Min: 0, Position: "right"}),
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
		hist, err := a.GetAgentRecordsFromDB(p, a.Reset, 7*24*time.Hour)
		if err != nil {
			slog.Error("error getting agent records from DB", "error", err)
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

func (a *App) JumpgateConstructionChart(data map[string][]types.ConstructionRecord, duration time.Duration) *charts.Line {
	line := charts.NewLine()
	tfha := int(time.Now().Add(-duration).UnixMilli())
	line.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{
			Theme: "dark",
			Width: "100%",
		}),
		charts.WithTitleOpts(opts.Title{
			Title:    "Jumpgate Construction Progress",
			Subtitle: "Materials vs Time",
		}),
		charts.WithYAxisOpts(opts.YAxis{
			Min:      0,
			Position: "right",
		}),
		charts.WithXAxisOpts(opts.XAxis{
			Type: "time",
			Min:  tfha,
		}),
		charts.WithTooltipOpts(opts.Tooltip{
			Show:    opts.Bool(true),
			Trigger: "axis",
		}),
	)
	for jg, recs := range data {
		fabItems := make([]opts.LineData, 0)
		advItems := make([]opts.LineData, 0)
		for _, r := range recs {
			fabItems = append(fabItems, opts.LineData{Value: []interface{}{r.Timestamp, r.Fabmat}})
			advItems = append(advItems, opts.LineData{Value: []interface{}{r.Timestamp, r.Advcct}})
		}
		line.AddSeries(jg+" (Fabmat)", fabItems)
		line.AddSeries(jg+" (Advcct)", advItems)
	}
	return line
}

type ChartSnippet struct {
	Element template.HTML
	Script  template.HTML
}

type ChartPageData struct {
	CreditChart       ChartSnippet
	ConstructionTable []types.ConstructionOverview
	ConstructionChart ChartSnippet
}

// RenderChartFragment renders the chart page content to the ResponseWriter.
func (a *App) RenderChartFragment(w io.Writer, data ChartPageData) error {
	return a.t.ExecuteTemplate(w, "chart.html", data)
}
