package main

var indexPartial = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>fluffy robot Dashboard</title>
    <script src="https://unpkg.com/htmx.org@1.9.10"></script>
	<script src="https://go-echarts.github.io/go-echarts-assets/assets/echarts.min.js"></script>
    <style>
/* --- Global & Layout Refinements --- */
body {
    /* Use a slightly softer dark gray for the background */
    background-color: #121212;
    color: #E0E0E0; /* Light gray for main text color */
    font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; /* A more modern, clean font */
    display: flex;
    flex-direction: column;
    height: 100vh;
    margin: 0;
}

header {
    /* A deep, corporate blue for the header */
    padding: 15px 20px; /* Increased padding */
    background: #1F3C88;
    color: #FFFFFF;
    font-size: 1.5em;
    font-weight: bold;
    box-shadow: 0 2px 4px rgba(0, 0, 0, 0.2); /* Subtle shadow for depth */
}

.main-container {
    display: flex;
    flex: 1;
}

/* --- Sidebar Styling --- */
#sidebar {
    width: 250px; /* Slightly wider sidebar */
    padding: 15px 10px;
    background: #1E1E1E; /* A slightly darker shade than the body */
    overflow-y: auto;
    border-right: 1px solid #333; /* Separator line */
    box-shadow: 2px 0 5px rgba(0, 0, 0, 0.1);
}

/* --- Content Area Styling --- */
#content {
    flex: 1;
    padding: 20px;
    color: #D3D3D3; /* Slightly off-white for content text */
}

#chart {
    padding: 30px;
    background-color: #242424; /* Background for the chart area */
    border-radius: 8px; /* Rounded corners for the chart area */
    box-shadow: inset 0 0 10px rgba(0, 0, 0, 0.3);
}

/* --- Time Button Styling (Assuming this is for general time navigation) --- */
.time-button {
    display: block;
    width: 100%;
    margin-bottom: 8px; /* Increased margin */
    padding: 12px;
    border: none;
    background-color: #3A3A3A; /* Dark gray background */
    color: #FFFFFF;
    text-align: left;
    cursor: pointer;
    border-radius: 20px; /* **More rounded** */
    transition: background-color 0.2s, transform 0.1s;
}

.time-button:hover {
    background-color: #555555;
    transform: translateY(-1px);
}

/* --- Agent List Styling --- */
.agent-list {
    margin-top: 30px;
    border-top: 1px solid #333; /* Darker border */
    padding-top: 15px;
}

/* Default style for an agent button (not selected) */
.agent-button {
    display: block;
    width: 100%;
    margin-bottom: 8px;
    padding: 10px 15px;
    border: 1px solid #444; /* Subtle border */
    background-color: #282828; /* Slightly darker than sidebar */
    color: #B0B0B0; /* Muted text color */
    text-align: left;
    cursor: pointer;
    border-radius: 10px; /* **Rounded corners** for agent list */
    transition: background-color 0.3s, color 0.3s, border-color 0.3s;
}

.agent-button:hover {
    background-color: #3A3A3A;
    color: #FFFFFF;
    border-color: #1F3C88;
}

/* Style for the **selected** agent button */
.agent-button.selected {
    background-color: #1F3C88; /* The primary blue for selection */
    color: #FFFFFF;
    font-weight: bold;
    border-color: #1F3C88;
    box-shadow: 0 2px 5px rgba(31, 60, 136, 0.5); /* Subtle glow effect */
}

.agent-button.selected:hover {
    background-color: #264fa5; /* A slightly lighter blue on hover */
}
.button-group {

  display: flex;
  gap: 15px; 
}

.time-button {
  width: 120px;
  
  flex-shrink: 0; 
  flex-grow: 0;
  text-align: center;
  
  /* height: 60px;  */
  white-space: nowrap; 
}
    </style>
</head>
<body>

    <!-- Hidden container that mirrors current URL query params needed for htmx requests -->
    <form id="query-state" style="display:none;">
        <input type="hidden" name="agents" id="qs-agents" value="">
    </form>

    <script>
      // Mirror URL query params needed by htmx and optionally auto-load a chart
      // when a period is present. Keeps JS minimal and avoids inline JS in attributes.
      document.addEventListener('DOMContentLoaded', function () {
        try {
          var usp = new URLSearchParams(window.location.search || "");

          // Mirror agents from URL into hidden field WITHOUT client-side dedupe.
          // Supports repeated keys (?agents=A&agents=B) and CSV (?agents=A,B).
          var vals = usp.getAll('agents');
          var field = document.getElementById('qs-agents');
          if (field) {
            // Preserve order and duplicates; server will normalize.
            field.value = vals.join(',');
          }

          // Auto-load chart if period is present
          var period = usp.get('period');
          if (period && period.trim() !== '') {
            var values = { period: period };
            if (field && field.value) values.agents = field.value; // comma-separated OK
            htmx.ajax('GET', '/chart', {
              target: '#chart-content', swap: 'innerHTML', values: values
            });
          }
        } catch (_) { /* no-op */ }
      });
    </script>

    <header id="header-status">
	<!--
        <div hx-get="/status"  hx-swap="outerHTML"> 
        </div>
		-->
        <div class="button-group">
		Time range: 
            <button class="time-button" hx-get="/chart" hx-vals='{"period":"1h"}' hx-include="#query-state" hx-target="#chart-content">Last 1h</button>
            <button class="time-button" hx-get="/chart" hx-vals='{"period":"4h"}' hx-include="#query-state" hx-target="#chart-content">Last 4h</button>
            <button class="time-button" hx-get="/chart" hx-vals='{"period":"24h"}' hx-include="#query-state" hx-target="#chart-content">Last 24h</button>
            <button class="time-button" hx-get="/chart" hx-vals='{"period":"7d"}' hx-include="#query-state" hx-target="#chart-content">Last 7d</button>
		</div>
    </header>

    <div class="main-container">
        
	<!--
        <div id="sidebar">
            <h3>Time Filters</h3>
            <button class="time-button" hx-get="/chart" hx-vals='{"period": "1h"}' hx-target="#chart-content">Load Last Hour</button>
            <button class="time-button" hx-get="/chart" hx-vals='{"period": "4h"}' hx-target="#chart-content">Load Last 4 Hours</button>
            <button class="time-button" hx-get="/chart" hx-vals='{"period": "24h"}' hx-target="#chart-content">Load Last 24 Hours</button>
            
            <div class="agent-list" id="agent-list-container">
                <h3>Agents</h3>
                <div hx-get="/agents" hx-trigger="load" hx-target="this" hx-swap="innerHTML">
                    Loading agents...
                </div>
            </div>
        </div>
	-->
        <div id="content">
            <h2>Chart Visualization</h2>
            <div id="chart-content">
                Click a filter button to load a chart.
            </div>
        </div>

    </div>

</body>
</html>
`

var headerPartial = `
<header id="header-status">
    <h1>{{.Title}}</h1>
    <p>Reset Date: {{.ResetDate}} | Last Load: {{.LoadTime}}</p>
</header>
`

var agentsPartial = `
<h3>Agents</h3>
{{range .}}
    <div>
        <input type="checkbox" name="agent" value="{{.ID}}" id="agent-{{.ID}}">
        <label for="agent-{{.ID}}">{{.Name}}</label>
    </div>
{{end}}
`

var chartPartial = `
<div id="chart">{{ .Element }} {{ .Script }}</div>
`
