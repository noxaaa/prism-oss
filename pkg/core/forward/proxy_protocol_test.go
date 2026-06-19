package forward

import (
	"bufio"
	"bytes"
	"net"
	"strings"
	"testing"
)

func TestProxyProtocolV1ParseTCP4(t *testing.T) {
	header := []byte("PROXY TCP4 203.0.113.10 198.51.100.20 12345 443\r\npayload")

	info, rest, err := ParseProxyHeader(header)
	if err != nil {
		t.Fatalf("parse proxy header: %v", err)
	}
	if info.Version != ProxyProtocolV1 {
		t.Fatalf("expected v1, got %v", info.Version)
	}
	if info.SourceIP.String() != "203.0.113.10" || info.DestinationIP.String() != "198.51.100.20" {
		t.Fatalf("unexpected addresses: %#v", info)
	}
	if info.SourcePort != 12345 || info.DestinationPort != 443 {
		t.Fatalf("unexpected ports: %#v", info)
	}
	if !bytes.Equal(rest, []byte("payload")) {
		t.Fatalf("unexpected rest %q", rest)
	}
}

func TestProxyProtocolV1ParseTCP6(t *testing.T) {
	header := []byte("PROXY TCP6 2001:db8::1 2001:db8::2 12345 443\r\npayload")

	info, rest, err := ParseProxyHeader(header)
	if err != nil {
		t.Fatalf("parse proxy header: %v", err)
	}
	if info.Version != ProxyProtocolV1 {
		t.Fatalf("expected v1, got %v", info.Version)
	}
	if info.SourceIP.String() != "2001:db8::1" || info.DestinationIP.String() != "2001:db8::2" {
		t.Fatalf("unexpected addresses: %#v", info)
	}
	if info.SourcePort != 12345 || info.DestinationPort != 443 {
		t.Fatalf("unexpected ports: %#v", info)
	}
	if !bytes.Equal(rest, []byte("payload")) {
		t.Fatalf("unexpected rest %q", rest)
	}
}

func TestProxyProtocolV1RejectsMalformedTCP4Address(t *testing.T) {
	tests := []struct {
		name   string
		header []byte
	}{
		{
			name:   "source",
			header: []byte("PROXY TCP4 not-an-ip 198.51.100.20 12345 443\r\npayload"),
		},
		{
			name:   "destination",
			header: []byte("PROXY TCP4 203.0.113.10 not-an-ip 12345 443\r\npayload"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := ParseProxyHeader(test.header); err == nil {
				t.Fatalf("expected malformed address error")
			}
		})
	}
}

func TestProxyProtocolV1RejectsOutOfRangePorts(t *testing.T) {
	tests := []struct {
		name   string
		header []byte
	}{
		{
			name:   "negative source",
			header: []byte("PROXY TCP4 203.0.113.10 198.51.100.20 -1 443\r\npayload"),
		},
		{
			name:   "large source",
			header: []byte("PROXY TCP4 203.0.113.10 198.51.100.20 70000 443\r\npayload"),
		},
		{
			name:   "negative destination",
			header: []byte("PROXY TCP4 203.0.113.10 198.51.100.20 12345 -1\r\npayload"),
		},
		{
			name:   "large destination",
			header: []byte("PROXY TCP4 203.0.113.10 198.51.100.20 12345 70000\r\npayload"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := ParseProxyHeader(test.header); err == nil {
				t.Fatalf("expected out-of-range port error")
			}
		})
	}
}

func TestConsumeInboundProxyHeaderRejectsMissingV1Header(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("GET / HTTP/1.1\r\npayload"))
	if _, err := consumeInboundProxyHeader(reader, "V1"); err == nil {
		t.Fatalf("expected missing proxy header error")
	}
}

func TestConsumeInboundProxyHeaderRejectsOverlongV1Header(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("PROXY " + strings.Repeat("A", maxProxyProtocolV1LineLen+1)))
	if _, err := consumeInboundProxyHeader(reader, "V1"); err == nil {
		t.Fatalf("expected overlong proxy header error")
	}
}

func TestProxyProtocolV1BuildTCP4(t *testing.T) {
	info := ProxyInfo{
		Version:         ProxyProtocolV1,
		SourceIP:        net.ParseIP("203.0.113.10"),
		DestinationIP:   net.ParseIP("198.51.100.20"),
		SourcePort:      12345,
		DestinationPort: 443,
	}

	header, err := BuildProxyHeader(info)
	if err != nil {
		t.Fatalf("build proxy header: %v", err)
	}
	if string(header) != "PROXY TCP4 203.0.113.10 198.51.100.20 12345 443\r\n" {
		t.Fatalf("unexpected header %q", header)
	}
}

