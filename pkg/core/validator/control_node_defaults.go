package validator

import (
	"net"
	"strconv"
	"strings"
)

func validateListenIPs(values []NodeListenIP) ([]NodeListenIP, error) {
	seen := make(map[string]bool)
	normalized := make([]NodeListenIP, 0, len(values))
	for _, value := range values {
		value.ListenIP = strings.TrimSpace(value.ListenIP)
		value.DisplayName = strings.TrimSpace(value.DisplayName)
		if value.ListenIP == "" {
			continue
		}
		if net.ParseIP(value.ListenIP) == nil {
			return nil, invalidFieldError("listen_ips", "Listen IP must be a valid IP address.", map[string]any{"actual": value.ListenIP})
		}
		if len(value.DisplayName) > 120 {
			return nil, invalidFieldError("listen_ips", "Listen IP label must be at most 120 characters.", nil)
		}
		if seen[value.ListenIP] {
			return nil, invalidFieldError("listen_ips", "Listen IP entries must be unique.", map[string]any{"actual": value.ListenIP})
		}
		if value.DisplayName == "" {
			if value.ListenIP == "0.0.0.0" {
				value.DisplayName = "default"
			} else {
				value.DisplayName = value.ListenIP
			}
		}
		seen[value.ListenIP] = true
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return []NodeListenIP{{ListenIP: "0.0.0.0", DisplayName: "default"}}, nil
	}
	return normalized, nil
}

func validatePortRanges(values []NodePortRange) ([]NodePortRange, error) {
	normalized := make([]NodePortRange, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		value.Protocol = strings.ToUpper(strings.TrimSpace(value.Protocol))
		if value.Protocol == "" {
			value.Protocol = "TCP"
		}
		if value.StartPort == 0 {
			value.StartPort = 10000
		}
		if value.EndPort == 0 {
			value.EndPort = 20000
		}
		if value.Protocol != "TCP" && value.Protocol != "UDP" {
			return nil, invalidFieldError("port_ranges", "Port range protocol must be TCP or UDP.", map[string]any{"actual": value.Protocol})
		}
		if value.StartPort < 1 || value.StartPort > 65535 || value.EndPort < 1 || value.EndPort > 65535 || value.StartPort > value.EndPort {
			return nil, invalidFieldError("port_ranges", "Port ranges must be within 1-65535 and start_port must be <= end_port.", map[string]any{"start_port": value.StartPort, "end_port": value.EndPort, "min": 1, "max": 65535})
		}
		key := value.Protocol + ":" + strconv.Itoa(value.StartPort) + ":" + strconv.Itoa(value.EndPort)
		if seen[key] {
			return nil, invalidFieldError("port_ranges", "Port range entries must be unique.", map[string]any{"protocol": value.Protocol, "start_port": value.StartPort, "end_port": value.EndPort})
		}
		seen[key] = true
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return []NodePortRange{{Protocol: "TCP", StartPort: 10000, EndPort: 20000}}, nil
	}
	return normalized, nil
}
