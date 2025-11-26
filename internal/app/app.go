package app

import (
	"context"
	"html/template"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/agentcache"
	"github.com/papaburgs/fluffy-robot/internal/agentcollector"
)

type AgentRecord struct {
	Timestamp time.Time
	ShipCount int
	Credits   int
}

// App is our main application
// it holds some
type App struct {
	StorageRoot string
	// Reset is on the server status page, we use it to sort data
	Reset string
	// collectPointsPerHour is used to change the charts density
	collectPointsPerHour int
	// agentCache points to the agent Processor that loads files from disk and stores them for 5 mins
	agentCache *agentcache.AgentProcessor
	t          *template.Template
}

// NewApp starts the collector is collect is true
// it returns an app that contains all the handlers
// for the ui
func NewApp(storage string, collect bool) *App {
	var (
		err          error
		collectEvery = 2
	)
	slog.Warn("TODO - reset collectEvery back to 5")
	a := App{
		StorageRoot:          storage,
		Reset:                "00000",
		collectPointsPerHour: 60 / collectEvery,
	}
	a.t = template.Must(template.ParseGlob("templates/*.html"))
	a.agentCache, err = agentcache.NewAgentProcessor(storage)
	if err != nil {
		slog.Error("error starting Agent Processor", "error", err)
	}
	ctx := context.Background()
	// resetChan is passed all they way down to the sererstatus call
	// so it can report what the current reset is.
	// Not sure I like it.
	resetChan := make(chan string)
	if collect {
		go agentcollector.Init(ctx, "https://api.spacetraders.io/v2", a.StorageRoot, collectEvery, resetChan)
	}
	go func(c chan string) {
		for {
			a.Reset = <-c
			slog.Debug("got new reset", "date", a.Reset)
		}
	}(resetChan)
	return &a
}

// mergeAgents merges the global base agent list with any additional agents
// provided via the URL query values. It supports repeated keys
// (?agents=A&agents=B) and comma-separated lists (?agents=A,B), trims
// whitespace, ignores empties, and de-duplicates while preserving order.
// The base slice is not mutated.
func mergeAgents(base []string, q url.Values) []string {
	// Copy base to avoid mutating the global slice.
	outBase := make([]string, len(base))
	copy(outBase, base)

	// Collect extras from query params.
	var extras []string
	if vals, ok := q["agents"]; ok {
		for _, v := range vals {
			for _, part := range strings.Split(v, ",") {
				s := strings.TrimSpace(part)
				if s != "" {
					extras = append(extras, s)
				}
			}
		}
	}

	if len(extras) == 0 {
		return outBase
	}

	// De-duplicate while preserving order: globals first, then extras.
	seen := make(map[string]struct{}, len(outBase)+len(extras))
	merged := make([]string, 0, len(outBase)+len(extras))
	for _, a := range outBase {
		if _, ok := seen[a]; !ok {
			seen[a] = struct{}{}
			merged = append(merged, a)
		}
	}
	for _, e := range extras {
		if _, ok := seen[e]; !ok {
			seen[e] = struct{}{}
			merged = append(merged, e)
		}
	}
	return merged
}
