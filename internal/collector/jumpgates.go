package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	ds "github.com/papaburgs/fluffy-robot/internal/datastore"
	"github.com/papaburgs/fluffy-robot/internal/metrics"
)

// updateJumpgatesFromAgents will loop over the agents we have and find any new jumpgates we haven't seen before,
// this is the first stage of jumpgate tracking, we are making sure each agent has a system
// registered in the jumpgate table. We will update the status to either no activity or active
// based on the agent's credits.
func (c *Collector) updateJumpgatesFromAgents(ctx context.Context, agents []ds.PublicAgent) error {
	l := c.plog.With("function", "updateJumpgatesFromAgents")
	l.Info("start")

	l.Debug("starting to merge agents with existing jumpgates")
	c.currentTimestamp = time.Now().Round(time.Minute).Unix()
	c.apiCalls = 0
	c.ingestStart = time.Now()

	// get a copy of the jumpgates
	jgs := ds.GetJumpgates(c.currentReset, time.Duration(5*time.Second))

	// loop over the agents we got and update where needed
	l.Debug("starting loop")
	for _, a := range agents {
		l.Debug("looking at agent", "agent", a.Symbol)
		thisSystem := ds.SystemFromWaypoint(a.Headquarters)
		thisJG, ok := jgs[thisSystem]
		if !ok {
			l.Debug("Not found in current jumpgates")
			// if we don't have this in the db, we need to add it to the activeSystems map to check it later.
			jumpgateSymbol, err := c.findJumpgateSymbol(ctx, thisSystem)
			if err != nil {
				slog.Error("failed to find jumpgate symbol", "system", thisSystem, "error", err)
				continue
			}
			thisJG = ds.JGInfo{
				System:       thisSystem,
				Headquarters: a.Headquarters,
				Jumpgate:     jumpgateSymbol,
				Status:       ds.NoActivity,
			}
		}
		if a.Credits != 175000 && thisJG.Status == ds.NoActivity {
			l.Debug("Marking agent as active")
			thisJG.Status = ds.Active
		}
		jgs[thisSystem] = thisJG
	}

	l.Debug("done scan")

	// now write it back to ds
	jgList := []ds.JGInfo{}
	for _, j := range jgs {
		jgList = append(jgList, j)
	}
	ds.UpdateJumpGates(jgList)

	l.Info("Update complete construction complete",
		"apicalls", c.apiCalls,
		"duration", time.Now().Sub(c.ingestStart))

	return nil
}

