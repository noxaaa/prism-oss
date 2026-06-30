package forward

import (
	"testing"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
)

func TestMetricsPayloadIncludesPositiveRuleTrafficDeltas(t *testing.T) {
	metrics := newMetricsCounter()
	metrics.addTargetTCPConnection("rule_1", "target_1", 1)
	metrics.addTargetUpload("rule_1", "target_1", 100)
	metrics.addTargetDownload("rule_1", "target_1", 50)
	metrics.addTargetUDP("rule_1", "target_1", 2)

	payload := metrics.agentPayload(time.Now().Add(time.Second))
	if len(payload.TrafficDeltas) != 1 {
		t.Fatalf("expected one traffic delta, got %#v", payload.TrafficDeltas)
	}
	delta := payload.TrafficDeltas[0]
	if delta.RuleID != "rule_1" || delta.UploadBytes != 100 || delta.DownloadBytes != 50 || delta.TCPConnections != 1 || delta.UDPPackets != 2 {
		t.Fatalf("unexpected traffic delta %#v", delta)
	}

	metrics.addTargetTCPConnection("rule_1", "target_1", -1)
	payload = metrics.agentPayload(time.Now().Add(2 * time.Second))
	if len(payload.TrafficDeltas) != 0 {
		t.Fatalf("closing a TCP connection must not emit negative traffic delta, got %#v", payload.TrafficDeltas)
	}
}

func TestMetricsPayloadPreservesDeltaWhenTargetIsPrunedBeforeNextTick(t *testing.T) {
	metrics := newMetricsCounter()
	metrics.addTargetUpload("rule_1", "target_1", 100)
	metrics.addTargetDownload("rule_1", "target_1", 50)
	_ = metrics.agentPayload(time.Now().Add(time.Second))

	metrics.addTargetUpload("rule_1", "target_1", 25)
	metrics.addTargetDownload("rule_1", "target_1", 10)
	metrics.addTargetUDP("rule_1", "target_1", 3)
	metrics.setActiveTargets(nil)

	payload := metrics.agentPayload(time.Now().Add(2 * time.Second))
	if len(payload.Targets) != 0 {
		t.Fatalf("expected pruned target to be absent from live metrics, got %#v", payload.Targets)
	}
	if len(payload.TrafficDeltas) != 1 {
		t.Fatalf("expected final traffic delta for pruned target, got %#v", payload.TrafficDeltas)
	}
	delta := payload.TrafficDeltas[0]
	if delta.RuleID != "rule_1" || delta.UploadBytes != 25 || delta.DownloadBytes != 10 || delta.UDPPackets != 3 {
		t.Fatalf("unexpected final traffic delta %#v", delta)
	}

	payload = metrics.agentPayload(time.Now().Add(3 * time.Second))
	if len(payload.TrafficDeltas) != 0 {
		t.Fatalf("final pruned delta must only be emitted once, got %#v", payload.TrafficDeltas)
	}
}

func TestMetricsPayloadKeepsCountingInactiveTargetWithOpenConnection(t *testing.T) {
	metrics := newMetricsCounter()
	metrics.addTargetTCPConnection("rule_1", "target_1", 1)
	metrics.addTargetUpload("rule_1", "target_1", 100)
	_ = metrics.agentPayload(time.Now().Add(time.Second))

	metrics.setActiveTargets(nil)
	metrics.addTargetUpload("rule_1", "target_1", 25)
	metrics.addTargetDownload("rule_1", "target_1", 10)

	payload := metrics.agentPayload(time.Now().Add(2 * time.Second))
	if len(payload.TrafficDeltas) != 1 {
		t.Fatalf("expected inactive open connection delta, got %#v", payload.TrafficDeltas)
	}
	delta := payload.TrafficDeltas[0]
	if delta.RuleID != "rule_1" || delta.UploadBytes != 25 || delta.DownloadBytes != 10 {
		t.Fatalf("unexpected inactive open connection delta %#v", delta)
	}

	metrics.addTargetUpload("rule_1", "target_1", 5)
	metrics.addTargetTCPConnection("rule_1", "target_1", -1)
	payload = metrics.agentPayload(time.Now().Add(3 * time.Second))
	if len(payload.TrafficDeltas) != 1 {
		t.Fatalf("expected final inactive connection delta on close, got %#v", payload.TrafficDeltas)
	}
	delta = payload.TrafficDeltas[0]
	if delta.RuleID != "rule_1" || delta.UploadBytes != 5 {
		t.Fatalf("unexpected final inactive connection delta %#v", delta)
	}
}

