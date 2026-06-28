package forward

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"

	"nhooyr.io/websocket"
)

func reserveLocalTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve tcp port: %v", err)
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}

func reserveLocalUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve udp port: %v", err)
	}
	defer func() { _ = conn.Close() }()
	return conn.LocalAddr().(*net.UDPAddr).Port
}

func mustPort(t *testing.T, address string) int {
	t.Helper()
	_, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("split address %q: %v", address, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse port %q: %v", portText, err)
	}
	return port
}

func readTCPResponse(t *testing.T, listenPort int, payload string) string {
	t.Helper()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)), time.Second)
	if err != nil {
		t.Fatalf("dial tcp listener: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte(payload)); err != nil {
		t.Fatalf("write tcp payload: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set tcp read deadline: %v", err)
	}
	buffer := make([]byte, len(payload)+len("primary-a:"))
	n, err := conn.Read(buffer)
	if err != nil {
		t.Fatalf("read tcp response: %v", err)
	}
	return string(buffer[:n])
}

func startTCPUDPEchoServer(t *testing.T) (int, func()) {
	t.Helper()
	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp echo server: %v", err)
	}
	port := tcpListener.Addr().(*net.TCPAddr).Port
	udpConn, err := net.ListenPacket("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		_ = tcpListener.Close()
		t.Fatalf("listen udp echo server on tcp port: %v", err)
	}

	tcpDone := make(chan struct{})
	go func() {
		defer close(tcpDone)
		for {
			conn, err := tcpListener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()

	udpDone := make(chan struct{})
	go func() {
		defer close(udpDone)
		buffer := make([]byte, 65535)
		for {
			n, addr, err := udpConn.ReadFrom(buffer)
			if err != nil {
				return
			}
			_, _ = udpConn.WriteTo(buffer[:n], addr)
		}
	}()

	return port, func() {
		_ = tcpListener.Close()
		_ = udpConn.Close()
		<-tcpDone
		<-udpDone
	}
}

func startTCPPrefixServer(t *testing.T, prefix string) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen prefix tcp server: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				buffer := make([]byte, 1024)
				n, err := conn.Read(buffer)
				if err != nil {
					return
				}
				_, _ = conn.Write(append([]byte(prefix), buffer[:n]...))
			}()
		}
	}()
	return listener.Addr().String(), func() {
		_ = listener.Close()
		<-done
	}
}

func startTLSClientHelloPrefixServer(t *testing.T, prefix string) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tls prefix tcp server: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
					return
				}
				header := make([]byte, 5)
				if _, err := io.ReadFull(conn, header); err != nil {
					return
				}
				recordLength := int(header[3])<<8 | int(header[4])
				if recordLength <= 0 || recordLength > maxTLSClientHelloBytes {
					return
				}
				body := make([]byte, recordLength)
				if _, err := io.ReadFull(conn, body); err != nil {
					return
				}
				_ = conn.SetReadDeadline(time.Time{})
				_, _ = conn.Write([]byte(prefix))
			}()
		}
	}()
	return listener.Addr().String(), func() {
		_ = listener.Close()
		<-done
	}
}

func startExpectedBytesPrefixServer(t *testing.T, expected []byte, prefix string) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen expected bytes tcp server: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
					return
				}
				buffer := make([]byte, len(expected))
				if _, err := io.ReadFull(conn, buffer); err != nil {
					return
				}
				if !bytes.Equal(buffer, expected) {
					return
				}
				_ = conn.SetReadDeadline(time.Time{})
				_, _ = conn.Write([]byte(prefix))
			}()
		}
	}()
	return listener.Addr().String(), func() {
		_ = listener.Close()
		<-done
	}
}

func writeClientHelloAndReadPrefix(t *testing.T, listenPort int, serverName string) string {
	t.Helper()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)), time.Second)
	if err != nil {
		t.Fatalf("dial sni listener: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write(capturedClientHello(t, serverName)); err != nil {
		t.Fatalf("write client hello: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set sni read deadline: %v", err)
	}
	buffer := make([]byte, 64)
	n, err := conn.Read(buffer)
	if err != nil {
		t.Fatalf("read sni response: %v", err)
	}
	return string(buffer[:n])
}

