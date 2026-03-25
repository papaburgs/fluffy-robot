package datastore

import (
	"fmt"
	"strings"
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

func SystemFromWaypoint(w string) string {
	split := strings.Split(w, "-")
	return fmt.Sprintf("%s:%s", split[0], split[1])
}
