package frontend

import (
	"log/slog"
	"strings"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/types"
)

// // GetAgentRecordsFromDB reads agent history from Turso DB.
// func GetAgentRecordsFromDB(symbol string, reset string, duration time.Duration) ([]ds.AgentStatus, error) {
// 	slog.Error("don't use this")
// 	return nil, fmt.Errorf("GetAgentRecordsFromDB is deprecated and should not be used")
// }
//
// GetConstructionRecordsFromDB retrieves jumpgate construction progress history.
func GetConstructionRecordsFromDB(agents []string, reset string, duration time.Duration) (map[string][]types.ConstructionRecord, error) {
	res := make(map[string][]types.ConstructionRecord)
	startTime := time.Now().Add(-duration).Unix()

	for _, agent := range agents {
		var hq string
		err := a.DB.QueryRow("SELECT headquarters FROM agents WHERE reset = ? AND symbol = ?", reset, agent).Scan(&hq)
		if err != nil {
			continue
		}
		parts := strings.Split(hq, "-")
		if len(parts) < 2 {
			continue
		}
		system := parts[0] + "-" + parts[1]

		var jg string
		err = a.DB.QueryRow("SELECT jumpgate FROM jumpgates WHERE reset = ? AND system = ? AND status IN (2, 3)", reset, system).Scan(&jg)
		if err != nil {
			continue
		}

		// Avoid querying same jumpgate multiple times if agents share hq system
		if _, ok := res[jg]; ok {
			continue
		}

		rows, err := a.DB.Query("SELECT timestamp, fabmat, advcct FROM construction WHERE reset = ? AND jumpgate = ? AND timestamp >= ? ORDER BY timestamp ASC", reset, jg, startTime)
		if err != nil {
			slog.Error("failed to query construction records", "jg", jg, "error", err)
			continue
		}
		defer rows.Close()

		recs := []types.ConstructionRecord{}
		for rows.Next() {
			var ts int64
			var fab, adv int
			if err := rows.Scan(&ts, &fab, &adv); err != nil {
				continue
			}
			recs = append(recs, types.ConstructionRecord{
				Timestamp: time.Unix(ts, 0).UTC(),
				Fabmat:    fab,
				Advcct:    adv,
			})
		}
		if len(recs) > 0 {
			res[jg] = recs
		}
	}
	return res, nil
}

