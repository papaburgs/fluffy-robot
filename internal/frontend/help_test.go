package frontend

import (
	"bytes"
	"html/template"
	"path/filepath"
	"strings"
	"testing"
)

// TestHelpTemplateRenders renders the static help page so a broken or
// missing template fails here instead of in the browser.
func TestHelpTemplateRenders(t *testing.T) {
	tmpl, err := template.New("").Funcs(templateFuncs()).ParseGlob(filepath.Join("templates", "*.html"))
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "help.html", nil); err != nil {
		t.Fatalf("execute help.html: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"Fluffy Robot Spacetraders Credit Report", // page heading
		"Pick the account you want to view",       // agent selection section
		"Agents or Preferences",
		"5 minutes",  // agent collection cadence
		"30 minutes", // construction progress cadence
		"4 hours",    // new-construction scan cadence
		"Sunday",     // weekly reset
		"Known Issues",
		"Mobile friendly, but not optimized for mobile",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered help page missing %q", want)
		}
	}
}

// TestNavIncludesHelp pins the help entry in the nav menu.
func TestNavIncludesHelp(t *testing.T) {
	tmpl, err := template.New("").Funcs(templateFuncs()).ParseGlob(filepath.Join("templates", "*.html"))
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "nav.html", nil); err != nil {
		t.Fatalf("execute nav.html: %v", err)
	}
	for _, want := range []string{`hx-get="/help"`, "Help"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("nav menu missing %q", want)
		}
	}
}
