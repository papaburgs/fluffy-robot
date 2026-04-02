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
func loadAgents(thisReset Reset) error {
	l := plog.With("function", "LoadAgents")
	zeroTimer.Reset(cacheLifetime)
	if len(agentsList[thisReset]) > 0 {
		l.Info("Cache built, this is noop")
		return nil
	}
	// use readdata to get back a map of filename to byte buffers
	// NB use the . on the end so we don't get agentStatus files
	m, err := readData("agents.", thisReset)
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
		agentsList[thisReset] = v
	}
	l.Debug("loaded agents", "count", len(agentsList[thisReset]))
	return nil
}

// loadAgentHistory gets all the data from a reset and makes a list
// then user called functions use that list to make specific maps
func loadAgentHistory(thisReset Reset) error {
	l := plog.With("function", "makeResetListofAgents")
	start := time.Now()
	zeroTimer.Reset(cacheLifetime)
	if len(agentHistory[thisReset]) == 0 {
		l.Info("Cache built, this is noop")
		return nil
	}
	// use readdata to get back a map of filename to byte buffers
	// NB use the dash to make it more unique
	m, err := readData("agentsStatus-", thisReset)
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
		agentHistory[thisReset] = v
	}
	l.Debug("Generated List of all agents on reset", "reset", thisReset, "duration", time.Now().Sub(start))
	return nil
}

func GetAgentRecordsCredits(thisReset Reset, agent string, dur time.Duration) []DataPoint {
	l := plog.With("function", "GetAgentRecordsCredits")
	start := time.Now()
	if err := loadAgentHistory(thisReset); err != nil {
		l.Error("error making list of agents for reset", "reset", thisReset, "error", err)
		return nil
	}
	var res []DataPoint
	cutoff := time.Now().Add(-dur).Unix()
	for _, r := range agentHistory[thisReset] {
		if r.Symbol == agent && cutoff > r.Timestamp {
			res = append(res, DataPoint{Timestamp: r.Timestamp, Value: r.Credits})
		}
	}
	l.Debug("Got agent records for credits", "reset", thisReset, "agent", agent, "duration", time.Now().Sub(start))
	return res
}

func GetAgentRecordsShips(thisReset Reset, agent string, dur time.Duration) []DataPoint {
	l := plog.With("function", "GetAgentRecordsCredits")
	start := time.Now()
	if err := loadAgentHistory(thisReset); err != nil {
		l.Error("error making list of agents for reset", "reset", thisReset, "error", err)
		return nil
	}
	var res []DataPoint
	cutoff := time.Now().Add(-dur).Unix()
	for _, r := range agentHistory[thisReset] {
		if r.Symbol == agent && cutoff > r.Timestamp {
			res = append(res, DataPoint{Timestamp: r.Timestamp, Value: r.Ships})
		}
	}
	l.Debug("Got agent records for ships", "reset", thisReset, "agent", agent, "duration", time.Now().Sub(start))
	return res
}

// GetAgents returns a map of agent name to agent data for provided reset
func GetAgents(thisReset Reset) map[string]Agent {
	l := plog.With("function", "GetAgents")
	if err := loadAgents(thisReset); err != nil {
		l.Error("error loading agents", "reset", thisReset, "error", err)
		return nil
	}
	res := make(map[string]Agent)
	for _, a := range agentsList[thisReset] {
		res[a.Symbol] = a
	}
	return res
}
