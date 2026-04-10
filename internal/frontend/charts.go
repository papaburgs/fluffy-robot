package frontend

import (
	"fmt"
	"html/template"
	"io"
	"sort"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	ds "github.com/papaburgs/fluffy-robot/internal/datastore"
)

const targetDataPoints int = 50

func agentsMap(agents []ds.Agent) map[string]ds.Agent {
	res := make(map[string]ds.Agent, len(agents))
	for _, a := range agents {
		res[a.Symbol] = a
	}
	return res
}

func latestShips(history []ds.AgentStatus) map[string]int64 {
	latest := make(map[string]int64)
	latestTs := make(map[string]int64)
	for _, r := range history {
		if r.Timestamp > latestTs[r.Symbol] {
			latestTs[r.Symbol] = r.Timestamp
			latest[r.Symbol] = r.Ships
		}
	}
	return latest
}

func agentRecordsCredits(history []ds.AgentStatus, agent string, dur time.Duration) []ds.DataPoint {
	var res []ds.DataPoint
	cutoff := time.Now().Add(-1 * dur).Unix()
	for _, r := range history {
		if r.Symbol == agent && r.Timestamp >= cutoff {
			res = append(res, ds.DataPoint{Timestamp: r.Timestamp, Value: r.Credits})
		}
	}
	return res
}

func agentRecordsShips(history []ds.AgentStatus, agent string, dur time.Duration) []ds.DataPoint {
	var res []ds.DataPoint
	cutoff := time.Now().Add(-1 * dur).Unix()
	for _, r := range history {
		if r.Symbol == agent && r.Timestamp >= cutoff {
			res = append(res, ds.DataPoint{Timestamp: r.Timestamp, Value: r.Ships})
		}
	}
	return res
}

func constructionRecords(agentsMap map[string]ds.Agent, jgs map[string]ds.JGInfo, constructions []ds.JGConstruction, agentNames []string) map[string][]ds.ConstructionRecord {
	res := make(map[string][]ds.ConstructionRecord)
	for _, a := range agentNames {
		thisAgent, ok := agentsMap[a]
		if !ok {
			continue
		}
		thisJG, ok := jgs[thisAgent.System]
		if !ok {
			continue
		}
		for _, rec := range constructions {
			if rec.Jumpgate == thisJG.Jumpgate {
				res[a] = append(res[a], ds.ConstructionRecord{
					Timestamp: rec.Timestamp,
					Fabmat:    rec.Fabmat,
					Advcct:    rec.Advcct,
				})
			}
		}
	}
	return res
}

func latestConstructionRecords(agentsMap map[string]ds.Agent, jgs map[string]ds.JGInfo, constructions []ds.JGConstruction, agentNames []string) []ds.ConstructionOverview {
	res := []ds.ConstructionOverview{}
	for _, a := range agentNames {
		thisAgent, ok := agentsMap[a]
		if !ok {
			continue
		}
		thisJG, ok := jgs[thisAgent.System]
		if !ok {
			continue
		}
		jgLatest := ds.JGConstruction{}
		for _, rec := range constructions {
			if rec.Jumpgate != thisJG.Jumpgate {
				continue
			}
			if rec.Timestamp > jgLatest.Timestamp {
				jgLatest = rec
			}
		}
		if jgLatest.Timestamp == 0 {
			jgLatest.Timestamp = time.Now().Unix()
		}
		res = append(res, ds.ConstructionOverview{
			Agent:     a,
			Jumpgate:  thisJG.Jumpgate,
			Fabmat:    jgLatest.Fabmat,
			Advcct:    jgLatest.Advcct,
			Timestamp: time.Unix(jgLatest.Timestamp, 0).UTC(),
		})
	}
	return res
}

func jumpgatesMap(jgs []ds.JGInfo) map[string]ds.JGInfo {
	res := make(map[string]ds.JGInfo, len(jgs))
	for _, j := range jgs {
		res[j.System] = j
	}
	return res
}

