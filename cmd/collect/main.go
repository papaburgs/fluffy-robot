package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/db"
	"github.com/papaburgs/fluffy-robot/internal/gate"
	"github.com/papaburgs/fluffy-robot/internal/logging"
	"github.com/papaburgs/fluffy-robot/internal/types"
)

func main() {
	logging.InitLogger()
	l := slog.With("function", "main")

	database, err := db.Connect()
	if err != nil {
		l.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := db.InitSchema(database); err != nil {
		l.Error("failed to initialize schema", "error", err)
		os.Exit(1)
	}

	gateBucketSize, err := strconv.Atoi(os.Getenv("FLUFFY_GATE_BUCKET_SIZE"))
	if err != nil {
		l.Error("error parsing FLUFFY_GATE_BUCKET_SIZE, defaulting to 20", "error", err)
		gateBucketSize = 20
	}
	baseURL := "https://api.spacetraders.io/v2"

	c := Collector{
		db:      database,
		gate:    gate.New(2, gateBucketSize),
		baseURL: baseURL,
	}
	c.Run(context.Background())
}

func (c *Collector) Run(ctx context.Context) {
	jumpgateticker := time.NewTicker(30 * time.Minute)
	var err error
	c.ingest(ctx)
	err = c.updateJumpgates(ctx)
	if err != nil {
		slog.Error("Error running updateJumpgates")
	}
	time.Sleep(time.Minute)
	agentticker := time.NewTicker(5 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			return
		case <-agentticker.C:
			c.ingest(ctx)
		case <-jumpgateticker.C:
			err = c.updateJumpgates(ctx)
			if err != nil {
				slog.Error("Error running updateJumpgates")
			}
		}
	}
}

func (c *Collector) ingest(ctx context.Context) {
	slog.Info("starting data ingestion")

	c.currentTimestamp = time.Now().Unix()
	c.apiCalls = 0
	c.ingestStart = time.Now()
	if err := c.updateStatus(ctx); err != nil {
		slog.Error("failed to update status", "error", err)
		return
	}

	if err := c.updateAgents(ctx); err != nil {
		slog.Error("failed to update agents", "error", err)
	}
	slog.Info("data ingestion completed", "apiCalls", c.apiCalls, "duration", time.Now().Sub(c.ingestStart))
}

