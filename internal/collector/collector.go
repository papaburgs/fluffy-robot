package collector

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/gate"
	"github.com/papaburgs/fluffy-robot/internal/types"
)

type Collector struct {
	db                     *sql.DB
	baseURL                string
	gate                   *gate.Gate
	reset                  string
	resetChan              chan string
	jumpGateListUpdateChan chan types.JumpGateAgentListStruct
}

func New(db *sql.DB, baseURL string, resetChan chan string, jumpGateListUpdateChan chan types.JumpGateAgentListStruct) *Collector {
	return &Collector{
		db:                     db,
		baseURL:                baseURL,
		gate:                   gate.New(2, 25), // Based on agentcollector logic
		resetChan:              resetChan,
		jumpGateListUpdateChan: jumpGateListUpdateChan,
	}
}

func (c *Collector) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on start
	c.Ingest(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.Ingest(ctx)
		}
	}
}

func (c *Collector) Ingest(ctx context.Context) {
	slog.Info("starting data ingestion")

	// 1. Call status/health endpoint
	if err := c.updateStatus(ctx); err != nil {
		slog.Error("failed to update status", "error", err)
		return
	}

	// 2. Call agents endpoint
	if err := c.updateAgents(ctx); err != nil {
		slog.Error("failed to update agents", "error", err)
	}

	// 3. Handle jumpgates and construction
	if err := c.updateJumpgates(ctx); err != nil {
		slog.Error("failed to update jumpgates", "error", err)
	}

	slog.Info("data ingestion completed")
}

func (c *Collector) updateJumpgates(ctx context.Context) error {
	slog.Debug("updating jumpgates")

	// Get agents with credits != 175000 from DB
	rows, err := c.db.QueryContext(ctx, "SELECT symbol, headquarters, credits FROM agents WHERE reset = ?", c.reset)
	if err != nil {
		return err
	}
	defer rows.Close()

	activeSystems := make(map[string]string) // system -> headquarters
	for rows.Next() {
		var symbol, hq string
		var credits int64
		if err := rows.Scan(&symbol, &hq, &credits); err != nil {
			continue
		}
		if credits != 175000 {
			// System is HQ minus last 3 chars
			if len(hq) > 3 {
				system := hq[:len(hq)-3]
				activeSystems[system] = hq
			}
		}
	}

	for system, hq := range activeSystems {
		// Check if jumpgate for this system is already in DB
		var jumpgateSymbol string
		var complete int64
		err := c.db.QueryRowContext(ctx, "SELECT jumpgate, complete FROM jumpgates WHERE reset = ? AND system = ?", c.reset, system).Scan(&jumpgateSymbol, &complete)
		
		if err == sql.ErrNoRows {
			// Need to find jumpgate symbol for this system
			slog.Debug("finding jumpgate for system", "system", system)
			jumpgateSymbol, err = c.findJumpgateSymbol(ctx, system)
			if err != nil {
				slog.Error("failed to find jumpgate symbol", "system", system, "error", err)
				continue
			}

			// Insert into jumpgates table
			_, err = c.db.ExecContext(ctx, `
				INSERT INTO jumpgates (reset, system, headquarters, jumpgate, complete, activeagent)
				VALUES (?, ?, ?, ?, ?, ?)
			`, c.reset, system, hq, jumpgateSymbol, 0, true)
			if err != nil {
				slog.Error("failed to insert jumpgate", "system", system, "error", err)
				continue
			}
		} else if err != nil {
			slog.Error("failed to query jumpgate", "system", system, "error", err)
			continue
		}

		// If jumpgate is not complete, check construction status
		if complete == 0 {
			slog.Debug("checking construction status", "system", system, "jumpgate", jumpgateSymbol)
			status, err := c.fetchConstructionStatus(ctx, system, jumpgateSymbol)
			if err != nil {
				slog.Error("failed to fetch construction status", "jumpgate", jumpgateSymbol, "error", err)
				continue
			}

			// Update construction table
			var fabmat, advcct int
			for _, m := range status.Materials {
				if m.TradeSymbol == "FAB_MATS" {
					fabmat = m.Fulfilled
				} else if m.TradeSymbol == "ADVANCED_CIRCUITRY" {
					advcct = m.Fulfilled
				}
			}

			_, err = c.db.ExecContext(ctx, `
				INSERT INTO construction (reset, timestamp, jumpgate, fabmat, advcct)
				VALUES (?, ?, ?, ?, ?)
			`, c.reset, time.Now().Unix(), jumpgateSymbol, fabmat, advcct)
			if err != nil {
				slog.Error("failed to insert construction record", "jumpgate", jumpgateSymbol, "error", err)
			}

			// If complete, update jumpgates table
			if status.IsComplete {
				_, err = c.db.ExecContext(ctx, "UPDATE jumpgates SET complete = ? WHERE reset = ? AND jumpgate = ?", time.Now().Unix(), c.reset, jumpgateSymbol)
				if err != nil {
					slog.Error("failed to update jumpgate completion", "jumpgate", jumpgateSymbol, "error", err)
				}
			}
		}
	}

	return nil
}

