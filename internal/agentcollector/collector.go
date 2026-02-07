package agentcollector

// wanted to split this into multiple packages, but want to make sure we share the gate
import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/gate"
)

const (
	fnServerStatus string = "serverstatus.json"
)

// type agentSystemMap map[string]string

type Collector struct {
	// map of agent name to jumpgate
	agentsToCheck map[string]string
	// map of headquarters to jumpgate
	hq2jg map[string]string
	// map of jumpgate to bool
	completedGates map[string]time.Time
	baseURL        string
	basePath       string
	gate           *gate.Gate
	CurrentReset   string
}

// Init starts a new Collector that will run forever collecting
// from the base url every set amount of time
// Suggest the collectEvery value to be an even divisor of 60, ie 1,2,5,10,12,15,30
// calling function can store the 'data points per hour' value as 60 over the supplied checkEvery number
func Init(ctx context.Context, baseURL, basePath string, collectEvery int, resetChan chan string) *Collector {
	c := &Collector{
		agentsToCheck:  make(map[string]string),
		completedGates: make(map[string]time.Time),
		hq2jg:          make(map[string]string),
		baseURL:        baseURL,
		basePath:       basePath,
		gate:           gate.New(2, 25),
		CurrentReset:   "",
	}
	collectTime := time.Duration(collectEvery) * time.Minute
	// jumpgateTime := time.NewTicker(time.Minute * 15)
	go c.collect(ctx, collectTime)
	// go c.updateJumpgateConstruction(ctx, jumpgateTime)
	return c
}

func (c *Collector) collect(ctx context.Context, collectTime time.Duration) {
	for {
		client := http.Client{
			Timeout: 5 * time.Second, // Set a reasonable timeout
		}

		if ctx.Err() != nil {
			return
		}

		c.updateServerStatus(ctx)
		page := 1
		allAgents := []PublicAgent{}
		for { // loop until we have all the pages
			if ctx.Err() != nil {
				return
			}
			slog.Debug("Getting agents", "page", page)
			var perPage = 20 // can't see ever changing this unless the api offers more
			fullURL := fmt.Sprintf("%s/agents?limit=%d&page=%d", c.baseURL, perPage, page)

			c.gate.Latch(ctx)
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
		// === CSV Writing Logic ===
		now := time.Now().UTC()
		csvFilename := filepath.Join(c.basePath, "reset-"+c.CurrentReset, "agents.csv")

		if err := ensureDirExists(filepath.Dir(csvFilename)); err != nil {
			slog.Error("failed to ensure directory exists for CSV file", "error", err, "path", filepath.Dir(csvFilename))
			return
		}

		file, err := os.OpenFile(csvFilename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			slog.Error("error opening CSV file", "error", err, "filename", csvFilename)
			return
		}
		defer file.Close()

		writer := csv.NewWriter(file)
		defer writer.Flush()

		for _, agent := range allAgents {
			record := []string{
				strconv.FormatInt(now.Unix(), 10),
				agent.Symbol,
				strconv.FormatInt(agent.Credits, 10),
				strconv.Itoa(agent.ShipCount),
				agent.Headquarters,
			}
			if err := writer.Write(record); err != nil {
				slog.Error("error writing CSV record", "error", err, "record", record)
				return
			}
		}
		// === End CSV Writing Logic ===

		c.updateJumpgateLists(ctx, allAgents)

		// all done, empty out values to free memory
		allAgents = nil
		time.Sleep(collectTime)
	}
}

func (c *Collector) updateServerStatus(ctx context.Context) {
	client := http.Client{
		Timeout: 5 * time.Second, // Set a reasonable timeout
	}

	slog.Debug("Looking to update server status")
	fullURL := fmt.Sprintf("%s/", c.baseURL)

	c.gate.Latch(ctx)
	resp, err := client.Get(fullURL)
	if err != nil {
		slog.Error("error in calling api", "error", err)
		return
	}
	defer resp.Body.Close() // Ensure the response body is closed

	var b bytes.Buffer
	b.ReadFrom(resp.Body)

	if resp.StatusCode != http.StatusOK {
		slog.Error("api request failed", "rc", resp.StatusCode, "body", b.String)
		return
	}

	filename := filepath.Join(c.basePath, fnServerStatus)
	err = os.WriteFile(filename, b.Bytes(), 0644)
	if err != nil {
		slog.Error("error writing server status file", "error", err)
		return
	}

	key := []byte(`"resetDate":"`)
	start := bytes.Index(b.Bytes(), key)
	if start == -1 {
		slog.Error("did not find reset Date start")
	}
	start += len(key)
	end := bytes.IndexByte(b.Bytes()[start:], '"')
	if end == -1 {
		slog.Error("did not find end")
	}
	c.CurrentReset = string(b.Bytes()[start : start+end])
	ensureDirExists(filepath.Join(c.basePath, "reset-"+c.CurrentReset))
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

// extend this Collector to get all the jumpgate statuses we care about
// first it goes over all the agents downloaded in the latest update
// we filter out ones that have no activity yet and just hold on to 'active' players
// anyone that is not exactly 175k we check add to the list to check the jumpgates

// then once every 15 mins we look up the jumpgate and store its progress
// this list of active agents (agents to check) will be starting as the system,
// we will update that to the actual jumpgate symbol when we check.
// then we check all the jumpgates, if it is done, we add it to the complete list.

// updateJumpgateLists takes the list of all agents and updates the in-memory jumpGateAgentList and
// the headquarter to jumpgate map if required
// It identifies agents that are not at 175k credits.
// adding them to agentsToCheck.
func (c *Collector) updateJumpgateLists(ctx context.Context, allAgents []PublicAgent) {
	if ctx.Err() != nil {
		slog.Warn("context closed, return")
		return
	}

	// first go through all agents and find agents that have a non-175k value
	for _, a := range allAgents {
		if a.Credits != 175000 {
			if _, found := c.agentsToCheck[a.Symbol]; !found {
				c.agentsToCheck[a.Symbol] = a.Headquarters
				if _, hqfound := c.hq2jg[a.Headquarters]; !hqfound {
					c.hq2jg[a.Headquarters] = ""
				}
			}
		}
	}
}
