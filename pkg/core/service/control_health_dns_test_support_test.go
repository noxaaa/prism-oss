package service

import (
	"context"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

type recordingHealthActionExecutor struct {
	executed []recordingHealthEventAction
	types    []string
}

type recordingHealthEventAction struct {
	EventID         string
	RuleID          string
	HealthCheckName string
	Status          string
	ConfigJSON      string
}

func (executor *recordingHealthActionExecutor) Supports(eventType string) bool {
	eventType = normalizeHealthActionType(eventType)
	for _, candidate := range executor.types {
		if eventType == normalizeHealthActionType(candidate) {
			return true
		}
	}
	return eventType == "WEBHOOK"
}

func (executor *recordingHealthActionExecutor) HealthActionTypes() []string {
	if len(executor.types) == 0 {
		return []string{"WEBHOOK"}
	}
	return executor.types
}

func (executor *recordingHealthActionExecutor) BuildAction(_ context.Context, _ repo.Repositories, input HealthActionExecutionInput) (any, bool, error) {
	return recordingHealthEventAction{EventID: input.Event.ID, RuleID: input.Rule.ID, HealthCheckName: input.HealthCheck.Name, Status: input.Result.Status, ConfigJSON: input.Event.ConfigJSON}, true, nil
}

func (executor *recordingHealthActionExecutor) Execute(_ context.Context, action any) error {
	executor.executed = append(executor.executed, action.(recordingHealthEventAction))
	return nil
}

type healthDNSTestStore struct {
	results                                         []repo.HealthResultRecord
	rules                                           []repo.HealthEvaluationRuleRecord
	createdHealthRule                               repo.HealthEvaluationRuleRecord
	createdHealthEvents                             []repo.HealthEventRecord
	credential                                      repo.DNSCredentialRecord
	createdCredential                               repo.DNSCredentialRecord
	updatedCredential                               repo.DNSCredentialRecord
	credentialZones                                 []repo.DNSCredentialZoneRecord
	managedRecords                                  []repo.DNSManagedRecordRecord
	nodes                                           []repo.NodeRecord
	nodeGroups                                      map[string]repo.NodeGroupRecord
	monitor                                         repo.MonitorRecord
	monitors                                        []repo.MonitorRecord
	checks                                          []repo.HealthCheckRecord
	latestHealthBatchCalls, latestHealthSingleCalls int
	monitorGroups                                   map[string]repo.MonitorGroupRecord
	targetGroups                                    map[string]repo.TargetGroupRecord
	targetsByID                                     map[string]repo.TargetRecord
	forwardingRules                                 []repo.RuleRecord
	syncedHealthTargets, respectContextCancellation bool
	deletedHealthCheckID, deletedDNSManagedRecordID string
	deletedCredentialID                             string
	deletedMonitorID, deletedMonitorGroupID         string
	onLockDNSManagedRecord                          func(recordID string)
	lockedDNSManagedRecords, lockedDNSInstances     []string
	notificationChannels                            []repo.NotificationChannelRecord
	notificationDeliveries                          []repo.NotificationDeliveryRecord
	notificationLookupErr, notificationDeliveryErr  error
	txDepth                                         int
}

func (store *healthDNSTestStore) WithinTx(ctx context.Context, fn func(context.Context, repo.Repositories) error) error {
	if store.respectContextCancellation {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	store.txDepth++
	defer func() {
		store.txDepth--
	}()
	return fn(ctx, healthDNSTestRepositories{store: store})
}

type healthDNSTestRepositories struct {
	store *healthDNSTestStore
}

func (repositories healthDNSTestRepositories) Users() repo.UserRepository                 { return nil }
func (repositories healthDNSTestRepositories) Organizations() repo.OrganizationRepository { return nil }
func (repositories healthDNSTestRepositories) Members() repo.MemberRepository             { return nil }
func (repositories healthDNSTestRepositories) Roles() repo.RoleRepository                 { return nil }
func (repositories healthDNSTestRepositories) NodeGroups() repo.NodeGroupRepository {
	return healthDNSTestNodeGroupRepository(repositories)
}
func (repositories healthDNSTestRepositories) Nodes() repo.NodeRepository {
	return healthDNSTestNodeRepository(repositories)
}
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
func (repositories healthDNSTestRepositories) Rules() repo.RuleRepository {
	return healthDNSTestRuleRepository(repositories)
}
func (repositories healthDNSTestRepositories) Quotas() repo.QuotaRepository { return nil }
func (repositories healthDNSTestRepositories) AgentRegistrationTokens() repo.AgentRegistrationTokenRepository {
	return nil
}
func (repositories healthDNSTestRepositories) NodeEnrollmentProfiles() repo.NodeEnrollmentProfileRepository {
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

type healthDNSTestNodeRepository struct {
	store *healthDNSTestStore
}

type healthDNSTestNodeGroupRepository struct {
	store *healthDNSTestStore
}

func (repository healthDNSTestNodeGroupRepository) ListNodeGroupsByOrganization(_ context.Context, organizationID string) ([]repo.NodeGroupRecord, error) {
	result := make([]repo.NodeGroupRecord, 0, len(repository.store.nodeGroups))
	for _, group := range repository.store.nodeGroups {
		if group.OrganizationID == organizationID && group.DeletedAt == "" {
			result = append(result, group)
		}
	}
	return result, nil
}

func (repository healthDNSTestNodeGroupRepository) FindNodeGroupByID(_ context.Context, organizationID string, nodeGroupID string) (repo.NodeGroupRecord, error) {
	group, ok := repository.store.nodeGroups[nodeGroupID]
	if ok && group.OrganizationID == organizationID && group.DeletedAt == "" {
		return group, nil
	}
	return repo.NodeGroupRecord{}, repo.ErrNotFound
}

func (repository healthDNSTestNodeGroupRepository) CreateNodeGroup(_ context.Context, group repo.NodeGroupRecord) error {
	if repository.store.nodeGroups == nil {
		repository.store.nodeGroups = make(map[string]repo.NodeGroupRecord)
	}
	repository.store.nodeGroups[group.ID] = group
	return nil
}

func (repository healthDNSTestNodeGroupRepository) UpdateNodeGroup(_ context.Context, group repo.NodeGroupRecord) error {
	if repository.store.nodeGroups == nil {
		return repo.ErrNotFound
	}
	if _, ok := repository.store.nodeGroups[group.ID]; !ok {
		return repo.ErrNotFound
	}
	repository.store.nodeGroups[group.ID] = group
	return nil
}

func (repository healthDNSTestNodeGroupRepository) DeleteNodeGroup(_ context.Context, organizationID string, nodeGroupID string, deletedAt string) error {
	if repository.store.nodeGroups == nil {
		return repo.ErrNotFound
	}
	group, ok := repository.store.nodeGroups[nodeGroupID]
	if !ok || group.OrganizationID != organizationID {
		return repo.ErrNotFound
	}
	group.DeletedAt = deletedAt
	repository.store.nodeGroups[nodeGroupID] = group
	return nil
}

func (repository healthDNSTestNodeRepository) ListNodesByOrganization(_ context.Context, organizationID string) ([]repo.NodeRecord, error) {
	nodes := make([]repo.NodeRecord, 0, len(repository.store.nodes))
	for _, node := range repository.store.nodes {
		if node.OrganizationID == organizationID && node.DeletedAt == "" {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

func (repository healthDNSTestNodeRepository) FindNodeByID(_ context.Context, organizationID string, nodeID string) (repo.NodeRecord, error) {
	for _, node := range repository.store.nodes {
		if node.OrganizationID == organizationID && node.ID == nodeID && node.DeletedAt == "" {
			return node, nil
		}
	}
	return repo.NodeRecord{}, repo.ErrNotFound
}

func (repository healthDNSTestNodeRepository) CreateNode(context.Context, repo.NodeRecord, []string, []repo.NodeListenIPRecord, []repo.NodePortRangeRecord, string, func() string) error {
	return nil
}

func (repository healthDNSTestNodeRepository) UpdateNode(context.Context, repo.NodeRecord, bool, []string, bool, []repo.NodeListenIPRecord, bool, []repo.NodePortRangeRecord, string, func() string) error {
	return nil
}

func (repository healthDNSTestNodeRepository) MarkNodeAgentConnected(_ context.Context, organizationID string, nodeID string, now string) error {
	for index := range repository.store.nodes {
		node := &repository.store.nodes[index]
		if node.OrganizationID == organizationID && node.ID == nodeID && node.DeletedAt == "" {
			node.Status = "ONLINE"
			node.LastSeenAt = now
			node.UpdatedAt = now
			return nil
		}
	}
	return repo.ErrNotFound
}

func (repository healthDNSTestNodeRepository) UpdateNodeAgentVersion(context.Context, string, string, repo.NodeAgentVersionRecord, string) error {
	return nil
}

func (repository healthDNSTestNodeRepository) UpdateNodeAgentUpdatePolicy(context.Context, string, string, bool, string) error {
	return nil
}

func (repository healthDNSTestNodeRepository) MarkNodeAgentUpdateRequested(context.Context, string, string, string, string) error {
	return nil
}

func (repository healthDNSTestNodeRepository) MarkNodeAgentUpdateSatisfied(context.Context, string, string, string, string) error {
	return nil
}

func (repository healthDNSTestNodeRepository) RecordNodeAgentUpdateResult(context.Context, string, string, string, string, string) error {
	return nil
}

func (repository healthDNSTestNodeRepository) MarkNodeAgentDisconnected(_ context.Context, organizationID string, nodeID string, now string) error {
	for index := range repository.store.nodes {
		node := &repository.store.nodes[index]
		if node.OrganizationID == organizationID && node.ID == nodeID && node.DeletedAt == "" {
			node.Status = "OFFLINE"
			node.LastSeenAt = now
			node.UpdatedAt = now
			return nil
		}
	}
	return repo.ErrNotFound
}

func (repository healthDNSTestNodeRepository) UpsertAutoNodeDNSPublishAddress(_ context.Context, organizationID string, nodeID string, addressType string, address string, now string, nextID func() string) error {
	for nodeIndex := range repository.store.nodes {
		node := &repository.store.nodes[nodeIndex]
		if node.OrganizationID != organizationID || node.ID != nodeID || node.DeletedAt != "" {
			continue
		}
		for addressIndex := range node.DNSPublishAddresses {
			candidate := &node.DNSPublishAddresses[addressIndex]
			if candidate.Source == "AUTO" && candidate.AddressType == addressType && candidate.Address != address && candidate.Enabled {
				candidate.Enabled = false
				candidate.UpdatedAt = now
			}
		}
		for addressIndex := range node.DNSPublishAddresses {
			candidate := &node.DNSPublishAddresses[addressIndex]
			if candidate.Source == "AUTO" && candidate.AddressType == addressType && candidate.Address == address {
				candidate.Enabled = true
				candidate.ObservedAt = now
				candidate.UpdatedAt = now
				return nil
			}
		}
		node.DNSPublishAddresses = append(node.DNSPublishAddresses, repo.NodeDNSPublishAddressRecord{
			ID:             nextID(),
			OrganizationID: organizationID,
			NodeID:         nodeID,
			AddressType:    addressType,
			Address:        address,
			Source:         "AUTO",
			Enabled:        true,
			ObservedAt:     now,
			CreatedAt:      now,
			UpdatedAt:      now,
		})
		return nil
	}
	return repo.ErrNotFound
}

func (repository healthDNSTestNodeRepository) RecordNodeConfigAck(context.Context, string, string, repo.NodeConfigAckRecord, string) error {
	return nil
}

func (repository healthDNSTestNodeRepository) EnsureDesiredConfigVersionAtLeast(context.Context, string, string, int, string) error {
	return nil
}

func (repository healthDNSTestNodeRepository) IncrementDesiredConfigForNode(context.Context, string, string, string) error {
	return nil
}

func (repository healthDNSTestNodeRepository) IncrementDesiredConfigForNodeGroup(context.Context, string, string, string) error {
	return nil
}

func (repository healthDNSTestNodeRepository) DeleteNode(_ context.Context, organizationID string, nodeID string, deletedAt string) error {
	for index := range repository.store.nodes {
		node := &repository.store.nodes[index]
		if node.OrganizationID == organizationID && node.ID == nodeID && node.DeletedAt == "" {
			node.DeletedAt = deletedAt
			node.UpdatedAt = deletedAt
			return nil
		}
	}
	return repo.ErrNotFound
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
	repository.store.latestHealthSingleCalls++
	resultsByCheck, err := repository.ListLatestHealthResultsByChecks(context.Background(), organizationID, []string{healthCheckID})
	if err != nil {
		return nil, err
	}
	return resultsByCheck[healthCheckID], nil
}

func (repository healthDNSTestHealthRepository) ListLatestHealthResultsByChecks(_ context.Context, organizationID string, healthCheckIDs []string) (map[string][]repo.HealthResultRecord, error) {
	repository.store.latestHealthBatchCalls++
	allowed := make(map[string]bool, len(healthCheckIDs))
	for _, healthCheckID := range healthCheckIDs {
		allowed[healthCheckID] = true
	}
	latestByPair := map[string]repo.HealthResultRecord{}
	for _, result := range repository.store.results {
		if result.OrganizationID != organizationID || !allowed[result.HealthCheckID] {
			continue
		}
		key := result.HealthCheckID + "\x00" + result.MonitorID + "\x00" + result.HealthCheckTargetID
		current, ok := latestByPair[key]
		if !ok || result.ObservedAt > current.ObservedAt || (result.ObservedAt == current.ObservedAt && result.CreatedAt > current.CreatedAt) {
			latestByPair[key] = result
		}
	}
	out := make(map[string][]repo.HealthResultRecord, len(healthCheckIDs))
	for _, result := range latestByPair {
		out[result.HealthCheckID] = append(out[result.HealthCheckID], result)
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

func (repository healthDNSTestHealthRepository) CreateHealthEvaluationRule(_ context.Context, rule repo.HealthEvaluationRuleRecord, events []repo.HealthEventRecord) error {
	rule.Events = append([]repo.HealthEventRecord(nil), events...)
	repository.store.createdHealthRule = rule
	repository.store.createdHealthEvents = append([]repo.HealthEventRecord(nil), events...)
	repository.store.rules = append(repository.store.rules, rule)
	return nil
}

type healthDNSTestDNSCredentialRepository struct {
	store *healthDNSTestStore
}

func (repository healthDNSTestDNSCredentialRepository) ListDNSCredentialsByOrganization(context.Context, string) ([]repo.DNSCredentialRecord, error) {
	if repository.store.credential.ID == "" {
		return nil, nil
	}
	return []repo.DNSCredentialRecord{repository.store.credential}, nil
}

func (repository healthDNSTestDNSCredentialRepository) FindDNSCredentialByID(_ context.Context, organizationID string, credentialID string) (repo.DNSCredentialRecord, error) {
	for _, credential := range []repo.DNSCredentialRecord{repository.store.credential, repository.store.createdCredential, repository.store.updatedCredential} {
		if credential.OrganizationID == organizationID && credential.ID == credentialID {
			return credential, nil
		}
	}
	return repo.DNSCredentialRecord{}, repo.ErrNotFound
}

func (repository healthDNSTestDNSCredentialRepository) CreateDNSCredential(_ context.Context, credential repo.DNSCredentialRecord) error {
	repository.store.createdCredential = credential
	repository.store.credential = credential
	return nil
}

func (repository healthDNSTestDNSCredentialRepository) UpdateDNSCredential(_ context.Context, credential repo.DNSCredentialRecord, _ bool) error {
	repository.store.updatedCredential = credential
	repository.store.credential = credential
	return nil
}

func (repository healthDNSTestDNSCredentialRepository) ListDNSCredentialZonesByOrganization(_ context.Context, organizationID string) ([]repo.DNSCredentialZoneRecord, error) {
	zones := make([]repo.DNSCredentialZoneRecord, 0)
	for _, zone := range repository.store.credentialZones {
		if zone.OrganizationID == organizationID {
			zones = append(zones, zone)
		}
	}
	return zones, nil
}

func (repository healthDNSTestDNSCredentialRepository) ListDNSCredentialZonesByCredential(_ context.Context, organizationID string, credentialID string) ([]repo.DNSCredentialZoneRecord, error) {
	zones := make([]repo.DNSCredentialZoneRecord, 0)
	for _, zone := range repository.store.credentialZones {
		if zone.OrganizationID == organizationID && zone.DNSCredentialID == credentialID {
			zones = append(zones, zone)
		}
	}
	return zones, nil
}

func (repository healthDNSTestDNSCredentialRepository) FindDNSCredentialZoneByID(_ context.Context, organizationID string, credentialZoneID string) (repo.DNSCredentialZoneRecord, error) {
	for _, zone := range repository.store.credentialZones {
		if zone.OrganizationID == organizationID && zone.ID == credentialZoneID {
			return zone, nil
		}
	}
	return repo.DNSCredentialZoneRecord{}, repo.ErrNotFound
}

func (repository healthDNSTestDNSCredentialRepository) ReplaceDNSCredentialZones(_ context.Context, organizationID string, credentialID string, zones []repo.DNSCredentialZoneRecord, now string, nextID func() string) error {
	seen := map[string]bool{}
	for _, zone := range zones {
		seen[zone.ZoneID] = true
		matched := false
		for index := range repository.store.credentialZones {
			existing := &repository.store.credentialZones[index]
			if existing.OrganizationID == organizationID && existing.DNSCredentialID == credentialID && existing.ZoneID == zone.ZoneID {
				existing.ZoneName = zone.ZoneName
				existing.Status = zone.Status
				existing.LastSyncedAt = now
				existing.UpdatedAt = now
				matched = true
			}
		}
		if !matched {
			if zone.ID == "" {
				zone.ID = nextID()
			}
			zone.OrganizationID = organizationID
			zone.DNSCredentialID = credentialID
			zone.LastSyncedAt = now
			zone.CreatedAt = now
			zone.UpdatedAt = now
			repository.store.credentialZones = append(repository.store.credentialZones, zone)
		}
	}
	for index := range repository.store.credentialZones {
		zone := &repository.store.credentialZones[index]
		if zone.OrganizationID == organizationID && zone.DNSCredentialID == credentialID && !seen[zone.ZoneID] {
			zone.Status = "UNAVAILABLE"
			zone.LastSyncedAt = now
			zone.UpdatedAt = now
		}
	}
	return nil
}

func (repository healthDNSTestDNSCredentialRepository) DeleteDNSCredential(context.Context, string, string, string) error {
	repository.store.deletedCredentialID = repository.store.credential.ID
	return nil
}

type healthDNSTestDNSRecordRepository struct {
	store *healthDNSTestStore
}

func (repository healthDNSTestDNSRecordRepository) ListDNSManagedRecordsByOrganization(_ context.Context, organizationID string) ([]repo.DNSManagedRecordRecord, error) {
	records := make([]repo.DNSManagedRecordRecord, 0, len(repository.store.managedRecords))
	for _, record := range repository.store.managedRecords {
		if record.OrganizationID == organizationID && record.DeletedAt == "" {
			records = append(records, record)
		}
	}
	return records, nil
}

func (repository healthDNSTestDNSRecordRepository) FindDNSManagedRecordByID(_ context.Context, organizationID string, recordID string) (repo.DNSManagedRecordRecord, error) {
	for _, record := range repository.store.managedRecords {
		if record.OrganizationID == organizationID && record.ID == recordID && record.DeletedAt == "" {
			return record, nil
		}
	}
	return repo.DNSManagedRecordRecord{}, repo.ErrNotFound
}

func (repository healthDNSTestDNSRecordRepository) LockDNSManagedRecordEvaluation(_ context.Context, _ string, recordID string) error {
	repository.store.lockedDNSManagedRecords = append(repository.store.lockedDNSManagedRecords, recordID)
	if repository.store.onLockDNSManagedRecord != nil {
		repository.store.onLockDNSManagedRecord(recordID)
	}
	return nil
}

func (repository healthDNSTestDNSRecordRepository) CreateDNSManagedRecord(_ context.Context, record repo.DNSManagedRecordRecord) error {
	repository.store.managedRecords = append(repository.store.managedRecords, record)
	return nil
}

func (repository healthDNSTestDNSRecordRepository) UpdateDNSManagedRecord(_ context.Context, record repo.DNSManagedRecordRecord) error {
	for index := range repository.store.managedRecords {
		if repository.store.managedRecords[index].OrganizationID == record.OrganizationID && repository.store.managedRecords[index].ID == record.ID {
			repository.store.managedRecords[index] = record
			return nil
		}
	}
	return repo.ErrNotFound
}

func (repository healthDNSTestDNSRecordRepository) DeleteDNSManagedRecord(_ context.Context, organizationID string, recordID string, deletedAt string) error {
	for index := range repository.store.managedRecords {
		record := repository.store.managedRecords[index]
		if record.OrganizationID == organizationID && record.ID == recordID && record.DeletedAt == "" {
			repository.store.managedRecords[index].DeletedAt = deletedAt
			repository.store.deletedDNSManagedRecordID = recordID
			return nil
		}
	}
	return repo.ErrNotFound
}
func (repository healthDNSTestDNSRecordRepository) ListDNSInstancesByOrganization(_ context.Context, organizationID string) ([]repo.DNSInstanceRecord, error) {
	var result []repo.DNSInstanceRecord
	for _, record := range repository.store.managedRecords {
		if record.OrganizationID != organizationID || record.DeletedAt != "" {
			continue
		}
		for _, instance := range record.Instances {
			if instance.DeletedAt == "" {
				result = append(result, instance)
			}
		}
	}
	return result, nil
}

func (repository healthDNSTestDNSRecordRepository) ListDNSInstancesByManagedRecord(_ context.Context, organizationID string, recordID string) ([]repo.DNSInstanceRecord, error) {
	for _, record := range repository.store.managedRecords {
		if record.OrganizationID == organizationID && record.ID == recordID && record.DeletedAt == "" {
			result := make([]repo.DNSInstanceRecord, 0, len(record.Instances))
			for _, instance := range record.Instances {
				if instance.DeletedAt == "" {
					result = append(result, instance)
				}
			}
			return result, nil
		}
	}
	return nil, nil
}

func (repository healthDNSTestDNSRecordRepository) FindDNSInstanceByID(_ context.Context, organizationID string, instanceID string) (repo.DNSInstanceRecord, error) {
	for _, record := range repository.store.managedRecords {
		if record.OrganizationID != organizationID || record.DeletedAt != "" {
			continue
		}
		for _, instance := range record.Instances {
			if instance.ID == instanceID && instance.DeletedAt == "" {
				return instance, nil
			}
		}
	}
	return repo.DNSInstanceRecord{}, repo.ErrNotFound
}

func (repository healthDNSTestDNSRecordRepository) LockDNSInstanceMutation(_ context.Context, _ string, instanceID string) error {
	repository.store.lockedDNSInstances = append(repository.store.lockedDNSInstances, instanceID)
	return nil
}

func (repository healthDNSTestDNSRecordRepository) CreateDNSInstance(_ context.Context, instance repo.DNSInstanceRecord) error {
	for index := range repository.store.managedRecords {
		record := &repository.store.managedRecords[index]
		if record.OrganizationID == instance.OrganizationID && record.ID == instance.ManagedRecordID && record.DeletedAt == "" {
			record.Instances = append(record.Instances, instance)
			return nil
		}
	}
	return repo.ErrNotFound
}

func (repository healthDNSTestDNSRecordRepository) UpdateDNSInstance(_ context.Context, instance repo.DNSInstanceRecord) error {
	for recordIndex := range repository.store.managedRecords {
		for instanceIndex := range repository.store.managedRecords[recordIndex].Instances {
			current := repository.store.managedRecords[recordIndex].Instances[instanceIndex]
			if current.OrganizationID == instance.OrganizationID && current.ID == instance.ID && current.DeletedAt == "" {
				repository.store.managedRecords[recordIndex].Instances = append(repository.store.managedRecords[recordIndex].Instances[:instanceIndex], repository.store.managedRecords[recordIndex].Instances[instanceIndex+1:]...)
				for targetRecordIndex := range repository.store.managedRecords {
					if repository.store.managedRecords[targetRecordIndex].OrganizationID == instance.OrganizationID &&
						repository.store.managedRecords[targetRecordIndex].ID == instance.ManagedRecordID &&
						repository.store.managedRecords[targetRecordIndex].DeletedAt == "" {
						repository.store.managedRecords[targetRecordIndex].Instances = append(repository.store.managedRecords[targetRecordIndex].Instances, instance)
						return nil
					}
				}
				return repo.ErrNotFound
			}
		}
	}
	return repo.ErrNotFound
}

func (repository healthDNSTestDNSRecordRepository) DeleteDNSInstance(_ context.Context, organizationID string, instanceID string, deletedAt string) error {
	for recordIndex := range repository.store.managedRecords {
		for instanceIndex := range repository.store.managedRecords[recordIndex].Instances {
			instance := &repository.store.managedRecords[recordIndex].Instances[instanceIndex]
			if instance.OrganizationID == organizationID && instance.ID == instanceID && instance.DeletedAt == "" {
				instance.DeletedAt = deletedAt
				instance.UpdatedAt = deletedAt
				return nil
			}
		}
	}
	return repo.ErrNotFound
}

func (repository healthDNSTestDNSRecordRepository) ClearDNSManagedRecordActiveInstance(_ context.Context, organizationID string, instanceID string, updatedAt string) error {
	for index := range repository.store.managedRecords {
		record := &repository.store.managedRecords[index]
		if record.OrganizationID == organizationID && record.ActiveInstanceID == instanceID && record.DeletedAt == "" {
			record.ActiveInstanceID = ""
			record.LastEvaluationStatus = "PENDING"
			record.LastEvaluationError = ""
			record.LastDiagnosticsJSON = diagnosticsJSON([]DNSDiagnosticPayload{{Code: "STALE_ACTIVE_INSTANCE_CLEARED", Message: "Active DNS instance changed; re-evaluation is required."}})
			record.UpdatedAt = updatedAt
		}
	}
	return nil
}

func (repository healthDNSTestDNSRecordRepository) UpdateDNSManagedRecordEvaluation(_ context.Context, record repo.DNSManagedRecordRecord) error {
	for index := range repository.store.managedRecords {
		if repository.store.managedRecords[index].OrganizationID == record.OrganizationID && repository.store.managedRecords[index].ID == record.ID {
			record.Instances = repository.store.managedRecords[index].Instances
			repository.store.managedRecords[index] = record
			return nil
		}
	}
	return repo.ErrNotFound
}

func (repository healthDNSTestDNSRecordRepository) UpdateDNSInstanceEvaluation(_ context.Context, instance repo.DNSInstanceRecord) error {
	for recordIndex := range repository.store.managedRecords {
		for instanceIndex := range repository.store.managedRecords[recordIndex].Instances {
			current := repository.store.managedRecords[recordIndex].Instances[instanceIndex]
			if current.OrganizationID == instance.OrganizationID && current.ID == instance.ID {
				repository.store.managedRecords[recordIndex].Instances[instanceIndex] = instance
				return nil
			}
		}
	}
	return repo.ErrNotFound
}

func (repository healthDNSTestDNSRecordRepository) ListNotificationChannelsByOrganization(context.Context, string) ([]repo.NotificationChannelRecord, error) {
	return nil, nil
}

func (repository healthDNSTestDNSRecordRepository) FindNotificationChannelByID(_ context.Context, _ string, channelID string) (repo.NotificationChannelRecord, error) {
	if repository.store.notificationLookupErr != nil {
		return repo.NotificationChannelRecord{}, repository.store.notificationLookupErr
	}
	for _, channel := range repository.store.notificationChannels {
		if channel.ID == channelID {
			return channel, nil
		}
	}
	return repo.NotificationChannelRecord{}, repo.ErrNotFound
}

func (repository healthDNSTestDNSRecordRepository) CreateNotificationChannel(context.Context, repo.NotificationChannelRecord) error {
	return nil
}

func (repository healthDNSTestDNSRecordRepository) UpdateNotificationChannel(context.Context, repo.NotificationChannelRecord, bool) error {
	return nil
}

func (repository healthDNSTestDNSRecordRepository) DeleteNotificationChannel(context.Context, string, string, string) error {
	return nil
}

func (repository healthDNSTestDNSRecordRepository) CreateNotificationDelivery(_ context.Context, delivery repo.NotificationDeliveryRecord) error {
	if repository.store.notificationDeliveryErr != nil {
		return repository.store.notificationDeliveryErr
	}
	repository.store.notificationDeliveries = append(repository.store.notificationDeliveries, delivery)
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

type healthDNSTestAuthorizer struct {
	allowedNodeGroups map[string]bool
}

func (healthDNSTestAuthorizer) HasPermission(identity InternalIdentity, permission string) bool {
	return stringSliceHas(identity.Permissions, permission)
}

func (authorizer healthDNSTestAuthorizer) AllowedNodeGroupIDs(InternalIdentity, string) map[string]bool {
	if authorizer.allowedNodeGroups == nil {
		return map[string]bool{"*": true}
	}
	return authorizer.allowedNodeGroups
}

func (healthDNSTestAuthorizer) EnsureCanDelegateRoleScopes(context.Context, repo.Repositories, InternalIdentity, []repo.ResourceScopeRecord) error {
	return nil
}
