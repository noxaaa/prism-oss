package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/dns"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestRecordMonitorHealthResultsAppliesDNSFailover(t *testing.T) {
	store := &healthDNSTestStore{
		monitor: repo.MonitorRecord{ID: "monitor_1", OrganizationID: "org_1"},
		checks: []repo.HealthCheckRecord{{
			ID:             "health_1",
			OrganizationID: "org_1",
			Enabled:        true,
			Targets: []repo.HealthCheckTargetRecord{{
				ID:            "health_target_1",
				TargetID:      "target_1",
				TargetGroupID: "",
			}},
			MonitorScopes: []repo.HealthCheckMonitorScopeRecord{{
				ScopeType: "MONITOR",
				MonitorID: "monitor_1",
			}},
		}},
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		record: repo.DNSRecordRecord{
			ID:                    "dns_1",
			OrganizationID:        "org_1",
			DNSCredentialID:       "credential_1",
			Zone:                  "zone_1",
			RecordName:            "health.example.com",
			RecordType:            "A",
			DesiredValuesJSON:     `["192.0.2.1"]`,
			LastAppliedValuesJSON: "[]",
		},
		rules: []repo.HealthEvaluationRuleRecord{{
			ID:             "rule_1",
			OrganizationID: "org_1",
			HealthCheckID:  "health_1",
			Enabled:        true,
			Events: []repo.HealthEventRecord{{
				ID:         "event_1",
				EventType:  "DNS_FAILOVER",
				Enabled:    true,
				ConfigJSON: `{"dns_record_id":"dns_1","failover_values":["198.51.100.10"]}`,
			}},
		}},
	}
	provider := &healthDNSTestProvider{}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		DNSSecretEncryptionKey: "test-dns-key",
		DNSProviders:           dns.StaticProviderRegistry{"CLOUDFLARE": provider},
	})
	encrypted, err := control.encryptDNSSecret("cloudflare-token")
	if err != nil {
		t.Fatalf("encrypt test secret: %v", err)
	}
	store.credential.EncryptedSecret = encrypted

	if err := control.RecordMonitorHealthResults(context.Background(), "org_1", "monitor_1", []HealthResultInput{{
		HealthCheckID:       "health_1",
		HealthCheckTargetID: "health_target_1",
		TargetID:            "target_1",
		Status:              "OFFLINE",
		ObservedAt:          "2026-06-20T00:00:00Z",
	}}); err != nil {
		t.Fatalf("record monitor health results: %v", err)
	}
	if len(store.results) != 1 {
		t.Fatalf("expected health result to be recorded, got %#v", store.results)
	}
	if provider.input.ProviderSecret != "cloudflare-token" || provider.input.Zone != "zone_1" || provider.input.RecordName != "health.example.com" {
		t.Fatalf("unexpected provider input: %#v", provider.input)
	}
	if got := provider.input.Values; len(got) != 1 || got[0] != "198.51.100.10" {
		t.Fatalf("expected failover value to be applied, got %#v", got)
	}
	var lastApplied []string
	if err := json.Unmarshal([]byte(store.record.LastAppliedValuesJSON), &lastApplied); err != nil {
		t.Fatalf("decode last applied values: %v", err)
	}
	if len(lastApplied) != 1 || lastApplied[0] != "198.51.100.10" || store.record.LastAppliedAt == "" {
		t.Fatalf("expected last applied failover values, got values=%#v at=%q", lastApplied, store.record.LastAppliedAt)
	}
}

