package main

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/types"
)

// GetAgentRecordsFromDB reads agent history from Turso DB.
func (a *App) GetAgentRecordsFromDB(symbol string, duration time.Duration) ([]types.AgentRecord, error) {
	records := []types.AgentRecord{}
	startTime := time.Now().Add(-duration).Unix()

	rows, err := a.DB.Query("SELECT timestamp, ships, credits FROM agents WHERE symbol = ? AND timestamp >= ? ORDER BY timestamp ASC", symbol, startTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query agent history: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ts int64
		var ships, credits int
		if err := rows.Scan(&ts, &ships, &credits); err != nil {
			continue
		}
		records = append(records, types.AgentRecord{
			Timestamp: time.Unix(ts, 0).UTC(),
			ShipCount: ships,
			Credits:   credits,
		})
	}
	return records, nil
}

// GetAllAgentsFromDB returns all agents and their active status from Turso DB.
func (a *App) GetAllAgentsFromDB() (map[string]AgentStatus, error) {
	l := slog.With("function", "GetAllAgentsFromDB")
	res := make(map[string]AgentStatus)
	// We use the stored Reset in App struct which should be updated periodically or at start
	if a.Reset == "" {
		l.Error("failed to get reset for GetAllAgentsFromDB")
		return nil, fmt.Errorf("reset not set in App struct")
	}

	rows, err := a.DB.Query(`
		SELECT symbol, credits 
		FROM agents 
		WHERE (symbol, timestamp) IN (
			SELECT symbol, MAX(timestamp) 
			FROM agents 
			WHERE reset = ? 
			GROUP BY symbol
		)
	`, a.Reset)
	if err != nil {
		return nil, fmt.Errorf("failed to query agents: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var symbol string
		var credits int64
		if err := rows.Scan(&symbol, &credits); err != nil {
			continue
		}
		res[symbol] = AgentStatus{
			Active:  credits != 175000,
			Credits: credits,
		}
	}
	return res, nil
}

// GetStats returns the latest server stats for the current reset.
func (a *App) GetStats() (map[string]interface{}, error) {
	if a.Reset == "" {
		return nil, fmt.Errorf("reset not set in App struct")
	}

	var marketUpdate time.Time
	var agents, accounts, ships, systems, waypoints int
	var status, version string
	var nextReset time.Time

	err := a.DB.QueryRow(`
		SELECT marketUpdate, agents, accounts, ships, systems, waypoints, status, version, nextReset
		FROM stats
		WHERE reset = ?
	`, a.Reset).Scan(&marketUpdate, &agents, &accounts, &ships, &systems, &waypoints, &status, &version, &nextReset)

	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"reset":        a.Reset,
		"marketUpdate": marketUpdate,
		"agents":       agents,
		"accounts":     accounts,
		"ships":        ships,
		"systems":      systems,
		"waypoints":    waypoints,
		"status":       status,
		"version":      version,
		"nextReset":    nextReset,
	}, nil
}

// GetLeaderboard returns the leaderboard for the current reset.
// type can be 'credits' or 'charts'.
func (a *App) GetLeaderboard(leaderboardType string) ([]map[string]interface{}, error) {
	slog.Debug("getting leaderboard details", "type", leaderboardType)
	if a.Reset == "" {
		return nil, fmt.Errorf("reset not set in App struct")
	}

	rows, err := a.DB.Query(`
		SELECT symbol, count
		FROM leaderboard
		WHERE reset = ? AND type = ? AND timestamp = (
			SELECT MAX(timestamp) from leaderboard where reset = ? and type = ? 
		)
		ORDER BY count DESC
	`, a.Reset, leaderboardType, a.Reset, leaderboardType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var symbol string
		var count int64
		if err := rows.Scan(&symbol, &count); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"symbol": symbol,
			"count":  count,
		})
	}
	return results, nil
}

// GetJumpgates returns all jumpgates for the current reset.
func (a *App) GetJumpgates() ([]map[string]interface{}, error) {
	l := slog.With("function", "GetJumpgates")
	l.Info("check")
	if a.Reset == "" {
		return nil, fmt.Errorf("reset not set in App struct")
	}

	rows, err := a.DB.Query(`
		SELECT system, headquarters, jumpgate, complete
		FROM jumpgates
		WHERE reset = ?
		ORDER BY system ASC
	`, a.Reset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var system, headquarters, jumpgate string
		var complete int64
		if err := rows.Scan(&system, &headquarters, &jumpgate, &complete); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"system":       system,
			"headquarters": headquarters,
			"jumpgate":     jumpgate,
			"complete":     complete > 0,           // boolean
			"completeTime": time.Unix(complete, 0), // meaningful only if complete > 0
		})
	}
	return results, nil
}
