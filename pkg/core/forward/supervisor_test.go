package forward

import (
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
)

func TestSupervisorAppliesSnapshotAndForwardsTCP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	targetAddr, closeTarget := startTCPEchoServer(t)
	defer closeTarget()
	_, targetPortText, err := net.SplitHostPort(targetAddr)
	if err != nil {
		t.Fatalf("split target address: %v", err)
	}
	targetPort, err := strconv.Atoi(targetPortText)
	if err != nil {
		t.Fatalf("parse target port: %v", err)
	}
	listenPort := reserveLocalTCPPort(t)

	supervisor := NewSupervisor()
	defer supervisor.Close()
	if err := supervisor.Apply(ctx, agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			{
				ID:        "rule_tcp",
				Enabled:   true,
				Protocol:  domain.ProtocolTCP,
				ListenIP:  "127.0.0.1",
				Port:      listenPort,
				MatchType: "ANY_INBOUND",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target_tcp", Host: "127.0.0.1", Port: targetPort, Enabled: true},
				},
			},
		},
	}); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)), time.Second)
	if err != nil {
		t.Fatalf("dial supervisor listener: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("write through supervisor: %v", err)
	}
	buffer := make([]byte, 5)
	if _, err := io.ReadFull(conn, buffer); err != nil {
		t.Fatalf("read echo through supervisor: %v", err)
	}
	if string(buffer) != "hello" {
		t.Fatalf("expected echo, got %q", buffer)
	}
	waitForSupervisorMetrics(t, supervisor, func(metrics Metrics) bool {
		return metrics.TCPConnections > 0 && metrics.UploadBytes > 0 && metrics.DownloadBytes > 0
	})
}

func TestSupervisorAgentMetricsIncludesPerTargetDiagnostics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	targetAddr, closeTarget := startTCPEchoServer(t)
	defer closeTarget()
	_, targetPortText, err := net.SplitHostPort(targetAddr)
	if err != nil {
		t.Fatalf("split target address: %v", err)
	}
	targetPort, err := strconv.Atoi(targetPortText)
	if err != nil {
		t.Fatalf("parse target port: %v", err)
	}
	listenPort := reserveLocalTCPPort(t)

	supervisor := NewSupervisor()
	defer supervisor.Close()
	if err := supervisor.Apply(ctx, agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			{
				ID:        "rule_tcp",
				Enabled:   true,
				Protocol:  domain.ProtocolTCP,
				ListenIP:  "127.0.0.1",
				Port:      listenPort,
				MatchType: "ANY_INBOUND",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target_tcp", Host: "127.0.0.1", Port: targetPort, Enabled: true},
				},
			},
		},
	}); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	assertTCPEchoThroughPort(t, listenPort, "diagnostic")

	var payload agent.MetricsPayload
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		payload = supervisor.AgentMetrics()
		if len(payload.Targets) == 1 && payload.Targets[0].TCPConnections == 0 && payload.Targets[0].UploadBytes > 0 && payload.Targets[0].DownloadBytes > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(payload.Targets) != 1 {
		t.Fatalf("expected one target metric, got %#v", payload.Targets)
	}
	targetMetric := payload.Targets[0]
	if targetMetric.RuleID != "rule_tcp" || targetMetric.TargetID != "target_tcp" {
		t.Fatalf("expected metric for rule_tcp/target_tcp, got %#v", targetMetric)
	}
	if targetMetric.TCPConnections != 0 || targetMetric.UploadBytes == 0 || targetMetric.DownloadBytes == 0 || targetMetric.LatencyMS <= 0 {
		t.Fatalf("expected traffic and latency in target metric, got %#v", targetMetric)
	}
}

