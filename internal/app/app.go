package app

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"html/template"
	"io" // Added this import
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/collector"
	"github.com/papaburgs/fluffy-robot/internal/types"
)

// CSVAgentRecord represents a single row parsed from the agents.csv file.
type CSVAgentRecord struct {
	Timestamp    int64  // Unix epoch
	Symbol       string
	Credits      int64
	ShipCount    int
	Headquarters string
}

// GetAgentRecordsFromCSV reads the agents.csv file for the current reset and returns
// filtered records for a given symbol and duration.
func (a *App) GetAgentRecordsFromCSV(symbol string, duration time.Duration) ([]types.AgentRecord, error) {
	records := []types.AgentRecord{}
	csvFilePath := filepath.Join(a.StorageRoot, "reset-"+a.Reset, "agents.csv")

	file, err := os.Open(csvFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Debug("agents.csv file not found", "path", csvFilePath)
			return records, nil // Return empty if file doesn't exist
		}
		slog.Error("error opening agents.csv file", "error", err, "path", csvFilePath)
		return nil, fmt.Errorf("error opening CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	// reader.FieldsPerRecord = 5 // We expect 5 fields per record: Timestamp,Symbol,Credits,ShipCount,Headquarters

	endTime := time.Now().UTC()
	startTime := endTime.Add(-duration)

	for {
		line, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			slog.Error("error reading CSV line", "error", err)
			continue
		}

		if len(line) != 5 {
			slog.Warn("skipping malformed CSV line", "line", line)
			continue
		}

		csvRecord := CSVAgentRecord{}
		csvRecord.Timestamp, err = strconv.ParseInt(line[0], 10, 64)
		if err != nil {
			slog.Warn("skipping CSV line, invalid timestamp", "line", line, "error", err)
			continue
		}
		csvRecord.Symbol = line[1]
		csvRecord.Credits, err = strconv.ParseInt(line[2], 10, 64)
		if err != nil {
			slog.Warn("skipping CSV line, invalid credits", "line", line, "error", err)
			continue
		}
		csvRecord.ShipCount, err = strconv.Atoi(line[3])
		if err != nil {
			slog.Warn("skipping CSV line, invalid shipCount", "line", line, "error", err)
			continue
		}
		csvRecord.Headquarters = line[4]

		// Filter by symbol
		if csvRecord.Symbol != symbol {
			continue
		}

		// Filter by time range
		recordTime := time.Unix(csvRecord.Timestamp, 0).UTC()
		if recordTime.Before(startTime) || recordTime.After(endTime) {
			continue
		}

		records = append(records, types.AgentRecord{
			Timestamp: recordTime,
			Credits:   int(csvRecord.Credits),
			ShipCount: csvRecord.ShipCount,
		})
	}
	return records, nil
}

// GetAllAgentsFromCSV reads the agents.csv file for the current reset and returns
// a map of agent symbols to their activity status (true if active, false if inactive, i.e., 175k credits).
// It considers the latest credit entry for each agent.
func (a *App) GetAllAgentsFromCSV() (map[string]bool, error) {
	res := make(map[string]bool)
	csvFilePath := filepath.Join(a.StorageRoot, "reset-"+a.Reset, "agents.csv")

	file, err := os.Open(csvFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Debug("agents.csv file not found", "path", csvFilePath)
			return res, nil // Return empty map if file doesn't exist
		}
		slog.Error("error opening agents.csv file", "error", err, "path", csvFilePath)
		return nil, fmt.Errorf("error opening CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	// reader.FieldsPerRecord = 5 // We expect 5 fields per record: Timestamp,Symbol,Credits,ShipCount,Headquarters

	latestAgentCredits := make(map[string]int64)
	latestAgentTimestamps := make(map[string]int64) // To ensure we get the truly latest record

	for {
		line, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			slog.Error("error reading CSV line", "error", err)
			continue
		}

		if len(line) != 5 {
			slog.Warn("skipping malformed CSV line", "line", line)
			continue
		}

		timestamp, err := strconv.ParseInt(line[0], 10, 64)
		if err != nil {
			slog.Warn("skipping CSV line, invalid timestamp", "line", line, "error", err)
			continue
		}
		symbol := line[1]
		credits, err := strconv.ParseInt(line[2], 10, 64)
		if err != nil {
			slog.Warn("skipping CSV line, invalid credits", "line", line, "error", err)
			continue
		}

		// Only update if this record is newer than the previously recorded latest for this agent
		if currentLatestTimestamp, found := latestAgentTimestamps[symbol]; !found || timestamp > currentLatestTimestamp {
			latestAgentCredits[symbol] = credits
			latestAgentTimestamps[symbol] = timestamp
		}
	}

	for symbol, credits := range latestAgentCredits {
		res[symbol] = true
		if credits == 175000 {
			res[symbol] = false
		}
	}
	return res, nil
}

