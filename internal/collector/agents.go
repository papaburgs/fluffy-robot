package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	ds "github.com/papaburgs/fluffy-robot/internal/datastore"
	"github.com/papaburgs/fluffy-robot/internal/logging"
	"github.com/papaburgs/fluffy-robot/internal/metrics"
)

func (c *Collector) updateStatus(ctx context.Context) error {
	var err error
	// logging.Debug("updating server status")
	c.currentTimestamp = time.Now().Truncate(time.Minute).Unix()
	c.apiCalls = 0
	c.ingestStart = time.Now()

	resp, err := c.doGET(ctx, c.baseURL+"/")
	if err != nil {
		return err
	}

	var status ds.ResponseStatus
	if err := json.Unmarshal(resp.Bytes, &status); err != nil {
		return err
	}
	// logging.Debug("api call done")

	prevReset := c.currentReset
	c.currentReset = ds.Reset(status.ResetDate)
	ds.UpdateReset(c.currentReset)
	c.nextReset = status.ServerResets.Next

	// logging.Debug("processing response")
	err = ds.StoreStats(status)
	if err != nil {
		logging.Error("Error saving stats", err)
	}
	err = ds.StoreLeaderboards(status)
	if err != nil {
		logging.Error("Error saving leaderboards", err)
	}
	// Factions are fetched once per reset (the list is stable within a
	// week): first status of a new game — including fresh boot, when
	// prevReset is "" — triggers it. Errors are logged, never fatal.
	if prevReset != c.currentReset {
		if ferr := c.updateFactions(ctx); ferr != nil {
			logging.Error("Error fetching factions for new reset", ferr)
		}
	}
	logging.Info("status ingestion completed", "apiCalls", c.apiCalls, "duration", time.Now().Sub(c.ingestStart))
	return nil
}

func (c *Collector) updateAgents(ctx context.Context) error {
	// logging.Debug("updating agents")

	var allAgents []ds.PublicAgent
	page := 1
	perPage := 20

	for {
		// logging.Debug("fetching agents page", "page", page)
		url := fmt.Sprintf("%s/agents?limit=%d&page=%d", c.baseURL, perPage, page)
		resp, err := c.doGET(ctx, url)
		if err != nil {
			return err
		}

		var data ResponseAgents
		if err := json.Unmarshal(resp.Bytes, &data); err != nil {
			return err
		}

		allAgents = append(allAgents, data.Data...)

		if page*perPage >= data.Meta.Total {
			break
		}
		page++
	}

	if len(allAgents) == 0 {
		return nil
	}

	if err := ds.StoreAgents(allAgents, c.currentTimestamp); err != nil {
		logging.Error("agent cycle write failed", err)
	}

	err := c.updateJumpgatesFromAgents(ctx, allAgents)
	if err != nil {
		return err
	}

	metrics.CollectorAgentUpdates.Add(1)
	metrics.CollectorLastTimestamp.Set(time.Now().Unix())
	logging.Info("agent ingestion completed", "apiCalls", c.apiCalls, "duration", time.Now().Sub(c.ingestStart))
	allAgents = nil
	return nil
}

func (c *Collector) updateFactions(ctx context.Context) error {
	// logging.Debug("updating factions")

	allFactions := []ds.Faction{}
	page := 1
	perPage := 20

	for {
		// logging.Debug("fetching factions page", "page", page)
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

	if err := ds.StoreFactions(allFactions); err != nil {
		logging.Error("faction write failed", err)
	}

	logging.Info("faction", "apiCalls", c.apiCalls, "duration", time.Now().Sub(c.ingestStart))
	allFactions = nil
	return nil
}
