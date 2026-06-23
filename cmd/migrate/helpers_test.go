package main

import (
	"database/sql"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
)

func openMigratedCoreDB(t *testing.T) *sql.DB {
	t.Helper()
	return openMigratedCoreDBToVersion(t, 0)
}

func openMigratedCoreDBToVersion(t *testing.T, coreVersion int64) *sql.DB {
	t.Helper()

	baseURL := os.Getenv("TEST_DATABASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("DATABASE_URL")
	}
	if baseURL == "" {
		t.Skip("TEST_DATABASE_URL or DATABASE_URL is required for PostgreSQL migration tests")
	}
	databaseName := "prism_migrate_test_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	adminDB, err := sql.Open("pgx", baseURL)
	if err != nil {
		t.Fatalf("open postgres admin database: %v", err)
	}
	if _, err := adminDB.Exec(`CREATE DATABASE ` + quoteIdentifier(databaseName)); err != nil {
		_ = adminDB.Close()
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := adminDB.Exec(`DROP DATABASE IF EXISTS ` + quoteIdentifier(databaseName) + ` WITH (FORCE)`); err != nil {
			t.Fatalf("drop test database: %v", err)
		}
		_ = adminDB.Close()
	})
	databaseURL, err := databaseURLWithName(baseURL, databaseName, "app,auth,public")
	if err != nil {
		t.Fatalf("build test database URL: %v", err)
	}
	migrationURL, err := databaseURLWithName(baseURL, databaseName, "")
	if err != nil {
		t.Fatalf("build migration database URL: %v", err)
	}
	if err := run([]string{
		"-database", migrationURL,
		"-dir", "../../migrations/auth",
		"up",
	}); err != nil {
		t.Fatalf("run auth migrate up: %v", err)
	}
	if coreVersion == 0 {
		if err := run([]string{
			"-database", migrationURL,
			"-dir", "../../migrations/core",
			"up",
		}); err != nil {
			t.Fatalf("run core migrate up: %v", err)
		}
	} else {
		coreDB, err := openPostgres(migrationURL)
		if err != nil {
			t.Fatalf("open core migration database: %v", err)
		}
		defer func() { _ = coreDB.Close() }()
		if err := goose.SetDialect("postgres"); err != nil {
			t.Fatalf("set goose dialect: %v", err)
		}
		if err := runGooseUpToWithTable(coreDB, splitMigrationDirs("../../migrations/core")[0], coreVersion); err != nil {
			t.Fatalf("run core migrate up to %d: %v", coreVersion, err)
		}
	}

	db, err := openPostgres(databaseURL)
	if err != nil {
		t.Fatalf("open migrated database: %v", err)
	}
	return db
}

func runGooseUpToWithTable(db *sql.DB, group migrationGroup, version int64) error {
	if err := ensureMigrationSchema(db, group.SchemaName); err != nil {
		return err
	}
	if err := applyMigrationSearchPath(db, group.SearchPath); err != nil {
		return err
	}
	previousTableName := goose.TableName()
	goose.SetTableName(group.TableName)
	defer goose.SetTableName(previousTableName)
	return goose.UpTo(db, group.Dir, version)
}

