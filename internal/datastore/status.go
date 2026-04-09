package datastore

import (
	"encoding/gob"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/logging"
)

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

func GetStats(thisReset Reset) (Stats, error) {
	res := Stats{}
	m, err := readData("stats.", "")
	if err != nil {
		return res, err
	}

	if len(m) != 1 {
		return res, fmt.Errorf("invalid read")
	}

	for _, b := range m {
		gobDec := gob.NewDecoder(b)
		if err := gobDec.Decode(&res); err != nil {
			return res, err
		}
	}
	m = nil
	return res, nil
}

func GetLeaderboard(thisReset Reset) ([]LeaderboardEntry, []LeaderboardEntry, error) {
	res := LeaderboardRecord{}
	m, err := readData("leaderboard.", thisReset)
	if err != nil {
		logging.Error("Failed to read file", err)
		return nil, nil, err
	}

	if len(m) != 1 {
		return nil, nil, fmt.Errorf("invalid read")
	}

	for _, b := range m {
		gobDec := gob.NewDecoder(b)
		if err := gobDec.Decode(&res); err != nil {
			logging.Error("error decoding gob", err)
			return nil, nil, err
		}
	}
	m = nil
	return res.CreditsList, res.ChartsList, nil
}

func AllResets() []string {
	resets := []string{}
	files, err := os.ReadDir(path)
	if err != nil {
		logging.Error("Failed to read resets directory", err)
		return resets
	}

	for _, f := range files {
		if f.IsDir() {
			resets = append(resets, f.Name())
		}
	}
	sort.Slice(resets, func(i, j int) bool {
		return resets[i] > resets[j]
	})
	return resets
}

func LatestReset() Reset {
	for {
		if currentReset == "" {
			// logging.Debug("reset is not updated yet")
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
			// logging.Debug("reset is not updated yet")
			time.Sleep(time.Second)
		} else {
			break
		}
	}
	st, err := GetStats(currentReset)
	if err != nil {
		logging.Error("error loading stats for NextReset", err)
		return time.Time{}
	}
	return st.NextReset
}
