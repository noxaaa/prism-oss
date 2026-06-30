package arch

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestOSSNodesConsoleKeepsAgentControlsInDropdownAndMetricsFocused(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "packages", "web-core", "src", "components", "console", "features", "nodes.tsx"))
	metricsSource := readText(t, filepath.Join(root, "packages", "web-core", "src", "components", "console", "features", "node-metrics-panel.tsx"))

	if !strings.Contains(metricsSource, "function NodeMetricsPanel") {
		t.Fatalf("nodes console must keep a NodeMetricsPanel component")
	}
	for _, forbidden := range []string{
		`<TableHead>{t("nodes.agent")}</TableHead>`,
		`<NodeAgentSummary node={node} />`,
	} {
		if strings.Contains(metricsSource, forbidden) {
			t.Fatalf("node metrics panel must focus on live metrics instead of duplicating agent version UI; found %q", forbidden)
		}
	}

	for _, required := range []string{
		"HoverCard",
		"Progress",
		"formatBitrateBps",
		"ramLabel(metrics,",
		"ramDetail(metrics,",
		"NodeCPUHover",
		"DropdownMenuCheckboxItem",
		`t("nodes.agentAutoUpdate")`,
		`t("nodes.upgradeAgent")`,
		`onCheckedChange={(checked) => updateAgentAutoUpdate(node, checked === true)}`,
		`onSelect={() => void requestAgentUpgrade(node)}`,
	} {
		if !strings.Contains(source, required) && !strings.Contains(metricsSource, required) {
			t.Fatalf("nodes console must expose agent update controls through the row dropdown; missing %q", required)
		}
	}
	for _, forbidden := range []string{
		`function NodeAgentUpdateControls`,
		`@/components/ui/switch`,
		`${bytes(metrics.ram_used_bytes)} / ${bytes(metrics.ram_total_bytes)}`,
	} {
		if strings.Contains(source, forbidden) || strings.Contains(metricsSource, forbidden) {
			t.Fatalf("nodes console must keep metrics and agent controls resilient; found %q", forbidden)
		}
	}
}

func TestOSSRulesConsoleLoadsTrafficAutomatically(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "packages", "web-core", "src", "components", "console", "features", "rules.tsx"))
	for _, required := range []string{
		"trafficReadableRules",
		"void loadTraffic()",
		"`/api/control/rules/${rule.id}/traffic`",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("rules console must auto-load readable rule traffic; missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"onTraffic",
		`t("rules.trafficButton")`,
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("rules console must not expose a manual per-row traffic button; found %q", forbidden)
		}
	}
}

func TestOSSUsageConsoleGatesAutoTrafficByPermission(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "packages", "web-core", "src", "components", "console", "features", "usage.tsx"))
	for _, required := range []string{
		"canReadTraffic && (canReadAllTraffic || rule.owner_user_id === session?.user.id)",
		"if (!canReadTraffic || trafficReadableRules.length === 0)",
		"if (canReadTraffic && trafficReadableRules.length > 0)",
		"[canReadTraffic, trafficReadableRules]",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("usage console must not auto-load traffic without traffic permissions; missing %q", required)
		}
	}
}

func TestOSSAgentTrafficReportsAreGatedByCurrentConnection(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "pkg", "core", "handler", "agent_ws.go"))
	metricsIndex := strings.Index(source, `case "metrics":`)
	if metricsIndex < 0 {
		t.Fatalf("agent websocket handler must handle metrics messages")
	}
	metricsSource := source[metricsIndex:]
	updateIndex := strings.Index(metricsSource, "UpdateMetricsForConnection")
	recordIndex := strings.Index(metricsSource, "RecordNodeTrafficReport")
	ackIndex := strings.Index(metricsSource, `"metrics_ack"`)
	for name, index := range map[string]int{
		"UpdateMetricsForConnection": updateIndex,
		"RecordNodeTrafficReport":    recordIndex,
		"metrics_ack":                ackIndex,
	} {
		if index < 0 {
			t.Fatalf("agent websocket metrics handler missing %s", name)
		}
	}
	if updateIndex > recordIndex {
		t.Fatalf("agent traffic reports must be persisted only after the current-connection metrics guard succeeds")
	}
	if recordIndex > ackIndex {
		t.Fatalf("agent traffic reports must be persisted before acknowledging the report")
	}
}
