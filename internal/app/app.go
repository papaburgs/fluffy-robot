package app

import (
	"context"
	"encoding/csv"
	"html/template"
	"io" // Added this import
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/agentcollector"
)

type AgentRecord struct {
	Timestamp time.Time
	ShipCount int
	Credits   int
}

type PublicAgent struct {
	// Credits The number of credits the agent has available. Credits can be negative if funds have been overdrawn.
	Credits int64 `json:"credits"`
	// Headquarters The headquarters of the agent.
	Headquarters string `json:"headquarters"`
	// ShipCount How many ships are owned by the agent.
	ShipCount int `json:"shipCount"`
	// StartingFaction The faction the agent started with.
	StartingFaction string `json:"startingFaction"`
	// Symbol Symbol of the agent.
	Symbol string `json:"symbol"`
}

type JumpGateAgentListStruct struct {
	AgentsToCheck  []PublicAgent `json:"agents_to_check"`
	AgentsToIgnore []PublicAgent `json:"agents_to_ignore"`
}

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
func (a *App) GetAgentRecordsFromCSV(symbol string, duration time.Duration) ([]AgentRecord, error) {
	records := []AgentRecord{}
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

		records = append(records, AgentRecord{
			Timestamp: recordTime,
			Credits:   csvRecord.Credits,
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

// App is our main application
// it holds some
type App struct {
	StorageRoot string
	// Reset is on the server status page, we use it to sort data
	Reset string
	// collectPointsPerHour is used to change the charts density
	collectPointsPerHour int
	JumpGateAgents       JumpGateAgentListStruct // New field for in-memory jumpgate agent lists
	t                    *template.Template
}

// NewApp starts the collector is collect is true
// it returns an app that contains all the handlers
// for the ui
func NewApp(storage string, collect bool) *App {
	var collectEvery = 5
	)
	a := App{
		StorageRoot:          storage,
		Reset:                "00000",
		collectPointsPerHour: 60 / collectEvery,
	}
	a.t = template.Must(template.ParseGlob("templates/*.html"))
	ctx := context.Background()
	// resetChan is passed all they way down to the sererstatus call
	// so it can report what the current reset is.
	// Not sure I like it.
	resetChan := make(chan string)
	jumpGateListUpdateChan := make(chan JumpGateAgentListStruct)
	if collect {
		go agentcollector.Init(ctx, "https://api.spacetraders.io/v2", a.StorageRoot, collectEvery, resetChan, jumpGateListUpdateChan, a.JumpGateAgents)
	}
	go func(c chan JumpGateAgentListStruct) {
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
