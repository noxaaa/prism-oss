package validator

import "testing"

func TestValidateBootstrapRequestNormalizesOrganizationFields(t *testing.T) {
	input, err := ValidateBootstrapRequest(BootstrapRequest{
		OrganizationName: "  Acme Inc  ",
		OrganizationSlug: "  Acme  ",
	})
	if err != nil {
		t.Fatalf("validate bootstrap request: %v", err)
	}

	if input.OrganizationName != "Acme Inc" || input.OrganizationSlug != "acme" {
		t.Fatalf("unexpected normalized input: %#v", input)
	}
}

func TestValidateRoleRequestRejectsFreeFormWildcardInputForLowLevelShape(t *testing.T) {
	_, err := ValidateRoleRequest(RoleRequest{
		Name:        "Operator",
		Permissions: []string{"rules.manage_own"},
		ResourceScopes: []ResourceScopeRequest{
			{ResourceType: "NODE_GROUP", ResourceID: " ", AccessLevel: "USE"},
		},
	})
	if err == nil {
		t.Fatalf("expected blank resource scope to be rejected")
	}
}

func TestValidateRuleRequestOnlyRequiresSNIForTLSSNI(t *testing.T) {
	anyInbound, err := ValidateRuleRequest(validRuleRequest(RuleMatchRequest{
		Type:        "ANY_INBOUND",
		SNIHostname: "ignored.example.com",
	}))
	if err != nil {
		t.Fatalf("ANY_INBOUND should not require SNI: %v", err)
	}
	if anyInbound.Match.SNIHostname != "" {
		t.Fatalf("expected ANY_INBOUND SNI to be cleared, got %q", anyInbound.Match.SNIHostname)
	}

	_, err = ValidateRuleRequest(validRuleRequest(RuleMatchRequest{Type: "TLS_SNI"}))
	if err == nil {
		t.Fatalf("expected TLS_SNI without SNI to be rejected")
	}
}

func TestValidateRuleRequestAcceptsTCPUDPWithoutSNIOrProxyProtocol(t *testing.T) {
	request, err := ValidateRuleRequest(RuleRequest{
		Name:        "Rule",
		NodeGroupID: "node_group_1",
		ListenIP:    "0.0.0.0",
		Protocol:    "tcp_udp",
		Port:        443,
		Match:       RuleMatchRequest{Type: "ANY_INBOUND"},
		ProxyProtocol: ProxyProtocolRequest{
			In:  "NONE",
			Out: "NONE",
		},
		Upstream: RuleUpstreamRequest{
			Type:          "TARGET_GROUP",
			TargetGroupID: "target_group_1",
		},
	})
	if err != nil {
		t.Fatalf("TCP_UDP ANY_INBOUND should be valid: %v", err)
	}
	if request.Protocol != "TCP_UDP" {
		t.Fatalf("expected normalized TCP_UDP protocol, got %q", request.Protocol)
	}
}

func TestValidateRuleRequestRejectsTCPUDPWithTLSSNIAndAllowsProxyProtocol(t *testing.T) {
	_, err := ValidateRuleRequest(validRuleRequestWithProtocol("TCP_UDP", RuleMatchRequest{Type: "TLS_SNI", SNIHostname: "app.example.com"}))
	if err == nil {
		t.Fatalf("expected TCP_UDP TLS_SNI to be rejected")
	}

	request := validRuleRequestWithProtocol("TCP_UDP", RuleMatchRequest{Type: "ANY_INBOUND"})
	request.ProxyProtocol.In = "V1"
	if _, err = ValidateRuleRequest(request); err != nil {
		t.Fatalf("expected TCP_UDP proxy protocol to be accepted for the TCP side: %v", err)
	}
}