func (c *Collector) findJumpgateSymbol(ctx context.Context, systemSymbol string) (string, error) {
	url := fmt.Sprintf("%s/systems/%s", c.baseURL, systemSymbol)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	c.gate.Latch(ctx)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("api returned %d", resp.StatusCode)
	}

	var systemResponse struct {
		Data struct {
			Waypoints []struct {
				Symbol string `json:"symbol"`
				Type   string `json:"type"`
			} `json:"waypoints"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&systemResponse); err != nil {
		return "", err
	}

	for _, w := range systemResponse.Data.Waypoints {
		if w.Type == "JUMP_GATE" {
			return w.Symbol, nil
		}
	}

	return "", fmt.Errorf("no jumpgate found in system %s", systemSymbol)
}

type ConstructionMaterial struct {
	TradeSymbol string `json:"tradeSymbol"`
	Required    int    `json:"required"`
	Fulfilled   int    `json:"fulfilled"`
}

type ConstructionStatus struct {
	Symbol     string                 `json:"symbol"`
	Materials  []ConstructionMaterial `json:"materials"`
	IsComplete bool                   `json:"isComplete"`
}

func (c *Collector) fetchConstructionStatus(ctx context.Context, systemSymbol, jumpgateSymbol string) (ConstructionStatus, error) {
	url := fmt.Sprintf("%s/systems/%s/waypoints/%s/construction", c.baseURL, systemSymbol, jumpgateSymbol)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return ConstructionStatus{}, err
	}

	c.gate.Latch(ctx)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ConstructionStatus{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ConstructionStatus{}, fmt.Errorf("api returned %d", resp.StatusCode)
	}

	var response struct {
		Data ConstructionStatus `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return ConstructionStatus{}, err
	}

	return response.Data, nil
}

func (c *Collector) updateStatus(ctx context.Context) error {
	slog.Debug("updating server status")
	
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/", nil)
	if err != nil {
		return err
	}

	c.gate.Latch(ctx)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status endpoint returned %d", resp.StatusCode)
	}

	var status ResponseStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return err
	}

	c.reset = status.ResetDate
	if c.resetChan != nil {
		c.resetChan <- c.reset
	}

	// Update stats table
	_, err = c.db.ExecContext(ctx, `
		INSERT INTO stats (reset, marketUpdate, agents, accounts, ships, systems, waypoints, status, version, nextReset)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(reset) DO UPDATE SET
			marketUpdate = excluded.marketUpdate,
			agents = excluded.agents,
			accounts = excluded.accounts,
			ships = excluded.ships,
			systems = excluded.systems,
			waypoints = excluded.waypoints,
			status = excluded.status,
			version = excluded.version,
			nextReset = excluded.nextReset
	`,
		status.ResetDate,
		time.Now(), // Using now for marketUpdate as placeholder or parse from response if available
		status.Stats.Agents,
		status.Stats.Accounts,
		status.Stats.Ships,
		status.Stats.Systems,
		status.Stats.Waypoints,
		status.Status,
		status.Version,
		status.ServerResets.Next,
	)
	if err != nil {
		return fmt.Errorf("failed to update stats table: %w", err)
	}

	// Update leaderboard table
	timestamp := time.Now().Unix()
	for _, l := range status.Leaderboards.MostCredits {
		_, err = c.db.ExecContext(ctx, `
			INSERT INTO leaderboard (timestamp, reset, count, symbol, type)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(timestamp, symbol, type) DO NOTHING
		`, timestamp, c.reset, l.Credits, l.AgentSymbol, "credits")
		if err != nil {
			slog.Error("failed to insert credits leaderboard", "error", err, "symbol", l.AgentSymbol)
		}
	}

	for _, l := range status.Leaderboards.MostSubmittedCharts {
		_, err = c.db.ExecContext(ctx, `
			INSERT INTO leaderboard (timestamp, reset, count, symbol, type)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(timestamp, symbol, type) DO NOTHING
		`, timestamp, c.reset, l.ChartCount, l.AgentSymbol, "charts")
		if err != nil {
			slog.Error("failed to insert charts leaderboard", "error", err, "symbol", l.AgentSymbol)
		}
	}

	return nil
}

