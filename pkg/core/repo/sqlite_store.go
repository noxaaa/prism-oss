package repo

import (
	"context"
	"database/sql"
)

type dbExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type SQLiteStore struct {
	db dbExecutor
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func (store *SQLiteStore) WithinTx(ctx context.Context, fn func(ctx context.Context, repositories Repositories) error) error {
	db, ok := store.db.(*sql.DB)
	if !ok {
		return fn(ctx, store)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	txStore := &SQLiteStore{db: tx}
	if err := fn(ctx, txStore); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (store *SQLiteStore) Users() UserRepository {
	return store
}

func (store *SQLiteStore) Organizations() OrganizationRepository {
	return store
}

func (store *SQLiteStore) Members() MemberRepository {
	return store
}

func (store *SQLiteStore) Roles() RoleRepository {
	return store
}

func (store *SQLiteStore) NodeGroups() NodeGroupRepository {
	return store
}

func (store *SQLiteStore) Nodes() NodeRepository {
	return store
}

func (store *SQLiteStore) MonitorGroups() MonitorGroupRepository {
	return store
}

func (store *SQLiteStore) Monitors() MonitorRepository {
	return store
}

func (store *SQLiteStore) Targets() TargetRepository {
	return store
}

func (store *SQLiteStore) TargetGroups() TargetGroupRepository {
	return store
}

func (store *SQLiteStore) Rules() RuleRepository {
	return store
}

func (store *SQLiteStore) Quotas() QuotaRepository {
	return store
}

func (store *SQLiteStore) AgentRegistrationTokens() AgentRegistrationTokenRepository {
	return store
}

func (store *SQLiteStore) AgentCredentials() AgentCredentialRepository {
	return store
}

func (store *SQLiteStore) AuditLogs() AuditLogRepository {
	return store
}
