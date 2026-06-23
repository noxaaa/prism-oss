package repo

import (
	"os"
	"strings"
	"testing"
)

func TestListLatestHealthResultsByChecksFiltersCurrentMonitorScope(t *testing.T) {
	source, err := os.ReadFile("health_dns.go")
	if err != nil {
		t.Fatalf("read health dns repo source: %v", err)
	}
	text := string(source)
	start := strings.Index(text, "func (store *PostgresStore) ListLatestHealthResultsByChecks")
	if start < 0 {
		t.Fatalf("ListLatestHealthResultsByChecks was not found")
	}
	end := strings.Index(text[start:], "func (store *PostgresStore) RecordHealthResults")
	if end < 0 {
		t.Fatalf("RecordHealthResults was not found after ListLatestHealthResultsByChecks")
	}
	section := text[start : start+end]
	for _, expected := range []string{
		"health_check_monitor_scopes",
		"monitor_group_members",
		"JOIN monitors monitor",
		"monitor.deleted_at IS NULL",
		"scope.scope_type = 'MONITOR'",
		"scope.scope_type = 'MONITOR_GROUP'",
	} {
		if !strings.Contains(section, expected) {
			t.Fatalf("ListLatestHealthResultsByChecks must filter latest results to current monitor scopes; missing %q", expected)
		}
	}
}
