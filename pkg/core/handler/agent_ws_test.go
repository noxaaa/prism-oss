package handler

import (
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
