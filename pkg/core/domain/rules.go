package domain

import (
	"net"
	"strings"
)

type Protocol string

const (
	ProtocolTCP    Protocol = "TCP"
	ProtocolUDP    Protocol = "UDP"
	ProtocolTCPUDP Protocol = "TCP_UDP"
)

type ForwardingType string

const (
	ForwardingTypeDirect ForwardingType = "DIRECT"
	ForwardingTypeTunnel ForwardingType = "TUNNEL"
)

type MatchType string

const (
	MatchTypeAnyInbound MatchType = "ANY_INBOUND"
	MatchTypeTLSSNI     MatchType = "TLS_SNI"
	MatchTypeFeature    MatchType = "FEATURE"
)

type InboundBinding struct {
	NodeID          string
	ListenIP        string
	Protocol        Protocol
	Port            int
	StartPort       int
	EndPort         int
	MatchType       MatchType
	SNI             string
	ProxyProtocolIn string
	ForwardRule     string
}

func ValidateInboundBindingConflict(existing []InboundBinding, candidate InboundBinding) *DomainError {
	if candidate.MatchType == MatchTypeTLSSNI && candidate.Protocol != ProtocolTCP {
		return newDomainError(ErrValidationFailed, "TLS_SNI is only valid for TCP inbound bindings")
	}
	if candidate.Protocol == ProtocolTCPUDP && candidate.MatchType != MatchTypeAnyInbound {
		return newDomainError(ErrValidationFailed, "TCP_UDP inbound bindings only support ANY_INBOUND matching")
	}
	if candidate.MatchType == MatchTypeTLSSNI && strings.TrimSpace(candidate.SNI) == "" {
		return newDomainError(ErrValidationFailed, "TLS_SNI inbound bindings require an SNI hostname")
	}
	if candidate.MatchType == MatchTypeFeature {
		return newDomainError(ErrValidationFailed, "FEATURE matching is reserved for future protocol support")
	}

	for _, binding := range existing {
		if !sameEndpoint(binding, candidate) {
			continue
		}
		if binding.MatchType == MatchTypeAnyInbound || candidate.MatchType == MatchTypeAnyInbound {
			return newDomainError(ErrRulePortConflict, "ANY_INBOUND reserves the full inbound endpoint")
		}
		if binding.MatchType != MatchTypeTLSSNI || candidate.MatchType != MatchTypeTLSSNI {
			return newDomainError(ErrRulePortConflict, "unsupported match types reserve the full inbound endpoint")
		}
		if binding.MatchType == MatchTypeTLSSNI &&
			candidate.MatchType == MatchTypeTLSSNI &&
			!sameListenIPValue(binding.ListenIP, candidate.ListenIP) {
			return newDomainError(ErrRulePortConflict, "TLS_SNI bindings cannot mix overlapping wildcard and specific listen IPs on the same inbound endpoint")
		}
		if binding.MatchType == MatchTypeTLSSNI &&
			candidate.MatchType == MatchTypeTLSSNI &&
			normalizeProxyProtocol(binding.ProxyProtocolIn) != normalizeProxyProtocol(candidate.ProxyProtocolIn) {
			return newDomainError(ErrRulePortConflict, "TLS_SNI bindings on the same inbound endpoint require the same inbound proxy protocol mode")
		}
		if binding.MatchType == MatchTypeTLSSNI &&
			candidate.MatchType == MatchTypeTLSSNI &&
			strings.EqualFold(binding.SNI, candidate.SNI) {
			return newDomainError(ErrRuleDuplicateSNI, "TLS_SNI binding already exists on this inbound endpoint")
		}
	}

	return nil
}

func normalizeProxyProtocol(value string) string {
	value = strings.TrimSpace(strings.ToUpper(value))
	if value == "" || value == "NONE" {
		return ""
	}
	return value
}

func sameEndpoint(left InboundBinding, right InboundBinding) bool {
	return left.NodeID == right.NodeID &&
		listenIPsOverlap(left.ListenIP, right.ListenIP) &&
		protocolsOverlap(left.Protocol, right.Protocol) &&
		portRangesOverlap(left, right)
}

func portRangesOverlap(left InboundBinding, right InboundBinding) bool {
	leftStart, leftEnd := normalizedBindingPorts(left)
	rightStart, rightEnd := normalizedBindingPorts(right)
	return leftStart <= rightEnd && rightStart <= leftEnd
}

func normalizedBindingPorts(binding InboundBinding) (int, int) {
	start := binding.StartPort
	end := binding.EndPort
	if start == 0 {
		start = binding.Port
	}
	if end == 0 {
		end = start
	}
	return start, end
}

func protocolsOverlap(left Protocol, right Protocol) bool {
	if left == right {
		return true
	}
	return left == ProtocolTCPUDP && (right == ProtocolTCP || right == ProtocolUDP) ||
		right == ProtocolTCPUDP && (left == ProtocolTCP || left == ProtocolUDP)
}

func sameListenIPValue(left string, right string) bool {
	return normalizeListenIPValue(left) == normalizeListenIPValue(right)
}

func listenIPsOverlap(left string, right string) bool {
	left = normalizeListenIPValue(left)
	right = normalizeListenIPValue(right)
	if left == right {
		return true
	}
	return isWildcardListenIP(left) || isWildcardListenIP(right)
}

func normalizeListenIPValue(value string) string {
	return strings.Trim(strings.TrimSpace(value), "[]")
}

func isWildcardListenIP(value string) bool {
	if value == "" {
		return true
	}
	ip := net.ParseIP(value)
	return ip != nil && ip.IsUnspecified()
}
