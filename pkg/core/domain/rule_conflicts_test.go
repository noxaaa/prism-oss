package domain

import "testing"

func TestANYInboundExcludesAllOtherRulesOnSameEndpoint(t *testing.T) {
	existing := []InboundBinding{
		{
			NodeID:      "node_1",
			ListenIP:    "0.0.0.0",
			Protocol:    ProtocolTCP,
			Port:        443,
			MatchType:   MatchTypeAnyInbound,
			ForwardRule: "rule_any",
		},
	}
	candidate := InboundBinding{
		NodeID:    "node_1",
		ListenIP:  "0.0.0.0",
		Protocol:  ProtocolTCP,
		Port:      443,
		MatchType: MatchTypeTLSSNI,
		SNI:       "app.example.com",
	}

	err := ValidateInboundBindingConflict(existing, candidate)
	if err == nil {
		t.Fatalf("expected conflict")
	}
	if err.Code != ErrRulePortConflict {
		t.Fatalf("expected %s, got %s", ErrRulePortConflict, err.Code)
	}
}

func TestANYInboundExcludesSpecificIPWhenWildcardReservesPort(t *testing.T) {
	existing := []InboundBinding{
		{
			NodeID:    "node_1",
			ListenIP:  "0.0.0.0",
			Protocol:  ProtocolTCP,
			Port:      443,
			MatchType: MatchTypeAnyInbound,
		},
	}
	candidate := InboundBinding{
		NodeID:    "node_1",
		ListenIP:  "127.0.0.1",
		Protocol:  ProtocolTCP,
		Port:      443,
		MatchType: MatchTypeAnyInbound,
	}

	err := ValidateInboundBindingConflict(existing, candidate)
	if err == nil {
		t.Fatalf("expected wildcard listen IP conflict")
	}
	if err.Code != ErrRulePortConflict {
		t.Fatalf("expected %s, got %s", ErrRulePortConflict, err.Code)
	}
}

func TestTLSSNIAllowsSharedTCPPortWhenSNIIsUnique(t *testing.T) {
	existing := []InboundBinding{
		{
			NodeID:    "node_1",
			ListenIP:  "0.0.0.0",
			Protocol:  ProtocolTCP,
			Port:      443,
			MatchType: MatchTypeTLSSNI,
			SNI:       "a.example.com",
		},
	}
	candidate := InboundBinding{
		NodeID:    "node_1",
		ListenIP:  "0.0.0.0",
		Protocol:  ProtocolTCP,
		Port:      443,
		MatchType: MatchTypeTLSSNI,
		SNI:       "b.example.com",
	}

	if err := ValidateInboundBindingConflict(existing, candidate); err != nil {
		t.Fatalf("expected no conflict, got %v", err)
	}
}

func TestTLSSNIRejectsSpecificIPWhenWildcardSharesSamePort(t *testing.T) {
	existing := []InboundBinding{
		{
			NodeID:    "node_1",
			ListenIP:  "0.0.0.0",
			Protocol:  ProtocolTCP,
			Port:      443,
			MatchType: MatchTypeTLSSNI,
			SNI:       "a.example.com",
		},
	}
	candidate := InboundBinding{
		NodeID:    "node_1",
		ListenIP:  "127.0.0.1",
		Protocol:  ProtocolTCP,
		Port:      443,
		MatchType: MatchTypeTLSSNI,
		SNI:       "b.example.com",
	}

	err := ValidateInboundBindingConflict(existing, candidate)
	if err == nil {
		t.Fatalf("expected wildcard/specific SNI conflict")
	}
	if err.Code != ErrRulePortConflict {
		t.Fatalf("expected %s, got %s", ErrRulePortConflict, err.Code)
	}
}

func TestTLSSNIRejectsDuplicateSNIOnSameEndpoint(t *testing.T) {
	existing := []InboundBinding{
		{
			NodeID:    "node_1",
			ListenIP:  "0.0.0.0",
			Protocol:  ProtocolTCP,
			Port:      443,
			MatchType: MatchTypeTLSSNI,
			SNI:       "app.example.com",
		},
	}
	candidate := InboundBinding{
		NodeID:    "node_1",
		ListenIP:  "0.0.0.0",
		Protocol:  ProtocolTCP,
		Port:      443,
		MatchType: MatchTypeTLSSNI,
		SNI:       "APP.example.com",
	}

	err := ValidateInboundBindingConflict(existing, candidate)
	if err == nil {
		t.Fatalf("expected duplicate SNI conflict")
	}
	if err.Code != ErrRuleDuplicateSNI {
		t.Fatalf("expected %s, got %s", ErrRuleDuplicateSNI, err.Code)
	}
}

func TestTLSSNIRejectsMixedInboundProxyProtocolOnSameEndpoint(t *testing.T) {
	existing := []InboundBinding{
		{
			NodeID:          "node_1",
			ListenIP:        "0.0.0.0",
			Protocol:        ProtocolTCP,
			Port:            443,
			MatchType:       MatchTypeTLSSNI,
			SNI:             "a.example.com",
			ProxyProtocolIn: "NONE",
		},
	}
	candidate := InboundBinding{
		NodeID:          "node_1",
		ListenIP:        "0.0.0.0",
		Protocol:        ProtocolTCP,
		Port:            443,
		MatchType:       MatchTypeTLSSNI,
		SNI:             "b.example.com",
		ProxyProtocolIn: "V1",
	}

	err := ValidateInboundBindingConflict(existing, candidate)
	if err == nil {
		t.Fatalf("expected mixed proxy protocol conflict")
	}
	if err.Code != ErrRulePortConflict {
		t.Fatalf("expected %s, got %s", ErrRulePortConflict, err.Code)
	}
}

