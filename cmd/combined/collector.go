package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/datastore"
	"github.com/papaburgs/fluffy-robot/internal/gate"
)

type Collector struct {
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

var epochStart = errors.New("epoch reset detected")

func (c *Collector) Run(ctx context.Context) {
	l := slog.With("function", "Run")

	var err error
	err = c.updateStatusAgents(ctx)
	if err != nil {
		slog.Error("Error running updateAgents", "error", err)
	}
	l.Info("agents should be updated, test variable builders")
	time.Sleep(time.Second)
	begin := time.Now()
	datastore.LoadAgents(c.reset)
	datastore.LoadAgentHistory(c.reset)

	l.Info("done loading content, _charts_ can now use them")
	fmt.Println("------")
	fmt.Println(datastore.Agents["BURG"])
	fmt.Println("------")
	fmt.Println(datastore.AgentShipHistory["BURG"])
	fmt.Println("------")
	l.Info("that took some time", "elapsed", time.Now().Sub(begin))
	
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
	time.Sleep(5 * time.Minute)
	c.agentTicker = time.NewTicker(5 * time.Minute)
	c.jumpgateTicker = time.NewTicker(30 * time.Minute)
	c.constTicker = time.NewTicker(4 * 60 * time.Minute)
	resetTimer := time.NewTimer(7 * 24 * time.Hour)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.agentTicker.C:
			l.Info("agent ticker emit")
			err := c.updateStatusAgents(ctx)
			if err != nil {
				l.Error("Error running updateAgents", "error", err)
			}
			// set the resetTimer to the nextReset time minus 3 minutes, to give us a buffer to finish before the reset happens
			timeUntilReset := time.Until(c.nextReset.Add(-3 * time.Minute))
			if timeUntilReset > 0 {
				resetTimer.Reset(timeUntilReset)
			}
		case <-c.jumpgateTicker.C:
			// err = c.updateJumpgates(ctx)
			// if err != nil {
			// 	l.Error("Error running updateJumpgates")
			// }
		case <-c.constTicker.C:
			// err = c.updateInactiveJumpgates(ctx)
			// if err != nil {
			// 	l.Error("Error running updateJumpgates")
			// }
		case <-resetTimer.C:
			l.Info("reset timer emit, stopping tickers doing one last check and then looping until reset is complete")
			c.agentTicker.Stop()
			c.jumpgateTicker.Stop()
			c.constTicker.Stop()
			err := c.updateStatusAgents(ctx)
			if err != nil {
				l.Error("Error running updateAgents", "error", err)
			}

			l.Info("checking for reset, this may take a while...")
			time.Sleep(3 * time.Minute)
			if err := c.loopAtReset(ctx); err != nil {
				l.Error("Error in loopAtReset", "error", err)
			}
			c.agentTicker = time.NewTicker(5 * time.Minute)
			c.jumpgateTicker = time.NewTicker(30 * time.Minute)
			c.constTicker = time.NewTicker(4 * 60 * time.Minute)
			l.Info("restarted tickers")
		}
	}
}

// loopAtReset will sleep a minute and then hit the status endpoint
// until it looks like the reset is complete.
// these must be true in order to return includes:
//
//	the reset date is today
//	the leaderboard is mostly empty
func (c *Collector) loopAtReset(ctx context.Context) error {
	l := slog.With("function", "loopAtReset")

	for {
		l.Info("Sleeping")
		time.Sleep(time.Minute)
		l.Info("Checking")
		req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/", nil)
		if err != nil {
			l.Error("Error creating request", "error", err)
			return err
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			l.Warn("Error on call")
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			l.Error("Error reading body", "response", resp)
			continue
		}

		if resp.StatusCode != 200 {
			l.Info("Non-200 error, probably ok", "statusCode", resp.StatusCode)
			continue
		}

		// at this point we have a 200, now make sure everything else is ok

		var status datastore.ResponseStatus
		if err := json.Unmarshal(body, &status); err != nil {
			l.Error("error unmarshalling, that is weird", "body", string(body))
			continue
		}
		// compare the returned ResetDate, it should be today, which is a string like "2006-01-02"
		l.Debug("checking reset date", "resetDate", status.ResetDate, "today", time.Now().Format("2006-01-02"))
		if !strings.HasPrefix(status.ResetDate, time.Now().Format("2006-01-02")) {
			l.Info("reset date is not today, probably not reset yet", "resetDate", status.ResetDate)
			continue
		}

		// check the leaderboard, the most submitted charts should be empty
		if len(status.Leaderboards.MostSubmittedCharts) > 0 {
			l.Info("chart leaderboard is not empty, probably not reset yet", "mostSubmittedCharts", status.Leaderboards.MostSubmittedCharts)
			continue
		}

		if len(status.Leaderboards.MostSubmittedCharts) == 0 {
			l.Info("Credit leaderboard is empty, probably ready to go, sleep 1 more minute")
			time.Sleep(time.Minute)
			return nil
		}

		if status.Leaderboards.MostCredits != nil && len(status.Leaderboards.MostCredits) > 0 {
			if status.Leaderboards.MostCredits[0].Credits < 500000 {
				l.Info("Credit leaderboard is not empty but low, probably ready to go, sleep 1 more minute")
				time.Sleep(time.Minute)
				return nil
			}
		}
	}
}
