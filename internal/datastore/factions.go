package datastore

import (
	"encoding/gob"
	"fmt"
)

func StoreFactions(f []Faction) error {
	return writeData("factions", 0, f)
}

func loadFactions(thisReset Reset) error {
	l := plog.With("function", "loadFactions")

	zeroTimer.Reset(cacheLifetime)
	l.Debug("try to load factions", "reset", thisReset)
	if stats[thisReset].Reset != "" {
		l.Info("Cache built, this is noop")
		return nil
	}
	// use readdata to get back a map of filename to byte buffers
	// NB use the . on the end so we don't get agentStatus files
	m, err := readData("factions.", "")
	if err != nil {
		l.Error("Failed to read stats file", "error", err)
		return err
	}

	if len(m) != 1 {
		l.Error("should only get one result", "count", len(m))
		return fmt.Errorf("invalid read")
	}

	for k, b := range m {
		l.Debug("de-gobbing file", "filename", k)
		var v []Faction
		// make a new decoder on the buffer, which is a Reader
		gobDec := gob.NewDecoder(b)

		// try to decode the gob into the stats object
		if err := gobDec.Decode(&v); err != nil {
			l.Error("error decoding gob", "error", err)
			return err
		}
		factionLists[thisReset] = v
	}
	return nil
}

func GetFactions(thisReset Reset) []Faction {
	l := plog.With("function", "GetFactions")
	l.Debug("try to load factions", "reset", thisReset)
	if err := loadStats(thisReset); err != nil {
		l.Error("error loading stats", "thisReset", thisReset, "error", err)
		return Stats{}
	}
	return factionLists[thisReset]
}
