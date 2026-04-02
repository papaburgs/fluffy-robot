package datastore

import "time"

type ConstructionStatus uint8

// jumpgate status codes:
const (
	NoActivity ConstructionStatus = iota
	Active
	Const
	Complete
)

// To make the keys of maps more descriptive, make a 'reset' string type

type Reset string

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

type Agent struct {
	Symbol       string
	Credits      int64
	Faction      string
	Headquarters string
	System       string
}

type resetAgentKey struct {
	Reset string
	Agent string
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
	Status       ConstructionStatus
	Complete     int64
}

type JGConstruction struct {
	Timestamp int64
	Jumpgate  string
	Fabmat    int
	Advcct    int
}

type JumpGateAgentListStruct struct {
	AgentsToCheck  []PublicAgent `json:"agents_to_check"`
	AgentsToIgnore []PublicAgent `json:"agents_to_ignore"`
}

// type TimedConstructionRecord struct {
// 	Timestamp time.Time
// 	Fabmat    int
// 	Advcct    int
// }

type ConstructionOverview struct {
	Agent     string
	Jumpgate  string
	Fabmat    int
	Advcct    int
	Timestamp time.Time
}

// these are variables that should be zeroed on startup or after inactivity
// originally had make these public, but instead, making these just big maps
// of lists of _the thing_ referenced by reset
// the getter funcs will filter and manipulate the lists when needed
var (
	// ************  Agent vars  ************* \\
	// maps the reset to the list of agents
	agentsList   map[Reset][]Agent
	agentHistory map[Reset][]AgentStatus

	// *********** Stats vars *************** \\
	stats         map[Reset]Stats
	creditLeaders map[Reset][]LeaderboardEntry
	chartLeaders  map[Reset][]LeaderboardEntry

	// ************ Jumpgates *************** \\
	// map of reset to list of jumpgate statuses
	jumpgateLists map[Reset][]JGInfo

	// ************ Constructions *************** \\
	// map of reset to list of construction statuses
	constructionsLists map[Reset][]JGConstruction
)
