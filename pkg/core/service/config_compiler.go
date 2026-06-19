package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
)

type AgentConfigCompiler interface {
	Compile(ctx context.Context, input AgentConfigInput) (AgentConfig, error)
}

type BasicAgentConfigCompiler struct{}

type AgentConfigInput struct {
	NodeID     string
	NodeGroups []string
	Rules      []RuleConfig
}

type RuleConfig = agent.RuleConfig
type AgentConfig = agent.ConfigSnapshot
type RuleUpstreamConfig = agent.RuleUpstreamConfig
type TargetPriorityBucket = agent.TargetPriorityBucket
type TargetEndpoint = agent.TargetEndpoint

func EmptyNodeAgentConfig(nodeID string) AgentConfig {
	config := AgentConfig{
		AgentProtocolVersion: agent.CurrentProtocolVersion(),
		NodeID:               nodeID,
		ConfigVersion:        0,
		Rules:                []RuleConfig{},
	}
	config.ConfigHash = configHash(config)
	return config
}

func (compiler BasicAgentConfigCompiler) Compile(ctx context.Context, input AgentConfigInput) (AgentConfig, error) {
	rules, configVersion, err := matchingEnabledRules(input.NodeID, input.NodeGroups, input.Rules)
	if err != nil {
		return AgentConfig{}, err
	}
	compiled := AgentConfig{
		AgentProtocolVersion: agent.CurrentProtocolVersion(),
		NodeID:               input.NodeID,
		ConfigVersion:        configVersion,
		Rules:                rules,
	}
	compiled.ConfigHash = configHash(compiled)
	return compiled, nil
}

func matchingEnabledRules(nodeID string, nodeGroups []string, rules []RuleConfig) ([]RuleConfig, int, error) {
	nodeGroupSet := make(map[string]struct{}, len(nodeGroups))
	for _, nodeGroupID := range nodeGroups {
		nodeGroupSet[nodeGroupID] = struct{}{}
	}

	matches := make([]RuleConfig, 0)
	configVersion := 0
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if ruleMatchesNode(rule, nodeID, nodeGroupSet) {
			if rule.ConfigVersion > configVersion {
				configVersion = rule.ConfigVersion
			}
			supported, err := openCoreExecutableRuleSupported(rule)
			if err != nil {
				return nil, 0, err
			}
			if !supported {
				continue
			}
			matches = append(matches, normalizedRule(rule))
		}
	}
	sort.SliceStable(matches, func(i int, j int) bool {
		return matches[i].ID < matches[j].ID
	})
	return matches, configVersion, nil
}

func openCoreExecutableRuleSupported(rule RuleConfig) (bool, error) {
	forwardingType := rule.ForwardingType
	if forwardingType == "" {
		forwardingType = domain.ForwardingTypeDirect
	}
	if forwardingType != domain.ForwardingTypeDirect {
		return false, nil
	}
	if strings.TrimSpace(rule.MatchType) == "" {
		return false, ErrInvalidInput
	}
	switch domain.MatchType(rule.MatchType) {
	case domain.MatchTypeAnyInbound, domain.MatchTypeTLSSNI:
		return true, nil
	case domain.MatchTypeFeature:
		return false, nil
	default:
		return false, nil
	}
}

func ruleMatchesNode(rule RuleConfig, nodeID string, nodeGroupSet map[string]struct{}) bool {
	for _, ruleNodeID := range rule.NodeIDs {
		if ruleNodeID == nodeID {
			return true
		}
	}
	return ruleMatchesAnyNodeGroup(rule, nodeGroupSet)
}

func ruleMatchesAnyNodeGroup(rule RuleConfig, nodeGroupSet map[string]struct{}) bool {
	for _, nodeGroupID := range rule.NodeGroupIDs {
		if _, ok := nodeGroupSet[nodeGroupID]; ok {
			return true
		}
	}
	return false
}

func normalizedRule(rule RuleConfig) RuleConfig {
	out := rule
	if out.ForwardingType == "" {
		out.ForwardingType = domain.ForwardingTypeDirect
	}
	out.NodeIDs = append([]string(nil), rule.NodeIDs...)
	out.NodeGroupIDs = append([]string(nil), rule.NodeGroupIDs...)
	sort.Strings(out.NodeIDs)
	sort.Strings(out.NodeGroupIDs)
	out.Upstream = normalizedUpstream(rule.Upstream)
	return out
}

func normalizedUpstream(upstream RuleUpstreamConfig) RuleUpstreamConfig {
	out := upstream
	if upstream.Target != nil {
		target := *upstream.Target
		out.Target = &target
	}
	out.TargetGroup = append([]TargetPriorityBucket(nil), upstream.TargetGroup...)
	for index := range out.TargetGroup {
		out.TargetGroup[index].Targets = append([]TargetEndpoint(nil), out.TargetGroup[index].Targets...)
		sort.SliceStable(out.TargetGroup[index].Targets, func(i int, j int) bool {
			return out.TargetGroup[index].Targets[i].ID < out.TargetGroup[index].Targets[j].ID
		})
	}
	sort.SliceStable(out.TargetGroup, func(i int, j int) bool {
		if out.TargetGroup[i].Priority == out.TargetGroup[j].Priority {
			if len(out.TargetGroup[i].Targets) == 0 || len(out.TargetGroup[j].Targets) == 0 {
				return len(out.TargetGroup[i].Targets) < len(out.TargetGroup[j].Targets)
			}
			return out.TargetGroup[i].Targets[0].ID < out.TargetGroup[j].Targets[0].ID
		}
		return out.TargetGroup[i].Priority < out.TargetGroup[j].Priority
	})
	return out
}

func configHash(config AgentConfig) string {
	hashInput := struct {
		AgentProtocolVersion agent.ProtocolVersion `json:"agent_protocol_version"`
		NodeID               string                `json:"node_id"`
		ConfigVersion        int                   `json:"config_version"`
		Rules                []RuleConfig          `json:"rules"`
	}{
		AgentProtocolVersion: config.AgentProtocolVersion,
		NodeID:               config.NodeID,
		ConfigVersion:        config.ConfigVersion,
		Rules:                config.Rules,
	}
	payload, _ := json.Marshal(hashInput)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