func constructionString(co ds.ConstructionOverview, jg ds.JGInfo) (string, bool) {
	if jg.Status == ds.Complete {
		return "Complete", true
	}
	if co.Fabmat > 0 || co.Advcct > 0 {
		return fmt.Sprintf("%d/1600 FB, %d/400 AC", co.Fabmat, co.Advcct), true
	}
	return "\u2014", false
}

func CreditChart(agents []string, history []ds.AgentStatus, dur time.Duration, title string) *charts.Line {
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
			Name:     "Credits",
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

	for _, p := range agents {
		creditHist := agentRecordsCredits(history, p, dur)
		creditItems := make([]opts.LineData, 0, targetDataPoints*2)
		for i, r := range creditHist {
			if i%stride == 0 {
				creditItems = append(creditItems, opts.LineData{Value: []interface{}{r.Timestamp * 1000, r.Value}})
			}
		}
		line.AddSeries(p, creditItems)

		creditHist = nil
	}

	return line
}

func ShipChart(agents []string, history []ds.AgentStatus, dur time.Duration, title string) *charts.Line {
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
			Name:     "Ships",
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

	for _, p := range agents {
		shipHist := agentRecordsShips(history, p, dur)
		shipItems := make([]opts.LineData, 0, targetDataPoints*2)
		for i, r := range shipHist {
			if i%stride == 0 {
				shipItems = append(shipItems, opts.LineData{Value: []interface{}{r.Timestamp * 1000, r.Value}})
			}
		}
		line.AddSeries(p, shipItems)

		shipHist = nil
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

type ConstructionParallelRow struct {
	Agent    string
	Jumpgate string
	Fabmat   int
	Advcct   int
}

func ConstructionParallelChart(rows []ConstructionParallelRow) *charts.Parallel {
	agentSet := make(map[string]struct{}, len(rows))
	jgSet := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		agentSet[r.Agent] = struct{}{}
		jgSet[r.Jumpgate] = struct{}{}
	}
	agents := make([]string, 0, len(agentSet))
	for a := range agentSet {
		agents = append(agents, a)
	}
	sort.Strings(agents)
	jgs := make([]string, 0, len(jgSet))
	for j := range jgSet {
		jgs = append(jgs, j)
	}
	sort.Strings(jgs)

	parallel := charts.NewParallel()
	parallel.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{
			Theme:  "dark",
			Width:  "100%",
			Height: "150%",
		}),
		charts.WithTitleOpts(opts.Title{
			Title: "Jumpgate Construction by Agent",
		}),
		charts.WithParallelAxisList([]opts.ParallelAxis{
			{Dim: 0, Name: "Agent", Type: "category", Data: agents},
			{Dim: 1, Name: "Jumpgate", Type: "category", Data: jgs},
			{Dim: 2, Name: "Fabmat", Type: "value", Max: 1600},
			{Dim: 3, Name: "Adv CCT", Type: "value", Max: 400},
		}),
		charts.WithTooltipOpts(opts.Tooltip{
			Show:    opts.Bool(true),
			Trigger: "item",
		}),
	)

	data := make([]opts.ParallelData, 0, len(rows))
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Agent < rows[j].Agent
	})
	for _, r := range rows {
		data = append(data, opts.ParallelData{
			Value: []interface{}{r.Agent, r.Jumpgate, r.Fabmat, r.Advcct},
		})
	}
	parallel.AddSeries("Construction", data)

	return parallel
}

type ChartSnippet struct {
	Element template.HTML
	Script  template.HTML
}

type ChartPageData struct {
	CreditChart       ChartSnippet
	ShipChart         ChartSnippet
	ConstructionTable []ds.ConstructionOverview
	ConstructionChart ChartSnippet
}

func RenderChartFragment(w io.Writer, data ChartPageData) error {
	return t.ExecuteTemplate(w, "chart.html", data)
}
