package agentcollector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/gate"
)

// Init starts a new collector that will run forever collecting
// from the base url every set amount of time
// Suggest the collectEvery value to be an even divisor of 60, ie 1,2,5,10,12,15,30
// calling function can store the 'data points per hour' value as 60 over the supplied checkEvery number
func Init(ctx context.Context, baseURL, basePath string, collectEvery int, resetChan chan string) {
	checkTimerDuration := time.Duration(collectEvery) * time.Minute
	checkTimer := time.NewTicker(checkTimerDuration)
	gate := gate.New(2, 25)
	collect(ctx, baseURL, basePath, gate, resetChan)
	for {
		select {
		case <-checkTimer.C:
			collect(ctx, baseURL, basePath, gate, resetChan)
		case <-ctx.Done():
			return
		}
	}
}

func collect(ctx context.Context, apiURL, basePath string, gate *gate.Gate, resetChan chan string) {
	client := http.Client{
		Timeout: 5 * time.Second, // Set a reasonable timeout
	}

	if ctx.Err() != nil {
		return
	}
	currentReset := getServerStatus(ctx, apiURL, basePath, gate, resetChan)
	page := 1
	allAgents := []PublicAgent{}
	for { // loop until we have all the pages
		if ctx.Err() != nil {
			return
		}
		slog.Info("Getting agents", "page", page)
		var perPage = 20 // can't see ever changing this unless the api offers more
		fullURL := fmt.Sprintf("%s/agents?limit=%d&page=%d", apiURL, perPage, page)

		gate.Latch(ctx)
		resp, err := client.Get(fullURL)
		if err != nil {
			slog.Error("error in calling api", "error", err)
			return
		}
		defer resp.Body.Close() // Ensure the response body is closed

		if resp.StatusCode != http.StatusOK {
			var b bytes.Buffer
			b.ReadFrom(resp.Body)
			slog.Error("api request failed", "rc", resp.StatusCode, "body", b.String)
			return
		}

		var data ApiResponse
		err = json.NewDecoder(resp.Body).Decode(&data)
		if err != nil {
			slog.Error("error decoding JSON response", "error", err)
			return
		}

		allAgents = append(allAgents, data.Data...)

		if page*perPage > data.Meta.Total {
			break
		}
		page++
	}
	now := time.Now().Unix()
	filename := filepath.Join(basePath, "reset-"+currentReset, fmt.Sprintf("agents-%d.json", now))
	dataBytes, err := json.Marshal(allAgents)
	if err != nil {
		slog.Error("error marshalling agents data", "error", err)
		return
	}
	err = os.WriteFile(filename, dataBytes, 0644)
	if err != nil {
		slog.Error("error writing file", "error", err)
		return
	}

	// all done, empty out values to free memory
	allAgents = nil
}

func getServerStatus(ctx context.Context, apiURL, basePath string, gate *gate.Gate, resetChan chan string) string {
	client := http.Client{
		Timeout: 5 * time.Second, // Set a reasonable timeout
	}

	slog.Info("Looking to update server status")
	fullURL := fmt.Sprintf("%s/", apiURL)

	gate.Latch(ctx)
	resp, err := client.Get(fullURL)
	if err != nil {
		slog.Error("error in calling api", "error", err)
		return ""
	}
	defer resp.Body.Close() // Ensure the response body is closed

	var b bytes.Buffer
	b.ReadFrom(resp.Body)

	if resp.StatusCode != http.StatusOK {
		slog.Error("api request failed", "rc", resp.StatusCode, "body", b.String)
		return ""
	}

	filename := filepath.Join(basePath, "serverstatus.json")
	err = os.WriteFile(filename, b.Bytes(), 0644)
	if err != nil {
		slog.Error("error writing server status file", "error", err)
		return ""
	}

	var status ServerStatus
	err = json.Unmarshal(b.Bytes(), &status)
	if err != nil {
		slog.Error("error unmarshalling server status JSON", "error", err)
		return ""
	}
	ensureDirExists(filepath.Join(basePath, "reset-"+status.ResetDate))
	resetChan <- status.ResetDate
	return status.ResetDate
}

func ensureDirExists(dirPath string) error {
	_, err := os.Stat(dirPath)

	if os.IsNotExist(err) {
		err = os.MkdirAll(dirPath, 0755)
		if err != nil {
			slog.Error("failed to create directory", "directory", dirPath, "error", err)
			return err
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("error checking directory status for '%s': %w", dirPath, err)
	}
	return nil
}

type ServerStatus struct {
	Leaderboards struct {
		// MostCredits Top agents with the most credits.
		MostCredits []struct {
			// AgentSymbol Symbol of the agent.
			AgentSymbol string `json:"agentSymbol"`

			// Credits Amount of credits.
			Credits int64 `json:"credits"`
		} `json:"mostCredits"`
		// MostSubmittedCharts Top agents with the most charted submitted.
		MostSubmittedCharts []struct {
			// AgentSymbol Symbol of the agent.
			AgentSymbol string `json:"agentSymbol"`

			// ChartCount Amount of charts done by the agent.
			ChartCount int `json:"chartCount"`
		} `json:"mostSubmittedCharts"`
	} `json:"leaderboards"`
	// ResetDate The date when the game server was last reset.
	ResetDate    string `json:"resetDate"`
	ServerResets struct {
		// Frequency How often we intend to reset the game server.
		Frequency string `json:"frequency"`
		// Next The date and time when the game server will reset.
		Next time.Time `json:"next"`
	} `json:"serverResets"`
	Stats struct {
		// Accounts Total number of accounts registered on the game server.
		Accounts *int `json:"accounts,omitempty"`
		// Agents Number of registered agents in the game.
		Agents int `json:"agents"`
		// Ships Total number of ships in the game.
		Ships int `json:"ships"`
		// Systems Total number of systems in the game.
		Systems int `json:"systems"`
		// Waypoints Total number of waypoints in the game.
		Waypoints int `json:"waypoints"`
	} `json:"stats"`
	// Status The current status of the game server.
	Status string `json:"status"`
	// Version The current version of the API.
	Version string `json:"version"`
}

type ApiResponse struct {
	Data []PublicAgent `json:"data"`
	Meta Meta          `json:"meta"`
}

type PublicAgent struct {
	// Credits The number of credits the agent has available. Credits can be negative if funds have been overdrawn.
	Credits int64 `json:"credits"`
	// Headquarters The headquarters of the agent.
	Headquarters string `json:"headquarters"`
	// ShipCount How many ships are owned by the agent.
	ShipCount int `json:"shipCount"`
	// StartingFaction The faction the agent started with.
	StartingFaction string `json:"startingFaction"`
	// Symbol Symbol of the agent.
	Symbol string `json:"symbol"`
}

type Meta struct {
	Limit int `json:"limit"`
	Page  int `json:"page"`
	Total int `json:"total"`
}