func TestSupervisorAgentMetricsPrunesTargetsOutsideAppliedSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listenPort := reserveLocalTCPPort(t)
	supervisor := NewSupervisor()
	defer supervisor.Close()
	supervisor.metrics.addTargetUpload("rule_tcp", "target_tcp", 100)
	supervisor.metrics.addTargetDownload("rule_removed", "target_removed", 200)

	if err := supervisor.Apply(ctx, agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			{
				ID:        "rule_tcp",
				Enabled:   true,
				Protocol:  domain.ProtocolTCP,
				ListenIP:  "127.0.0.1",
				Port:      listenPort,
				MatchType: "ANY_INBOUND",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target_tcp", Host: "127.0.0.1", Port: 9, Enabled: true},
				},
			},
		},
	}); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}

	payload := supervisor.AgentMetrics()
	if len(payload.Targets) != 1 || payload.Targets[0].RuleID != "rule_tcp" || payload.Targets[0].TargetID != "target_tcp" {
		t.Fatalf("expected only active target metrics after apply, got %#v", payload.Targets)
	}

	if err := supervisor.Apply(ctx, agent.ConfigSnapshot{NodeID: "node_1", ConfigVersion: 2}); err != nil {
		t.Fatalf("apply empty snapshot: %v", err)
	}
	payload = supervisor.AgentMetrics()
	if len(payload.Targets) != 0 {
		t.Fatalf("expected target metrics to be pruned after rule removal, got %#v", payload.Targets)
	}
}

func TestMetricsCounterDoesNotReportNegativeTargetTCPAfterInactivePrune(t *testing.T) {
	activeRules := []agent.RuleConfig{
		{
			ID:      "rule_tcp",
			Enabled: true,
			Upstream: agent.RuleUpstreamConfig{
				Type:   "TARGET",
				Target: &agent.TargetEndpoint{ID: "target_tcp", Enabled: true},
			},
		},
	}
	metrics := newMetricsCounter()
	now := time.Now()
	metrics.setActiveTargets(activeRules)
	metrics.addTargetTCPConnection("rule_tcp", "target_tcp", 1)
	metrics.setActiveTargets(nil)
	payload := metrics.agentPayload(now.Add(time.Second))
	if len(payload.Targets) != 1 || payload.Targets[0].TCPConnections != 1 {
		t.Fatalf("expected inactive target with an open connection to stay reportable, got %#v", payload.Targets)
	}

	metrics.addTargetTCPConnection("rule_tcp", "target_tcp", -1)
	metrics.setActiveTargets(activeRules)
	payload = metrics.agentPayload(now.Add(2 * time.Second))
	for _, target := range payload.Targets {
		if target.TCPConnections < 0 {
			t.Fatalf("expected target tcp connections not to go negative after inactive decrement, got %#v", payload.Targets)
		}
	}
}

func TestActiveTargetKeysUsesRuntimeTargetGroupPriorityBucket(t *testing.T) {
	activeTargets := activeTargetKeys([]agent.RuleConfig{
		{
			ID:      "rule_group",
			Enabled: true,
			Upstream: agent.RuleUpstreamConfig{
				Type: "TARGET_GROUP",
				TargetGroup: []agent.TargetPriorityBucket{
					{
						Priority: 20,
						Targets:  []agent.TargetEndpoint{{ID: "backup", Enabled: true}},
					},
					{
						Priority: 10,
						Targets: []agent.TargetEndpoint{
							{ID: "primary-a", Enabled: true},
							{ID: "primary-b", Enabled: true},
						},
					},
					{
						Priority: 5,
						Targets:  []agent.TargetEndpoint{{ID: "disabled-primary", Enabled: false}},
					},
				},
			},
		},
	})

	if !activeTargets[targetMetricKey{ruleID: "rule_group", targetID: "primary-a"}] || !activeTargets[targetMetricKey{ruleID: "rule_group", targetID: "primary-b"}] {
		t.Fatalf("expected active target set to include enabled targets in the runtime priority bucket, got %#v", activeTargets)
	}
	if activeTargets[targetMetricKey{ruleID: "rule_group", targetID: "backup"}] || activeTargets[targetMetricKey{ruleID: "rule_group", targetID: "disabled-primary"}] {
		t.Fatalf("expected inactive target group buckets to be pruned, got %#v", activeTargets)
	}
}

