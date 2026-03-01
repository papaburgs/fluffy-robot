package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/types"
)

func (c *Collector) updateStatusAgents(ctx context.Context) error {
	l := slog.With("function", "updateStatusAgents")
	l.Info("starting data ingestion")

	// set the current timestamp, rounded to the current minute.
	c.currentTimestamp = time.Now().Truncate(time.Minute).Unix()
	c.apiCalls = 0
	c.ingestStart = time.Now()
	if err := c.updateStatus(ctx); err != nil {
		l.Error("failed to update status", "error", err)
		return err
	}

	if err := c.updateAgents(ctx); err != nil {
		l.Error("failed to update agents", "error", err)
		return err
	}
	slog.Info("data ingestion completed", "apiCalls", c.apiCalls, "duration", time.Now().Sub(c.ingestStart))
	return nil
}

func (c *Collector) updateStatus(ctx context.Context) error {
	l := slog.With("function", "updateStatus")
	l.Debug("updating server status")

	resp, err := c.doGET(ctx, c.baseURL+"/")
	if err != nil {
		return err
	}

	var status ResponseStatus
	if err := json.Unmarshal(resp.Bytes, &status); err != nil {
		return err
	}

	// set this locally as we use it often
	c.reset = status.ResetDate
	// this will be checked after and timers will be adjusted
	c.nextReset = status.ServerResets.Next

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Update stats table
	_, err = tx.ExecContext(ctx, `INSERT INTO stats (reset, marketUpdate, agents, accounts, ships, systems, waypoints, status, version, nextReset, lats, lsts) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(reset) DO UPDATE SET marketUpdate = excluded.marketUpdate, agents = excluded.agents, accounts = excluded.accounts, ships = excluded.ships, systems = excluded.systems, waypoints = excluded.waypoints, status = excluded.status, version = excluded.version, nextReset = excluded.nextReset, lats = excluded.lats, lsts = excluded.lsts `, status.ResetDate, status.Health.LastMarketUpdate, status.Stats.Agents, status.Stats.Accounts, status.Stats.Ships, status.Stats.Systems, status.Stats.Waypoints, status.Status, status.Version, status.ServerResets.Next, c.currentTimestamp, 0)
	if err != nil {
		return fmt.Errorf("failed to update stats table: %w", err)
	}

	l.Debug("processing leaderboards")
	chartsList := []string{}
	for _, x := range status.Leaderboards.MostSubmittedCharts {
		chartsList = append(chartsList, fmt.Sprintf("%s,%d", x.AgentSymbol, x.ChartCount))
	}
	creditsList := []string{}
	for _, x := range status.Leaderboards.MostCredits {
		creditsList = append(creditsList, fmt.Sprintf("%s,%d", x.AgentSymbol, x.Credits))
	}

	// Update leaderboard table
	_, err = tx.ExecContext(ctx, `
		INSERT INTO leaderboard (reset, credits, charts)
		VALUES (?, ?, ?)
		ON CONFLICT(reset) DO UPDATE SET credits = excluded.credits, charts = excluded.charts
	`,
		c.reset,
		strings.Join(creditsList, "|"),
		strings.Join(chartsList, "|"),
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (c *Collector) updateAgents(ctx context.Context) error {
	l := slog.With("function", "updateAgents")
	l.Debug("updating agents")

	var allAgents []types.PublicAgent
	page := 1
	perPage := 20

	for {
		l.Debug("fetching agents page", "page", page)
		url := fmt.Sprintf("%s/agents?limit=%d&page=%d", c.baseURL, perPage, page)
		resp, err := c.doGET(ctx, url)
		if err != nil {
			return err
		}

		var data ResponseAgents
		if err := json.Unmarshal(resp.Bytes, &data); err != nil {
			return err
		}

		for _, agent := range data.Data {
			addAgent := true
			for _, re := range c.filterRegexes {
				if re.MatchString(agent.Symbol) {
					l.Info("skipping agent due to filter", "symbol", agent.Symbol, "pattern", re.String())
					addAgent = false
					continue
				}
			}
			if addAgent {
				allAgents = append(allAgents, agent)
			}
		}

		if page*perPage >= data.Meta.Total {
			break
		}
		page++
	}

	if len(allAgents) == 0 {
		return nil
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// we will be updating every agent in this reset on every call
	// if they don't exist, we are adding everything,
	// if they do exist we are just updating the credits,
	// we use credits to see if the agent is active and for sorting
	agentstmt, err := tx.PrepareContext(ctx, `
		INSERT INTO agents (reset, symbol, credits, faction, headquarters)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(reset, symbol) DO UPDATE SET 
		 credits = excluded.credits
	`)
	statusstmt, err := tx.PrepareContext(ctx, `
	    INSERT INTO agentstatus (reset, symbol, timestamp, credits, ships)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer agentstmt.Close()
	defer statusstmt.Close()

	for _, agent := range allAgents {
		_, err = agentstmt.ExecContext(ctx,
			c.reset,
			agent.Symbol,
			agent.Credits,
			agent.StartingFaction,
			agent.Headquarters,
		)
		if err != nil {
			slog.Error("error to add agent call to batch", "error", err, "symbol", agent.Symbol)
		}
		_, err = statusstmt.ExecContext(ctx,
			c.reset,
			agent.Symbol,
			c.currentTimestamp,
			agent.Credits,
			agent.ShipCount,
		)
		if err != nil {
			slog.Error("error to add agent status to batch", "error", err, "symbol", agent.Symbol)
		}
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	err = c.updateJumpgatesFromAgents(ctx, allAgents)
	return err
}
