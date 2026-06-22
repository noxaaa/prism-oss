package arch

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestOSSMigrationsIncludeMonitorHealthAndDNSTables(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "migrations", "core", "00001_core.sql"))
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
		"CREATE TABLE dns_credentials",
		"CREATE TABLE dns_records",
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
