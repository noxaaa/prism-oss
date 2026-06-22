package service

import (
	"context"
	"errors"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestControlServicePersistsTargetGroupSchedulerAndMembers(t *testing.T) {
	store := newTargetGroupServiceTestStore()
	control := NewControlService(store)
	identity := targetGroupServiceTestIdentity()

	result, err := control.CreateTargetGroup(context.Background(), identity, TargetGroupMutationInput{
		Name:        "Primary pool",
		Description: "Priority iphash pool",
		Scheduler:   "PRIORITY_IPHASH",
		Members: []TargetGroupMemberInput{
			{TargetID: "target_a", Priority: 10, Enabled: true},
			{TargetID: "target_b", Priority: 20, Enabled: false},
		},
	})
	if err != nil {
		t.Fatalf("create target group: %v", err)
	}
	if result.Scheduler != "PRIORITY_IPHASH" {
		t.Fatalf("expected scheduler to be persisted, got %#v", result)
	}
	if len(result.Members) != 2 || result.Members[0].Priority != 10 || result.Members[1].Priority != 20 || result.Members[1].Enabled {
		t.Fatalf("expected per-member priority and enabled state to be persisted, got %#v", result.Members)
	}

	updated, err := control.UpdateTargetGroup(context.Background(), identity, result.ID, TargetGroupMutationInput{
		Name:        "Primary pool",
		Description: "Priority iphash pool",
		Scheduler:   "PRIORITY_IPHASH",
		Members: []TargetGroupMemberInput{
			{TargetID: "target_a", Priority: 30, Enabled: true},
			{TargetID: "target_b", Priority: 5, Enabled: true},
		},
	})
	if err != nil {
		t.Fatalf("update target group: %v", err)
	}
	if updated.Scheduler != "PRIORITY_IPHASH" {
		t.Fatalf("expected updated scheduler to remain persisted, got %#v", updated)
	}
	if len(updated.Members) != 2 || updated.Members[0].Priority != 30 || updated.Members[1].Priority != 5 || !updated.Members[1].Enabled {
		t.Fatalf("expected updated per-member priority and enabled state, got %#v", updated.Members)
	}
}

func TestControlServiceRejectsUnsupportedOSSTargetGroupScheduler(t *testing.T) {
	store := newTargetGroupServiceTestStore()
	control := NewControlService(store)
	identity := targetGroupServiceTestIdentity()

	_, err := control.CreateTargetGroup(context.Background(), identity, TargetGroupMutationInput{
		Name:        "Geo pool",
		Description: "Commercial scheduler",
		Scheduler:   "GEO_IP",
		Members:     []TargetGroupMemberInput{{TargetID: "target_a", Priority: 10, Enabled: true}},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected OSS service to reject GEO_IP scheduler, got %v", err)
	}
}

func TestControlServiceCoreSchedulerSupportIgnoresExtensionPolicy(t *testing.T) {
	control := NewControlServiceWithOptions(nil, ControlServiceOptions{
		TargetGroupSchedulers: func(string) bool {
			return true
		},
	})

	if !control.targetGroupSchedulerSupported("GEO_IP") {
		t.Fatalf("expected extension policy to allow GEO_IP mutations")
	}
	if control.targetGroupSchedulerSupportedByCore(repo.TargetGroupRecord{Scheduler: "GEO_IP"}) {
		t.Fatalf("expected OSS core compiler support to remain limited to PRIORITY_IPHASH")
	}
	if !control.targetGroupSchedulerSupportedByCore(repo.TargetGroupRecord{Scheduler: "PRIORITY_IPHASH"}) {
		t.Fatalf("expected OSS core compiler to support PRIORITY_IPHASH")
	}
}

func TestControlServiceUpdatesPolicySupportedTargetGroupScheduler(t *testing.T) {
	store := newTargetGroupServiceTestStore()
	store.targetGroups["group_geo"] = repo.TargetGroupRecord{
		ID:             "group_geo",
		OrganizationID: "org_1",
		Name:           "Geo pool",
		Description:    "Commercial scheduler",
		Scheduler:      "GEO_IP",
		Members:        []repo.TargetGroupMemberRecord{{TargetID: "target_a", Priority: 10, Enabled: true}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		TargetGroupSchedulers: func(string) bool {
			return true
		},
	})

	updated, err := control.UpdateTargetGroup(context.Background(), targetGroupServiceTestIdentity(), "group_geo", TargetGroupMutationInput{
		Name:        "Geo pool updated",
		Description: "Commercial scheduler",
		Scheduler:   "GEO_IP",
		Members:     []TargetGroupMemberInput{{TargetID: "target_b", Priority: 20, Enabled: true}},
	})
	if err != nil {
		t.Fatalf("update policy-supported target group: %v", err)
	}
	if updated.Scheduler != "GEO_IP" || len(updated.Members) != 1 || updated.Members[0].TargetID != "target_b" {
		t.Fatalf("expected policy-supported scheduler and members to update, got %#v", updated)
	}
}

func TestControlServiceSyncsPolicySupportedTargetGroupMemberships(t *testing.T) {
	store := newTargetGroupServiceTestStore()
	store.targetGroups["group_geo"] = repo.TargetGroupRecord{
		ID:             "group_geo",
		OrganizationID: "org_1",
		Name:           "Geo pool",
		Description:    "Commercial scheduler",
		Scheduler:      "GEO_IP",
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		TargetGroupSchedulers: func(string) bool {
			return true
		},
	})

	_, err := control.UpdateTarget(context.Background(), targetGroupServiceTestIdentity(), "target_a", TargetMutationInput{
		Name:                   "A",
		Host:                   "10.0.0.1",
		Port:                   443,
		Enabled:                true,
		TargetGroupIDs:         []string{"group_geo"},
		TargetGroupIDsProvided: true,
	})
	if err != nil {
		t.Fatalf("sync policy-supported target group membership: %v", err)
	}
	members := store.targetGroups["group_geo"].Members
	if len(members) != 1 || members[0].TargetID != "target_a" || members[0].Priority != defaultTargetGroupMemberPriority || !members[0].Enabled {
		t.Fatalf("expected target membership to be synced, got %#v", members)
	}
}

func TestControlServiceRejectsDeletingTargetGroupUsedByHealthCheck(t *testing.T) {
	store := newTargetGroupServiceTestStore()
	store.targetGroups["group_health"] = repo.TargetGroupRecord{
		ID:             "group_health",
		OrganizationID: "org_1",
		Name:           "Health checked pool",
		Scheduler:      "PRIORITY_IPHASH",
	}
	store.healthChecks = []repo.HealthCheckRecord{{
		ID:             "health_1",
		OrganizationID: "org_1",
		Targets: []repo.HealthCheckTargetRecord{{
			ScopeType:     "TARGET_GROUP",
			TargetGroupID: "group_health",
		}},
	}}
	control := NewControlService(store)

	err := control.DeleteTargetGroup(context.Background(), targetGroupServiceTestIdentity(), "group_health")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	if _, ok := store.targetGroups["group_health"]; !ok {
		t.Fatalf("target group must not be deleted while health checks reference it")
	}
}

func TestControlServiceRejectsDeletingTargetUsedByHealthCheck(t *testing.T) {
	store := newTargetGroupServiceTestStore()
	store.healthChecks = []repo.HealthCheckRecord{{
		ID:             "health_1",
		OrganizationID: "org_1",
		Targets: []repo.HealthCheckTargetRecord{{
			ScopeType: "TARGET",
			TargetID:  "target_a",
		}},
	}}
	control := NewControlService(store)

	err := control.DeleteTarget(context.Background(), targetGroupServiceTestIdentity(), "target_a")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	if store.deletedTargetID != "" {
		t.Fatalf("target must not be deleted while health checks reference it")
	}
}

func TestControlServiceRejectsDeletingTargetUsedByHealthCheckedTargetGroup(t *testing.T) {
	store := newTargetGroupServiceTestStore()
	store.targetGroups["group_health"] = repo.TargetGroupRecord{
		ID:             "group_health",
		OrganizationID: "org_1",
		Name:           "Health checked pool",
		Scheduler:      "PRIORITY_IPHASH",
		Members: []repo.TargetGroupMemberRecord{{
			TargetID: "target_a",
			Enabled:  true,
		}},
	}
	store.healthChecks = []repo.HealthCheckRecord{{
		ID:             "health_1",
		OrganizationID: "org_1",
		Targets: []repo.HealthCheckTargetRecord{{
			ScopeType:     "TARGET_GROUP",
			TargetGroupID: "group_health",
		}},
	}}
	control := NewControlService(store)

	err := control.DeleteTarget(context.Background(), targetGroupServiceTestIdentity(), "target_a")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	if store.deletedTargetID != "" {
		t.Fatalf("target must not be deleted while a health-checked target group references it")
	}
}

func newTargetGroupServiceTestStore() *targetGroupServiceTestStore {
	return &targetGroupServiceTestStore{
		targets: map[string]repo.TargetRecord{
			"target_a": {ID: "target_a", OrganizationID: "org_1", Name: "A", Host: "10.0.0.1", Port: 443, Enabled: true},
			"target_b": {ID: "target_b", OrganizationID: "org_1", Name: "B", Host: "10.0.0.2", Port: 443, Enabled: true},
		},
		targetGroups: map[string]repo.TargetGroupRecord{},
	}
}

type targetGroupServiceTestStore struct {
	targets         map[string]repo.TargetRecord
	targetGroups    map[string]repo.TargetGroupRecord
	healthChecks    []repo.HealthCheckRecord
	auditLogs       []repo.AuditLogRecord
	deletedTargetID string
}

func (store *targetGroupServiceTestStore) WithinTx(ctx context.Context, fn func(context.Context, repo.Repositories) error) error {
	return fn(ctx, targetGroupServiceTestRepositories{store: store})
}

type targetGroupServiceTestRepositories struct {
	store *targetGroupServiceTestStore
}

func (repositories targetGroupServiceTestRepositories) Users() repo.UserRepository { return nil }
func (repositories targetGroupServiceTestRepositories) Organizations() repo.OrganizationRepository {
	return nil
}
func (repositories targetGroupServiceTestRepositories) Members() repo.MemberRepository { return nil }
func (repositories targetGroupServiceTestRepositories) Roles() repo.RoleRepository     { return nil }
func (repositories targetGroupServiceTestRepositories) NodeGroups() repo.NodeGroupRepository {
	return nil
}
func (repositories targetGroupServiceTestRepositories) Nodes() repo.NodeRepository { return nil }
func (repositories targetGroupServiceTestRepositories) MonitorGroups() repo.MonitorGroupRepository {
	return nil
}
func (repositories targetGroupServiceTestRepositories) Monitors() repo.MonitorRepository {
	return nil
}
func (repositories targetGroupServiceTestRepositories) HealthChecks() repo.HealthCheckRepository {
	return targetGroupServiceTestHealthRepository(repositories)
}
func (repositories targetGroupServiceTestRepositories) DNSCredentials() repo.DNSCredentialRepository {
	return nil
}
func (repositories targetGroupServiceTestRepositories) DNSRecords() repo.DNSRecordRepository {
	return nil
}
func (repositories targetGroupServiceTestRepositories) Targets() repo.TargetRepository {
	return targetGroupServiceTestTargetRepository(repositories)
}
func (repositories targetGroupServiceTestRepositories) TargetGroups() repo.TargetGroupRepository {
	return targetGroupServiceTestTargetGroupRepository(repositories)
}
func (repositories targetGroupServiceTestRepositories) Rules() repo.RuleRepository {
	return targetGroupServiceTestRuleRepository{}
}
func (repositories targetGroupServiceTestRepositories) Quotas() repo.QuotaRepository { return nil }
func (repositories targetGroupServiceTestRepositories) AgentRegistrationTokens() repo.AgentRegistrationTokenRepository {
	return nil
}
func (repositories targetGroupServiceTestRepositories) AgentCredentials() repo.AgentCredentialRepository {
	return nil
}
func (repositories targetGroupServiceTestRepositories) AuditLogs() repo.AuditLogRepository {
	return targetGroupServiceTestAuditRepository(repositories)
}

type targetGroupServiceTestTargetRepository struct {
	store *targetGroupServiceTestStore
}

func (targets targetGroupServiceTestTargetRepository) ListTargetsByOrganization(context.Context, string) ([]repo.TargetRecord, error) {
	result := make([]repo.TargetRecord, 0, len(targets.store.targets))
	for _, target := range targets.store.targets {
		result = append(result, target)
	}
	return result, nil
}

func (targets targetGroupServiceTestTargetRepository) FindTargetByID(_ context.Context, _ string, targetID string) (repo.TargetRecord, error) {
	target, ok := targets.store.targets[targetID]
	if !ok {
		return repo.TargetRecord{}, repo.ErrNotFound
	}
	return target, nil
}

func (targets targetGroupServiceTestTargetRepository) CreateTarget(context.Context, repo.TargetRecord) error {
	return nil
}

func (targets targetGroupServiceTestTargetRepository) UpdateTarget(context.Context, repo.TargetRecord) error {
	return nil
}

func (targets targetGroupServiceTestTargetRepository) DeleteTarget(_ context.Context, _ string, targetID string, _ string) error {
	targets.store.deletedTargetID = targetID
	return nil
}

type targetGroupServiceTestTargetGroupRepository struct {
	store *targetGroupServiceTestStore
}

func (targetGroups targetGroupServiceTestTargetGroupRepository) ListTargetGroupsByOrganization(context.Context, string) ([]repo.TargetGroupRecord, error) {
	result := make([]repo.TargetGroupRecord, 0, len(targetGroups.store.targetGroups))
	for _, group := range targetGroups.store.targetGroups {
		result = append(result, group)
	}
	return result, nil
}

func (targetGroups targetGroupServiceTestTargetGroupRepository) FindTargetGroupByID(_ context.Context, _ string, targetGroupID string) (repo.TargetGroupRecord, error) {
	group, ok := targetGroups.store.targetGroups[targetGroupID]
	if !ok {
		return repo.TargetGroupRecord{}, repo.ErrNotFound
	}
	return group, nil
}

func (targetGroups targetGroupServiceTestTargetGroupRepository) CreateTargetGroup(_ context.Context, group repo.TargetGroupRecord, members []repo.TargetGroupMemberRecord, now string, nextID func() string) error {
	group.Members = testTargetGroupMembers(group.OrganizationID, group.ID, members, now, nextID)
	targetGroups.store.targetGroups[group.ID] = group
	return nil
}

func (targetGroups targetGroupServiceTestTargetGroupRepository) UpdateTargetGroup(_ context.Context, group repo.TargetGroupRecord, members []repo.TargetGroupMemberRecord, now string, nextID func() string) error {
	group.Members = testTargetGroupMembers(group.OrganizationID, group.ID, members, now, nextID)
	targetGroups.store.targetGroups[group.ID] = group
	return nil
}

func (targetGroups targetGroupServiceTestTargetGroupRepository) DeleteTargetGroup(_ context.Context, _ string, targetGroupID string, _ string) error {
	delete(targetGroups.store.targetGroups, targetGroupID)
	return nil
}

func testTargetGroupMembers(organizationID string, targetGroupID string, members []repo.TargetGroupMemberRecord, now string, nextID func() string) []repo.TargetGroupMemberRecord {
	out := make([]repo.TargetGroupMemberRecord, 0, len(members))
	for _, member := range members {
		member.ID = nextID()
		member.OrganizationID = organizationID
		member.TargetGroupID = targetGroupID
		member.CreatedAt = now
		member.UpdatedAt = now
		out = append(out, member)
	}
	return out
}

type targetGroupServiceTestRuleRepository struct{}

func (rules targetGroupServiceTestRuleRepository) ListRulesByOrganization(context.Context, string) ([]repo.RuleRecord, error) {
	return nil, nil
}

func (rules targetGroupServiceTestRuleRepository) FindRuleByID(context.Context, string, string) (repo.RuleRecord, error) {
	return repo.RuleRecord{}, repo.ErrNotFound
}

func (rules targetGroupServiceTestRuleRepository) CreateRule(context.Context, repo.RuleRecord, repo.InboundBindingRecord, []string, string, func() string) error {
	return nil
}

func (rules targetGroupServiceTestRuleRepository) UpdateRule(context.Context, repo.RuleRecord, repo.InboundBindingRecord, []string, string, func() string) error {
	return nil
}

func (rules targetGroupServiceTestRuleRepository) DeleteRule(context.Context, string, string, string) error {
	return nil
}

func (rules targetGroupServiceTestRuleRepository) ListEnabledInboundBindings(context.Context, string) ([]repo.RuleRecord, error) {
	return nil, nil
}

func (rules targetGroupServiceTestRuleRepository) CountRulesByOrganization(context.Context, string) (int, error) {
	return 0, nil
}

func (rules targetGroupServiceTestRuleRepository) CountRulesByOwner(context.Context, string, string) (int, error) {
	return 0, nil
}

func (rules targetGroupServiceTestRuleRepository) SumRuleTraffic(context.Context, string, string) (repo.RuleTrafficRecord, error) {
	return repo.RuleTrafficRecord{}, nil
}

func (rules targetGroupServiceTestRuleRepository) RecordNodeRuleTrafficAssignments(context.Context, string, string, []string, string) error {
	return nil
}

func (rules targetGroupServiceTestRuleRepository) RecordRuleTrafficReport(context.Context, string, string, repo.RuleTrafficReportRecord, []repo.RuleTrafficDeltaRecord, string, func() string) (bool, error) {
	return false, nil
}

type targetGroupServiceTestHealthRepository struct {
	store *targetGroupServiceTestStore
}

func (healthChecks targetGroupServiceTestHealthRepository) ListHealthChecksByOrganization(_ context.Context, organizationID string) ([]repo.HealthCheckRecord, error) {
	result := make([]repo.HealthCheckRecord, 0, len(healthChecks.store.healthChecks))
	for _, check := range healthChecks.store.healthChecks {
		if check.OrganizationID == organizationID {
			result = append(result, check)
		}
	}
	return result, nil
}
func (healthChecks targetGroupServiceTestHealthRepository) FindHealthCheckByID(context.Context, string, string) (repo.HealthCheckRecord, error) {
	return repo.HealthCheckRecord{}, repo.ErrNotFound
}
func (healthChecks targetGroupServiceTestHealthRepository) CreateHealthCheck(context.Context, repo.HealthCheckRecord, []repo.HealthCheckTargetRecord, []repo.HealthCheckMonitorScopeRecord, string, func() string) error {
	return nil
}
func (healthChecks targetGroupServiceTestHealthRepository) UpdateHealthCheck(context.Context, repo.HealthCheckRecord, []repo.HealthCheckTargetRecord, []repo.HealthCheckMonitorScopeRecord, string, func() string) error {
	return nil
}
func (healthChecks targetGroupServiceTestHealthRepository) SyncHealthCheckTargets(context.Context, string, string, []repo.HealthCheckTargetRecord, string, func() string) error {
	return nil
}
func (healthChecks targetGroupServiceTestHealthRepository) DeleteHealthCheck(context.Context, string, string, string) error {
	return nil
}
func (healthChecks targetGroupServiceTestHealthRepository) ListHealthResults(context.Context, string, string, int) ([]repo.HealthResultRecord, error) {
	return nil, nil
}
func (healthChecks targetGroupServiceTestHealthRepository) RecordHealthResults(context.Context, string, []repo.HealthResultRecord) error {
	return nil
}
func (healthChecks targetGroupServiceTestHealthRepository) ListHealthEvaluationRulesByCheck(context.Context, string, string) ([]repo.HealthEvaluationRuleRecord, error) {
	return nil, nil
}
func (healthChecks targetGroupServiceTestHealthRepository) CreateHealthEvaluationRule(context.Context, repo.HealthEvaluationRuleRecord, []repo.HealthEventRecord) error {
	return nil
}
func (healthChecks targetGroupServiceTestHealthRepository) DeleteHealthEvaluationRulesForDNSRecord(context.Context, string, string, string) error {
	return nil
}

type targetGroupServiceTestAuditRepository struct {
	store *targetGroupServiceTestStore
}

func (audits targetGroupServiceTestAuditRepository) CreateAuditLog(_ context.Context, audit repo.AuditLogRecord) error {
	audits.store.auditLogs = append(audits.store.auditLogs, audit)
	return nil
}

func targetGroupServiceTestIdentity() InternalIdentity {
	return InternalIdentity{
		UserID:         "user_1",
		OrganizationID: "org_1",
		Permissions:    []string{string(domain.PermissionTargetsManage)},
	}
}
