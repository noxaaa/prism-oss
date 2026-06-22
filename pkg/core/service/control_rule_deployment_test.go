package service

import (
	"context"
	"testing"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestNodeAgentConfigBehindHonorsFailedVersionBackoff(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	store := newRuleDeploymentTestStore()
	store.nodes["node_1"] = repo.NodeRecord{
		ID:                        "node_1",
		OrganizationID:            "org_1",
		DesiredConfigVersion:      5,
		AppliedConfigVersion:      4,
		ConfigStatus:              "FAILED",
		ConfigStatusConfigVersion: 5,
		ConfigRetryCount:          1,
		ConfigNextRetryAt:         now.Add(30 * time.Second).Format(time.RFC3339Nano),
	}
	service := NewControlService(store)
	service.now = func() time.Time { return now }

	behind, err := service.NodeAgentConfigBehind(context.Background(), "org_1", "node_1", 4)
	if err != nil {
		t.Fatalf("check config behind during backoff: %v", err)
	}
	if behind {
		t.Fatalf("expected failed version to wait until config_next_retry_at")
	}

	service.now = func() time.Time { return now.Add(31 * time.Second) }
	behind, err = service.NodeAgentConfigBehind(context.Background(), "org_1", "node_1", 4)
	if err != nil {
		t.Fatalf("check config behind after backoff: %v", err)
	}
	if !behind {
		t.Fatalf("expected retry after config_next_retry_at")
	}
}

func TestNodeAgentConfigBehindHonorsPostgresRetryTimestamp(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	store := newRuleDeploymentTestStore()
	store.nodes["node_1"] = repo.NodeRecord{
		ID:                        "node_1",
		OrganizationID:            "org_1",
		DesiredConfigVersion:      5,
		AppliedConfigVersion:      4,
		ConfigStatus:              "FAILED",
		ConfigStatusConfigVersion: 5,
		ConfigRetryCount:          1,
		ConfigNextRetryAt:         "2026-06-21 12:00:30+00",
	}
	service := NewControlService(store)
	service.now = func() time.Time { return now }

	behind, err := service.NodeAgentConfigBehind(context.Background(), "org_1", "node_1", 4)
	if err != nil {
		t.Fatalf("check config behind during postgres timestamp backoff: %v", err)
	}
	if behind {
		t.Fatalf("expected PostgreSQL timestamptz text to honor config backoff")
	}
}

func TestAcknowledgeNodeAgentConfigPersistsRuleFailureAndBackoff(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	store := newRuleDeploymentTestStore()
	store.nodes["node_1"] = repo.NodeRecord{ID: "node_1", OrganizationID: "org_1", DesiredConfigVersion: 5, AppliedConfigVersion: 4}
	store.rules["rule_1"] = ruleDeploymentTestRule("rule_1", "group_1", FailurePolicyKeepEnabled)
	service := NewControlService(store)
	service.now = func() time.Time { return now }

	err := service.AcknowledgeNodeAgentConfig(context.Background(), "org_1", "node_1", 5, "FAILED", "listen failed", []ConfigApplyErrorInput{
		{
			Code:     "LISTENER_BIND_FAILED",
			RuleIDs:  []string{"rule_1"},
			Protocol: domain.ProtocolTCP,
			ListenIP: "0.0.0.0",
			Port:     443,
			Message:  "address already in use",
		},
	})
	if err != nil {
		t.Fatalf("ack failed config: %v", err)
	}

	node := store.nodes["node_1"]
	if node.ConfigStatus != "FAILED" || node.ConfigStatusConfigVersion != 5 || node.ConfigRetryCount != 1 {
		t.Fatalf("expected node failed status with retry count, got %#v", node)
	}
	if node.ConfigNextRetryAt != now.Add(15*time.Second).Format(time.RFC3339Nano) {
		t.Fatalf("expected first retry at +15s, got %q", node.ConfigNextRetryAt)
	}
	deployment := store.deployments["rule_1|node_1"]
	if deployment.Status != RuleDeploymentStatusFailed || deployment.ErrorCode != "LISTENER_BIND_FAILED" || deployment.Port != 443 {
		t.Fatalf("expected persisted rule deployment failure, got %#v", deployment)
	}
	if !store.rules["rule_1"].Enabled {
		t.Fatalf("KEEP_ENABLED policy must not disable failed rule")
	}
}

func TestAcknowledgeNodeAgentConfigDisablesRuleOnlyWhenEveryCurrentNodeFailed(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	store := newRuleDeploymentTestStore()
	store.nodes["node_a"] = repo.NodeRecord{ID: "node_a", OrganizationID: "org_1", DesiredConfigVersion: 7, AppliedConfigVersion: 6, GroupIDs: []string{"group_1"}}
	store.nodes["node_b"] = repo.NodeRecord{ID: "node_b", OrganizationID: "org_1", DesiredConfigVersion: 7, AppliedConfigVersion: 6, GroupIDs: []string{"group_1"}}
	store.rules["rule_1"] = ruleDeploymentTestRule("rule_1", "group_1", FailurePolicyDisableWhenAllNodesFailed)
	service := NewControlService(store)
	service.now = func() time.Time { return now }

	failure := []ConfigApplyErrorInput{{
		Code:     "LISTENER_BIND_FAILED",
		RuleIDs:  []string{"rule_1"},
		Protocol: domain.ProtocolTCP,
		ListenIP: "0.0.0.0",
		Port:     443,
		Message:  "address already in use",
	}}
	if err := service.AcknowledgeNodeAgentConfig(context.Background(), "org_1", "node_a", 7, "FAILED", "node a failed", failure); err != nil {
		t.Fatalf("ack node_a failure: %v", err)
	}
	if !store.rules["rule_1"].Enabled {
		t.Fatalf("rule must remain enabled while another current node is pending")
	}

	service.now = func() time.Time { return now.Add(time.Second) }
	if err := service.AcknowledgeNodeAgentConfig(context.Background(), "org_1", "node_b", 7, "FAILED", "node b failed", failure); err != nil {
		t.Fatalf("ack node_b failure: %v", err)
	}
	rule := store.rules["rule_1"]
	if rule.Enabled || rule.Status != "DISABLED" {
		t.Fatalf("expected rule disabled after every current node failed, got enabled=%v status=%q", rule.Enabled, rule.Status)
	}
	if rule.ConfigVersion != 8 {
		t.Fatalf("expected auto-disable to bump rule config version, got %d", rule.ConfigVersion)
	}
	if store.bumpedNodeGroups["group_1"] != 1 {
		t.Fatalf("expected desired config bump for rule node group, got %#v", store.bumpedNodeGroups)
	}
	if len(store.auditLogs) != 1 || store.auditLogs[0].Action != "rules.auto_disable_deploy_failure" {
		t.Fatalf("expected auto-disable audit, got %#v", store.auditLogs)
	}
}

func TestAcknowledgeNodeAgentConfigDoesNotAutoDisableFromStaleNodeFailure(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	store := newRuleDeploymentTestStore()
	store.nodes["node_a"] = repo.NodeRecord{ID: "node_a", OrganizationID: "org_1", DesiredConfigVersion: 8, AppliedConfigVersion: 6, GroupIDs: []string{"group_1"}}
	store.nodes["node_b"] = repo.NodeRecord{ID: "node_b", OrganizationID: "org_1", DesiredConfigVersion: 8, AppliedConfigVersion: 6, GroupIDs: []string{"group_1"}}
	store.rules["rule_1"] = ruleDeploymentTestRule("rule_1", "group_1", FailurePolicyDisableWhenAllNodesFailed)
	store.deployments["rule_1|node_a"] = repo.RuleDeploymentRecord{
		OrganizationID:    "org_1",
		RuleID:            "rule_1",
		NodeID:            "node_a",
		ConfigVersion:     7,
		RuleConfigVersion: store.rules["rule_1"].ConfigVersion,
		Status:            RuleDeploymentStatusFailed,
		ErrorCode:         "LISTENER_BIND_FAILED",
	}
	service := NewControlService(store)
	service.now = func() time.Time { return now }

	err := service.AcknowledgeNodeAgentConfig(context.Background(), "org_1", "node_b", 8, "FAILED", "node b failed", []ConfigApplyErrorInput{{
		Code:     "LISTENER_BIND_FAILED",
		RuleIDs:  []string{"rule_1"},
		Protocol: domain.ProtocolTCP,
		ListenIP: "0.0.0.0",
		Port:     443,
		Message:  "address already in use",
	}})
	if err != nil {
		t.Fatalf("ack node_b failure: %v", err)
	}
	if !store.rules["rule_1"].Enabled {
		t.Fatalf("stale node_a failure must not combine with current node_b failure to auto-disable the rule")
	}
	if len(store.auditLogs) != 0 {
		t.Fatalf("stale failure must not write auto-disable audit, got %#v", store.auditLogs)
	}
}

func TestSyncRuleDeploymentPendingUsesNodeDesiredConfigVersion(t *testing.T) {
	store := newRuleDeploymentTestStore()
	store.nodes["node_1"] = repo.NodeRecord{ID: "node_1", OrganizationID: "org_1", DesiredConfigVersion: 12, GroupIDs: []string{"group_1"}}
	store.rules["rule_1"] = ruleDeploymentTestRule("rule_1", "group_1", FailurePolicyKeepEnabled)

	err := syncRuleDeploymentPending(context.Background(), ruleDeploymentTestRepositories{store: store}, "org_1", store.rules["rule_1"], "2026-06-21T12:00:00Z", func() string { return "deployment_1" })
	if err != nil {
		t.Fatalf("sync pending deployment: %v", err)
	}
	deployment := store.deployments["rule_1|node_1"]
	if deployment.ConfigVersion != 12 {
		t.Fatalf("expected pending row to use node desired config version 12, got %#v", deployment)
	}
	if deployment.RuleConfigVersion != store.rules["rule_1"].ConfigVersion {
		t.Fatalf("expected pending row to preserve rule config version, got %#v", deployment)
	}
}

func TestAcknowledgeNodeAgentConfigIgnoresStaleRuleDeploymentFailure(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	store := newRuleDeploymentTestStore()
	store.nodes["node_1"] = repo.NodeRecord{ID: "node_1", OrganizationID: "org_1", DesiredConfigVersion: 8, AppliedConfigVersion: 6, GroupIDs: []string{"group_1"}}
	store.rules["rule_1"] = ruleDeploymentTestRule("rule_1", "group_1", FailurePolicyDisableWhenAllNodesFailed)
	store.deployments["rule_1|node_1"] = repo.RuleDeploymentRecord{
		OrganizationID:    "org_1",
		RuleID:            "rule_1",
		NodeID:            "node_1",
		ConfigVersion:     8,
		RuleConfigVersion: store.rules["rule_1"].ConfigVersion,
		Status:            RuleDeploymentStatusPending,
	}
	service := NewControlService(store)
	service.now = func() time.Time { return now }

	err := service.AcknowledgeNodeAgentConfig(context.Background(), "org_1", "node_1", 7, "FAILED", "stale failed", []ConfigApplyErrorInput{{
		Code:     "LISTENER_BIND_FAILED",
		RuleIDs:  []string{"rule_1"},
		Protocol: domain.ProtocolTCP,
		ListenIP: "0.0.0.0",
		Port:     443,
		Message:  "old failure",
	}})
	if err != nil {
		t.Fatalf("ack stale failed config: %v", err)
	}
	deployment := store.deployments["rule_1|node_1"]
	if deployment.Status != RuleDeploymentStatusPending || deployment.ConfigVersion != 8 || deployment.ErrorCode != "" {
		t.Fatalf("stale failure must not overwrite current pending deployment, got %#v", deployment)
	}
	if !store.rules["rule_1"].Enabled {
		t.Fatalf("stale failure must not trigger all-nodes auto-disable")
	}
}

func TestAcknowledgeNodeAgentConfigUpsertsAppliedDeploymentWhenMissing(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	store := newRuleDeploymentTestStore()
	store.nodes["node_1"] = repo.NodeRecord{ID: "node_1", OrganizationID: "org_1", DesiredConfigVersion: 5, AppliedConfigVersion: 4, GroupIDs: []string{"group_1"}}
	store.rules["rule_1"] = ruleDeploymentTestRule("rule_1", "group_1", FailurePolicyKeepEnabled)
	service := NewControlService(store)
	service.now = func() time.Time { return now }

	if err := service.AcknowledgeNodeAgentConfig(context.Background(), "org_1", "node_1", 5, "APPLIED", "", nil); err != nil {
		t.Fatalf("ack applied config: %v", err)
	}
	deployment := store.deployments["rule_1|node_1"]
	if deployment.Status != RuleDeploymentStatusApplied || deployment.ConfigVersion != 5 || deployment.RuleConfigVersion != store.rules["rule_1"].ConfigVersion {
		t.Fatalf("expected missing deployment row to be upserted as applied, got %#v", deployment)
	}
}

func TestAcknowledgeNodeAgentConfigOnlyAppliesRulesSentToAgent(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	store := newRuleDeploymentTestStore()
	store.nodes["node_1"] = repo.NodeRecord{ID: "node_1", OrganizationID: "org_1", DesiredConfigVersion: 5, AppliedConfigVersion: 4, GroupIDs: []string{"group_1"}}
	store.rules["rule_supported"] = ruleDeploymentTestRule("rule_supported", "group_1", FailurePolicyKeepEnabled)
	unsupported := ruleDeploymentTestRule("rule_unsupported", "group_1", FailurePolicyKeepEnabled)
	unsupported.MatchType = string(domain.MatchTypeFeature)
	unsupported.Binding.MatchType = string(domain.MatchTypeFeature)
	store.rules["rule_unsupported"] = unsupported
	store.deployments["rule_supported|node_1"] = repo.RuleDeploymentRecord{
		OrganizationID:    "org_1",
		RuleID:            "rule_supported",
		NodeID:            "node_1",
		ConfigVersion:     5,
		RuleConfigVersion: store.rules["rule_supported"].ConfigVersion,
		Status:            RuleDeploymentStatusPending,
	}
	store.deployments["rule_unsupported|node_1"] = repo.RuleDeploymentRecord{
		OrganizationID:    "org_1",
		RuleID:            "rule_unsupported",
		NodeID:            "node_1",
		ConfigVersion:     5,
		RuleConfigVersion: unsupported.ConfigVersion,
		Status:            RuleDeploymentStatusPending,
	}
	service := NewControlService(store)
	service.now = func() time.Time { return now }

	if err := service.AcknowledgeNodeAgentConfig(context.Background(), "org_1", "node_1", 5, "APPLIED", "", nil); err != nil {
		t.Fatalf("ack applied config: %v", err)
	}
	if got := store.deployments["rule_supported|node_1"].Status; got != RuleDeploymentStatusApplied {
		t.Fatalf("supported rule should be marked applied, got %q", got)
	}
	if got := store.deployments["rule_unsupported|node_1"].Status; got != RuleDeploymentStatusPending {
		t.Fatalf("unsupported rule must not be marked applied, got %q", got)
	}
}

func TestRuleDeploymentPayloadDoesNotReportDisabledRulesPending(t *testing.T) {
	rule := ruleDeploymentTestRule("rule_1", "group_1", FailurePolicyKeepEnabled)
	rule.Enabled = false
	rule.Status = "DISABLED"

	deployment := ruleDeploymentPayload(rule, []repo.NodeRecord{{ID: "node_1", Name: "node"}}, nil, true)
	if deployment.Status != RuleDeploymentAggregateDisabled || deployment.Total != 0 || deployment.Pending != 0 || len(deployment.Nodes) != 0 {
		t.Fatalf("disabled rule must not fabricate pending deployment rows, got %#v", deployment)
	}
}

func TestRuleDeploymentPayloadCanHideNodeDetails(t *testing.T) {
	rule := ruleDeploymentTestRule("rule_1", "group_1", FailurePolicyKeepEnabled)
	nodes := []repo.NodeRecord{{ID: "node_1", Name: "secret-node"}}
	deployments := []repo.RuleDeploymentRecord{{
		OrganizationID: "org_1",
		RuleID:         "rule_1",
		NodeID:         "node_1",
		Status:         RuleDeploymentStatusFailed,
		ErrorCode:      "LISTENER_BIND_FAILED",
		ErrorMessage:   "address already in use",
	}}

	deployment := ruleDeploymentPayload(rule, nodes, deployments, false)
	if deployment.Failed != 1 || deployment.Total != 1 {
		t.Fatalf("aggregate deployment counts must remain visible, got %#v", deployment)
	}
	if len(deployment.Nodes) != 0 {
		t.Fatalf("node-level deployment details must be hidden without node read permission, got %#v", deployment.Nodes)
	}
}

func TestPortableRulePayloadPreservesFailurePolicy(t *testing.T) {
	rule := ruleDeploymentTestRule("rule_1", "group_1", FailurePolicyDisableWhenAllNodesFailed)
	portable := toPortableRulePayload(rule, map[string]string{"target_1": "target_1"}, nil)
	if portable.FailurePolicy != FailurePolicyDisableWhenAllNodesFailed {
		t.Fatalf("expected exported failure policy to be preserved, got %#v", portable)
	}

	input, err := ruleInputFromPortablePayload(portable, RuleImportEntry{NodeGroupID: "group_1", ListenIP: "0.0.0.0"}, map[string]string{"target_1": "target_1"}, nil)
	if err != nil {
		t.Fatalf("import portable rule: %v", err)
	}
	if input.FailurePolicy != FailurePolicyDisableWhenAllNodesFailed {
		t.Fatalf("expected imported failure policy to be preserved, got %#v", input)
	}
}

func ruleDeploymentTestRule(ruleID string, nodeGroupID string, failurePolicy string) repo.RuleRecord {
	return repo.RuleRecord{
		ID:               ruleID,
		OrganizationID:   "org_1",
		OwnerUserID:      "user_1",
		Name:             ruleID,
		Enabled:          true,
		Status:           "ENABLED",
		ForwardingType:   string(domain.ForwardingTypeDirect),
		Protocol:         string(domain.ProtocolTCP),
		MatchType:        string(domain.MatchTypeAnyInbound),
		TargetType:       "TARGET",
		TargetID:         "target_1",
		FailurePolicy:    failurePolicy,
		ConfigVersion:    7,
		ProxyProtocolIn:  "NONE",
		ProxyProtocolOut: "NONE",
		Binding: repo.InboundBindingRecord{
			ID:             "binding_" + ruleID,
			OrganizationID: "org_1",
			NodeGroupID:    nodeGroupID,
			ListenIP:       "0.0.0.0",
			Protocol:       string(domain.ProtocolTCP),
			Port:           443,
			MatchType:      string(domain.MatchTypeAnyInbound),
		},
	}
}

type ruleDeploymentTestStore struct {
	nodes            map[string]repo.NodeRecord
	rules            map[string]repo.RuleRecord
	targets          map[string]repo.TargetRecord
	targetGroups     map[string]repo.TargetGroupRecord
	deployments      map[string]repo.RuleDeploymentRecord
	bumpedNodeGroups map[string]int
	auditLogs        []repo.AuditLogRecord
}

func newRuleDeploymentTestStore() *ruleDeploymentTestStore {
	return &ruleDeploymentTestStore{
		nodes:            make(map[string]repo.NodeRecord),
		rules:            make(map[string]repo.RuleRecord),
		targets:          map[string]repo.TargetRecord{"target_1": {ID: "target_1", OrganizationID: "org_1", Host: "127.0.0.1", Port: 443, Enabled: true}},
		targetGroups:     make(map[string]repo.TargetGroupRecord),
		deployments:      make(map[string]repo.RuleDeploymentRecord),
		bumpedNodeGroups: make(map[string]int),
	}
}

func (store *ruleDeploymentTestStore) WithinTx(ctx context.Context, fn func(context.Context, repo.Repositories) error) error {
	return fn(ctx, ruleDeploymentTestRepositories{store: store})
}

type ruleDeploymentTestRepositories struct {
	store *ruleDeploymentTestStore
}

func (repositories ruleDeploymentTestRepositories) Users() repo.UserRepository { return nil }
func (repositories ruleDeploymentTestRepositories) Organizations() repo.OrganizationRepository {
	return nil
}
func (repositories ruleDeploymentTestRepositories) Members() repo.MemberRepository       { return nil }
func (repositories ruleDeploymentTestRepositories) Roles() repo.RoleRepository           { return nil }
func (repositories ruleDeploymentTestRepositories) NodeGroups() repo.NodeGroupRepository { return nil }
func (repositories ruleDeploymentTestRepositories) Nodes() repo.NodeRepository {
	return ruleDeploymentTestNodeRepository(repositories)
}
func (repositories ruleDeploymentTestRepositories) MonitorGroups() repo.MonitorGroupRepository {
	return nil
}
func (repositories ruleDeploymentTestRepositories) Monitors() repo.MonitorRepository { return nil }
func (repositories ruleDeploymentTestRepositories) HealthChecks() repo.HealthCheckRepository {
	return nil
}
func (repositories ruleDeploymentTestRepositories) DNSCredentials() repo.DNSCredentialRepository {
	return nil
}
func (repositories ruleDeploymentTestRepositories) DNSRecords() repo.DNSRecordRepository {
	return nil
}
func (repositories ruleDeploymentTestRepositories) Targets() repo.TargetRepository {
	return ruleDeploymentTestTargetRepository(repositories)
}
func (repositories ruleDeploymentTestRepositories) TargetGroups() repo.TargetGroupRepository {
	return ruleDeploymentTestTargetGroupRepository(repositories)
}
func (repositories ruleDeploymentTestRepositories) Rules() repo.RuleRepository {
	return ruleDeploymentTestRuleRepository(repositories)
}
func (repositories ruleDeploymentTestRepositories) Quotas() repo.QuotaRepository { return nil }
func (repositories ruleDeploymentTestRepositories) AgentRegistrationTokens() repo.AgentRegistrationTokenRepository {
	return nil
}
func (repositories ruleDeploymentTestRepositories) AgentCredentials() repo.AgentCredentialRepository {
	return nil
}
func (repositories ruleDeploymentTestRepositories) AuditLogs() repo.AuditLogRepository {
	return ruleDeploymentTestAuditRepository(repositories)
}

type ruleDeploymentTestNodeRepository struct {
	store *ruleDeploymentTestStore
}

func (nodes ruleDeploymentTestNodeRepository) ListNodesByOrganization(context.Context, string) ([]repo.NodeRecord, error) {
	result := make([]repo.NodeRecord, 0, len(nodes.store.nodes))
	for _, node := range nodes.store.nodes {
		result = append(result, node)
	}
	return result, nil
}
func (nodes ruleDeploymentTestNodeRepository) FindNodeByID(_ context.Context, _ string, nodeID string) (repo.NodeRecord, error) {
	node, ok := nodes.store.nodes[nodeID]
	if !ok {
		return repo.NodeRecord{}, repo.ErrNotFound
	}
	return node, nil
}
func (nodes ruleDeploymentTestNodeRepository) CreateNode(context.Context, repo.NodeRecord, []string, []repo.NodeListenIPRecord, []repo.NodePortRangeRecord, string, func() string) error {
	return nil
}
func (nodes ruleDeploymentTestNodeRepository) UpdateNode(context.Context, repo.NodeRecord, bool, []string, bool, []repo.NodeListenIPRecord, bool, []repo.NodePortRangeRecord, string, func() string) error {
	return nil
}
func (nodes ruleDeploymentTestNodeRepository) MarkNodeAgentConnected(context.Context, string, string, string) error {
	return nil
}
func (nodes ruleDeploymentTestNodeRepository) UpdateNodeAgentVersion(context.Context, string, string, repo.NodeAgentVersionRecord, string) error {
	return nil
}
func (nodes ruleDeploymentTestNodeRepository) UpdateNodeAgentUpdatePolicy(context.Context, string, string, bool, string) error {
	return nil
}
func (nodes ruleDeploymentTestNodeRepository) MarkNodeAgentUpdateRequested(context.Context, string, string, string, string) error {
	return nil
}
func (nodes ruleDeploymentTestNodeRepository) MarkNodeAgentUpdateSatisfied(context.Context, string, string, string, string) error {
	return nil
}
func (nodes ruleDeploymentTestNodeRepository) RecordNodeAgentUpdateResult(context.Context, string, string, string, string, string) error {
	return nil
}
func (nodes ruleDeploymentTestNodeRepository) MarkNodeAgentDisconnected(context.Context, string, string, string) error {
	return nil
}
func (nodes ruleDeploymentTestNodeRepository) RecordNodeConfigAck(_ context.Context, _ string, nodeID string, ack repo.NodeConfigAckRecord, now string) error {
	node := nodes.store.nodes[nodeID]
	node.ConfigStatus = ack.Status
	node.ConfigErrorMessage = ack.ErrorMessage
	node.ConfigStatusConfigVersion = ack.ConfigVersion
	node.ConfigRetryCount = ack.RetryCount
	node.ConfigNextRetryAt = ack.NextRetryAt
	node.ConfigStatusUpdatedAt = now
	if ack.Status == "APPLIED" {
		node.AppliedConfigVersion = ack.ConfigVersion
	}
	nodes.store.nodes[nodeID] = node
	return nil
}
func (nodes ruleDeploymentTestNodeRepository) EnsureDesiredConfigVersionAtLeast(context.Context, string, string, int, string) error {
	return nil
}
func (nodes ruleDeploymentTestNodeRepository) IncrementDesiredConfigForNode(context.Context, string, string, string) error {
	return nil
}
func (nodes ruleDeploymentTestNodeRepository) IncrementDesiredConfigForNodeGroup(_ context.Context, _ string, nodeGroupID string, _ string) error {
	nodes.store.bumpedNodeGroups[nodeGroupID]++
	return nil
}
func (nodes ruleDeploymentTestNodeRepository) DeleteNode(context.Context, string, string, string) error {
	return nil
}

type ruleDeploymentTestTargetRepository struct {
	store *ruleDeploymentTestStore
}

func (targets ruleDeploymentTestTargetRepository) ListTargetsByOrganization(context.Context, string) ([]repo.TargetRecord, error) {
	result := make([]repo.TargetRecord, 0, len(targets.store.targets))
	for _, target := range targets.store.targets {
		result = append(result, target)
	}
	return result, nil
}
func (targets ruleDeploymentTestTargetRepository) FindTargetByID(_ context.Context, _ string, targetID string) (repo.TargetRecord, error) {
	target, ok := targets.store.targets[targetID]
	if !ok {
		return repo.TargetRecord{}, repo.ErrNotFound
	}
	return target, nil
}
func (targets ruleDeploymentTestTargetRepository) CreateTarget(context.Context, repo.TargetRecord) error {
	return nil
}
func (targets ruleDeploymentTestTargetRepository) UpdateTarget(context.Context, repo.TargetRecord) error {
	return nil
}
func (targets ruleDeploymentTestTargetRepository) DeleteTarget(context.Context, string, string, string) error {
	return nil
}

type ruleDeploymentTestTargetGroupRepository struct {
	store *ruleDeploymentTestStore
}

func (groups ruleDeploymentTestTargetGroupRepository) ListTargetGroupsByOrganization(context.Context, string) ([]repo.TargetGroupRecord, error) {
	result := make([]repo.TargetGroupRecord, 0, len(groups.store.targetGroups))
	for _, group := range groups.store.targetGroups {
		result = append(result, group)
	}
	return result, nil
}
func (groups ruleDeploymentTestTargetGroupRepository) FindTargetGroupByID(_ context.Context, _ string, targetGroupID string) (repo.TargetGroupRecord, error) {
	group, ok := groups.store.targetGroups[targetGroupID]
	if !ok {
		return repo.TargetGroupRecord{}, repo.ErrNotFound
	}
	return group, nil
}
func (groups ruleDeploymentTestTargetGroupRepository) CreateTargetGroup(context.Context, repo.TargetGroupRecord, []repo.TargetGroupMemberRecord, string, func() string) error {
	return nil
}
func (groups ruleDeploymentTestTargetGroupRepository) UpdateTargetGroup(context.Context, repo.TargetGroupRecord, []repo.TargetGroupMemberRecord, string, func() string) error {
	return nil
}
func (groups ruleDeploymentTestTargetGroupRepository) DeleteTargetGroup(context.Context, string, string, string) error {
	return nil
}

type ruleDeploymentTestRuleRepository struct {
	store *ruleDeploymentTestStore
}

func (rules ruleDeploymentTestRuleRepository) ListRulesByOrganization(context.Context, string) ([]repo.RuleRecord, error) {
	result := make([]repo.RuleRecord, 0, len(rules.store.rules))
	for _, rule := range rules.store.rules {
		result = append(result, rule)
	}
	return result, nil
}
func (rules ruleDeploymentTestRuleRepository) FindRuleByID(_ context.Context, _ string, ruleID string) (repo.RuleRecord, error) {
	rule, ok := rules.store.rules[ruleID]
	if !ok {
		return repo.RuleRecord{}, repo.ErrNotFound
	}
	return rule, nil
}
func (rules ruleDeploymentTestRuleRepository) CreateRule(context.Context, repo.RuleRecord, repo.InboundBindingRecord, []string, string, func() string) error {
	return nil
}
func (rules ruleDeploymentTestRuleRepository) UpdateRule(_ context.Context, rule repo.RuleRecord, _ repo.InboundBindingRecord, _ []string, _ string, _ func() string) error {
	rules.store.rules[rule.ID] = rule
	return nil
}
func (rules ruleDeploymentTestRuleRepository) DeleteRule(context.Context, string, string, string) error {
	return nil
}
func (rules ruleDeploymentTestRuleRepository) ListEnabledInboundBindings(context.Context, string) ([]repo.RuleRecord, error) {
	return nil, nil
}
func (rules ruleDeploymentTestRuleRepository) CountRulesByOrganization(context.Context, string) (int, error) {
	return 0, nil
}
func (rules ruleDeploymentTestRuleRepository) CountRulesByOwner(context.Context, string, string) (int, error) {
	return 0, nil
}
func (rules ruleDeploymentTestRuleRepository) SumRuleTraffic(context.Context, string, string) (repo.RuleTrafficRecord, error) {
	return repo.RuleTrafficRecord{}, nil
}
func (rules ruleDeploymentTestRuleRepository) RecordNodeRuleTrafficAssignments(context.Context, string, string, []string, string) error {
	return nil
}
func (rules ruleDeploymentTestRuleRepository) RecordRuleTrafficReport(context.Context, string, string, repo.RuleTrafficReportRecord, []repo.RuleTrafficDeltaRecord, string, func() string) (bool, error) {
	return false, nil
}
func (rules ruleDeploymentTestRuleRepository) ListRuleDeploymentsByOrganization(context.Context, string) ([]repo.RuleDeploymentRecord, error) {
	result := make([]repo.RuleDeploymentRecord, 0, len(rules.store.deployments))
	for _, deployment := range rules.store.deployments {
		result = append(result, deployment)
	}
	return result, nil
}
func (rules ruleDeploymentTestRuleRepository) ReplaceRuleDeploymentPending(_ context.Context, _ string, rule repo.RuleRecord, deployments []repo.RuleDeploymentPendingRecord, now string, _ func() string) error {
	for key, deployment := range rules.store.deployments {
		if deployment.RuleID == rule.ID {
			delete(rules.store.deployments, key)
		}
	}
	for _, pending := range deployments {
		rules.store.deployments[rule.ID+"|"+pending.NodeID] = repo.RuleDeploymentRecord{
			OrganizationID:    rule.OrganizationID,
			RuleID:            rule.ID,
			NodeID:            pending.NodeID,
			ConfigVersion:     pending.ConfigVersion,
			RuleConfigVersion: rule.ConfigVersion,
			Status:            RuleDeploymentStatusPending,
			UpdatedAt:         now,
		}
	}
	return nil
}
func (rules ruleDeploymentTestRuleRepository) UpsertRuleDeploymentPending(_ context.Context, organizationID string, rule repo.RuleRecord, pending repo.RuleDeploymentPendingRecord, now string, _ func() string) error {
	rules.store.deployments[rule.ID+"|"+pending.NodeID] = repo.RuleDeploymentRecord{
		OrganizationID:    organizationID,
		RuleID:            rule.ID,
		NodeID:            pending.NodeID,
		ConfigVersion:     pending.ConfigVersion,
		RuleConfigVersion: rule.ConfigVersion,
		Status:            RuleDeploymentStatusPending,
		UpdatedAt:         now,
	}
	return nil
}
func (rules ruleDeploymentTestRuleRepository) RecordRuleDeploymentApplied(_ context.Context, organizationID string, nodeID string, configVersion int, applied []repo.RuleDeploymentAppliedRecord, now string, _ func() string) error {
	for _, deployment := range applied {
		key := deployment.RuleID + "|" + nodeID
		current, ok := rules.store.deployments[key]
		if ok && current.ConfigVersion > configVersion {
			continue
		}
		rules.store.deployments[key] = repo.RuleDeploymentRecord{
			OrganizationID:    organizationID,
			RuleID:            deployment.RuleID,
			NodeID:            nodeID,
			ConfigVersion:     configVersion,
			RuleConfigVersion: deployment.RuleConfigVersion,
			Status:            RuleDeploymentStatusApplied,
			UpdatedAt:         now,
		}
	}
	return nil
}
func (rules ruleDeploymentTestRuleRepository) RecordRuleDeploymentFailures(_ context.Context, organizationID string, nodeID string, configVersion int, failures []repo.RuleDeploymentFailureRecord, now string, _ func() string) error {
	for _, failure := range failures {
		key := failure.RuleID + "|" + nodeID
		current, ok := rules.store.deployments[key]
		if ok && (current.ConfigVersion != configVersion || current.RuleConfigVersion != failure.RuleConfigVersion) {
			continue
		}
		rules.store.deployments[failure.RuleID+"|"+nodeID] = repo.RuleDeploymentRecord{
			OrganizationID:    organizationID,
			RuleID:            failure.RuleID,
			NodeID:            nodeID,
			ConfigVersion:     configVersion,
			RuleConfigVersion: failure.RuleConfigVersion,
			Status:            RuleDeploymentStatusFailed,
			ErrorCode:         failure.ErrorCode,
			ErrorMessage:      failure.ErrorMessage,
			Protocol:          failure.Protocol,
			ListenIP:          failure.ListenIP,
			Port:              failure.Port,
			UpdatedAt:         now,
		}
	}
	return nil
}
func (rules ruleDeploymentTestRuleRepository) DeleteRuleDeploymentForNode(_ context.Context, _ string, ruleID string, nodeID string) error {
	delete(rules.store.deployments, ruleID+"|"+nodeID)
	return nil
}
func (rules ruleDeploymentTestRuleRepository) DeleteRuleDeployments(_ context.Context, _ string, ruleID string) error {
	for key, deployment := range rules.store.deployments {
		if deployment.RuleID == ruleID {
			delete(rules.store.deployments, key)
		}
	}
	return nil
}

type ruleDeploymentTestAuditRepository struct {
	store *ruleDeploymentTestStore
}

func (audits ruleDeploymentTestAuditRepository) CreateAuditLog(_ context.Context, audit repo.AuditLogRecord) error {
	audits.store.auditLogs = append(audits.store.auditLogs, audit)
	return nil
}
