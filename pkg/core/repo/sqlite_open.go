package repo

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

func OpenSQLite(databaseURL string) (*sql.DB, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("sqlite database URL is required")
	}
	db, err := sql.Open("sqlite", normalizeSQLiteDatabaseURL(databaseURL))
	if err != nil {
		return nil, err
	}
	if err := configureSQLite(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func normalizeSQLiteDatabaseURL(databaseURL string) string {
	return strings.TrimPrefix(databaseURL, "sqlite://")
}

func configureSQLite(db *sql.DB) error {
	db.SetMaxOpenConns(1)
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("configure sqlite %s: %w", pragma, err)
		}
	}
	return nil
}