// GetLatestConstructionRecords retrieves the most recent construction progress for a set of agents.
func GetLatestConstructionRecords(agents []string, reset string) ([]types.ConstructionOverview, error) {
	results := []types.ConstructionOverview{}

	for _, agent := range agents {
		var hq string
		err := a.DB.QueryRow("SELECT headquarters FROM agents WHERE reset = ? AND symbol = ?", reset, agent).Scan(&hq)
		if err != nil {
			continue
		}
		parts := strings.Split(hq, "-")
		if len(parts) < 2 {
			continue
		}
		system := parts[0] + "-" + parts[1]

		var jg string
		err = a.DB.QueryRow("SELECT jumpgate FROM jumpgates WHERE reset = ? AND system = ? AND status IN (2, 3)", reset, system).Scan(&jg)
		if err != nil {
			continue
		}

		var ts int64
		var fab, adv int
		err = a.DB.QueryRow("SELECT timestamp, fabmat, advcct FROM construction WHERE reset = ? AND jumpgate = ? ORDER BY timestamp DESC LIMIT 1", reset, jg).Scan(&ts, &fab, &adv)
		if err != nil {
			continue
		}

		results = append(results, types.ConstructionOverview{
			Agent:     agent,
			Jumpgate:  jg,
			Fabmat:    fab,
			Advcct:    adv,
			Timestamp: time.Unix(ts, 0).UTC(),
		})
	}
	return results, nil
}
//
// // GetAllAgentsFromDB returns all agents and their active status from Turso DB.
// // done
// func GetAllAgentsFromDB(reset string) (map[string]AgentStatus, error) {
// 	res := make(map[string]AgentStatus)
// 	// We use the stored Reset in App struct which should be updated periodically or at start
//
// 	rows, err := a.DB.Query(`
// 		SELECT symbol, credits 
// 		FROM agents 
// 		WHERE reset = ? 
// 	`, reset)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to query agents: %w", err)
// 	}
// 	defer rows.Close()
//
// 	for rows.Next() {
// 		var symbol string
// 		var credits int64
// 		if err := rows.Scan(&symbol, &credits); err != nil {
// 			continue
// 		}
// 		res[symbol] = AgentStatus{
// 			Active:  credits != 175000,
// 			Credits: credits,
// 		}
// 	}
// 	return res, nil
// }
//
// // GetStats returns the latest server stats for the current reset.
// // done
// func GetStats(reset string) (map[string]interface{}, error) {
// 	var marketUpdate time.Time
// 	var agents, accounts, ships, systems, waypoints int
// 	var status, version string
// 	var nextReset time.Time
//
// 	err := a.DB.QueryRow(`
// 		SELECT marketUpdate, agents, accounts, ships, systems, waypoints, status, version, nextReset
// 		FROM stats
// 		WHERE reset = ?
// 	`, reset).Scan(&marketUpdate, &agents, &accounts, &ships, &systems, &waypoints, &status, &version, &nextReset)
//
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	return map[string]interface{}{
// 		"reset":        reset,
// 		"marketUpdate": marketUpdate,
// 		"agents":       agents,
// 		"accounts":     accounts,
// 		"ships":        ships,
// 		"systems":      systems,
// 		"waypoints":    waypoints,
// 		"status":       status,
// 		"version":      version,
// 		"nextReset":    nextReset,
// 	}, nil
// }

// GetLeaderboard returns the leaderboard for the current reset.
// type can be 'credits' or 'charts'.
// func GetLeaderboard(leaderboardType string, reset string) ([]map[string]interface{}, error) {
// 	slog.Debug("getting leaderboard details", "type", leaderboardType)
// 	if a.Reset == "" {
// 		return nil, fmt.Errorf("reset not set in App struct")
// 	}
//
// 	var credits, charts string
// 	err := a.DB.QueryRow(`
// 		SELECT credits, charts
// 		FROM leaderboard
// 		WHERE reset = ?
// 	`, reset).Scan(&credits, &charts)
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	var rawData string
// 	if leaderboardType == "credits" {
// 		rawData = credits
// 	} else {
// 		rawData = charts
// 	}
//
// 	var results []map[string]interface{}
// 	if rawData == "" {
// 		return results, nil
// 	}
//
// 	// Data is stored as SYMBOL,VALUE|SYMBOL,VALUE
// 	entries := strings.Split(rawData, "|")
// 	for _, entry := range entries {
// 		parts := strings.Split(entry, ",")
// 		if len(parts) != 2 {
// 			continue
// 		}
// 		symbol := parts[0]
// 		count, _ := strconv.ParseInt(parts[1], 10, 64)
// 		results = append(results, map[string]interface{}{
// 			"symbol": symbol,
// 			"count":  count,
// 		})
// 	}
// 	return results, nil
// }

// GetJumpgates returns all jumpgates for the current reset.
func GetJumpgates(reset string) ([]map[string]interface{}, error) {
	rows, err := a.DB.Query(`
		SELECT system, headquarters, jumpgate, completetimestamp, status
		FROM jumpgates
		WHERE reset = ?
		ORDER BY system ASC
	`, reset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var system, headquarters, jumpgate string
		var complete int64
		var status int
		if err := rows.Scan(&system, &headquarters, &jumpgate, &complete, &status); err != nil {
			continue
		}

		statusStr := "Unknown"
		switch status {
		case 0:
			statusStr = "No Activity"
		case 1:
			statusStr = "Active"
		case 2:
			statusStr = "Under Construction"
		case 3:
			statusStr = "Complete"
		}

		results = append(results, map[string]interface{}{
			"system":       system,
			"headquarters": headquarters,
			"jumpgate":     jumpgate,
			"complete":     status == 3,            // boolean for template styling
			"completeTime": time.Unix(complete, 0), // meaningful only if complete > 0
			"status":       statusStr,
		})
	}
	return results, nil
}
