package datastore

import (
	"encoding/gob"
)

func LoadJumpgates() error {
	l := plog.With("function", "LoadJumpgates")
	zeroTimer.Reset(cacheLifetime)
	// noop if this is done already
	if len(JumpgatesBySystem) > 0 {
		return nil
	}

	m, err := readData("jumpgates.")
	if err != nil {
		l.Error("Failed to read data file", "error", err)
		return err
	}

	for k, b := range m {
		l.Debug("de-gobbing file", "filename", k)
		var v []JGInfo
		// make a new decoder on the buffer, which is a Reader
		gobDec := gob.NewDecoder(b)

		// try to decode the gob into an array of Agent, which is how its written
		if err := gobDec.Decode(&v); err != nil {
			l.Error("error decoding gob", "error", err)
			return err
		}
		for _, a := range v {
			JumpgatesBySystem[a.System] = a
		}
	}
	return nil
}

// UpdateJumpGates overwrites the current jumpgate data
// this one is not written directly from an api call, but is a generated one
// run after the agents are collected
// the collector builds these up as it correlates the agent headquarters, system, and jumpgate
func UpdateJumpGates(jg []JGInfo) {
	writeData("jumpgates", 0, jg)
}
