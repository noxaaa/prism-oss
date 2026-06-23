package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"testing"
)

func TestRunRejectsMissingDatabaseURL(t *testing.T) {
	err := run([]string{"up"})
	if err == nil {
		t.Fatalf("expected missing PostgreSQL database URL error")
	}
}

func TestCoreMigrationCreatesPostgresSchemasAndVersionTables(t *testing.T) {
	db := openMigratedCoreDB(t)
	defer func() { _ = db.Close() }()

	for _, table := range []struct {
		schema string
		name   string
	}{
		{schema: "auth", name: "user"},
		{schema: "auth", name: "goose_db_version_auth"},
		{schema: "app", name: "organizations"},
		{schema: "app", name: "goose_db_version_core"},
	} {
		if !tableExistsInSchema(t, db, table.schema, table.name) {
			t.Fatalf("expected %s.%s to exist", table.schema, table.name)
		}
	}
}

func TestCoreInitialMigrationIncludesAgentUpdateColumns(t *testing.T) {
	db := openMigratedCoreDB(t)
	defer func() { _ = db.Close() }()

	for _, column := range []string{
		"agent_version",
		"agent_commit",
		"agent_build_time",
		"agent_auto_update_enabled",
		"desired_agent_version",
		"agent_update_status",
		"agent_update_error",
		"agent_update_started_at",
		"agent_update_finished_at",
	} {
		if !nodesTableHasColumn(t, db, column) {
			t.Fatalf("nodes table missing %s", column)
		}
	}
}

