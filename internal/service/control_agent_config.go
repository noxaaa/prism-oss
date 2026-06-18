package service

import (
	"context"
	"sort"
	"strings"

	"github.com/noxaaa/prism-oss/internal/domain"
	"github.com/noxaaa/prism-oss/internal/repo"
)

func (service *ControlService) MarkNodeAgentConnected(ctx context.Context, organizationID string, nodeID string) error {
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		return repositories.Nodes().MarkNodeAgentConnected(ctx, organizationID, nodeID, service.timestamp())
	})
	return mapServiceError(err)
}

func (service *ControlService) MarkNodeAgentDisconnected(ctx context.Context, organizationID string, nodeID string) error {
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		return repositories.Nodes().MarkNodeAgentDisconnected(ctx, organizationID, nodeID, service.timestamp())
	})
	return mapServiceError(err)
}

func (service *ControlService) MarkMonitorAgentConnected(ctx context.Context, organizationID string, monitorID string) error {
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		return repositories.Monitors().MarkMonitorAgentConnected(ctx, organizationID, monitorID, service.timestamp())
	})
	return mapServiceError(err)
}

func (service *ControlService) MarkMonitorAgentDisconnected(ctx context.Context, organizationID string, monitorID string) error {
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		return repositories.Monitors().MarkMonitorAgentDisconnected(ctx, organizationID, monitorID, service.timestamp())
	})
	return mapServiceError(err)
}

func (service *ControlService) CompileNodeAgentConfig(ctx context.Context, organizationID string, nodeID string) (AgentConfig, error) {
	var result AgentConfig
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		node, err := repositories.Nodes().FindNodeByID(ctx, organizationID, nodeID)
		if err != nil {
			return err
		}
		rules, err := repositories.Rules().ListRulesByOrganization(ctx, organizationID)
		if err != nil {
			return err
		}
		targets, err := repositories.Targets().ListTargetsByOrganization(ctx, organizationID)
		if err != nil {
			return err
		}
		targetGroups, err := repositories.TargetGroups().ListTargetGroupsByOrganization(ctx, organizationID)
		if err != nil {
			return err
		}
		candidateRules := executableRulesForNode(node, rules)
		candidateConfigVersion := maxRuleRecordConfigVersion(candidateRules)
		ruleConfigs, err := toRuleConfigs(candidateRules, targets, targetGroups)
		if err != nil {
			return err
		}
		compiled, err := BasicAgentConfigCompiler{}.Compile(ctx, AgentConfigInput{
			NodeID:     node.ID,
			NodeGroups: node.GroupIDs,
			Rules:      ruleConfigs,
		})
		if err != nil {
			return err
		}
		targetDesiredConfigVersion := node.DesiredConfigVersion
		if compiled.ConfigVersion > targetDesiredConfigVersion {
			targetDesiredConfigVersion = compiled.ConfigVersion
		}
		if candidateConfigVersion > targetDesiredConfigVersion {
			targetDesiredConfigVersion = candidateConfigVersion
		}
		if targetDesiredConfigVersion > node.DesiredConfigVersion {
			if err := repositories.Nodes().EnsureDesiredConfigVersionAtLeast(ctx, organizationID, nodeID, targetDesiredConfigVersion, service.timestamp()); err != nil {
				return err
			}
		}
		if targetDesiredConfigVersion > compiled.ConfigVersion {
			compiled.ConfigVersion = targetDesiredConfigVersion
			compiled.ConfigHash = configHash(compiled)
		}
		result = compiled
		return nil
	})
	return result, mapServiceError(err)
}

func executableRulesForNode(node repo.NodeRecord, rules []repo.RuleRecord) []repo.RuleRecord {
	nodeGroups := make(map[string]struct{}, len(node.GroupIDs))
	for _, nodeGroupID := range node.GroupIDs {
		nodeGroups[nodeGroupID] = struct{}{}
	}
	filtered := make([]repo.RuleRecord, 0, len(rules))
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if _, ok := nodeGroups[rule.Binding.NodeGroupID]; !ok {
			continue
		}
		filtered = append(filtered, rule)
	}
	return filtered
}

func maxRuleRecordConfigVersion(rules []repo.RuleRecord) int {
	maxVersion := 0
	for _, rule := range rules {
		if rule.ConfigVersion > maxVersion {
			maxVersion = rule.ConfigVersion
		}
	}
	return maxVersion
}

