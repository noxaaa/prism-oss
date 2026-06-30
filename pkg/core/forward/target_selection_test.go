package forward

import (
	"net"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
)

func TestLeastLoadTargetGroupUsesWeightedActiveConnections(t *testing.T) {
	supervisor := NewSupervisor()
	defer supervisor.Close()
	rule := agent.RuleConfig{
		ID:       "rule_weighted",
		Protocol: domain.ProtocolTCP,
		Upstream: agent.RuleUpstreamConfig{
			Type:      "TARGET_GROUP",
			Scheduler: "LEAST_LOAD",
			TargetGroup: []agent.TargetPriorityBucket{
				{
					Priority: 10,
					Targets: []agent.TargetEndpoint{
						{ID: "target_a", Host: "127.0.0.1", Port: 10001, Weight: 1, Enabled: true},
						{ID: "target_b", Host: "127.0.0.1", Port: 10002, Weight: 2, Enabled: true},
						{ID: "target_zero", Host: "127.0.0.1", Port: 10003, Weight: 0, Enabled: true},
					},
				},
				{
					Priority: 20,
					Targets: []agent.TargetEndpoint{
						{ID: "backup", Host: "127.0.0.1", Port: 10004, Weight: 100, Enabled: true},
					},
				},
			},
		},
	}

	selected, ok := supervisor.selectTarget(rule, &net.TCPAddr{IP: net.IPv4(203, 0, 113, 10), Port: 12345})
	if !ok || selected.ID != "target_a" {
		t.Fatalf("expected stable first candidate on equal score, got %#v ok=%v", selected, ok)
	}

	supervisor.metrics.addTargetTCPConnection(rule.ID, "target_a", 1)
	selected, ok = supervisor.selectTarget(rule, &net.TCPAddr{IP: net.IPv4(203, 0, 113, 10), Port: 12346})
	if !ok || selected.ID != "target_b" {
		t.Fatalf("expected weighted least active target_b, got %#v ok=%v", selected, ok)
	}

	supervisor.metrics.addTargetTCPConnection(rule.ID, "target_b", 2)
	selected, ok = supervisor.selectTarget(rule, &net.TCPAddr{IP: net.IPv4(203, 0, 113, 10), Port: 12347})
	if !ok || selected.ID != "target_a" {
		t.Fatalf("expected tie to keep earlier candidate target_a, got %#v ok=%v", selected, ok)
	}
}

func TestLeastLoadTargetGroupFallsBackToBackupOnlyWhenPrimaryUnavailable(t *testing.T) {
	supervisor := NewSupervisor()
	defer supervisor.Close()
	rule := agent.RuleConfig{
		ID: "rule_backup",
		Upstream: agent.RuleUpstreamConfig{
			Type:      "TARGET_GROUP",
			Scheduler: "LEAST_LOAD",
			TargetGroup: []agent.TargetPriorityBucket{
				{Priority: 10, Targets: []agent.TargetEndpoint{{ID: "primary", Weight: 0, Enabled: true}}},
				{Priority: 20, Targets: []agent.TargetEndpoint{{ID: "backup", Weight: 1, Enabled: true}}},
			},
		},
	}

	selected, ok := supervisor.selectTarget(rule, nil)
	if !ok || selected.ID != "backup" {
		t.Fatalf("expected backup bucket when primary bucket has no participating targets, got %#v ok=%v", selected, ok)
	}
}

func TestTargetStillConfiguredForRuleUsesCurrentTargetGroupCandidates(t *testing.T) {
	rule := agent.RuleConfig{
		ID: "rule_udp",
		Upstream: agent.RuleUpstreamConfig{
			Type:      "TARGET_GROUP",
			Scheduler: "LEAST_LOAD",
			TargetGroup: []agent.TargetPriorityBucket{
				{Priority: 10, Targets: []agent.TargetEndpoint{{ID: "old", Host: "127.0.0.1", Port: 10000, Weight: 0, Enabled: true}}},
				{Priority: 20, Targets: []agent.TargetEndpoint{{ID: "new", Host: "127.0.0.2", Port: 10000, Weight: 1, Enabled: true}}},
			},
		},
	}
	if targetStillConfiguredForRule(rule, "old", "127.0.0.1:10000") {
		t.Fatalf("expected drained least-load target to force UDP session reopen")
	}
	if !targetStillConfiguredForRule(rule, "new", "127.0.0.2:10000") {
		t.Fatalf("expected active fallback least-load target to remain configured")
	}
}
