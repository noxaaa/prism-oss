package arch

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestOSSIsPostgresOnly(t *testing.T) {
	root := repoRoot(t)
	for _, relative := range []string{
		"go.mod",
		"cmd/control-plane-oss/main.go",
		"cmd/migrate/main.go",
		"pkg/core/config/config.go",
		"pkg/core/repo",
		"apps/web/package.json",
		"apps/web/next.config.mjs",
		"apps/web/src/lib",
		"docker-compose.yml",
		"scripts/install.sh",
		"README.md",
	} {
		source := readPathText(t, filepath.Join(root, filepath.FromSlash(relative)))
		for _, forbidden := range []string{
			"modernc.org/sqlite",
			"better-sqlite3",
			"sqlite_master",
			"sqlite-data",
			"sqlite-permissions",
			"OpenSQLite",
			"NewSQLiteStore",
			"SQLiteStore",
			"goose.SetDialect(\"sqlite3\")",
			"PRAGMA ",
		} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s must be PostgreSQL-only; found %q", relative, forbidden)
			}
		}
	}
}

func TestOSSPostgresInstallAndReleaseShape(t *testing.T) {
	root := repoRoot(t)
	compose := readText(t, filepath.Join(root, "docker-compose.yml"))
	for _, required := range []string{
		"postgres:16",
		"postgres-data:",
		"pg_isready",
		"DATABASE_URL:",
		"postgres://${POSTGRES_USER:-prism}:${POSTGRES_PASSWORD:-prism}@postgres:5432/${POSTGRES_DB:-prism}?sslmode=disable",
		"/migrations/auth,/migrations/core",
	} {
		if !strings.Contains(compose, required) {
			t.Fatalf("docker-compose.yml must define PostgreSQL install shape; missing %q", required)
		}
	}
	if strings.Contains(compose, "postgres://prism:prism@postgres:5432/prism?sslmode=disable") {
		t.Fatalf("docker-compose.yml must derive default DATABASE_URL from POSTGRES_USER, POSTGRES_PASSWORD, and POSTGRES_DB")
	}

	install := readText(t, filepath.Join(root, "scripts", "install.sh"))
	for _, required := range []string{
		"--database-url",
		"POSTGRES_PASSWORD",
		"postgres:16",
		"DATABASE_URL",
		"/migrations/auth,/migrations/core",
	} {
		if !strings.Contains(install, required) {
			t.Fatalf("install.sh must support PostgreSQL-only installs; missing %q", required)
		}
	}

	release := readText(t, filepath.Join(root, ".github", "workflows", "release.yml"))
	for _, required := range []string{
		"migrations/auth",
		"migrations/core",
		"migrate-linux-amd64.tar.gz",
		"migrate-linux-arm64.tar.gz",
	} {
		if !strings.Contains(release, required) {
			t.Fatalf("release workflow must package PostgreSQL auth/core migrations; missing %q", required)
		}
	}
}

func TestOSSPostgresQueriesCastNullableUUIDsBeforeCoalesce(t *testing.T) {
	root := repoRoot(t)
	source := readPathText(t, filepath.Join(root, "pkg", "core", "repo"))
	pattern := regexp.MustCompile(`coalesce\((?:[a-z_]+\.)?(subject_rule_id|registration_token_id|target_id|target_group_id),\s*''\)`)
	if match := pattern.FindString(source); match != "" {
		t.Fatalf("nullable PostgreSQL UUID columns must be cast to text before coalesce; found %q", match)
	}
}

func TestOSSOpenPostgresValidatesConnectionBeforeReturning(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "pkg", "core", "repo", "open.go"))
	for _, required := range []string{
		"context.WithTimeout",
		"db.PingContext(ctx)",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("OpenPostgres must validate PostgreSQL connectivity before returning; missing %q", required)
		}
	}
}

func TestOSSRepoFilesDoNotUseDatabaseAdapterPrefixes(t *testing.T) {
	root := repoRoot(t)
	matches, err := filepath.Glob(filepath.Join(root, "pkg", "core", "repo", "postgres_*.go"))
	if err != nil {
		t.Fatalf("glob repo files: %v", err)
	}
	if len(matches) > 0 {
		t.Fatalf("PostgreSQL is the only supported database; repo files must not look like database adapters: %v", matches)
	}
}

func readPathText(t *testing.T, path string) string {
	t.Helper()
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if !stat.IsDir() {
		return readText(t, path)
	}
	var builder strings.Builder
	matches, err := filepath.Glob(filepath.Join(path, "*.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", path, err)
	}
	for _, match := range matches {
		builder.WriteString(readText(t, match))
		builder.WriteByte('\n')
	}
	return builder.String()
}
