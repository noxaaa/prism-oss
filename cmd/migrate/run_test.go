package main

import (
	"database/sql"
	"testing"
)

func TestRunRejectsMissingDatabaseURL(t *testing.T) {
	err := run([]string{"up"})
	if err == nil {
		t.Fatalf("expected missing PostgreSQL database URL error")
	}
}

func TestCoreMigrationCreatesPostgresSchemasAndVersionTables(t *testing.T) {
	db := openMigratedCoreDB(t)
	defer func() { _ = db.Close() }()

	for _, table := range []struct {
		schema string
		name   string
	}{
		{schema: "auth", name: "user"},
		{schema: "auth", name: "goose_db_version_auth"},
		{schema: "app", name: "organizations"},
		{schema: "app", name: "goose_db_version_core"},
	} {
		if !tableExistsInSchema(t, db, table.schema, table.name) {
			t.Fatalf("expected %s.%s to exist", table.schema, table.name)
		}
	}
}

func TestCoreInitialMigrationIncludesAgentUpdateColumns(t *testing.T) {
	db := openMigratedCoreDB(t)
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
			t.Fatalf("nodes table missing %s", column)
		}
	}
}

func tableExistsInSchema(t *testing.T, db *sql.DB, schema string, table string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = $1 AND table_name = $2
	`, schema, table).Scan(&count); err != nil {
		t.Fatalf("read table %s.%s existence: %v", schema, table, err)
	}
	return count > 0
}

func nodesTableHasColumn(t *testing.T, db *sql.DB, column string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_schema = 'app' AND table_name = 'nodes' AND column_name = $1
	`, column).Scan(&count); err != nil {
		t.Fatalf("read nodes column %s: %v", column, err)
	}
	return count > 0
}
