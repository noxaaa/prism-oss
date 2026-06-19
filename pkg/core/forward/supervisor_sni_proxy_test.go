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

func TestSupervisorRoutesTCPByTLSClientHelloSNI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	targetA, closeTargetA := startTLSClientHelloPrefixServer(t, "app-a:")
	defer closeTargetA()
	targetB, closeTargetB := startTLSClientHelloPrefixServer(t, "app-b:")
	defer closeTargetB()
	listenPort := reserveLocalTCPPort(t)

	supervisor := NewSupervisor()
	defer supervisor.Close()
	if err := supervisor.Apply(ctx, agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			{
				ID:          "rule_app_a",
				Enabled:     true,
				Protocol:    domain.ProtocolTCP,
				ListenIP:    "127.0.0.1",
				Port:        listenPort,
				MatchType:   "TLS_SNI",
				SNIHostname: "app-a.example.com",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target_a", Host: "127.0.0.1", Port: mustPort(t, targetA), Enabled: true},
				},
			},
			{
				ID:          "rule_app_b",
				Enabled:     true,
				Protocol:    domain.ProtocolTCP,
				ListenIP:    "127.0.0.1",
				Port:        listenPort,
				MatchType:   "TLS_SNI",
				SNIHostname: "app-b.example.com",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target_b", Host: "127.0.0.1", Port: mustPort(t, targetB), Enabled: true},
				},
			},
		},
	}); err != nil {
		t.Fatalf("apply sni snapshot: %v", err)
	}

	responseA := writeClientHelloAndReadPrefix(t, listenPort, "app-a.example.com")
	if !strings.HasPrefix(responseA, "app-a:") {
		t.Fatalf("expected app-a target, got %q", responseA)
	}
	responseB := writeClientHelloAndReadPrefix(t, listenPort, "app-b.example.com")
	if !strings.HasPrefix(responseB, "app-b:") {
		t.Fatalf("expected app-b target, got %q", responseB)
	}
}

func TestSupervisorRoutesFragmentedTLSClientHelloSNI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientHello := capturedClientHello(t, "fragmented.example.com")
	fragmentedClientHello := fragmentedTLSHandshakeRecords(t, clientHello, 2)
	target, closeTarget := startExpectedBytesPrefixServer(t, fragmentedClientHello, "fragmented:")
	defer closeTarget()
	listenPort := reserveLocalTCPPort(t)

	supervisor := NewSupervisor()
	defer supervisor.Close()
	if err := supervisor.Apply(ctx, agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			{
				ID:          "rule_fragmented_sni",
				Enabled:     true,
				Protocol:    domain.ProtocolTCP,
				ListenIP:    "127.0.0.1",
				Port:        listenPort,
				MatchType:   "TLS_SNI",
				SNIHostname: "fragmented.example.com",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target_fragmented", Host: "127.0.0.1", Port: mustPort(t, target), Enabled: true},
				},
			},
		},
	}); err != nil {
		t.Fatalf("apply fragmented sni snapshot: %v", err)
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)), time.Second)
	if err != nil {
		t.Fatalf("dial fragmented sni listener: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write(fragmentedClientHello); err != nil {
		t.Fatalf("write fragmented client hello: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set fragmented sni read deadline: %v", err)
	}
	buffer := make([]byte, 64)
	n, err := conn.Read(buffer)
	if err != nil {
		t.Fatalf("read fragmented sni response: %v", err)
	}
	if string(buffer[:n]) != "fragmented:" {
		t.Fatalf("expected fragmented target response, got %q", string(buffer[:n]))
	}
}