func TestRecordMonitorHealthResultsAggregatesDNSActionPerCheck(t *testing.T) {
	store := &healthDNSTestStore{
		monitor: repo.MonitorRecord{ID: "monitor_1", OrganizationID: "org_1"},
		checks: []repo.HealthCheckRecord{{
			ID:             "health_1",
			OrganizationID: "org_1",
			Enabled:        true,
			Targets: []repo.HealthCheckTargetRecord{{
				ID:       "health_target_1",
				TargetID: "target_1",
			}, {
				ID:       "health_target_2",
				TargetID: "target_2",
			}},
			MonitorScopes: []repo.HealthCheckMonitorScopeRecord{{
				ScopeType: "MONITOR",
				MonitorID: "monitor_1",
			}},
		}},
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		record: repo.DNSRecordRecord{
			ID:                "dns_1",
			OrganizationID:    "org_1",
			DNSCredentialID:   "credential_1",
			Zone:              "zone_1",
			RecordName:        "health.example.com",
			RecordType:        "A",
			DesiredValuesJSON: `["192.0.2.1"]`,
		},
		rules: []repo.HealthEvaluationRuleRecord{{
			ID:             "rule_1",
			OrganizationID: "org_1",
			HealthCheckID:  "health_1",
			Enabled:        true,
			Events: []repo.HealthEventRecord{{
				ID:         "event_1",
				EventType:  "DNS_FAILOVER",
				Enabled:    true,
				ConfigJSON: `{"dns_record_id":"dns_1","failover_values":["198.51.100.10"]}`,
			}},
		}},
	}
	provider := &healthDNSTestProvider{}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		DNSSecretEncryptionKey: "test-dns-key",
		DNSProviders:           dns.StaticProviderRegistry{"CLOUDFLARE": provider},
	})
	encrypted, err := control.encryptDNSSecret("cloudflare-token")
	if err != nil {
		t.Fatalf("encrypt test secret: %v", err)
	}
	store.credential.EncryptedSecret = encrypted

	if err := control.RecordMonitorHealthResults(context.Background(), "org_1", "monitor_1", []HealthResultInput{{
		HealthCheckID:       "health_1",
		HealthCheckTargetID: "health_target_1",
		TargetID:            "target_1",
		Status:              "ONLINE",
		ObservedAt:          "2026-06-20T00:00:00Z",
	}, {
		HealthCheckID:       "health_1",
		HealthCheckTargetID: "health_target_2",
		TargetID:            "target_2",
		Status:              "OFFLINE",
		ObservedAt:          "2026-06-20T00:00:01Z",
	}}); err != nil {
		t.Fatalf("record monitor health results: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("expected one DNS apply for the check evaluation, got %d", provider.calls)
	}
	if got := provider.input.Values; len(got) != 1 || got[0] != "198.51.100.10" {
		t.Fatalf("expected aggregate offline status to apply failover value, got %#v", got)
	}
}

func TestRecordMonitorHealthResultsUsesCustomHealthActionExecutor(t *testing.T) {
	store := &healthDNSTestStore{
		monitor: repo.MonitorRecord{ID: "monitor_1", OrganizationID: "org_1"},
		checks: []repo.HealthCheckRecord{{
			ID:             "health_1",
			OrganizationID: "org_1",
			Enabled:        true,
			Targets: []repo.HealthCheckTargetRecord{{
				ID:            "health_target_1",
				TargetID:      "target_1",
				TargetGroupID: "",
			}},
			MonitorScopes: []repo.HealthCheckMonitorScopeRecord{{
				ScopeType: "MONITOR",
				MonitorID: "monitor_1",
			}},
		}},
		rules: []repo.HealthEvaluationRuleRecord{{
			ID:             "rule_1",
			OrganizationID: "org_1",
			HealthCheckID:  "health_1",
			Enabled:        true,
			Events: []repo.HealthEventRecord{{
				ID:         "event_1",
				EventType:  "WEBHOOK",
				Enabled:    true,
				ConfigJSON: `{"url":"https://hooks.example.test/health"}`,
			}},
		}},
	}
	executor := &recordingHealthActionExecutor{}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		HealthActionExecutors: []HealthActionExecutor{executor},
	})

	if err := control.RecordMonitorHealthResults(context.Background(), "org_1", "monitor_1", []HealthResultInput{{
		HealthCheckID:       "health_1",
		HealthCheckTargetID: "health_target_1",
		TargetID:            "target_1",
		Status:              "OFFLINE",
		ObservedAt:          "2026-06-20T00:00:00Z",
	}}); err != nil {
		t.Fatalf("record monitor health results: %v", err)
	}
	if len(executor.executed) != 1 {
		t.Fatalf("expected custom executor to run once, got %#v", executor.executed)
	}
	action := executor.executed[0]
	if action.EventID != "event_1" || action.Status != "OFFLINE" || action.ConfigJSON != `{"url":"https://hooks.example.test/health"}` {
		t.Fatalf("unexpected custom action: %#v", action)
	}
}

