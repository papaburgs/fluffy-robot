package types

import "time"

type PublicAgent struct {
	Credits         int64  `json:"credits"`
	Headquarters    string `json:"headquarters"`
	ShipCount       int    `json:"shipCount"`
	StartingFaction string `json:"startingFaction"`
	Symbol          string `json:"symbol"`
}

type JumpGateAgentListStruct struct {
	AgentsToCheck  []PublicAgent `json:"agents_to_check"`
	AgentsToIgnore []PublicAgent `json:"agents_to_ignore"`
}

type AgentRecord struct {
	Timestamp time.Time
	ShipCount int
	Credits   int
}

type ConstructionRecord struct {
	Timestamp time.Time
	Fabmat    int
	Advcct    int
}

type ConstructionOverview struct {
	Agent     string
	Jumpgate  string
	Fabmat    int
	Advcct    int
	Timestamp time.Time
}
