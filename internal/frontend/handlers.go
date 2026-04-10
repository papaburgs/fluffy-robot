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

	chartAgents := mergeAgents(storageAgents, paramAgents)

	pageData := ChartPageData{}

	period := q.Get("period")
	var duration time.Duration
	var startTime int64
	var creditChart *charts.Line
	var shipChart *charts.Line
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
	startTime = time.Now().Add(-1 * duration).Unix()

	thisReset := ds.Reset(resets[0])

	agentHist, _ := ds.GetAgentHistory(thisReset, startTime, 0)
	jgList, _ := ds.GetJumpgateList(thisReset)
	constrList, _ := ds.GetConstructions(thisReset, startTime, 0)

	agentsLookup := make(map[string]ds.Agent)
	aList, _ := ds.GetAgentList(thisReset)
	agentsLookup = agentsMap(aList)
	jgLookup := jumpgatesMap(jgList)

	creditChart = CreditChart(chartAgents, agentHist, duration, title)
	shipChart = ShipChart(chartAgents, agentHist, duration, title)

	if creditChart != nil {
		snippet := creditChart.RenderSnippet()
		pageData.CreditChart = ChartSnippet{
			Element: template.HTML(snippet.Element),
			Script:  template.HTML(snippet.Script),
		}
	}

	if shipChart != nil {
		snippet := shipChart.RenderSnippet()
		pageData.ShipChart = ChartSnippet{
			Element: template.HTML(snippet.Element),
			Script:  template.HTML(snippet.Script),
		}
	}

	overview := latestConstructionRecords(agentsLookup, jgLookup, constrList, chartAgents)
	pageData.ConstructionTable = overview
	overview = nil

	recs := constructionRecords(agentsLookup, jgLookup, constrList, chartAgents)
	constChart := JumpgateConstructionChart(recs, duration)
	if constChart != nil {
		snippet := constChart.RenderSnippet()
		pageData.ConstructionChart = ChartSnippet{
			Element: template.HTML(snippet.Element),
			Script:  template.HTML(snippet.Script),
		}
	}
	recs = nil

	agentHist = nil
	aList = nil
	jgList = nil
	constrList = nil

	w.Header().Set("Content-Type", "text/html")
	if err := RenderChartFragment(w, pageData); err != nil {
		logging.Error("template error", err)
	}
	metrics.RecordDuration("chart", start)
}

func PermissionsHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	aList, err := ds.GetAgentList(ds.Reset(resets[0]))
	if err != nil {
		logging.Error("error loading agents", err)
	}
	agents := agentsMap(aList)

	if err := t.ExecuteTemplate(w, "permissions.html", map[string]interface{}{
		"Agents": agents,
	}); err != nil {
		logging.Error("template error", err)
	}
	agents = nil
	aList = nil
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
	aList, err := ds.GetAgentList(ds.LatestReset())
	if err != nil {
		logging.Error("error loading agents", err)
	}
	agents := agentsMap(aList)

	type data struct {
		Name      string
		Credits   int64
		IsActive  bool
		IsChecked bool
	}
	d := []data{}
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
	aList = nil
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
	thisReset := ds.LatestReset()

	aList, _ := ds.GetAgentList(thisReset)
	jgList, _ := ds.GetJumpgateList(thisReset)
	constrList, _ := ds.GetConstructions(thisReset, 0, 0)

	agentsLookup := agentsMap(aList)
	jumpgates := jumpgatesMap(jgList)

	allNames := make([]string, 0, len(agentsLookup))
	for name := range agentsLookup {
		allNames = append(allNames, name)
	}
	construction := latestConstructionRecords(agentsLookup, jumpgates, constrList, allNames)
	constructMap := make(map[string]ds.ConstructionOverview, len(construction))
	for _, c := range construction {
		constructMap[c.Agent] = c
	}

	rows := []ConstructionParallelRow{}
	for name, a := range agentsLookup {
		jg, ok := jumpgates[a.System]
		if !ok || jg.Status == ds.NoActivity {
			continue
		}
		co, hasConstruct := constructMap[name]
		if !hasConstruct || (co.Fabmat == 0 && co.Advcct == 0) {
			continue
		}
		rows = append(rows, ConstructionParallelRow{
			Agent:    name,
			Jumpgate: jg.Jumpgate,
			Fabmat:   co.Fabmat,
			Advcct:   co.Advcct,
		})
	}

	aList = nil
	jgList = nil
	constrList = nil
	agentsLookup = nil
	jumpgates = nil
	construction = nil
	constructMap = nil

	parallel := ConstructionParallelChart(rows)
	rows = nil

	snippet := parallel.RenderSnippet()
	pageData := struct {
		ParallelChart ChartSnippet
	}{
		ParallelChart: ChartSnippet{
			Element: template.HTML(snippet.Element),
			Script:  template.HTML(snippet.Script),
		},
	}

	w.Header().Set("Content-Type", "text/html")
	if err := t.ExecuteTemplate(w, "jumpgates.html", pageData); err != nil {
		logging.Error("template error", err)
	}
	metrics.RecordDuration("jumpgates", start)
}

type factionInfo struct {
	Symbol string
	Name   string
	Color  string
}

