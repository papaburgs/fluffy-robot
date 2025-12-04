package agentcache

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

// The duration after which the cache will be evicted if not accessed.
const EvictionDuration = 2 * time.Minute

// PublicAgent represents the structure found in the JSON files.
type PublicAgent struct {
	// Credits The number of credits the agent has available. Credits can be negative if funds have been overdrawn.
	Credits int64 `json:"credits"`
	// Headquarters The headquarters of the agent.
	Headquarters string `json:"headquarters"`
	// ShipCount How many ships are owned by the agent.
	ShipCount int `json:"shipCount"`
	// StartingFaction The faction the agent started with.
	StartingFaction string `json:"startingFaction"`
	// Symbol Symbol of the agent. This is used as the key for the cache.
	Symbol string `json:"symbol"`
}

// AgentRecord stores a snapshot of an agent's historical state.
type AgentRecord struct {
	// Timestamp is derived from the filename (e.g., data-1672531200.json).
	Timestamp time.Time
	Credits   int64
	ShipCount int
}

// AgentData is the final, in-memory cache structure:
// map[AgentSymbol][]HistoricalRecords
type AgentData map[string][]AgentRecord

// AgentProcessor manages the agent data cache and its eviction timer.
type AgentProcessor struct {
	cache         AgentData
	cacheLock     sync.RWMutex // Protects access to the cache and the timer
	evictionTimer *time.Timer
	dirPath       string
}

// NewAgentProcessor initializes the processor, loads the initial data, and starts the timer.
func NewAgentProcessor(dirPath string) (*AgentProcessor, error) {
	// ensure directory exists
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}
	if _, err = os.Stat(dirPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("directory not found: %s", dirPath)
	}

	p := &AgentProcessor{
		dirPath: dirPath,
	}

	return p, nil
}

// loadData reads all JSON files in the directory and populates the cache.
// It is called during initialization or if the cache is reloaded later.
func (p *AgentProcessor) loadData(resetDate string) error {
	start := time.Now()
	p.cache = make(AgentData)
	slog.Debug("loaddata start", "reset", resetDate)

	// Find all files matching the data-xxxxxx.json pattern
	files, err := filepath.Glob(filepath.Join(p.dirPath, "reset-"+resetDate, "agents-*.json"))
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no data files found in %s", p.dirPath)
	}

	for _, file := range files {
		base := filepath.Base(file)    // e.g., data-1672531200.json
		parts := base[7 : len(base)-5] // e.g., 1672531200

		timestampInt, err := strconv.ParseInt(parts, 10, 64)
		if err != nil {
			slog.Warn("Skipping file, invalid timestamp in name", "file", file, "error", err)
			continue
		}
		timestamp := time.Unix(timestampInt, 0)

		data, err := os.ReadFile(file)
		if err != nil {
			slog.Warn("Skipping file, failed to read", "file", file, "error", err)
			continue
		}

		var agents []PublicAgent
		if err := json.Unmarshal(data, &agents); err != nil {
			slog.Warn("Skipping file, failed to unmarshal", "file", file, "error", err)
			continue
		}

		for _, agent := range agents {
			record := AgentRecord{
				Timestamp: timestamp,
				Credits:   agent.Credits,
				ShipCount: agent.ShipCount,
			}
			p.cache[agent.Symbol] = append(p.cache[agent.Symbol], record)
		}
	}

	for _, records := range p.cache {
		sort.Slice(records, func(i, j int) bool {
			return records[i].Timestamp.Before(records[j].Timestamp)
		})
	}
	dur := time.Now().Sub(start)
	slog.Info("Cache generated", "duration", dur)

	return nil
}

// GetAgentRecords retrieves the historical records for a given agent symbol.
// Accessing this function resets the eviction timer.
func (p *AgentProcessor) GetAgentRecords(symbol string) ([]AgentRecord, error) {
	p.cacheLock.RLock()
	defer p.cacheLock.RUnlock()

	// Check if the cache has been evicted
	if p.cache == nil {
		return nil, fmt.Errorf("cache is empty, please call ReloadData() first")
	}

	// Reset the timer as the cache was accessed
	p.resetTimer()

	records, ok := p.cache[symbol]
	if !ok {
		return nil, fmt.Errorf("agent symbol %s not found in cache", symbol)
	}

	// Return a copy to prevent external modification of the cache slice
	recordsCopy := make([]AgentRecord, len(records))
	copy(recordsCopy, records)
	return recordsCopy, nil
}

// GetAllAgents returns a map of a string to a bool
// if true that means the agent looks like they are active, ie, not 175k
func (p *AgentProcessor) GetAllAgents() (map[string]bool, error) {
	p.cacheLock.RLock()
	defer p.cacheLock.RUnlock()

	res := make(map[string]bool)

	// Check if the cache has been evicted
	if p.cache == nil {
		return nil, fmt.Errorf("cache is empty, please call ReloadData() first")
	}

	// Reset the timer as the cache was accessed
	p.resetTimer()

	for symbol, recordList := range p.cache {
		res[symbol] = true
		if recordList[len(recordList)-1].Credits == 175000 {
			res[symbol] = false
		}
	}

	return res, nil
}

// evictCache is called when the timer expires. It releases memory by setting the cache to nil.
func (p *AgentProcessor) evictCache() {
	p.cacheLock.Lock()
	defer p.cacheLock.Unlock()

	if p.cache != nil {
		slog.Info("Cache evicted", "inactive minutes", EvictionDuration)
		p.cache = nil          // Release the memory
		p.evictionTimer.Stop() // Stop the timer if it hasn't stopped already
	}
}

// resetTimer stops the current timer (if active) and starts a new one.
// This must be called under a write lock or inside a function that already holds a lock (like GetAgentRecords using RLock).
// Note: This logic must be protected by the mutex, as the timer channel could be accessed concurrently.
func (p *AgentProcessor) resetTimer() {
	if p.evictionTimer != nil {
		// Attempt to stop the existing timer.
		// Drain the channel if Stop returns false (meaning the timer expired or was already stopped)
		if !p.evictionTimer.Stop() {
			select {
			case <-p.evictionTimer.C:
				// Timer channel drained successfully
			default:
				// Channel was empty
			}
		}
	}

	// Start a new timer
	p.evictionTimer = time.AfterFunc(EvictionDuration, p.evictCache)
	slog.Debug("Eviction timer reset", "timerDuration", EvictionDuration)
}

// ReloadData can be called manually to force a reload from the directory.
func (p *AgentProcessor) ReloadData(resetDate string) error {
	p.cacheLock.Lock()
	defer p.cacheLock.Unlock()

	if err := p.loadData(resetDate); err != nil {
		return err
	}
	p.resetTimer() // Reset timer after successful reload
	return nil
}

// IsCacheEvicted checks the status of the cache (for testing/monitoring purposes).
func (p *AgentProcessor) IsCacheEvicted() bool {
	p.cacheLock.RLock()
	defer p.cacheLock.RUnlock()
	return p.cache == nil
}
