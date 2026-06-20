package repo

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
)

type dbExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type PostgresStore struct {
	db   dbExecutor
	root *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: postgresExecutor{inner: db}, root: db}
}

func (store *PostgresStore) WithinTx(ctx context.Context, fn func(ctx context.Context, repositories Repositories) error) error {
	if store.root == nil {
		return fn(ctx, store)
	}
	tx, err := store.root.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	txStore := &PostgresStore{db: postgresExecutor{inner: tx}}
	if err := fn(ctx, txStore); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

type postgresExecutor struct {
	inner dbExecutor
}

func (executor postgresExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return executor.inner.ExecContext(ctx, rebindPostgresPlaceholders(query), args...)
}

func (executor postgresExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return executor.inner.QueryContext(ctx, rebindPostgresPlaceholders(query), args...)
}

func (executor postgresExecutor) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return executor.inner.QueryRowContext(ctx, rebindPostgresPlaceholders(query), args...)
}

func rebindPostgresPlaceholders(query string) string {
	if !strings.ContainsRune(query, '?') {
		return query
	}
	var builder strings.Builder
	builder.Grow(len(query) + 8)
	index := 1
	inSingleQuote := false
	inDoubleQuote := false
	inLineComment := false
	inBlockComment := false
	runes := []rune(query)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}
		switch {
		case inLineComment:
			builder.WriteRune(r)
			if r == '\n' {
				inLineComment = false
			}
		case inBlockComment:
			builder.WriteRune(r)
			if r == '*' && next == '/' {
				builder.WriteRune(next)
				i++
				inBlockComment = false
			}
		case inSingleQuote:
			builder.WriteRune(r)
			if r == '\'' {
				if next == '\'' {
					builder.WriteRune(next)
					i++
					continue
				}
				inSingleQuote = false
			}
		case inDoubleQuote:
			builder.WriteRune(r)
			if r == '"' {
				if next == '"' {
					builder.WriteRune(next)
					i++
					continue
				}
				inDoubleQuote = false
			}
		case r == '-' && next == '-':
			builder.WriteRune(r)
			builder.WriteRune(next)
			i++
			inLineComment = true
		case r == '/' && next == '*':
			builder.WriteRune(r)
			builder.WriteRune(next)
			i++
			inBlockComment = true
		case r == '\'':
			builder.WriteRune(r)
			inSingleQuote = true
		case r == '"':
			builder.WriteRune(r)
			inDoubleQuote = true
		case r == '?':
			builder.WriteByte('$')
			builder.WriteString(strconv.Itoa(index))
			index++
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func (store *PostgresStore) Users() UserRepository {
	return store
}

func (store *PostgresStore) Organizations() OrganizationRepository {
	return store
}

func (store *PostgresStore) Members() MemberRepository {
	return store
}

func (store *PostgresStore) Roles() RoleRepository {
	return store
}

func (store *PostgresStore) NodeGroups() NodeGroupRepository {
	return store
}

func (store *PostgresStore) Nodes() NodeRepository {
	return store
}

func (store *PostgresStore) MonitorGroups() MonitorGroupRepository {
	return store
}

func (store *PostgresStore) Monitors() MonitorRepository {
	return store
}

func (store *PostgresStore) Targets() TargetRepository {
	return store
}

func (store *PostgresStore) TargetGroups() TargetGroupRepository {
	return store
}

func (store *PostgresStore) Rules() RuleRepository {
	return store
}

func (store *PostgresStore) Quotas() QuotaRepository {
	return store
}

func (store *PostgresStore) AgentRegistrationTokens() AgentRegistrationTokenRepository {
	return store
}

func (store *PostgresStore) AgentCredentials() AgentCredentialRepository {
	return store
}

func (store *PostgresStore) AuditLogs() AuditLogRepository {
	return store
}
