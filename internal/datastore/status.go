package datastore

import "time"

// Stats consists of data on the status endpoint, not counting leaderboard
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

func StoreStats(r ResponseStatus) error {
	st := Stats{
		Reset:        r.ResetDate,
		MarketUpdate: r.Health.LastMarketUpdate,
		Agents:       r.Stats.Agents,
		Accounts:     *r.Stats.Accounts,
		Ships:        r.Stats.Ships,
		Systems:      r.Stats.Systems,
		Waypoints:    r.Stats.Waypoints,
		Status:       r.Status,
		Version:      r.Version,
		NextReset:    r.ServerResets.Next,
		LastUpdate:   time.Now(),
	}
	writeData("stats", 0, st)
	return nil
}

func StoreLeaderboards(r ResponseStatus) error {
	ldrbd := LeaderboardRecord{}
	ldrbd.ChartsList = []LeaderboardEntry{}
	for _, x := range r.Leaderboards.MostSubmittedCharts {
		ldrbd.ChartsList = append(ldrbd.ChartsList,
			LeaderboardEntry{
				Symbol: x.AgentSymbol,
				Value:  int64(x.ChartCount),
			},
		)
	}
	ldrbd.CreditsList = []LeaderboardEntry{}
	for _, x := range r.Leaderboards.MostCredits {
		ldrbd.CreditsList = append(ldrbd.CreditsList,
			LeaderboardEntry{
				Symbol: x.AgentSymbol,
				Value:  int64(x.Credits),
			},
		)
	}

	writeData("leaderboard", 0, ldrbd)

	return nil

}