func TestSupervisorRejectsUnsupportedForwardingType(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listenPort := reserveLocalTCPPort(t)
	supervisor := NewSupervisor()
	defer supervisor.Close()
	err := supervisor.Apply(ctx, agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			{
				ID:             "rule_tunnel",
				Enabled:        true,
				ForwardingType: domain.ForwardingTypeTunnel,
				Protocol:       domain.ProtocolTCP,
				ListenIP:       "127.0.0.1",
				Port:           listenPort,
				MatchType:      "ANY_INBOUND",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target_tcp", Host: "127.0.0.1", Port: 9, Enabled: true},
				},
			},
		},
	})
	if err == nil {
		t.Fatalf("expected unsupported forwarding type to be rejected")
	}
}

func TestSupervisorReusesListenerWhenSnapshotChangesSamePort(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	targetA, closeTargetA := startTCPPrefixServer(t, "a:")
	defer closeTargetA()
	targetB, closeTargetB := startTCPPrefixServer(t, "b:")
	defer closeTargetB()
	listenPort := reserveLocalTCPPort(t)

	supervisor := NewSupervisor()
	defer supervisor.Close()
	if err := supervisor.Apply(ctx, agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			{
				ID:        "rule_tcp",
				Enabled:   true,
				Protocol:  domain.ProtocolTCP,
				ListenIP:  "127.0.0.1",
				Port:      listenPort,
				MatchType: "ANY_INBOUND",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target_a", Host: "127.0.0.1", Port: mustPort(t, targetA), Enabled: true},
				},
			},
		},
	}); err != nil {
		t.Fatalf("apply initial snapshot: %v", err)
	}
	if response := readTCPResponse(t, listenPort, "hello"); !strings.HasPrefix(response, "a:") {
		t.Fatalf("expected initial target a, got %q", response)
	}

	if err := supervisor.Apply(ctx, agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: 2,
		Rules: []agent.RuleConfig{
			{
				ID:        "rule_tcp",
				Enabled:   true,
				Protocol:  domain.ProtocolTCP,
				ListenIP:  "127.0.0.1",
				Port:      listenPort,
				MatchType: "ANY_INBOUND",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target_b", Host: "127.0.0.1", Port: mustPort(t, targetB), Enabled: true},
				},
			},
		},
	}); err != nil {
		t.Fatalf("apply updated snapshot on same listener: %v", err)
	}
	if response := readTCPResponse(t, listenPort, "hello"); !strings.HasPrefix(response, "b:") {
		t.Fatalf("expected updated target b, got %q", response)
	}
}

func TestSupervisorStopsWildcardListenerBeforeBindingSpecificAddress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	targetA, closeTargetA := startTCPPrefixServer(t, "a:")
	defer closeTargetA()
	targetB, closeTargetB := startTCPPrefixServer(t, "b:")
	defer closeTargetB()
	listenPort := reserveLocalTCPPort(t)

	supervisor := NewSupervisor()
	defer supervisor.Close()
	if err := supervisor.Apply(ctx, agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			{
				ID:        "rule_wildcard",
				Enabled:   true,
				Protocol:  domain.ProtocolTCP,
				ListenIP:  "0.0.0.0",
				Port:      listenPort,
				MatchType: "ANY_INBOUND",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target_a", Host: "127.0.0.1", Port: mustPort(t, targetA), Enabled: true},
				},
			},
		},
	}); err != nil {
		t.Fatalf("apply wildcard snapshot: %v", err)
	}
	if response := readTCPResponse(t, listenPort, "hello"); !strings.HasPrefix(response, "a:") {
		t.Fatalf("expected wildcard target a, got %q", response)
	}

	if err := supervisor.Apply(ctx, agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: 2,
		Rules: []agent.RuleConfig{
			{
				ID:        "rule_specific",
				Enabled:   true,
				Protocol:  domain.ProtocolTCP,
				ListenIP:  "127.0.0.1",
				Port:      listenPort,
				MatchType: "ANY_INBOUND",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target_b", Host: "127.0.0.1", Port: mustPort(t, targetB), Enabled: true},
				},
			},
		},
	}); err != nil {
		t.Fatalf("apply specific listener after wildcard: %v", err)
	}
	if response := readTCPResponse(t, listenPort, "hello"); !strings.HasPrefix(response, "b:") {
		t.Fatalf("expected specific target b, got %q", response)
	}
}

