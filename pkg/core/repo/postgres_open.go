package repo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

const defaultPostgresSearchPath = "app,auth,public"
const defaultPostgresPingTimeout = 5 * time.Second

func OpenPostgres(databaseURL string) (*sql.DB, error) {
	config, err := postgresConnConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	db := stdlib.OpenDB(*config)
	if err := configurePostgres(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func postgresConnConfig(databaseURL string) (*pgx.ConnConfig, error) {
	normalizedURL := strings.TrimSpace(databaseURL)
	if normalizedURL == "" {
		return nil, errors.New("PostgreSQL DATABASE_URL is required")
	}
	config, err := pgx.ParseConfig(normalizedURL)
	if err != nil {
		return nil, err
	}
	if config.RuntimeParams == nil {
		config.RuntimeParams = make(map[string]string)
	}
	config.RuntimeParams["search_path"] = defaultPostgresSearchPath
	return config, nil
}

func configurePostgres(db *sql.DB) error {
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	ctx, cancel := context.WithTimeout(context.Background(), defaultPostgresPingTimeout)
	defer cancel()
	return db.PingContext(ctx)
}
