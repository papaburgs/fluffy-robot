package frontend

import (
	"expvar"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	ds "github.com/papaburgs/fluffy-robot/internal/datastore"
	"github.com/papaburgs/fluffy-robot/internal/logging"
)

var (
	resets []string
	t      *template.Template
)

func StartServer() {
	funcMap := template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
		"unixTime": func(ts int64) string {
			return time.Unix(ts, 0).Format("2006-01-02 15:04")
		},
		"constructionStatus": func(s uint8) string {
			switch s {
			case 0:
				return "NoActivity"
			case 1:
				return "Active"
			case 2:
				return "Const"
			case 3:
				return "Complete"
			default:
				return "Unknown"
			}
		},
	}

	templateDir := "internal/frontend"
	if env, ok := os.LookupEnv("FLUFFY_TEMPLATE_DIR"); ok {
		templateDir = env
	}
	t = template.Must(template.New("").Funcs(funcMap).ParseGlob(filepath.Join(templateDir, "templates", "*.html")))

	staticDir := "internal/frontend"
	if env, ok := os.LookupEnv("FLUFFY_STATIC_DIR"); ok {
		staticDir = env
	}
	fs := http.FileServer(http.Dir(filepath.Join(staticDir, "static")))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	var portNumber string = ":8845"
	if pn, ok := os.LookupEnv("FLUFFY_PORT"); ok {
		portNumber = pn
	}
	if !strings.HasPrefix(portNumber, ":") {
		portNumber = ":" + portNumber
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

	http.HandleFunc("/export", ExportHandler)

	http.Handle("/debug/vars", expvar.Handler())

	logging.Info("Starting server on http://localhost on " + portNumber)
	logging.Warn("Server Done", http.ListenAndServe(portNumber, nil))
}

func updateResetLoop() {
	for {
		// logging.Debug("find all resets we have data for")
		resets = ds.AllResets()
		// logging.Debug("find next reset")
		nextReset := ds.NextReset()

		// logging.Debug("Ready to sleep", "resets", resets, "next reset", nextReset)

		sleepDuration := time.Until(nextReset) + 5*time.Minute
		if sleepDuration < 0 {
			sleepDuration = 5 * time.Minute
		}
		// logging.Debug("sleeping until next reset check", "duration", sleepDuration)
		time.Sleep(sleepDuration)
	}
}

func mergeAgents(args ...any) []string {
	seen := make(map[string]bool)

	for _, arg := range args {
		if arg == nil {
			continue
		}

		v := reflect.ValueOf(arg)
		kind := v.Kind()

		switch kind {
		case reflect.String:
			s := v.String()
			for _, part := range strings.Split(s, ",") {
				trimmed := strings.TrimSpace(part)
				if trimmed != "" {
					seen[trimmed] = true
				}
			}

		case reflect.Slice:
			if v.Type().Elem().Kind() == reflect.String {
				for i := 0; i < v.Len(); i++ {
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
