package datastore

import (
	"encoding/gob"
	"fmt"
	"time"
)

// Stats consists of data on the status endpoint, not counting leaderboard

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

func LoadStats(r string) error {
	l := plog.With("function", "LoadAgents")
	zeroTimer.Reset(cacheLifetime)
	// use readdata to get back a map of filename to byte buffers
	// NB use the . on the end so we don't get agentStatus files
	m, err := readData("stats.")
	if err != nil {
		l.Error("Failed to read stats file", "error", err)
		return err
	}

	if len(m) != 1 {
		l.Error("should only get one result", "count", len(m))
		return fmt.Errorf("invalid read")
	}

	for k, b := range m {
		l.Debug("de-gobbing file", "filename", k)
		var v Stats
		// make a new decoder on the buffer, which is a Reader
		gobDec := gob.NewDecoder(b)

		// try to decode the gob into the stats object
		if err := gobDec.Decode(&v); err != nil {
			l.Error("error decoding gob", "error", err)
			return err
		}
		StoredStats = v
	}
	return nil
}

func LoadLeaderboard(r string) error {
	l := plog.With("function", "LoadLeaderboard")
	zeroTimer.Reset(cacheLifetime)
	// use readdata to get back a map of filename to byte buffers
	// NB use the . on the end so we don't get agentStatus files
	m, err := readData("leaderboard.")
	if err != nil {
		l.Error("Failed to read file", "error", err)
		return err
	}

	if len(m) != 1 {
		l.Error("should only get one result", "count", len(m))
		return fmt.Errorf("invalid read")
	}

	for k, b := range m {
		l.Debug("de-gobbing file", "filename", k)
		var v LeaderboardRecord
		// make a new decoder on the buffer, which is a Reader
		gobDec := gob.NewDecoder(b)

		// try to decode the gob into the stats object
		if err := gobDec.Decode(&v); err != nil {
			l.Error("error decoding gob", "error", err)
			return err
		}
		LatestCreditLeaders = v.CreditsList
		LatestChartLeaders = v.ChartsList
	}
	return nil
}

func LatestReset() string {
	return StoredStats.Reset
}

func NextReset() time.Time {
	return StoredStats.NextReset
}
