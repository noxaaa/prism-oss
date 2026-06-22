package arch

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestOSSMigrationsIncludeMonitorHealthAndDNSTables(t *testing.T) {
	root := repoRoot(t)
	initial := readText(t, filepath.Join(root, "migrations", "core", "00001_core.sql"))
	for _, forbidden := range []string{
		"CREATE TABLE monitor_groups",
		"CREATE TABLE health_checks",
		"CREATE TABLE dns_records",
	} {
		if strings.Contains(initial, forbidden) {
			t.Fatalf("new monitor/health/DNS schema must live in a forward migration, not 00001_core.sql; found %q", forbidden)
		}
	}

	source := readText(t, filepath.Join(root, "migrations", "core", "00003_monitor_health_dns.sql"))
	for _, required := range []string{
		"CREATE TABLE monitor_groups",
		"CREATE TABLE monitors",
		"CREATE TABLE monitor_group_members",
		"CREATE TABLE health_checks",
		"CREATE TABLE health_check_targets",
		"CREATE TABLE health_check_monitor_scopes",
		"CREATE TABLE health_results",
		"CREATE TABLE health_evaluation_rules",
		"CREATE TABLE health_events",
		"'WEBHOOK'",
		"'EMAIL'",
		"CREATE TABLE dns_credentials",
		"CREATE TABLE dns_records",
		"CREATE UNIQUE INDEX uniq_dns_records_active_name",
		"ON dns_records(organization_id, zone, record_name, record_type)",
		"WHERE deleted_at IS NULL",
		"validate_monitor_agent_auth",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("OSS core migration must include monitor/health/DNS foundation; missing %q", required)
		}
	}
	if strings.Contains(source, "commercial_health") {
		t.Fatalf("OSS core migration must not reference commercial health capability names")
	}
}

func TestOSSReadmesExposeMonitorAgentLifecycleCommands(t *testing.T) {
	root := repoRoot(t)
	for _, relative := range []string{"README.md", "README.zh-CN.md"} {
		source := readText(t, filepath.Join(root, filepath.FromSlash(relative)))
		for _, required := range []string{
			"install-monitor-agent.sh",
			"uninstall-monitor-agent.sh",
			"prism-monitor-agent",
			"DNS_SECRET_ENCRYPTION_KEY",
		} {
			if !strings.Contains(source, required) {
				t.Fatalf("%s must include monitor-agent and DNS secret documentation; missing %q", relative, required)
			}
		}
	}
}

func TestHealthCheckTargetQueriesExcludeDeletedTargets(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "pkg", "core", "repo", "health_dns.go"))
	if !strings.Contains(source, "targets.deleted_at IS NULL") {
		t.Fatalf("health check target queries must exclude soft-deleted targets from monitor configs")
	}
}

func TestHealthCheckTargetsSupportEmptyTargetGroupBindings(t *testing.T) {
	root := repoRoot(t)
	migration := readText(t, filepath.Join(root, "migrations", "core", "00003_monitor_health_dns.sql"))
	for _, required := range []string{
		"target_id uuid",
		"CREATE UNIQUE INDEX uniq_health_check_targets_group_binding",
		"WHERE scope_type = 'TARGET_GROUP' AND target_id IS NULL",
	} {
		if !strings.Contains(migration, required) {
			t.Fatalf("health check targets must support empty target-group bindings; missing %q", required)
		}
	}
	repoSource := readText(t, filepath.Join(root, "pkg", "core", "repo", "health_dns.go"))
	for _, required := range []string{
		"LEFT JOIN targets",
		"COALESCE(hct.target_id::text, '')",
		"NULLIF(?, '')::uuid",
	} {
		if !strings.Contains(repoSource, required) {
			t.Fatalf("health check target repo must preserve empty target-group bindings; missing %q", required)
		}
	}
}

func TestDNSRecordUpdatePersistsAppliedProviderState(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "pkg", "core", "repo", "health_dns.go"))
	updateIndex := strings.Index(source, "func (store *PostgresStore) UpdateDNSRecord(")
	if updateIndex == -1 {
		t.Fatalf("UpdateDNSRecord implementation not found")
	}
	lastAppliedValuesIndex := strings.Index(source[updateIndex:], "last_applied_values_json = ?::jsonb")
	lastAppliedAtIndex := strings.Index(source[updateIndex:], "last_applied_at = NULLIF(?, '')::timestamptz")
	if lastAppliedValuesIndex == -1 || lastAppliedAtIndex == -1 {
		t.Fatalf("UpdateDNSRecord must persist last_applied_values_json and last_applied_at")
	}
}

func TestHealthEvaluationRuleQueriesCloseRowsBeforeLoadingEvents(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "pkg", "core", "repo", "health_dns.go"))
	functionIndex := strings.Index(source, "func (store *PostgresStore) ListHealthEvaluationRulesByCheck(")
	if functionIndex == -1 {
		t.Fatalf("ListHealthEvaluationRulesByCheck implementation not found")
	}
	functionSource := source[functionIndex:]
	closeIndex := strings.Index(functionSource, "if err := rows.Close(); err != nil")
	eventsIndex := strings.Index(functionSource, "store.listHealthEventsByRule")
	if closeIndex == -1 || eventsIndex == -1 {
		t.Fatalf("ListHealthEvaluationRulesByCheck must close rule rows before loading events")
	}
	if closeIndex > eventsIndex {
		t.Fatalf("ListHealthEvaluationRulesByCheck loads events while rule rows are still open")
	}
}