func TestSupervisorRejectsOverlappingListenersInsideSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	target, closeTarget := startTCPPrefixServer(t, "target:")
	defer closeTarget()
	listenPort := reserveLocalTCPPort(t)
	supervisor := NewSupervisor()
	defer supervisor.Close()

	err := supervisor.Apply(ctx, agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			{
				ID:        "rule_wildcard",
				Enabled:   true,
				Protocol:  domain.ProtocolTCP,
				ListenIP:  "0.0.0.0",
				Port:      listenPort,
				MatchType: "ANY_INBOUND",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target", Host: "127.0.0.1", Port: mustPort(t, target), Enabled: true},
				},
			},
			{
				ID:        "rule_specific",
				Enabled:   true,
				Protocol:  domain.ProtocolTCP,
				ListenIP:  "127.0.0.1",
				Port:      listenPort,
				MatchType: "ANY_INBOUND",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target", Host: "127.0.0.1", Port: mustPort(t, target), Enabled: true},
				},
			},
		},
	})
	if err == nil {
		t.Fatalf("expected overlapping listener snapshot to be rejected")
	}
	if !strings.Contains(err.Error(), "overlapping listeners") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSupervisorAppliesSnapshotAndForwardsUDP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	targetAddr, closeTarget := startUDPEchoServer(t)
	defer closeTarget()
	_, targetPortText, err := net.SplitHostPort(targetAddr)
	if err != nil {
		t.Fatalf("split target address: %v", err)
	}
	targetPort, err := strconv.Atoi(targetPortText)
	if err != nil {
		t.Fatalf("parse target port: %v", err)
	}
	listenPort := reserveLocalUDPPort(t)

	supervisor := NewSupervisor()
	defer supervisor.Close()
	if err := supervisor.Apply(ctx, agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			{
				ID:        "rule_udp",
				Enabled:   true,
				Protocol:  domain.ProtocolUDP,
				ListenIP:  "127.0.0.1",
				Port:      listenPort,
				MatchType: "ANY_INBOUND",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target_udp", Host: "127.0.0.1", Port: targetPort, Enabled: true},
				},
			},
		},
	}); err != nil {
		t.Fatalf("apply udp snapshot: %v", err)
	}

	conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)))
	if err != nil {
		t.Fatalf("dial udp listener: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("write datagram: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set udp read deadline: %v", err)
	}
	buffer := make([]byte, 5)
	if _, err := io.ReadFull(conn, buffer); err != nil {
		t.Fatalf("read udp echo: %v", err)
	}
	if string(buffer) != "hello" {
		t.Fatalf("expected udp echo, got %q", buffer)
	}
	waitForSupervisorMetrics(t, supervisor, func(metrics Metrics) bool {
		return metrics.UDPPackets > 0 && metrics.UploadBytes > 0 && metrics.DownloadBytes > 0
	})
	var payload agent.MetricsPayload
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		payload = supervisor.AgentMetrics()
		if len(payload.Targets) > 0 && payload.Targets[0].TargetID == "target_udp" && payload.Targets[0].UploadBytes > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(payload.Targets) != 1 || payload.Targets[0].TargetID != "target_udp" {
		t.Fatalf("expected udp target metrics, got %#v", payload.Targets)
	}
	if payload.Targets[0].LatencyMS != 0 {
		t.Fatalf("expected udp target latency to stay unset without a probe, got %#v", payload.Targets[0])
	}
}

