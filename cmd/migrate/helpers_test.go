package main

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func openMigratedCoreDB(t *testing.T) *sql.DB {
	t.Helper()

	databasePath := filepath.Join(t.TempDir(), "app.db")
	if err := run([]string{
		"-database", databasePath,
		"-dir", "../../migrations/core",
		"up",
	}); err != nil {
		t.Fatalf("run migrate up: %v", err)
	}

	db, err := openSQLite(databasePath)
	if err != nil {
		t.Fatalf("open migrated database: %v", err)
	}
	return db
}

func seedTenantFixture(t *testing.T, db *sql.DB) {
	t.Helper()

	statements := []string{
		`INSERT INTO "user" (id, name, email, emailVerified, createdAt, updatedAt) VALUES ('user_a', 'User A', 'a@example.com', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO "user" (id, name, email, emailVerified, createdAt, updatedAt) VALUES ('user_b', 'User B', 'b@example.com', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
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
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
		t.Fatalf("read table %s existence: %v", table, err)
	}
	return count > 0
}

func mustExec(t *testing.T, db *sql.DB, statement string) {
	t.Helper()
	if _, err := db.Exec(statement); err != nil {
		t.Fatalf("exec statement: %v\n%s", err, statement)
	}
}

func expectExecError(t *testing.T, db *sql.DB, statement string) {
	t.Helper()
	if _, err := db.Exec(statement); err == nil {
		t.Fatalf("expected statement to fail:\n%s", statement)
	}
}
