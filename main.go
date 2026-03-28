package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"

	"github.com/papaburgs/fluffy-robot/internal/collector"
	"github.com/papaburgs/fluffy-robot/internal/datastore"
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

	c := collector.NewCollector(gate.New(2, gateBucketSize), baseURL)

	// tried this in an 'init' but it did not work as required
	datastore.Init()

	c.Run(context.Background())

	// at this point we would add the frontend, but need some more database calls before I can start that process
}
