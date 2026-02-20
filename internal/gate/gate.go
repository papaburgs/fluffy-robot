package gate

import (
	"container/list"
	"context"
	"log/slog"
	"sync"
	"time"
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

// var (
// 	t1Time    time.Duration = time.Second + 20*time.Millisecond
// 	t60Time   time.Duration = time.Minute
// 	checkTime time.Duration = 20 * time.Millisecond
// )

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
	l := slog.With("func", "Loop")
	ratelimit := 1
	for {
		select {
		case <-g.t1Ticker.C:
			// A second has passed, reset counter
			g.queueMutex.Lock()
			g.t1Count = 0
			ratelimit = 1
			g.queueMutex.Unlock()
		case <-g.t60Ticker.C:
			// A minute has passed, reset that counter
			g.queueMutex.Lock()
			g.t60Count = 0
			ratelimit = 1
			g.queueMutex.Unlock()
		case <-g.tCheck.C:
			// If the check timer hits, process the queue
			g.queueMutex.Lock()
			node := g.queue.Front()
			if node != nil {
				if c, ok := node.Value.(chan bool); ok {
					switch {
					case g.t1Count < g.t1Limit:
						g.t1Count++
						c <- true
						g.queue.Remove(node)
						// l.Debug("sending in t1")

					case g.t60Count < g.t60Limit:
						if g.t60Count == 0 {
							g.t60Ticker.Reset(time.Minute)
						}
						g.t60Count++
						c <- true
						g.queue.Remove(node)
						// l.Debug("sending in t60")
					default:
						// if we get here we are blocking the Latch until the second ticks over
						// we will go through the loop and if we fall back here, we increase wait time
						ratelimit++
						time.Sleep(time.Duration(ratelimit) * 100 * time.Millisecond)
					}
				} else {
					// this has happened once, before I added mutex, should not get here
					l.Warn("Not sure what's going on here - type is not a chan bool")
					g.queue.Remove(node)
				}
			}
			g.queueMutex.Unlock()
		}
	}
}

// Latch takes in a context and blocks until it is safe to proceed
// a channel is created and put on a queue.
// the loop function goes over that queue and sends a true back, this unblocks
// this function, it will return and the call can proceed
func (g *Gate) Latch(ctx context.Context) {
	g.queueMutex.Lock()
	c := make(chan bool)
	g.queue.PushBack(c)
	g.queueMutex.Unlock()
	select {
	case <-c:
		return
	case <-ctx.Done():
		slog.Warn("context cancelled")
	}
}


// if we get a 429, we will lock the gate, meaning no one gets through until they reset
func (g *Gate) Lock(ctx context.Context) {
	g.queueMutex.Lock()
	g.t1Count = 9999
	g.t60Count = 99999
	g.queueMutex.Unlock()
}
