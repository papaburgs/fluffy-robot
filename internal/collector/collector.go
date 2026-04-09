package collector

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	ds "github.com/papaburgs/fluffy-robot/internal/datastore"
	"github.com/papaburgs/fluffy-robot/internal/gate"
	"github.com/papaburgs/fluffy-robot/internal/logging"
	"github.com/papaburgs/fluffy-robot/internal/metrics"
)

type Collector struct {
	baseURL          string
	gate             *gate.Gate
	currentReset     ds.Reset
	nextReset        time.Time
	currentTimestamp int64
	apiCalls         int
	ingestStart      time.Time
	filterRegexes    []*regexp.Regexp
	agentTicker      *time.Ticker
	constTicker      *time.Ticker
	jumpgateTicker   *time.Ticker
}

func NewCollector(gate *gate.Gate, baseURL string) *Collector {
	c := Collector{
		gate:    gate,
		baseURL: baseURL,
	}
	return &c
}

func (c *Collector) Run(ctx context.Context) {
	var err error

	err = c.updateStatus(ctx)
	if err != nil {
		logging.Error("Error running updateStatus", err)
	}

	c.agentTicker = time.NewTicker(5 * time.Minute)
	c.jumpgateTicker = time.NewTicker(30 * time.Minute)
	c.constTicker = time.NewTicker(4 * 60 * time.Minute)
	resetTimer := time.NewTimer(7 * 24 * time.Hour)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.agentTicker.C:
			// logging.Debug("agent ticker emit")
			err := c.updateStatus(ctx)
			if err != nil {
				logging.Error("Error running updateAgents", err)
			}
			err = c.updateAgents(ctx)
			if err != nil {
				logging.Error("Error running updateStatus", err)
			}
			timeUntilReset := time.Until(c.nextReset.Add(-3 * time.Minute))
			if timeUntilReset > 0 {
				resetTimer.Reset(timeUntilReset)
			}
		case <-c.jumpgateTicker.C:
			err = c.updateJumpgates(ctx)
			if err != nil {
				logging.Error("Error running updateJumpgates")
			}
		case <-c.constTicker.C:
			err = c.updateInactiveJumpgates(ctx)
			if err != nil {
				logging.Error("Error running updateJumpgates")
			}
		case <-resetTimer.C:
			logging.Info("reset timer emit, stopping tickers doing one last check and then looping until reset is complete")
			c.agentTicker.Stop()
			c.jumpgateTicker.Stop()
			c.constTicker.Stop()
			metrics.CollectorResetDetections.Add(1)
			err := c.updateStatus(ctx)
			if err != nil {
				logging.Error("Error running updateAgents", err)
			}

			logging.Info("checking for reset, this may take a while...")
			time.Sleep(3 * time.Minute)
			if err := c.loopAtReset(ctx); err != nil {
				logging.Error("Error in loopAtReset", err)
			}
			c.agentTicker = time.NewTicker(5 * time.Minute)
			c.jumpgateTicker = time.NewTicker(30 * time.Minute)
			c.constTicker = time.NewTicker(4 * 60 * time.Minute)
			logging.Info("restarted tickers")
		}
	}
}

func (c *Collector) loopAtReset(ctx context.Context) error {
	for {
		logging.Info("Sleeping")
		time.Sleep(time.Minute)
		logging.Info("Checking")
		req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/", nil)
		if err != nil {
			logging.Error("Error creating request", err)
			return err
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			logging.Warn("Error on call")
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			logging.Error("Error reading body", resp)
			continue
		}

		if resp.StatusCode != 200 {
			logging.Info("Non-200 error, probably ok", resp.StatusCode)
			continue
		}

		var status ds.ResponseStatus
		if err := json.Unmarshal(body, &status); err != nil {
			logging.Error("error unmarshalling", string(body))
			continue
		}
		// logging.Debug("checking reset date", "resetDate", status.ResetDate, "today", time.Now().Format("2006-01-02"))
		if !strings.HasPrefix(status.ResetDate, time.Now().Format("2006-01-02")) {
			logging.Info("reset date is not today", status.ResetDate)
			continue
		}

		if len(status.Leaderboards.MostSubmittedCharts) > 0 {
			logging.Info("chart leaderboard is not empty")
			continue
		}

		if len(status.Leaderboards.MostSubmittedCharts) == 0 {
			logging.Info("Credit leaderboard is empty, probably ready to go, sleep 1 more minute")
			time.Sleep(time.Minute)
			return nil
		}

		if status.Leaderboards.MostCredits != nil && len(status.Leaderboards.MostCredits) > 0 {
			if status.Leaderboards.MostCredits[0].Credits < 500000 {
				logging.Info("Credit leaderboard not empty but low, probably ready to go, sleep 1 more minute")
				time.Sleep(time.Minute)
				return nil
			}
		}
	}
}
