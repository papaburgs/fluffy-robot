package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/datastore"
)

// jumpgate status codes:
const (
	jsNoActivity int = 0
	jsActive     int = 1
	jsConst      int = 2
	jsComplete   int = 3
)

// updateJumpgatesFromAgents will loop over the agents we have and find any new jumpgates we haven't seen before,
// this is the first stage of jumpgate tracking, we are making sure each agent has a system
// registered in the jumpgate table. We will update the status to either no activity or active
// based on the agent's credits.
func (c *Collector) updateJumpgatesFromAgents(ctx context.Context, agents []datastore.PublicAgent) error {
	l := c.plog.With("function", "updateJumpgatesFromAgents")
	l.Info("start")

	// read in the jumpgates - this should be updated when the
	l.Debug("scanning existing jumpgates from db")
	existingJGs := datastore.JumpgatesBySystem

	// loop over the agents we got and update where needed
	for _, a := range agents {
		thisSystem := getSystemFromHQ(a.Headquarters)
		thisJG, ok := existingJGs[thisSystem]
		if !ok {
			// if we don't have this in the db, we need to add it to the activeSystems map to check it later.
			jumpgateSymbol, err := c.findJumpgateSymbol(ctx, thisSystem)
			if err != nil {
				slog.Error("failed to find jumpgate symbol", "system", thisSystem, "error", err)
				continue
			}
			thisJG = datastore.JGInfo{
				System:       thisSystem,
				Headquarters: a.Headquarters,
				Jumpgate:     jumpgateSymbol,
			}
		}
		if a.Credits != 175000 && thisJG.Status == jsNoActivity {
			thisJG.Status = jsActive
		}
		existingJGs[thisSystem] = thisJG
	}

	l.Debug("done scan")
	// now write it back to datastore
	jgList := make([]datastore.JGInfo, 1000)
	for _, j := range existingJGs {
		jgList = append(jgList, j)
	}
	datastore.UpdateJumpGates(jgList)

	return nil
}

