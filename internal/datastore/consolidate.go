package datastore

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/papaburgs/fluffy-robot/internal/logging"
)

func consolidate(basename string, data any, files map[string]*bytes.Buffer) {
	start := time.Now()
	ts := start.Unix()
	err := writeData(basename, ts, data)
	if err != nil {
		logging.Error("consolidate: write failed for", basename, err)
		return
	}

	for name := range files {
		fullPath := filepath.Join(resetPath, name)
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			logging.Error("consolidate: failed to remove", fullPath, err)
		}

		if writeJSON {
			jsonPath := strings.TrimSuffix(fullPath, ".gob.zst") + ".json"
			if err := os.Remove(jsonPath); err != nil && !os.IsNotExist(err) {
				logging.Error("consolidate: failed to remove", jsonPath, err)
			}
		}
	}
	fmt.Println("Consolidate complete, took ", time.Now().Sub(start))
}
