package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

// Connect establishes a connection to the Turso/libSQL database.
func Connect() (*sql.DB, error) {
	url := os.Getenv("TURSO_URL")
	if url == "" {
		return nil, fmt.Errorf("TURSO_URL environment variable is not set")
	}

	token := os.Getenv("TURSO_AUTH_TOKEN")
	if token != "" {
		url = fmt.Sprintf("%s?authToken=%s", url, token)
	} else {
		slog.Error("TURSO_AUTH_TOKEN needs to be set")
	}

	db, err := sql.Open("libsql", url)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

// InitSchema creates the necessary tables if they don't exist.
func InitSchema(db *sql.DB) error {
	slog.Info("initializing database schema")

	queries := []string{
		`CREATE TABLE IF NOT EXISTS agents (
			timestamp INTEGER,
			reset TEXT,
			symbol TEXT PRIMARY KEY,
			ships INTEGER,
			faction TEXT,
			credits INTEGER,
			headquarters TEXT
		)`,
		// Leaderboard table: symbol cannot be PK if it's history.
		// Using (timestamp, symbol, type) as composite PK for uniqueness of record.
		`CREATE TABLE IF NOT EXISTS leaderboard (
			timestamp INTEGER,
			reset TEXT,
			count INTEGER,
			symbol TEXT,
			type TEXT,
			PRIMARY KEY (timestamp, symbol, type)
		)`,
		`CREATE TABLE IF NOT EXISTS jumpgates (
			reset TEXT,
			system TEXT,
			headquarters TEXT,
			jumpgate TEXT,
			complete INTEGER,
			activeagent BOOLEAN,
			PRIMARY KEY (reset, jumpgate)
		)`,
		`CREATE TABLE IF NOT EXISTS construction (
			reset TEXT,
			timestamp INTEGER,
			jumpgate TEXT,
			fabmat INTEGER,
			advcct INTEGER,
			PRIMARY KEY (reset, timestamp, jumpgate)
		)`,
		`CREATE TABLE IF NOT EXISTS stats (
			reset TEXT PRIMARY KEY,
			marketUpdate DATETIME,
			agents INTEGER,
			accounts INTEGER,
			ships INTEGER,
			systems INTEGER,
			waypoints INTEGER,
			status TEXT,
			version TEXT,
			nextReset DATETIME
		)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("failed to execute query %q: %w", q, err)
		}
	}

	slog.Info("database schema initialized successfully")
	return nil
}