// updateJumpgates will loop over jumpgates that are marked under construction and
// and get the contruction status.
// this is run more often than the updateJumpgatesActive so we get more accurate construction status
// for jumpgates that are not being working on, we will check them less often
func (c *Collector) updateJumpgates(ctx context.Context) error {
	l := slog.With("function", "updateJumpgates")
	l.Info("start")
	if err := datastore.LoadJumpgates(); err != nil {
		l.Error("did not load any data", "error", err)
	}

	c.currentTimestamp = time.Now().Round(time.Minute).Unix()
	c.apiCalls = 0
	c.ingestStart = time.Now()

	existingJGs := make(map[string]jgInfo)
	jgRows, err := c.db.QueryContext(ctx, `
	    SELECT system, jumpgate
		FROM jumpgates 
		WHERE reset = ? AND status = ?
	`, c.reset, jsConst)
	if err == nil {
		for jgRows.Next() {
			var (
				system, jumpgate string
			)
			if err := jgRows.Scan(&system, &jumpgate); err == nil {
				existingJGs[system] = jgInfo{
					system:   system,
					jumpgate: jumpgate,
					status:   jsConst,
				}
			}
		}
		jgRows.Close()
	}

	var constructions []jgConstruction
	var completions []string

	l.Debug("looking through jumpates being built")
	for system, jg := range existingJGs {

		l.Debug("checking construction status", "jumpgate", jg.jumpgate)
		status, err := c.fetchConstructionStatus(ctx, system, jg.jumpgate)
		if err != nil {
			l.Error("failed to fetch construction status", "jumpgate", jg.jumpgate, "error", err)
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

		constructions = append(constructions, jgConstruction{
			jumpgate: jg.jumpgate,
			fabmat:   fabmat,
			advcct:   advcct,
		})

		// If complete, update jumpgates table
		if status.IsComplete {
			l.Debug("jumpgate construction complete", "jumpgate", jg.jumpgate)
			completions = append(completions, jg.jumpgate)
		} else {
			l.Debug("jumpgate still under construction", "jumpgate", jg.jumpgate, "fabmat", fabmat, "advcct", advcct)
		}
	}
	l.Debug("done scan")

	if len(constructions) == 0 && len(completions) == 0 {
		l.Info("nothing to update")
		return nil
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if len(constructions) > 0 {
		l.Debug("Updating jumpgate constructions")
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO construction (reset, timestamp, jumpgate, fabmat, advcct)
			VALUES (?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, cn := range constructions {
			l.Debug("inserting construction status for jumpgate", "jumpgate", cn.jumpgate, "fabmat", cn.fabmat, "advcct", cn.advcct)
			if _, err := stmt.ExecContext(ctx, c.reset, c.currentTimestamp, cn.jumpgate, cn.fabmat, cn.advcct); err != nil {
				slog.Error("failed to insert construction in batch", "jumpgate", cn.jumpgate, "error", err)
			}
		}
	}

	if len(completions) > 0 {
		l.Debug("updating completions")
		stmt, err := tx.PrepareContext(ctx, `
		UPDATE jumpgates SET status = ?, completetimestamp = ?
		WHERE reset = ? AND system = ?
	    `)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, cp := range completions {
			l.Debug("marking jumpgate as complete", "system", cp)
			if _, err := stmt.ExecContext(ctx, jsComplete, c.currentTimestamp, c.reset, cp); err != nil {
				slog.Error("failed to update jumpgate completion in batch", "jumpgate", cp, "error", err)
			}
		}
	}

	err = tx.Commit()
	if err == nil {
		slog.Info("jumpgate completed", "apiCalls", c.apiCalls, "duration", time.Now().Sub(c.ingestStart))
	}
	return err
}

// updateInactiveJumpgates will loop over jumpgates that are not not being worked on, but have active agent
// this is similar to the main updateJumpgates, but we do not write contrction status if there is no contstruction
// if we find one that has started being worked on, we update the status to construction active
// so they can be tracked more closely in the main updateJumpgates function. This is run less often than the main updateJumpgates function
func (c *Collector) updateInactiveJumpgates(ctx context.Context) error {
	l := slog.With("function", "updateInactiveJumpgates")
	l.Info("start")

	c.currentTimestamp = time.Now().Unix()
	c.apiCalls = 0
	c.ingestStart = time.Now()

	existingJGs := make(map[string]jgInfo)
	jgRows, err := c.db.QueryContext(ctx, `
	    SELECT system, jumpgate
		FROM jumpgates 
		WHERE reset = ? AND status = ?
	`, c.reset, jsActive)
	if err == nil {
		for jgRows.Next() {
			var (
				system, jumpgate string
			)
			if err := jgRows.Scan(&system, &jumpgate); err == nil {
				existingJGs[system] = jgInfo{
					system:   system,
					jumpgate: jumpgate,
					status:   jsConst,
				}
			}
		}
		jgRows.Close()
	}

	var constructions []jgConstruction
	var updateConst []string

	l.Debug("looking through jumpates not being built")
	for system, jg := range existingJGs {
		l.Debug("checking construction status", "jumpgate", jg.jumpgate)
		status, err := c.fetchConstructionStatus(ctx, system, jg.jumpgate)
		if err != nil {
			l.Error("failed to fetch construction status", "jumpgate", jg.jumpgate, "error", err)
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
			constructions = append(constructions, jgConstruction{
				jumpgate: jg.jumpgate,
				fabmat:   fabmat,
				advcct:   advcct,
			})
			updateConst = append(updateConst, system)
		}

	}
	l.Debug("done scan")

	if len(constructions) == 0 && len(updateConst) == 0 {
		l.Info("nothing to update")
		return nil
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if len(constructions) > 0 {
		l.Debug("Updating jumpgate constructions")
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO construction (reset, timestamp, jumpgate, fabmat, advcct)
			VALUES (?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, cn := range constructions {
			l.Debug("inserting construction status for jumpgate", "jumpgate", cn.jumpgate, "fabmat", cn.fabmat, "advcct", cn.advcct)
			if _, err := stmt.ExecContext(ctx, c.reset, c.currentTimestamp, cn.jumpgate, cn.fabmat, cn.advcct); err != nil {
				slog.Error("failed to insert construction in batch", "jumpgate", cn.jumpgate, "error", err)
			}
		}
	}

	if len(updateConst) > 0 {
		l.Debug("updating newly active constructions")
		stmt, err := tx.PrepareContext(ctx, `
		UPDATE jumpgates SET status = ?
		WHERE reset = ? AND system = ?
	    `)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, cp := range updateConst {
			l.Debug("marking jumpgate as underConst", "system", cp)
			if _, err := stmt.ExecContext(ctx, jsConst, c.reset, cp); err != nil {
				slog.Error("failed to update jumpgate completion in batch", "jumpgate", cp, "error", err)
			}
		}
	}

	err = tx.Commit()
	if err == nil {
		slog.Info("jumpgate completed", "apiCalls", c.apiCalls, "duration", time.Now().Sub(c.ingestStart))
	}
	return err
}

// getSystemFromHQ is a helper function that takes in the headquarts, splits on "-", and
// then rejoins the first two items from there. usually the format is X1-[system]-A1
func getSystemFromHQ(hq string) string {
	parts := strings.Split(hq, "-")
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "-" + parts[1]
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

type jgInfo struct {
	jumpgate     string
	system       string
	headquarters string
	status       int
	complete     int64
}

type jgConstruction struct {
	reset     string
	timestamp int64
	jumpgate  string
	fabmat    int
	advcct    int
}
