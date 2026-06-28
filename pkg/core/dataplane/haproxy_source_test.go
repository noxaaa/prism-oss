package dataplane

import (
	"strings"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
)

func TestRenderHAProxyConfigBindsRuleSendIP(t *testing.T) {
	rule := anyRule("rule_source", ModeHAProxy, domain.ProtocolTCP, 10000)
	rule.SendIP = "127.0.0.2"
	config, err := renderHAProxyConfig(agent.ConfigSnapshot{
		ConfigVersion: 1,
		Rules:         []agent.RuleConfig{rule},
	}, haproxyPaths{pidFile: "/run/prism/haproxy.pid", socketFile: "/run/prism/haproxy.sock"})
	if err != nil {
		t.Fatalf("render haproxy config: %v", err)
	}
	if !strings.Contains(config, " source 127.0.0.2") {
		t.Fatalf("expected HAProxy server line to bind source IP:\n%s", config)
	}
}

func TestRenderHAProxyConfigUsesRuntimeIDForExpandedPortBackends(t *testing.T) {
	first := anyRule("rule_range", ModeHAProxy, domain.ProtocolTCP, 10000)
	first.RuntimeID = "rule_range-p10000"
	second := anyRule("rule_range", ModeHAProxy, domain.ProtocolTCP, 10001)
	second.RuntimeID = "rule_range-p10001"
	config, err := renderHAProxyConfig(agent.ConfigSnapshot{
		ConfigVersion: 1,
		Rules:         []agent.RuleConfig{first, second},
	}, haproxyPaths{pidFile: "/run/prism/haproxy.pid", socketFile: "/run/prism/haproxy.sock"})
	if err != nil {
		t.Fatalf("render haproxy config: %v", err)
	}
	if strings.Count(config, "backend be_rule_range\n") != 0 {
		t.Fatalf("expected expanded rules to avoid duplicate base backend names:\n%s", config)
	}
	if !strings.Contains(config, "backend be_rule_range_p10000\n") || !strings.Contains(config, "backend be_rule_range_p10001\n") {
		t.Fatalf("expected per-port backend names from runtime ids:\n%s", config)
	}
}
