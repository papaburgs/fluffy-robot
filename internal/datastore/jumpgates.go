package datastore

import (
	"encoding/gob"
	"time"
)

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
	l := plog.With("function", "MarkJumpgatesStarted")

	if err := loadJumpgates(currentReset); err != nil {
		l.Error("error loading current jumpgates")
	}
	updated := []JGInfo{}
	for _, j := range jumpgateLists[currentReset] {
		rec := j
		for _, k := range jgs {
			if j.System == k {
				rec.Status = Complete
				rec.Complete = ts
			}
		}
		updated = append(updated, rec)
	}
	UpdateJumpGates(updated)
}

// MarkJumpgatesStarted updates the internal map,
// marking those systems in the array as under construction
// Then writes the updated list to disk
func MarkJumpgatesStarted(jgs []string) {
	l := plog.With("function", "MarkJumpgatesStarted")

	if err := loadJumpgates(currentReset); err != nil {
		l.Error("error loading current jumpgates")
	}
	updated := []JGInfo{}
	for _, j := range jumpgateLists[currentReset] {
		rec := j
		for _, k := range jgs {
			if j.System == k {
				rec.Status = Const
			}
		}
		updated = append(updated, rec)
	}
	UpdateJumpGates(updated)
}

func AddConstructions(cList []JGConstruction, ts int64) {
	writeData("construction", ts, cList)
}

// LoadConstructions reads all the construction data in a reset
// and builds the construcionsLists map entry for the provided reset
// exported functions will filter and convert this list as needed.
func loadConstructions(thisReset Reset) error {
	l := plog.With("function", "LoadJumpgates")
	zeroTimer.Reset(cacheLifetime)
	// noop if this is done already
	if len(constructionsLists[thisReset]) > 0 {
		return nil
	}

	m, err := readData("construction-", thisReset)
	if err != nil {
		l.Error("Failed to read data file", "error", err)
		return err
	}

	for k, b := range m {
		l.Debug("de-gobbing file", "filename", k)
		var v []JGConstruction
		gobDec := gob.NewDecoder(b)

		if err := gobDec.Decode(&v); err != nil {
			l.Error("error decoding gob", "error", err)
			return err
		}
		constructionsLists[thisReset] = append(constructionsLists[thisReset], v...)
	}
	l.Debug("Constructions built", "count", len(constructionsLists))
	return nil
}

// LoadJumpgates reads all the jumpgates in a reset
// and builds the jumpageLists map entry for the provided reset
// exported functions will filter and convert this list as needed.
func loadJumpgates(thisReset Reset) error {
	l := plog.With("function", "LoadJumpgates")
	zeroTimer.Reset(cacheLifetime)
	// noop if this is done already
	if len(jumpgateLists[thisReset]) > 0 {
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

// GetJumpgates makes a map of system to the JGInfo
func GetJumpgates(thisReset Reset) map[string]JGInfo {
	if err := loadJumpgates(thisReset); err != nil {
		plog.Error("error loading jumpgates", "thisReset", thisReset, "error", err)
		return nil
	}
	res := make(map[string]JGInfo)
	for _, j := range jumpgateLists[thisReset] {
		res[j.System] = j
	}
	return res
}

// Jumpgates is here, think it is used to get a list of all jumpgates in the systems
func GetJumpgatesUnderConst(thisReset Reset) map[string]JGInfo {
	if err := loadJumpgates(thisReset); err != nil {
		plog.Error("error loading jumpgates", "thisReset", thisReset, "error", err)
		return nil
	}
	res := make(map[string]JGInfo)
	for _, j := range jumpgateLists[thisReset] {
		if j.Status == Const {
			res[j.System] = j
		}
	}
	return res
}

// GetJumpgatesNotStarted retuns jumpgates that have an active agent in the system
// This does not return any other Status other than Active
func GetJumpgatesNotStarted(thisReset Reset) map[string]JGInfo {
	if err := loadJumpgates(thisReset); err != nil {
		plog.Error("error loading jumpgates", "thisReset", thisReset, "error", err)
		return nil
	}
	res := make(map[string]JGInfo)
	for _, j := range jumpgateLists[thisReset] {
		if j.Status == Active {
			res[j.System] = j
		}
	}
	return res
}

// want to move these to types later, but I don't like these types,

type ConstructionRecord struct {
	Timestamp int64
	Fabmat    int
	Advcct    int
}

// type ConstructionOverview struct {
// 	Agent     string
// 	Jumpgate  string
// 	Fabmat    int
// 	Advcct    int
// 	Timestamp time.Time
// }

func GetConstructionRecords(thisReset Reset, agents []string, dur time.Duration) map[string][]ConstructionRecord {
	if err := loadJumpgates(thisReset); err != nil {
		plog.Error("error loading jumpgates", "thisReset", thisReset, "error", err)
		return nil
	}
	agentRecords := GetAgents(thisReset)
	jumpGates := GetJumpgates(thisReset)
	loadConstructions(thisReset)

	res := make(map[string][]ConstructionRecord)
	for _, a := range agents {
		thisAgent := agentRecords[a]
		thisJumpgate := jumpGates[thisAgent.System]
		for _, rec := range constructionsLists[thisReset] {
			if rec.Jumpgate == thisJumpgate.Jumpgate {
				res[a] = append(res[a], ConstructionRecord{
					Timestamp: rec.Timestamp,
					Fabmat:    rec.Fabmat,
					Advcct:    rec.Advcct,
				})
			}
		}
	}
	return res
}
func GetLatestConstructionRecords(thisReset Reset, agents []string) []ConstructionOverview {
	if err := loadJumpgates(thisReset); err != nil {
		plog.Error("error loading jumpgates", "thisReset", thisReset, "error", err)
		return nil
	}
	agentRecords := GetAgents(thisReset)
	jumpGates := GetJumpgates(thisReset)
	loadConstructions(thisReset)

	res := []ConstructionOverview{}
	for _, a := range agents {
		jgLatest := JGConstruction{}
		thisAgent := agentRecords[a]
		thisJumpgate := jumpGates[thisAgent.System]
		for _, rec := range constructionsLists[thisReset] {
			if rec.Jumpgate != thisJumpgate.Jumpgate {
				continue
			}
			if rec.Timestamp > jgLatest.Timestamp {
				jgLatest = rec
			}
		}
		plog.Debug("adding record for agent", "agent", a, "record", jgLatest)
		if jgLatest.Timestamp == 0 {
			jgLatest.Timestamp = time.Now().Unix()
		}
		res = append(res, ConstructionOverview{
			Agent:     a,
			Jumpgate:  thisJumpgate.Jumpgate,
			Fabmat:    jgLatest.Fabmat,
			Advcct:    jgLatest.Advcct,
			Timestamp: time.Unix(jgLatest.Timestamp, 0).UTC(),
		})
	}
	return res
}
