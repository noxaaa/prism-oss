package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestBasicAgentConfigCompilerIncludesOnlyEnabledRulesForNodeGroup(t *testing.T) {
	compiler := BasicAgentConfigCompiler{}
	input := AgentConfigInput{
		NodeID:     "node_1",
		NodeGroups: []string{"ng_01"},
		Rules: []RuleConfig{
			{
				ID:        "rule_enabled",
				Enabled:   true,
				Protocol:  domain.ProtocolTCP,
				MatchType: string(domain.MatchTypeAnyInbound),
				NodeGroupIDs: []string{
					"ng_01",
				},
			},
			{
				ID:        "rule_other_group",
				Enabled:   true,
				Protocol:  domain.ProtocolTCP,
				MatchType: string(domain.MatchTypeAnyInbound),
				NodeGroupIDs: []string{
					"ng_02",
				},
			},
			{
				ID:        "rule_disabled",
				Enabled:   false,
				Protocol:  domain.ProtocolTCP,
				MatchType: string(domain.MatchTypeAnyInbound),
				NodeGroupIDs: []string{
					"ng_01",
				},
			},
		},
	}

	config, err := compiler.Compile(context.Background(), input)
	if err != nil {
		t.Fatalf("compile config: %v", err)
	}
	if config.AgentProtocolVersion.Major != agent.ProtocolMajor {
		t.Fatalf("expected protocol major %d, got %d", agent.ProtocolMajor, config.AgentProtocolVersion.Major)
	}
	if len(config.Rules) != 1 {
		t.Fatalf("expected one compiled rule, got %d", len(config.Rules))
	}
	if config.Rules[0].ID != "rule_enabled" {
		t.Fatalf("expected enabled rule, got %s", config.Rules[0].ID)
	}
}

func TestBasicAgentConfigCompilerIncludesDirectNodeScopedRules(t *testing.T) {
	compiler := BasicAgentConfigCompiler{}
	input := AgentConfigInput{
		NodeID:     "node_1",
		NodeGroups: []string{"ng_01"},
		Rules: []RuleConfig{
			{ID: "rule_direct_node", Enabled: true, NodeIDs: []string{"node_1"}, MatchType: string(domain.MatchTypeAnyInbound)},
			{ID: "rule_other_node", Enabled: true, NodeIDs: []string{"node_2"}, MatchType: string(domain.MatchTypeAnyInbound)},
			{ID: "rule_group", Enabled: true, NodeGroupIDs: []string{"ng_01"}, MatchType: string(domain.MatchTypeAnyInbound)},
		},
	}

	config, err := compiler.Compile(context.Background(), input)
	if err != nil {
		t.Fatalf("compile config: %v", err)
	}

	if len(config.Rules) != 2 {
		t.Fatalf("expected direct node and group rules, got %d", len(config.Rules))
	}
	if config.Rules[0].ID != "rule_direct_node" || config.Rules[1].ID != "rule_group" {
		t.Fatalf("expected sorted direct node and group rules, got %+v", config.Rules)
	}
}

func TestBasicAgentConfigCompilerIncrementsStableHashWhenRulesChange(t *testing.T) {
	compiler := BasicAgentConfigCompiler{}
	first, err := compiler.Compile(context.Background(), AgentConfigInput{
		NodeID:     "node_1",
		NodeGroups: []string{"ng_01"},
		Rules: []RuleConfig{
			{ID: "rule_a", Enabled: true, NodeGroupIDs: []string{"ng_01"}, MatchType: string(domain.MatchTypeAnyInbound)},
		},
	})
	if err != nil {
		t.Fatalf("compile first: %v", err)
	}
	second, err := compiler.Compile(context.Background(), AgentConfigInput{
		NodeID:     "node_1",
		NodeGroups: []string{"ng_01"},
		Rules: []RuleConfig{
			{ID: "rule_b", Enabled: true, NodeGroupIDs: []string{"ng_01"}, MatchType: string(domain.MatchTypeAnyInbound)},
		},
	})
	if err != nil {
		t.Fatalf("compile second: %v", err)
	}
	if first.ConfigHash == second.ConfigHash {
		t.Fatalf("expected hash to change when rule set changes")
	}
}

