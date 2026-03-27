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

// ***********  Agent types *************** \\
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

type DataPoint struct {
	Timestamp int64
	Value     int64
}

type Stats struct {
	Reset        string
	MarketUpdate time.Time
	Agents       int
	Accounts     int
	Ships        int
	Systems      int
	Waypoints    int
	Status       string
	Version      string
	NextReset    time.Time
	LastUpdate   time.Time
}

type JGInfo struct {
	Jumpgate     string
	System       string
	Headquarters string
	Status       int
	Complete     int64
}

type JGConstruction struct {
	Reset     string
	Timestamp int64
	Jumpgate  string
	Fabmat    int
	Advcct    int
}

var (
	// ************  Agent vars  ************* \\
	Agents             map[string]Agent
	AgentCreditHistory map[string][]DataPoint
	AgentShipHistory   map[string][]DataPoint

	// *********** Stats vars *************** \\
	StoredStats         Stats
	LatestCreditLeaders []LeaderboardEntry
	LatestChartLeaders  []LeaderboardEntry

	// ************ Jumpgates *************** \\
	hydratedJGList []JGInfo
	JumpgatesBySystem map[string]JGInfo
)
