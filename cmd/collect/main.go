package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
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

	c.filterRegexes = []*regexp.Regexp{}
	val, ok := os.LookupEnv("FLUFFY_AGENT_IGNORE_FILTERS")
	if ok {
		rawPatterns := strings.Split(val, ";")

		for _, p := range rawPatterns {
			// Clean up whitespace in case of "regex1; regex2"
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}

			re, err := regexp.Compile(p)
			if err != nil {
				// Log the error but keep going so one bad regex doesn't kill the app
				l.Error("Error compiling regex", "pattern", p, "error", err)
				continue
			}
			slog.Info("Adding agent ignore filter regex", "pattern", p)

			c.filterRegexes = append(c.filterRegexes, re)
		}
	}

	c.Run(context.Background())
}

func (c *Collector) Run(ctx context.Context) {
	l := slog.With("function", "Run")

	var err error
	// err = c.updateStatusAgents(ctx)
	// if err != nil {
	// 	slog.Error("Error running updateAgents", "error", err)
	// }
	// l.Warn("sleeping before first jumpgate update")
	// time.Sleep(1 * time.Minute)
	// err = c.updateInactiveJumpgates(ctx)
	// if err != nil {
	// 	slog.Error("Error running updateInactiveJumpgates", "error", err)
	// }
	// l.Warn("sleeping before second jumpgate update")
	// time.Sleep(1 * time.Minute)
	// err = c.updateJumpgates(ctx)
	// time.Sleep(1 * time.Minute)
	// if err != nil {
	// 	slog.Error("Error running updateAgents", "error", err)
	// }
	c.agentTicker = time.NewTicker(5 * time.Minute)
	c.jumpgateTicker = time.NewTicker(30 * time.Minute)
	c.constTicker = time.NewTicker(4 * 60 * time.Minute)
	resetTimer := time.NewTimer(7 * 24 * time.Hour)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.agentTicker.C:
			err := c.updateStatusAgents(ctx)
			if err != nil {
				l.Error("Error running updateAgents", "error", err)
			}
			// set the resetTimer to the nextReset time minus 1 min
			timeUntilReset := time.Until(c.nextReset.Add(-1 * time.Minute))
			if timeUntilReset > 0 {
				resetTimer.Reset(timeUntilReset)
			}
		case <-c.jumpgateTicker.C:
			err = c.updateJumpgates(ctx)
			if err != nil {
				l.Error("Error running updateJumpgates")
			}
		case <-c.constTicker.C:
			err = c.updateInactiveJumpgates(ctx)
			if err != nil {
				l.Error("Error running updateJumpgates")
			}
		case <-resetTimer.C:
			c.agentTicker.Stop()
			c.jumpgateTicker.Stop()
			c.constTicker.Stop()
			time.Sleep(15 * time.Minute)
			c.agentTicker = time.NewTicker(5 * time.Minute)
			c.jumpgateTicker = time.NewTicker(30 * time.Minute)
			c.constTicker = time.NewTicker(4 * 60 * time.Minute)
		}
	}
}

var epochStart = errors.New("epoch reset detected")

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
	nextReset        time.Time
	currentTimestamp int64
	apiCalls         int
	ingestStart      time.Time
	filterRegexes    []*regexp.Regexp
	agentTicker      *time.Ticker
	constTicker      *time.Ticker
	jumpgateTicker   *time.Ticker
}