func TestBasicAgentConfigCompilerEmitsExecutableRuleWithTargetGroupBuckets(t *testing.T) {
	compiler := BasicAgentConfigCompiler{}
	config, err := compiler.Compile(context.Background(), AgentConfigInput{
		NodeID:     "node_1",
		NodeGroups: []string{"ng_01"},
		Rules: []RuleConfig{
			{
				ID:               "rule_tls",
				ConfigVersion:    7,
				Enabled:          true,
				Protocol:         domain.ProtocolTCP,
				NodeGroupIDs:     []string{"ng_01"},
				ListenIP:         "127.0.0.1",
				Port:             443,
				MatchType:        "TLS_SNI",
				SNIHostname:      "app.example.com",
				ProxyProtocolIn:  "V1",
				ProxyProtocolOut: "V2",
				Upstream: RuleUpstreamConfig{
					Type: "TARGET_GROUP",
					TargetGroup: []TargetPriorityBucket{
						{
							Priority: 10,
							Targets: []TargetEndpoint{
								{ID: "target_a", Host: "10.0.0.1", Port: 8443, Enabled: true},
								{ID: "target_b", Host: "10.0.0.2", Port: 8443, Enabled: true},
							},
						},
						{
							Priority: 20,
							Targets: []TargetEndpoint{
								{ID: "target_backup", Host: "10.0.0.3", Port: 8443, Enabled: true},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("compile config: %v", err)
	}
	if config.ConfigVersion != 7 {
		t.Fatalf("expected config version 7, got %d", config.ConfigVersion)
	}
	if len(config.Rules) != 1 {
		t.Fatalf("expected one executable rule, got %d", len(config.Rules))
	}
	rule := config.Rules[0]
	if rule.ListenIP != "127.0.0.1" || rule.Port != 443 || rule.MatchType != "TLS_SNI" || rule.SNIHostname != "app.example.com" {
		t.Fatalf("compiled rule lost listener/match fields: %#v", rule)
	}
	if rule.ProxyProtocolIn != "V1" || rule.ProxyProtocolOut != "V2" {
		t.Fatalf("compiled rule lost proxy protocol fields: %#v", rule)
	}
	if rule.Upstream.Type != "TARGET_GROUP" || len(rule.Upstream.TargetGroup) != 2 {
		t.Fatalf("expected target group buckets, got %#v", rule.Upstream)
	}
	if rule.Upstream.TargetGroup[0].Priority != 10 || len(rule.Upstream.TargetGroup[0].Targets) != 2 {
		t.Fatalf("expected first priority bucket with two targets, got %#v", rule.Upstream.TargetGroup[0])
	}
	compiledRuleJSON, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("marshal compiled rule: %v", err)
	}
	var compiledRule map[string]any
	if err := json.Unmarshal(compiledRuleJSON, &compiledRule); err != nil {
		t.Fatalf("decode compiled rule json: %v", err)
	}
	if compiledRule["forwarding_type"] != "DIRECT" {
		t.Fatalf("expected compiled rule forwarding_type DIRECT, got %#v", compiledRule)
	}
}

func TestAgentConfigHashIncludesProtocolVersion(t *testing.T) {
	base := AgentConfig{
		AgentProtocolVersion: agent.ProtocolVersion{Major: 1, Minor: 0},
		NodeID:               "node_1",
		Rules: []RuleConfig{
			{ID: "rule_a", Enabled: true, NodeGroupIDs: []string{"ng_01"}},
		},
	}
	upgraded := base
	upgraded.AgentProtocolVersion = agent.ProtocolVersion{Major: 1, Minor: 1}

	if configHash(base) == configHash(upgraded) {
		t.Fatalf("expected hash to change when agent protocol version changes")
	}
}

func TestBasicAgentConfigCompilerSkipsUnsupportedForwardingTypeForClosingSnapshot(t *testing.T) {
	compiler := BasicAgentConfigCompiler{}
	config, err := compiler.Compile(context.Background(), AgentConfigInput{
		NodeID:     "node_1",
		NodeGroups: []string{"ng_01"},
		Rules: []RuleConfig{
			{
				ID:             "rule_tunnel",
				ConfigVersion:  12,
				Enabled:        true,
				ForwardingType: domain.ForwardingTypeTunnel,
				NodeGroupIDs:   []string{"ng_01"},
				MatchType:      "ANY_INBOUND",
			},
		},
	})
	if err != nil {
		t.Fatalf("compile config: %v", err)
	}
	if len(config.Rules) != 0 || config.ConfigVersion != 12 {
		t.Fatalf("expected unsupported rule to be removed while retaining closing version, got %#v", config)
	}
}

func TestBasicAgentConfigCompilerSkipsFeatureMatchTypeForClosingSnapshot(t *testing.T) {
	compiler := BasicAgentConfigCompiler{}
	config, err := compiler.Compile(context.Background(), AgentConfigInput{
		NodeID:     "node_1",
		NodeGroups: []string{"ng_01"},
		Rules: []RuleConfig{
			{
				ID:            "rule_feature",
				ConfigVersion: 13,
				Enabled:       true,
				NodeGroupIDs:  []string{"ng_01"},
				MatchType:     string(domain.MatchTypeFeature),
			},
		},
	})
	if err != nil {
		t.Fatalf("compile config: %v", err)
	}
	if len(config.Rules) != 0 || config.ConfigVersion != 13 {
		t.Fatalf("expected unsupported rule to be removed while retaining closing version, got %#v", config)
	}
}

func TestBasicAgentConfigCompilerSkipsUnknownMatchTypeForClosingSnapshot(t *testing.T) {
	compiler := BasicAgentConfigCompiler{}
	config, err := compiler.Compile(context.Background(), AgentConfigInput{
		NodeID:     "node_1",
		NodeGroups: []string{"ng_01"},
		Rules: []RuleConfig{
			{
				ID:            "rule_commercial_match",
				ConfigVersion: 14,
				Enabled:       true,
				NodeGroupIDs:  []string{"ng_01"},
				MatchType:     "COMMERCIAL_MATCH",
			},
		},
	})
	if err != nil {
		t.Fatalf("compile config: %v", err)
	}
	if len(config.Rules) != 0 || config.ConfigVersion != 14 {
		t.Fatalf("expected unsupported rule to be removed while retaining closing version, got %#v", config)
	}
}

func TestBasicAgentConfigCompilerRejectsBlankMatchType(t *testing.T) {
	compiler := BasicAgentConfigCompiler{}
	_, err := compiler.Compile(context.Background(), AgentConfigInput{
		NodeID:     "node_1",
		NodeGroups: []string{"ng_01"},
		Rules: []RuleConfig{
			{
				ID:           "rule_blank_match",
				Enabled:      true,
				NodeGroupIDs: []string{"ng_01"},
			},
		},
	})
	if err != ErrInvalidInput {
		t.Fatalf("expected blank match type to fail closed, got %v", err)
	}
}

func TestRuleConfigConversionSkipsUnknownTargetGroupScheduler(t *testing.T) {
	configs, err := toRuleConfigs(
		[]repo.RuleRecord{
			{
				ID:             "rule_group",
				Enabled:        true,
				ForwardingType: string(domain.ForwardingTypeDirect),
				MatchType:      string(domain.MatchTypeAnyInbound),
				TargetType:     "TARGET_GROUP",
				TargetGroupID:  "target_group_custom",
				Binding: repo.InboundBindingRecord{
					NodeGroupID: "ng_01",
					Protocol:    string(domain.ProtocolTCP),
					MatchType:   string(domain.MatchTypeAnyInbound),
				},
			},
		},
		nil,
		[]repo.TargetGroupRecord{
			{ID: "target_group_custom", Scheduler: "COMMERCIAL_CUSTOM"},
		},
	)
	if err != nil {
		t.Fatalf("convert configs: %v", err)
	}
	if len(configs) != 0 {
		t.Fatalf("expected unknown scheduler to be skipped, got %#v", configs)
	}
}

func TestRuleConfigConversionSkipsUnsupportedCommercialRules(t *testing.T) {
	configs, err := toRuleConfigs(
		[]repo.RuleRecord{
			{
				ID:             "rule_tunnel",
				ConfigVersion:  9,
				Enabled:        true,
				ForwardingType: string(domain.ForwardingTypeTunnel),
				TargetType:     "TARGET",
				TargetID:       "target_a",
				Binding: repo.InboundBindingRecord{
					NodeGroupID: "ng_01",
					Protocol:    string(domain.ProtocolTCP),
					MatchType:   string(domain.MatchTypeAnyInbound),
				},
			},
			{
				ID:             "rule_feature",
				ConfigVersion:  10,
				Enabled:        true,
				ForwardingType: string(domain.ForwardingTypeDirect),
				MatchType:      string(domain.MatchTypeFeature),
				TargetType:     "TARGET",
				TargetID:       "target_a",
				Binding: repo.InboundBindingRecord{
					NodeGroupID: "ng_01",
					Protocol:    string(domain.ProtocolTCP),
					MatchType:   string(domain.MatchTypeFeature),
				},
			},
			{
				ID:             "rule_blank_match",
				ConfigVersion:  11,
				Enabled:        true,
				ForwardingType: string(domain.ForwardingTypeDirect),
				TargetType:     "TARGET",
				TargetID:       "target_a",
				Binding: repo.InboundBindingRecord{
					NodeGroupID: "ng_01",
					Protocol:    string(domain.ProtocolTCP),
				},
			},
			{
				ID:             "rule_direct",
				ConfigVersion:  3,
				Enabled:        true,
				ForwardingType: string(domain.ForwardingTypeDirect),
				MatchType:      string(domain.MatchTypeAnyInbound),
				TargetType:     "TARGET",
				TargetID:       "target_a",
				Binding: repo.InboundBindingRecord{
					NodeGroupID: "ng_01",
					Protocol:    string(domain.ProtocolTCP),
					MatchType:   string(domain.MatchTypeAnyInbound),
				},
			},
		},
		[]repo.TargetRecord{{ID: "target_a", Host: "127.0.0.1", Port: 8080, Enabled: true}},
		nil,
	)
	if err != nil {
		t.Fatalf("convert configs: %v", err)
	}
	if len(configs) != 1 || configs[0].ID != "rule_direct" {
		t.Fatalf("expected only core direct rule to be converted, got %#v", configs)
	}
}

func TestMaxRuleRecordConfigVersionIncludesSkippedUnsupportedRules(t *testing.T) {
	version := maxRuleRecordConfigVersion([]repo.RuleRecord{
		{ID: "rule_direct", ConfigVersion: 3},
		{ID: "rule_commercial", ConfigVersion: 11},
	})
	if version != 11 {
		t.Fatalf("expected skipped unsupported rule version to be retained for closing snapshots, got %d", version)
	}
}

func TestRuleConfigConversionSkipsUnsupportedSchedulersOutsideExecutableRules(t *testing.T) {
	rules := executableRulesForNode(
		repo.NodeRecord{ID: "node_1", GroupIDs: []string{"ng_01"}},
		[]repo.RuleRecord{
			{
				ID:             "rule_disabled_commercial_group",
				Enabled:        false,
				ForwardingType: string(domain.ForwardingTypeDirect),
				MatchType:      string(domain.MatchTypeAnyInbound),
				TargetType:     "TARGET_GROUP",
				TargetGroupID:  "target_group_custom",
				Binding: repo.InboundBindingRecord{
					NodeGroupID: "ng_01",
					Protocol:    string(domain.ProtocolTCP),
					MatchType:   string(domain.MatchTypeAnyInbound),
				},
			},
			{
				ID:             "rule_unrelated_commercial_group",
				Enabled:        true,
				ForwardingType: string(domain.ForwardingTypeDirect),
				MatchType:      string(domain.MatchTypeAnyInbound),
				TargetType:     "TARGET_GROUP",
				TargetGroupID:  "target_group_custom",
				Binding: repo.InboundBindingRecord{
					NodeGroupID: "ng_02",
					Protocol:    string(domain.ProtocolTCP),
					MatchType:   string(domain.MatchTypeAnyInbound),
				},
			},
			{
				ID:             "rule_current_node",
				Enabled:        true,
				ForwardingType: string(domain.ForwardingTypeDirect),
				MatchType:      string(domain.MatchTypeAnyInbound),
				TargetType:     "TARGET",
				TargetID:       "target_a",
				Binding: repo.InboundBindingRecord{
					NodeGroupID: "ng_01",
					Protocol:    string(domain.ProtocolTCP),
					MatchType:   string(domain.MatchTypeAnyInbound),
				},
			},
		},
	)
	configs, err := toRuleConfigs(
		rules,
		[]repo.TargetRecord{{ID: "target_a", Host: "127.0.0.1", Port: 8080, Enabled: true}},
		[]repo.TargetGroupRecord{{ID: "target_group_custom", Scheduler: "COMMERCIAL_CUSTOM"}},
	)
	if err != nil {
		t.Fatalf("expected unrelated unsupported scheduler to be skipped before conversion: %v", err)
	}
	if len(configs) != 1 || configs[0].ID != "rule_current_node" {
		t.Fatalf("expected only current node rule to be converted, got %#v", configs)
	}
}
