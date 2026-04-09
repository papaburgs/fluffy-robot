package datastore

import (
	"encoding/gob"
	"fmt"

	"github.com/papaburgs/fluffy-robot/internal/logging"
)

func StoreFactions(fac []Faction) error {
	if currentReset == "" {
		logging.Error("Current Reset is empty")
	}
	for i, k := range fac {
		k.Reset = currentReset
		fac[i] = k
	}
	return writeData("factions", 0, fac)
}

func GetFactions(thisReset Reset) ([]Faction, error) {
	res := []Faction{}
	m, err := readData("factions.", thisReset)
	if err != nil {
		logging.Error("Failed to read factions file:", err)
		return res, err
	}

	if len(m) != 1 {
		return res, fmt.Errorf("more than one file returned")
	}

	for _, b := range m {
		gobDec := gob.NewDecoder(b)
		if err := gobDec.Decode(&res); err != nil {
			logging.Error("error decoding gob:", err)
			return res, err
		}
	}
	m = nil
	return res, nil
}
