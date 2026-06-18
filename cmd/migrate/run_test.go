package main

import (
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
