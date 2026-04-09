package gate

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/logging"
	"github.com/papaburgs/fluffy-robot/internal/metrics"
)

type Gate struct {
	t1Ticker   *time.Ticker
	t60Ticker  *time.Ticker
	tCheck     *time.Ticker
	t1Limit    int
	t60Limit   int
	t1Count    int
	t60Count   int
	queue      *list.List
	queueMutex sync.Mutex
}

func New(t1Limit, t60Limit int) *Gate {
	var g = Gate{
		t1Ticker:   time.NewTicker(time.Second + (20 * time.Millisecond)),
		t60Ticker:  time.NewTicker(time.Minute),
		tCheck:     time.NewTicker(time.Millisecond * 20),
		t1Limit:    t1Limit,
		t60Limit:   t60Limit,
		t1Count:    0,
		t60Count:   0,
		queue:      &list.List{},
		queueMutex: sync.Mutex{},
	}
	g.queue.Init()
	go g.loop()
	return &g
}

func (g *Gate) loop() {
	ratelimit := 1
	for {
		select {
		case <-g.t1Ticker.C:
			g.queueMutex.Lock()
			g.t1Count = 0
			ratelimit = 1
			g.queueMutex.Unlock()
		case <-g.t60Ticker.C:
			g.queueMutex.Lock()
			g.t60Count = 0
			ratelimit = 1
			g.queueMutex.Unlock()
		case <-g.tCheck.C:
			g.queueMutex.Lock()
			node := g.queue.Front()
			if node != nil {
				if c, ok := node.Value.(chan bool); ok {
					switch {
					case g.t1Count < g.t1Limit:
						g.t1Count++
						metrics.GateT1Requests.Add(1)
						c <- true
						g.queue.Remove(node)
					case g.t60Count < g.t60Limit:
						if g.t60Count == 0 {
							g.t60Ticker.Reset(time.Minute)
						}
						g.t60Count++
						metrics.GateT60Requests.Add(1)
						c <- true
						g.queue.Remove(node)
					default:
						ratelimit++
						metrics.GateBlocked.Add(1)
						time.Sleep(time.Duration(ratelimit) * 100 * time.Millisecond)
					}
				} else {
					logging.Warn("Not sure what's going on here - type is not a chan bool")
					g.queue.Remove(node)
				}
			}
			g.queueMutex.Unlock()
		}
	}
}

func (g *Gate) Latch(ctx context.Context) {
	g.queueMutex.Lock()
	c := make(chan bool)
	g.queue.PushBack(c)
	g.queueMutex.Unlock()
	metrics.GateQueueLength.Set(int64(g.queue.Len()))
	select {
	case <-c:
		return
	case <-ctx.Done():
		fmt.Println("level=warn context cancelled")
	}
}

func (g *Gate) Lock(ctx context.Context) {
	g.queueMutex.Lock()
	g.t1Count = 9999
	g.t60Count = 99999
	metrics.GateLockCount.Add(1)
	g.queueMutex.Unlock()
}
