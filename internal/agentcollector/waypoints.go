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
	"strings"

	"github.com/papaburgs/fluffy-robot/internal/gate"
)

// --- Struct Definitions ---

// SystemWaypoint is the struct for a single waypoint within a system.
type SystemWaypoint struct {
	Symbol   string `json:"symbol"`
	Type     string `json:"type"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Orbitals []struct {
		Symbol string `json:"symbol"`
	} `json:"orbitals"`
	Orbits string `json:"orbits"`
}

// SystemResponseData is the inner data structure from the API response.
// We only care about the Waypoints field for storage.
type SystemResponseData struct {
	Waypoints []SystemWaypoint `json:"waypoints"`
}

// SystemResponse is the top-level structure of the API response.
type SystemResponse struct {
	Data SystemResponseData `json:"data"`
}

// --- Main Logic Function ---

// fetchAndCacheWaypoints processes a list of PublicAgents, determines their home system,
// and fetches/caches the system's waypoints, skipping the API call if cached.
func (c *collector) fetchAndCacheWaypoints(ctx context.Context, gate *gate.Gate, basePath string, systemName string) ([]SystemWaypoint, error) {
	const subDir = "waypoints"

	// Define Cache File Path
	cacheDir := filepath.Join(basePath, "reset-"+c.CurrentReset)
	cacheFilePath := filepath.Join(cacheDir, systemName)

	// Check Cache
	if _, err := os.Stat(cacheFilePath); err == nil {
		// File exists, skip API call
		slog.Debug("Waypoint data found in cache. Skipping API call.", slog.String("path", cacheFilePath))
		data, err := os.ReadFile(cacheFilePath)
		if err != nil {
			slog.Error("Error reading cache file",
				slog.String("path", cacheFilePath),
				slog.String("error", err.Error()))
			return []SystemWaypoint{}, fmt.Errorf("error reading cache file for %s: %w", systemName, err)
		}
		var waypoint []SystemWaypoint
		if err := json.Unmarshal(data, &waypoint); err != nil {
			slog.Error("Error unmarshaling cached waypoint data",
				slog.String("path", cacheFilePath),
				slog.String("error", err.Error()))
			return []SystemWaypoint{}, fmt.Errorf("error unmarshaling cached waypoint data for %s: %w", systemName, err)
		}
		return waypoint, nil

	} else if !os.IsNotExist(err) {
		// An error occurred that wasn't "file not found"
		slog.Error("Error checking cache file status",
			slog.String("path", cacheFilePath),
			slog.String("error", err.Error()))
		return []SystemWaypoint{}, fmt.Errorf("error checking cache file status for %s: %w", systemName, err)
	}

	// Cache Miss - Call API
	slog.Debug("Cache miss. Calling API for waypoints.",
		slog.String("system", systemName))

	waypoints, err := fetchWaypointsFromAPI(ctx, gate, systemName)
	if err != nil {
		slog.Error("Failed to fetch waypoints from API",
			slog.String("system", systemName),
			slog.String("error", err.Error()))
		return []SystemWaypoint{}, fmt.Errorf("error fetching api for %s: %w", systemName, err)
	}

	//  Store Result in Cache File
	err = storeWaypoints(cacheFilePath, cacheDir, waypoints)
	if err != nil {
		slog.Error("Failed to store waypoints to cache file",
			slog.String("system", systemName),
			slog.String("path", cacheFilePath),
			slog.String("error", err.Error()))
		return []SystemWaypoint{}, fmt.Errorf("error storing waypoints for %s: %w", systemName, err)
	}

	slog.Debug("Successfully fetched and cached waypoints",
		slog.String("system", systemName),
		slog.String("path", cacheFilePath))

	return waypoints, nil
}

// fetchWaypointsFromAPI performs the HTTP GET request.
func fetchWaypointsFromAPI(ctx context.Context, gate *gate.Gate, systemName string) ([]SystemWaypoint, error) {
	url := fmt.Sprintf("https://api.spacetraders.io/v2/systems/%s", systemName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create request: %w", err)
	}

	gate.Latch(ctx)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Warn("API call returned non-200 status",
			slog.String("status", resp.Status),
			slog.String("body", strings.TrimSpace(string(body))))
		return nil, fmt.Errorf("api call failed with status: %s", resp.Status)
	}

	var systemResponse SystemResponse
	if err := json.NewDecoder(resp.Body).Decode(&systemResponse); err != nil {
		return nil, fmt.Errorf("failed to decode API response: %w", err)
	}

	return systemResponse.Data.Waypoints, nil
}

// storeWaypoints marshals the waypoints and writes them to the specified cache file.
func storeWaypoints(filePath, dirPath string, waypoints []SystemWaypoint) error {
	// Create the directory if it doesn't exist
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dirPath, err)
	}

	// We only need to store the list of waypoints, so we marshal that array.
	data, err := json.Marshal(waypoints)
	if err != nil {
		return fmt.Errorf("failed to marshal waypoints: %w", err)
	}

	// Write the JSON data to the file
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write to file %s: %w", filePath, err)
	}

	return nil
}