func TestRecordMonitorHealthResultsRejectsOutOfScopeMonitor(t *testing.T) {
	store := &healthDNSTestStore{
		monitor: repo.MonitorRecord{ID: "monitor_1", OrganizationID: "org_1"},
		checks: []repo.HealthCheckRecord{{
			ID:             "health_1",
			OrganizationID: "org_1",
			Enabled:        true,
			Targets: []repo.HealthCheckTargetRecord{{
				ID:       "health_target_1",
				TargetID: "target_1",
			}},
			MonitorScopes: []repo.HealthCheckMonitorScopeRecord{{
				ScopeType: "MONITOR",
				MonitorID: "monitor_2",
			}},
		}},
		rules: []repo.HealthEvaluationRuleRecord{{
			ID:             "rule_1",
			OrganizationID: "org_1",
			HealthCheckID:  "health_1",
			Enabled:        true,
			Events: []repo.HealthEventRecord{{
				ID:         "event_1",
				EventType:  "WEBHOOK",
				Enabled:    true,
				ConfigJSON: `{"url":"https://hooks.example.test/health"}`,
			}},
		}},
	}
	executor := &recordingHealthActionExecutor{}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		HealthActionExecutors: []HealthActionExecutor{executor},
	})

	err := control.RecordMonitorHealthResults(context.Background(), "org_1", "monitor_1", []HealthResultInput{{
		HealthCheckID:       "health_1",
		HealthCheckTargetID: "health_target_1",
		TargetID:            "target_1",
		Status:              "OFFLINE",
		ObservedAt:          "2026-06-20T00:00:00Z",
	}})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if len(store.results) != 0 {
		t.Fatalf("out-of-scope result must not be recorded, got %#v", store.results)
	}
	if len(executor.executed) != 0 {
		t.Fatalf("out-of-scope result must not execute events, got %#v", executor.executed)
	}
}

func TestDeleteDNSCredentialRejectsActiveRecords(t *testing.T) {
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1"},
		record: repo.DNSRecordRecord{
			ID:              "dns_1",
			OrganizationID:  "org_1",
			DNSCredentialID: "credential_1",
		},
	}
	control := NewControlService(store)

	err := control.DeleteDNSCredential(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "credential_1")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	if store.deletedCredentialID != "" {
		t.Fatalf("credential should not be deleted while active records reference it")
	}
}

func TestCompileMonitorAgentConfigRefreshesTargetGroupMembers(t *testing.T) {
	store := &healthDNSTestStore{
		monitor: repo.MonitorRecord{
			ID:                   "monitor_1",
			OrganizationID:       "org_1",
			DesiredConfigVersion: 7,
		},
		checks: []repo.HealthCheckRecord{{
			ID:              "health_1",
			OrganizationID:  "org_1",
			ProbeType:       "TCP_PORT",
			IntervalSeconds: 30,
			TimeoutSeconds:  3,
			ConfigJSON:      "{}",
			Enabled:         true,
			Targets: []repo.HealthCheckTargetRecord{{
				ID:             "health_target_stale",
				OrganizationID: "org_1",
				HealthCheckID:  "health_1",
				ScopeType:      "TARGET_GROUP",
				TargetID:       "target_stale",
				TargetGroupID:  "target_group_1",
				TargetName:     "stale",
				TargetHost:     "192.0.2.10",
				TargetPort:     443,
			}},
			MonitorScopes: []repo.HealthCheckMonitorScopeRecord{{
				ScopeType: "MONITOR",
				MonitorID: "monitor_1",
			}},
		}},
		targetGroups: map[string]repo.TargetGroupRecord{
			"target_group_1": {
				ID:             "target_group_1",
				OrganizationID: "org_1",
				Members: []repo.TargetGroupMemberRecord{
					{TargetID: "target_current", Enabled: true},
					{TargetID: "target_disabled", Enabled: false},
				},
			},
		},
		targetsByID: map[string]repo.TargetRecord{
			"target_current": {
				ID:             "target_current",
				OrganizationID: "org_1",
				Name:           "current",
				Host:           "198.51.100.20",
				Port:           8443,
				Enabled:        true,
			},
			"target_disabled": {
				ID:             "target_disabled",
				OrganizationID: "org_1",
				Name:           "disabled",
				Host:           "198.51.100.30",
				Port:           443,
				Enabled:        true,
			},
		},
	}
	control := NewControlService(store)

	snapshot, err := control.CompileMonitorAgentConfig(context.Background(), "org_1", "monitor_1")
	if err != nil {
		t.Fatalf("compile monitor config: %v", err)
	}
	if !store.syncedHealthTargets {
		t.Fatalf("expected target-group health targets to be synchronized")
	}
	if len(snapshot.HealthChecks) != 1 || len(snapshot.HealthChecks[0].Targets) != 1 {
		t.Fatalf("expected one current target, got %#v", snapshot.HealthChecks)
	}
	target := snapshot.HealthChecks[0].Targets[0]
	if target.TargetID != "target_current" || target.Name != "current" || target.Host != "198.51.100.20" || target.Port != 8443 {
		t.Fatalf("expected refreshed target-group member, got %#v", target)
	}
}

