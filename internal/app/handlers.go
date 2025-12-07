package app

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/go-echarts/go-echarts/v2/charts"
)

func (a *App) RootHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("RootHandler")
	w.Header().Set("Content-Type", "text/html")
	if a.agentCache.IsCacheEvicted() {
		slog.Debug("reloading cache")
		a.agentCache.ReloadData(a.Reset)
	}
	slog.Info("Incoming request", "endpoint", "index")
	q := r.URL.Query()
	slog.Debug("query output")
	for k, v := range q {
		slog.Debug("root handler", "key", k, "val", v)
	}
	paramAgents := mergeAgents(q.Get("paramAgents"), q.Get("agent"), q.Get("agents"))
	d := struct {
		AgentVals string
	}{
		AgentVals: strings.Join(paramAgents, ","),
	}
	a.t.ExecuteTemplate(w, "index.html", d)
}

func (a *App) LoadChartHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("LoadChartHandler")
	if a.agentCache.IsCacheEvicted() {
		slog.Debug("reloading cache")
		a.agentCache.ReloadData(a.Reset)
	}

	q := r.URL.Query()
	slog.Debug("query output")
	for k, v := range q {
		slog.Debug("chart handler", "key", k, "val", v)
	}
	storageAgents := q.Get("storageAgents")
	paramAgents := q.Get("paramAgents")

	agents := mergeAgents(storageAgents, paramAgents)
	var line *charts.Line

	// Read period from query and select chart.
	period := q.Get("period")
	slog.Info("Incoming request", "endpoint", "chart", "period", period)
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
	slog.Debug("AgentsHandler")
	a.t.ExecuteTemplate(w, "agentlistpage.html", nil)
}

func (a *App) HeaderHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("HeaderHandler")
	a.t.ExecuteTemplate(w, "header.html", nil)
}

func (a *App) ExportHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("ExportHandler")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="backup.json"`)
	data, err := json.Marshal(a)
	if err != nil {
		http.Error(w, "failed to marshal export data", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(data)
}

func (a *App) AgentListHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("AgentListHandler")
	w.Header().Set("Content-Type", "application/json")
	if a.agentCache.IsCacheEvicted() {
		slog.Debug("reloading cache")
		a.agentCache.ReloadData(a.Reset)
	}
	type data struct {
		Name      string
		IsActive  bool
		IsChecked bool
	}
	d := []data{}
	agents, err := a.agentCache.GetAllAgents()
	if err != nil {
		slog.Error("Error getting agents", "error", err)
		return
	}
	searchStr := strings.ToLower(r.URL.Query().Get("agentSearch"))
	slog.Debug("search", "agentSearch", searchStr)
	storageAgentsMap := make(map[string]bool)
	storageAgentsParam := r.URL.Query().Get("storageAgents")
	slog.Debug("stored agents", "agents", storageAgentsParam)
	for _, i := range strings.Split(storageAgentsParam, ",") {
		storageAgentsMap[i] = true
	}

	for agent, active := range agents {
		_, ok := storageAgentsMap[agent]
		if searchStr == "" || strings.Contains(strings.ToLower(agent), searchStr) {
			slog.Debug("adding", "name", agent, "checked", ok)
			d = append(d, data{
				Name:      agent,
				IsActive:  active,
				IsChecked: ok,
			})
		}
	}

	// sort agents based on the Name field
	sort.Slice(d, func(i, j int) bool {
		return d[i].Name < d[j].Name
	})

	a.t.ExecuteTemplate(w, "agentlist.html", d)
}
