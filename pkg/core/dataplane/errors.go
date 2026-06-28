package dataplane

import (
	"fmt"
	"sort"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
)

type listenerKey struct {
	protocol domain.Protocol
	listenIP string
	port     int
}

type plannedRule struct {
	rule      agent.RuleConfig
	dataplane string
}

func configApplyError(message string, details ...agent.ConfigApplyErrorDetail) agent.ConfigApplyError {
	return agent.ConfigApplyError{
		Message: strings.TrimSpace(message),
		Errors:  details,
	}
}

func unsupportedRuleError(rule agent.RuleConfig, dataplane string, message string) agent.ConfigApplyErrorDetail {
	return agent.ConfigApplyErrorDetail{
		Code:      ErrorDataplaneUnsupportedRule,
		RuleIDs:   []string{rule.ID},
		Protocol:  rule.Protocol,
		ListenIP:  rule.ListenIP,
		Port:      rule.Port,
		Dataplane: dataplane,
		Message:   message,
	}
}

func listenerError(code string, key listenerKey, rules []agent.RuleConfig, dataplane string, owner string, message string) agent.ConfigApplyErrorDetail {
	return agent.ConfigApplyErrorDetail{
		Code:      code,
		RuleIDs:   ruleIDs(rules),
		Protocol:  key.protocol,
		ListenIP:  key.listenIP,
		Port:      key.port,
		Dataplane: dataplane,
		Owner:     owner,
		Message:   message,
	}
}

func ruleIDs(rules []agent.RuleConfig) []string {
	values := make([]string, 0, len(rules))
	for _, rule := range rules {
		if strings.TrimSpace(rule.ID) != "" {
			values = append(values, rule.ID)
		}
	}
	sort.Strings(values)
	return values
}

func listenerKeyForRule(rule agent.RuleConfig) listenerKey {
	return listenerKeysForRule(rule)[0]
}

func listenerKeysForRule(rule agent.RuleConfig) []listenerKey {
	listenIP := normalizeListenIP(rule.ListenIP)
	if rule.Protocol == domain.ProtocolTCPUDP {
		return []listenerKey{
			{protocol: domain.ProtocolTCP, listenIP: listenIP, port: rule.Port},
			{protocol: domain.ProtocolUDP, listenIP: listenIP, port: rule.Port},
		}
	}
	return []listenerKey{{protocol: rule.Protocol, listenIP: listenIP, port: rule.Port}}
}

func listenerKeyString(key listenerKey) string {
	return fmt.Sprintf("%s/%s:%d", key.protocol, key.listenIP, key.port)
}

func normalizeListenIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0.0.0.0"
	}
	return value
}