func TestCompileMonitorAgentConfigPreservesEmptyTargetGroupBinding(t *testing.T) {
	store := &healthDNSTestStore{
		monitor: repo.MonitorRecord{
			ID:             "monitor_1",
			OrganizationID: "org_1",
		},
		checks: []repo.HealthCheckRecord{{
			ID:              "health_1",
			OrganizationID:  "org_1",
			ProbeType:       "TCP_PORT",
			IntervalSeconds: 30,
			TimeoutSeconds:  3,
			ConfigJSON:      "{}",
			Enabled:         true,
			Targets: []repo.HealthCheckTargetRecord{{
				ID:             "health_target_stale",
				OrganizationID: "org_1",
				HealthCheckID:  "health_1",
				ScopeType:      "TARGET_GROUP",
				TargetID:       "target_stale",
				TargetGroupID:  "target_group_1",
			}},
			MonitorScopes: []repo.HealthCheckMonitorScopeRecord{{
				ScopeType: "MONITOR",
				MonitorID: "monitor_1",
			}},
		}},
		targetGroups: map[string]repo.TargetGroupRecord{
			"target_group_1": {ID: "target_group_1", OrganizationID: "org_1"},
		},
		targetsByID: map[string]repo.TargetRecord{},
	}
	control := NewControlService(store)

	snapshot, err := control.CompileMonitorAgentConfig(context.Background(), "org_1", "monitor_1")
	if err != nil {
		t.Fatalf("compile empty target-group monitor config: %v", err)
	}
	if len(snapshot.HealthChecks) != 1 || len(snapshot.HealthChecks[0].Targets) != 0 {
		t.Fatalf("empty target group should not emit probe targets, got %#v", snapshot.HealthChecks)
	}
	if got := store.checks[0].Targets; len(got) != 1 || got[0].TargetGroupID != "target_group_1" || got[0].TargetID != "" {
		t.Fatalf("expected placeholder target-group binding to be retained, got %#v", got)
	}

	store.targetGroups["target_group_1"] = repo.TargetGroupRecord{
		ID:             "target_group_1",
		OrganizationID: "org_1",
		Members: []repo.TargetGroupMemberRecord{
			{TargetID: "target_current", Enabled: true},
		},
	}
	store.targetsByID["target_current"] = repo.TargetRecord{
		ID:             "target_current",
		OrganizationID: "org_1",
		Name:           "current",
		Host:           "198.51.100.20",
		Port:           8443,
		Enabled:        true,
	}

	snapshot, err = control.CompileMonitorAgentConfig(context.Background(), "org_1", "monitor_1")
	if err != nil {
		t.Fatalf("compile repopulated target-group monitor config: %v", err)
	}
	if len(snapshot.HealthChecks) != 1 || len(snapshot.HealthChecks[0].Targets) != 1 {
		t.Fatalf("expected target group to repopulate after a member is added, got %#v", snapshot.HealthChecks)
	}
	if snapshot.HealthChecks[0].Targets[0].TargetID != "target_current" {
		t.Fatalf("expected current target after repopulation, got %#v", snapshot.HealthChecks[0].Targets[0])
	}
}

func TestBuildHealthBindingsPreservesEmptyTargetGroupScope(t *testing.T) {
	store := &healthDNSTestStore{
		monitor: repo.MonitorRecord{ID: "monitor_1", OrganizationID: "org_1"},
		targetGroups: map[string]repo.TargetGroupRecord{
			"target_group_1": {ID: "target_group_1", OrganizationID: "org_1"},
		},
	}
	control := NewControlService(store)

	targets, _, err := control.buildHealthBindings(context.Background(), healthDNSTestRepositories{store: store}, "org_1", HealthCheckMutationInput{
		TargetScope: HealthTargetScopeInput{Type: "TARGET_GROUP", TargetGroupID: "target_group_1"},
		MonitorScope: HealthMonitorScopeInput{
			Type:      "MONITOR",
			MonitorID: "monitor_1",
		},
	})
	if err != nil {
		t.Fatalf("build empty target-group health bindings: %v", err)
	}
	if len(targets) != 1 || targets[0].ScopeType != "TARGET_GROUP" || targets[0].TargetGroupID != "target_group_1" || targets[0].TargetID != "" {
		t.Fatalf("expected one placeholder target-group binding, got %#v", targets)
	}
}