func TestSupervisorConsumesProxyHeaderBeforeTLSClientHelloSNI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	target, closeTarget := startTLSClientHelloPrefixServer(t, "app:")
	defer closeTarget()
	listenPort := reserveLocalTCPPort(t)
	supervisor := NewSupervisor()
	defer supervisor.Close()
	if err := supervisor.Apply(ctx, agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			{
				ID:              "rule_app",
				Enabled:         true,
				Protocol:        domain.ProtocolTCP,
				ListenIP:        "127.0.0.1",
				Port:            listenPort,
				MatchType:       "TLS_SNI",
				SNIHostname:     "app.example.com",
				ProxyProtocolIn: "V1",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target_app", Host: "127.0.0.1", Port: mustPort(t, target), Enabled: true},
				},
			},
		},
	}); err != nil {
		t.Fatalf("apply sni proxy snapshot: %v", err)
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)), time.Second)
	if err != nil {
		t.Fatalf("dial sni proxy listener: %v", err)
	}
	defer func() { _ = conn.Close() }()
	header, err := BuildProxyHeader(ProxyInfo{
		Version:         ProxyProtocolV1,
		SourceIP:        net.ParseIP("203.0.113.10"),
		DestinationIP:   net.ParseIP("127.0.0.1"),
		SourcePort:      12345,
		DestinationPort: listenPort,
	})
	if err != nil {
		t.Fatalf("build proxy header: %v", err)
	}
	if _, err := conn.Write(append(header, capturedClientHello(t, "app.example.com")...)); err != nil {
		t.Fatalf("write proxy header and client hello: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	buffer := make([]byte, 4)
	if _, err := io.ReadFull(conn, buffer); err != nil {
		t.Fatalf("read sni proxy response: %v", err)
	}
	if string(buffer) != "app:" {
		t.Fatalf("expected sni target response prefix, got %q", buffer)
	}
}

func TestSupervisorAgentMetricsReportsIntervalRates(t *testing.T) {
	supervisor := NewSupervisor()
	supervisor.metrics.mu.Lock()
	supervisor.metrics.lastSnapshotAt = time.Unix(10, 0)
	supervisor.metrics.lastSnapshot = Metrics{}
	supervisor.metrics.mu.Unlock()
	supervisor.metrics.addUDP(10)
	supervisor.metrics.addUpload(100)
	supervisor.metrics.addDownload(50)

	payload := supervisor.metrics.agentPayload(time.Unix(12, 0))
	if payload.UDPPacketsPerSecond != 5 {
		t.Fatalf("expected 5 udp packets per second, got %d", payload.UDPPacketsPerSecond)
	}
	if payload.BandwidthBps != 600 {
		t.Fatalf("expected 600 bps from interval byte delta, got %d", payload.BandwidthBps)
	}
	if payload.UploadBytes != 100 || payload.DownloadBytes != 50 {
		t.Fatalf("expected cumulative traffic bytes to remain available, got %#v", payload)
	}
}

func TestSupervisorConsumesInboundProxyProtocolV1BeforeForwarding(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	targetAddr, closeTarget := startTCPReadFiveServer(t)
	defer closeTarget()
	listenPort := reserveLocalTCPPort(t)
	supervisor := NewSupervisor()
	defer supervisor.Close()
	if err := supervisor.Apply(ctx, singleTCPRuleSnapshot(t, listenPort, mustPort(t, targetAddr), "V1", "NONE")); err != nil {
		t.Fatalf("apply proxy in snapshot: %v", err)
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)), time.Second)
	if err != nil {
		t.Fatalf("dial proxy in listener: %v", err)
	}
	defer func() { _ = conn.Close() }()
	header, err := BuildProxyHeader(ProxyInfo{
		Version:         ProxyProtocolV1,
		SourceIP:        net.ParseIP("203.0.113.10"),
		DestinationIP:   net.ParseIP("127.0.0.1"),
		SourcePort:      12345,
		DestinationPort: listenPort,
	})
	if err != nil {
		t.Fatalf("build proxy v1 header: %v", err)
	}
	if _, err := conn.Write(append(header, []byte("hello")...)); err != nil {
		t.Fatalf("write proxy header and payload: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	buffer := make([]byte, len("got:hello"))
	if _, err := io.ReadFull(conn, buffer); err != nil {
		t.Fatalf("read proxy in response: %v", err)
	}
	if string(buffer) != "got:hello" {
		t.Fatalf("expected proxy header consumed, got %q", buffer)
	}
}

func TestSupervisorWritesOutboundProxyProtocolV1BeforeForwarding(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	targetAddr, closeTarget := startTCPExpectProxyV1Server(t)
	defer closeTarget()
	listenPort := reserveLocalTCPPort(t)
	supervisor := NewSupervisor()
	defer supervisor.Close()
	if err := supervisor.Apply(ctx, singleTCPRuleSnapshot(t, listenPort, mustPort(t, targetAddr), "NONE", "V1")); err != nil {
		t.Fatalf("apply proxy out snapshot: %v", err)
	}

	response := readTCPResponse(t, listenPort, "hello")
	if response != "proxy:hello" {
		t.Fatalf("expected upstream to receive proxy header before payload, got %q", response)
	}
}

func TestSupervisorOutboundProxyProtocolUsesResolvedUpstreamDestination(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	targetAddr, destinationPorts, closeTarget := startTCPRecordProxyV1DestinationServer(t)
	defer closeTarget()
	targetPort := mustPort(t, targetAddr)
	listenPort := reserveLocalTCPPort(t)
	supervisor := NewSupervisor()
	defer supervisor.Close()
	if err := supervisor.Apply(ctx, agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			{
				ID:               "rule_proxy_dns",
				Enabled:          true,
				Protocol:         domain.ProtocolTCP,
				ListenIP:         "127.0.0.1",
				Port:             listenPort,
				MatchType:        "ANY_INBOUND",
				ProxyProtocolOut: "V1",
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target_proxy_dns", Host: "localhost", Port: targetPort, Enabled: true},
				},
			},
		},
	}); err != nil {
		t.Fatalf("apply proxy dns snapshot: %v", err)
	}

	response := readTCPResponse(t, listenPort, "hello")
	if response != "proxy:hello" {
		t.Fatalf("expected proxy response, got %q", response)
	}
	select {
	case destinationPort := <-destinationPorts:
		if destinationPort != targetPort {
			t.Fatalf("expected PROXY destination port %d, got %d", targetPort, destinationPort)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for proxy destination port")
	}
}

