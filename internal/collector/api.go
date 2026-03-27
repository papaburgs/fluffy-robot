package collector

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/datastore"
)

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


type ResponseAgents struct {
	Data []datastore.PublicAgent `json:"data"`
	Meta Meta                    `json:"meta"`
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
