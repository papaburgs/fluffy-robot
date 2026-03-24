// Package datastore defines the storage for the app.
// given a path, it will manage a directory for each reset
// in that directory there will be sets of files that represent the data that is saved from spacetraders.
// it will be able to load saved data into maps and write updates
// this package also holds all the datatypes that are used.
// the collector will call the api, marshal data into these types and then call the save functions
// as data is read, this will also build maps to make finding the data quicker.
package datastore

import (
	"encoding/gob"
	"encoding/json"
	"os"
	"time"

	"github.com/klauspost/compress/zstd"
)

var path = ""

func Init(p string) {
	path = p
}

type JumpGateAgentListStruct struct {
	AgentsToCheck  []PublicAgent `json:"agents_to_check"`
	AgentsToIgnore []PublicAgent `json:"agents_to_ignore"`
}

type TimedConstructionRecord struct {
	Timestamp time.Time
	Fabmat    int
	Advcct    int
}

type ConstructionOverview struct {
	Agent     string
	Jumpgate  string
	Fabmat    int
	Advcct    int
	Timestamp time.Time
}

func writeData(basename string, v any) error {
	// Write JSON
	jsonFile, err := os.Create(path + "/" + basename + ".json")
	if err != nil {
		return err
	}
	enc := json.NewEncoder(jsonFile)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		jsonFile.Close()
		return err
	}
	jsonFile.Close()

	// Write compressed gob
	gobFile, err := os.Create(path + "/" + basename + ".gob.zst")
	if err != nil {
		return err
	}
	encoder, err := zstd.NewWriter(gobFile)
	if err != nil {
		gobFile.Close()
		return err
	}
	gobEnc := gob.NewEncoder(encoder)
	if err := gobEnc.Encode(v); err != nil {
		encoder.Close()
		gobFile.Close()
		return err
	}
	encoder.Close()
	gobFile.Close()
	return nil
}