func TestProxyProtocolV2RoundTripTCP4(t *testing.T) {
	info := ProxyInfo{
		Version:         ProxyProtocolV2,
		SourceIP:        net.ParseIP("203.0.113.10"),
		DestinationIP:   net.ParseIP("198.51.100.20"),
		SourcePort:      12345,
		DestinationPort: 443,
	}

	header, err := BuildProxyHeader(info)
	if err != nil {
		t.Fatalf("build v2 header: %v", err)
	}
	parsed, rest, err := ParseProxyHeader(append(header, []byte("payload")...))
	if err != nil {
		t.Fatalf("parse v2 header: %v", err)
	}
	if parsed.Version != ProxyProtocolV2 {
		t.Fatalf("expected v2, got %v", parsed.Version)
	}
	if parsed.SourceIP.String() != info.SourceIP.String() || parsed.DestinationIP.String() != info.DestinationIP.String() {
		t.Fatalf("unexpected addresses %#v", parsed)
	}
	if parsed.SourcePort != info.SourcePort || parsed.DestinationPort != info.DestinationPort {
		t.Fatalf("unexpected ports %#v", parsed)
	}
	if !bytes.Equal(rest, []byte("payload")) {
		t.Fatalf("unexpected rest %q", rest)
	}
}

func TestProxyProtocolV2AcceptsTCP4HeaderWithTLV(t *testing.T) {
	info := ProxyInfo{
		Version:         ProxyProtocolV2,
		SourceIP:        net.ParseIP("203.0.113.10"),
		DestinationIP:   net.ParseIP("198.51.100.20"),
		SourcePort:      12345,
		DestinationPort: 443,
	}
	header, err := BuildProxyHeader(info)
	if err != nil {
		t.Fatalf("build v2 header: %v", err)
	}
	header[15] = 16
	header = append(header, []byte{0x20, 0x02, 0xab, 0xcd}...)

	parsed, rest, err := ParseProxyHeader(append(header, []byte("payload")...))
	if err != nil {
		t.Fatalf("parse v2 header with TLV: %v", err)
	}
	if parsed.SourceIP.String() != info.SourceIP.String() || parsed.DestinationIP.String() != info.DestinationIP.String() {
		t.Fatalf("unexpected addresses %#v", parsed)
	}
	if parsed.SourcePort != info.SourcePort || parsed.DestinationPort != info.DestinationPort {
		t.Fatalf("unexpected ports %#v", parsed)
	}
	if !bytes.Equal(rest, []byte("payload")) {
		t.Fatalf("unexpected rest %q", rest)
	}
}

func TestProxyProtocolV2ParseTCP6(t *testing.T) {
	header := buildProxyV2TCP6TestHeader(t, "2001:db8::10", "2001:db8::20", 12345, 443)

	info, rest, err := ParseProxyHeader(append(header, []byte("payload")...))
	if err != nil {
		t.Fatalf("parse proxy v2 tcp6 header: %v", err)
	}
	if info.Version != ProxyProtocolV2 {
		t.Fatalf("expected v2, got %v", info.Version)
	}
	if info.SourceIP.String() != "2001:db8::10" || info.DestinationIP.String() != "2001:db8::20" {
		t.Fatalf("unexpected addresses: %#v", info)
	}
	if info.SourcePort != 12345 || info.DestinationPort != 443 {
		t.Fatalf("unexpected ports: %#v", info)
	}
	if !bytes.Equal(rest, []byte("payload")) {
		t.Fatalf("unexpected rest %q", rest)
	}
}

func TestConsumeInboundProxyHeaderAcceptsV2TCP6(t *testing.T) {
	header := buildProxyV2TCP6TestHeader(t, "2001:db8::10", "2001:db8::20", 12345, 443)
	reader := bufio.NewReader(bytes.NewReader(append(header, []byte("payload")...)))

	info, err := consumeInboundProxyHeader(reader, "V2")
	if err != nil {
		t.Fatalf("consume proxy v2 tcp6 header: %v", err)
	}
	if info.SourceIP.String() != "2001:db8::10" || info.DestinationIP.String() != "2001:db8::20" {
		t.Fatalf("unexpected addresses: %#v", info)
	}
	remaining, err := reader.ReadString('d')
	if err != nil {
		t.Fatalf("read remaining payload: %v", err)
	}
	if remaining != "payload" {
		t.Fatalf("expected payload to remain after header, got %q", remaining)
	}
}

func buildProxyV2TCP6TestHeader(t *testing.T, sourceIP string, destinationIP string, sourcePort int, destinationPort int) []byte {
	t.Helper()
	source := net.ParseIP(sourceIP).To16()
	destination := net.ParseIP(destinationIP).To16()
	if source == nil || destination == nil || net.ParseIP(sourceIP).To4() != nil || net.ParseIP(destinationIP).To4() != nil {
		t.Fatalf("test addresses must be valid IPv6: %s %s", sourceIP, destinationIP)
	}
	header := make([]byte, 52)
	copy(header[:12], proxyV2Signature)
	header[12] = 0x21
	header[13] = 0x21
	header[14] = 0
	header[15] = 36
	copy(header[16:32], source)
	copy(header[32:48], destination)
	header[48] = byte(sourcePort >> 8)
	header[49] = byte(sourcePort)
	header[50] = byte(destinationPort >> 8)
	header[51] = byte(destinationPort)
	return header
}
