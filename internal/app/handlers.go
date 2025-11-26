package app

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-echarts/go-echarts/v2/charts"
)

func (a *App) RootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	if a.agentCache.IsCacheEvicted() {
		slog.Debug("reloading cache")
		a.agentCache.ReloadData(a.Reset)
	}
	a.t.ExecuteTemplate(w, "index.html", nil)
}

func (a *App) LoadChartHandler(w http.ResponseWriter, r *http.Request) {
	if a.agentCache.IsCacheEvicted() {
		slog.Debug("reloading cache")
		a.agentCache.ReloadData(a.Reset)
	}

	// Read query params
	q := r.URL.Query()

	agents := []string{"BURG", "HIVE"}
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
