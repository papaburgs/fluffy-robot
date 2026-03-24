package datastore

import "time"

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

