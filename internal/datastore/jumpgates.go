package datastore

import (
	"encoding/gob"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/logging"
)

func UpdateJumpGates(jgList []JGInfo) {
	writeData("jumpgates", 0, jgList)
}

func GetConstructions(thisReset Reset) ([]JGConstruction, error) {
	res := []JGConstruction{}
	m, err := readData("construction-", thisReset)
	if err != nil {
		return res, err
	}
	for _, b := range m {
		var v []JGConstruction
		gobDec := gob.NewDecoder(b)
		if err := gobDec.Decode(&v); err != nil {
			return res, err
		}
		res = append(res, v...)
		v = nil
	}
	m = nil
	return res, nil
}

func GetJumpgateList(thisReset Reset) ([]JGInfo, error) {
	res := []JGInfo{}
	m, err := readData("jumpgates.", thisReset)
	if err != nil {
		return res, err
	}

	for _, b := range m {
		gobDec := gob.NewDecoder(b)
		if err := gobDec.Decode(&res); err != nil {
			return res, err
		}
	}
	m = nil
	return res, nil
}

func MarkJumpgatesComplete(jgs []string, ts int64) {
	current, err := GetJumpgateList(currentReset)
	if err != nil {
		logging.Error("error loading current jumpgates")
	}
	updated := []JGInfo{}
	for _, j := range current {
		rec := j
		for _, k := range jgs {
			if j.System == k {
				rec.Status = Complete
				rec.Complete = ts
			}
		}
		updated = append(updated, rec)
	}
	current = nil
	UpdateJumpGates(updated)
	updated = nil
}

func MarkJumpgatesStarted(jgs []string) {

	current, err := GetJumpgateList(currentReset)
	if err != nil {
		logging.Error("error loading current jumpgates")
	}
	updated := []JGInfo{}
	for _, j := range current {
		rec := j
		for _, k := range jgs {
			if j.System == k {
				rec.Status = Const
			}
		}
		updated = append(updated, rec)
	}
	current = nil
	UpdateJumpGates(updated)
	updated = nil
}

func AddConstructions(cList []JGConstruction, ts int64) {
	writeData("construction", ts, cList)
}

func GetJumpgates(thisReset Reset) map[string]JGInfo {
	current, err := GetJumpgateList(currentReset)
	if err != nil {
		logging.Error("error loading current jumpgates")
		return nil
	}
	res := make(map[string]JGInfo, len(current))
	for _, j := range current {
		res[j.System] = j
	}
	current = nil
	return res
}

func GetJumpgatesUnderConst(thisReset Reset) map[string]JGInfo {
	current, err := GetJumpgateList(currentReset)
	if err != nil {
		logging.Error("error loading current jumpgates")
		return nil
	}
	res := make(map[string]JGInfo)
	for _, j := range current {
		if j.Status == Const {
			res[j.System] = j
		}
	}
	current = nil
	return res
}

func GetJumpgatesNotStarted(thisReset Reset) map[string]JGInfo {
	current, err := GetJumpgateList(currentReset)
	if err != nil {
		logging.Error("error loading current jumpgates")
		return nil
	}
	res := make(map[string]JGInfo)
	for _, j := range current {
		if j.Status == Active {
			res[j.System] = j
		}
	}
	current = nil
	return res
}

func GetJumpgatesComplete(thisReset Reset) []JGInfo {
	current, err := GetJumpgateList(currentReset)
	if err != nil {
		logging.Error("error loading current jumpgates")
		return nil
	}
	res := []JGInfo{}
	for _, j := range current {
		if j.Status == Complete {
			res = append(res, j)
		}
	}
	current = nil
	return res
}

type ConstructionRecord struct {
	Timestamp int64
	Fabmat    int
	Advcct    int
}

func GetConstructionRecords(thisReset Reset, agents []string, dur time.Duration) map[string][]ConstructionRecord {
	agentRecords := GetAgents(thisReset)
	jumpGates := GetJumpgates(thisReset)
	constructions, _ := GetConstructions(thisReset)
	res := make(map[string][]ConstructionRecord)
	for _, a := range agents {
		thisAgent := agentRecords[a]
		thisJumpgate := jumpGates[thisAgent.System]
		for _, rec := range constructions {
			if rec.Jumpgate == thisJumpgate.Jumpgate {
				res[a] = append(res[a], ConstructionRecord{
					Timestamp: rec.Timestamp,
					Fabmat:    rec.Fabmat,
					Advcct:    rec.Advcct,
				})
			}
		}
	}
	agentRecords = nil
	jumpGates = nil
	constructions = nil
	return res
}

func GetLatestConstructionRecords(thisReset Reset, agents []string) []ConstructionOverview {
	agentRecords := GetAgents(thisReset)
	jumpGates := GetJumpgates(thisReset)
	constructions, _ := GetConstructions(thisReset)

	res := []ConstructionOverview{}
	for _, a := range agents {
		jgLatest := JGConstruction{}
		thisAgent := agentRecords[a]
		thisJumpgate := jumpGates[thisAgent.System]
		for _, rec := range constructions {
			if rec.Jumpgate != thisJumpgate.Jumpgate {
				continue
			}
			if rec.Timestamp > jgLatest.Timestamp {
				jgLatest = rec
			}
		}
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
	agentRecords = nil
	jumpGates = nil
	constructions = nil
	return res
}