func TestCoreMigrationRemovesLegacyDNSRecordsAndHealthDNSActions(t *testing.T) {
	db := openMigratedCoreDB(t)
	defer func() { _ = db.Close() }()

	if tableExistsInSchema(t, db, "app", "dns_records") {
		t.Fatalf("legacy app.dns_records table must not exist after current core migrations")
	}

	mustExec(t, db, `INSERT INTO organizations (id, name, slug, created_at, updated_at) VALUES ('11111111-1111-1111-1111-111111111111', 'Org', 'org', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO health_checks (id, organization_id, name, probe_type, interval_seconds, timeout_seconds, created_at, updated_at) VALUES ('22222222-2222-2222-2222-222222222222', '11111111-1111-1111-1111-111111111111', 'Health', 'TCP_PORT', 30, 5, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO health_evaluation_rules (id, organization_id, health_check_id, name, enabled, expression_json, created_at, updated_at) VALUES ('33333333-3333-3333-3333-333333333333', '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', 'Notify', true, '{}'::jsonb, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO health_events (id, organization_id, health_evaluation_rule_id, event_type, config_json, enabled, created_at, updated_at) VALUES ('44444444-4444-4444-4444-444444444444', '11111111-1111-1111-1111-111111111111', '33333333-3333-3333-3333-333333333333', 'WEBHOOK', '{}'::jsonb, true, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	expectExecError(t, db, `INSERT INTO health_events (id, organization_id, health_evaluation_rule_id, event_type, config_json, enabled, created_at, updated_at) VALUES ('55555555-5555-5555-5555-555555555555', '11111111-1111-1111-1111-111111111111', '33333333-3333-3333-3333-333333333333', 'DNS_' || 'FAILOVER', '{}'::jsonb, true, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
}

func TestCoreMigrationMigratesPublishedDNSRecordsToManagedRecords(t *testing.T) {
	db := openMigratedCoreDBToVersion(t, 4)
	defer func() { _ = db.Close() }()

	mustExec(t, db, `INSERT INTO organizations (id, name, slug, created_at, updated_at) VALUES ('11111111-1111-1111-1111-111111111111', 'Org', 'org', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO dns_credentials (id, organization_id, provider, name, encrypted_secret, created_at, updated_at) VALUES ('22222222-2222-2222-2222-222222222222', '11111111-1111-1111-1111-111111111111', 'CLOUDFLARE', 'Cloudflare', 'encrypted', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO dns_records (id, organization_id, dns_credential_id, zone, record_name, record_type, desired_values_json, last_applied_values_json, last_applied_at, created_at, updated_at) VALUES ('33333333-3333-3333-3333-333333333333', '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', 'zone-example', 'www.example.com', 'A', '["192.0.2.10","192.0.2.11"]'::jsonb, '["192.0.2.10"]'::jsonb, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO health_checks (id, organization_id, name, probe_type, interval_seconds, timeout_seconds, created_at, updated_at) VALUES ('44444444-4444-4444-4444-444444444444', '11111111-1111-1111-1111-111111111111', 'Health', 'TCP_PORT', 30, 5, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO health_evaluation_rules (id, organization_id, health_check_id, name, enabled, expression_json, created_at, updated_at) VALUES ('55555555-5555-5555-5555-555555555555', '11111111-1111-1111-1111-111111111111', '44444444-4444-4444-4444-444444444444', 'Legacy DNS', true, '{}'::jsonb, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO health_events (id, organization_id, health_evaluation_rule_id, event_type, config_json, enabled, created_at, updated_at) VALUES ('66666666-6666-6666-6666-666666666666', '11111111-1111-1111-1111-111111111111', '55555555-5555-5555-5555-555555555555', 'DNS_FAILOVER', '{"dns_record_id":"33333333-3333-3333-3333-333333333333","failover_values":["192.0.2.20"]}'::jsonb, true, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)

	if err := run([]string{
		"-database", testDatabaseURL(t, db),
		"-dir", "../../migrations/core",
		"up",
	}); err != nil {
		t.Fatalf("run remaining core migrations: %v", err)
	}

	if tableExistsInSchema(t, db, "app", "dns_records") {
		t.Fatalf("legacy app.dns_records table must be dropped by forward migration")
	}

	var recordHost, recordName, recordType, lastApplied string
	if err := db.QueryRow(`
		SELECT record_host, record_name, record_type, last_applied_values_json::text
		FROM dns_managed_records
		WHERE id = '33333333-3333-3333-3333-333333333333'
	`).Scan(&recordHost, &recordName, &recordType, &lastApplied); err != nil {
		t.Fatalf("read migrated managed record: %v", err)
	}
	if recordHost != "www.example.com" || recordName != "www.example.com" || recordType != "A" {
		t.Fatalf("unexpected migrated managed record: host=%q name=%q type=%q", recordHost, recordName, recordType)
	}
	assertJSONStringList(t, lastApplied, []string{"192.0.2.10"})

	var actionJSON, outputJSON string
	if err := db.QueryRow(`
		SELECT action_json::text, last_output_values_json::text
		FROM dns_instances
		WHERE managed_record_id = '33333333-3333-3333-3333-333333333333'
	`).Scan(&actionJSON, &outputJSON); err != nil {
		t.Fatalf("read migrated DNS instance: %v", err)
	}
	assertJSONField(t, actionJSON, "type", "SET_STATIC_ADDRESSES")
	assertJSONStringList(t, outputJSON, []string{"192.0.2.10", "192.0.2.11"})

	var dnsEvents int
	if err := db.QueryRow(`SELECT COUNT(*) FROM health_events WHERE event_type LIKE 'DNS_%'`).Scan(&dnsEvents); err != nil {
		t.Fatalf("count migrated DNS health events: %v", err)
	}
	if dnsEvents != 0 {
		t.Fatalf("expected DNS health events to be removed, got %d", dnsEvents)
	}
}

func TestCoreMigrationSkipsCaseOnlyDuplicateLegacyDNSRecords(t *testing.T) {
	db := openMigratedCoreDBToVersion(t, 4)
	defer func() { _ = db.Close() }()

	mustExec(t, db, `INSERT INTO organizations (id, name, slug, created_at, updated_at) VALUES ('11111111-1111-1111-1111-111111111111', 'Org', 'org', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO dns_credentials (id, organization_id, provider, name, encrypted_secret, created_at, updated_at) VALUES ('22222222-2222-2222-2222-222222222222', '11111111-1111-1111-1111-111111111111', 'CLOUDFLARE', 'Cloudflare', 'encrypted', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO dns_records (id, organization_id, dns_credential_id, zone, record_name, record_type, desired_values_json, created_at, updated_at) VALUES ('33333333-3333-3333-3333-333333333333', '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', 'zone-example', 'www.example.com', 'A', '["192.0.2.10"]'::jsonb, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO dns_records (id, organization_id, dns_credential_id, zone, record_name, record_type, desired_values_json, created_at, updated_at) VALUES ('44444444-4444-4444-4444-444444444444', '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', 'zone-example', 'WWW.example.com', 'A', '["192.0.2.11"]'::jsonb, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)

	if err := run([]string{
		"-database", testDatabaseURL(t, db),
		"-dir", "../../migrations/core",
		"up",
	}); err != nil {
		t.Fatalf("run remaining core migrations with duplicate legacy DNS records: %v", err)
	}

	var managedRecords int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM dns_managed_records
		WHERE organization_id = '11111111-1111-1111-1111-111111111111'
		  AND zone_id = 'zone-example'
		  AND lower(record_name) = 'www.example.com'
		  AND record_type = 'A'
	`).Scan(&managedRecords); err != nil {
		t.Fatalf("count migrated duplicate managed records: %v", err)
	}
	if managedRecords != 1 {
		t.Fatalf("expected one migrated managed record after case-only duplicate skip, got %d", managedRecords)
	}
}

func TestCoreMigrationPreservesOneCaseOnlyDuplicateLegacyCNAME(t *testing.T) {
	db := openMigratedCoreDBToVersion(t, 4)
	defer func() { _ = db.Close() }()

	mustExec(t, db, `INSERT INTO organizations (id, name, slug, created_at, updated_at) VALUES ('11111111-1111-1111-1111-111111111111', 'Org', 'org', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO dns_credentials (id, organization_id, provider, name, encrypted_secret, created_at, updated_at) VALUES ('22222222-2222-2222-2222-222222222222', '11111111-1111-1111-1111-111111111111', 'CLOUDFLARE', 'Cloudflare', 'encrypted', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO dns_records (id, organization_id, dns_credential_id, zone, record_name, record_type, desired_values_json, created_at, updated_at) VALUES ('33333333-3333-3333-3333-333333333333', '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', 'zone-example', 'alias.example.com', 'CNAME', '["origin.example.com"]'::jsonb, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO dns_records (id, organization_id, dns_credential_id, zone, record_name, record_type, desired_values_json, created_at, updated_at) VALUES ('44444444-4444-4444-4444-444444444444', '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', 'zone-example', 'ALIAS.example.com', 'CNAME', '["other.example.com"]'::jsonb, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)

	if err := run([]string{
		"-database", testDatabaseURL(t, db),
		"-dir", "../../migrations/core",
		"up",
	}); err != nil {
		t.Fatalf("run remaining core migrations with duplicate legacy CNAME records: %v", err)
	}

	var managedRecords int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM dns_managed_records
		WHERE organization_id = '11111111-1111-1111-1111-111111111111'
		  AND zone_id = 'zone-example'
		  AND lower(record_name) = 'alias.example.com'
		  AND record_type = 'CNAME'
	`).Scan(&managedRecords); err != nil {
		t.Fatalf("count migrated duplicate CNAME managed records: %v", err)
	}
	if managedRecords != 1 {
		t.Fatalf("expected one migrated CNAME managed record after case-only duplicate skip, got %d", managedRecords)
	}
}

func TestCoreMigrationRepairsPublishedDNSPolicyState(t *testing.T) {
	db := openMigratedCoreDBToVersion(t, 7)
	defer func() { _ = db.Close() }()

	mustExec(t, db, `INSERT INTO organizations (id, name, slug, created_at, updated_at) VALUES ('11111111-1111-1111-1111-111111111111', 'Org', 'org', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO dns_credentials (id, organization_id, provider, name, encrypted_secret, created_at, updated_at) VALUES ('22222222-2222-2222-2222-222222222222', '11111111-1111-1111-1111-111111111111', 'CLOUDFLARE', 'Cloudflare', 'encrypted', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO dns_credential_zones (id, organization_id, dns_credential_id, zone_id, zone_name, status, last_synced_at, created_at, updated_at) VALUES ('33333333-3333-3333-3333-333333333333', '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', 'zone-example', 'example.com', 'ACTIVE', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO dns_managed_records (id, organization_id, dns_credential_id, credential_zone_id, zone_id, zone_name, record_host, record_name, record_type, ttl, proxied, last_applied_values_json, last_evaluation_status, last_evaluation_error, last_diagnostics_json, created_at, updated_at) VALUES ('44444444-4444-4444-4444-444444444444', '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', '33333333-3333-3333-3333-333333333333', 'zone-example', 'example.com', 'www', 'www.example.com', 'A', 60, false, '["192.0.2.10"]'::jsonb, 'APPLIED', '', '[]'::jsonb, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO dns_instances (id, organization_id, managed_record_id, name, priority, enabled, node_group_ids_json, answer_count, condition_json, action_json, notification_channel_ids_json, last_output_values_json, last_status, last_diagnostics_json, created_at, updated_at) VALUES ('55555555-5555-5555-5555-555555555555', '11111111-1111-1111-1111-111111111111', '44444444-4444-4444-4444-444444444444', 'Disabled instance', 100, false, '[]'::jsonb, -1, '{}'::jsonb, '{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}'::jsonb, '[]'::jsonb, '["192.0.2.10"]'::jsonb, 'APPLIED', '[]'::jsonb, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `UPDATE dns_managed_records SET active_instance_id = '55555555-5555-5555-5555-555555555555' WHERE id = '44444444-4444-4444-4444-444444444444'`)

	if err := run([]string{
		"-database", testDatabaseURL(t, db),
		"-dir", "../../migrations/core",
		"up",
	}); err != nil {
		t.Fatalf("run remaining core migrations: %v", err)
	}

	if tableExistsInSchema(t, db, "app", "dns_records") {
		t.Fatalf("legacy app.dns_records table must be dropped by forward migration")
	}

	var activeInstanceID sql.NullString
	var status string
	var diagnostics string
	if err := db.QueryRow(`
		SELECT active_instance_id::text, last_evaluation_status, last_diagnostics_json::text
		FROM dns_managed_records
		WHERE id = '44444444-4444-4444-4444-444444444444'
	`).Scan(&activeInstanceID, &status, &diagnostics); err != nil {
		t.Fatalf("read repaired managed record: %v", err)
	}
	if activeInstanceID.Valid {
		t.Fatalf("expected stale active instance to be cleared, got %s", activeInstanceID.String)
	}
	if status != "PENDING" {
		t.Fatalf("expected repaired record to return to PENDING, got %s", status)
	}
	assertJSONContainsDiagnostic(t, diagnostics, "STALE_ACTIVE_INSTANCE_CLEARED")
}

func TestCoreMigrationRestrictsHealthEventConstraintToSupportedExecutors(t *testing.T) {
	db := openMigratedCoreDBToVersion(t, 8)
	defer func() { _ = db.Close() }()

	if err := run([]string{
		"-database", testDatabaseURL(t, db),
		"-dir", "../../migrations/core",
		"up",
	}); err != nil {
		t.Fatalf("run remaining core migrations: %v", err)
	}

	mustExec(t, db, `INSERT INTO organizations (id, name, slug, created_at, updated_at) VALUES ('11111111-1111-1111-1111-111111111111', 'Org', 'org', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO health_checks (id, organization_id, name, probe_type, interval_seconds, timeout_seconds, created_at, updated_at) VALUES ('22222222-2222-2222-2222-222222222222', '11111111-1111-1111-1111-111111111111', 'Health', 'TCP_PORT', 30, 5, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO health_evaluation_rules (id, organization_id, health_check_id, name, enabled, expression_json, created_at, updated_at) VALUES ('33333333-3333-3333-3333-333333333333', '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', 'Notify', true, '{}'::jsonb, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO health_events (id, organization_id, health_evaluation_rule_id, event_type, config_json, enabled, created_at, updated_at) VALUES ('44444444-4444-4444-4444-444444444444', '11111111-1111-1111-1111-111111111111', '33333333-3333-3333-3333-333333333333', 'WEBHOOK', '{}'::jsonb, true, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	expectExecError(t, db, `INSERT INTO health_events (id, organization_id, health_evaluation_rule_id, event_type, config_json, enabled, created_at, updated_at) VALUES ('55555555-5555-5555-5555-555555555555', '11111111-1111-1111-1111-111111111111', '33333333-3333-3333-3333-333333333333', 'DNS_FAILOVER', '{}'::jsonb, true, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	expectExecError(t, db, `INSERT INTO health_events (id, organization_id, health_evaluation_rule_id, event_type, config_json, enabled, created_at, updated_at) VALUES ('66666666-6666-6666-6666-666666666666', '11111111-1111-1111-1111-111111111111', '33333333-3333-3333-3333-333333333333', 'CUSTOM_NOTIFICATION', '{}'::jsonb, true, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
}

func tableExistsInSchema(t *testing.T, db *sql.DB, schema string, table string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = $1 AND table_name = $2
	`, schema, table).Scan(&count); err != nil {
		t.Fatalf("read table %s.%s existence: %v", schema, table, err)
	}
	return count > 0
}

func testDatabaseURL(t *testing.T, db *sql.DB) string {
	t.Helper()
	var databaseName string
	if err := db.QueryRow(`SELECT current_database()`).Scan(&databaseName); err != nil {
		t.Fatalf("read current database: %v", err)
	}
	baseURL := getenvNonEmpty("TEST_DATABASE_URL", "DATABASE_URL")
	if baseURL == "" {
		t.Fatalf("TEST_DATABASE_URL or DATABASE_URL is required")
	}
	databaseURL, err := databaseURLWithName(baseURL, databaseName, "")
	if err != nil {
		t.Fatalf("build database URL: %v", err)
	}
	return databaseURL
}

func getenvNonEmpty(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func assertJSONField(t *testing.T, raw string, key string, want string) {
	t.Helper()
	var values map[string]any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		t.Fatalf("parse JSON object %s: %v", raw, err)
	}
	if got, _ := values[key].(string); got != want {
		t.Fatalf("expected JSON field %s=%q, got %q in %s", key, want, got, raw)
	}
}

func assertJSONStringList(t *testing.T, raw string, want []string) {
	t.Helper()
	var got []string
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("parse JSON list %s: %v", raw, err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected JSON list %v, got %v", want, got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("expected JSON list %v, got %v", want, got)
		}
	}
}

func assertJSONContainsDiagnostic(t *testing.T, raw string, code string) {
	t.Helper()
	var diagnostics []map[string]any
	if err := json.Unmarshal([]byte(raw), &diagnostics); err != nil {
		t.Fatalf("parse diagnostics JSON %s: %v", raw, err)
	}
	for _, diagnostic := range diagnostics {
		if got, _ := diagnostic["code"].(string); got == code {
			return
		}
	}
	t.Fatalf("expected diagnostic %q in %s", code, raw)
}

func nodesTableHasColumn(t *testing.T, db *sql.DB, column string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_schema = 'app' AND table_name = 'nodes' AND column_name = $1
	`, column).Scan(&count); err != nil {
		t.Fatalf("read nodes column %s: %v", column, err)
	}
	return count > 0
}
