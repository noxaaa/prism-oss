package handler

import (
	"net"
	"net/http"
	"testing"
)

func TestObservedDirectAgentRemoteAddrIgnoresForwardedProxyPeer(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "/agent/v1/connect", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	request.RemoteAddr = "198.51.100.10:443"
	request.Header.Set("X-Forwarded-For", "203.0.113.25")

	if got := observedDirectAgentRemoteAddr(request); got != "" {
		t.Fatalf("forwarded proxy peer should not be used as node publish address, got %q", got)
	}
}

func TestObservedDirectAgentRemoteAddrKeepsDirectPeer(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "/agent/v1/connect", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	request.RemoteAddr = "203.0.113.25:443"

	if got := observedDirectAgentRemoteAddr(request); got != "203.0.113.25:443" {
		t.Fatalf("expected direct peer to be preserved, got %q", got)
	}
}

func TestObservedAgentRemoteIPUsesTrustedForwardedClient(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "/agent/v1/connect", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	request.RemoteAddr = "198.51.100.10:443"
	request.Header.Set("X-Forwarded-For", "203.0.113.25, 198.51.100.10")
	server := &ControlServer{trustedProxies: parseTrustedProxyCIDRs([]string{"198.51.100.0/24"})}

	if got := server.observedAgentRemoteIP(request); got != "203.0.113.25" {
		t.Fatalf("expected trusted forwarded client IP, got %q", got)
	}
}

func TestObservedAgentRemoteIPWalksForwardedForFromTrustedSide(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "/agent/v1/connect", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	request.RemoteAddr = "192.0.2.10:443"
	request.Header.Set("X-Forwarded-For", "203.0.113.25, 198.51.100.77")
	server := &ControlServer{trustedProxies: parseTrustedProxyCIDRs([]string{"192.0.2.0/24"})}

	if got := server.observedAgentRemoteIP(request); got != "198.51.100.77" {
		t.Fatalf("expected trusted chain to use rightmost untrusted client IP, got %q", got)
	}
}

func TestObservedAgentRemoteIPUsesTrustedForwardedHeaderChain(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "/agent/v1/connect", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	request.RemoteAddr = "198.51.100.10:443"
	request.Header.Set("Forwarded", "for=203.0.113.25, for=198.51.100.10")
	server := &ControlServer{trustedProxies: parseTrustedProxyCIDRs([]string{"198.51.100.0/24"})}

	if got := server.observedAgentRemoteIP(request); got != "203.0.113.25" {
		t.Fatalf("expected trusted Forwarded chain client IP, got %q", got)
	}
}

func TestObservedAgentRemoteIPParsesBracketedForwardedIPv6(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "/agent/v1/connect", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	request.RemoteAddr = "198.51.100.10:443"
	request.Header.Set("Forwarded", `for="[2001:db8::1]:443", for=198.51.100.10`)
	server := &ControlServer{trustedProxies: parseTrustedProxyCIDRs([]string{"198.51.100.0/24"})}

	if got := server.observedAgentRemoteIP(request); got != "2001:db8::1" {
		t.Fatalf("expected trusted Forwarded IPv6 client IP, got %q", got)
	}
}

func TestObservedAgentRemoteIPIgnoresUntrustedForwardedClient(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "/agent/v1/connect", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	request.RemoteAddr = "198.51.100.10:443"
	request.Header.Set("X-Forwarded-For", "203.0.113.25")
	server := &ControlServer{trustedProxies: parseTrustedProxyCIDRs([]string{"192.0.2.0/24"})}

	if got := server.observedAgentRemoteIP(request); got != "198.51.100.10" {
		t.Fatalf("expected untrusted proxy peer IP, got %q", got)
	}
}

func TestParseTrustedProxyCIDRsSkipsInvalidValues(t *testing.T) {
	cidrs := parseTrustedProxyCIDRs([]string{"invalid", "198.51.100.0/24"})
	if len(cidrs) != 1 || !cidrs[0].Contains(net.ParseIP("198.51.100.10")) {
		t.Fatalf("expected only valid CIDR to be parsed, got %#v", cidrs)
	}
}
