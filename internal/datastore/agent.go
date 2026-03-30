package datastore

import (
	"encoding/gob"
	"fmt"
	"time"
)

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
func LoadAgents(reset string) error {
	l := plog.With("function", "LoadAgents")
	zeroTimer.Reset(cacheLifetime)
	if len(Agents) > 0 {
		l.Info("Cache built, this is noop")
		return nil
	}
	// use readdata to get back a map of filename to byte buffers
	// NB use the . on the end so we don't get agentStatus files
	m, err := readData("agents.", "")
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

func GetAgents(reset string) map[string]Agent {
	l := plog.With("function", "GetAgentsWithActiveCredits")
	if err := LoadAgents(reset); err != nil {
		l.Error("error loading agents", "reset", reset, "error", err)
		return nil
	}
	return Agents
}

// LoadAgentHistory procedure
// save internal map of reset date to list of agent records
// that is zeroed when idle
// front end can call a function to Get Agent records of either ships or credits
// after a specfic point
// At that point we make the required data set and return it
// if we it is taking too long to make we can make a cache for it.

// makeResetListOfAgents gets all the data from a reset and makes a list
// then user called functions use that list to make specific maps
func makeResetListOfAgents(reset string) error {
	l := plog.With("function", "makeResetListofAgents")
	start := time.Now()
	zeroTimer.Reset(cacheLifetime)
	if len(allAgentHistory[reset]) == 0 {
		l.Info("Cache built, this is noop")
		return nil
	}
	// use readdata to get back a map of filename to byte buffers
	// NB use the dash to make it more unique
	m, err := readData("agentsStatus-", reset)
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
		allAgentHistory[reset] = v
	}
	l.Debug("Generated List of all agents on reset", "reset", reset, "duration", time.Now().Sub(start))
	return nil
}

func GetAgentRecordsCredits(reset, agent string, dur time.Duration) []DataPoint {
	l := plog.With("function", "GetAgentRecordsCredits")
	start := time.Now()
	if err := makeResetListOfAgents(reset); err != nil {
		l.Error("error making list of agents for reset", "reset", reset, "error", err)
		return nil
	}
	records := allAgentHistory[reset]
	var res []DataPoint
	cutoff := time.Now().Add(-dur).Unix()
	for _, r := range records {
		if r.Symbol == agent && cutoff > r.Timestamp {
			res = append(res, DataPoint{Timestamp: r.Timestamp, Value: r.Credits})
		}
	}
	l.Debug("Got agent records for credits", "reset", reset, "agent", agent, "duration", time.Now().Sub(start))
	return res
}

func GetAgentRecordsShips(reset, agent string, dur time.Duration) []DataPoint {
	l := plog.With("function", "GetAgentRecordsCredits")
	start := time.Now()
	if err := makeResetListOfAgents(reset); err != nil {
		l.Error("error making list of agents for reset", "reset", reset, "error", err)
		return nil
	}
	records := allAgentHistory[reset]
	var res []DataPoint
	cutoff := time.Now().Add(-dur).Unix()
	for _, r := range records {
		if r.Symbol == agent && cutoff > r.Timestamp {
			res = append(res, DataPoint{Timestamp: r.Timestamp, Value: r.Ships})
		}
	}
	l.Debug("Got agent records for ships", "reset", reset, "agent", agent, "duration", time.Now().Sub(start))
	return res
}

// LoadAgentHistory makes the agentname to credit and ship count maps
// Currently not being used - but good example once I see what we need
func LoadAgentHistory(reset string) error {
	l := plog.With("function", "LoadAgentHistory")
	// use readdata to get back a map of filename to byte buffers
	// NB use the dash to make it more unique
	m, err := readData("agentsStatus-", "")
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
