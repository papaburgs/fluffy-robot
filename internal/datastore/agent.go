package datastore

import (
	"encoding/gob"
	"fmt"
	"sort"
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
	for consolidating {
		fmt.Println("still consolidating")
		time.Sleep(time.Second)
	}
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

func GetAgentHistory(thisReset Reset, start, end int64) ([]AgentStatus, error) {
	for consolidating {
		fmt.Println("still consolidating")
		time.Sleep(time.Second)
	}
	if end == 0 {
		end = time.Now().Unix()
	}
	res := []AgentStatus{}
	m, err := readData("agentsStatus-", thisReset)
	if err != nil {
		logging.Error("Failed to load agent history:", err)
		return res, err
	}

	allRecords := make([]AgentStatus, 0, len(m)*2)
	for _, b := range m {
		gobDec := gob.NewDecoder(b)
		var v []AgentStatus
		if err := gobDec.Decode(&v); err != nil {
			logging.Error("error decoding gob:", err)
			return res, err
		}
		allRecords = append(allRecords, v...)
	}

	if len(m) > 10 {
		consolidating = true
		// consolidate("agentsStatus", allRecords, m)
		consolidating = false
		m = nil
	} else {
		m = nil
	}

	for _, r := range allRecords {
		if r.Timestamp >= start && r.Timestamp <= end {
			res = append(res, r)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Timestamp < res[j].Timestamp
	})
	return res, nil
}
