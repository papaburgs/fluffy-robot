package agentcollector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
func updateJumpgateConstruction(ctx context.Context, gate *gate.Gate, basePath string) error {
	slog.Info("Starting jumpgate construction update process.")

	// 1. Load known agents and agent list from files
	knownAgents, err := loadKnownAgents(basePath)
	if err != nil {
		return fmt.Errorf("failed to load known agents: %w", err)
	}

	filename := filepath.Join(basePath, "reset-"+getResetDate(basePath), fnJumpgateAgents)
	allPublicAgents := loadJumpgateAgentsFile(ctx, filename)

	// Helper map for quick lookup of existing systems
	knownSystems := make(map[string]bool)
	for _, a := range knownAgents {
		knownSystems[a.HomeSystem] = true
	}

	// Identify and Process New Agents/Systems
	newKnownAgents := make([]KnownAgent, 0, len(allPublicAgents.AgentsToCheck))
	jumpgateSymbols := make(map[string]string) // Map systemName -> jumpgateSymbol

	for _, agent := range allPublicAgents.AgentsToCheck {
		hq := agent.Headquarters
		if len(hq) < 3 {
			continue // Skip invalid Headquarters
		}
		systemName := hq[:len(hq)-3]

		// Check if system is already known
		if knownSystems[systemName] {
			continue
		}
		knownSystems[systemName] = true // Mark as processed

		slog.Debug("Processing new system", slog.String("system", systemName))

		// a. Fetch Waypoints (reusing logic from previous request)
		// NOTE: Assuming Waypoints are cached in basePath/waypoints/systemName
		waypoints, err := fetchAndCacheWaypoints(ctx, gate, basePath, systemName)
		if err != nil {
			slog.Error("Failed to get waypoints for new system", slog.String("system", systemName), slog.String("error", err.Error()))
			continue
		}

		// b. Find Jumpgate
		jumpgateSymbol := ""
		for _, wp := range waypoints {
			// The Spacetraders API defines a Jump Gate as having the type "JUMP_GATE"
			if wp.Type == "JUMP_GATE" {
				jumpgateSymbol = wp.Symbol
				break
			}
		}

		if jumpgateSymbol == "" {
			slog.Warn("No JUMP_GATE found in system", slog.String("system", systemName))
			continue
		}

		slog.Debug("Found jumpgate", slog.String("system", systemName), slog.String("jumpgate", jumpgateSymbol))

		// c. Store the new known agent/system data
		newKnownAgents = append(newKnownAgents, KnownAgent{
			Symbol:         agent.Symbol,
			HomeSystem:     systemName,
			JumpgateSymbol: jumpgateSymbol,
		})
		jumpgateSymbols[systemName] = jumpgateSymbol
	}

	// Update the known agents file
	updatedKnownAgents := append(knownAgents, newKnownAgents...)
	if err := saveKnownAgents(basePath, updatedKnownAgents); err != nil {
		slog.Error("Failed to save updated known agents file", slog.String("error", err.Error()))
		// This is non-fatal for the current run, so we continue.
	}

	// Collect ALL Jumpgates (from old and newly processed agents)
	allJumpgates := make(map[string]string) // SystemSymbol -> JumpgateSymbol
	for _, agent := range updatedKnownAgents {
		// Use the jumpgate symbol from the loaded or newly found data
		allJumpgates[agent.HomeSystem] = agent.JumpgateSymbol
	}

	// Call Construction API for all Jumpgates
	constructionStatuses := make([]JumpgateConstructionStatus, 0, len(allJumpgates))
	for systemSymbol, waypointSymbol := range allJumpgates {
		status, err := fetchConstructionStatus(ctx, gate, systemSymbol, waypointSymbol)
		if err != nil {
			slog.Error("Failed to fetch construction status",
				slog.String("system", systemSymbol),
				slog.String("waypoint", waypointSymbol),
				slog.String("error", err.Error()))
			continue
		}
		constructionStatuses = append(constructionStatuses, JumpgateConstructionStatus{
			SystemSymbol:   systemSymbol,
			WaypointSymbol: waypointSymbol,
			Status:         status,
		})
		if status.IsComplete {
			markJumpgateComplete(ctx, basePath, systemSymbol)
		}

	}

	// Save Final Results
	outputDir := filepath.Join(basePath, "reset-"+getResetDate(basePath), "construction_data")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
	}

	// Timestamped File
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	timestampedFileName := fmt.Sprintf("construction_status_%s.json", timestamp)
	timestampedPath := filepath.Join(outputDir, timestampedFileName)
	if err := saveConstructionStatus(timestampedPath, constructionStatuses); err != nil {
		return fmt.Errorf("failed to save timestamped status file: %w", err)
	}
	slog.Info("Saved timestamped construction status", slog.String("file", timestampedPath))

	// Latest File
	latestPath := filepath.Join(outputDir, "latest.json")
	if err := saveConstructionStatus(latestPath, constructionStatuses); err != nil {
		return fmt.Errorf("failed to save latest status file: %w", err)
	}
	slog.Info("Saved latest construction status", slog.String("file", latestPath))

	return nil
}

// --- Helper Functions ---
const subDir = "jumpgates"

// loadKnownAgents reads the local file containing systems/jumpgates we've already processed.
func loadKnownAgents(basePath string) ([]KnownAgent, error) {
	filePath := filepath.Join(basePath, "reset-"+getResetDate(basePath), subDir, knownAgentsFile)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Debug("Known agents file not found, starting fresh.", slog.String("path", filePath))
			return []KnownAgent{}, nil // Return empty list if file doesn't exist
		}
		return nil, err
	}
	var agents []KnownAgent
	if err := json.Unmarshal(data, &agents); err != nil {
		return nil, fmt.Errorf("failed to unmarshal known agents: %w", err)
	}
	return agents, nil
}

// saveKnownAgents writes the updated list of known systems/jumpgates.
func saveKnownAgents(basePath string, agents []KnownAgent) error {
	filePath := filepath.Join(basePath, "reset-"+getResetDate(basePath), subDir, knownAgentsFile)
	data, err := json.MarshalIndent(agents, "", "  ")
	if err != nil {
		return err
	}
	// Ensure the parent directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// loadPublicAgents is a placeholder for loading the list of all agents.
// In a real application, this might load from a different API or data source.
func loadPublicAgents(filePath string) ([]PublicAgent, error) {
	// Dummy implementation for the example - assuming a file with a list of PublicAgent structs
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var agents []PublicAgent
	if err := json.Unmarshal(data, &agents); err != nil {
		return nil, fmt.Errorf("failed to unmarshal public agents list: %w", err)
	}
	return agents, nil
}

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
