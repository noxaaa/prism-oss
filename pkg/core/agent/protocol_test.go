package agent

import "testing"

func TestCurrentProtocolVersionCoversTCPUDPConfigContract(t *testing.T) {
	current := CurrentProtocolVersion()
	if current.Major < 2 {
		t.Fatalf("TCP_UDP rule config requires agent protocol major 2+, got %d.%d", current.Major, current.Minor)
	}
}

func TestCurrentProtocolVersionCoversManagedDataplaneConfigContract(t *testing.T) {
	current := CurrentProtocolVersion()
	required := ManagedDataplaneProtocolVersion()
	if current.Major < required.Major || (current.Major == required.Major && current.Minor < required.Minor) {
		t.Fatalf("managed dataplane config requires agent protocol %d.%d+, got %d.%d", required.Major, required.Minor, current.Major, current.Minor)
	}
}

func TestProtocolVersionAcceptsSameMajorAndCompatibleMinor(t *testing.T) {
	server := ProtocolVersion{Major: 1, Minor: 2}
	agent := ProtocolVersion{Major: 1, Minor: 1}

	if !server.Accepts(agent) {
		t.Fatalf("expected server to accept one minor behind")
	}
}

func TestProtocolVersionRejectsDifferentMajor(t *testing.T) {
	server := ProtocolVersion{Major: 1, Minor: 2}
	agent := ProtocolVersion{Major: 2, Minor: 0}

	if server.Accepts(agent) {
		t.Fatalf("expected server to reject different major")
	}
}

func TestProtocolVersionRejectsAgentMoreThanOneMinorBehind(t *testing.T) {
	server := ProtocolVersion{Major: 1, Minor: 3}
	agent := ProtocolVersion{Major: 1, Minor: 1}

	if server.Accepts(agent) {
		t.Fatalf("expected server to reject agent more than one minor behind")
	}
}

func TestProtocolVersionRejectsAgentNewerMinor(t *testing.T) {
	server := ProtocolVersion{Major: 1, Minor: 0}
	agent := ProtocolVersion{Major: 1, Minor: 1}

	if server.Accepts(agent) {
		t.Fatalf("expected server to reject newer agent minor")
	}
}
