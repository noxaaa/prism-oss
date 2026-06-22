package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/dns"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestDeleteDNSRecordProviderFailureKeepsRecordRetryable(t *testing.T) {
	providerErr := errors.New("provider delete failed")
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		record: repo.DNSRecordRecord{
			ID:                    "dns_1",
			OrganizationID:        "org_1",
			DNSCredentialID:       "credential_1",
			Zone:                  "zone_1",
			RecordName:            "app.example.com",
			RecordType:            "A",
			LastAppliedValuesJSON: `["192.0.2.1"]`,
		},
	}
	provider := &healthDNSTestProvider{err: providerErr}
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

	err = control.DeleteDNSRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "dns_1")
	if !errors.Is(err, providerErr) {
		t.Fatalf("expected provider error, got %v", err)
	}
	if store.record.ID != "dns_1" || store.deletedDNSRecordID != "" {
		t.Fatalf("provider failure must leave DNS record retryable, record=%#v deleted=%q", store.record, store.deletedDNSRecordID)
	}

	provider.err = nil
	if err := control.DeleteDNSRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "dns_1"); err != nil {
		t.Fatalf("retry delete dns record: %v", err)
	}
	if provider.calls() != 2 {
		t.Fatalf("expected retry to call provider delete, got %d calls", provider.calls())
	}
	if store.deletedDNSRecordID != "dns_1" || store.record.ID != "" {
		t.Fatalf("expected retry to soft delete local record, deleted=%q record=%#v", store.deletedDNSRecordID, store.record)
	}
}

func TestRecordMonitorHealthResultsSkipsDNSRecordPendingProviderDelete(t *testing.T) {
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
			ID:                      "dns_1",
			OrganizationID:          "org_1",
			DNSCredentialID:         "credential_1",
			Zone:                    "zone_1",
			RecordName:              "app.example.com",
			RecordType:              "A",
			DesiredValuesJSON:       `["192.0.2.1"]`,
			LastAppliedValuesJSON:   `["192.0.2.1"]`,
			ProviderDeletePendingAt: "2026-06-20T00:00:00Z",
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
		ObservedAt:          "2026-06-20T00:00:01Z",
	}}); err != nil {
		t.Fatalf("record monitor health results: %v", err)
	}
	if provider.calls() != 0 {
		t.Fatalf("provider delete pending records must not be changed by health actions, got %d calls", provider.calls())
	}
}

func TestUpdateDNSRecordIdentityRetireFailureRetriesOldIdentity(t *testing.T) {
	providerErr := errors.New("provider retire failed")
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		record: repo.DNSRecordRecord{
			ID:                    "dns_1",
			OrganizationID:        "org_1",
			DNSCredentialID:       "credential_1",
			Zone:                  "old-zone",
			RecordName:            "old.example.com",
			RecordType:            "A",
			DesiredValuesJSON:     `["192.0.2.1"]`,
			LastAppliedValuesJSON: `["192.0.2.1"]`,
			LastAppliedAt:         "2026-06-20T00:00:00Z",
		},
	}
	provider := &healthDNSTestProvider{err: providerErr, errAt: 1}
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

	_, err = control.UpdateDNSRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "dns_1", DNSRecordMutationInput{
		DNSCredentialID: "credential_1",
		Zone:            "new-zone",
		RecordName:      "new.example.com",
		RecordType:      "A",
		DesiredValues:   []string{"192.0.2.1"},
	})
	if !errors.Is(err, providerErr) {
		t.Fatalf("expected provider retire error, got %v", err)
	}
	if provider.calls() != 1 {
		t.Fatalf("expected failed retire call, got %d", provider.calls())
	}

	provider.err = nil
	_, err = control.UpdateDNSRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "dns_1", DNSRecordMutationInput{
		DNSCredentialID: "credential_1",
		Zone:            "new-zone",
		RecordName:      "new.example.com",
		RecordType:      "A",
		DesiredValues:   []string{"192.0.2.1"},
	})
	if err != nil {
		t.Fatalf("retry update dns record: %v", err)
	}
	if provider.calls() != 3 {
		t.Fatalf("expected retry to retire old identity and apply new identity, got %d calls", provider.calls())
	}
	if input := provider.inputs[1]; input.Zone != "old-zone" || input.RecordName != "old.example.com" || len(input.Values) != 0 {
		t.Fatalf("expected retry to delete old provider record, got %#v", input)
	}
	if input := provider.inputs[2]; input.Zone != "new-zone" || input.RecordName != "new.example.com" || len(input.Values) != 1 || input.Values[0] != "192.0.2.1" {
		t.Fatalf("expected retry to apply new provider record, got %#v", input)
	}
}