func (service *ControlService) NodeAgentConfigBehind(ctx context.Context, organizationID string, nodeID string, appliedConfigVersion int) (bool, error) {
	var result bool
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		node, err := repositories.Nodes().FindNodeByID(ctx, organizationID, nodeID)
		if err != nil {
			return err
		}
		result = node.DesiredConfigVersion > appliedConfigVersion || node.ConfigStatus == "FAILED"
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) AcknowledgeNodeAgentConfig(ctx context.Context, organizationID string, nodeID string, configVersion int, status string, errorMessage string) error {
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "APPLIED" && status != "FAILED" {
		return ErrInvalidInput
	}
	errorMessage = strings.TrimSpace(errorMessage)
	if len(errorMessage) > 1000 {
		errorMessage = errorMessage[:1000]
	}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		return repositories.Nodes().RecordNodeConfigAck(ctx, organizationID, nodeID, configVersion, status, errorMessage, service.timestamp())
	})
	return mapServiceError(err)
}

func toRuleConfigs(rules []repo.RuleRecord, targets []repo.TargetRecord, targetGroups []repo.TargetGroupRecord) ([]RuleConfig, error) {
	targetsByID := make(map[string]repo.TargetRecord, len(targets))
	for _, target := range targets {
		targetsByID[target.ID] = target
	}
	targetGroupsByID := make(map[string]repo.TargetGroupRecord, len(targetGroups))
	for _, group := range targetGroups {
		targetGroupsByID[group.ID] = group
	}
	configs := make([]RuleConfig, 0, len(rules))
	for _, rule := range rules {
		if !openCoreRuleRecordSupported(rule) {
			continue
		}
		config := RuleConfig{
			ID:               rule.ID,
			ConfigVersion:    rule.ConfigVersion,
			Enabled:          rule.Enabled,
			ForwardingType:   domain.ForwardingType(defaultForwardingType(rule.ForwardingType)),
			Protocol:         domain.Protocol(rule.Protocol),
			NodeGroupIDs:     []string{rule.Binding.NodeGroupID},
			ListenIP:         rule.Binding.ListenIP,
			Port:             rule.Binding.Port,
			MatchType:        rule.MatchType,
			SNIHostname:      rule.SNIHostname,
			ProxyProtocolIn:  rule.ProxyProtocolIn,
			ProxyProtocolOut: rule.ProxyProtocolOut,
		}
		switch rule.TargetType {
		case "TARGET":
			if target, ok := targetsByID[rule.TargetID]; ok {
				endpoint := toTargetEndpoint(target)
				config.Upstream = RuleUpstreamConfig{Type: "TARGET", Target: &endpoint}
			}
		case "TARGET_GROUP":
			if group, ok := targetGroupsByID[rule.TargetGroupID]; ok {
				if group.Scheduler != "PRIORITY_IPHASH" {
					continue
				}
				config.Upstream = RuleUpstreamConfig{Type: "TARGET_GROUP", TargetGroup: toTargetPriorityBuckets(group, targetsByID)}
			}
		}
		configs = append(configs, config)
	}
	return configs, nil
}

func openCoreRuleRecordSupported(rule repo.RuleRecord) bool {
	if defaultForwardingType(rule.ForwardingType) != string(domain.ForwardingTypeDirect) {
		return false
	}
	switch domain.MatchType(rule.MatchType) {
	case domain.MatchTypeAnyInbound, domain.MatchTypeTLSSNI:
		return true
	default:
		return false
	}
}

func toTargetEndpoint(target repo.TargetRecord) TargetEndpoint {
	return TargetEndpoint{
		ID:      target.ID,
		Host:    target.Host,
		Port:    target.Port,
		Enabled: target.Enabled,
	}
}

func toTargetPriorityBuckets(group repo.TargetGroupRecord, targetsByID map[string]repo.TargetRecord) []TargetPriorityBucket {
	bucketsByPriority := make(map[int][]TargetEndpoint)
	for _, member := range group.Members {
		target, ok := targetsByID[member.TargetID]
		if !ok {
			continue
		}
		endpoint := toTargetEndpoint(target)
		endpoint.Enabled = endpoint.Enabled && member.Enabled
		bucketsByPriority[member.Priority] = append(bucketsByPriority[member.Priority], endpoint)
	}
	priorities := make([]int, 0, len(bucketsByPriority))
	for priority := range bucketsByPriority {
		priorities = append(priorities, priority)
	}
	sort.Ints(priorities)
	buckets := make([]TargetPriorityBucket, 0, len(priorities))
	for _, priority := range priorities {
		buckets = append(buckets, TargetPriorityBucket{Priority: priority, Targets: bucketsByPriority[priority]})
	}
	return buckets
}
