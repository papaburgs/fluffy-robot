package main

import (
	"time"

	"github.com/papaburgs/fluffy-robot/internal/datastore"
)

type ResponseStatus struct {
	Leaderboards struct {
		MostCredits []struct {
			AgentSymbol string `json:"agentSymbol"`
			Credits     int64  `json:"credits"`
		} `json:"mostCredits"`
		MostSubmittedCharts []struct {
			AgentSymbol string `json:"agentSymbol"`
			ChartCount  int    `json:"chartCount"`
		} `json:"mostSubmittedCharts"`
	} `json:"leaderboards"`
	ResetDate string `json:"resetDate"`
	Health    struct {
		LastMarketUpdate time.Time `json:"lastMarketUpdate"`
	} `json:"health"`
	ServerResets struct {
		Frequency string    `json:"frequency"`
		Next      time.Time `json:"next"`
	} `json:"serverResets"`
	Stats struct {
		Accounts  *int `json:"accounts,omitempty"`
		Agents    int  `json:"agents"`
		Ships     int  `json:"ships"`
		Systems   int  `json:"systems"`
		Waypoints int  `json:"waypoints"`
	} `json:"stats"`
	Status  string `json:"status"`
	Version string `json:"version"`
}

type ResponseAgents struct {
	Data []datastore.PublicAgent `json:"data"`
	Meta Meta                    `json:"meta"`
}

type Meta struct {
	Limit int `json:"limit"`
	Page  int `json:"page"`
	Total int `json:"total"`
}

type ConstructionMaterial struct {
	TradeSymbol string `json:"tradeSymbol"`
	Required    int    `json:"required"`
	Fulfilled   int    `json:"fulfilled"`
}

type ConstructionStatus struct {
	Symbol     string                 `json:"symbol"`
	Materials  []ConstructionMaterial `json:"materials"`
	IsComplete bool                   `json:"isComplete"`
}

type HTTPResponse struct {
	Bytes      []byte
	StatusCode int
}

