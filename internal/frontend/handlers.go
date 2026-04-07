package frontend

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	ds "github.com/papaburgs/fluffy-robot/internal/datastore"
)

func RootHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("RootHandler")
	w.Header().Set("Content-Type", "text/html")
	slog.Info("Incoming request", "endpoint", "index")
	if err := t.ExecuteTemplate(w, "index.html", nil); err != nil {
		slog.Error("template error", "error", err)
	}
}

func LoadChartHandler(w http.ResponseWriter, r *http.Request) {
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
	title := ""
	switch period {
	case "24h":
		duration = 24 * time.Hour
		title = "Last 24 hours"
	case "4h":
		duration = 4 * time.Hour
		title = "Last 4 hours"
	case "7d":
		duration = 7 * 24 * time.Hour
		title = "Last 7 days"
	default:
		duration = 1 * time.Hour
		title = "Last Hour"
	}
	creditChart = CreditChart(agents, duration, title)

	if creditChart != nil {
		snippet := creditChart.RenderSnippet()
		pageData.CreditChart = ChartSnippet{
			Element: template.HTML(snippet.Element),
			Script:  template.HTML(snippet.Script),
		}
	}
	// Get latest construction overview
	// returns array of Construction record
	overview := ds.GetLatestConstructionRecords(ds.Reset(resets[0]), agents)
	pageData.ConstructionTable = overview

	// Generate construction chart if any data
	// returns map[string][]types.ConstructionRecord,
	recs := ds.GetConstructionRecords(ds.Reset(resets[0]), agents, duration)
	constChart := JumpgateConstructionChart(recs, duration)
	if constChart != nil {
		snippet := constChart.RenderSnippet()
		pageData.ConstructionChart = ChartSnippet{
			Element: template.HTML(snippet.Element),
			Script:  template.HTML(snippet.Script),
		}
	}

	w.Header().Set("Content-Type", "text/html")
	if err := RenderChartFragment(w, pageData); err != nil {
		slog.Error("template error", "error", err)
	}
}

func PermissionsHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("PermissionsHandler")
	// reset := ds.LatestReset()
	agents := ds.GetAgents(ds.Reset(resets[0]))

	if err := t.ExecuteTemplate(w, "permissions.html", map[string]interface{}{
		"Agents": agents,
	}); err != nil {
		slog.Error("template error", "error", err)
	}
}

func HeaderHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("HeaderHandler")
	if err := t.ExecuteTemplate(w, "header.html", map[string]interface{}{
		"Reset": ds.LatestReset(),
	}); err != nil {
		slog.Error("template error", "error", err)
	}
}

func ExportHandler(w http.ResponseWriter, r *http.Request) {
	plog.Info("Export Handler called")

	// 1. Set the filename with a timestamp
	filename := "data_export.tar.gz"
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	// 2. Initialize writers
	gw := gzip.NewWriter(w)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	srcDir := ds.DataPath()

	// 3. Walk the directory
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip the root directory itself
		if path == srcDir {
			return nil
		}

		// Create a tar header
		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}

		// Ensure the path inside the tar matches your structure (2026-03-29/file.gob.zst)
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		header.Name = relPath

		// Write header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// If it's a directory, we're done with this iteration
		if info.IsDir() {
			return nil
		}

		// 4. Stream the file content
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tw, file)
		return err
	})

	if err != nil {
		http.Error(w, "Failed to package data", http.StatusInternalServerError)
	}
}

func PermissionsGridHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("PermissionsGridHandler")

	type data struct {
		Name      string
		Credits   int64
		IsActive  bool
		IsChecked bool
	}
	d := []data{}
	agents := ds.GetAgents(ds.LatestReset())
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
		if hideInactive && details.Credits == 175000 {
			continue
		}

		_, ok := storageAgentsMap[agent]
		d = append(d, data{
			Name:      agent,
			Credits:   details.Credits,
			IsActive:  details.Credits != 175000,
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

	if err := t.ExecuteTemplate(w, "permissions-grid.html", d); err != nil {
		slog.Error("template error", "error", err)
	}
}

func LeaderboardHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("LeaderboardHandler")
	leaderboardType := r.URL.Query().Get("type")
	if leaderboardType == "" {
		leaderboardType = "credits"
	}
	slog.Debug("Have leaderboard type", "lbt", leaderboardType)
	myAgent := r.URL.Query().Get("myAgent")

	creditLB, chartLB := ds.GetLeaderboard(ds.LatestReset())

	data := creditLB
	if leaderboardType == "charts" {
		data = chartLB
	}

	slog.Debug("have data", "data", data, "lbt", leaderboardType, "agent", myAgent)

	err := t.ExecuteTemplate(w, "leaderboard.html", map[string]interface{}{
		"Type":    leaderboardType,
		"Data":    data,
		"MyAgent": myAgent,
	})
	if err != nil {
		slog.Error("template error", "error", err)
	}
}

func StatsHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("StatsHandler")
	stats := ds.GetStats(ds.LatestReset())
	if err := t.ExecuteTemplate(w, "stats.html", stats); err != nil {
		slog.Error("template error", "error", err)
	}
}

func JumpgatesHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("JumpgatesHandler")
	gates := ds.GetJumpgates(ds.LatestReset(), 0)
	if err := t.ExecuteTemplate(w, "jumpgates.html", gates); err != nil {
		slog.Error("template error", "error", err)
	}
}
