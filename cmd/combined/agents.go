package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/datastore"
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

	// if err := c.updateAgents(ctx); err != nil {
	// 	l.Error("failed to update agents", "error", err)
	// 	return err
	// }
	slog.Info("data ingestion completed", "apiCalls", c.apiCalls, "duration", time.Now().Sub(c.ingestStart))
	return nil
}

func (c *Collector) updateStatus(ctx context.Context) error {
	var err error
	l := slog.With("function", "updateStatus")
	l.Debug("updating server status")

	resp, err := c.doGET(ctx, c.baseURL+"/")
	if err != nil {
		return err
	}

	var status datastore.ResponseStatus
	if err := json.Unmarshal(resp.Bytes, &status); err != nil {
		return err
	}

	// set this locally as we use it often
	c.reset = status.ResetDate
	// this will be checked after and timers will be adjusted
	c.nextReset = status.ServerResets.Next

	l.Debug("processing response")
	err = datastore.StoreStats(status)
	if err != nil {
		l.Error("Error saving stats", "error", err)
	}
	err = datastore.StoreLeaderboards(status)
	if err != nil {
		l.Error("Error saving leaderboards", "error", err)
	}
	return nil
}

// func (c *Collector) updateAgents(ctx context.Context) error {
// 	l := slog.With("function", "updateAgents")
// 	l.Debug("updating agents")
//
// 	var allAgents []types.PublicAgent
// 	page := 1
// 	perPage := 20
//
// 	for {
// 		l.Debug("fetching agents page", "page", page)
// 		url := fmt.Sprintf("%s/agents?limit=%d&page=%d", c.baseURL, perPage, page)
// 		resp, err := c.doGET(ctx, url)
// 		if err != nil {
// 			return err
// 		}
//
// 		var data ResponseAgents
// 		if err := json.Unmarshal(resp.Bytes, &data); err != nil {
// 			return err
// 		}
//
// 		for _, agent := range data.Data {
// 			addAgent := true
// 			for _, re := range c.filterRegexes {
// 				if re.MatchString(agent.Symbol) {
// 					l.Info("skipping agent due to filter", "symbol", agent.Symbol, "pattern", re.String())
// 					addAgent = false
// 					continue
// 				}
// 			}
// 			if addAgent {
// 				allAgents = append(allAgents, agent)
// 			}
// 		}
//
// 		if page*perPage >= data.Meta.Total {
// 			break
// 		}
// 		page++
// 	}
//
// 	if len(allAgents) == 0 {
// 		return nil
// 	}
//
// 	tx, err := c.db.BeginTx(ctx, nil)
// 	if err != nil {
// 		return err
// 	}
// 	defer tx.Rollback()
//
// 	// we will be updating every agent in this reset on every call
// 	// if they don't exist, we are adding everything,
// 	// if they do exist we are just updating the credits,
// 	// we use credits to see if the agent is active and for sorting
// 	agentstmt, err := tx.PrepareContext(ctx, `
// 		INSERT INTO agents (reset, symbol, credits, faction, headquarters, system)
// 		VALUES (?, ?, ?, ?, ?, ?)
// 		ON CONFLICT(reset, symbol) DO UPDATE SET
// 		 credits = excluded.credits,
// 		 system = excluded.system
//
// 	`)
// 	statusstmt, err := tx.PrepareContext(ctx, `
// 	    INSERT INTO agentstatus (reset, symbol, timestamp, credits, ships)
// 		VALUES (?, ?, ?, ?, ?)
// 	`)
// 	if err != nil {
// 		return err
// 	}
// 	defer agentstmt.Close()
// 	defer statusstmt.Close()
//
// 	for _, agent := range allAgents {
// 		_, err = agentstmt.ExecContext(ctx,
// 			c.reset,
// 			agent.Symbol,
// 			agent.Credits,
// 			agent.StartingFaction,
// 			agent.Headquarters,
// 			getSystemFromHQ(agent.Headquarters),
// 		)
// 		if err != nil {
// 			slog.Error("error to add agent call to batch", "error", err, "symbol", agent.Symbol)
// 		}
// 		_, err = statusstmt.ExecContext(ctx,
// 			c.reset,
// 			agent.Symbol,
// 			c.currentTimestamp,
// 			agent.Credits,
// 			agent.ShipCount,
// 		)
// 		if err != nil {
// 			slog.Error("error to add agent status to batch", "error", err, "symbol", agent.Symbol)
// 		}
// 	}
// 	err = tx.Commit()
// 	if err != nil {
// 		return err
// 	}
// 	err = c.updateJumpgatesFromAgents(ctx, allAgents)
// 	return err
// }
