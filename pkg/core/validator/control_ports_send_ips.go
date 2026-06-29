package validator

import (
	"net"
	"sort"
	"strings"
)

func validateSendIPs(values []NodeSendIP) ([]NodeSendIP, error) {
	seen := make(map[string]bool)
	normalized := make([]NodeSendIP, 0, len(values))
	for _, value := range values {
		value.SendIP = strings.TrimSpace(value.SendIP)
		value.DisplayName = strings.TrimSpace(value.DisplayName)
		if value.SendIP == "" {
			continue
		}
		if net.ParseIP(value.SendIP) == nil {
			return nil, invalidFieldError("send_ips", "Send IP must be a valid IP address.", map[string]any{"actual": value.SendIP})
		}
		if len(value.DisplayName) > 120 {
			return nil, invalidFieldError("send_ips", "Send IP label must be at most 120 characters.", nil)
		}
		if seen[value.SendIP] {
			return nil, invalidFieldError("send_ips", "Send IP entries must be unique.", map[string]any{"actual": value.SendIP})
		}
		if value.DisplayName == "" {
			value.DisplayName = value.SendIP
		}
		seen[value.SendIP] = true
		normalized = append(normalized, value)
	}
	return normalized, nil
}

func validateMaxRulePorts(value int) (int, error) {
	if value == 0 {
		return 256, nil
	}
	if value < 1 || value > 65535 {
		return 0, invalidFieldError("max_rule_ports", "Maximum rule ports must be between 1 and 65535.", map[string]any{"actual": value, "min": 1, "max": 65535})
	}
	return value, nil
}

func validateRulePortSegments(port int, values []RulePortSegmentRequest) ([]RulePortSegmentRequest, error) {
	if len(values) == 0 {
		if port < 1 || port > 65535 {
			return nil, invalidFieldError("port", "Rule port must be between 1 and 65535.", map[string]any{"actual": port, "min": 1, "max": 65535})
		}
		return []RulePortSegmentRequest{{StartPort: port, EndPort: port}}, nil
	}
	segments := make([]RulePortSegmentRequest, 0, len(values))
	for _, value := range values {
		if value.EndPort == 0 {
			value.EndPort = value.StartPort
		}
		if value.StartPort < 1 || value.StartPort > 65535 || value.EndPort < 1 || value.EndPort > 65535 || value.StartPort > value.EndPort {
			return nil, invalidFieldError("port_segments", "Rule port segments must be within 1-65535 and start_port must be <= end_port.", map[string]any{"start_port": value.StartPort, "end_port": value.EndPort})
		}
		segments = append(segments, value)
	}
	sort.Slice(segments, func(i int, j int) bool {
		if segments[i].StartPort == segments[j].StartPort {
			return segments[i].EndPort < segments[j].EndPort
		}
		return segments[i].StartPort < segments[j].StartPort
	})
	merged := make([]RulePortSegmentRequest, 0, len(segments))
	for _, segment := range segments {
		if len(merged) == 0 || segment.StartPort > merged[len(merged)-1].EndPort+1 {
			merged = append(merged, segment)
			continue
		}
		if segment.EndPort > merged[len(merged)-1].EndPort {
			merged[len(merged)-1].EndPort = segment.EndPort
		}
	}
	if port != 0 && (len(merged) != 1 || merged[0].StartPort != port || merged[0].EndPort != port) {
		return nil, invalidFieldError("port", "Rule port must be omitted when using port_segments.", map[string]any{"port": port})
	}
	return merged, nil
}