func TestSupervisorHandlesProxyProtocolV2InAndOut(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inTargetAddr, closeInTarget := startTCPReadFiveServer(t)
	defer closeInTarget()
	inListenPort := reserveLocalTCPPort(t)
	inSupervisor := NewSupervisor()
	defer inSupervisor.Close()
	if err := inSupervisor.Apply(ctx, singleTCPRuleSnapshot(t, inListenPort, mustPort(t, inTargetAddr), "V2", "NONE")); err != nil {
		t.Fatalf("apply proxy v2 in snapshot: %v", err)
	}
	inConn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(inListenPort)), time.Second)
	if err != nil {
		t.Fatalf("dial proxy v2 in listener: %v", err)
	}
	defer func() { _ = inConn.Close() }()
	header, err := BuildProxyHeader(ProxyInfo{
		Version:         ProxyProtocolV2,
		SourceIP:        net.ParseIP("203.0.113.20"),
		DestinationIP:   net.ParseIP("127.0.0.1"),
		SourcePort:      12346,
		DestinationPort: inListenPort,
	})
	if err != nil {
		t.Fatalf("build proxy v2 header: %v", err)
	}
	if _, err := inConn.Write(append(header, []byte("hello")...)); err != nil {
		t.Fatalf("write proxy v2 header and payload: %v", err)
	}
	if err := inConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set proxy v2 read deadline: %v", err)
	}
	buffer := make([]byte, len("got:hello"))
	if _, err := io.ReadFull(inConn, buffer); err != nil {
		t.Fatalf("read proxy v2 in response: %v", err)
	}
	if string(buffer) != "got:hello" {
		t.Fatalf("expected proxy v2 header consumed, got %q", buffer)
	}

	outTargetAddr, closeOutTarget := startTCPExpectProxyV2Server(t)
	defer closeOutTarget()
	outListenPort := reserveLocalTCPPort(t)
	outSupervisor := NewSupervisor()
	defer outSupervisor.Close()
	if err := outSupervisor.Apply(ctx, singleTCPRuleSnapshot(t, outListenPort, mustPort(t, outTargetAddr), "NONE", "V2")); err != nil {
		t.Fatalf("apply proxy v2 out snapshot: %v", err)
	}
	response := readTCPResponse(t, outListenPort, "hello")
	if response != "proxy2:hello" {
		t.Fatalf("expected upstream to receive proxy v2 header before payload, got %q", response)
	}
}
