package frontend

import (
	"embed"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	ds "github.com/papaburgs/fluffy-robot/internal/datastore"
)

// // AgentStatus holds the current status of an agent
// type AgentStatus struct {
// 	Active  bool
// 	Credits int64
// }

var (
	resets []string
	t      *template.Template
	plog   *slog.Logger
)

//go:embed static
var staticFiles embed.FS // This variable now holds the entire 'static' directory

// NewApp returns an app that contains all the handlers for the ui
func StartServer() {
	plog = slog.With("package", "frontend")

	funcMap := template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
	}

	templateDir := "internal/frontend"
	if env, ok := os.LookupEnv("FLUFFY_TEMPLATE_DIR"); ok {
		plog.Debug("using environment for template dir", "env", env)
		templateDir = env
	} else {
		plog.Debug("generating templates and static from default path")
	}
	plog.Debug("generating templates", "path", templateDir)
	t = template.Must(template.New("").Funcs(funcMap).ParseGlob(filepath.Join(templateDir, "templates", "*.html")))

	////////// Static file handling \\\\\\\\\\
	// define this value to be something in order to use the 'external' css to make it easier to work on
	// leaving it unset will embed the directory instead to make it easier for a server
	if _, ok := os.LookupEnv("FLUFFY_STATIC_DEV"); ok {
		// Dev case
		fs := http.FileServer(http.Dir(filepath.Join(templateDir, "static")))
		http.Handle("/static/", http.StripPrefix("/static/", fs))
	} else {
		// production case
		fsHandler := http.FileServer(http.FS(staticFiles))
		http.Handle("/static/", fsHandler)
	}
	var portNumber string = ":8845"
	if pn, ok := os.LookupEnv("FLUFFY_PORT"); ok {
		portNumber = pn
	}
	if !strings.HasPrefix(portNumber, ":") {
		portNumber = ":" + portNumber
	}

	if templateDir, ok := os.LookupEnv("FLUFFY_TEMPLATE_DIR"); !ok {
		plog.Debug("generating templates from default path")
		t = template.Must(template.New("").Funcs(funcMap).ParseGlob("internal/frontend/templates/*.html"))
	} else {
		plog.Debug("generating templates", "path", templateDir)
		t = template.Must(template.New("").Funcs(funcMap).ParseGlob(templateDir + "templates/*.html"))
	}

	go updateResetLoop()

	http.HandleFunc("/", RootHandler)
	http.HandleFunc("/permissions", PermissionsHandler)
	http.HandleFunc("/status", HeaderHandler)
	http.HandleFunc("/chart", LoadChartHandler)
	http.HandleFunc("/permissions-grid", PermissionsGridHandler)

	http.HandleFunc("/leaderboard", LeaderboardHandler)
	http.HandleFunc("/stats", StatsHandler)
	http.HandleFunc("/jumpgates", JumpgatesHandler)

	slog.Info("Starting server on http://localhost on " + portNumber)
	slog.Warn("Server Done", "Error", http.ListenAndServe(portNumber, nil))

}

// updateResetLoop continuously updates the Reset field of the App struct in the background.
func updateResetLoop() {
	l := slog.With("function", "updateResetLoop")
	for {
		l.Debug("find all resets we have data for")
		resets = ds.AllResets()
		l.Debug("find next reset")
		nextReset := ds.NextReset()

		l.Debug("Ready to sleep", "resets", resets, "next reset", nextReset)

		// Sleep until the next reset plus a buffer, or a shorter interval if nextReset is in the past
		sleepDuration := time.Until(nextReset) + 5*time.Minute
		if sleepDuration < 0 {
			sleepDuration = 5 * time.Minute
		}
		l.Debug("sleeping until next reset check", "duration", sleepDuration)
		time.Sleep(sleepDuration)
	}
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
