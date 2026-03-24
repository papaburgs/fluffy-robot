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

func (a *App) Last24CreditChart(agents []string, isMobile bool) *charts.Line {
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
		if a.agentCache.IsCacheEvicted() {
			a.agentCache.ReloadData(a.Reset)
		}
		hist, err := a.agentCache.GetAgentRecords(p)
		if err != nil {
			slog.Error("error getting agent records", "error", err)
			continue
		}
		items := make([]opts.LineData, 0)

		// Mobile data reduction
		stride := 1
		if isMobile && len(hist) > 100 {
			stride = len(hist) / 100
			if stride < 1 {
				stride = 1
			}
		}

		for i, r := range hist {
			if i%stride == 0 {
				items = append(items, opts.LineData{Value: []interface{}{r.Timestamp, r.Credits}})
			}
		}
		line.AddSeries(p, items)
	}
	return line
}

func (a *App) Last4CreditChart(agents []string, isMobile bool) *charts.Line {
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
		if a.agentCache.IsCacheEvicted() {
			a.agentCache.ReloadData(a.Reset)
		}
		hist, err := a.agentCache.GetAgentRecords(p)
		if err != nil {
			slog.Error("error getting agent records", "error", err)
			continue
		}
		items := make([]opts.LineData, 0)

		// Mobile data reduction
		stride := 1
		if isMobile && len(hist) > 80 {
			stride = len(hist) / 80
			if stride < 1 {
				stride = 1
			}
		}

		for i, r := range hist {
			if i%stride == 0 {
				items = append(items, opts.LineData{Value: []interface{}{r.Timestamp, r.Credits}})
			}
		}
		line.AddSeries(p, items)
	}

	return line
}

func (a *App) Last1CreditChart(agents []string, isMobile bool) *charts.Line {
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
		if a.agentCache.IsCacheEvicted() {
			a.agentCache.ReloadData(a.Reset)
		}
		hist, err := a.agentCache.GetAgentRecords(p)
		if err != nil {
			slog.Error("error getting agent records", "error", err)
			continue
		}
		items := make([]opts.LineData, 0)

		// Mobile data reduction
		stride := 1
		if isMobile && len(hist) > 60 {
			stride = len(hist) / 60
			if stride < 1 {
				stride = 1
			}
		}

		for i, r := range hist {
			if i%stride == 0 {
				items = append(items, opts.LineData{Value: []interface{}{r.Timestamp, r.Credits}})
			}
		}
		line.AddSeries(p, items)
	}

	return line
}

func (a *App) Last7dCreditChart(agents []string, isMobile bool) *charts.Line {
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

	totalPerHour := a.collectPointsPerHour
	if totalPerHour == 0 {
		totalPerHour = 12
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
		if a.agentCache.IsCacheEvicted() {
			a.agentCache.ReloadData(a.Reset)
		}
		hist, err := a.agentCache.GetAgentRecords(p)
		if err != nil {
			slog.Error("error getting agent records", "error", err)
			continue
		}

		// Additional mobile reduction
		if isMobile && len(hist) > 300 {
			mobileStride := len(hist) / 300
			if mobileStride > stride {
				stride = mobileStride
			}
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