// GetAgentRecordsFromDB reads agent history from Turso DB.
func (a *App) GetAgentRecordsFromDB(symbol string, duration time.Duration) ([]types.AgentRecord, error) {
	records := []types.AgentRecord{}
	startTime := time.Now().Add(-duration).Unix()

	rows, err := a.DB.Query("SELECT timestamp, ships, credits FROM agent WHERE symbol = ? AND timestamp >= ? ORDER BY timestamp ASC", symbol, startTime)
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
func (a *App) GetAllAgentsFromDB() (map[string]bool, error) {
	res := make(map[string]bool)
	rows, err := a.DB.Query("SELECT symbol, credits FROM agents WHERE reset = ?", a.Reset)
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
		res[symbol] = (credits != 175000)
	}
	return res, nil
}

// App is our main application
// it holds some
type App struct {
	StorageRoot string
	// Reset is on the server status page, we use it to sort data
	Reset string
	// collectPointsPerHour is used to change the charts density
	collectPointsPerHour int
	JumpGateAgents       types.JumpGateAgentListStruct // New field for in-memory jumpgate agent lists
	t                    *template.Template
	DB                   *sql.DB
}

// NewApp starts the collector is collect is true
// it returns an app that contains all the handlers
// for the ui
func NewApp(storage string, collect bool, db *sql.DB) *App {
	var collectEvery = 5
	a := App{
		StorageRoot:          storage,
		Reset:                "00000",
		collectPointsPerHour: 60 / collectEvery,
		DB:                   db,
	}
	a.t = template.Must(template.ParseGlob("templates/*.html"))
	ctx := context.Background()
	// resetChan is passed all they way down to the sererstatus call
	// so it can report what the current reset is.
	// Not sure I like it.
	resetChan := make(chan string)
	jumpGateListUpdateChan := make(chan types.JumpGateAgentListStruct)
	if collect {
		c := collector.New(a.DB, "https://api.spacetraders.io/v2", resetChan, jumpGateListUpdateChan)
		go c.Run(ctx, time.Duration(collectEvery)*time.Minute)
	}
	go func(c chan types.JumpGateAgentListStruct) {
		for {
			a.JumpGateAgents = <-c
			slog.Debug("got new JumpGateAgents list", "agents_to_check", len(a.JumpGateAgents.AgentsToCheck))
		}
	}(jumpGateListUpdateChan)
	go func(c chan string) {
		for {
			a.Reset = <-c
			slog.Debug("got new reset", "date", a.Reset)
		}
	}(resetChan)
	return &a
}

// mergeAgents accepts a variable number of 'any' type arguments.
// It processes the arguments to collect strings:
// - If an argument is a string, it is split by comma, trimmed, and added.
// - If an argument is a []string (list of strings), its elements are appended.
// The function returns a deduplicated list of strings
func mergeAgents(args ...any) []string {

	// 'seen' map tracks elements already added to 'merged'
	seen := make(map[string]bool)

	for _, arg := range args {
		if arg == nil {
			continue // Skip nil arguments
		}

		// Use reflection to check the type of the argument
		v := reflect.ValueOf(arg)
		kind := v.Kind()

		switch kind {
		case reflect.String:
			// if its a string, treat it as comma separated
			s := v.String()
			for _, part := range strings.Split(s, ",") {
				trimmed := strings.TrimSpace(part)
				if trimmed != "" {
					seen[trimmed] = true
				}
			}

		case reflect.Slice:
			// Case 2: Argument is a slice. Check if it's a []string.
			if v.Type().Elem().Kind() == reflect.String {
				// Iterate over the slice elements and append them
				for i := 0; i < v.Len(); i++ {
					// v.Index(i).Interface() gets the element as 'any', then cast to string
					if s, ok := v.Index(i).Interface().(string); ok {
						trimmed := strings.TrimSpace(s)
						if trimmed != "" {
							seen[trimmed] = true
						}
					}
				}
			}
		default:
			continue
		}
	}

	merged := make([]string, 0, len(seen)*2)
	for e := range seen {
		merged = append(merged, e)
	}

	return merged
}