func capturedClientHello(t *testing.T, serverName string) []byte {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	defer func() { _ = clientConn.Close() }()
	defer func() { _ = serverConn.Close() }()

	done := make(chan struct{})
	go func() {
		defer close(done)
		tlsConn := tls.Client(clientConn, &tls.Config{ServerName: serverName, InsecureSkipVerify: true})
		_ = tlsConn.Handshake()
	}()

	if err := serverConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set pipe deadline: %v", err)
	}
	header := make([]byte, 5)
	if _, err := io.ReadFull(serverConn, header); err != nil {
		t.Fatalf("read client hello header: %v", err)
	}
	length := int(header[3])<<8 | int(header[4])
	body := make([]byte, length)
	if _, err := io.ReadFull(serverConn, body); err != nil {
		t.Fatalf("read client hello body: %v", err)
	}
	_ = serverConn.Close()
	<-done
	return append(header, body...)
}

func fragmentedTLSHandshakeRecords(t *testing.T, record []byte, firstBodyBytes int) []byte {
	t.Helper()
	if len(record) <= 5 {
		t.Fatalf("tls record too short: %d", len(record))
	}
	body := record[5:]
	if firstBodyBytes <= 0 || firstBodyBytes >= len(body) {
		t.Fatalf("invalid first TLS fragment size %d for body length %d", firstBodyBytes, len(body))
	}
	first := tlsRecord(record[1:3], body[:firstBodyBytes])
	second := tlsRecord(record[1:3], body[firstBodyBytes:])
	return append(first, second...)
}

func tlsRecord(version []byte, body []byte) []byte {
	record := []byte{0x16, version[0], version[1], byte(len(body) >> 8), byte(len(body))}
	return append(record, body...)
}

func singleTCPRuleSnapshot(t *testing.T, listenPort int, targetPort int, proxyIn string, proxyOut string) agent.ConfigSnapshot {
	t.Helper()
	return agent.ConfigSnapshot{
		NodeID:        "node_1",
		ConfigVersion: 1,
		Rules: []agent.RuleConfig{
			{
				ID:               "rule_proxy",
				Enabled:          true,
				Protocol:         domain.ProtocolTCP,
				ListenIP:         "127.0.0.1",
				Port:             listenPort,
				MatchType:        "ANY_INBOUND",
				ProxyProtocolIn:  proxyIn,
				ProxyProtocolOut: proxyOut,
				Upstream: agent.RuleUpstreamConfig{
					Type:   "TARGET",
					Target: &agent.TargetEndpoint{ID: "target_proxy", Host: "127.0.0.1", Port: targetPort, Enabled: true},
				},
			},
		},
	}
}

func startTCPReadFiveServer(t *testing.T) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen read-five server: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
					return
				}
				buffer := make([]byte, 5)
				if _, err := io.ReadFull(conn, buffer); err != nil {
					return
				}
				_, _ = conn.Write(append([]byte("got:"), buffer...))
			}()
		}
	}()
	return listener.Addr().String(), func() {
		_ = listener.Close()
		<-done
	}
}

func startTCPExpectProxyV1Server(t *testing.T) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen proxy expect server: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
					return
				}
				reader := bufio.NewReader(conn)
				line, err := reader.ReadString('\n')
				if err != nil || !strings.HasPrefix(line, "PROXY TCP4 ") {
					return
				}
				buffer := make([]byte, 5)
				if _, err := io.ReadFull(reader, buffer); err != nil {
					return
				}
				_, _ = conn.Write(append([]byte("proxy:"), buffer...))
			}()
		}
	}()
	return listener.Addr().String(), func() {
		_ = listener.Close()
		<-done
	}
}

func startTCPRecordProxyV1DestinationServer(t *testing.T) (string, <-chan int, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen proxy destination server: %v", err)
	}
	destinationPorts := make(chan int, 4)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
					return
				}
				reader := bufio.NewReader(conn)
				line, err := reader.ReadBytes('\n')
				if err != nil {
					return
				}
				info, _, err := ParseProxyHeader(line)
				if err != nil {
					return
				}
				destinationPorts <- info.DestinationPort
				buffer := make([]byte, 5)
				if _, err := io.ReadFull(reader, buffer); err != nil {
					return
				}
				_, _ = conn.Write(append([]byte("proxy:"), buffer...))
			}()
		}
	}()
	return listener.Addr().String(), destinationPorts, func() {
		_ = listener.Close()
		<-done
	}
}

