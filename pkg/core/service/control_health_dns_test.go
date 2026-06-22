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

func TestRecordMonitorHealthResultsEvaluatesLatestResultsAcrossMonitorGroup(t *testing.T) {
	store := &healthDNSTestStore{
		monitor: repo.MonitorRecord{
			ID:             "monitor_1",
			OrganizationID: "org_1",
			GroupIDs:       []string{"monitor_group_1"},
		},
		monitors: []repo.MonitorRecord{{
			ID:             "monitor_1",
			OrganizationID: "org_1",
			GroupIDs:       []string{"monitor_group_1"},
		}, {
			ID:             "monitor_2",
			OrganizationID: "org_1",
			GroupIDs:       []string{"monitor_group_1"},
		}},
		monitorGroups: map[string]repo.MonitorGroupRecord{
			"monitor_group_1": {ID: "monitor_group_1", OrganizationID: "org_1"},
		},
		checks: []repo.HealthCheckRecord{{
			ID:             "health_1",
			OrganizationID: "org_1",
			Enabled:        true,
			Targets: []repo.HealthCheckTargetRecord{{
				ID:       "health_target_1",
				TargetID: "target_1",
			}},
			MonitorScopes: []repo.HealthCheckMonitorScopeRecord{{
				ScopeType:      "MONITOR_GROUP",
				MonitorGroupID: "monitor_group_1",
			}},
		}},
		results: []repo.HealthResultRecord{{
			ID:                  "existing_result_1",
			OrganizationID:      "org_1",
			HealthCheckID:       "health_1",
			HealthCheckTargetID: "health_target_1",
			MonitorID:           "monitor_2",
			TargetID:            "target_1",
			Status:              "OFFLINE",
			ObservedAt:          "2026-06-20T00:00:00Z",
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
			LastAppliedValuesJSON: `["198.51.100.10"]`,
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
		ObservedAt:          "2026-06-20T00:00:05Z",
	}}); err != nil {
		t.Fatalf("record monitor health results: %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("expected monitor group evaluation to keep failover while another scoped monitor is offline, got %d provider calls", provider.calls)
	}
}

func TestRecordMonitorHealthResultsUsesCustomHealthActionExecutor(t *testing.T) {
	store := &healthDNSTestStore{
		monitor: repo.MonitorRecord{ID: "monitor_1", OrganizationID: "org_1"},
		checks: []repo.HealthCheckRecord{{
			ID:             "health_1",
			OrganizationID: "org_1",
			Name:           "edge probe",
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
	if action.EventID != "event_1" || action.RuleID != "rule_1" || action.HealthCheckName != "edge probe" || action.Status != "OFFLINE" || action.ConfigJSON != `{"url":"https://hooks.example.test/health"}` {
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

func TestRecordMonitorHealthResultsRejectsDeletedMonitorGroupScope(t *testing.T) {
	store := &healthDNSTestStore{
		monitor: repo.MonitorRecord{
			ID:             "monitor_1",
			OrganizationID: "org_1",
			GroupIDs:       []string{"monitor_group_1"},
		},
		monitorGroups: map[string]repo.MonitorGroupRecord{
			"monitor_group_1": {
				ID:             "monitor_group_1",
				OrganizationID: "org_1",
				DeletedAt:      "2026-06-20T00:00:00Z",
			},
		},
		checks: []repo.HealthCheckRecord{{
			ID:             "health_1",
			OrganizationID: "org_1",
			Enabled:        true,
			Targets: []repo.HealthCheckTargetRecord{{
				ID:       "health_target_1",
				TargetID: "target_1",
			}},
			MonitorScopes: []repo.HealthCheckMonitorScopeRecord{{
				ScopeType:      "MONITOR_GROUP",
				MonitorGroupID: "monitor_group_1",
			}},
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	err := control.RecordMonitorHealthResults(context.Background(), "org_1", "monitor_1", []HealthResultInput{{
		HealthCheckID:       "health_1",
		HealthCheckTargetID: "health_target_1",
		TargetID:            "target_1",
		Status:              "ONLINE",
		ObservedAt:          "2026-06-20T00:00:00Z",
	}})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for deleted monitor group scope, got %v", err)
	}
	if len(store.results) != 0 {
		t.Fatalf("deleted monitor group scope must not record results, got %#v", store.results)
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
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	err := control.DeleteDNSCredential(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "credential_1")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	if store.deletedCredentialID != "" {
		t.Fatalf("credential should not be deleted while active records reference it")
	}
}

func TestCompileMonitorAgentConfigTreatsMissingTargetGroupAsEmptyBinding(t *testing.T) {
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
				TargetGroupID:  "target_group_deleted",
			}},
			MonitorScopes: []repo.HealthCheckMonitorScopeRecord{{
				ScopeType: "MONITOR",
				MonitorID: "monitor_1",
			}},
		}},
		targetGroups: map[string]repo.TargetGroupRecord{},
		targetsByID:  map[string]repo.TargetRecord{},
	}
	control := NewControlService(store)

	snapshot, err := control.CompileMonitorAgentConfig(context.Background(), "org_1", "monitor_1")
	if err != nil {
		t.Fatalf("compile monitor config with missing target group: %v", err)
	}
	if !store.syncedHealthTargets {
		t.Fatalf("expected missing target group binding to be synchronized")
	}
	if len(snapshot.HealthChecks) != 1 || len(snapshot.HealthChecks[0].Targets) != 0 {
		t.Fatalf("missing target group should compile as an empty binding, got %#v", snapshot.HealthChecks)
	}
	if got := store.checks[0].Targets; len(got) != 1 || got[0].TargetGroupID != "target_group_deleted" || got[0].TargetID != "" {
		t.Fatalf("expected deleted target group placeholder to be retained, got %#v", got)
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

func TestCompileMonitorAgentConfigIgnoresDeletedMonitorGroupScope(t *testing.T) {
	store := &healthDNSTestStore{
		monitor: repo.MonitorRecord{
			ID:             "monitor_1",
			OrganizationID: "org_1",
			GroupIDs:       []string{"monitor_group_1"},
		},
		monitorGroups: map[string]repo.MonitorGroupRecord{
			"monitor_group_1": {
				ID:             "monitor_group_1",
				OrganizationID: "org_1",
				DeletedAt:      "2026-06-20T00:00:00Z",
			},
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
				ID:       "health_target_1",
				TargetID: "target_1",
			}},
			MonitorScopes: []repo.HealthCheckMonitorScopeRecord{{
				ScopeType:      "MONITOR_GROUP",
				MonitorGroupID: "monitor_group_1",
			}},
		}},
	}
	control := NewControlService(store)

	snapshot, err := control.CompileMonitorAgentConfig(context.Background(), "org_1", "monitor_1")
	if err != nil {
		t.Fatalf("compile monitor config: %v", err)
	}
	if len(snapshot.HealthChecks) != 0 {
		t.Fatalf("deleted monitor group scope must not match monitor config, got %#v", snapshot.HealthChecks)
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
