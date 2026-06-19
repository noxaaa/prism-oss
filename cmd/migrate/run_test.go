package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestRunRejectsMissingDatabasePath(t *testing.T) {
	err := run([]string{"up"})
	if err == nil {
		t.Fatalf("expected missing database path error")
	}
}

func TestMainUsesEnvironmentDefaults(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "env.db")
	t.Setenv("DATABASE_URL", databasePath)
	t.Setenv("MIGRATIONS_DIRS", "../../migrations/core")

	if err := run([]string{"up"}); err != nil {
		t.Fatalf("run with env defaults: %v", err)
	}
	if _, err := os.Stat(databasePath); err != nil {
		t.Fatalf("expected database file to be created: %v", err)
	}
}

func TestRunNormalizesSQLiteDatabaseURL(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "sqlite-url.db")
	if err := run([]string{
		"-database", "sqlite://" + databasePath,
		"-dir", "../../migrations/core",
		"up",
	}); err != nil {
		t.Fatalf("run migrate up with sqlite URL: %v", err)
	}

	db, err := openSQLite(databasePath)
	if err != nil {
		t.Fatalf("open normalized sqlite database: %v", err)
	}
	defer func() { _ = db.Close() }()

	var name string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'organizations'`).Scan(&name)
	if err != nil {
		t.Fatalf("expected migrations to apply to normalized database path: %v", err)
	}
	if name != "organizations" {
		t.Fatalf("expected organizations table, got %q", name)
	}
}

func TestCoreMigrationUpgradesVersionOneNodeSchema(t *testing.T) {
	tempDir := t.TempDir()
	initialDir := filepath.Join(tempDir, "initial", "core")
	if err := os.MkdirAll(initialDir, 0o755); err != nil {
		t.Fatalf("create initial migration dir: %v", err)
	}
	initialMigration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "core", "00001_core.sql"))
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	if err := os.WriteFile(filepath.Join(initialDir, "00001_core.sql"), initialMigration, 0o644); err != nil {
		t.Fatalf("write initial migration fixture: %v", err)
	}

	databasePath := filepath.Join(tempDir, "upgrade.db")
	if err := run([]string{"-database", databasePath, "-dir", initialDir, "up"}); err != nil {
		t.Fatalf("run initial migrate up: %v", err)
	}
	db, err := openSQLite(databasePath)
	if err != nil {
		t.Fatalf("open initial database: %v", err)
	}
	if nodesTableHasColumn(t, db, "agent_version") {
		t.Fatalf("version 1 nodes table must not already contain agent update columns")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close initial database: %v", err)
	}

	if err := run([]string{"-database", databasePath, "-dir", filepath.Join("..", "..", "migrations", "core"), "up"}); err != nil {
		t.Fatalf("run current migrate up: %v", err)
	}
	db, err = openSQLite(databasePath)
	if err != nil {
		t.Fatalf("open upgraded database: %v", err)
	}
	defer func() { _ = db.Close() }()
	for _, column := range []string{
		"agent_version",
		"agent_commit",
		"agent_build_time",
		"agent_auto_update_enabled",
		"desired_agent_version",
		"agent_update_status",
		"agent_update_error",
		"agent_update_started_at",
		"agent_update_finished_at",
	} {
		if !nodesTableHasColumn(t, db, column) {
			t.Fatalf("upgraded nodes table missing %s", column)
		}
	}
}

func nodesTableHasColumn(t *testing.T, db *sql.DB, column string) bool {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(nodes)")
	if err != nil {
		t.Fatalf("read nodes table info: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan nodes table info: %v", err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate nodes table info: %v", err)
	}
	return false
}