func TestCommercialMatchTypeReservesFullEndpoint(t *testing.T) {
	existing := []InboundBinding{
		{
			NodeID:    "node_1",
			ListenIP:  "0.0.0.0",
			Protocol:  ProtocolTCP,
			Port:      443,
			MatchType: MatchType("COMMERCIAL_MATCH"),
		},
	}
	candidate := InboundBinding{
		NodeID:    "node_1",
		ListenIP:  "127.0.0.1",
		Protocol:  ProtocolTCP,
		Port:      443,
		MatchType: MatchTypeTLSSNI,
		SNI:       "app.example.com",
	}

	err := ValidateInboundBindingConflict(existing, candidate)
	if err == nil {
		t.Fatalf("expected commercial wildcard endpoint owner conflict")
	}
	if err.Code != ErrRulePortConflict {
		t.Fatalf("expected %s, got %s", ErrRulePortConflict, err.Code)
	}
}

func TestCommercialCandidateConflictsWithCoreEndpointOwner(t *testing.T) {
	existing := []InboundBinding{
		{
			NodeID:    "node_1",
			ListenIP:  "0.0.0.0",
			Protocol:  ProtocolTCP,
			Port:      443,
			MatchType: MatchTypeTLSSNI,
			SNI:       "app.example.com",
		},
	}
	candidate := InboundBinding{
		NodeID:    "node_1",
		ListenIP:  "127.0.0.1",
		Protocol:  ProtocolTCP,
		Port:      443,
		MatchType: MatchType("COMMERCIAL_MATCH"),
	}

	err := ValidateInboundBindingConflict(existing, candidate)
	if err == nil {
		t.Fatalf("expected commercial candidate conflict")
	}
	if err.Code != ErrRulePortConflict {
		t.Fatalf("expected %s, got %s", ErrRulePortConflict, err.Code)
	}
}

func TestTLSSNIRejectsEmptySNI(t *testing.T) {
	candidate := InboundBinding{
		NodeID:    "node_1",
		ListenIP:  "0.0.0.0",
		Protocol:  ProtocolTCP,
		Port:      443,
		MatchType: MatchTypeTLSSNI,
		SNI:       " \t ",
	}

	err := ValidateInboundBindingConflict(nil, candidate)
	if err == nil {
		t.Fatalf("expected empty SNI validation error")
	}
	if err.Code != ErrValidationFailed {
		t.Fatalf("expected %s, got %s", ErrValidationFailed, err.Code)
	}
}

func TestUDPRejectsTLSSNI(t *testing.T) {
	candidate := InboundBinding{
		NodeID:    "node_1",
		ListenIP:  "0.0.0.0",
		Protocol:  ProtocolUDP,
		Port:      443,
		MatchType: MatchTypeTLSSNI,
		SNI:       "app.example.com",
	}

	err := ValidateInboundBindingConflict(nil, candidate)
	if err == nil {
		t.Fatalf("expected UDP SNI validation error")
	}
	if err.Code != ErrValidationFailed {
		t.Fatalf("expected %s, got %s", ErrValidationFailed, err.Code)
	}
}

func TestTCPUDPAnyInboundConflictsWithTCPAndUDPOnSameEndpoint(t *testing.T) {
	existing := []InboundBinding{
		{
			NodeID:    "node_1",
			ListenIP:  "0.0.0.0",
			Protocol:  ProtocolTCP,
			Port:      443,
			MatchType: MatchTypeAnyInbound,
		},
	}
	candidate := InboundBinding{
		NodeID:    "node_1",
		ListenIP:  "0.0.0.0",
		Protocol:  Protocol("TCP_UDP"),
		Port:      443,
		MatchType: MatchTypeAnyInbound,
	}

	err := ValidateInboundBindingConflict(existing, candidate)
	if err == nil {
		t.Fatalf("expected TCP_UDP to conflict with existing TCP endpoint")
	}
	if err.Code != ErrRulePortConflict {
		t.Fatalf("expected %s, got %s", ErrRulePortConflict, err.Code)
	}

	existing[0].Protocol = ProtocolUDP
	err = ValidateInboundBindingConflict(existing, candidate)
	if err == nil {
		t.Fatalf("expected TCP_UDP to conflict with existing UDP endpoint")
	}
	if err.Code != ErrRulePortConflict {
		t.Fatalf("expected %s, got %s", ErrRulePortConflict, err.Code)
	}
}

func TestTCPUDPRejectsTLSSNIAndAllowsProxyProtocol(t *testing.T) {
	tlsCandidate := InboundBinding{
		NodeID:    "node_1",
		ListenIP:  "0.0.0.0",
		Protocol:  Protocol("TCP_UDP"),
		Port:      443,
		MatchType: MatchTypeTLSSNI,
		SNI:       "app.example.com",
	}
	if err := ValidateInboundBindingConflict(nil, tlsCandidate); err == nil || err.Code != ErrValidationFailed {
		t.Fatalf("expected TCP_UDP TLS_SNI validation error, got %v", err)
	}

	proxyCandidate := InboundBinding{
		NodeID:          "node_1",
		ListenIP:        "0.0.0.0",
		Protocol:        Protocol("TCP_UDP"),
		Port:            443,
		MatchType:       MatchTypeAnyInbound,
		ProxyProtocolIn: "V1",
	}
	if err := ValidateInboundBindingConflict(nil, proxyCandidate); err != nil {
		t.Fatalf("expected TCP_UDP proxy protocol to be accepted for the TCP side: %v", err)
	}
}
