package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/noxaaa/prism-oss/pkg/edition"
)

type migrationGroup struct {
	Dir        string
	TableName  string
	SchemaName string
	SearchPath string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	databaseURL := flags.String("database", os.Getenv("DATABASE_URL"), "PostgreSQL connection URL")
	migrationsDir := flags.String("dir", "", "goose migrations directory or comma-separated directories")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if *databaseURL == "" {
		return errors.New("PostgreSQL DATABASE_URL is required through -database or DATABASE_URL")
	}
	if *migrationsDir == "" {
		defaultDir, err := defaultMigrationsDir()
		if err != nil {
			return err
		}
		*migrationsDir = defaultDir
	}

	command := "up"
	if flags.NArg() > 0 {
		command = flags.Arg(0)
	}

	db, err := openPostgres(*databaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	migrationGroups := splitMigrationDirs(*migrationsDir)
	if len(migrationGroups) == 0 {
		return fmt.Errorf("at least one migrations directory is required")
	}
	switch command {
	case "up":
		for _, group := range migrationGroups {
			if err := runGooseWithTable(db, group, goose.Up); err != nil {
				return err
			}
		}
		return nil
	case "down":
		if len(migrationGroups) > 1 {
			return fmt.Errorf("multi-directory down is not supported; pass a single migration directory")
		}
		return runGooseWithTable(db, migrationGroups[0], goose.Down)
	case "status":
		for _, group := range migrationGroups {
			if err := runGooseWithTable(db, group, goose.Status); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported migration command %q", command)
	}
}

func openPostgres(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db, nil
}

func defaultMigrationsDir() (string, error) {
	key, err := edition.KeyFromString(os.Getenv("PRISM_EDITION"))
	if err != nil {
		return "", err
	}
	provider, err := migrationProviderForKey(key)
	if err != nil {
		return "", err
	}
	if value := os.Getenv("MIGRATIONS_DIRS"); value != "" {
		return value, nil
	}
	return strings.Join(provider.DefaultMigrationDirs(), ","), nil
}

func splitMigrationDirs(value string) []migrationGroup {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == os.PathListSeparator
	})
	groups := make([]migrationGroup, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		dir := filepath.Clean(part)
		groups = append(groups, migrationGroup{
			Dir:        dir,
			TableName:  migrationVersionTableName(dir),
			SchemaName: migrationSchemaName(dir),
			SearchPath: migrationSearchPath(dir),
		})
	}
	return groups
}

func migrationVersionTableName(dir string) string {
	base := filepath.Base(filepath.Clean(dir))
	switch base {
	case "core":
		return "goose_db_version_core"
	case "auth":
		return "goose_db_version_auth"
	case "commercial":
		return "goose_db_version_commercial"
	default:
		return "goose_db_version_" + sanitizeIdentifier(base)
	}
}

func migrationSchemaName(dir string) string {
	switch filepath.Base(filepath.Clean(dir)) {
	case "auth":
		return "auth"
	case "commercial":
		return "commercial"
	default:
		return "app"
	}
}

func migrationSearchPath(dir string) string {
	switch filepath.Base(filepath.Clean(dir)) {
	case "auth":
		return "auth, public"
	case "commercial":
		return "commercial, app, auth, public"
	default:
		return "app, auth, public"
	}
}

func sanitizeIdentifier(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte('_')
	}
	result := strings.Trim(builder.String(), "_")
	if result == "" {
		return "migrations"
	}
	return result
}

func runGooseWithTable(db *sql.DB, group migrationGroup, runFunc func(*sql.DB, string, ...goose.OptionsFunc) error) error {
	if err := ensureMigrationSchema(db, group.SchemaName); err != nil {
		return err
	}
	if err := applyMigrationSearchPath(db, group.SearchPath); err != nil {
		return err
	}
	previousTableName := goose.TableName()
	goose.SetTableName(group.TableName)
	defer goose.SetTableName(previousTableName)
	return runFunc(db, group.Dir)
}

func ensureMigrationSchema(db *sql.DB, schemaName string) error {
	if !isIdentifier(schemaName) {
		return fmt.Errorf("invalid migration schema %q", schemaName)
	}
	if _, err := db.Exec(`CREATE SCHEMA IF NOT EXISTS ` + schemaName); err != nil {
		return fmt.Errorf("create migration schema %s: %w", schemaName, err)
	}
	return nil
}

func applyMigrationSearchPath(db *sql.DB, searchPath string) error {
	if _, err := db.Exec(`SET search_path TO ` + searchPath); err != nil {
		return fmt.Errorf("set migration search_path %s: %w", searchPath, err)
	}
	return nil
}

func isIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}
