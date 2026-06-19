package arch

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestOSSNodesConsoleKeepsAgentControlsInDropdownAndMetricsFocused(t *testing.T) {
	root := repoRoot(t)
	source := readText(t, filepath.Join(root, "apps", "web", "src", "components", "console", "features", "nodes.tsx"))

	metricsStart := strings.Index(source, "function NodeMetricsPanel")
	if metricsStart < 0 {
		t.Fatalf("nodes console must keep a NodeMetricsPanel component")
	}
	metricsSource := source[metricsStart:]
	for _, forbidden := range []string{
		`<TableHead>{t("nodes.agent")}</TableHead>`,
		`<NodeAgentSummary node={node} />`,
	} {
		if strings.Contains(metricsSource, forbidden) {
			t.Fatalf("node metrics panel must focus on live metrics instead of duplicating agent version UI; found %q", forbidden)
		}
	}

	for _, required := range []string{
		"DropdownMenuCheckboxItem",
		`t("nodes.agentAutoUpdate")`,
		`t("nodes.upgradeAgent")`,
		`onCheckedChange={(checked) => updateAgentAutoUpdate(node, checked === true)}`,
		`onSelect={() => void requestAgentUpgrade(node)}`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("nodes console must expose agent update controls through the row dropdown; missing %q", required)
		}
	}
	for _, forbidden := range []string{
		`function NodeAgentUpdateControls`,
		`@/components/ui/switch`,
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("nodes console must not render agent update controls as a separate inline block; found %q", forbidden)
		}
	}
}
