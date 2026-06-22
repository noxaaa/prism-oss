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

func TestMonitorConsoleNavigationRequiresReadPermission(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "apps", "web", "src", "components", "console", "edition-registry.ts"))
	required := `{ href: "/console/admin/monitors", icon: RadarIcon, key: "monitors", labelKey: "nav.monitors", permissions: ["monitors.read"] }`
	if !strings.Contains(source, required) {
		t.Fatalf("monitors nav item must require monitors.read because the page reads monitor lists")
	}
	required = `{ href: "/console/admin/health", icon: HeartPulseIcon, key: "health", labelKey: "nav.health", permissions: ["health_checks.read"] }`
	if !strings.Contains(source, required) {
		t.Fatalf("health nav item must require health_checks.read because the page reads health-check lists")
	}
}

func TestMonitorAgentUninstallAcceptsCredentialFile(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "cmd", "monitor-agent", "main.go"))
	if !strings.Contains(source, `flags.StringVar(&options.CredentialFile, "credential-file"`) {
		t.Fatalf("monitor-agent uninstall must accept --credential-file so purge can remove non-default credentials")
	}
	if strings.Contains(source, "os.RemoveAll(filepath.Dir(options.CredentialFile))") {
		t.Fatalf("monitor-agent purge must not delete a custom credential file's parent directory")
	}
	for _, required := range []string{
		"os.Remove(options.CredentialFile)",
		"filepath.Clean(options.CredentialFile) == defaultCredentialFile(options.ServiceName)",
		"os.Remove(filepath.Dir(options.CredentialFile))",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("monitor-agent purge must remove only the selected credential and clean the default state dir when empty; missing %q", required)
		}
	}
	script := readText(t, filepath.Join(root, "scripts", "uninstall-monitor-agent.sh"))
	for _, required := range []string{
		`--credential-file) credential_file=`,
		`--credential-file "$credential_file"`,
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("uninstall-monitor-agent.sh must pass through --credential-file; missing %q", required)
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

func TestDNSRecordWritesMapConstraintErrorsToConflicts(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "pkg", "core", "repo", "health_dns.go"))
	createIndex := strings.Index(source, "func (store *PostgresStore) CreateDNSRecord(")
	updateIndex := strings.Index(source, "func (store *PostgresStore) UpdateDNSRecord(")
	if createIndex == -1 || updateIndex == -1 {
		t.Fatalf("DNS record write implementations not found")
	}
	createSource := source[createIndex:updateIndex]
	updateSource := source[updateIndex:]
	if !strings.Contains(createSource, "return mapWriteError(err)") {
		t.Fatalf("CreateDNSRecord must map unique constraint failures to repo.ErrConflict")
	}
	if !strings.Contains(updateSource, "return mapWriteError(err)") {
		t.Fatalf("UpdateDNSRecord must map unique constraint failures to repo.ErrConflict")
	}
}

func TestHealthCheckChildrenAreDiffedOnUpdate(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "pkg", "core", "repo", "health_dns.go"))
	functionIndex := strings.Index(source, "func (store *PostgresStore) replaceHealthCheckChildren(")
	if functionIndex == -1 {
		t.Fatalf("replaceHealthCheckChildren implementation not found")
	}
	functionSource := source[functionIndex:]
	for _, forbidden := range []string{
		"`DELETE FROM health_check_targets WHERE organization_id = ? AND health_check_id = ?`",
		"`DELETE FROM health_check_monitor_scopes WHERE organization_id = ? AND health_check_id = ?`",
	} {
		if strings.Contains(functionSource, forbidden) {
			t.Fatalf("health check child update must preserve unchanged child rows; found unconditional delete %q", forbidden)
		}
	}
	for _, required := range []string{
		"replaceHealthCheckTargets",
		"replaceHealthCheckMonitorScopes",
		"healthCheckTargetKey",
		"healthCheckMonitorScopeKey",
		"ON CONFLICT DO NOTHING",
	} {
		if !strings.Contains(functionSource, required) {
			t.Fatalf("health check child update must diff bindings and preserve unchanged rows; missing %q", required)
		}
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

func TestHealthEvaluationUsesActionRegistryBoundary(t *testing.T) {
	root := repoRoot(t)
	actionsSource := readText(t, filepath.Join(root, "pkg", "core", "service", "control_health_actions.go"))
	for _, forbidden := range []string{"DNS_FAILOVER", "DNS_DELETE_OFFLINE", "DNS_DELETE_ALL", "DNS_RESTORE", "executeDNSProviderAction"} {
		if strings.Contains(actionsSource, forbidden) {
			t.Fatalf("health evaluation must dispatch actions through the registry, not embed DNS action logic; found %q", forbidden)
		}
	}
	executorSource := readText(t, filepath.Join(root, "pkg", "core", "service", "health_event_executor.go"))
	for _, required := range []string{"type HealthActionRegistry", "SupportedHealthActionTypes", "HealthActionTypes"} {
		if !strings.Contains(executorSource, required) {
			t.Fatalf("health action layer must expose an extension registry for future Webhook/Email executors; missing %q", required)
		}
	}
}
