package main

import (
	"context"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/papaburgs/fluffy-robot/internal/gate"
	"github.com/papaburgs/fluffy-robot/internal/logging"
)

func main() {
	logging.InitLogger()
	l := slog.With("function", "main")

	gateBucketSize, err := strconv.Atoi(os.Getenv("FLUFFY_GATE_BUCKET_SIZE"))
	if err != nil {
		l.Error("error parsing FLUFFY_GATE_BUCKET_SIZE, defaulting to 20", "error", err)
		gateBucketSize = 20
	}
	baseURL := "https://api.spacetraders.io/v2"

	c := Collector{
		gate:    gate.New(2, gateBucketSize),
		baseURL: baseURL,
	}

	// setup the ignore filters in case someone is spamming with new agents
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