func TestSupervisorAppliesTCPUDPRuleAndForwardsBothProtocols(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	targetPort, closeTarget := startTCPUDPEchoServer(t)
	defer closeTarget()
	listenPort := reserveLocalTCPPort(t)

	supervisor := NewSupervisor()
	defer supervisor.Close()
	if err := supervisor.Apply(ctx, agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			{
				ID:        "rule_tcp_udp",
				Enabled:   true,
				Protocol:  domain.Protocol("TCP_UDP"),
				ListenIP:  "127.0.0.1",
				Port:      listenPort,
				MatchType: "ANY_INBOUND",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target_tcp_udp", Host: "127.0.0.1", Port: targetPort, Enabled: true},
				},
			},
		},
	}); err != nil {
		t.Fatalf("apply tcp+udp snapshot: %v", err)
	}

	if response := readTCPResponse(t, listenPort, "hello"); response != "hello" {
		t.Fatalf("expected tcp echo, got %q", response)
	}

	conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)))
	if err != nil {
		t.Fatalf("dial udp listener: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte("world")); err != nil {
		t.Fatalf("write udp datagram: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set udp read deadline: %v", err)
	}
	buffer := make([]byte, 5)
	if _, err := io.ReadFull(conn, buffer); err != nil {
		t.Fatalf("read udp echo: %v", err)
	}
	if string(buffer) != "world" {
		t.Fatalf("expected udp echo, got %q", buffer)
	}
}

func TestSupervisorPreservesUDPClientMappingAcrossPackets(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	targetAddr, sourceAddresses, closeTarget := startUDPSourceRecorder(t)
	defer closeTarget()
	listenPort := reserveLocalUDPPort(t)

	supervisor := NewSupervisor()
	defer supervisor.Close()
	if err := supervisor.Apply(ctx, agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			{
				ID:        "rule_udp",
				Enabled:   true,
				Protocol:  domain.ProtocolUDP,
				ListenIP:  "127.0.0.1",
				Port:      listenPort,
				MatchType: "ANY_INBOUND",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target_udp", Host: "127.0.0.1", Port: mustPort(t, targetAddr), Enabled: true},
				},
			},
		},
	}); err != nil {
		t.Fatalf("apply udp snapshot: %v", err)
	}

	conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)))
	if err != nil {
		t.Fatalf("dial udp listener: %v", err)
	}
	defer func() { _ = conn.Close() }()
	for _, payload := range []string{"one", "two"} {
		if _, err := conn.Write([]byte(payload)); err != nil {
			t.Fatalf("write udp payload %q: %v", payload, err)
		}
		if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatalf("set udp read deadline: %v", err)
		}
		buffer := make([]byte, len(payload))
		if _, err := io.ReadFull(conn, buffer); err != nil {
			t.Fatalf("read udp echo %q: %v", payload, err)
		}
	}

	first := readUDPSourceAddress(t, sourceAddresses)
	second := readUDPSourceAddress(t, sourceAddresses)
	if first != second {
		t.Fatalf("expected UDP target to see stable source address, got first=%s second=%s", first, second)
	}
}

func TestSupervisorRefreshesUDPClientSessionWhenTargetIDChanges(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	targetAddr, closeTarget := startUDPEchoServer(t)
	defer closeTarget()
	targetPort := mustPort(t, targetAddr)
	listenPort := reserveLocalUDPPort(t)

	supervisor := NewSupervisor()
	defer supervisor.Close()
	if err := supervisor.Apply(ctx, udpTargetSnapshot(listenPort, targetPort, "target_udp_a", 1)); err != nil {
		t.Fatalf("apply initial udp snapshot: %v", err)
	}

	conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)))
	if err != nil {
		t.Fatalf("dial udp listener: %v", err)
	}
	defer func() { _ = conn.Close() }()
	writeAndReadUDPEcho(t, conn, "one")
	waitForTargetMetrics(t, supervisor, "target_udp_a", func(metrics agent.TargetMetricsPayload) bool {
		return metrics.UploadBytes > 0 && metrics.DownloadBytes > 0
	})

	if err := supervisor.Apply(ctx, udpTargetSnapshot(listenPort, targetPort, "target_udp_b", 2)); err != nil {
		t.Fatalf("apply updated udp snapshot: %v", err)
	}
	writeAndReadUDPEcho(t, conn, "two")
	waitForTargetMetrics(t, supervisor, "target_udp_b", func(metrics agent.TargetMetricsPayload) bool {
		return metrics.UploadBytes > 0 && metrics.DownloadBytes > 0
	})
}

