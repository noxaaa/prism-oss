package arch

import (
	"os"
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
		"validate_monitor_agent_auth",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("OSS core migration must include monitor/health/DNS foundation; missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"commercial_health",
		"dns_record_id",
		"failover_values",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("OSS core migration must not include commercial or legacy action config leakage; found %q", forbidden)
		}
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
	source := readText(t, filepath.Join(root, "packages", "web-core", "src", "components", "console", "edition-registry.ts"))
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

func TestLegacyDNSRecordsAreRemovedFromOSSCore(t *testing.T) {
	root := repoRoot(t)
	for _, relative := range []string{
		"pkg/core/handler/control.go",
		"pkg/core/handler/control_health_dns.go",
		"pkg/core/service/control_health_dns.go",
		"pkg/core/repo/health_dns.go",
		"pkg/core/repo/repository.go",
		"pkg/core/validator/control.go",
	} {
		source := readText(t, filepath.Join(root, filepath.FromSlash(relative)))
		for _, forbidden := range []string{
			"/dns/records",
			"DNSRecordRequest",
			"DNSRecordMutationInput",
			"DNSRecordPayload",
			"DNSRecordRecord",
			"CreateDNSRecord",
			"UpdateDNSRecord",
			"DeleteDNSRecord",
			"ListDNSRecords",
		} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s must not retain legacy DNS record API or storage code; found %q", relative, forbidden)
			}
		}
	}
}

func TestLegacyDNSRecordsHaveForwardCleanupMigration(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "migrations", "core", "00009_dns_policy_post_release_constraint_repair.sql"))
	for _, required := range []string{
		"DROP TABLE IF EXISTS dns_records",
		"event_type IN ('DNS_FAILOVER', 'DNS_DELETE_OFFLINE', 'DNS_DELETE_ALL', 'DNS_RESTORE')",
		"ADD CONSTRAINT health_events_event_type_check",
		"CHECK (event_type IN ('WEBHOOK', 'EMAIL'))",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("post-release DNS cleanup must live in a forward migration; missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"event_type NOT IN ('WEBHOOK', 'EMAIL')",
		"CHECK (event_type NOT IN ('DNS_FAILOVER', 'DNS_DELETE_OFFLINE', 'DNS_DELETE_ALL', 'DNS_RESTORE'))",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("post-release DNS cleanup must allow only supported health action executors; found %q", forbidden)
		}
	}
}

func TestLegacyDNSRecordCleanupStateIsMigratedBeforeDrop(t *testing.T) {
	root := repoRoot(t)
	cleanup := readText(t, filepath.Join(root, "migrations", "core", "00007_remove_legacy_dns_records.sql"))
	for _, required := range []string{
		"provider_retirements_json",
		"pending_retire_dns_credential_id",
		"pending_retire_zone",
		"pending_retire_record_name",
		"pending_retire_record_type",
		"pending_retire_values_json",
		"pending_retire_at",
		"provider_delete_pending_at",
		"records.record_type = 'CNAME'",
		"cname.record_type = 'CNAME'",
		"dns_credentials retire_credentials",
		"dns_credentials delete_credentials",
		"jsonb_build_object(",
		"'provider',",
	} {
		if !strings.Contains(cleanup, required) {
			t.Fatalf("legacy DNS cleanup migration must preserve retryable provider cleanup state before dropping dns_records; missing %q", required)
		}
	}
	dropIndex := strings.LastIndex(cleanup, "DROP TABLE IF EXISTS dns_records")
	retirementIndex := strings.Index(cleanup, "provider_retirements_json")
	if retirementIndex == -1 || dropIndex == -1 || retirementIndex > dropIndex {
		t.Fatalf("legacy DNS cleanup state must be migrated before dropping dns_records")
	}

	repair := readText(t, filepath.Join(root, "migrations", "core", "00011_dns_policy_retryable_legacy_cleanup_repair.sql"))
	for _, required := range []string{
		"ADD COLUMN IF NOT EXISTS provider_retirements_json",
		"to_regclass('dns_records')",
		"pending_retire_dns_credential_id",
		"provider_delete_pending_at",
	} {
		if !strings.Contains(repair, required) {
			t.Fatalf("post-release DNS retryable cleanup repair migration must be present; missing %q", required)
		}
	}
}

func TestPostReleaseDNSCleanupDownDoesNotDropPublishedRetirementColumn(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "migrations", "core", "00010_dns_policy_retryable_cleanup_and_health_secrets.sql"))
	downIndex := strings.Index(source, "-- +goose Down")
	if downIndex == -1 {
		t.Fatalf("00010 migration must include a down section")
	}
	down := source[downIndex:]
	if strings.Contains(down, "provider_retirements_json") {
		t.Fatalf("00010 down must not drop provider_retirements_json; the column belongs to the earlier published DNS policy cleanup migration")
	}
}

func TestDNSCleanupMigrationsAllowOnlySupportedHealthActions(t *testing.T) {
	root := repoRoot(t)
	for _, migration := range []string{
		"00007_remove_legacy_dns_records.sql",
		"00008_dns_policy_post_release_repair.sql",
		"00009_dns_policy_post_release_constraint_repair.sql",
	} {
		source := readText(t, filepath.Join(root, "migrations", "core", migration))
		for _, forbidden := range []string{
			"event_type NOT IN ('WEBHOOK', 'EMAIL')",
			"CHECK (event_type NOT IN ('DNS_FAILOVER', 'DNS_DELETE_OFFLINE', 'DNS_DELETE_ALL', 'DNS_RESTORE'))",
		} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s must allow only supported health action executors; found %q", migration, forbidden)
			}
		}
		if !strings.Contains(source, "CHECK (event_type IN ('WEBHOOK', 'EMAIL'))") {
			t.Fatalf("%s must constrain health actions to supported executors", migration)
		}
		if !strings.Contains(source, "event_type IN ('DNS_FAILOVER', 'DNS_DELETE_OFFLINE', 'DNS_DELETE_ALL', 'DNS_RESTORE')") {
			t.Fatalf("%s must only remove legacy DNS health actions", migration)
		}
	}
}

func TestDNSManagedRecordTypeCompatibilityIsSerialized(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "migrations", "core", "00006_dns_policy.sql"))
	functionIndex := strings.Index(source, "CREATE OR REPLACE FUNCTION enforce_dns_managed_record_type_compatibility()")
	if functionIndex == -1 {
		t.Fatalf("DNS managed record compatibility trigger function not found")
	}
	functionSource := source[functionIndex:]
	for _, required := range []string{
		"pg_advisory_xact_lock",
		"NEW.organization_id::text",
		"NEW.zone_id || ':' || lower(NEW.record_name)",
	} {
		if !strings.Contains(functionSource, required) {
			t.Fatalf("DNS managed record compatibility checks must serialize same-name writes; missing %q", required)
		}
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
	for _, relative := range []string{
		"pkg/core/service/health_action_dns.go",
		"pkg/core/service/control_health_dns.go",
	} {
		source := ""
		path := filepath.Join(root, filepath.FromSlash(relative))
		if _, err := os.Stat(path); err == nil {
			source = readText(t, path)
		}
		for _, forbidden := range []string{"DNS_FAILOVER", "DNS_DELETE_OFFLINE", "DNS_DELETE_ALL", "DNS_RESTORE", "dns_record_id", "failover_values"} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("health service must not retain direct DNS action behavior in %s; found %q", relative, forbidden)
			}
		}
	}
}