func TestValidateTargetRequestDoesNotRequireProtocol(t *testing.T) {
	target, err := ValidateTargetRequest(TargetRequest{
		Name:    "Origin",
		Host:    "origin.example.com",
		Port:    443,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("target protocol should be rule-owned, got validation error: %v", err)
	}
	_ = target
}

func TestValidateTargetRequestNormalizesTargetGroupIDs(t *testing.T) {
	groupIDs := []string{" target_group_a ", "target_group_b", "target_group_a"}
	target, err := ValidateTargetRequest(TargetRequest{
		Name:           "Origin",
		Host:           "origin.example.com",
		Port:           443,
		Enabled:        true,
		TargetGroupIDs: &groupIDs,
	})
	if err != nil {
		t.Fatalf("target group ids should be valid resource selections: %v", err)
	}
	if target.TargetGroupIDs == nil {
		t.Fatalf("expected normalized target group ids")
	}
	expected := []string{"target_group_a", "target_group_b"}
	if len(*target.TargetGroupIDs) != len(expected) {
		t.Fatalf("expected %v target group ids, got %#v", expected, *target.TargetGroupIDs)
	}
	for index, id := range expected {
		if (*target.TargetGroupIDs)[index] != id {
			t.Fatalf("expected normalized group id %q at %d, got %#v", id, index, *target.TargetGroupIDs)
		}
	}
}

func TestValidateTargetGroupRequestAllowsEmptyMembers(t *testing.T) {
	group, err := ValidateTargetGroupRequest(TargetGroupRequest{
		Name:        "Empty pool",
		Description: "Targets can be attached later.",
		Members:     []TargetGroupMemberRequest{},
	})
	if err != nil {
		t.Fatalf("empty target group should be valid: %v", err)
	}
	if len(group.Members) != 0 {
		t.Fatalf("expected empty members to remain empty, got %#v", group.Members)
	}
}

func TestValidateTargetGroupRequestNormalizesScheduler(t *testing.T) {
	group, err := ValidateTargetGroupRequest(TargetGroupRequest{
		Name:        "Pool",
		Description: "Targets can be attached later.",
		Scheduler:   " priority_iphash ",
		Members:     []TargetGroupMemberRequest{},
	})
	if err != nil {
		t.Fatalf("target group scheduler should be normalized: %v", err)
	}
	if group.Scheduler != "PRIORITY_IPHASH" {
		t.Fatalf("expected normalized scheduler PRIORITY_IPHASH, got %q", group.Scheduler)
	}
}

func TestValidateTargetGroupRequestDefaultsSchedulerWithoutBlockingExtensions(t *testing.T) {
	defaulted, err := ValidateTargetGroupRequest(TargetGroupRequest{
		Name:        "Default pool",
		Description: "Targets can be attached later.",
		Members:     []TargetGroupMemberRequest{},
	})
	if err != nil {
		t.Fatalf("target group scheduler should default: %v", err)
	}
	if defaulted.Scheduler != "PRIORITY_IPHASH" {
		t.Fatalf("expected default scheduler PRIORITY_IPHASH, got %q", defaulted.Scheduler)
	}

	commercial, err := ValidateTargetGroupRequest(TargetGroupRequest{
		Name:        "Commercial pool",
		Description: "Targets can be attached later.",
		Scheduler:   " geo_ip ",
		Members:     []TargetGroupMemberRequest{},
	})
	if err != nil {
		t.Fatalf("validator should leave scheduler support policy to service layer: %v", err)
	}
	if commercial.Scheduler != "GEO_IP" {
		t.Fatalf("expected commercial scheduler to be normalized, got %q", commercial.Scheduler)
	}
}

func TestValidateNodeRequestDefaultsListenIPsAndPortRange(t *testing.T) {
	node, err := ValidateNodeRequest(NodeRequest{
		Name:     "edge-a",
		GroupIDs: []string{" node_group_a "},
	})
	if err != nil {
		t.Fatalf("empty listen IPs and port ranges should default: %v", err)
	}
	if len(node.ListenIPs) != 1 || node.ListenIPs[0].ListenIP != "0.0.0.0" || node.ListenIPs[0].DisplayName != "default" {
		t.Fatalf("expected default listen IP 0.0.0.0/default, got %#v", node.ListenIPs)
	}
	if len(node.PortRanges) != 1 || node.PortRanges[0].Protocol != "TCP" || node.PortRanges[0].StartPort != 10000 || node.PortRanges[0].EndPort != 20000 {
		t.Fatalf("expected default TCP 10000-20000 port range, got %#v", node.PortRanges)
	}
}

func TestValidateNodeRequestNormalizesMultipleListenIPsAndBlankLabels(t *testing.T) {
	node, err := ValidateNodeRequest(NodeRequest{
		Name:     "edge-a",
		GroupIDs: []string{"node_group_a"},
		ListenIPs: []NodeListenIP{
			{ListenIP: " 0.0.0.0 "},
			{ListenIP: " 192.0.2.10 "},
		},
		PortRanges: []NodePortRange{{Protocol: "tcp"}},
	})
	if err != nil {
		t.Fatalf("multiple listen IPs should be valid: %v", err)
	}
	if len(node.ListenIPs) != 2 {
		t.Fatalf("expected two listen IPs, got %#v", node.ListenIPs)
	}
	if node.ListenIPs[0].DisplayName != "default" {
		t.Fatalf("expected wildcard listen IP label default, got %#v", node.ListenIPs[0])
	}
	if node.ListenIPs[1].DisplayName != "192.0.2.10" {
		t.Fatalf("expected blank custom IP label to use IP, got %#v", node.ListenIPs[1])
	}
	if len(node.PortRanges) != 1 || node.PortRanges[0].Protocol != "TCP" || node.PortRanges[0].StartPort != 10000 || node.PortRanges[0].EndPort != 20000 {
		t.Fatalf("expected blank ports to default under normalized protocol, got %#v", node.PortRanges)
	}
}

func validRuleRequest(match RuleMatchRequest) RuleRequest {
	return validRuleRequestWithProtocol("TCP", match)
}

func validRuleRequestWithProtocol(protocol string, match RuleMatchRequest) RuleRequest {
	return RuleRequest{
		Name:        "Rule",
		NodeGroupID: "node_group_1",
		ListenIP:    "0.0.0.0",
		Protocol:    protocol,
		Port:        443,
		Match:       match,
		Upstream: RuleUpstreamRequest{
			Type:          "TARGET_GROUP",
			TargetGroupID: "target_group_1",
		},
	}
}
