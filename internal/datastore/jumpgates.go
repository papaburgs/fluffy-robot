package datastore

import (
	"encoding/gob"
	"fmt"
	"sort"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/logging"
)

func UpdateJumpGates(jgList []JGInfo) {
	writeData("jumpgates", 0, jgList)
}

var consolidating bool

func GetConstructions(thisReset Reset, start, end int64) ([]JGConstruction, error) {
	for consolidating {
		fmt.Println("still consolidating")
		time.Sleep(time.Second)
	}
	if end == 0 {
		end = time.Now().Unix()
	}
	res := []JGConstruction{}
	m, err := readData("construction-", thisReset)
	if err != nil {
		return res, err
	}

	allRecords := make([]JGConstruction, 0, len(m)*2)
	for _, b := range m {
		var v []JGConstruction
		gobDec := gob.NewDecoder(b)
		if err := gobDec.Decode(&v); err != nil {
			return res, err
		}
		allRecords = append(allRecords, v...)
	}

	if len(m) > 5 {
		consolidating = true
		// consolidate("construction", allRecords, m)
		consolidating = false
	}
	m = nil

	for _, r := range allRecords {
		if r.Timestamp >= start && r.Timestamp <= end {
			res = append(res, r)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Timestamp < res[j].Timestamp
	})
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
	for consolidating {
		fmt.Println("still consolidating")
		time.Sleep(time.Second)
	}
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
