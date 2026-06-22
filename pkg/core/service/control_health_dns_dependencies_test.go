package service

import (
	"context"
	"errors"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/dns"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestRecordMonitorHealthResultsSkipsDNSWhenAppliedValuesUnchanged(t *testing.T) {
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
		Status:              "OFFLINE",
		ObservedAt:          "2026-06-20T00:00:00Z",
	}}); err != nil {
		t.Fatalf("record monitor health results: %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("expected unchanged DNS state to skip provider calls, got %d", provider.calls)
	}
	if store.record.LastAppliedAt != "" {
		t.Fatalf("last_applied_at should not change when DNS action is skipped")
	}
}

func TestCreateDNSRecordRequiresHealthPermissionForHealthBinding(t *testing.T) {
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1"},
		checks: []repo.HealthCheckRecord{{
			ID:             "health_1",
			OrganizationID: "org_1",
			Enabled:        true,
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	_, err := control.CreateDNSRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), DNSRecordMutationInput{
		DNSCredentialID: "credential_1",
		Zone:            "zone_1",
		RecordName:      "app.example.com",
		RecordType:      "A",
		DesiredValues:   []string{"192.0.2.1"},
		HealthCheckID:   "health_1",
		EventType:       "DNS_FAILOVER",
		FailoverValues:  []string{"198.51.100.10"},
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if store.createdDNSRecord.ID != "" {
		t.Fatalf("dns-only identity must not create health-bound DNS records")
	}
}

func TestUpdateDNSRecordRequiresHealthPermissionForHealthBinding(t *testing.T) {
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1"},
		record: repo.DNSRecordRecord{
			ID:              "dns_1",
			OrganizationID:  "org_1",
			DNSCredentialID: "credential_1",
			RecordType:      "A",
		},
		checks: []repo.HealthCheckRecord{{
			ID:             "health_1",
			OrganizationID: "org_1",
			Enabled:        true,
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	_, err := control.UpdateDNSRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "dns_1", DNSRecordMutationInput{
		DNSCredentialID: "credential_1",
		Zone:            "zone_1",
		RecordName:      "app.example.com",
		RecordType:      "A",
		DesiredValues:   []string{"192.0.2.1"},
		HealthCheckID:   "health_1",
		EventType:       "DNS_FAILOVER",
		FailoverValues:  []string{"198.51.100.10"},
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if store.updatedDNSRecord.ID != "" {
		t.Fatalf("dns-only identity must not update health-bound DNS records")
	}
}

func TestUpdateDNSRecordRequiresHealthPermissionToRemoveHealthBinding(t *testing.T) {
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1"},
		record: repo.DNSRecordRecord{
			ID:              "dns_1",
			OrganizationID:  "org_1",
			DNSCredentialID: "credential_1",
			RecordType:      "A",
		},
		checks: []repo.HealthCheckRecord{{
			ID:             "health_1",
			OrganizationID: "org_1",
			Enabled:        true,
		}},
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
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	_, err := control.UpdateDNSRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "dns_1", DNSRecordMutationInput{
		DNSCredentialID: "credential_1",
		Zone:            "zone_1",
		RecordName:      "app.example.com",
		RecordType:      "A",
		DesiredValues:   []string{"192.0.2.1"},
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if store.deletedRulesRecordID != "" {
		t.Fatalf("dns-only identity must not remove health-bound DNS rules")
	}
}

func TestCreateDNSRecordWithoutHealthBindingAppliesDesiredValues(t *testing.T) {
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
	}
	provider := &healthDNSTestProvider{}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer:             healthDNSTestAuthorizer{},
		DNSSecretEncryptionKey: "test-dns-key",
		DNSProviders:           dns.StaticProviderRegistry{"CLOUDFLARE": provider},
	})
	encrypted, err := control.encryptDNSSecret("cloudflare-token")
	if err != nil {
		t.Fatalf("encrypt test secret: %v", err)
	}
	store.credential.EncryptedSecret = encrypted

	record, err := control.CreateDNSRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), DNSRecordMutationInput{
		DNSCredentialID: "credential_1",
		Zone:            "zone_1",
		RecordName:      "app.example.com",
		RecordType:      "A",
		DesiredValues:   []string{"192.0.2.1"},
	})
	if err != nil {
		t.Fatalf("create dns record: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("expected immediate provider apply for unbound DNS record, got %d calls", provider.calls)
	}
	if provider.input.ProviderSecret != "cloudflare-token" || provider.input.Zone != "zone_1" || provider.input.RecordName != "app.example.com" {
		t.Fatalf("unexpected provider input: %#v", provider.input)
	}
	if got := provider.input.Values; len(got) != 1 || got[0] != "192.0.2.1" {
		t.Fatalf("expected desired values to be applied, got %#v", got)
	}
	if store.record.ID != record.ID || store.record.LastAppliedValuesJSON != `["192.0.2.1"]` || store.record.LastAppliedAt == "" {
		t.Fatalf("expected last applied state to be recorded, got %#v", store.record)
	}
}

func TestRecordMonitorHealthResultsDoesNotDeleteAllWhileOnline(t *testing.T) {
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
			LastAppliedValuesJSON: `["192.0.2.1"]`,
		},
		rules: []repo.HealthEvaluationRuleRecord{{
			ID:             "rule_1",
			OrganizationID: "org_1",
			HealthCheckID:  "health_1",
			Enabled:        true,
			Events: []repo.HealthEventRecord{{
				ID:         "event_1",
				EventType:  "DNS_DELETE_ALL",
				Enabled:    true,
				ConfigJSON: `{"dns_record_id":"dns_1"}`,
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
	}}); err != nil {
		t.Fatalf("record monitor health results: %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("DNS_DELETE_ALL must not delete healthy records, got %d provider calls", provider.calls)
	}
}

func TestDeleteHealthCheckRejectsDNSHealthRules(t *testing.T) {
	store := &healthDNSTestStore{
		checks: []repo.HealthCheckRecord{{
			ID:             "health_1",
			OrganizationID: "org_1",
			Enabled:        true,
		}},
		rules: []repo.HealthEvaluationRuleRecord{{
			ID:             "rule_1",
			OrganizationID: "org_1",
			HealthCheckID:  "health_1",
			Enabled:        true,
			Events: []repo.HealthEventRecord{{
				ID:         "event_1",
				EventType:  "DNS_FAILOVER",
				Enabled:    true,
				ConfigJSON: `{"dns_record_id":"dns_1"}`,
			}},
		}},
	}
	control := NewControlService(store)

	err := control.DeleteHealthCheck(context.Background(), healthDNSTestIdentity(string(domain.PermissionHealthChecksManage)), "health_1")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	if store.deletedHealthCheckID != "" {
		t.Fatalf("health check must not be deleted while DNS health rules reference it")
	}
}

func TestDeleteMonitorRejectsHealthCheckScope(t *testing.T) {
	store := &healthDNSTestStore{
		monitor: repo.MonitorRecord{ID: "monitor_1", OrganizationID: "org_1"},
		checks: []repo.HealthCheckRecord{{
			ID:             "health_1",
			OrganizationID: "org_1",
			Enabled:        true,
			MonitorScopes: []repo.HealthCheckMonitorScopeRecord{{
				ScopeType: "MONITOR",
				MonitorID: "monitor_1",
			}},
		}},
	}
	control := NewControlService(store)

	err := control.DeleteMonitor(context.Background(), healthDNSTestIdentity(string(domain.PermissionMonitorsManage)), "monitor_1")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	if store.deletedMonitorID != "" {
		t.Fatalf("monitor must not be deleted while health checks reference it")
	}
}

func TestDeleteMonitorGroupRejectsHealthCheckScope(t *testing.T) {
	store := &healthDNSTestStore{
		monitorGroups: map[string]repo.MonitorGroupRecord{
			"monitor_group_1": {ID: "monitor_group_1", OrganizationID: "org_1"},
		},
		checks: []repo.HealthCheckRecord{{
			ID:             "health_1",
			OrganizationID: "org_1",
			Enabled:        true,
			MonitorScopes: []repo.HealthCheckMonitorScopeRecord{{
				ScopeType:      "MONITOR_GROUP",
				MonitorGroupID: "monitor_group_1",
			}},
		}},
	}
	control := NewControlService(store)

	err := control.DeleteMonitorGroup(context.Background(), healthDNSTestIdentity(string(domain.PermissionMonitorsManage)), "monitor_group_1")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	if store.deletedMonitorGroupID != "" {
		t.Fatalf("monitor group must not be deleted while health checks reference it")
	}
}
