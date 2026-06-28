package dataplane

import (
	"context"
	"strings"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
)

func TestNFTablesApplyRequiresKernelForwardingForRemoteTargets(t *testing.T) {
	original := readSysctlFile
	originalGOOS := currentGOOS
	currentGOOS = "linux"
	readSysctlFile = func(path string) ([]byte, error) {
		if path == "/proc/sys/net/ipv4/ip_forward" {
			return []byte("0\n"), nil
		}
		return original(path)
	}
	defer func() {
		readSysctlFile = original
		currentGOOS = originalGOOS
	}()

	rule := anyRule("rule_remote", ModeNFTables, domain.ProtocolTCP, 10000)
	rule.Upstream.Target.Host = "1.1.1.1"
	backend := NewNFTablesBackend(Options{
		InstanceID:       "test",
		CommandRunner:    &recordingCommandRunner{},
		ExternalBackends: true,
	})
	err := backend.Apply(context.Background(), agent.ConfigSnapshot{ConfigVersion: 1, Rules: []agent.RuleConfig{rule}})
	details := agent.StructuredApplyErrors(err)
	if len(details) != 1 || details[0].Code != ErrorDataplaneUnsupportedRule || !strings.Contains(err.Error(), "IPv4 forwarding is disabled") {
		t.Fatalf("expected forwarding preflight error, got err=%v details=%#v", err, details)
	}
}

func TestNFTablesRejectsLoopbackTarget(t *testing.T) {
	rule := anyRule("rule_loopback_target", ModeNFTables, domain.ProtocolTCP, 10000)
	rule.Upstream.Target.Host = "127.0.0.1"
	if nftablesSupportsRule(rule) {
		t.Fatalf("nftables must reject loopback DNAT targets without route_localnet handling")
	}
}

func TestNFTablesRejectsLoopbackListenAddress(t *testing.T) {
	rule := anyRule("rule_loopback_listen", ModeNFTables, domain.ProtocolTCP, 10000)
	rule.ListenIP = "127.0.0.1"
	rule.Upstream.Target.Host = "1.1.1.1"
	if nftablesSupportsRule(rule) {
		t.Fatalf("nftables must reject loopback listen addresses because prerouting does not handle localhost clients")
	}
}

func TestNFTablesRejectsSendIPBinding(t *testing.T) {
	rule := anyRule("rule_send_ip", ModeNFTables, domain.ProtocolTCP, 10000)
	rule.Upstream.Target.Host = "1.1.1.1"
	rule.SendIP = "127.0.0.2"
	if nftablesSupportsRule(rule) {
		t.Fatalf("nftables must reject send_ip because it would require SNAT or policy routing")
	}
}
