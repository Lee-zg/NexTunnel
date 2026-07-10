// Package config implements local configuration persistence using SQLite.
package config

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS tunnel_configs (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    proxy_type  TEXT NOT NULL DEFAULT 'tcp',
    local_addr  TEXT NOT NULL,
    local_port  INTEGER NOT NULL,
    remote_port INTEGER NOT NULL,
    server_addr TEXT NOT NULL DEFAULT '',
    domain      TEXT NOT NULL DEFAULT '',
    host_header TEXT NOT NULL DEFAULT '',
    public_url  TEXT NOT NULL DEFAULT '',
    access_policy_id TEXT NOT NULL DEFAULT '',
    inspect_enabled INTEGER NOT NULL DEFAULT 0,
    expires_at TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'stopped',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS app_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS favorite_ports (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    category    TEXT NOT NULL,
    port        INTEGER NOT NULL,
    protocol    TEXT NOT NULL DEFAULT 'tcp',
    description TEXT NOT NULL DEFAULT '',
    enabled     INTEGER NOT NULL DEFAULT 1,
    builtin     INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_favorite_ports_protocol_port
    ON favorite_ports(protocol, port);

CREATE TABLE IF NOT EXISTS activity_logs (
    id            TEXT PRIMARY KEY,
    level         TEXT NOT NULL,
    category      TEXT NOT NULL,
    action        TEXT NOT NULL,
    target_type   TEXT NOT NULL DEFAULT '',
    target_id     TEXT NOT NULL DEFAULT '',
    title         TEXT NOT NULL,
    message       TEXT NOT NULL DEFAULT '',
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_activity_logs_created_at
    ON activity_logs(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_activity_logs_level_category
    ON activity_logs(level, category, created_at DESC);
`

// DB wraps the SQLite database connection.
type DB struct {
	db   *sql.DB
	path string
}

// Open opens or creates a SQLite database at the given path.
// If path is empty, uses the default location (~/.nextunnel/config.db).
func Open(path string) (*DB, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		dir := filepath.Join(home, ".nextunnel")
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create config dir: %w", err)
		}
		path = filepath.Join(dir, "config.db")
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	pragmas := []struct {
		name  string
		query string
	}{
		{name: "busy timeout", query: "PRAGMA busy_timeout=5000"},
		{name: "WAL mode", query: "PRAGMA journal_mode=WAL"},
		{name: "synchronous mode", query: "PRAGMA synchronous=NORMAL"},
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma.query); err != nil {
			db.Close()
			return nil, fmt.Errorf("set %s: %w", pragma.name, err)
		}
	}

	d := &DB{db: db, path: path}
	if err := d.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return d, nil
}

// migrate runs database migrations.
func (d *DB) migrate() error {
	if _, err := d.db.Exec(schema); err != nil {
		return fmt.Errorf("run schema migration: %w", err)
	}
	// 旧版 alpha 配置库没有 Public Endpoint 相关列，按列级迁移可避免用户丢失本地隧道配置。
	for _, migration := range []struct {
		column string
		sql    string
	}{
		{column: "domain", sql: "ALTER TABLE tunnel_configs ADD COLUMN domain TEXT NOT NULL DEFAULT ''"},
		{column: "host_header", sql: "ALTER TABLE tunnel_configs ADD COLUMN host_header TEXT NOT NULL DEFAULT ''"},
		{column: "public_url", sql: "ALTER TABLE tunnel_configs ADD COLUMN public_url TEXT NOT NULL DEFAULT ''"},
		{column: "access_policy_id", sql: "ALTER TABLE tunnel_configs ADD COLUMN access_policy_id TEXT NOT NULL DEFAULT ''"},
		{column: "inspect_enabled", sql: "ALTER TABLE tunnel_configs ADD COLUMN inspect_enabled INTEGER NOT NULL DEFAULT 0"},
		{column: "expires_at", sql: "ALTER TABLE tunnel_configs ADD COLUMN expires_at TEXT NOT NULL DEFAULT ''"},
	} {
		exists, err := d.columnExists("tunnel_configs", migration.column)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := d.db.Exec(migration.sql); err != nil {
				return fmt.Errorf("add tunnel_configs.%s: %w", migration.column, err)
			}
		}
	}
	return nil
}

func (d *DB) columnExists(table, column string) (bool, error) {
	rows, err := d.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, fmt.Errorf("inspect table %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name string
		var dataType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return false, fmt.Errorf("scan table info %s: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// Path returns the database file path.
func (d *DB) Path() string {
	return d.path
}
