package forward

import (
	"sort"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
)

func listenerBindApplyError(key listenerKey, rules []agent.RuleConfig, err error) agent.ConfigApplyError {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return agent.ConfigApplyError{
		Message: message,
		Errors: []agent.ConfigApplyErrorDetail{
			{
				Code:     "LISTENER_BIND_FAILED",
				RuleIDs:  ruleIDsForListener(rules),
				Protocol: key.protocol,
				ListenIP: key.listenIP,
				Port:     key.port,
				Message:  message,
			},
		},
	}
}

func ruleIDsForListener(rules []agent.RuleConfig) []string {
	ruleIDs := make([]string, 0, len(rules))
	for _, rule := range rules {
		if strings.TrimSpace(rule.ID) != "" {
			ruleIDs = append(ruleIDs, rule.ID)
		}
	}
	sort.Strings(ruleIDs)
	return ruleIDs
}
