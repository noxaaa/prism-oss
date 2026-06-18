package main

import "testing"

func TestDefaultMigrationsDirUsesOSSBuildDefault(t *testing.T) {
	t.Setenv("MIGRATIONS_DIRS", "")
	t.Setenv("PRISM_EDITION", "")

	got, err := defaultMigrationsDir()
	if err != nil {
		t.Fatalf("default migrations dir: %v", err)
	}
	if got != "migrations/core" {
		t.Fatalf("expected OSS build default migrations to be core-only, got %q", got)
	}
}

func TestDefaultMigrationsDirRejectsFullInOSSBuild(t *testing.T) {
	t.Setenv("MIGRATIONS_DIRS", "")
	t.Setenv("PRISM_EDITION", "full")

	if got, err := defaultMigrationsDir(); err == nil {
		t.Fatalf("expected OSS build to reject full migrations, got %q", got)
	}
}

func TestDefaultMigrationsDirRejectsFullInOSSBuildBeforeEnvironmentDirs(t *testing.T) {
	t.Setenv("MIGRATIONS_DIRS", "migrations/core")
	t.Setenv("PRISM_EDITION", "full")

	if got, err := defaultMigrationsDir(); err == nil {
		t.Fatalf("expected OSS build to reject full migrations before using MIGRATIONS_DIRS, got %q", got)
	}
}
