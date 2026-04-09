package main

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/collector"
	"github.com/papaburgs/fluffy-robot/internal/datastore"
	"github.com/papaburgs/fluffy-robot/internal/frontend"
	"github.com/papaburgs/fluffy-robot/internal/gate"
	"github.com/papaburgs/fluffy-robot/internal/logging"
)

func main() {
	logging.InitLogger()

	gateBucketSize, err := strconv.Atoi(os.Getenv("FLUFFY_GATE_BUCKET_SIZE"))
	if err != nil {
		logging.Error("error parsing FLUFFY_GATE_BUCKET_SIZE, defaulting to 20", "error", err)
		gateBucketSize = 20
	}
	baseURL := "https://api.spacetraders.io/v2"

	c := collector.NewCollector(gate.New(2, gateBucketSize), baseURL)

	datastore.Init()
	time.Sleep(time.Second)
	go c.Run(context.Background())

	time.Sleep(2 * time.Second)
	frontend.StartServer()
}
