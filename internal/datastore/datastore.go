package datastore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/papaburgs/fluffy-robot/internal/logging"
)

var path = "./"
var currentReset Reset = ""

// Init opens (creating if needed) the SQLite store at
// {FLUFFY_STORAGE_PATH}/fluffy.db. Per-reset directories no longer exist —
// reset is a column in the data.
func Init() {
	if env, ok := os.LookupEnv("FLUFFY_STORAGE_PATH"); ok {
		path = env
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		logging.Error("Failed to create directory at", path)
		os.Exit(1)
	}

	parseRetentionWeeks()

	d, err := openDB(filepath.Join(path, "fluffy.db"))
	if err != nil {
		logging.Error("Failed to open sqlite db:", err)
		os.Exit(1)
	}
	db = d
}

// UpdateReset switches the current reset. In-memory only — storage is a
// single file; there is nothing to create per reset.
func UpdateReset(r Reset) {
	currentReset = r
}

func SystemFromWaypoint(w string) string {
	split := strings.Split(w, "-")
	return fmt.Sprintf("%s-%s", split[0], split[1])
}

func DataPath() string {
	return path
}
