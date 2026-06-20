package forward

import (
	"testing"
	"time"
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
