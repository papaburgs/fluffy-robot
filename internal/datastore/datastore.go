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
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

var path = "./"
var reset = ""
var resetPath = ""
var writeJSON = false

func Init() {
	env, ok := os.LookupEnv("FLUFFY_STORAGE_PATH")
	if ok {
		path = env
	}
	err := os.MkdirAll(path, 0755)
	if err != nil {
		slog.Error("Failed to create directory", "path", path)
		os.Exit(1)
	}
	env, ok = os.LookupEnv("FLUFFY_WRITE_JSON")
	if ok {
		for _, a := range []string{"yes", "y", "true"} {
			if strings.ToLower(env) == a {
				writeJSON = true
				slog.Debug("writing json")
			}
		}
	}
}

func UpdateReset(r string) {
	reset = r
	resetPath = filepath.Join(path, reset)
	err := os.MkdirAll(resetPath, 0755)
	if err != nil {
		slog.Error("Failed to create directory", "path", path)
		os.Exit(1)
	}
	slog.Debug("set reset", "current", resetPath)
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

func writeData(basename string, timestamp int64, v any) error {
	// Write JSON
	slog.Debug("Writing files", "basepath", resetPath)
	var filename string
	if writeJSON {
		slog.Debug("writing json file")
		if timestamp > 0 {
			filename = filepath.Join(resetPath, fmt.Sprintf("%s-%v.json", basename, timestamp))
		} else {
			filename = filepath.Join(resetPath, fmt.Sprintf("%s.json", basename))
		}
		jsonFile, err := os.Create(filename)
		if err != nil {
			return err
		}
		defer jsonFile.Close()
		enc := json.NewEncoder(jsonFile)
		enc.SetIndent("", "  ")
		if err := enc.Encode(v); err != nil {
			return err
		}
	}
	// Write compressed gob
	slog.Debug("Writing compressed gob")
	if timestamp > 0 {
		filename = filepath.Join(resetPath, fmt.Sprintf("%s-%v.gob.zst", basename, timestamp))
	} else {
		filename = filepath.Join(resetPath, fmt.Sprintf("%s.gob.zst", basename))
	}
	gobFile, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer gobFile.Close()
	encoder, err := zstd.NewWriter(gobFile)
	if err != nil {
		slog.Error("encoder error")
		return err
	}
	defer encoder.Close()
	gobEnc := gob.NewEncoder(encoder)
	if err := gobEnc.Encode(v); err != nil {
		slog.Error("encoding error")
		return err
	}
	encoder.Close()
	return nil
}
