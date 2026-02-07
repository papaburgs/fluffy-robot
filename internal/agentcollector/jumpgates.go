package agentcollector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/gate"
)

const knownAgentsFile = "knownagents.json"

// --- New Struct Definitions ---

// KnownAgent is the structure for the data stored in the local 'agents.json' file.
// This file tracks agents whose waypoints have already been fetched.
type KnownAgent struct {
	Symbol           string `json:"symbol"`
	HomeSystem       string `json:"homeSystem"`
	JumpgateSymbol   string `json:"jumpgateSymbol"`
	ConstructionDone bool   `json:"constructionDone"`
}

// ConstructionMaterial defines a required material for construction.
type ConstructionMaterial struct {
	TradeSymbol string `json:"tradeSymbol"`
	Required    int    `json:"required"`
	Fulfilled   int    `json:"fulfilled"`
}

// ConstructionData is the core data returned by the construction API endpoint.
type ConstructionData struct {
	Symbol     string                 `json:"symbol"`
	Materials  []ConstructionMaterial `json:"materials"`
	IsComplete bool                   `json:"isComplete"`
}

// ConstructionResponse is the top-level structure of the construction API response.
type ConstructionResponse struct {
	Data ConstructionData `json:"data"`
}

// JumpgateConstructionStatus combines the system and waypoint symbol
// with the construction data for the final output file.
type JumpgateConstructionStatus struct {
	SystemSymbol   string           `json:"systemSymbol"`
	WaypointSymbol string           `json:"waypointSymbol"`
	Status         ConstructionData `json:"status"`
}

// --- Main Logic Function ---

// UpdateJumpgateConstruction reads known agents, finds new agents, fetches waypoints,
// identifies jumpgates, calls the construction API for each, and saves the results.
func (c *collector) updateJumpgateConstruction(ctx context.Context) {
	slog.Info("Starting jumpgate construction update process.")

	return
}

// --- Helper Functions ---
const subDir = "jumpgates"

func fetchConstructionStatus(ctx context.Context, gate *gate.Gate, systemSymbol, waypointSymbol string) (ConstructionData, error) {
	url := fmt.Sprintf("https://api.spacetraders.io/v2/systems/%s/waypoints/%s/construction", systemSymbol, waypointSymbol)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ConstructionData{}, fmt.Errorf("could not create request: %w", err)
	}

	gate.Latch(ctx)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ConstructionData{}, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return ConstructionData{}, fmt.Errorf("api call failed with status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var response ConstructionResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return ConstructionData{}, fmt.Errorf("failed to decode construction API response: %w", err)
	}

	return response.Data, nil
}

// saveConstructionStatus marshals and saves the final list of statuses.
func saveConstructionStatus(filePath string, statuses []JumpgateConstructionStatus) error {
	data, err := json.MarshalIndent(statuses, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal construction statuses: %w", err)
	}
	return os.WriteFile(filePath, data, 0644)
}

// NOTE: The main package's `fetchWaypointsFromAPI`, `storeWaypoints`,
// and the main `PublicAgent`/`SystemWaypoint` structs from the previous answer
// are still required and assumed to be present in this package.
