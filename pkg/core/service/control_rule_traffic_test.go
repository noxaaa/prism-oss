package service

import (
	"context"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestRecordNodeTrafficReportDelegatesRuleDeltas(t *testing.T) {
	store := &trafficReportTestStore{node: repo.NodeRecord{ID: "node_1", OrganizationID: "org_1"}}
	service := NewControlService(store)

	recorded, err := service.RecordNodeTrafficReport(context.Background(), "org_1", "node_1", AgentTrafficReportInput{
		ReportID: "report_1",
		Deltas: []agent.RuleTrafficDelta{
			{RuleID: "rule_1", UploadBytes: 100, DownloadBytes: 50, TCPConnections: 1, UDPPackets: 2},
		},
	})
	if err != nil {
		t.Fatalf("record node traffic report: %v", err)
	}
	if !recorded {
		t.Fatalf("expected first report to be recorded")
	}
	if store.ruleReport.ReportID != "report_1" || store.ruleAgentID != "node_1" {
		t.Fatalf("unexpected recorded report metadata: report=%#v agent=%q", store.ruleReport, store.ruleAgentID)
	}
	if len(store.ruleDeltas) != 1 || store.ruleDeltas[0].RuleID != "rule_1" || store.ruleDeltas[0].UploadBytes != 100 {
		t.Fatalf("unexpected recorded deltas: %#v", store.ruleDeltas)
	}
}

func TestRecordNodeTrafficReportReturnsFalseForDuplicateReport(t *testing.T) {
	store := &trafficReportTestStore{node: repo.NodeRecord{ID: "node_1", OrganizationID: "org_1"}, duplicate: true}
	service := NewControlService(store)

	recorded, err := service.RecordNodeTrafficReport(context.Background(), "org_1", "node_1", AgentTrafficReportInput{
		ReportID: "report_1",
		Deltas:   []agent.RuleTrafficDelta{{RuleID: "rule_1", UploadBytes: 100}},
	})
	if err != nil {
		t.Fatalf("record duplicate node traffic report: %v", err)
	}
	if recorded {
		t.Fatalf("expected duplicate report to be ignored")
	}
}

type trafficReportTestStore struct {
	node        repo.NodeRecord
	duplicate   bool
	ruleAgentID string
	ruleReport  repo.RuleTrafficReportRecord
	ruleDeltas  []repo.RuleTrafficDeltaRecord
}

func (store *trafficReportTestStore) WithinTx(ctx context.Context, fn func(context.Context, repo.Repositories) error) error {
	return fn(ctx, trafficReportTestRepositories{store: store})
}

type trafficReportTestRepositories struct {
	store *trafficReportTestStore
}

func (repositories trafficReportTestRepositories) Users() repo.UserRepository { return nil }
func (repositories trafficReportTestRepositories) Organizations() repo.OrganizationRepository {
	return nil
}
func (repositories trafficReportTestRepositories) Members() repo.MemberRepository { return nil }
func (repositories trafficReportTestRepositories) Roles() repo.RoleRepository     { return nil }
func (repositories trafficReportTestRepositories) NodeGroups() repo.NodeGroupRepository {
	return nil
}
func (repositories trafficReportTestRepositories) Nodes() repo.NodeRepository {
	return trafficReportTestNodeRepository(repositories)
}
func (repositories trafficReportTestRepositories) MonitorGroups() repo.MonitorGroupRepository {
	return nil
}
func (repositories trafficReportTestRepositories) Monitors() repo.MonitorRepository { return nil }
func (repositories trafficReportTestRepositories) HealthChecks() repo.HealthCheckRepository {
	return nil
}
func (repositories trafficReportTestRepositories) DNSCredentials() repo.DNSCredentialRepository {
	return nil
}
func (repositories trafficReportTestRepositories) DNSRecords() repo.DNSRecordRepository {
	return nil
}
func (repositories trafficReportTestRepositories) Targets() repo.TargetRepository { return nil }
func (repositories trafficReportTestRepositories) TargetGroups() repo.TargetGroupRepository {
	return nil
}
func (repositories trafficReportTestRepositories) Rules() repo.RuleRepository {
	return trafficReportTestRuleRepository(repositories)
}
func (repositories trafficReportTestRepositories) Quotas() repo.QuotaRepository { return nil }
func (repositories trafficReportTestRepositories) AgentRegistrationTokens() repo.AgentRegistrationTokenRepository {
	return nil
}
func (repositories trafficReportTestRepositories) AgentCredentials() repo.AgentCredentialRepository {
	return nil
}
func (repositories trafficReportTestRepositories) AuditLogs() repo.AuditLogRepository { return nil }

type trafficReportTestNodeRepository struct {
	store *trafficReportTestStore
}

func (nodes trafficReportTestNodeRepository) ListNodesByOrganization(context.Context, string) ([]repo.NodeRecord, error) {
	return nil, nil
}
func (nodes trafficReportTestNodeRepository) FindNodeByID(_ context.Context, organizationID string, nodeID string) (repo.NodeRecord, error) {
	if nodes.store.node.OrganizationID == organizationID && nodes.store.node.ID == nodeID {
		return nodes.store.node, nil
	}
	return repo.NodeRecord{}, repo.ErrNotFound
}
func (nodes trafficReportTestNodeRepository) CreateNode(context.Context, repo.NodeRecord, []string, []repo.NodeListenIPRecord, []repo.NodePortRangeRecord, string, func() string) error {
	return nil
}
func (nodes trafficReportTestNodeRepository) UpdateNode(context.Context, repo.NodeRecord, bool, []string, bool, []repo.NodeListenIPRecord, bool, []repo.NodePortRangeRecord, string, func() string) error {
	return nil
}
func (nodes trafficReportTestNodeRepository) MarkNodeAgentConnected(context.Context, string, string, string) error {
	return nil
}
func (nodes trafficReportTestNodeRepository) UpdateNodeAgentVersion(context.Context, string, string, repo.NodeAgentVersionRecord, string) error {
	return nil
}
func (nodes trafficReportTestNodeRepository) UpdateNodeAgentUpdatePolicy(context.Context, string, string, bool, string) error {
	return nil
}
func (nodes trafficReportTestNodeRepository) MarkNodeAgentUpdateRequested(context.Context, string, string, string, string) error {
	return nil
}
func (nodes trafficReportTestNodeRepository) MarkNodeAgentUpdateSatisfied(context.Context, string, string, string, string) error {
	return nil
}
func (nodes trafficReportTestNodeRepository) RecordNodeAgentUpdateResult(context.Context, string, string, string, string, string) error {
	return nil
}
func (nodes trafficReportTestNodeRepository) MarkNodeAgentDisconnected(context.Context, string, string, string) error {
	return nil
}
func (nodes trafficReportTestNodeRepository) RecordNodeConfigAck(context.Context, string, string, repo.NodeConfigAckRecord, string) error {
	return nil
}
func (nodes trafficReportTestNodeRepository) EnsureDesiredConfigVersionAtLeast(context.Context, string, string, int, string) error {
	return nil
}
func (nodes trafficReportTestNodeRepository) IncrementDesiredConfigForNode(context.Context, string, string, string) error {
	return nil
}
func (nodes trafficReportTestNodeRepository) IncrementDesiredConfigForNodeGroup(context.Context, string, string, string) error {
	return nil
}
func (nodes trafficReportTestNodeRepository) DeleteNode(context.Context, string, string, string) error {
	return nil
}

type trafficReportTestRuleRepository struct {
	store *trafficReportTestStore
}

func (rules trafficReportTestRuleRepository) ListRulesByOrganization(context.Context, string) ([]repo.RuleRecord, error) {
	return nil, nil
}
func (rules trafficReportTestRuleRepository) FindRuleByID(context.Context, string, string) (repo.RuleRecord, error) {
	return repo.RuleRecord{}, repo.ErrNotFound
}
func (rules trafficReportTestRuleRepository) CreateRule(context.Context, repo.RuleRecord, repo.InboundBindingRecord, []string, string, func() string) error {
	return nil
}
func (rules trafficReportTestRuleRepository) UpdateRule(context.Context, repo.RuleRecord, repo.InboundBindingRecord, []string, string, func() string) error {
	return nil
}
func (rules trafficReportTestRuleRepository) DeleteRule(context.Context, string, string, string) error {
	return nil
}
func (rules trafficReportTestRuleRepository) ListEnabledInboundBindings(context.Context, string) ([]repo.RuleRecord, error) {
	return nil, nil
}
func (rules trafficReportTestRuleRepository) CountRulesByOrganization(context.Context, string) (int, error) {
	return 0, nil
}
func (rules trafficReportTestRuleRepository) CountRulesByOwner(context.Context, string, string) (int, error) {
	return 0, nil
}
func (rules trafficReportTestRuleRepository) SumRuleTraffic(context.Context, string, string) (repo.RuleTrafficRecord, error) {
	return repo.RuleTrafficRecord{}, nil
}
func (rules trafficReportTestRuleRepository) RecordNodeRuleTrafficAssignments(context.Context, string, string, []string, string) error {
	return nil
}
func (rules trafficReportTestRuleRepository) RecordRuleTrafficReport(_ context.Context, _ string, agentID string, report repo.RuleTrafficReportRecord, deltas []repo.RuleTrafficDeltaRecord, _ string, _ func() string) (bool, error) {
	rules.store.ruleAgentID = agentID
	rules.store.ruleReport = report
	rules.store.ruleDeltas = append([]repo.RuleTrafficDeltaRecord(nil), deltas...)
	return !rules.store.duplicate, nil
}
func (rules trafficReportTestRuleRepository) ListRuleDeploymentsByOrganization(context.Context, string) ([]repo.RuleDeploymentRecord, error) {
	return nil, nil
}
func (rules trafficReportTestRuleRepository) ReplaceRuleDeploymentPending(context.Context, string, repo.RuleRecord, []repo.RuleDeploymentPendingRecord, string, func() string) error {
	return nil
}
func (rules trafficReportTestRuleRepository) UpsertRuleDeploymentPending(context.Context, string, repo.RuleRecord, repo.RuleDeploymentPendingRecord, string, func() string) error {
	return nil
}
func (rules trafficReportTestRuleRepository) RecordRuleDeploymentApplied(context.Context, string, string, int, []repo.RuleDeploymentAppliedRecord, string, func() string) error {
	return nil
}
func (rules trafficReportTestRuleRepository) RecordRuleDeploymentFailures(context.Context, string, string, int, []repo.RuleDeploymentFailureRecord, string, func() string) error {
	return nil
}
func (rules trafficReportTestRuleRepository) DeleteRuleDeploymentForNode(context.Context, string, string, string) error {
	return nil
}
func (rules trafficReportTestRuleRepository) DeleteRuleDeployments(context.Context, string, string) error {
	return nil
}