func TestRecordMonitorHealthResultsDeleteOfflineRemovesAllOfflineTargets(t *testing.T) {
	store := &healthDNSTestStore{
		monitor:  repo.MonitorRecord{ID: "monitor_1", OrganizationID: "org_1", Status: "ONLINE"},
		monitors: []repo.MonitorRecord{{ID: "monitor_1", OrganizationID: "org_1", Status: "ONLINE"}},
		checks: []repo.HealthCheckRecord{{
			ID:             "health_1",
			OrganizationID: "org_1",
			Enabled:        true,
			Targets: []repo.HealthCheckTargetRecord{{
				ID:         "health_target_1",
				TargetID:   "target_1",
				TargetHost: "192.0.2.1",
			}, {
				ID:         "health_target_2",
				TargetID:   "target_2",
				TargetHost: "192.0.2.2",
			}, {
				ID:         "health_target_3",
				TargetID:   "target_3",
				TargetHost: "192.0.2.3",
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
			RecordName:            "pool.example.com",
			RecordType:            "A",
			DesiredValuesJSON:     `["192.0.2.1","192.0.2.2","192.0.2.3"]`,
			LastAppliedValuesJSON: `["192.0.2.1","192.0.2.2","192.0.2.3"]`,
		},
		rules: []repo.HealthEvaluationRuleRecord{{
			ID:             "rule_1",
			OrganizationID: "org_1",
			HealthCheckID:  "health_1",
			Enabled:        true,
			Events: []repo.HealthEventRecord{{
				ID:         "event_1",
				EventType:  "DNS_DELETE_OFFLINE",
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
		Status:              "OFFLINE",
		ObservedAt:          "2026-06-20T00:00:00Z",
	}, {
		HealthCheckID:       "health_1",
		HealthCheckTargetID: "health_target_2",
		TargetID:            "target_2",
		Status:              "OFFLINE",
		ObservedAt:          "2026-06-20T00:00:00Z",
	}, {
		HealthCheckID:       "health_1",
		HealthCheckTargetID: "health_target_3",
		TargetID:            "target_3",
		Status:              "ONLINE",
		ObservedAt:          "2026-06-20T00:00:00Z",
	}}); err != nil {
		t.Fatalf("record monitor health results: %v", err)
	}
	if provider.calls() != 1 {
		t.Fatalf("expected DNS provider update, got %d calls", provider.calls())
	}
	if input := provider.lastInput(); len(input.Values) != 1 || input.Values[0] != "192.0.2.3" {
		t.Fatalf("expected all offline target values to be removed, got %#v", input.Values)
	}
}

func TestRecordMonitorHealthResultsClampsFutureObservedAt(t *testing.T) {
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
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})
	control.now = func() time.Time { return time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC) }

	if err := control.RecordMonitorHealthResults(context.Background(), "org_1", "monitor_1", []HealthResultInput{{
		HealthCheckID:       "health_1",
		HealthCheckTargetID: "health_target_1",
		TargetID:            "target_1",
		Status:              "ONLINE",
		ObservedAt:          "2099-01-01T00:00:00Z",
	}}); err != nil {
		t.Fatalf("record monitor health results: %v", err)
	}
	if len(store.results) != 1 || store.results[0].ObservedAt != "2026-06-20T12:00:00Z" {
		t.Fatalf("expected future observed_at to be clamped to server time, got %#v", store.results)
	}
}
