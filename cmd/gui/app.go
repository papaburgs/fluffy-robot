package main

import (
	"database/sql"
	"html/template"
	"log/slog"
	"reflect"
	"strings"
	"time"
)

// AgentStatus holds the current status of an agent
type AgentStatus struct {
	Active  bool
	Credits int64
}

// App is our main application
type App struct {
	// Reset is on the server status page, we use it to sort data
	Reset string
	// collectPointsPerHour is used to change the charts density
	collectPointsPerHour int
	t                    *template.Template
	DB                   *sql.DB
}

// NewApp returns an app that contains all the handlers for the ui
func NewApp(db *sql.DB) *App {
	var collectEvery = 5 // This should match the collection frequency
	a := App{
		collectPointsPerHour: 60 / collectEvery,
		DB:                   db,
	}

	funcMap := template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
	}

	a.t = template.Must(template.New("").Funcs(funcMap).ParseGlob("templates/*.html"))

	// Perform initial synchronous update
	if err := a.updateReset(); err != nil {
		slog.Error("initial reset fetch failed", "error", err)
	}

	go a.updateResetLoop()

	return &a
}

// updateReset performs a single, blocking update of the Reset field.
func (a *App) updateReset() error {
	var reset string
	var nextReset time.Time
	l := slog.With("function", "updateReset")

	err := a.DB.QueryRow("SELECT reset, nextReset FROM stats ORDER BY reset DESC LIMIT 1").Scan(&reset, &nextReset)
	if err != nil {
		l.Error("failed to fetch reset from database", "error", err)
		a.Reset = ""
		return err
	}

	l.Info("fetched reset from database", "reset", reset, "nextReset", nextReset)
	a.Reset = reset
	return nil
}

// updateResetLoop continuously updates the Reset field of the App struct in the background.
func (a *App) updateResetLoop() {
	l := slog.With("function", "updateResetLoop")
	for {
		var reset string
		var nextReset time.Time

		err := a.DB.QueryRow("SELECT reset, nextReset FROM stats ORDER BY reset DESC LIMIT 1").Scan(&reset, &nextReset)
		if err != nil {
			l.Error("failed to fetch reset in loop", "error", err)
			// Don't wipe the reset on a temporary failure
			time.Sleep(1 * time.Minute) // Wait a minute before retrying on error
			continue
		}

		if a.Reset != reset {
			l.Info("detected new reset", "reset", reset, "nextReset", nextReset)
			a.Reset = reset
		}

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
