package app

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-echarts/go-echarts/v2/charts"
)

func (a *App) RootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	if a.agentCache.IsCacheEvicted() {
		slog.Debug("reloading cache")
		a.agentCache.ReloadData(a.Reset)
	}
	q := r.URL.Query()
	slog.Debug("query output")
	for k, v := range q {
		slog.Debug("chart handler", "key", k, "val", v)
	}
	agents := mergeAgents([]string{}, q)
	slog.Debug("agents after  merge", "a", agents)
	if len(agents) == 0 {
		agents = []string{"BURG", "HIVE"}
	}

	d := struct {
		AgentVals string
	}{
		AgentVals: strings.Join(agents, ","),
	}
	a.t.ExecuteTemplate(w, "index.html", d)
}

func (a *App) LoadChartHandler(w http.ResponseWriter, r *http.Request) {
	if a.agentCache.IsCacheEvicted() {
		slog.Debug("reloading cache")
		a.agentCache.ReloadData(a.Reset)
	}

	q := r.URL.Query()
	slog.Debug("query output")
	for k, v := range q {
		slog.Debug("chart handler", "key", k, "val", v)
	}
	agents := mergeAgents([]string{}, q)
	slog.Debug("agents after  merge", "a", agents)
	if len(agents) == 0 {
		agents = []string{"BURG", "HIVE"}
	}
	slog.Debug("query output")
	for k, v := range q {
		slog.Debug("chart handler", "key", k, "val", v)
	}

	var line *charts.Line

	// Read period from query and select chart.
	period := q.Get("period")
	switch period {
	case "24h":
		line = a.Last24CreditChart(agents)
	case "4h":
		line = a.Last4CreditChart(agents)
	case "7d":
		line = a.Last7dCreditChart(agents)
	default:
		line = a.Last1CreditChart(agents)

	}

	w.Header().Set("Content-Type", "text/html")
	a.RenderChartFragment(w, line)
}

func (a *App) AgentsHandler(w http.ResponseWriter, r *http.Request) {
	a.t.ExecuteTemplate(w, "agents.html", nil)
}

func (a *App) HeaderHandler(w http.ResponseWriter, r *http.Request) {
	a.t.ExecuteTemplate(w, "header.html", nil)
}

func (a *App) ExportHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="backup.json"`)
	data, err := json.Marshal(a)
	if err != nil {
		http.Error(w, "failed to marshal export data", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(data)
}
