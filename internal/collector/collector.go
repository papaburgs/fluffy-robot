package collector

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	ds "github.com/papaburgs/fluffy-robot/internal/datastore"
	"github.com/papaburgs/fluffy-robot/internal/gate"
	"github.com/papaburgs/fluffy-robot/internal/logging"
	"github.com/papaburgs/fluffy-robot/internal/metrics"
)

type Collector struct {
	baseURL              string
	gate                 *gate.Gate
	currentReset         ds.Reset
	nextReset            time.Time
	currentTimestamp     int64
	apiCalls             int
	ingestStart          time.Time
	filterRegexes        []*regexp.Regexp
	agentTicker          *time.Ticker
	constTicker          *time.Ticker
	jumpgateTicker       *time.Ticker
	supplyTicker         *time.Ticker
	agentInterval        time.Duration
	jumpgateInterval     time.Duration
	constructionInterval time.Duration
	supplyInterval       time.Duration
}

// tickDuration reads a Go duration from an env var, falling back to the
// default on unset/invalid/non-positive values (a bad value must never
// reach time.NewTicker).
func tickDuration(env string, def time.Duration) time.Duration {
	raw, ok := os.LookupEnv(env)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		logging.Warn("invalid "+env+", using default", "value", raw, "default", def)
		return def
	}
	return d
}

func NewCollector(gate *gate.Gate, baseURL string) *Collector {
	c := Collector{
		gate:    gate,
		baseURL: baseURL,
	}
	c.agentInterval = tickDuration("FLUFFY_AGENT_TICK", 5*time.Minute)
	c.jumpgateInterval = tickDuration("FLUFFY_JUMPGATE_TICK", 30*time.Minute)
	c.constructionInterval = tickDuration("FLUFFY_CONSTRUCTION_TICK", 4*time.Hour)
	c.supplyInterval = tickDuration("FLUFFY_SUPPLY_TICK", 24*time.Hour)
	return &c
}

func (c *Collector) Run(ctx context.Context) {
	var err error

	err = c.updateStatus(ctx)
	if err != nil {
		logging.Error("Error running updateStatus", err)
	}

	// Fetch the static supply chain once at startup so a fresh DB populates
	// within seconds of boot; a no-op once collected. Non-fatal on error.
	if err := c.updateSupplyChain(ctx); err != nil {
		logging.Error("Error running updateSupplyChain", err)
	}

	c.agentTicker = time.NewTicker(c.agentInterval)
	c.jumpgateTicker = time.NewTicker(c.jumpgateInterval)
	c.constTicker = time.NewTicker(c.constructionInterval)
	c.supplyTicker = time.NewTicker(c.supplyInterval)
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
		case <-c.supplyTicker.C:
			err = c.updateSupplyChain(ctx)
			if err != nil {
				logging.Error("Error running updateSupplyChain", err)
			}
		case <-resetTimer.C:
			logging.Info("reset timer emit, stopping tickers doing one last check and then looping until reset is complete")
			c.agentTicker.Stop()
			c.jumpgateTicker.Stop()
			c.constTicker.Stop()
			c.supplyTicker.Stop()
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
			c.agentTicker = time.NewTicker(c.agentInterval)
			c.jumpgateTicker = time.NewTicker(c.jumpgateInterval)
			c.constTicker = time.NewTicker(c.constructionInterval)
			c.supplyTicker = time.NewTicker(c.supplyInterval)
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
			// The new reset has begun: drop the week that just became the
			// 6th-oldest and vacuum, while tickers are stopped.
			if err := ds.Prune(ds.RetentionWeeks(), ds.Reset(status.ResetDate)); err != nil {
				logging.Error("prune failed", err)
			}
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