var factionMap = map[string]factionInfo{
	"COSMIC":   {Symbol: "COSMIC", Name: "Cosmic Engineers", Color: "#7B68EE"},
	"GALACTIC": {Symbol: "GALACTIC", Name: "Galactic Alliance", Color: "#4169E1"},
	"QUANTUM":  {Symbol: "QUANTUM", Name: "Quantum Federation", Color: "#00CED1"},
	"DOMINION": {Symbol: "DOMINION", Name: "Stellar Dominion", Color: "#DC143C"},
	"ASTRO":    {Symbol: "ASTRO", Name: "Astro-Salvage Alliance", Color: "#DAA520"},
	"CORSAIRS": {Symbol: "CORSAIRS", Name: "Seventh Space Corsairs", Color: "#8B0000"},
	"VOID":     {Symbol: "VOID", Name: "Voidfarers", Color: "#708090"},
	"OBSIDIAN": {Symbol: "OBSIDIAN", Name: "Obsidian Syndicate", Color: "#2F2F2F"},
	"AEGIS":    {Symbol: "AEGIS", Name: "Aegis Collective", Color: "#4682B4"},
	"UNITED":   {Symbol: "UNITED", Name: "United Independent Settlements", Color: "#228B22"},
}

type AgentRow struct {
	Symbol        string
	Faction       string
	Credits       int64
	Ships         int64
	System        string
	IsActive      bool
	FactionColor  string
	FactionName   string
	Construction  string
	SystemCount   int
	MultiSystem   bool
	IsChecked     bool
	ShowConstruct bool
}

func AgentsHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	thisReset := ds.LatestReset()

	uniqueFactions := make(map[string]factionInfo)
	for _, fi := range factionMap {
		uniqueFactions[fi.Symbol] = fi
	}
	aList, _ := ds.GetAgentList(thisReset)
	systemSet := make(map[string]bool)
	for _, a := range aList {
		systemSet[a.System] = true
	}
	systemList := make([]string, 0, len(systemSet))
	for s := range systemSet {
		systemList = append(systemList, s)
	}
	sort.Strings(systemList)
	aList = nil

	if err := t.ExecuteTemplate(w, "agents.html", map[string]interface{}{
		"Factions": uniqueFactions,
		"Systems":  systemList,
	}); err != nil {
		logging.Error("template error", err)
	}
	metrics.RecordDuration("agents", start)
}

func AgentsGridHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	thisReset := ds.LatestReset()

	aList, _ := ds.GetAgentList(thisReset)
	agentHist, _ := ds.GetAgentHistory(thisReset, 0, 0)
	jgList, _ := ds.GetJumpgateList(thisReset)
	constrList, _ := ds.GetConstructions(thisReset, 0, 0)

	agentsLookup := agentsMap(aList)
	ships := latestShips(agentHist)
	jumpgates := jumpgatesMap(jgList)

	allNames := make([]string, 0, len(agentsLookup))
	for name := range agentsLookup {
		allNames = append(allNames, name)
	}
	construction := latestConstructionRecords(agentsLookup, jumpgates, constrList, allNames)
	constructMap := make(map[string]ds.ConstructionOverview, len(construction))
	for _, c := range construction {
		constructMap[c.Agent] = c
	}

	systemCount := make(map[string]int)
	for _, a := range aList {
		systemCount[a.System]++
	}

	storageAgents := r.URL.Query().Get("storageAgents")
	paramAgents := r.URL.Query().Get("paramAgents")
	storageAgentsMap := make(map[string]bool)
	for _, i := range mergeAgents(storageAgents, paramAgents) {
		storageAgentsMap[i] = true
	}

	q := r.URL.Query()
	searchStr := strings.ToLower(q.Get("agentSearch"))
	hideInactive := q.Get("hideInactive") == "on"
	sortBy := q.Get("sortBy")
	filterFaction := q.Get("faction")
	filterSystem := q.Get("system")
	showConstructionOnly := q.Get("showConstruction") == "on"

	rows := []AgentRow{}
	for name, a := range agentsLookup {
		if searchStr != "" && !strings.Contains(strings.ToLower(name), searchStr) {
			continue
		}
		if hideInactive && a.Credits == 175000 {
			continue
		}
		if filterFaction != "" && a.Faction != filterFaction {
			continue
		}
		if filterSystem != "" && a.System != filterSystem {
			continue
		}

		constructStr, hasConstruct := "\u2014", false
		if co, ok := constructMap[name]; ok {
			constructStr, hasConstruct = constructionString(co, jumpgates[a.System])
		}

		if showConstructionOnly && !hasConstruct {
			continue
		}

		fi, ok := factionMap[a.Faction]
		factionColor := "#666"
		factionName := a.Faction
		if ok {
			factionColor = fi.Color
			factionName = fi.Name
		}

		rows = append(rows, AgentRow{
			Symbol:        name,
			Faction:       a.Faction,
			Credits:       a.Credits,
			Ships:         ships[name],
			System:        a.System,
			IsActive:      a.Credits != 175000,
			FactionColor:  factionColor,
			FactionName:   factionName,
			Construction:  constructStr,
			SystemCount:   systemCount[a.System],
			MultiSystem:   systemCount[a.System] > 1,
			IsChecked:     storageAgentsMap[name],
			ShowConstruct: hasConstruct,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if sortBy == "credits" {
			if rows[i].Credits != rows[j].Credits {
				return rows[i].Credits > rows[j].Credits
			}
			return rows[i].Symbol < rows[j].Symbol
		}
		if sortBy == "ships" {
			if rows[i].Ships != rows[j].Ships {
				return rows[i].Ships > rows[j].Ships
			}
			return rows[i].Symbol < rows[j].Symbol
		}
		return rows[i].Symbol < rows[j].Symbol
	})

	aList = nil
	agentHist = nil
	jgList = nil
	constrList = nil
	agentsLookup = nil
	ships = nil
	jumpgates = nil
	construction = nil
	constructMap = nil

	if err := t.ExecuteTemplate(w, "agents-grid.html", map[string]interface{}{
		"Agents": rows,
	}); err != nil {
		logging.Error("template error", err)
	}
	metrics.RecordDuration("agents_grid", start)
}
