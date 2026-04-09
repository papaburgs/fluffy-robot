package datastore

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/papaburgs/fluffy-robot/internal/logging"
	"github.com/papaburgs/fluffy-robot/internal/metrics"
)

var path = "./"
var currentReset Reset = ""
var resetPath = ""
var writeJSON = false

func Init() {
	env, ok := os.LookupEnv("FLUFFY_STORAGE_PATH")
	if ok {
		path = env
	}
	err := os.MkdirAll(path, 0755)
	if err != nil {
		logging.Error("Failed to create directory at", path)
		os.Exit(1)
	}
	env, ok = os.LookupEnv("FLUFFY_WRITE_JSON")
	if ok {
		for _, a := range []string{"yes", "y", "true"} {
			if strings.ToLower(env) == a {
				writeJSON = true
			}
		}
	}

}

func UpdateReset(r Reset) {
	currentReset = r
	resetPath = filepath.Join(path, string(currentReset))
	err := os.MkdirAll(resetPath, 0755)
	if err != nil {
		logging.Error("Failed to create directory:", path)
		os.Exit(1)
	}
}

func writeData(basename string, timestamp int64, v any) error {
	var filename string
	if writeJSON {
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
		metrics.DatastoreWrites.Add(1)
	}
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
		logging.Error("encoder Newwriter error:", err)
		return err
	}
	defer encoder.Close()
	gobEnc := gob.NewEncoder(encoder)
	if err := gobEnc.Encode(v); err != nil {
		return err
	}
	encoder.Close()
	metrics.DatastoreWrites.Add(1)
	return nil
}

func readData(prefix string, thisReset Reset) (map[string]*bytes.Buffer, error) {
	res := make(map[string]*bytes.Buffer)

	thisPath := resetPath
	if thisReset != "" {
		thisPath = filepath.Join(path, string(thisReset))
	}
	files, err := os.ReadDir(thisPath)
	if err != nil {
		return res, err
	}

	for _, f := range files {
		if !f.IsDir() && strings.HasPrefix(f.Name(), prefix) && strings.HasSuffix(f.Name(), ".gob.zst") {
			file, err := os.Open(filepath.Join(resetPath, f.Name()))
			if err != nil {
				logging.Error("Error opening file:", f.Name(), err)
				return res, err
			}
			defer file.Close()

			decoder, err := zstd.NewReader(file)
			if err != nil {
				logging.Error("decoder error:", f.Name(), err)
				return res, err
			}
			defer decoder.Close()
			b := bytes.NewBuffer([]byte{})
			_, err = decoder.WriteTo(b)
			if err != nil {
				logging.Error("decode to writer error:", err)
				continue
			}
			res[f.Name()] = b
			metrics.DatastoreReads.Add(1)
		}
	}
	return res, err
}

func SystemFromWaypoint(w string) string {
	split := strings.Split(w, "-")
	return fmt.Sprintf("%s-%s", split[0], split[1])
}

func DataPath() string {
	return path
}
