package collector

import (
	"testing"
	"time"
)

func TestTickDuration_Defaults(t *testing.T) {
	// no env set
	g := NewCollector(nil, "http://x")
	if g.agentInterval != 5*time.Minute {
		t.Errorf("agentInterval = %v, want 5m", g.agentInterval)
	}
	if g.jumpgateInterval != 30*time.Minute {
		t.Errorf("jumpgateInterval = %v, want 30m", g.jumpgateInterval)
	}
	if g.constructionInterval != 4*time.Hour {
		t.Errorf("constructionInterval = %v, want 4h", g.constructionInterval)
	}
	if g.supplyInterval != 24*time.Hour {
		t.Errorf("supplyInterval = %v, want 24h", g.supplyInterval)
	}
}

func TestTickDuration_FromEnv(t *testing.T) {
	t.Setenv("FLUFFY_AGENT_TICK", "30s")
	t.Setenv("FLUFFY_JUMPGATE_TICK", "2m")
	t.Setenv("FLUFFY_CONSTRUCTION_TICK", "10m")
	t.Setenv("FLUFFY_SUPPLY_TICK", "30s")
	g := NewCollector(nil, "http://x")
	if g.agentInterval != 30*time.Second {
		t.Errorf("agentInterval = %v, want 30s", g.agentInterval)
	}
	if g.jumpgateInterval != 2*time.Minute {
		t.Errorf("jumpgateInterval = %v, want 2m", g.jumpgateInterval)
	}
	if g.constructionInterval != 10*time.Minute {
		t.Errorf("constructionInterval = %v, want 10m", g.constructionInterval)
	}
	if g.supplyInterval != 30*time.Second {
		t.Errorf("supplyInterval = %v, want 30s", g.supplyInterval)
	}
}

func TestTickDuration_InvalidFallsBack(t *testing.T) {
	t.Setenv("FLUFFY_AGENT_TICK", "banana")
	t.Setenv("FLUFFY_JUMPGATE_TICK", "-5m") // negative durations must not reach time.NewTicker
	t.Setenv("FLUFFY_SUPPLY_TICK", "banana")
	g := NewCollector(nil, "http://x")
	if g.agentInterval != 5*time.Minute {
		t.Errorf("invalid value: agentInterval = %v, want default 5m", g.agentInterval)
	}
	if g.jumpgateInterval != 30*time.Minute {
		t.Errorf("negative value: jumpgateInterval = %v, want default 30m", g.jumpgateInterval)
	}
	if g.supplyInterval != 24*time.Hour {
		t.Errorf("invalid value: supplyInterval = %v, want default 24h", g.supplyInterval)
	}
}
