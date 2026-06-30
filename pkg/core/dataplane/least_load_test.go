package dataplane

import (
	"strings"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
)

func TestRenderHAProxyConfigUsesLeastConnAndWeightsForLeastLoadGroups(t *testing.T) {
	rule := anyRule("rule_least", ModeHAProxy, domain.ProtocolTCP, 10000)
	rule.Upstream = agent.RuleUpstreamConfig{
		Type:      "TARGET_GROUP",
		Scheduler: "LEAST_LOAD",
		TargetGroup: []agent.TargetPriorityBucket{
			{Priority: 10, Targets: []agent.TargetEndpoint{
				{ID: "primary", Host: "1.1.1.1", Port: 443, Weight: 5, Enabled: true},
				{ID: "zero", Host: "1.1.1.2", Port: 443, Weight: 0, Enabled: true},
			}},
			{Priority: 20, Targets: []agent.TargetEndpoint{
				{ID: "backup", Host: "1.1.1.3", Port: 443, Weight: 1, Enabled: true},
			}},
		},
	}
	config, err := renderHAProxyConfig(agent.ConfigSnapshot{ConfigVersion: 1, Rules: []agent.RuleConfig{rule}}, haproxyPaths{pidFile: "/run/prism/haproxy.pid", socketFile: "/run/prism/haproxy.sock"})
	if err != nil {
		t.Fatalf("render haproxy config: %v", err)
	}
	for _, fragment := range []string{"balance leastconn", "primary_0 1.1.1.1:443 weight 5 check", "backup_2 1.1.1.3:443 backup weight 1 check"} {
		if !strings.Contains(config, fragment) {
			t.Fatalf("expected config to contain %q:\n%s", fragment, config)
		}
	}
	if strings.Contains(config, "zero_") {
		t.Fatalf("least-load HAProxy config must skip weight 0 members:\n%s", config)
	}
}

func TestNFTablesRejectsLeastLoadTargetGroups(t *testing.T) {
	rule := anyRule("rule_least_nft", ModeNFTables, domain.ProtocolTCP, 10000)
	rule.Upstream = agent.RuleUpstreamConfig{
		Type:      "TARGET_GROUP",
		Scheduler: "LEAST_LOAD",
		TargetGroup: []agent.TargetPriorityBucket{{
			Priority: 10,
			Targets:  []agent.TargetEndpoint{{ID: "target", Host: "1.1.1.1", Port: 443, Weight: 1, Enabled: true}},
		}},
	}
	manager := NewManager(Options{Mode: ModeNFTables})
	_, err := manager.plan(agent.ConfigSnapshot{Rules: []agent.RuleConfig{rule}})
	details := agent.StructuredApplyErrors(err)
	if len(details) != 1 || details[0].Code != ErrorDataplaneUnsupportedRule || details[0].Dataplane != ModeNFTables {
		t.Fatalf("expected nftables unsupported detail, got err=%v details=%#v", err, details)
	}
}
