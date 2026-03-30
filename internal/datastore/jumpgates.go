package datastore

import (
	"encoding/gob"
)

// LoadJumpgates reads all the jumpgates in a reset and builds the jumpgateLists variable
// this is a map of reset to the list of jumpgates
// exported functions will filter and convert this list as needed.
func LoadJumpgates(thisReset string) error {
	l := plog.With("function", "LoadJumpgates")
	zeroTimer.Reset(cacheLifetime)
	// noop if this is done already
	if len(jumpgatesBySystem) > 0 {
		return nil
	}

	m, err := readData("jumpgates.", thisReset)
	if err != nil {
		l.Error("Failed to read data file", "error", err)
		return err
	}

	for k, b := range m {
		l.Debug("de-gobbing file", "filename", k)
		var v []JGInfo
		// make a new decoder on the buffer, which is a Reader
		gobDec := gob.NewDecoder(b)

		if err := gobDec.Decode(&v); err != nil {
			l.Error("error decoding gob", "error", err)
			return err
		}
		jumpgateLists[thisReset] = v
	}
	return nil
}

// Jumpgates is here, but what does it do?
func Jumpgates(thisReset string) map[string]JGInfo {
	if err := LoadJumpgates(); err != nil {
		plog.Error("error loading jumpgates", "thisReset", thisReset, "error", err)
		return nil
	}
	jumpgates[thisReset]
}

// func JumpgatesUnderConst() map[string]JGInfo {
// 	return jumpgatesUnderConst
// }

// UpdateJumpGates overwrites the current jumpgate data with the provided list
// this data file is not from and api call, but built from other conditions
// run after the agents are collected or updated
// the collector builds these up as it correlates the agent headquarters, system, and jumpgate
func UpdateJumpGates(jgList []JGInfo) {
	plog.Info("Writing updated jumpgates", "function", "UpdateJumpGates")
	writeData("jumpgates", 0, jgList)
}

// MarkJumpgatesComplete updates the internal map,
// marking those in the array as complete
// Then writes the updated list to disk
func MarkJumpgatesComplete(jgs []string, ts int64) {
	for _, j := range jgs {
		jg := jumpgatesBySystem[j]
		jg.Status = Complete
		jg.Complete = ts
		jumpgatesBySystem[j] = jg
	}
	jgList := []JGInfo{}
	for _, j := range jumpgatesBySystem {
		jgList = append(jgList, j)
	}
	UpdateJumpGates(jgList)
}

// MarkJumpgatesStarted updates the internal map,
// marking those in the array as under construction
// Then writes the updated list to disk
func MarkJumpgatesStarted(jgs []string) {
	for _, j := range jgs {
		jg := jumpgatesBySystem[j]
		jg.Status = Const
		jumpgatesBySystem[j] = jg
	}
	jgList := []JGInfo{}
	for _, j := range jumpgatesBySystem {
		jgList = append(jgList, j)
	}
	UpdateJumpGates(jgList)
}

func AddConstructions(cList []JGConstruction, ts int64) {
	writeData("construction", ts, cList)
}
