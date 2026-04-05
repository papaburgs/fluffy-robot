package datastore

import (
	"encoding/gob"
	"fmt"
	"os"
	"sort"
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

func loadStats(thisReset Reset) error {
	l := plog.With("function", "LoadStats")

	zeroTimer.Reset(cacheLifetime)
	l.Debug("try to load stats", "reset", thisReset)
	if stats[thisReset].Reset != "" {
		l.Info("Cache built, this is noop")
		return nil
	}
	// use readdata to get back a map of filename to byte buffers
	// NB use the . on the end so we don't get agentStatus files
	m, err := readData("stats.", "")
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
		stats[thisReset] = v
	}
	return nil
}

func GetStats(thisReset Reset) Stats {
	l := plog.With("function", "GetStats")
	l.Debug("try to load stats", "reset", thisReset)
	if err := loadStats(thisReset); err != nil {
		l.Error("error loading stats", "thisReset", thisReset, "error", err)
		return Stats{}
	}
	return stats[thisReset]
}

func loadLeaderboard(thisReset Reset) error {
	l := plog.With("function", "LoadLeaderboard")
	zeroTimer.Reset(cacheLifetime)
	// use readdata to get back a map of filename to byte buffers
	// NB use the . on the end so we don't get agentStatus files
	m, err := readData("leaderboard.", thisReset)
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
		creditLeaders[thisReset] = v.CreditsList
		chartLeaders[thisReset] = v.ChartsList
	}
	return nil
}

// GetLeaderboard returns the credit and charts leaderboard for provided reset
func GetLeaderboard(thisReset Reset) ([]LeaderboardEntry, []LeaderboardEntry) {
	l := plog.With("function", "GetLeaderboard")
	if err := loadLeaderboard(thisReset); err != nil {
		l.Error("error loading leaderboard", "thisReset", thisReset, "error", err)
		return nil, nil
	}
	return creditLeaders[thisReset], chartLeaders[thisReset]
}

// AllResets reads each directory in the path directory and makes a list
// of all the resets sorted in alphabetical order,
// which should be the same as chronological order
// since the resets are in YYYY-MM-DD format.
// This is used to populate the dropdown for selecting resets on the frontend.
func AllResets() []string {
	l := plog.With("function", "AllResets")
	resets := []string{}
	files, err := os.ReadDir(path)
	if err != nil {
		l.Error("Failed to read resets directory", "error", err)
		return resets
	}

	for _, f := range files {
		if f.IsDir() {
			resets = append(resets, f.Name())
		}
	}
	// before returning sort the resets in reverse order so the most recent reset is first
	sort.Slice(resets, func(i, j int) bool {
		return resets[i] > resets[j]
	})
	return resets
}

func LatestReset() Reset {
	for {
		if currentReset == "" {
			plog.Debug("reset is not updated yet")
			time.Sleep(time.Second)
		} else {
			break
		}
	}
	return currentReset
}

func NextReset() time.Time {
	for {
		if currentReset == "" {
			plog.Debug("reset is not updated yet")
			time.Sleep(time.Second)
		} else {
			break
		}
	}
	loadStats(currentReset)
	return stats[currentReset].NextReset
}
