// Package datastore defines the storage for the app.
// given a path, it will manage a directory for each reset
// in that directory there will be sets of files that represent the data that is saved from spacetraders.
// it will be able to load saved data into maps and write updates
// this package also holds all the datatypes that are used.
// the collector will call the api, marshal data into these types and then call the save functions
// as data is read, this will also build maps to make finding the data quicker.
package datastore

import (
	"bytes"
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
var zeroTimer *time.Timer
var cacheLifetime time.Duration
var plog *slog.Logger

func Init() {
	plog = slog.With("package", "datastore")
	l := plog.With("function", "init")
	env, ok := os.LookupEnv("FLUFFY_STORAGE_PATH")
	if ok {
		path = env
	}
	if env, ok = os.LookupEnv("FLUFFY_CACHE_DURATION"); ok {
		var err error
		cacheLifetime, err = time.ParseDuration(env)
		if err != nil {
			l.Warn("could not parse cache duration, setting to 5 mins")
			cacheLifetime = 5 * time.Minute
		}
	} else {
		cacheLifetime = 5 * time.Minute
	}
	err := os.MkdirAll(path, 0755)
	if err != nil {
		l.Error("Failed to create directory", "path", path)
		os.Exit(1)
	}
	env, ok = os.LookupEnv("FLUFFY_WRITE_JSON")
	if ok {
		for _, a := range []string{"yes", "y", "true"} {
			if strings.ToLower(env) == a {
				writeJSON = true
				l.Debug("writing json")
			}
		}
	}
	zeroTimer = time.NewTimer(time.Millisecond)
	go watchTimer()
}

func UpdateReset(r string) {
	l := plog.With("function", "updateReset")
	reset = r
	resetPath = filepath.Join(path, reset)
	err := os.MkdirAll(resetPath, 0755)
	if err != nil {
		l.Error("Failed to create directory", "path", path)
		os.Exit(1)
	}
	l.Debug("set reset", "current", resetPath)
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
	l := plog.With("function", "writeData")
	// Write JSON
	l.Debug("Writing files", "basepath", resetPath)
	var filename string
	if writeJSON {
		l.Debug("writing json file")
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
	l.Debug("Writing compressed gob")
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
		l.Error("encoder error")
		return err
	}
	defer encoder.Close()
	gobEnc := gob.NewEncoder(encoder)
	if err := gobEnc.Encode(v); err != nil {
		l.Error("encoding error")
		return err
	}
	encoder.Close()
	return nil
}

// readData loopsj
func readData(prefix string) (map[string]*bytes.Buffer, error) {
	l := plog.With("function", "readData")
	res := make(map[string]*bytes.Buffer)
	files, err := os.ReadDir(resetPath)
	if err != nil {
		return res, err
	}

	for _, f := range files {
		if !f.IsDir() && strings.HasPrefix(f.Name(), prefix) && strings.HasSuffix(f.Name(), ".gob.zst") {
			// l.Debug("reading file", "file", f.Name())
			file, err := os.Open(filepath.Join(resetPath, f.Name()))
			if err != nil {
				l.Error("Error opening file", "filename", f.Name(), "error", err)
				return res, err
			}
			defer file.Close()

			decoder, err := zstd.NewReader(file)
			if err != nil {
				l.Error("decoder error", "filename", f.Name(), "error", err)
				return res, err
			}
			defer decoder.Close()
			b := bytes.NewBuffer([]byte{})
			_, err = decoder.WriteTo(b)
			if err != nil {
				l.Error("decode to writer error", "error", err)
				continue
			}
			res[f.Name()] = b
		}
	}
	return res, err
}

// watchTimer is a func that is started on init - if the timer is ever fired, we remove all the data in stored variables
func watchTimer() {
	for {
		<-zeroTimer.C
		slog.Debug("Zero timer fired")
		zero()
	}
}

// zero initializes all the variables to empty
// can be called on startup and also when idle for too long
func zero() {
	slog.Debug("Zeroing")
	Agents = make(map[string]Agent)
	AgentCreditHistory = make(map[string][]DataPoint)
	AgentShipHistory = make(map[string][]DataPoint)
	StoredStats = Stats{}
	LatestCreditLeaders = []LeaderboardEntry{}
	LatestChartLeaders = []LeaderboardEntry{}
	jumpgatesBySystem = make(map[string]JGInfo)
	jumpgatesUnderConst = make(map[string]JGInfo)
}

func SystemFromWaypoint(w string) string {
	split := strings.Split(w, "-")
	return fmt.Sprintf("%s-%s", split[0], split[1])
}
