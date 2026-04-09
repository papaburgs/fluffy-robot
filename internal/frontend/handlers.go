package frontend

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	ds "github.com/papaburgs/fluffy-robot/internal/datastore"
	"github.com/papaburgs/fluffy-robot/internal/logging"
	"github.com/papaburgs/fluffy-robot/internal/metrics"
)

func RootHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	w.Header().Set("Content-Type", "text/html")
	logging.Info("Incoming request", "endpoint", "index")
	if err := t.ExecuteTemplate(w, "index.html", nil); err != nil {
		logging.Error("template error", err)
	}
	metrics.RecordDuration("root", start)
}

func LoadChartHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	q := r.URL.Query()
	storageAgents := q.Get("storageAgents")
	paramAgents := q.Get("paramAgents")

	agents := mergeAgents(storageAgents, paramAgents)

	pageData := ChartPageData{}

	period := q.Get("period")
	var duration time.Duration
	var creditChart *charts.Line
	logging.Info("Incoming request", "endpoint", "chart", "period", period)
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

	thisReset := ds.Reset(resets[0])
	overview := ds.GetLatestConstructionRecords(thisReset, agents)
	pageData.ConstructionTable = overview
	overview = nil

	recs := ds.GetConstructionRecords(thisReset, agents, duration)
	constChart := JumpgateConstructionChart(recs, duration)
	if constChart != nil {
		snippet := constChart.RenderSnippet()
		pageData.ConstructionChart = ChartSnippet{
			Element: template.HTML(snippet.Element),
			Script:  template.HTML(snippet.Script),
		}
	}
	recs = nil

	w.Header().Set("Content-Type", "text/html")
	if err := RenderChartFragment(w, pageData); err != nil {
		logging.Error("template error", err)
	}
	metrics.RecordDuration("chart", start)
}

func PermissionsHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	agents := ds.GetAgents(ds.Reset(resets[0]))

	if err := t.ExecuteTemplate(w, "permissions.html", map[string]interface{}{
		"Agents": agents,
	}); err != nil {
		logging.Error("template error", err)
	}
	agents = nil
	metrics.RecordDuration("permissions", start)
}

func HeaderHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if err := t.ExecuteTemplate(w, "header.html", map[string]interface{}{
		"Reset": ds.LatestReset(),
	}); err != nil {
		logging.Error("template error", err)
	}
	metrics.RecordDuration("header", start)
}

func ExportHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	logging.Info("Export Handler called")

	filename := "data_export.tar.gz"
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	gw := gzip.NewWriter(w)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	srcDir := ds.DataPath()

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == srcDir {
			return nil
		}

		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

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
	metrics.RecordDuration("export", start)
}

func PermissionsGridHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
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
		if searchStr != "" && !strings.Contains(strings.ToLower(agent), searchStr) {
			continue
		}
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

	sort.Slice(d, func(i, j int) bool {
		if sortBy == "credits" {
			if d[i].Credits != d[j].Credits {
				return d[i].Credits > d[j].Credits
			}
			return d[i].Name < d[j].Name
		}
		return d[i].Name < d[j].Name
	})

	agents = nil
	storageAgentsMap = nil

	if err := t.ExecuteTemplate(w, "permissions-grid.html", d); err != nil {
		logging.Error("template error", err)
	}
	metrics.RecordDuration("permissions_grid", start)
}

func LeaderboardHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	leaderboardType := r.URL.Query().Get("type")
	if leaderboardType == "" {
		leaderboardType = "credits"
	}
	myAgent := r.URL.Query().Get("myAgent")

	creditLB, chartLB, err := ds.GetLeaderboard(ds.LatestReset())
	if err != nil {
		logging.Error("error loading leaderboard", err)
		creditLB = nil
		chartLB = nil
	}

	data := creditLB
	if leaderboardType == "charts" {
		data = chartLB
	}

	err = t.ExecuteTemplate(w, "leaderboard.html", map[string]interface{}{
		"Type":    leaderboardType,
		"Data":    data,
		"MyAgent": myAgent,
	})
	creditLB = nil
	chartLB = nil
	data = nil
	if err != nil {
		logging.Error("template error", err)
	}
	metrics.RecordDuration("leaderboard", start)
}

func StatsHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	stats, err := ds.GetStats(ds.LatestReset())
	if err != nil {
		logging.Error("error loading stats", err)
		stats = ds.Stats{}
	}
	if err := t.ExecuteTemplate(w, "stats.html", stats); err != nil {
		logging.Error("template error", err)
	}
	metrics.RecordDuration("stats", start)
}

func JumpgatesHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	gates := ds.GetJumpgates(ds.LatestReset())
	if err := t.ExecuteTemplate(w, "jumpgates.html", gates); err != nil {
		logging.Error("template error", err)
	}
	gates = nil
	metrics.RecordDuration("jumpgates", start)
}
