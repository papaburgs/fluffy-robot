package frontend

import (
	"bytes"
	"html/template"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ds "github.com/papaburgs/fluffy-robot/internal/datastore"
)

func TestStatusBadge(t *testing.T) {
	tests := []struct {
		in    string
		label string
		class string
	}{
		{"online", "online", "status-badge-online"},
		{"ACTIVE", "ACTIVE", "status-badge-online"},
		{" offline ", "offline", "status-badge-offline"},
		{"", "Unknown", "status-badge-unknown"},
	}
	for _, tc := range tests {
		got := statusBadge(tc.in)
		if got.Label != tc.label || got.Class != tc.class {
			t.Errorf("statusBadge(%q) = %+v, want label %q class %q", tc.in, got, tc.label, tc.class)
		}
	}
}

func TestFmtTime(t *testing.T) {
	ts := time.Date(2026, 1, 4, 13, 0, 0, 0, time.UTC)
	if got, want := fmtTime(ts), "2026-01-04 13:00 UTC"; got != want {
		t.Errorf("fmtTime = %q, want %q", got, want)
	}
	if got, want := fmtTime(time.Time{}), "—"; got != want {
		t.Errorf("fmtTime(zero) = %q, want %q", got, want)
	}
	if got, want := fmtTime(time.Unix(0, 0)), "—"; got != want {
		t.Errorf("fmtTime(epoch) = %q, want %q", got, want)
	}
}

func TestFmtReset(t *testing.T) {
	if got, want := fmtReset("2026-01-04T13:00:00Z"), "2026-01-04 13:00 UTC"; got != want {
		t.Errorf("fmtReset = %q, want %q", got, want)
	}
	if got, want := fmtReset("not-a-date"), "not-a-date"; got != want {
		t.Errorf("fmtReset(raw) = %q, want %q", got, want)
	}
	if got, want := fmtReset(""), "—"; got != want {
		t.Errorf("fmtReset(empty) = %q, want %q", got, want)
	}
}

func TestTimeUntil(t *testing.T) {
	if got := timeUntil(time.Time{}); got != "" {
		t.Errorf("timeUntil(zero) = %q, want empty", got)
	}
	if got := timeUntil(time.Now().Add(-time.Hour)); got != "" {
		t.Errorf("timeUntil(past) = %q, want empty", got)
	}
	got := timeUntil(time.Now().Add(2*time.Hour + 30*time.Minute))
	if !strings.HasPrefix(got, "2h ") {
		t.Errorf("timeUntil(2h30m) = %q, want prefix %q", got, "2h ")
	}
	got = timeUntil(time.Now().Add(50 * time.Hour))
	if !strings.HasPrefix(got, "2d ") {
		t.Errorf("timeUntil(50h) = %q, want prefix %q", got, "2d ")
	}
}

// TestStatsTemplateRenders renders the stats page with real data so a
// broken template or a missing helper fails here instead of in the browser.
func TestStatsTemplateRenders(t *testing.T) {
	tmpl, err := template.New("").Funcs(templateFuncs()).ParseGlob(filepath.Join("templates", "*.html"))
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}

	stats := ds.Stats{
		Reset:        "2026-01-04T13:00:00Z",
		MarketUpdate: time.Date(2026, 1, 4, 12, 30, 0, 0, time.UTC),
		Agents:       42,
		Accounts:     7,
		Ships:        1234,
		Systems:      56,
		Waypoints:    89,
		Status:       "online",
		Version:      "v1.2.3",
		NextReset:    time.Now().Add(3 * time.Hour),
		LastUpdate:   time.Date(2026, 1, 4, 12, 35, 0, 0, time.UTC),
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "stats.html", stats); err != nil {
		t.Fatalf("execute stats.html: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"Server Stats",
		"status-badge-online",
		"42",   // agents
		"1234", // ships
		"56",   // systems
		"89",   // waypoints
		"v1.2.3",
		"2026-01-04 13:00 UTC",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered stats page missing %q", want)
		}
	}
}

// TestStatsTemplateEmptyStats makes sure a missing stats row still renders
// a page instead of blowing up on zero timestamps.
func TestStatsTemplateEmptyStats(t *testing.T) {
	tmpl, err := template.New("").Funcs(templateFuncs()).ParseGlob(filepath.Join("templates", "*.html"))
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "stats.html", ds.Stats{}); err != nil {
		t.Fatalf("execute stats.html: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "status-badge-unknown") {
		t.Errorf("empty stats should render an unknown status badge, got:\n%s", out)
	}
	if !strings.Contains(out, "—") {
		t.Errorf("empty stats should render dashes for missing timestamps, got:\n%s", out)
	}
}
