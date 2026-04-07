package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/datastore"
	ds "github.com/papaburgs/fluffy-robot/internal/datastore"
	"github.com/papaburgs/fluffy-robot/internal/metrics"
)

func (c *Collector) updateStatus(ctx context.Context) error {
	var err error
	l := c.plog.With("function", "updateStatus")
	l.Debug("updating server status")
	c.currentTimestamp = time.Now().Truncate(time.Minute).Unix()
	c.apiCalls = 0
	c.ingestStart = time.Now()

	resp, err := c.doGET(ctx, c.baseURL+"/")
	if err != nil {
		return err
	}

	var status datastore.ResponseStatus
	if err := json.Unmarshal(resp.Bytes, &status); err != nil {
		return err
	}
	l.Debug("api call done")

	// set this locally as we use it often
	c.currentReset = ds.Reset(status.ResetDate)
	datastore.UpdateReset(c.currentReset)
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
	l.Info("status ingestion completed", "apiCalls", c.apiCalls, "duration", time.Now().Sub(c.ingestStart))
	return nil
}

func (c *Collector) updateAgents(ctx context.Context) error {
	l := slog.With("function", "updateAgents")
	l.Debug("updating agents")

	var allAgents []datastore.PublicAgent
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

	// we will be updating every agent in this reset on every call
	// if they don't exist, we are adding everything,
	// if they do exist we are just updating the credits,
	// we use credits to see if the agent is active and for sorting
	datastore.StoreAgents(allAgents, c.currentTimestamp)

	err := c.updateJumpgatesFromAgents(ctx, allAgents)
	if err != nil {
		return err
	}

	l.Info("agent ingestion completed", "apiCalls", c.apiCalls, "duration", time.Now().Sub(c.ingestStart))
	metrics.CollectorAgentUpdates.Add(1)
	metrics.CollectorLastTimestamp.Set(time.Now().Unix())
	return nil
}

func (c *Collector) updateFactionss(ctx context.Context) error {
	l := slog.With("function", "updateFactions")
	l.Debug("updating factions")

	var allFactions = []datastore.Faction{}
	page := 1
	perPage := 20

	for {
		l.Debug("fetching factions page", "page", page)
		url := fmt.Sprintf("%s/factions?limit=%d&page=%d", c.baseURL, perPage, page)
		resp, err := c.doGET(ctx, url)
		if err != nil {
			return err
		}

		var data ResponseFactions
		if err := json.Unmarshal(resp.Bytes, &data); err != nil {
			return err
		}

		allFactions = append(allFactions, data.Data...)

		if page*perPage >= data.Meta.Total {
			break
		}
		page++
	}

	datastore.StoreFactions(allFactions)

	l.Info("faction", "apiCalls", c.apiCalls, "duration", time.Now().Sub(c.ingestStart))
	return nil
}
