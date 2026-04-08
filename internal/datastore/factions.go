package datastore

import (
	"encoding/gob"
	"fmt"
)

func StoreFactions(fac []Faction) error {
	if currentReset == "" {
		fmt.Println("level=Error, msg=\"Current Reset is empty\"")
	}
	for i, k := range fac {
		k.Reset = currentReset
		fac[i] = k
	}
	return writeData("factions", 0, fac)
}

func loadFactions(thisReset Reset) error {
	// l := plog.With("function", "loadFactions")

	zeroTimer.Reset(cacheLifetime)
	// l.Debug("try to load factions", "reset", thisReset)

	for _, c := range loadedCaches {
		if c.reset == thisReset && c.cType == faction {
			fmt.Println("Cache built, this is noop")
			return nil
		}
	}
	// use readdata to get back a map of filename to byte buffers
	// NB use the . on the end so we don't get agentStatus files
	m, err := readData("factions.", thisReset)
	if err != nil {
		fmt.Println("Failed to read stats file: ", err)
		return err
	}

	if len(m) != 1 {
		fmt.Println("should only get one result got ", len(m))
		return fmt.Errorf("invalid read")
	}

	for _, b := range m {
		// make a new decoder on the buffer, which is a Reader
		gobDec := gob.NewDecoder(b)

		// try to decode the gob into the stats object
		if err := gobDec.Decode(&factions); err != nil {
			fmt.Println("error decoding gob: ", err)
			return err
		}
	}
	m = nil
	return nil
}

func GetFactions(thisReset Reset) []Faction {
	l := plog.With("function", "GetFactions")
	l.Debug("try to load factions", "reset", thisReset)
	if err := loadFactions(thisReset); err != nil {
		l.Error("error loading stats", "thisReset", thisReset, "error", err)
		return []Faction{}
	}
	f := make([]Faction, 50)
	for _, k := range factions {
		if k.Reset == thisReset {
			f = append(f, k)
		}
	}
	return f
}