func TestMetricsPayloadKeepsCountingInactiveTargetWithOpenUDPSession(t *testing.T) {
	metrics := newMetricsCounter()
	metrics.addTargetUDPSession("rule_1", "target_1", 1)
	metrics.addTargetUDP("rule_1", "target_1", 1)
	metrics.addTargetUpload("rule_1", "target_1", 100)
	_ = metrics.agentPayload(time.Now().Add(time.Second))

	metrics.setActiveTargets(nil)
	metrics.addTargetDownload("rule_1", "target_1", 25)

	payload := metrics.agentPayload(time.Now().Add(2 * time.Second))
	if len(payload.TrafficDeltas) != 1 {
		t.Fatalf("expected inactive UDP session delta, got %#v", payload.TrafficDeltas)
	}
	delta := payload.TrafficDeltas[0]
	if delta.RuleID != "rule_1" || delta.DownloadBytes != 25 {
		t.Fatalf("unexpected inactive UDP session delta %#v", delta)
	}

	metrics.addTargetDownload("rule_1", "target_1", 5)
	metrics.addTargetUDPSession("rule_1", "target_1", -1)
	payload = metrics.agentPayload(time.Now().Add(3 * time.Second))
	if len(payload.TrafficDeltas) != 1 {
		t.Fatalf("expected final inactive UDP session delta on close, got %#v", payload.TrafficDeltas)
	}
	delta = payload.TrafficDeltas[0]
	if delta.RuleID != "rule_1" || delta.DownloadBytes != 5 {
		t.Fatalf("unexpected final inactive UDP session delta %#v", delta)
	}
}

func TestActiveTargetKeysUsesLeastLoadPositiveWeightFallbackBucket(t *testing.T) {
	activeTargets := activeTargetKeys([]agent.RuleConfig{{
		ID:       "rule_1",
		Enabled:  true,
		Protocol: domain.ProtocolTCP,
		Upstream: agent.RuleUpstreamConfig{
			Type:      "TARGET_GROUP",
			Scheduler: "LEAST_LOAD",
			TargetGroup: []agent.TargetPriorityBucket{
				{Priority: 10, Targets: []agent.TargetEndpoint{{ID: "primary_zero", Weight: 0, Enabled: true}}},
				{Priority: 20, Targets: []agent.TargetEndpoint{{ID: "backup_a", Weight: 1, Enabled: true}, {ID: "backup_b", Weight: 2, Enabled: true}}},
			},
		},
	}})

	if activeTargets[targetMetricKey{ruleID: "rule_1", targetID: "primary_zero"}] {
		t.Fatalf("least-load active target keys must skip zero-weight primary members")
	}
	for _, targetID := range []string{"backup_a", "backup_b"} {
		if !activeTargets[targetMetricKey{ruleID: "rule_1", targetID: targetID}] {
			t.Fatalf("expected positive-weight fallback target %s to be active, got %#v", targetID, activeTargets)
		}
	}
}

func TestTargetTCPDialReservationParticipatesInLeastLoadWithoutEmittingTraffic(t *testing.T) {
	metrics := newMetricsCounter()
	ruleID := "rule_1"
	targetID := "target_1"
	targets := []agent.TargetEndpoint{{ID: targetID, Weight: 1, Enabled: true}}

	if !metrics.reserveTargetTCPDial(ruleID, targetID) {
		t.Fatalf("expected reservation to succeed")
	}
	if counts := metrics.openConnectionsForTargets(ruleID, targets); counts[targetID] != 1 {
		t.Fatalf("expected reservation to count as an open connection, got %#v", counts)
	}
	if payload := metrics.agentPayload(time.Now().Add(time.Second)); len(payload.TrafficDeltas) != 0 {
		t.Fatalf("dial reservation must not emit traffic deltas, got %#v", payload.TrafficDeltas)
	}
	metrics.releaseTargetTCPDial(ruleID, targetID)
	if counts := metrics.openConnectionsForTargets(ruleID, targets); counts[targetID] != 0 {
		t.Fatalf("expected released reservation to stop counting, got %#v", counts)
	}

	if !metrics.reserveTargetTCPDial(ruleID, targetID) {
		t.Fatalf("expected second reservation to succeed")
	}
	metrics.promoteTargetTCPDial(ruleID, targetID)
	if counts := metrics.openConnectionsForTargets(ruleID, targets); counts[targetID] != 1 {
		t.Fatalf("expected promoted reservation to remain an open connection, got %#v", counts)
	}
	payload := metrics.agentPayload(time.Now().Add(2 * time.Second))
	if len(payload.TrafficDeltas) != 1 || payload.TrafficDeltas[0].TCPConnections != 1 {
		t.Fatalf("expected promoted dial to emit one TCP connection event, got %#v", payload.TrafficDeltas)
	}
	metrics.addTargetTCPConnection(ruleID, targetID, -1)
}

