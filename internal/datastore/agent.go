package datastore

import (
	"encoding/gob"
	"fmt"
	"time"
)

// Public agent is the type that is returned from the api
type PublicAgent struct {
	Credits         int64  `json:"credits"`
	Headquarters    string `json:"headquarters"`
	ShipCount       int    `json:"shipCount"`
	StartingFaction string `json:"startingFaction"`
	Symbol          string `json:"symbol"`
}

type TimedAgentRecord struct {
	Timestamp time.Time
	ShipCount int
	Credits   int
}

func StoreAgents(apiAgents []PublicAgent, now int64) {
	l := plog.With("function", "LoadAgents")
	l.Debug("Writing Agents")
	var (
		agentList  = []Agent{}
		statusList = []AgentStatus{}
	)
	for _, a := range apiAgents {
		ag := Agent{
			Symbol:       a.Symbol,
			Credits:      int64(a.Credits),
			Faction:      a.StartingFaction,
			Headquarters: a.Headquarters,
			System:       SystemFromWaypoint(a.Headquarters),
		}
		agentList = append(agentList, ag)

		as := AgentStatus{
			Symbol:    a.Symbol,
			Timestamp: now,
			Credits:   int64(a.Credits),
			Ships:     int64(a.ShipCount),
		}
		statusList = append(statusList, as)
	}
	writeData("agents", 0, agentList)
	writeData("agentsStatus", now, statusList)
}

// LoadAgents makes the Agents map
func LoadAgents(r string) error {
	l := plog.With("function", "LoadAgents")
	zeroTimer.Reset(cacheLifetime)
	// use readdata to get back a map of filename to byte buffers
	// NB use the . on the end so we don't get agentStatus files
	m, err := readData("agents.")
	if err != nil {
		l.Error("Failed to load agents", "error", err)
		return err
	}

	if len(m) != 1 {
		l.Error("should only get one result", "count", len(m))
		return fmt.Errorf("invalid read")
	}

	for _, b := range m {
		var v []Agent
		// make a new decoder on the buffer, which is a Reader
		gobDec := gob.NewDecoder(b)

		// try to decode the gob into an array of Agent, which is how its written
		if err := gobDec.Decode(&v); err != nil {
			l.Error("error decoding gob", "error", err)
			return err
		}
		for _, a := range v {
			Agents[a.Symbol] = a
		}
	}
	l.Debug("loaded agents", "count", len(Agents))
	return nil
}

// LoadAgentHistory makes the agentname to credit and ship count maps
func LoadAgentHistory(r string) error {
	l := plog.With("function", "LoadAgentHistory")
	// use readdata to get back a map of filename to byte buffers
	// NB use the dash to make it more unique
	m, err := readData("agentsStatus-")
	if err != nil {
		l.Error("Failed to load agents", "error", err)
		return err
	}

	for _, b := range m {
		// make a new decoder on the buffer, which is a Reader
		gobDec := gob.NewDecoder(b)

		// try to decode the gob into an array of AgentStatus, which is how its written
		var v []AgentStatus
		if err := gobDec.Decode(&v); err != nil {
			l.Error("error decoding gob", "error", err)
			return err
		}
		for _, a := range v {
			nextDataPointCredit := DataPoint{Timestamp: a.Timestamp, Value: a.Credits}
			nextDataPointShips := DataPoint{Timestamp: a.Timestamp, Value: a.Ships}
			var newCreditList []DataPoint
			var newShipList []DataPoint
			if cur, ok := AgentCreditHistory[a.Symbol]; ok {
				newCreditList = append(cur, nextDataPointCredit)
			} else {
				newCreditList = []DataPoint{nextDataPointCredit}
			}
			if cur, ok := AgentShipHistory[a.Symbol]; ok {
				newShipList = append(cur, nextDataPointShips)
			} else {
				newShipList = []DataPoint{nextDataPointShips}
			}

			AgentCreditHistory[a.Symbol] = newCreditList
			AgentShipHistory[a.Symbol] = newShipList

		}
	}
	return nil
}