func TestHealthCheckPayloadOmitsEmptyTargetGroupBinding(t *testing.T) {
	payload := toHealthCheckPayload(repo.HealthCheckRecord{
		ID:             "health_1",
		OrganizationID: "org_1",
		Targets: []repo.HealthCheckTargetRecord{{
			ID:            "health_target_placeholder",
			ScopeType:     "TARGET_GROUP",
			TargetGroupID: "target_group_1",
		}, {
			ID:         "health_target_1",
			ScopeType:  "TARGET_GROUP",
			TargetID:   "target_1",
			TargetName: "current",
			TargetHost: "198.51.100.20",
			TargetPort: 443,
		}},
	})

	if len(payload.Targets) != 1 || payload.Targets[0].TargetID != "target_1" {
		t.Fatalf("expected placeholder target-group binding to stay internal, got %#v", payload.Targets)
	}
}

type healthDNSTestProvider struct {
	input dns.ApplyRecordInput
	calls int
}

func (provider *healthDNSTestProvider) ApplyRecord(_ context.Context, input dns.ApplyRecordInput) error {
	provider.input = input
	provider.calls++
	return nil
}

type recordingHealthActionExecutor struct {
	executed []recordingHealthEventAction
}

type recordingHealthEventAction struct {
	EventID    string
	Status     string
	ConfigJSON string
}

func (executor *recordingHealthActionExecutor) Supports(eventType string) bool {
	return eventType == "WEBHOOK"
}

func (executor *recordingHealthActionExecutor) BuildAction(_ context.Context, _ repo.Repositories, input HealthActionExecutionInput) (any, bool, error) {
	return recordingHealthEventAction{EventID: input.Event.ID, Status: input.Result.Status, ConfigJSON: input.Event.ConfigJSON}, true, nil
}

func (executor *recordingHealthActionExecutor) Execute(_ context.Context, action any) error {
	executor.executed = append(executor.executed, action.(recordingHealthEventAction))
	return nil
}

type healthDNSTestStore struct {
	results             []repo.HealthResultRecord
	rules               []repo.HealthEvaluationRuleRecord
	credential          repo.DNSCredentialRecord
	record              repo.DNSRecordRecord
	monitor             repo.MonitorRecord
	checks              []repo.HealthCheckRecord
	targetGroups        map[string]repo.TargetGroupRecord
	targetsByID         map[string]repo.TargetRecord
	syncedHealthTargets bool
	deletedCredentialID string
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
func (repositories healthDNSTestRepositories) MonitorGroups() repo.MonitorGroupRepository { return nil }
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
func (repositories healthDNSTestRepositories) Targets() repo.TargetRepository { return nil }
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

func (repository healthDNSTestMonitorRepository) ListMonitorsByOrganization(context.Context, string) ([]repo.MonitorRecord, error) {
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
func (repository healthDNSTestMonitorRepository) DeleteMonitor(context.Context, string, string, string) error {
	return nil
}

type healthDNSTestTargetGroupRepository struct {
	store *healthDNSTestStore
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
func (repository healthDNSTestHealthRepository) DeleteHealthCheck(context.Context, string, string, string) error {
	return nil
}
func (repository healthDNSTestHealthRepository) ListHealthResults(context.Context, string, string, int) ([]repo.HealthResultRecord, error) {
	return nil, nil
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
func (repository healthDNSTestHealthRepository) DeleteHealthEvaluationRulesForDNSRecord(context.Context, string, string, string) error {
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
func (repository healthDNSTestDNSRecordRepository) CreateDNSRecord(context.Context, repo.DNSRecordRecord) error {
	return nil
}
func (repository healthDNSTestDNSRecordRepository) UpdateDNSRecord(context.Context, repo.DNSRecordRecord) error {
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
func (repository healthDNSTestDNSRecordRepository) DeleteDNSRecord(context.Context, string, string, string) error {
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