func TestLeastLoadTCPDialReservationSelectsAndReservesAtomically(t *testing.T) {
	metrics := newMetricsCounter()
	ruleID := "rule_1"
	targets := []agent.TargetEndpoint{
		{ID: "target_a", Weight: 1, Enabled: true},
		{ID: "target_b", Weight: 1, Enabled: true},
	}

	first, reserved, ok := metrics.reserveLeastLoadTargetTCPDial(ruleID, targets)
	if !ok || !reserved || first.ID != "target_a" {
		t.Fatalf("expected first tied reservation to choose target_a, got target=%#v reserved=%v ok=%v", first, reserved, ok)
	}
	second, reserved, ok := metrics.reserveLeastLoadTargetTCPDial(ruleID, targets)
	if !ok || !reserved || second.ID != "target_b" {
		t.Fatalf("expected second reservation to observe first reservation and choose target_b, got target=%#v reserved=%v ok=%v", second, reserved, ok)
	}
	counts := metrics.openConnectionsForTargets(ruleID, targets)
	if counts["target_a"] != 1 || counts["target_b"] != 1 {
		t.Fatalf("expected both reservations to be counted, got %#v", counts)
	}
}

func TestLeastLoadTCPDialReservationStillReturnsTargetWhenActiveTargetsAreStale(t *testing.T) {
	metrics := newMetricsCounter()
	metrics.setActiveTargets(nil)
	target, reserved, ok := metrics.reserveLeastLoadTargetTCPDial("rule_1", []agent.TargetEndpoint{{ID: "target_new", Weight: 1, Enabled: true}})
	if !ok || reserved || target.ID != "target_new" {
		t.Fatalf("expected stale active map to return an unreserved target, got target=%#v reserved=%v ok=%v", target, reserved, ok)
	}
}

func TestLeastLoadUDPSessionReservationSelectsAndReservesAtomically(t *testing.T) {
	metrics := newMetricsCounter()
	ruleID := "rule_1"
	targets := []agent.TargetEndpoint{
		{ID: "target_a", Weight: 1, Enabled: true},
		{ID: "target_b", Weight: 1, Enabled: true},
	}

	first, reserved, ok := metrics.reserveLeastLoadTargetUDPSession(ruleID, targets)
	if !ok || !reserved || first.ID != "target_a" {
		t.Fatalf("expected first tied reservation to choose target_a, got target=%#v reserved=%v ok=%v", first, reserved, ok)
	}
	second, reserved, ok := metrics.reserveLeastLoadTargetUDPSession(ruleID, targets)
	if !ok || !reserved || second.ID != "target_b" {
		t.Fatalf("expected second reservation to observe first reservation and choose target_b, got target=%#v reserved=%v ok=%v", second, reserved, ok)
	}
	counts := metrics.openConnectionsForTargets(ruleID, targets)
	if counts["target_a"] != 1 || counts["target_b"] != 1 {
		t.Fatalf("expected both reservations to be counted, got %#v", counts)
	}
	metrics.releaseTargetUDPSessionReservation(ruleID, "target_a")
	metrics.promoteTargetUDPSessionReservation(ruleID, "target_b")
	counts = metrics.openConnectionsForTargets(ruleID, targets)
	if counts["target_a"] != 0 || counts["target_b"] != 1 {
		t.Fatalf("expected release/promote to preserve only target_b session, got %#v", counts)
	}
}

func TestLeastLoadUDPSessionReservationStillReturnsTargetWhenActiveTargetsAreStale(t *testing.T) {
	metrics := newMetricsCounter()
	metrics.setActiveTargets(nil)
	target, reserved, ok := metrics.reserveLeastLoadTargetUDPSession("rule_1", []agent.TargetEndpoint{{ID: "target_new", Weight: 1, Enabled: true}})
	if !ok || reserved || target.ID != "target_new" {
		t.Fatalf("expected stale active map to return an unreserved target, got target=%#v reserved=%v ok=%v", target, reserved, ok)
	}
}