func (c *Collector) updateJumpgates(ctx context.Context) error {
	l := slog.With("function", "updateJumpgates")
	l.Info("start")

	c.currentTimestamp = time.Now().Unix()
	c.apiCalls = 0
	c.ingestStart = time.Now()

	rows, err := c.db.QueryContext(ctx, `
		SELECT symbol, headquarters, credits 
		FROM agents 
		WHERE (symbol, timestamp) IN (
			SELECT symbol, MAX(timestamp) 
			FROM agents 
			WHERE reset = ? 
			GROUP BY symbol
		)
	`, c.reset)
	if err != nil {
		return err
	}
	defer rows.Close()
	l.Debug("parsing query results")
	activeSystems := make(map[string]string) // system -> headquarters
	for rows.Next() {
		var symbol, hq string
		var credits int64
		if err := rows.Scan(&symbol, &hq, &credits); err != nil {
			continue
		}
		if credits != 175000 {
			// System is HQ minus last 3 chars
			if len(hq) > 3 {
				system := hq[:len(hq)-3]
				activeSystems[system] = hq
			}
		}
	}

	// Fetch existing jumpgates for this reset to avoid per-system queries
	type jgInfo struct {
		symbol   string
		complete int64
	}
	existingJGs := make(map[string]jgInfo)
	jgRows, err := c.db.QueryContext(ctx, "SELECT system, jumpgate, complete FROM jumpgates WHERE reset = ?", c.reset)
	if err == nil {
		for jgRows.Next() {
			var system, symbol string
			var complete int64
			if err := jgRows.Scan(&system, &symbol, &complete); err == nil {
				existingJGs[system] = jgInfo{symbol: symbol, complete: complete}
			}
		}
		jgRows.Close()
	}

	type jgToInsert struct {
		system, hq, symbol string
	}
	type constrToInsert struct {
		symbol         string
		fabmat, advcct int
	}
	type jgToComplete struct {
		symbol string
	}

	var inserts []jgToInsert
	var constructions []constrToInsert
	var completions []jgToComplete

	l.Debug("looking through activeSystems")
	for system, hq := range activeSystems {
		info, exists := existingJGs[system]
		jumpgateSymbol := info.symbol
		complete := info.complete

		if !exists {
			// Need to find jumpgate symbol for this system
			l.Debug("finding jumpgate for system", "system", system)
			var err error
			jumpgateSymbol, err = c.findJumpgateSymbol(ctx, system)
			if err != nil {
				slog.Error("failed to find jumpgate symbol", "system", system, "error", err)
				continue
			}
			inserts = append(inserts, jgToInsert{system: system, hq: hq, symbol: jumpgateSymbol})
			complete = 0
		}

		// If jumpgate is not complete, check construction status
		if complete == 0 {
			slog.Debug("checking construction status", "system", system, "jumpgate", jumpgateSymbol)
			status, err := c.fetchConstructionStatus(ctx, system, jumpgateSymbol)
			if err != nil {
				slog.Error("failed to fetch construction status", "jumpgate", jumpgateSymbol, "error", err)
				continue
			}

			// Update construction table
			var fabmat, advcct int
			for _, m := range status.Materials {
				if m.TradeSymbol == "FAB_MATS" {
					fabmat = m.Fulfilled
				} else if m.TradeSymbol == "ADVANCED_CIRCUITRY" {
					advcct = m.Fulfilled
				}
			}

			constructions = append(constructions, constrToInsert{symbol: jumpgateSymbol, fabmat: fabmat, advcct: advcct})

			// If complete, update jumpgates table
			if status.IsComplete {
				completions = append(completions, jgToComplete{symbol: jumpgateSymbol})
			}
		}
	}
	l.Debug("done scan")
	if len(inserts) == 0 && len(constructions) == 0 && len(completions) == 0 {
		l.Info("nothing to update")
		return nil
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if len(inserts) > 0 {
		l.Debug("Updating jumpgate inserts")
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO jumpgates (reset, system, headquarters, jumpgate, complete, activeagent)
			VALUES (?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, jg := range inserts {
			if _, err := stmt.ExecContext(ctx, c.reset, jg.system, jg.hq, jg.symbol, 0, true); err != nil {
				slog.Error("failed to insert jumpgate in batch", "system", jg.system, "error", err)
			}
		}
	}

	if len(constructions) > 0 {
		l.Debug("Updating jumpgate constructions")
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO construction (reset, timestamp, jumpgate, fabmat, advcct)
			VALUES (?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, cn := range constructions {
			if _, err := stmt.ExecContext(ctx, c.reset, c.currentTimestamp, cn.symbol, cn.fabmat, cn.advcct); err != nil {
				slog.Error("failed to insert construction in batch", "jumpgate", cn.symbol, "error", err)
			}
		}
	}

	if len(completions) > 0 {
		l.Debug("updating completions")
		stmt, err := tx.PrepareContext(ctx, "UPDATE jumpgates SET complete = ? WHERE reset = ? AND jumpgate = ?")
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, cp := range completions {
			if _, err := stmt.ExecContext(ctx, c.currentTimestamp, c.reset, cp.symbol); err != nil {
				slog.Error("failed to update jumpgate completion in batch", "jumpgate", cp.symbol, "error", err)
			}
		}
	}

	err = tx.Commit()
	if err == nil {
		slog.Info("jumpgate completed", "apiCalls", c.apiCalls, "duration", time.Now().Sub(c.ingestStart))
	}
	return err
}

func (c *Collector) findJumpgateSymbol(ctx context.Context, systemSymbol string) (string, error) {
	url := fmt.Sprintf("%s/systems/%s", c.baseURL, systemSymbol)
	resp, err := c.doGET(ctx, url)
	if err != nil {
		return "", err
	}

	var systemResponse struct {
		Data struct {
			Waypoints []struct {
				Symbol string `json:"symbol"`
				Type   string `json:"type"`
			} `json:"waypoints"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Bytes, &systemResponse); err != nil {
		return "", err
	}

	for _, w := range systemResponse.Data.Waypoints {
		if w.Type == "JUMP_GATE" {
			return w.Symbol, nil
		}
	}

	return "", fmt.Errorf("no jumpgate found in system %s", systemSymbol)
}

func (c *Collector) fetchConstructionStatus(ctx context.Context, systemSymbol, jumpgateSymbol string) (ConstructionStatus, error) {
	url := fmt.Sprintf("%s/systems/%s/waypoints/%s/construction", c.baseURL, systemSymbol, jumpgateSymbol)
	resp, err := c.doGET(ctx, url)
	if err != nil {
		return ConstructionStatus{}, err
	}

	var response struct {
		Data ConstructionStatus `json:"data"`
	}
	if err := json.Unmarshal(resp.Bytes, &response); err != nil {
		return ConstructionStatus{}, err
	}

	return response.Data, nil
}

func (c *Collector) updateStatus(ctx context.Context) error {
	slog.Debug("updating server status")

	resp, err := c.doGET(ctx, c.baseURL+"/")
	if err != nil {
		return err
	}

	var status ResponseStatus
	if err := json.Unmarshal(resp.Bytes, &status); err != nil {
		return err
	}

	// set this locally as we use it often
	c.reset = status.ResetDate

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Update stats table
	_, err = tx.ExecContext(ctx, `
		INSERT INTO stats (reset, marketUpdate, agents, accounts, ships, systems, waypoints, status, version, nextReset)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(reset) DO UPDATE SET
			marketUpdate = excluded.marketUpdate,
			agents = excluded.agents,
			accounts = excluded.accounts,
			ships = excluded.ships,
			systems = excluded.systems,
			waypoints = excluded.waypoints,
			status = excluded.status,
			version = excluded.version,
			nextReset = excluded.nextReset
	`,
		status.ResetDate,
		status.Health.LastMarketUpdate,
		status.Stats.Agents,
		status.Stats.Accounts,
		status.Stats.Ships,
		status.Stats.Systems,
		status.Stats.Waypoints,
		status.Status,
		status.Version,
		status.ServerResets.Next,
	)
	if err != nil {
		return fmt.Errorf("failed to update stats table: %w", err)
	}

	// Update leaderboard table
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO leaderboard (timestamp, reset, count, symbol, type)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(timestamp, symbol, type) DO NOTHING
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, l := range status.Leaderboards.MostCredits {
		_, err = stmt.ExecContext(ctx, c.currentTimestamp, c.reset, l.Credits, l.AgentSymbol, "credits")
		if err != nil {
			slog.Error("failed to insert credits leaderboard", "error", err, "symbol", l.AgentSymbol)
		}
	}

	for _, l := range status.Leaderboards.MostSubmittedCharts {
		_, err = stmt.ExecContext(ctx, c.currentTimestamp, c.reset, l.ChartCount, l.AgentSymbol, "charts")
		if err != nil {
			slog.Error("failed to insert charts leaderboard", "error", err, "symbol", l.AgentSymbol)
		}
	}

	return tx.Commit()
}

func (c *Collector) updateAgents(ctx context.Context) error {
	slog.Debug("updating agents")

	var allAgents []types.PublicAgent
	page := 1
	perPage := 20

	for {
		url := fmt.Sprintf("%s/agents?limit=%d&page=%d", c.baseURL, perPage, page)
		resp, err := c.doGET(ctx, url)
		if err != nil {
			return err
		}

		var data ResponseAgents
		if err := json.Unmarshal(resp.Bytes, &data); err != nil {
			return err
		}

		allAgents = append(allAgents, data.Data...)

		if page*perPage >= data.Meta.Total {
			break
		}
		page++
	}

	if len(allAgents) == 0 {
		return nil
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO agents (timestamp, reset, symbol, ships, faction, credits, headquarters)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, agent := range allAgents {
		_, err = stmt.ExecContext(ctx, c.currentTimestamp, c.reset, agent.Symbol, agent.ShipCount, agent.StartingFaction, agent.Credits, agent.Headquarters)
		if err != nil {
			slog.Error("failed to update agent in batch", "error", err, "symbol", agent.Symbol)
		}
	}

	return tx.Commit()
}

func (c *Collector) doGET(ctx context.Context, url string) (HTTPResponse, error) {
	var retries429 int
	var retriesOther int
	c.apiCalls++
	for {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return HTTPResponse{}, err
		}

		c.gate.Latch(ctx)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			retriesOther++
			if retriesOther >= 3 {
				return HTTPResponse{}, err
			}
			time.Sleep(time.Second)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return HTTPResponse{}, err
		}

		res := HTTPResponse{
			Bytes:      body,
			StatusCode: resp.StatusCode,
		}

		if resp.StatusCode == http.StatusOK {
			return res, nil
		}

		if resp.StatusCode == 429 {
			retries429++
			if retries429 >= 5 {
				return res, fmt.Errorf("received too many 429 errors")
			}
			c.gate.Lock(ctx)
			time.Sleep(time.Second)
			continue
		}

		// Handle 4xx or 5xx codes
		retriesOther++
		if retriesOther >= 3 {
			return res, fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
		}
		time.Sleep(time.Second)
	}
}

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

type ResponseAgents struct {
	Data []types.PublicAgent `json:"data"`
	Meta Meta                `json:"meta"`
}

type Meta struct {
	Limit int `json:"limit"`
	Page  int `json:"page"`
	Total int `json:"total"`
}

type ConstructionMaterial struct {
	TradeSymbol string `json:"tradeSymbol"`
	Required    int    `json:"required"`
	Fulfilled   int    `json:"fulfilled"`
}

type ConstructionStatus struct {
	Symbol     string                 `json:"symbol"`
	Materials  []ConstructionMaterial `json:"materials"`
	IsComplete bool                   `json:"isComplete"`
}

type HTTPResponse struct {
	Bytes      []byte
	StatusCode int
}

type Collector struct {
	db               *sql.DB
	baseURL          string
	gate             *gate.Gate
	reset            string
	currentTimestamp int64
	apiCalls         int
	ingestStart      time.Time
}
