package main

import (
	"encoding/gob"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/papaburgs/fluffy-robot/internal/datastore"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s <file_path>\n", os.Args[0])
		os.Exit(1)
	}

	filePath := os.Args[1]
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	decoder, err := zstd.NewReader(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating zstd reader: %v\n", err)
		os.Exit(1)
	}
	defer decoder.Close()

	var data any
	baseName := filepath.Base(filePath)

	switch {
	case strings.HasPrefix(baseName, "stats"):
		data = &datastore.Stats{}
	case strings.HasPrefix(baseName, "leaderboard"):
		data = &datastore.LeaderboardRecord{}
	case strings.HasPrefix(baseName, "jumpgates"):
		data = &[]datastore.JGInfo{}
	case strings.HasPrefix(baseName, "agentsStatus"):
		data = &[]datastore.AgentStatus{}
	case strings.HasPrefix(baseName, "agents"):
		data = &[]datastore.Agent{}
	case strings.HasPrefix(baseName, "construction"):
		data = &[]datastore.JGConstruction{}
	default:
		fmt.Fprintf(os.Stderr, "Error: Unknown file type for %s. Recognized prefixes: stats, leaderboard, jumpgates, agentsStatus, agents, construction.\n", baseName)
		os.Exit(1)
	}

	gobDec := gob.NewDecoder(decoder)
	if err := gobDec.Decode(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding gob: %v\n", err)
		os.Exit(1)
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(jsonData))
}
