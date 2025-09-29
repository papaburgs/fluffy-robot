package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

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

func (a *App) collect(apiURL string) {
	client := http.Client{
		Timeout: 5 * time.Second, // Set a reasonable timeout
	}

	a.getServerStatus(apiURL)
	page := 1

	for { // loop until we have all the pages
		var perPage = 20 // can't see ever changing this
		fullURL := fmt.Sprintf("%s/agents?limit=%d&page=%d", apiURL, perPage, page)

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
		go a.updateHistory(data.Data)

		if page*perPage > data.Meta.Total {
			break
		}
		page++
	}
	time.Sleep(10 * time.Second)
	slog.Debug("writing to disk")
	a.Backup()
}

func (a *App) updateHistory(dl []PublicAgent) {
	var (
		agent []AgentRecord
		rec   AgentRecord
		ok    bool
	)
	mapLock.Lock()
	defer mapLock.Unlock()
	// unpack the data, update each user we find.
	for _, i := range dl {
		agent, ok = a.Current[i.Symbol]
		if !ok {
			agent = []AgentRecord{}
		}
		rec = AgentRecord{
			Timestamp: time.Now(),
			ShipCount: i.ShipCount,
			Credits:   int(i.Credits),
		}
		agent = append(agent, rec)
		a.Current[i.Symbol] = agent
	}
	return
}

func (a *App) getServerStatus(apiURL string) {
	client := http.Client{
		Timeout: 5 * time.Second, // Set a reasonable timeout
	}

	slog.Error("Looking to update server status")
	fullURL := fmt.Sprintf("%s/", apiURL)

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

	var data ServerStatus
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		slog.Error("error decoding JSON response", "error", err)
		return
	}
	a.updateServerStatus(data)
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

func (a *App) updateServerStatus(data ServerStatus) {
	mapLock.Lock()
	defer mapLock.Unlock()
	if a.Reset != data.ResetDate {
		slog.Info("new reset date")
		a.Reset = data.ResetDate
		a.LastReset = a.Current
		a.Current = make(map[string][]AgentRecord)
	}
	if data.Stats.Accounts == nil {
		a.Accounts = 0
	} else {
		a.Accounts = *data.Stats.Accounts
	}
	a.Agents = data.Stats.Agents
	a.Ships = data.Stats.Ships
}
