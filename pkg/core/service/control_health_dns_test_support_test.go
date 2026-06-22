package service

import (
	"context"

	"github.com/noxaaa/prism-oss/pkg/core/dns"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

type healthDNSTestProvider struct {
	inputs []dns.ApplyRecordInput
	err    error
}

func (provider *healthDNSTestProvider) ApplyRecord(_ context.Context, input dns.ApplyRecordInput) error {
	provider.inputs = append(provider.inputs, input)
	return provider.err
}

func (provider *healthDNSTestProvider) calls() int {
	return len(provider.inputs)
}

func (provider *healthDNSTestProvider) lastInput() dns.ApplyRecordInput {
	if len(provider.inputs) == 0 {
		return dns.ApplyRecordInput{}
	}
	return provider.inputs[len(provider.inputs)-1]
}

type recordingHealthActionExecutor struct {
	executed []recordingHealthEventAction
}

type recordingHealthEventAction struct {
	EventID         string
	RuleID          string
	HealthCheckName string
	Status          string
	ConfigJSON      string
}

func (executor *recordingHealthActionExecutor) Supports(eventType string) bool {
	return eventType == "WEBHOOK"
}

func (executor *recordingHealthActionExecutor) BuildAction(_ context.Context, _ repo.Repositories, input HealthActionExecutionInput) (any, bool, error) {
	return recordingHealthEventAction{EventID: input.Event.ID, RuleID: input.Rule.ID, HealthCheckName: input.HealthCheck.Name, Status: input.Result.Status, ConfigJSON: input.Event.ConfigJSON}, true, nil
}

func (executor *recordingHealthActionExecutor) Execute(_ context.Context, action any) error {
	executor.executed = append(executor.executed, action.(recordingHealthEventAction))
	return nil
}

type healthDNSTestStore struct {
	results               []repo.HealthResultRecord
	rules                 []repo.HealthEvaluationRuleRecord
	credential            repo.DNSCredentialRecord
	record                repo.DNSRecordRecord
	createdDNSRecord      repo.DNSRecordRecord
	updatedDNSRecord      repo.DNSRecordRecord
	updateDNSRecordErr    error
	monitor               repo.MonitorRecord
	monitors              []repo.MonitorRecord
	checks                []repo.HealthCheckRecord
	monitorGroups         map[string]repo.MonitorGroupRecord
	targetGroups          map[string]repo.TargetGroupRecord
	targetsByID           map[string]repo.TargetRecord
	syncedHealthTargets   bool
	deletedHealthCheckID  string
	deletedRulesRecordID  string
	deletedDNSRecordID    string
	deletedCredentialID   string
	deletedMonitorID      string
	deletedMonitorGroupID string
}

func (store *healthDNSTestStore) WithinTx(ctx context.Context, fn func(context.Context, repo.Repositories) error) error {
	return fn(ctx, healthDNSTestRepositories{store: store})
}

type healthDNSTestRepositories struct {
	store *healthDNSTestStore
}

func (repositories healthDNSTestRepositories) Users() repo.UserRepository                 { return nil }
func (repositories healthDNSTestRepositories) Organizations() repo.OrganizationRepository { return nil }
func (repositories healthDNSTestRepositories) Members() repo.MemberRepository             { return nil }
func (repositories healthDNSTestRepositories) Roles() repo.RoleRepository                 { return nil }
func (repositories healthDNSTestRepositories) NodeGroups() repo.NodeGroupRepository       { return nil }
func (repositories healthDNSTestRepositories) Nodes() repo.NodeRepository                 { return nil }
func (repositories healthDNSTestRepositories) MonitorGroups() repo.MonitorGroupRepository {
	return healthDNSTestMonitorGroupRepository(repositories)
}
func (repositories healthDNSTestRepositories) Monitors() repo.MonitorRepository {
	return healthDNSTestMonitorRepository(repositories)
}
func (repositories healthDNSTestRepositories) HealthChecks() repo.HealthCheckRepository {
	return healthDNSTestHealthRepository(repositories)
}
func (repositories healthDNSTestRepositories) DNSCredentials() repo.DNSCredentialRepository {
	return healthDNSTestDNSCredentialRepository(repositories)
}
func (repositories healthDNSTestRepositories) DNSRecords() repo.DNSRecordRepository {
	return healthDNSTestDNSRecordRepository(repositories)
}
func (repositories healthDNSTestRepositories) Targets() repo.TargetRepository {
	return healthDNSTestTargetRepository(repositories)
}
func (repositories healthDNSTestRepositories) TargetGroups() repo.TargetGroupRepository {
	return healthDNSTestTargetGroupRepository(repositories)
}
func (repositories healthDNSTestRepositories) Rules() repo.RuleRepository   { return nil }
func (repositories healthDNSTestRepositories) Quotas() repo.QuotaRepository { return nil }
func (repositories healthDNSTestRepositories) AgentRegistrationTokens() repo.AgentRegistrationTokenRepository {
	return nil
}
func (repositories healthDNSTestRepositories) AgentCredentials() repo.AgentCredentialRepository {
	return nil
}
func (repositories healthDNSTestRepositories) AuditLogs() repo.AuditLogRepository {
	return healthDNSTestAuditRepository{}
}

type healthDNSTestMonitorRepository struct {
	store *healthDNSTestStore
}

type healthDNSTestMonitorGroupRepository struct {
	store *healthDNSTestStore
}

func (repository healthDNSTestMonitorGroupRepository) ListMonitorGroupsByOrganization(_ context.Context, organizationID string) ([]repo.MonitorGroupRecord, error) {
	result := make([]repo.MonitorGroupRecord, 0, len(repository.store.monitorGroups))
	for _, group := range repository.store.monitorGroups {
		if group.OrganizationID == organizationID && group.DeletedAt == "" {
			result = append(result, group)
		}
	}
	return result, nil
}

func (repository healthDNSTestMonitorGroupRepository) FindMonitorGroupByID(_ context.Context, organizationID string, monitorGroupID string) (repo.MonitorGroupRecord, error) {
	group, ok := repository.store.monitorGroups[monitorGroupID]
	if ok && group.OrganizationID == organizationID && group.DeletedAt == "" {
		return group, nil
	}
	return repo.MonitorGroupRecord{}, repo.ErrNotFound
}

func (repository healthDNSTestMonitorGroupRepository) CreateMonitorGroup(context.Context, repo.MonitorGroupRecord) error {
	return nil
}

func (repository healthDNSTestMonitorGroupRepository) UpdateMonitorGroup(context.Context, repo.MonitorGroupRecord) error {
	return nil
}

func (repository healthDNSTestMonitorGroupRepository) DeleteMonitorGroup(_ context.Context, _ string, monitorGroupID string, _ string) error {
	repository.store.deletedMonitorGroupID = monitorGroupID
	return nil
}

func (repository healthDNSTestMonitorRepository) ListMonitorsByOrganization(_ context.Context, organizationID string) ([]repo.MonitorRecord, error) {
	if len(repository.store.monitors) > 0 {
		result := make([]repo.MonitorRecord, 0, len(repository.store.monitors))
		for _, monitor := range repository.store.monitors {
			if monitor.OrganizationID == organizationID {
				result = append(result, monitor)
			}
		}
		return result, nil
	}
	return []repo.MonitorRecord{repository.store.monitor}, nil
}

func (repository healthDNSTestMonitorRepository) FindMonitorByID(_ context.Context, organizationID string, monitorID string) (repo.MonitorRecord, error) {
	if repository.store.monitor.OrganizationID == organizationID && repository.store.monitor.ID == monitorID {
		return repository.store.monitor, nil
	}
	return repo.MonitorRecord{}, repo.ErrNotFound
}

func (repository healthDNSTestMonitorRepository) CreateMonitor(context.Context, repo.MonitorRecord, []string, string, func() string) error {
	return nil
}

func (repository healthDNSTestMonitorRepository) UpdateMonitor(context.Context, repo.MonitorRecord, bool, []string, string, func() string) error {
	return nil
}

func (repository healthDNSTestMonitorRepository) MarkMonitorAgentConnected(context.Context, string, string, string) error {
	return nil
}

func (repository healthDNSTestMonitorRepository) MarkMonitorAgentDisconnected(context.Context, string, string, string) error {
	return nil
}

func (repository healthDNSTestMonitorRepository) RecordMonitorConfigAck(context.Context, string, string, int, string) error {
	return nil
}

func (repository healthDNSTestMonitorRepository) DeleteMonitor(_ context.Context, _ string, monitorID string, _ string) error {
	repository.store.deletedMonitorID = monitorID
	return nil
}

type healthDNSTestTargetGroupRepository struct {
	store *healthDNSTestStore
}

type healthDNSTestTargetRepository struct {
	store *healthDNSTestStore
}

func (repository healthDNSTestTargetRepository) ListTargetsByOrganization(_ context.Context, organizationID string) ([]repo.TargetRecord, error) {
	result := make([]repo.TargetRecord, 0, len(repository.store.targetsByID))
	for _, target := range repository.store.targetsByID {
		if target.OrganizationID == organizationID && target.DeletedAt == "" {
			result = append(result, target)
		}
	}
	return result, nil
}

func (repository healthDNSTestTargetRepository) FindTargetByID(_ context.Context, organizationID string, targetID string) (repo.TargetRecord, error) {
	target, ok := repository.store.targetsByID[targetID]
	if ok && target.OrganizationID == organizationID && target.DeletedAt == "" {
		return target, nil
	}
	return repo.TargetRecord{}, repo.ErrNotFound
}

func (repository healthDNSTestTargetRepository) CreateTarget(_ context.Context, target repo.TargetRecord) error {
	if repository.store.targetsByID == nil {
		repository.store.targetsByID = make(map[string]repo.TargetRecord)
	}
	repository.store.targetsByID[target.ID] = target
	return nil
}

func (repository healthDNSTestTargetRepository) UpdateTarget(_ context.Context, target repo.TargetRecord) error {
	if _, ok := repository.store.targetsByID[target.ID]; !ok {
		return repo.ErrNotFound
	}
	repository.store.targetsByID[target.ID] = target
	return nil
}

func (repository healthDNSTestTargetRepository) DeleteTarget(_ context.Context, _ string, targetID string, deletedAt string) error {
	target, ok := repository.store.targetsByID[targetID]
	if !ok {
		return repo.ErrNotFound
	}
	target.DeletedAt = deletedAt
	repository.store.targetsByID[targetID] = target
	return nil
}

func (repository healthDNSTestTargetGroupRepository) ListTargetGroupsByOrganization(context.Context, string) ([]repo.TargetGroupRecord, error) {
	return nil, nil
}

func (repository healthDNSTestTargetGroupRepository) FindTargetGroupByID(_ context.Context, organizationID string, targetGroupID string) (repo.TargetGroupRecord, error) {
	group, ok := repository.store.targetGroups[targetGroupID]
	if ok && group.OrganizationID == organizationID {
		return group, nil
	}
	return repo.TargetGroupRecord{}, repo.ErrNotFound
}

func (repository healthDNSTestTargetGroupRepository) CreateTargetGroup(context.Context, repo.TargetGroupRecord, []repo.TargetGroupMemberRecord, string, func() string) error {
	return nil
}

func (repository healthDNSTestTargetGroupRepository) UpdateTargetGroup(context.Context, repo.TargetGroupRecord, []repo.TargetGroupMemberRecord, string, func() string) error {
	return nil
}

func (repository healthDNSTestTargetGroupRepository) DeleteTargetGroup(context.Context, string, string, string) error {
	return nil
}

type healthDNSTestHealthRepository struct {
	store *healthDNSTestStore
}

func (repository healthDNSTestHealthRepository) ListHealthChecksByOrganization(context.Context, string) ([]repo.HealthCheckRecord, error) {
	return repository.store.checks, nil
}

func (repository healthDNSTestHealthRepository) FindHealthCheckByID(_ context.Context, organizationID string, healthCheckID string) (repo.HealthCheckRecord, error) {
	for _, check := range repository.store.checks {
		if check.OrganizationID == organizationID && check.ID == healthCheckID {
			return check, nil
		}
	}
	return repo.HealthCheckRecord{}, repo.ErrNotFound
}

func (repository healthDNSTestHealthRepository) CreateHealthCheck(context.Context, repo.HealthCheckRecord, []repo.HealthCheckTargetRecord, []repo.HealthCheckMonitorScopeRecord, string, func() string) error {
	return nil
}

func (repository healthDNSTestHealthRepository) UpdateHealthCheck(context.Context, repo.HealthCheckRecord, []repo.HealthCheckTargetRecord, []repo.HealthCheckMonitorScopeRecord, string, func() string) error {
	return nil
}

func (repository healthDNSTestHealthRepository) SyncHealthCheckTargets(_ context.Context, organizationID string, healthCheckID string, targets []repo.HealthCheckTargetRecord, _ string, nextID func() string) error {
	repository.store.syncedHealthTargets = true
	for checkIndex := range repository.store.checks {
		check := &repository.store.checks[checkIndex]
		if check.OrganizationID != organizationID || check.ID != healthCheckID {
			continue
		}
		existing := make(map[string]repo.HealthCheckTargetRecord)
		updated := make([]repo.HealthCheckTargetRecord, 0)
		for _, target := range check.Targets {
			if target.ScopeType != "TARGET_GROUP" {
				updated = append(updated, target)
				continue
			}
			existing[target.TargetID+"\x00"+target.TargetGroupID] = target
		}
		for _, target := range targets {
			if target.ScopeType != "TARGET_GROUP" {
				continue
			}
			key := target.TargetID + "\x00" + target.TargetGroupID
			merged, ok := existing[key]
			if !ok {
				merged = repo.HealthCheckTargetRecord{
					ID:             nextID(),
					OrganizationID: organizationID,
					HealthCheckID:  healthCheckID,
					ScopeType:      "TARGET_GROUP",
					TargetID:       target.TargetID,
					TargetGroupID:  target.TargetGroupID,
				}
			}
			if targetRecord, ok := repository.store.targetsByID[target.TargetID]; ok {
				merged.TargetName = targetRecord.Name
				merged.TargetHost = targetRecord.Host
				merged.TargetPort = targetRecord.Port
			}
			updated = append(updated, merged)
		}
		check.Targets = updated
		return nil
	}
	return repo.ErrNotFound
}

func (repository healthDNSTestHealthRepository) DeleteHealthCheck(_ context.Context, _ string, healthCheckID string, _ string) error {
	repository.store.deletedHealthCheckID = healthCheckID
	return nil
}

func (repository healthDNSTestHealthRepository) ListHealthResults(context.Context, string, string, int) ([]repo.HealthResultRecord, error) {
	return nil, nil
}

func (repository healthDNSTestHealthRepository) ListLatestHealthResultsByCheck(_ context.Context, organizationID string, healthCheckID string) ([]repo.HealthResultRecord, error) {
	latestByPair := map[string]repo.HealthResultRecord{}
	for _, result := range repository.store.results {
		if result.OrganizationID != organizationID || result.HealthCheckID != healthCheckID {
			continue
		}
		key := result.MonitorID + "\x00" + result.HealthCheckTargetID
		current, ok := latestByPair[key]
		if !ok || result.ObservedAt > current.ObservedAt || (result.ObservedAt == current.ObservedAt && result.CreatedAt > current.CreatedAt) {
			latestByPair[key] = result
		}
	}
	out := make([]repo.HealthResultRecord, 0, len(latestByPair))
	for _, result := range latestByPair {
		out = append(out, result)
	}
	return out, nil
}

func (repository healthDNSTestHealthRepository) RecordHealthResults(_ context.Context, _ string, results []repo.HealthResultRecord) error {
	repository.store.results = append(repository.store.results, results...)
	return nil
}

func (repository healthDNSTestHealthRepository) ListHealthEvaluationRulesByCheck(_ context.Context, organizationID string, healthCheckID string) ([]repo.HealthEvaluationRuleRecord, error) {
	matches := make([]repo.HealthEvaluationRuleRecord, 0)
	for _, rule := range repository.store.rules {
		if rule.OrganizationID == organizationID && rule.HealthCheckID == healthCheckID {
			matches = append(matches, rule)
		}
	}
	return matches, nil
}

func (repository healthDNSTestHealthRepository) CreateHealthEvaluationRule(context.Context, repo.HealthEvaluationRuleRecord, []repo.HealthEventRecord) error {
	return nil
}

func (repository healthDNSTestHealthRepository) DeleteHealthEvaluationRulesForDNSRecord(_ context.Context, _ string, dnsRecordID string, _ string) error {
	repository.store.deletedRulesRecordID = dnsRecordID
	return nil
}

type healthDNSTestDNSCredentialRepository struct {
	store *healthDNSTestStore
}

func (repository healthDNSTestDNSCredentialRepository) ListDNSCredentialsByOrganization(context.Context, string) ([]repo.DNSCredentialRecord, error) {
	return nil, nil
}

func (repository healthDNSTestDNSCredentialRepository) FindDNSCredentialByID(_ context.Context, organizationID string, credentialID string) (repo.DNSCredentialRecord, error) {
	if repository.store.credential.OrganizationID == organizationID && repository.store.credential.ID == credentialID {
		return repository.store.credential, nil
	}
	return repo.DNSCredentialRecord{}, repo.ErrNotFound
}

func (repository healthDNSTestDNSCredentialRepository) CreateDNSCredential(context.Context, repo.DNSCredentialRecord) error {
	return nil
}

func (repository healthDNSTestDNSCredentialRepository) UpdateDNSCredential(context.Context, repo.DNSCredentialRecord, bool) error {
	return nil
}

func (repository healthDNSTestDNSCredentialRepository) DeleteDNSCredential(context.Context, string, string, string) error {
	repository.store.deletedCredentialID = repository.store.credential.ID
	return nil
}

type healthDNSTestDNSRecordRepository struct {
	store *healthDNSTestStore
}

func (repository healthDNSTestDNSRecordRepository) ListDNSRecordsByOrganization(context.Context, string) ([]repo.DNSRecordRecord, error) {
	if repository.store.record.ID == "" {
		return nil, nil
	}
	return []repo.DNSRecordRecord{repository.store.record}, nil
}

func (repository healthDNSTestDNSRecordRepository) FindDNSRecordByID(_ context.Context, organizationID string, recordID string) (repo.DNSRecordRecord, error) {
	if repository.store.record.OrganizationID == organizationID && repository.store.record.ID == recordID {
		return repository.store.record, nil
	}
	return repo.DNSRecordRecord{}, repo.ErrNotFound
}

func (repository healthDNSTestDNSRecordRepository) CreateDNSRecord(_ context.Context, record repo.DNSRecordRecord) error {
	repository.store.createdDNSRecord = record
	repository.store.record = record
	return nil
}

func (repository healthDNSTestDNSRecordRepository) UpdateDNSRecord(_ context.Context, record repo.DNSRecordRecord) error {
	if repository.store.updateDNSRecordErr != nil {
		return repository.store.updateDNSRecordErr
	}
	repository.store.updatedDNSRecord = record
	repository.store.record = record
	return nil
}

func (repository healthDNSTestDNSRecordRepository) UpdateDNSRecordLastApplied(_ context.Context, organizationID string, recordID string, values string, appliedAt string) error {
	if repository.store.record.OrganizationID != organizationID || repository.store.record.ID != recordID {
		return repo.ErrNotFound
	}
	repository.store.record.LastAppliedValuesJSON = values
	repository.store.record.LastAppliedAt = appliedAt
	return nil
}

func (repository healthDNSTestDNSRecordRepository) DeleteDNSRecord(_ context.Context, organizationID string, recordID string, _ string) error {
	if repository.store.record.OrganizationID != organizationID || repository.store.record.ID != recordID {
		return repo.ErrNotFound
	}
	repository.store.deletedDNSRecordID = recordID
	repository.store.record = repo.DNSRecordRecord{}
	return nil
}

func healthDNSTestIdentity(permissions ...string) InternalIdentity {
	return InternalIdentity{
		UserID:         "user_1",
		OrganizationID: "org_1",
		Permissions:    permissions,
	}
}

type healthDNSTestAuditRepository struct{}

func (repository healthDNSTestAuditRepository) CreateAuditLog(context.Context, repo.AuditLogRecord) error {
	return nil
}

type healthDNSTestAuthorizer struct{}

func (healthDNSTestAuthorizer) HasPermission(identity InternalIdentity, permission string) bool {
	return stringSliceHas(identity.Permissions, permission)
}

func (healthDNSTestAuthorizer) AllowedNodeGroupIDs(InternalIdentity, string) map[string]bool {
	return map[string]bool{}
}

func (healthDNSTestAuthorizer) EnsureCanDelegateRoleScopes(context.Context, repo.Repositories, InternalIdentity, []repo.ResourceScopeRecord) error {
	return nil
}