func udpTargetSnapshot(listenPort int, targetPort int, targetID string, version int) agent.ConfigSnapshot {
	return agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: version,
		Rules: []agent.RuleConfig{
			{
				ID:        "rule_udp",
				Enabled:   true,
				Protocol:  domain.ProtocolUDP,
				ListenIP:  "127.0.0.1",
				Port:      listenPort,
				MatchType: "ANY_INBOUND",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: targetID, Host: "127.0.0.1", Port: targetPort, Enabled: true},
				},
			},
		},
	}
}

func writeAndReadUDPEcho(t *testing.T, conn net.Conn, payload string) {
	t.Helper()
	if _, err := conn.Write([]byte(payload)); err != nil {
		t.Fatalf("write udp payload %q: %v", payload, err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set udp read deadline: %v", err)
	}
	buffer := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, buffer); err != nil {
		t.Fatalf("read udp echo %q: %v", payload, err)
	}
	if string(buffer) != payload {
		t.Fatalf("expected udp echo %q, got %q", payload, buffer)
	}
}

func waitForTargetMetrics(t *testing.T, supervisor *Supervisor, targetID string, predicate func(agent.TargetMetricsPayload) bool) agent.TargetMetricsPayload {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	var lastPayload agent.MetricsPayload
	for time.Now().Before(deadline) {
		lastPayload = supervisor.AgentMetrics()
		for _, metrics := range lastPayload.Targets {
			if metrics.TargetID == targetID && predicate(metrics) {
				return metrics
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for target metrics for %s, last payload %#v", targetID, lastPayload)
	return agent.TargetMetricsPayload{}
}

func TestSupervisorTargetGroupUsesHighestPriorityBucketAndStableIPHash(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	primaryA, closePrimaryA := startTCPPrefixServer(t, "primary-a:")
	defer closePrimaryA()
	primaryB, closePrimaryB := startTCPPrefixServer(t, "primary-b:")
	defer closePrimaryB()
	backup, closeBackup := startTCPPrefixServer(t, "backup:")
	defer closeBackup()
	primaryAPort := mustPort(t, primaryA)
	primaryBPort := mustPort(t, primaryB)
	backupPort := mustPort(t, backup)
	listenPort := reserveLocalTCPPort(t)

	supervisor := NewSupervisor()
	defer supervisor.Close()
	if err := supervisor.Apply(ctx, agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			{
				ID:        "rule_group",
				Enabled:   true,
				Protocol:  domain.ProtocolTCP,
				ListenIP:  "127.0.0.1",
				Port:      listenPort,
				MatchType: "ANY_INBOUND",
				Upstream: agent.RuleUpstreamConfig{
					Type: "TARGET_GROUP",
					TargetGroup: []agent.TargetPriorityBucket{
						{
							Priority: 20,
							Targets:  []agent.TargetEndpoint{{ID: "backup", Host: "127.0.0.1", Port: backupPort, Enabled: true}},
						},
						{
							Priority: 10,
							Targets: []agent.TargetEndpoint{
								{ID: "primary-a", Host: "127.0.0.1", Port: primaryAPort, Enabled: true},
								{ID: "primary-b", Host: "127.0.0.1", Port: primaryBPort, Enabled: true},
							},
						},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("apply target group snapshot: %v", err)
	}

	first := readTCPResponse(t, listenPort, "hello")
	second := readTCPResponse(t, listenPort, "hello")
	if strings.HasPrefix(first, "backup:") || strings.HasPrefix(second, "backup:") {
		t.Fatalf("expected primary priority bucket, got first=%q second=%q", first, second)
	}
	if first != second {
		t.Fatalf("expected same source ip to hash to stable target, got first=%q second=%q", first, second)
	}
}
