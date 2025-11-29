package app

import (
	"context"
	"html/template"
	"log/slog"
	"reflect"
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
		collectEvery = 5
	)
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

// mergeAgents accepts a variable number of 'any' type arguments.
// It processes the arguments to collect strings:
// - If an argument is a string, it is split by comma, trimmed, and added.
// - If an argument is a []string (list of strings), its elements are appended.
// The function returns a deduplicated list of strings
func mergeAgents(args ...any) []string {

	// 'seen' map tracks elements already added to 'merged'
	seen := make(map[string]bool)

	for _, arg := range args {
		if arg == nil {
			continue // Skip nil arguments
		}

		// Use reflection to check the type of the argument
		v := reflect.ValueOf(arg)
		kind := v.Kind()

		switch kind {
		case reflect.String:
			// if its a string, treat it as comma separated
			s := v.String()
			for _, part := range strings.Split(s, ",") {
				trimmed := strings.TrimSpace(part)
				if trimmed != "" {
					seen[trimmed] = true
				}
			}

		case reflect.Slice:
			// Case 2: Argument is a slice. Check if it's a []string.
			if v.Type().Elem().Kind() == reflect.String {
				// Iterate over the slice elements and append them
				for i := 0; i < v.Len(); i++ {
					// v.Index(i).Interface() gets the element as 'any', then cast to string
					if s, ok := v.Index(i).Interface().(string); ok {
						trimmed := strings.TrimSpace(s)
						if trimmed != "" {
							seen[trimmed] = true
						}
					}
				}
			}
		default:
			continue
		}
	}

	merged := make([]string, 0, len(seen)*2)
	for e := range seen {
		merged = append(merged, e)
	}

	return merged
}
