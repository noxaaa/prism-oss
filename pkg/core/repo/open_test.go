package repo

import (
	"os"
	"testing"
)

func TestOpenPostgresRejectsMissingDatabaseURL(t *testing.T) {
	if _, err := OpenPostgres(""); err == nil {
		t.Fatalf("expected missing PostgreSQL DATABASE_URL error")
	}
}

func TestPostgresConnConfigDefaultsSearchPath(t *testing.T) {
	config, err := postgresConnConfig("postgres://prism:secret@localhost:5432/prism?sslmode=disable")
	if err != nil {
		t.Fatalf("parse PostgreSQL config: %v", err)
	}
	if got := config.RuntimeParams["search_path"]; got != defaultPostgresSearchPath {
		t.Fatalf("search_path = %q, want %q", got, defaultPostgresSearchPath)
	}
}

func TestOpenPostgresAppliesDefaultSearchPath(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	db, err := OpenPostgres(databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close PostgreSQL: %v", err)
		}
	})

	var searchPath string
	if err := db.QueryRow("SHOW search_path").Scan(&searchPath); err != nil {
		t.Fatalf("show search_path: %v", err)
	}
	if searchPath != defaultPostgresSearchPath {
		t.Fatalf("search_path = %q, want %q", searchPath, defaultPostgresSearchPath)
	}
}