func startTCPExpectProxyV2Server(t *testing.T) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen proxy v2 expect server: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
					return
				}
				header := make([]byte, 28)
				if _, err := io.ReadFull(conn, header); err != nil {
					return
				}
				if _, _, err := ParseProxyHeader(header); err != nil {
					return
				}
				buffer := make([]byte, 5)
				if _, err := io.ReadFull(conn, buffer); err != nil {
					return
				}
				_, _ = conn.Write(append([]byte("proxy2:"), buffer...))
			}()
		}
	}()
	return listener.Addr().String(), func() {
		_ = listener.Close()
		<-done
	}
}

type forwardRuntimeEnvelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func writeRuntimeEnvelopeForForwardTest(t *testing.T, ctx context.Context, conn *websocket.Conn, messageType string, payload any) {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"type":       messageType,
		"message_id": "test_" + messageType,
		"sent_at":    time.Now().UTC().Format(time.RFC3339Nano),
		"payload":    payload,
	})
	if err != nil {
		t.Fatalf("marshal runtime envelope: %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write runtime envelope: %v", err)
	}
}

func readRuntimeEnvelopeForForwardTest(t *testing.T, ctx context.Context, conn *websocket.Conn) forwardRuntimeEnvelope {
	t.Helper()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read runtime envelope: %v", err)
	}
	var envelope forwardRuntimeEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode runtime envelope: %v", err)
	}
	return envelope
}

func waitForRuntimeAck(t *testing.T, acks <-chan string) {
	t.Helper()
	select {
	case <-acks:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for runtime ack")
	}
}

func startUDPSourceRecorder(t *testing.T) (string, <-chan string, func()) {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp recorder: %v", err)
	}
	sources := make(chan string, 8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer := make([]byte, 65535)
		for {
			n, address, err := conn.ReadFrom(buffer)
			if err != nil {
				return
			}
			sources <- address.String()
			_, _ = conn.WriteTo(buffer[:n], address)
		}
	}()
	return conn.LocalAddr().String(), sources, func() {
		_ = conn.Close()
		<-done
	}
}

func startTCPSourceRecorder(t *testing.T) (string, <-chan string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp source recorder: %v", err)
	}
	sources := make(chan string, 8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			sources <- conn.RemoteAddr().String()
			_ = conn.Close()
		}
	}()
	return listener.Addr().String(), sources, func() {
		_ = listener.Close()
		<-done
	}
}

func readUDPSourceAddress(t *testing.T, sources <-chan string) string {
	t.Helper()
	select {
	case source := <-sources:
		return source
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for UDP source address")
		return ""
	}
}

func readTCPSourceAddress(t *testing.T, sources <-chan string) string {
	t.Helper()
	select {
	case source := <-sources:
		return source
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for TCP source address")
		return ""
	}
}

func probeTCPSourceIP(sendIP string, address string) error {
	conn, err := dialTCPUpstream(sendIP, address)
	if err != nil {
		return err
	}
	return conn.Close()
}

func probeUDPSourceIP(sendIP string, address string) error {
	upstreamAddress, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return err
	}
	conn, err := dialUDPUpstream(sendIP, upstreamAddress)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	_, err = conn.Write([]byte("probe"))
	return err
}

func waitForSupervisorMetrics(t *testing.T, supervisor *Supervisor, predicate func(Metrics) bool) Metrics {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	var metrics Metrics
	for time.Now().Before(deadline) {
		metrics = supervisor.Metrics()
		if predicate(metrics) {
			return metrics
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for supervisor metrics, last metrics %#v", metrics)
	return Metrics{}
}

func assertTCPEchoThroughPort(t *testing.T, port int, payload string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err != nil {
		t.Fatalf("dial runtime listener: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte(payload)); err != nil {
		t.Fatalf("write runtime payload: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set runtime read deadline: %v", err)
	}
	buffer := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, buffer); err != nil {
		t.Fatalf("read runtime echo: %v", err)
	}
	if string(buffer) != payload {
		t.Fatalf("expected runtime echo %q, got %q", payload, buffer)
	}
}
