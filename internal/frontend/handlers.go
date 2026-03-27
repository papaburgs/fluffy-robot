package main

import (
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
)

func (a *App) RootHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("RootHandler")
	w.Header().Set("Content-Type", "text/html")
	slog.Info("Incoming request", "endpoint", "index")
	a.t.ExecuteTemplate(w, "index.html", a)
}

func (a *App) LoadChartHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("LoadChartHandler")

	q := r.URL.Query()
	storageAgents := q.Get("storageAgents")
	paramAgents := q.Get("paramAgents")

	agents := mergeAgents(storageAgents, paramAgents)

	pageData := ChartPageData{}

	// Read period from query and select chart.
	period := q.Get("period")
	var duration time.Duration
	var creditChart *charts.Line
	slog.Info("Incoming request", "endpoint", "chart", "period", period)
	switch period {
	case "24h":
		duration = 24 * time.Hour
		creditChart = a.Last24CreditChart(agents)
	case "4h":
		duration = 4 * time.Hour
		creditChart = a.Last4CreditChart(agents)
	case "7d":
		duration = 7 * 24 * time.Hour
		creditChart = a.Last7dCreditChart(agents)
	default:
		duration = 1 * time.Hour
		creditChart = a.Last1CreditChart(agents)
	}

	if creditChart != nil {
		snippet := creditChart.RenderSnippet()
		pageData.CreditChart = ChartSnippet{
			Element: template.HTML(snippet.Element),
			Script:  template.HTML(snippet.Script),
		}
	}

	// Get latest construction overview
	overview, err := a.GetLatestConstructionRecords(agents, a.Reset)
	if err == nil && len(overview) > 0 {
		pageData.ConstructionTable = overview
	}

	// Generate construction chart if any data
	recs, err := a.GetConstructionRecordsFromDB(agents, a.Reset, duration)
	if err == nil && len(recs) > 0 {
		constChart := a.JumpgateConstructionChart(recs, duration)
		if constChart != nil {
			snippet := constChart.RenderSnippet()
			pageData.ConstructionChart = ChartSnippet{
				Element: template.HTML(snippet.Element),
				Script:  template.HTML(snippet.Script),
			}
		}
	}

	w.Header().Set("Content-Type", "text/html")
	a.RenderChartFragment(w, pageData)
}

func (a *App) PermissionsHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("PermissionsHandler")
	agents, err := a.GetAllAgentsFromDB(a.Reset)
	if err != nil {
		slog.Error("Error getting agents from DB", "error", err)
		http.Error(w, "Error getting agents", http.StatusInternalServerError)
		return
	}

	a.t.ExecuteTemplate(w, "permissions.html", map[string]interface{}{
		"Agents": agents,
	})
}

func (a *App) HeaderHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("HeaderHandler")
	// Header usually contains status info like Reset date, etc.
	// We might want to pass 'a' or some data to it.
	// For now, let's pass 'a' so it can access Reset.
	a.t.ExecuteTemplate(w, "header.html", a)
}

func (a *App) ExportHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("ExportHandler")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="backup.json"`)

	// Export limited data since we don't have local state
	data := map[string]interface{}{
		"reset":                a.Reset,
		"collectPointsPerHour": a.collectPointsPerHour,
	}

	b, err := json.Marshal(data)
	if err != nil {
		http.Error(w, "failed to marshal export data", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(b)
}

func (a *App) PermissionsGridHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("PermissionsGridHandler")
	// This handler returns a partial (HTML) for HTMX

	type data struct {
		Name      string
		Credits   int64
		IsActive  bool
		IsChecked bool
	}
	d := []data{}
	agents, err := a.GetAllAgentsFromDB(a.Reset)
	if err != nil {
		slog.Error("Error getting agents from DB", "error", err)
		http.Error(w, "Error getting agents", http.StatusInternalServerError)
		return
	}
	q := r.URL.Query()
	searchStr := strings.ToLower(q.Get("agentSearch"))
	hideInactive := q.Get("hideInactive") == "on"
	sortBy := q.Get("sortBy")

	storageAgents := q.Get("storageAgents")
	paramAgents := q.Get("paramAgents")

	storageAgentsMap := make(map[string]bool)
	for _, i := range mergeAgents(storageAgents, paramAgents) {
		storageAgentsMap[i] = true
	}

	for agent, details := range agents {
		// Filter by search string
		if searchStr != "" && !strings.Contains(strings.ToLower(agent), searchStr) {
			continue
		}
		// Filter inactive if requested
		if hideInactive && !details.Active {
			continue
		}

		_, ok := storageAgentsMap[agent]
		d = append(d, data{
			Name:      agent,
			Credits:   details.Credits,
			IsActive:  details.Active,
			IsChecked: ok,
		})
	}

	// Sort agents
	sort.Slice(d, func(i, j int) bool {
		if sortBy == "credits" {
			if d[i].Credits != d[j].Credits {
				return d[i].Credits > d[j].Credits // Descending credits
			}
			return d[i].Name < d[j].Name // Tie-break by name
		}
		return d[i].Name < d[j].Name // Ascending name (default)
	})

	a.t.ExecuteTemplate(w, "permissions-grid.html", d)
}
func (a *App) LeaderboardHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("LeaderboardHandler")
	leaderboardType := r.URL.Query().Get("type")
	if leaderboardType == "" {
		leaderboardType = "credits"
	}
	myAgent := r.URL.Query().Get("myAgent")

	data, err := a.GetLeaderboard(leaderboardType, a.Reset)
	if err != nil {
		slog.Error("failed to get leaderboard", "error", err)
		http.Error(w, "failed to get leaderboard", http.StatusInternalServerError)
		return
	}

	// If template doesn't exist yet, this will fail. We need to create it.
	a.t.ExecuteTemplate(w, "leaderboard.html", map[string]interface{}{
		"Type":    leaderboardType,
		"Data":    data,
		"MyAgent": myAgent,
	})
}

func (a *App) StatsHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("StatsHandler")
	stats, err := a.GetStats(a.Reset)
	if err != nil {
		slog.Error("failed to get stats", "error", err)
		http.Error(w, "failed to get stats", http.StatusInternalServerError)
		return
	}
	a.t.ExecuteTemplate(w, "stats.html", stats)
}

func (a *App) JumpgatesHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("JumpgatesHandler")
	gates, err := a.GetJumpgates(a.Reset)
	if err != nil {
		slog.Error("failed to get jumpgates", "error", err)
		http.Error(w, "failed to get jumpgates", http.StatusInternalServerError)
		return
	}
	a.t.ExecuteTemplate(w, "jumpgates.html", gates)
}
