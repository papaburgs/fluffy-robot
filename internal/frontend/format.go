package frontend

import (
	"fmt"
	"strings"
	"time"
)

// statusBadgeInfo is the label + CSS class for the server status pill.
type statusBadgeInfo struct {
	Label string
	Class string
}

// statusBadge maps the API status string to a badge label and class. The
// API reports "online"; older payloads used "ACTIVE".
func statusBadge(s string) statusBadgeInfo {
	label := strings.TrimSpace(s)
	switch strings.ToLower(label) {
	case "online", "active":
		return statusBadgeInfo{Label: label, Class: "status-badge-online"}
	case "":
		return statusBadgeInfo{Label: "Unknown", Class: "status-badge-unknown"}
	default:
		return statusBadgeInfo{Label: label, Class: "status-badge-offline"}
	}
}

// fmtTime renders a timestamp in UTC. The zero value and the unix epoch
// both mean "no data yet" and render as a dash.
func fmtTime(t time.Time) string {
	if t.IsZero() || t.Unix() == 0 {
		return "—"
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

// fmtReset renders the reset identifier. The API sends RFC3339, but fall
// back to the raw string if it ever changes shape.
func fmtReset(s string) string {
	if s == "" {
		return "—"
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return fmtTime(t)
	}
	return s
}

// timeUntil renders a countdown such as "2d 4h" until t. It returns an
// empty string when there is no meaningful target.
func timeUntil(t time.Time) string {
	if t.IsZero() || t.Unix() == 0 {
		return ""
	}
	d := t.Sub(time.Now())
	if d <= 0 {
		return ""
	}
	switch {
	case d >= 24*time.Hour:
		days := int(d.Hours()) / 24
		if h := int(d.Hours()) % 24; h > 0 {
			return fmt.Sprintf("%dd %dh", days, h)
		}
		return fmt.Sprintf("%dd", days)
	case d >= time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}
