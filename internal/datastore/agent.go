package datastore

import (
	"encoding/gob"
	"fmt"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/logging"
)

func StoreAgents(apiAgents []PublicAgent, now int64) {
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
	statusList = nil
}

func GetAgentList(thisReset Reset) ([]Agent, error) {
	res := []Agent{}
	m, err := readData("agents.", thisReset)
	if err != nil {
		logging.Error("Failed to load agents:", err)
		return res, err
	}

	if len(m) != 1 {
		return res, fmt.Errorf("invalid read")
	}

	for _, b := range m {
		gobDec := gob.NewDecoder(b)
		if err := gobDec.Decode(&res); err != nil {
			logging.Error("error decoding gob", err)
			return res, err
		}
	}
	m = nil
	return res, nil
}

func GetAgentHistory(thisReset Reset) ([]AgentStatus, error) {
	res := []AgentStatus{}
	m, err := readData("agentsStatus-", thisReset)
	if err != nil {
		logging.Error("Failed to load agent history:", err)
		return res, err
	}

	for _, b := range m {
		gobDec := gob.NewDecoder(b)
		v := make([]AgentStatus, len(m)*2)
		if err := gobDec.Decode(&v); err != nil {
			logging.Error("error decoding gob:", err)
			return res, err
		}
		res = append(res, v...)
	}
	m = nil
	return res, nil
}

func GetAgentRecordsCredits(thisReset Reset, agent string, dur time.Duration) []DataPoint {
	agents, err := GetAgentHistory(thisReset)
	if err != nil {
		logging.Error("error making list of agents for reset", "reset", thisReset, "error", err)
		return nil
	}
	var res []DataPoint
	cutoff := time.Now().Add(-1 * dur).Unix()
	for _, r := range agents {
		if r.Symbol == agent && cutoff < r.Timestamp {
			res = append(res, DataPoint{Timestamp: r.Timestamp, Value: r.Credits})
		}
	}
	agents = nil
	return res
}

func GetAgentRecordsShips(thisReset Reset, agent string, dur time.Duration) []DataPoint {
	agents, err := GetAgentHistory(thisReset)
	if err != nil {
		return nil
	}
	var res []DataPoint
	cutoff := time.Now().Add(-1 * dur).Unix()
	for _, r := range agents {
		if r.Symbol == agent && cutoff < r.Timestamp {
			res = append(res, DataPoint{Timestamp: r.Timestamp, Value: r.Ships})
		}
	}
	agents = nil
	return res
}

func GetAgents(thisReset Reset) map[string]Agent {
	agents, err := GetAgentList(thisReset)
	if err != nil {
		logging.Error("error loading agents", "reset", thisReset, "error", err)
		return nil
	}
	res := make(map[string]Agent, len(agents))
	for _, a := range agents {
		res[a.Symbol] = a
	}
	agents = nil
	return res
}