// updateJumpgates will loop over jumpgates that are marked under construction and
// and get the contruction status.
// this is run more often than the updateJumpgatesActive so we get more accurate construction status
// for jumpgates that are not being working on, we will check them less often
func (c *Collector) updateJumpgates(ctx context.Context) error {
	l := c.plog.With("function", "updateJumpgates")
	l.Info("start")

	c.currentTimestamp = time.Now().Round(time.Minute).Unix()
	c.apiCalls = 0
	c.ingestStart = time.Now()

	jgs := ds.GetJumpgatesUnderConst(c.currentReset)

	var constructions []ds.JGConstruction
	var completions []string

	l.Debug("looking through jumpates being built")
	for system, jg := range jgs {
		l.Debug("checking construction status", "jumpgate", jg.Jumpgate)
		status, err := c.fetchConstructionStatus(ctx, system, jg.Jumpgate)
		if err != nil {
			l.Error("failed to fetch construction status", "jumpgate", jg.Jumpgate, "error", err)
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

		constructions = append(constructions, ds.JGConstruction{
			Timestamp: c.currentTimestamp,
			Jumpgate:  jg.Jumpgate,
			Fabmat:    fabmat,
			Advcct:    advcct,
		})

		// If complete, update jumpgates table
		if status.IsComplete {
			l.Debug("jumpgate construction complete", "jumpgate", jg.Jumpgate)
			completions = append(completions, system)
		} else {
			l.Debug("jumpgate still under construction", "jumpgate", jg.Jumpgate, "fabmat", fabmat, "advcct", advcct)
		}
	}
	l.Debug("done scan")

	if len(completions) > 0 {
		ds.MarkJumpgatesComplete(completions, c.currentTimestamp)
	}

	if len(constructions) > 0 {
		ds.AddConstructions(constructions, c.currentTimestamp)
	}
	metrics.CollectorJumpgateUpdates.Add(1)
	metrics.CollectorLastTimestamp.Set(time.Now().Unix())

	l.Info("Update complete construction complete",
		"apicalls", c.apiCalls,
		"duration", time.Now().Sub(c.ingestStart))
	return nil
}

// updateInactiveJumpgates will loop over jumpgates that are not not being worked on, but have active agent
// this is similar to the main updateJumpgates
// if we find one that has started being worked on, we update the status to construction active and make a small update of construction status
// so they can be tracked more closely in the main updateJumpgates function. This is run less often than the main updateJumpgates function
func (c *Collector) updateInactiveJumpgates(ctx context.Context) error {
	l := c.plog.With("function", "updateJumpgates")
	l.Info("start")

	c.currentTimestamp = time.Now().Round(time.Minute).Unix()
	c.apiCalls = 0
	c.ingestStart = time.Now()

	jgs := ds.GetJumpgatesNotStarted(c.currentReset)

	var constructions []ds.JGConstruction
	var updateConst []string

	l.Debug("looking through jumpates not being built")
	for system, jg := range jgs {
		l.Debug("checking construction status", "jumpgate", jg.Jumpgate)
		status, err := c.fetchConstructionStatus(ctx, system, jg.Jumpgate)
		if err != nil {
			l.Error("failed to fetch construction status", "jumpgate", jg.Jumpgate, "error", err)
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
		if fabmat > 0 || advcct > 0 {
			constructions = append(constructions, ds.JGConstruction{
				Timestamp: c.currentTimestamp,
				Jumpgate:  jg.Jumpgate,
				Fabmat:    fabmat,
				Advcct:    advcct,
			})
			updateConst = append(updateConst, system)
		}
	}
	l.Debug("done scan")

	if len(updateConst) > 0 {
		ds.MarkJumpgatesStarted(updateConst)
	}

	if len(constructions) > 0 {
		ds.AddConstructions(constructions, c.currentTimestamp)
	}
	metrics.CollectorConstructionChecks.Add(1)
	metrics.CollectorLastTimestamp.Set(time.Now().Unix())

	l.Info("Update under construction complete",
		"apicalls", c.apiCalls,
		"duration", time.Now().Sub(c.ingestStart))
	return nil
}

// findJumpgateSymbol searche a system and returns the symbol of the jumpgate
func (c *Collector) findJumpgateSymbol(ctx context.Context, systemSymbol string) (string, error) {
	url := fmt.Sprintf("%s/systems/%s", c.baseURL, systemSymbol)
	resp, err := c.doGET(ctx, url)
	if err != nil {
		return "", err
	}

	var systemResponse struct {
		Data struct {
			Waypoints []struct {
				Symbol string `json:"symbol"`
				Type   string `json:"type"`
			} `json:"waypoints"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Bytes, &systemResponse); err != nil {
		return "", err
	}

	for _, w := range systemResponse.Data.Waypoints {
		if w.Type == "JUMP_GATE" {
			return w.Symbol, nil
		}
	}

	return "", fmt.Errorf("no jumpgate found in system %s", systemSymbol)
}

// fetchConstructionStatus calls the api and get the construction status
func (c *Collector) fetchConstructionStatus(ctx context.Context, systemSymbol, jumpgateSymbol string) (ConstructionStatus, error) {
	url := fmt.Sprintf("%s/systems/%s/waypoints/%s/construction", c.baseURL, systemSymbol, jumpgateSymbol)
	resp, err := c.doGET(ctx, url)
	if err != nil {
		return ConstructionStatus{}, err
	}

	var response struct {
		Data ConstructionStatus `json:"data"`
	}
	if err := json.Unmarshal(resp.Bytes, &response); err != nil {
		return ConstructionStatus{}, err
	}

	return response.Data, nil
}
