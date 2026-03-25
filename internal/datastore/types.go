package datastore

import "time"

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

type LeaderboardEntry struct {
	Symbol string
	Value  int64
}

type LeaderboardRecord struct {
	CreditsList []LeaderboardEntry
	ChartsList  []LeaderboardEntry
}

type Agent struct {
	Symbol       string
	Credits      int64
	Faction      string
	Headquarters string
	System       string
}

type AgentStatus struct {
	Symbol    string
	Timestamp int64
	Credits   int64
	Ships     int64
}
