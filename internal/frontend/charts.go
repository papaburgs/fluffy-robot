package frontend

import (
	"html/template"
	"io"
	"sort"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	ds "github.com/papaburgs/fluffy-robot/internal/datastore"
)

const targetDataPoints int = 150

func CreditChart(agents []string, dur time.Duration, title string) *charts.Line {
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{
			Theme: "dark",
			Width: "100%",
		}),
		charts.WithTitleOpts(opts.Title{
			Title: title,
		}),
		charts.WithYAxisOpts(opts.YAxis{
			Min:      0,
			Position: "right",
		}),
		charts.WithXAxisOpts(opts.XAxis{
			Type: "time",
			Min:  time.Now().Add(-1 * dur).UnixMilli(),
			Max:  time.Now().UnixMilli(),
		}),
		charts.WithTooltipOpts(opts.Tooltip{
			Show:    opts.Bool(true),
			Trigger: "axis",
		}),
	)
	collectionTime := 5
	dpPerHour := 60 / collectionTime
	estimatedTotal := int(dur.Hours()) * dpPerHour
	stride := 1
	if estimatedTotal > targetDataPoints {
		stride = estimatedTotal / targetDataPoints
		if stride < 1 {
			stride = 1
		}
	}

	thisReset := ds.Reset(resets[0])
	for _, p := range agents {
		hist := ds.GetAgentRecordsCredits(thisReset, p, dur)
		items := make([]opts.LineData, 0, targetDataPoints*2)
		sort.Slice(hist, func(i, j int) bool {
			return hist[i].Timestamp < hist[j].Timestamp
		})
		for i, r := range hist {
			if i%stride == 0 {
				items = append(items, opts.LineData{Value: []interface{}{r.Timestamp * 1000, r.Value}})
			}
		}
		line.AddSeries(p, items)
		hist = nil
	}

	return line
}

func JumpgateConstructionChart(data map[string][]ds.ConstructionRecord, duration time.Duration) *charts.Line {
	line := charts.NewLine()
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
			Min:  time.Now().Add(-1 * duration).UnixMilli(),
			Max:  time.Now().UnixMilli(),
		}),
		charts.WithTooltipOpts(opts.Tooltip{
			Show:    opts.Bool(true),
			Trigger: "axis",
		}),
	)
	for jg, recs := range data {
		fabItems := make([]opts.LineData, 0)
		advItems := make([]opts.LineData, 0)
		sort.Slice(recs, func(i, j int) bool {
			return recs[i].Timestamp < recs[j].Timestamp
		})
		for _, r := range recs {
			fabItems = append(fabItems, opts.LineData{Value: []interface{}{r.Timestamp * 1000, r.Fabmat}})
			advItems = append(advItems, opts.LineData{Value: []interface{}{r.Timestamp * 1000, r.Advcct}})
		}
		line.AddSeries(jg+" (Fabmat)", fabItems)
		line.AddSeries(jg+" (Advcct)", advItems)
		fabItems = nil
		advItems = nil
	}
	data = nil
	return line
}

type ChartSnippet struct {
	Element template.HTML
	Script  template.HTML
}

type ChartPageData struct {
	CreditChart       ChartSnippet
	ConstructionTable []ds.ConstructionOverview
	ConstructionChart ChartSnippet
}

func RenderChartFragment(w io.Writer, data ChartPageData) error {
	return t.ExecuteTemplate(w, "chart.html", data)
}
