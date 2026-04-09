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

func (c *Collector) updateJumpgatesFromAgents(ctx context.Context, agents []ds.PublicAgent) error {
	logging.Info("start")

	// logging.Debug("starting to merge agents with existing jumpgates")
	c.currentTimestamp = time.Now().Round(time.Minute).Unix()
	c.apiCalls = 0
	c.ingestStart = time.Now()

	jgs := ds.GetJumpgates(c.currentReset)

	// logging.Debug("starting loop")
	for _, a := range agents {
		// logging.Debug("looking at agent", a.Symbol)
		thisSystem := ds.SystemFromWaypoint(a.Headquarters)
		thisJG, ok := jgs[thisSystem]
		if !ok {
			// logging.Debug("Not found in current jumpgates")
			jumpgateSymbol, err := c.findJumpgateSymbol(ctx, thisSystem)
			if err != nil {
				logging.Error("failed to find jumpgate symbol", thisSystem, err)
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
			// logging.Debug("Marking agent as active")
			thisJG.Status = ds.Active
		}
		jgs[thisSystem] = thisJG
	}

	// logging.Debug("done scan")

	jgList := []ds.JGInfo{}
	for _, j := range jgs {
		jgList = append(jgList, j)
	}
	ds.UpdateJumpGates(jgList)

	logging.Info("Update complete construction complete", "apiCalls", c.apiCalls, "duration", time.Now().Sub(c.ingestStart))
	jgList = nil
	jgs = nil
	return nil
}

func (c *Collector) updateJumpgates(ctx context.Context) error {
	logging.Info("start")

	c.currentTimestamp = time.Now().Round(time.Minute).Unix()
	c.apiCalls = 0
	c.ingestStart = time.Now()

	jgs := ds.GetJumpgatesUnderConst(c.currentReset)

	var constructions []ds.JGConstruction
	var completions []string

	// logging.Debug("looking through jumpates being built")
	for system, jg := range jgs {
		// logging.Debug("checking construction status", jg.Jumpgate)
		status, err := c.fetchConstructionStatus(ctx, system, jg.Jumpgate)
		if err != nil {
			logging.Error("failed to fetch construction status", jg.Jumpgate, err)
			continue
		}

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

		if status.IsComplete {
			// logging.Debug("jumpgate construction complete", jg.Jumpgate)
			completions = append(completions, system)
		}
		// else {
		// logging.Debug("jumpgate still under construction", jg.Jumpgate, "fabmat", fabmat, "advcct", advcct)
		// }
	}
	// logging.Debug("done scan")

	// Add synthetic records for completed jumpgates so charts stay up to date
	completedJgs := ds.GetJumpgatesComplete(c.currentReset)
	for _, jg := range completedJgs {
		constructions = append(constructions, ds.JGConstruction{
			Timestamp: c.currentTimestamp,
			Jumpgate:  jg.Jumpgate,
			Fabmat:    1600,
			Advcct:    400,
		})
	}
	completedJgs = nil

	if len(completions) > 0 {
		ds.MarkJumpgatesComplete(completions, c.currentTimestamp)
	}

	if len(constructions) > 0 {
		ds.AddConstructions(constructions, c.currentTimestamp)
	}
	metrics.CollectorJumpgateUpdates.Add(1)
	metrics.CollectorLastTimestamp.Set(time.Now().Unix())

	logging.Info("Update complete construction complete", "apiCalls", c.apiCalls, "duration", time.Now().Sub(c.ingestStart))
	jgs = nil
	constructions = nil
	completions = nil
	return nil
}

func (c *Collector) updateInactiveJumpgates(ctx context.Context) error {
	logging.Info("start")

	c.currentTimestamp = time.Now().Round(time.Minute).Unix()
	c.apiCalls = 0
	c.ingestStart = time.Now()

	jgs := ds.GetJumpgatesNotStarted(c.currentReset)

	var constructions []ds.JGConstruction
	var updateConst []string

	// logging.Debug("looking through jumpates not being built")
	for system, jg := range jgs {
		// logging.Debug("checking construction status", jg.Jumpgate)
		status, err := c.fetchConstructionStatus(ctx, system, jg.Jumpgate)
		if err != nil {
			logging.Error("failed to fetch construction status", jg.Jumpgate, err)
			continue
		}

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
	// logging.Debug("done scan")

	if len(updateConst) > 0 {
		ds.MarkJumpgatesStarted(updateConst)
	}

	if len(constructions) > 0 {
		ds.AddConstructions(constructions, c.currentTimestamp)
	}
	metrics.CollectorConstructionChecks.Add(1)
	metrics.CollectorLastTimestamp.Set(time.Now().Unix())

	logging.Info("Update under construction complete", "apiCalls", c.apiCalls, "duration", time.Now().Sub(c.ingestStart))
	jgs = nil
	constructions = nil
	updateConst = nil
	return nil
}

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