func seedTenantFixture(t *testing.T, db *sql.DB) {
	t.Helper()

	statements := []string{
		`INSERT INTO "user" ("id", "name", "email", "emailVerified", "createdAt", "updatedAt") VALUES ('user_a', 'User A', 'a@example.com', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO "user" ("id", "name", "email", "emailVerified", "createdAt", "updatedAt") VALUES ('user_b', 'User B', 'b@example.com', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO organizations (id, name, slug, created_at, updated_at) VALUES ('org_a', 'Org A', 'org-a', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO organizations (id, name, slug, created_at, updated_at) VALUES ('org_b', 'Org B', 'org-b', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO node_groups (id, organization_id, name, created_at, updated_at) VALUES ('node_group_a', 'org_a', 'Node Group A', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO node_groups (id, organization_id, name, created_at, updated_at) VALUES ('node_group_b', 'org_b', 'Node Group B', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO nodes (id, organization_id, name, status, created_at, updated_at) VALUES ('node_a', 'org_a', 'Node A', 'ONLINE', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO nodes (id, organization_id, name, status, created_at, updated_at) VALUES ('node_b_same_org', 'org_a', 'Node B Same Org', 'ONLINE', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO nodes (id, organization_id, name, status, created_at, updated_at) VALUES ('node_b', 'org_b', 'Node B', 'ONLINE', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO node_listen_ips (id, organization_id, node_id, listen_ip, created_at, updated_at) VALUES ('listen_ip_a', 'org_a', 'node_a', '0.0.0.0', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO node_listen_ips (id, organization_id, node_id, listen_ip, created_at, updated_at) VALUES ('listen_ip_b_same_org', 'org_a', 'node_b_same_org', '0.0.0.0', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO node_listen_ips (id, organization_id, node_id, listen_ip, created_at, updated_at) VALUES ('listen_ip_b', 'org_b', 'node_b', '0.0.0.0', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO node_port_ranges (id, organization_id, node_id, protocol, start_port, end_port, enabled, created_at, updated_at) VALUES ('node_a_tcp_range', 'org_a', 'node_a', 'TCP', 1, 65535, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO node_port_ranges (id, organization_id, node_id, protocol, start_port, end_port, enabled, created_at, updated_at) VALUES ('node_b_same_org_tcp_range', 'org_a', 'node_b_same_org', 'TCP', 1, 65535, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO node_port_ranges (id, organization_id, node_id, protocol, start_port, end_port, enabled, created_at, updated_at) VALUES ('node_b_tcp_range', 'org_b', 'node_b', 'TCP', 1, 65535, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO targets (id, organization_id, name, host, port, created_at, updated_at) VALUES ('target_a', 'org_a', 'Target A', '192.0.2.10', 443, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO targets (id, organization_id, name, host, port, created_at, updated_at) VALUES ('target_b', 'org_b', 'Target B', '198.51.100.10', 443, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO target_groups (id, organization_id, name, created_at, updated_at) VALUES ('target_group_a', 'org_a', 'Target Group A', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO target_groups (id, organization_id, name, created_at, updated_at) VALUES ('target_group_b', 'org_b', 'Target Group B', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at) VALUES ('inbound_rule_a', 'org_a', 'node_group_a', '0.0.0.0', 'TCP', 10001, 'ANY_INBOUND', '2026-01-01T00:00:00Z')`,
		`INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at) VALUES ('inbound_rule_b', 'org_b', 'node_group_b', '0.0.0.0', 'TCP', 10002, 'ANY_INBOUND', '2026-01-01T00:00:00Z')`,
	}

	for _, statement := range statements {
		mustExec(t, db, statement)
	}

	if tableExists(t, db, "organization_members") {
		rbacStatements := []string{
			`INSERT INTO organization_members (id, organization_id, user_id, status, created_at, updated_at) VALUES ('member_a', 'org_a', 'user_a', 'ACTIVE', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
			`INSERT INTO organization_members (id, organization_id, user_id, status, created_at, updated_at) VALUES ('member_b', 'org_b', 'user_b', 'ACTIVE', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
			`INSERT INTO roles (id, organization_id, key, name, created_at, updated_at) VALUES ('role_a', 'org_a', 'owner', 'Owner', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
			`INSERT INTO roles (id, organization_id, key, name, created_at, updated_at) VALUES ('role_b', 'org_b', 'owner', 'Owner', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		}
		for _, statement := range rbacStatements {
			mustExec(t, db, statement)
		}
	}

	ruleStatements := []string{
		`INSERT INTO forwarding_rules (id, organization_id, owner_user_id, name, enabled, protocol, match_type, inbound_binding_id, target_id, target_group_id, config_version, created_at, updated_at) VALUES ('rule_a', 'org_a', 'user_a', 'Rule A', 1, 'TCP', 'ANY_INBOUND', 'inbound_rule_a', 'target_a', NULL, 0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO forwarding_rules (id, organization_id, owner_user_id, name, enabled, protocol, match_type, inbound_binding_id, target_id, target_group_id, config_version, created_at, updated_at) VALUES ('rule_b', 'org_b', 'user_b', 'Rule B', 1, 'TCP', 'ANY_INBOUND', 'inbound_rule_b', 'target_b', NULL, 0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
	}
	for _, statement := range ruleStatements {
		mustExec(t, db, statement)
	}
	if tableExists(t, db, "monitors") {
		mustExec(t, db, `INSERT INTO monitors (id, organization_id, name, status, created_at, updated_at) VALUES ('monitor_a', 'org_a', 'Monitor A', 'ONLINE', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	}
}

func tableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1`, table).Scan(&count); err != nil {
		t.Fatalf("read table %s existence: %v", table, err)
	}
	return count > 0
}

func mustExec(t *testing.T, db *sql.DB, statement string) {
	t.Helper()
	statement = rewriteFixtureSQL(statement)
	if _, err := db.Exec(statement); err != nil {
		t.Fatalf("exec statement: %v\n%s", err, statement)
	}
}

func expectExecError(t *testing.T, db *sql.DB, statement string) {
	t.Helper()
	statement = rewriteFixtureSQL(statement)
	if _, err := db.Exec(statement); err == nil {
		t.Fatalf("expected statement to fail:\n%s", statement)
	}
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func databaseURLWithName(rawURL string, databaseName string, searchPath string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	parsed.Path = "/" + databaseName
	query := parsed.Query()
	if searchPath != "" {
		query.Set("options", "-c search_path="+searchPath)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

var quotedFixtureTokenPattern = regexp.MustCompile(`'([a-z][a-z0-9_]*(?:_[a-z0-9]+)*)'`)

func rewriteFixtureSQL(statement string) string {
	statement = quotedFixtureTokenPattern.ReplaceAllStringFunc(statement, func(value string) string {
		token := strings.Trim(value, "'")
		if !isUUIDFixtureToken(token) {
			return value
		}
		return "'" + fixtureUUID(token) + "'"
	})
	statement = rewriteBooleanInsertValues(statement)
	replacer := strings.NewReplacer(
		"SET enabled = 1", "SET enabled = true",
		"SET enabled = 0", "SET enabled = false",
	)
	return replacer.Replace(statement)
}

func rewriteBooleanInsertValues(statement string) string {
	lower := strings.ToLower(statement)
	valuesIndex := strings.Index(lower, "values")
	if valuesIndex < 0 {
		return statement
	}
	columnsOpen := strings.Index(statement, "(")
	if columnsOpen < 0 || columnsOpen > valuesIndex {
		return statement
	}
	columnsClose := findMatchingParen(statement, columnsOpen)
	if columnsClose < 0 || columnsClose > valuesIndex {
		return statement
	}
	valuesOpen := strings.Index(statement[valuesIndex:], "(")
	if valuesOpen < 0 {
		return statement
	}
	valuesOpen += valuesIndex
	valuesClose := findMatchingParen(statement, valuesOpen)
	if valuesClose < 0 {
		return statement
	}

	columns := splitSQLList(statement[columnsOpen+1 : columnsClose])
	values := splitSQLList(statement[valuesOpen+1 : valuesClose])
	if len(columns) != len(values) {
		return statement
	}
	for index, column := range columns {
		if !isBooleanColumn(column) {
			continue
		}
		switch strings.TrimSpace(values[index]) {
		case "1":
			values[index] = "true"
		case "0":
			values[index] = "false"
		}
	}
	return statement[:valuesOpen+1] + strings.Join(values, ", ") + statement[valuesClose:]
}

func findMatchingParen(value string, openIndex int) int {
	depth := 0
	inQuote := false
	for index := openIndex; index < len(value); index++ {
		character := value[index]
		if character == '\'' {
			inQuote = !inQuote
			continue
		}
		if inQuote {
			continue
		}
		switch character {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func splitSQLList(value string) []string {
	parts := make([]string, 0)
	start := 0
	inQuote := false
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character == '\'' {
			inQuote = !inQuote
			continue
		}
		if character == ',' && !inQuote {
			parts = append(parts, strings.TrimSpace(value[start:index]))
			start = index + 1
		}
	}
	parts = append(parts, strings.TrimSpace(value[start:]))
	return parts
}

func isBooleanColumn(column string) bool {
	normalized := strings.Trim(strings.TrimSpace(column), `"`)
	switch normalized {
	case "enabled", "emailVerified", "is_system", "agent_auto_update_enabled":
		return true
	default:
		return false
	}
}

func isUUIDFixtureToken(token string) bool {
	for _, prefix := range []string{
		"org_",
		"node_",
		"listen_ip_",
		"target_",
		"inbound_",
		"rule_",
		"member_",
		"role_",
		"monitor_",
		"quota_",
	} {
		if strings.HasPrefix(token, prefix) {
			return true
		}
	}
	return false
}

func fixtureUUID(token string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(token)).String()
}
