package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/dns"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestRecordMonitorHealthResultsAppliesDNSFailover(t *testing.T) {
	store := &healthDNSTestStore{
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

func TestRecordMonitorHealthResultsUsesCustomHealthEventExecutor(t *testing.T) {
	store := &healthDNSTestStore{
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
	executor := &recordingHealthEventExecutor{}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		HealthEventExecutors: []HealthEventExecutor{executor},
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

type healthDNSTestProvider struct {
	input dns.ApplyRecordInput
}

func (provider *healthDNSTestProvider) ApplyRecord(_ context.Context, input dns.ApplyRecordInput) error {
	provider.input = input
	return nil
}

type recordingHealthEventExecutor struct {
	executed []recordingHealthEventAction
}

type recordingHealthEventAction struct {
	EventID    string
	Status     string
	ConfigJSON string
}

func (executor *recordingHealthEventExecutor) Supports(eventType string) bool {
	return eventType == "WEBHOOK"
}

func (executor *recordingHealthEventExecutor) BuildAction(_ context.Context, _ repo.Repositories, input HealthEventExecutionInput) (any, bool, error) {
	return recordingHealthEventAction{EventID: input.Event.ID, Status: input.Result.Status, ConfigJSON: input.Event.ConfigJSON}, true, nil
}

func (executor *recordingHealthEventExecutor) Execute(_ context.Context, action any) error {
	executor.executed = append(executor.executed, action.(recordingHealthEventAction))
	return nil
}

type healthDNSTestStore struct {
	results    []repo.HealthResultRecord
	rules      []repo.HealthEvaluationRuleRecord
	credential repo.DNSCredentialRecord
	record     repo.DNSRecordRecord
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
func (repositories healthDNSTestRepositories) Monitors() repo.MonitorRepository           { return nil }
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
	return nil
}
func (repositories healthDNSTestRepositories) Rules() repo.RuleRepository   { return nil }
func (repositories healthDNSTestRepositories) Quotas() repo.QuotaRepository { return nil }
func (repositories healthDNSTestRepositories) AgentRegistrationTokens() repo.AgentRegistrationTokenRepository {
	return nil
}
func (repositories healthDNSTestRepositories) AgentCredentials() repo.AgentCredentialRepository {
	return nil
}
func (repositories healthDNSTestRepositories) AuditLogs() repo.AuditLogRepository { return nil }

type healthDNSTestHealthRepository struct {
	store *healthDNSTestStore
}

func (repository healthDNSTestHealthRepository) ListHealthChecksByOrganization(context.Context, string) ([]repo.HealthCheckRecord, error) {
	return nil, nil
}
func (repository healthDNSTestHealthRepository) FindHealthCheckByID(context.Context, string, string) (repo.HealthCheckRecord, error) {
	return repo.HealthCheckRecord{}, repo.ErrNotFound
}
func (repository healthDNSTestHealthRepository) CreateHealthCheck(context.Context, repo.HealthCheckRecord, []repo.HealthCheckTargetRecord, []repo.HealthCheckMonitorScopeRecord, string, func() string) error {
	return nil
}
func (repository healthDNSTestHealthRepository) UpdateHealthCheck(context.Context, repo.HealthCheckRecord, []repo.HealthCheckTargetRecord, []repo.HealthCheckMonitorScopeRecord, string, func() string) error {
	return nil
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
	return nil
}

type healthDNSTestDNSRecordRepository struct {
	store *healthDNSTestStore
}

func (repository healthDNSTestDNSRecordRepository) ListDNSRecordsByOrganization(context.Context, string) ([]repo.DNSRecordRecord, error) {
	return nil, nil
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