func (c *Collector) updateAgents(ctx context.Context) error {
	slog.Debug("updating agents")
	
	page := 1
	perPage := 20
	timestamp := time.Now().Unix()

	for {
		url := fmt.Sprintf("%s/agents?limit=%d&page=%d", c.baseURL, perPage, page)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return err
		}

		c.gate.Latch(ctx)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("agents endpoint returned %d", resp.StatusCode)
		}

		var data ResponseAgents
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			resp.Body.Close()
			return err
		}
		resp.Body.Close()

		for _, agent := range data.Data {
			_, err = c.db.ExecContext(ctx, `
				INSERT INTO agents (timestamp, reset, symbol, ships, faction, credits, headquarters)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(symbol) DO UPDATE SET
					timestamp = excluded.timestamp,
					reset = excluded.reset,
					ships = excluded.ships,
					faction = excluded.faction,
					credits = excluded.credits,
					headquarters = excluded.headquarters
			`, timestamp, c.reset, agent.Symbol, agent.ShipCount, agent.StartingFaction, agent.Credits, agent.Headquarters)
			if err != nil {
				slog.Error("failed to update agent", "error", err, "symbol", agent.Symbol)
			}

			_, err = c.db.ExecContext(ctx, `
				INSERT INTO agent_history (timestamp, reset, symbol, ships, credits)
				VALUES (?, ?, ?, ?, ?)
				ON CONFLICT(timestamp, symbol) DO NOTHING
			`, timestamp, c.reset, agent.Symbol, agent.ShipCount, agent.Credits)
			if err != nil {
				slog.Error("failed to insert agent history", "error", err, "symbol", agent.Symbol)
			}
		}

		if page*perPage >= data.Meta.Total {
			break
		}
		page++
	}

	return nil
}

// Response structures copied/adapted from agentcollector

type ResponseStatus struct {
	Leaderboards struct {
		MostCredits []struct {
			AgentSymbol string `json:"agentSymbol"`
			Credits     int64  `json:"credits"`
		} `json:"mostCredits"`
		MostSubmittedCharts []struct {
			AgentSymbol string `json:"agentSymbol"`
			ChartCount  int    `json:"chartCount"`
		} `json:"mostSubmittedCharts"`
	} `json:"leaderboards"`
	ResetDate    string `json:"resetDate"`
	ServerResets struct {
		Frequency string    `json:"frequency"`
		Next      time.Time `json:"next"`
	} `json:"serverResets"`
	Stats struct {
		Accounts *int `json:"accounts,omitempty"`
		Agents   int  `json:"agents"`
		Ships    int  `json:"ships"`
		Systems  int  `json:"systems"`
		Waypoints int `json:"waypoints"`
	} `json:"stats"`
	Status  string `json:"status"`
	Version string `json:"version"`
}

type ResponseAgents struct {
	Data []types.PublicAgent `json:"data"`
	Meta Meta                `json:"meta"`
}

type Meta struct {
	Limit int `json:"limit"`
	Page  int `json:"page"`
	Total int `json:"total"`
}
