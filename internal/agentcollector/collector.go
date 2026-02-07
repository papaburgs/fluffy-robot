package agentcollector

// wanted to split this into multiple packages, but want to make sure we share the gate
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/gate"
)

const (
	fnServerStatus   string = "serverstatus.json"
	fnJumpgateAgents string = "jumpgateagents.json"
)

// Init starts a new collector that will run forever collecting
// from the base url every set amount of time
// Suggest the collectEvery value to be an even divisor of 60, ie 1,2,5,10,12,15,30
// calling function can store the 'data points per hour' value as 60 over the supplied checkEvery number
func Init(ctx context.Context, baseURL, basePath string, collectEvery int, resetChan chan string) {
	checkTimerDuration := time.Duration(collectEvery) * time.Minute
	checkTimer := time.NewTicker(checkTimerDuration)
	jumpgateTime := time.NewTicker(time.Minute)
	gate := gate.New(2, 25)
	collect(ctx, baseURL, basePath, gate, resetChan)
	for {
		select {
		case <-checkTimer.C:
			collect(ctx, baseURL, basePath, gate, resetChan)
		case <-jumpgateTime.C:
			// load in the jumpgate agents file for this reset
			updateJumpgateConstruction(ctx, gate, basePath)
			jumpgateTime.Reset(time.Minute * 15)
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
		slog.Debug("Getting agents", "page", page)
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
	updateJumpgateLists(ctx, basePath, allAgents)
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

	slog.Debug("Looking to update server status")
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

	filename := filepath.Join(basePath, fnServerStatus)
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

func getResetDate(basePath string) string {
	filename := filepath.Join(basePath, fnServerStatus)
	data, err := os.ReadFile(filename)
	if err != nil {
		slog.Error("Could not read file")
		return ""
	}
	var status ServerStatus
	err = json.Unmarshal(data, &status)
	if err != nil {
		slog.Error("error unmarshalling server status JSON", "error", err)
		return ""
	}
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

// extend this collector to get all the jumpgate statuses we care about
// first it goes over all the agents downloaded in the latest update
// we filter out ones that have no activity yet and just hold on to 'active' players
// anyone that is not exactly 175k we check add to the list to check the jumpgates

// then once every 15 mins we look up the jumpgate and store its progress
// if the jumpgate is complete, they get moved to the ignore list as we don't need to check on that gate anymore

type JumpGateAgentListStruct struct {
	AgentsToCheck  []PublicAgent `json:"agents_to_check"`
	AgentsToIgnore []PublicAgent `json:"agents_to_ignore"`
}

// updateJumpgateLists takes the list of all agents, loads in the jumpgate contenders file
// then updates the agents to check. The update is adding anyone that does not have 175K (exactly) and
// removing anyone that is already complete.
func updateJumpgateLists(ctx context.Context, basePath string, allAgents []PublicAgent) {
	var (
		err  error
		data []byte
	)
	if ctx.Err() != nil {
		slog.Warn("context closed, return")
		return
	}
	filename := filepath.Join(basePath, "reset-"+getResetDate(basePath), fnJumpgateAgents)
	currentFileContents := loadJumpgateAgentsFile(ctx, filename)

	// make a map of the ignore list to make it easier to search
	ignoreMap := make(map[string]bool)
	for _, pa := range currentFileContents.AgentsToIgnore {
		ignoreMap[pa.Symbol] = true
	}

	filtered := []PublicAgent{}
	for _, a := range allAgents {
		if a.Credits != 175000 {
			if _, found := ignoreMap[a.Symbol]; !found {
				filtered = append(filtered, a)
			}
		}
	}
	// make a copy of current File contents and update the file
	lists := JumpGateAgentListStruct{
		AgentsToCheck:  filtered,
		AgentsToIgnore: currentFileContents.AgentsToIgnore,
	}
	data, err = json.Marshal(lists)
	if err != nil {
		slog.Error("error marshalling jumpgate agents data", "error", err)
		return
	}
	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		slog.Error("error writing jumpgate agents file", "filename", filename, "error", err)
	}
	return
}

func markJumpgateComplete(ctx context.Context, basePath string, system string) {
	var (
		err  error
		data []byte
	)
	if ctx.Err() != nil {
		slog.Warn("context closed, return")
		return
	}
	filename := filepath.Join(basePath, "reset-"+getResetDate(basePath), fnJumpgateAgents)
	currentFileContents := loadJumpgateAgentsFile(ctx, filename)

	for _, agent := range currentFileContents.AgentsToCheck {
		if strings.Contains(agent.Headquarters, system) {
			currentFileContents.AgentsToIgnore = append(currentFileContents.AgentsToIgnore, agent)
		}
	}
	data, err = json.Marshal(currentFileContents)
	if err != nil {
		slog.Error("error marshalling jumpgate agents data", "error", err)
		return
	}
	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		slog.Error("error writing jumpgate agents file in marking it complete", "filename", filename, "error", err)
	}
	return
}

func loadJumpgateAgentsFile(ctx context.Context, filename string) JumpGateAgentListStruct {
	if ctx.Err() != nil {
		slog.Warn("context closed, return")
		return JumpGateAgentListStruct{}
	}
	var currentFileContents JumpGateAgentListStruct
	data, err := os.ReadFile(filename)
	if err != nil {
		// this could be a file doesn't exist, so we won't error out
		// just send a warn until we know what could happen here
		slog.Warn("Could not read current file with jumpgate agent lists", "error", err)
		return JumpGateAgentListStruct{}
	}
	if err := json.Unmarshal(data, &currentFileContents); err != nil {
		slog.Error("error unmarshalling current jumpgate agents file", "error", err)
		return JumpGateAgentListStruct{}
	}
	return currentFileContents
}
