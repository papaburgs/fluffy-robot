package datastore

import "database/sql"

// schemaVersion is bumped when the DDL changes; applySchema is idempotent
// and only moves user_version forward.
const schemaVersion = 1

const schemaDDL = `
CREATE TABLE IF NOT EXISTS agents (
	id         INTEGER PRIMARY KEY,
	symbol     TEXT NOT NULL UNIQUE,
	first_seen INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_snapshots (
	reset_date   INTEGER NOT NULL,
	agent_id     INTEGER NOT NULL REFERENCES agents(id),
	credits      INTEGER NOT NULL,
	faction      TEXT    NOT NULL,
	headquarters TEXT    NOT NULL,
	system       TEXT    NOT NULL,
	updated_at   INTEGER NOT NULL,
	PRIMARY KEY (reset_date, agent_id)
);

CREATE TABLE IF NOT EXISTS agent_status (
	agent_id  INTEGER NOT NULL REFERENCES agents(id),
	timestamp INTEGER NOT NULL,
	credits   INTEGER NOT NULL,
	ships     INTEGER NOT NULL,
	PRIMARY KEY (agent_id, timestamp)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS stats (
	reset_date    TEXT PRIMARY KEY,
	market_update INTEGER NOT NULL,
	agents        INTEGER NOT NULL,
	accounts      INTEGER NOT NULL,
	ships         INTEGER NOT NULL,
	systems       INTEGER NOT NULL,
	waypoints     INTEGER NOT NULL,
	status        TEXT    NOT NULL,
	version       TEXT    NOT NULL,
	next_reset    INTEGER NOT NULL,
	last_update   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS leaderboards (
	reset_date TEXT    NOT NULL,
	kind       TEXT    NOT NULL CHECK (kind IN ('credits','charts')),
	rank       INTEGER NOT NULL,
	symbol     TEXT    NOT NULL,
	value      INTEGER NOT NULL,
	PRIMARY KEY (reset_date, kind, rank)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS jumpgates (
	reset_date   TEXT    NOT NULL,
	system       TEXT    NOT NULL,
	jumpgate     TEXT    NOT NULL,
	headquarters TEXT    NOT NULL,
	status       INTEGER NOT NULL,
	completed_at INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (reset_date, system)
);

CREATE TABLE IF NOT EXISTS construction (
	jumpgate  TEXT    NOT NULL,
	timestamp INTEGER NOT NULL,
	fabmat    INTEGER NOT NULL,
	advcct    INTEGER NOT NULL,
	PRIMARY KEY (jumpgate, timestamp)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS factions (
	reset_date    TEXT    NOT NULL,
	symbol        TEXT    NOT NULL,
	name          TEXT    NOT NULL,
	description   TEXT    NOT NULL,
	headquarters  TEXT    NOT NULL,
	traits        TEXT    NOT NULL,
	is_recruiting INTEGER NOT NULL,
	PRIMARY KEY (reset_date, symbol)
) WITHOUT ROWID;
`

func applySchema(d *sql.DB) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(schemaDDL); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`PRAGMA user_version = 1`); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
