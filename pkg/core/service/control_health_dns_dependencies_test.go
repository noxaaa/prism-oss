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
	if provider.calls() != 0 {
		t.Fatalf("expected unchanged DNS state to skip provider calls, got %d", provider.calls())
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

func TestCreateHealthCheckRejectsDisabledDirectTarget(t *testing.T) {
	store := &healthDNSTestStore{
		monitor: repo.MonitorRecord{ID: "monitor_1", OrganizationID: "org_1"},
		targetsByID: map[string]repo.TargetRecord{
			"target_1": {
				ID:             "target_1",
				OrganizationID: "org_1",
				Name:           "disabled target",
				Host:           "192.0.2.1",
				Port:           443,
				Enabled:        false,
			},
		},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	_, err := control.CreateHealthCheck(context.Background(), healthDNSTestIdentity(string(domain.PermissionHealthChecksManage)), HealthCheckMutationInput{
		Name:            "disabled probe",
		ProbeType:       "TCP_PORT",
		IntervalSeconds: 30,
		TimeoutSeconds:  5,
		Enabled:         true,
		TargetScope: HealthTargetScopeInput{
			Type:      "TARGETS",
			TargetIDs: []string{"target_1"},
		},
		MonitorScope: HealthMonitorScopeInput{
			Type:      "MONITOR",
			MonitorID: "monitor_1",
		},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for disabled target, got %v", err)
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
	if provider.calls() != 1 {
		t.Fatalf("expected immediate provider apply for unbound DNS record, got %d calls", provider.calls())
	}
	if provider.lastInput().ProviderSecret != "cloudflare-token" || provider.lastInput().Zone != "zone_1" || provider.lastInput().RecordName != "app.example.com" {
		t.Fatalf("unexpected provider input: %#v", provider.lastInput())
	}
	if got := provider.lastInput().Values; len(got) != 1 || got[0] != "192.0.2.1" {
		t.Fatalf("expected desired values to be applied, got %#v", got)
	}
	if store.record.ID != record.ID || store.record.LastAppliedValuesJSON != `["192.0.2.1"]` || store.record.LastAppliedAt == "" {
		t.Fatalf("expected last applied state to be recorded, got %#v", store.record)
	}
}

func TestCreateDNSRecordWithHealthBindingAppliesDesiredValues(t *testing.T) {
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		checks: []repo.HealthCheckRecord{{
			ID:             "health_1",
			OrganizationID: "org_1",
			Enabled:        true,
		}},
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

	_, err = control.CreateDNSRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage), string(domain.PermissionHealthChecksRead)), DNSRecordMutationInput{
		DNSCredentialID: "credential_1",
		Zone:            "zone_1",
		RecordName:      "app.example.com",
		RecordType:      "A",
		DesiredValues:   []string{"192.0.2.1"},
		HealthCheckID:   "health_1",
		EventType:       "DNS_FAILOVER",
		FailoverValues:  []string{"198.51.100.10"},
	})
	if err != nil {
		t.Fatalf("create health-bound dns record: %v", err)
	}
	if provider.calls() != 1 {
		t.Fatalf("expected immediate provider apply for health-bound DNS record, got %d calls", provider.calls())
	}
	if got := provider.lastInput().Values; len(got) != 1 || got[0] != "192.0.2.1" {
		t.Fatalf("expected desired values to be applied, got %#v", got)
	}
	if store.record.LastAppliedValuesJSON != `["192.0.2.1"]` || store.record.LastAppliedAt == "" {
		t.Fatalf("expected health-bound create to persist last applied state, got %#v", store.record)
	}
}

func TestCreateDNSRecordProviderFailureDoesNotPersistRecord(t *testing.T) {
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
	}
	providerErr := errors.New("provider unavailable")
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

	_, err = control.CreateDNSRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), DNSRecordMutationInput{
		DNSCredentialID: "credential_1",
		Zone:            "zone_1",
		RecordName:      "app.example.com",
		RecordType:      "A",
		DesiredValues:   []string{"192.0.2.1"},
	})
	if !errors.Is(err, providerErr) {
		t.Fatalf("expected provider error, got %v", err)
	}
	if store.createdDNSRecord.ID == "" {
		t.Fatalf("expected DNS record to be created before provider apply")
	}
	if store.deletedDNSRecordID != store.createdDNSRecord.ID || store.record.ID != "" {
		t.Fatalf("provider failure must clean up active DNS record, deleted=%q created=%#v record=%#v", store.deletedDNSRecordID, store.createdDNSRecord, store.record)
	}
}

func TestDeleteDNSRecordDeletesProviderRecord(t *testing.T) {
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

	if err := control.DeleteDNSRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "dns_1"); err != nil {
		t.Fatalf("delete dns record: %v", err)
	}
	if provider.calls() != 1 {
		t.Fatalf("expected provider delete before DB soft delete, got %d calls", provider.calls())
	}
	if input := provider.lastInput(); input.Zone != "zone_1" || input.RecordName != "app.example.com" || len(input.Values) != 0 {
		t.Fatalf("expected provider delete for old record, got %#v", input)
	}
	if store.deletedDNSRecordID != "dns_1" {
		t.Fatalf("expected DNS record soft delete after provider delete, got %q", store.deletedDNSRecordID)
	}
}

func TestDeleteDNSRecordDoesNotApplyProviderBeforeLocalStateCommits(t *testing.T) {
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
		deleteDNSRecordErr: repo.ErrConflict,
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

	err = control.DeleteDNSRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "dns_1")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	if provider.calls() != 0 {
		t.Fatalf("provider must not be called before local delete commits, got %d calls", provider.calls())
	}
}

func TestDeleteDNSRecordRequiresHealthPermissionToRemoveHealthBinding(t *testing.T) {
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		record: repo.DNSRecordRecord{
			ID:              "dns_1",
			OrganizationID:  "org_1",
			DNSCredentialID: "credential_1",
			Zone:            "zone_1",
			RecordName:      "app.example.com",
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
	provider := &healthDNSTestProvider{}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer:             healthDNSTestAuthorizer{},
		DNSSecretEncryptionKey: "test-dns-key",
		DNSProviders:           dns.StaticProviderRegistry{"CLOUDFLARE": provider},
	})

	err := control.DeleteDNSRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "dns_1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if store.deletedRulesRecordID != "" || store.deletedDNSRecordID != "" || provider.calls() != 0 {
		t.Fatalf("dns-only identity must not delete health actions or provider state, rules=%q record=%q calls=%d", store.deletedRulesRecordID, store.deletedDNSRecordID, provider.calls())
	}
}

func TestUpdateDNSRecordRetiresOldProviderRecordWhenIdentityChanges(t *testing.T) {
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
		},
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

	_, err = control.UpdateDNSRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "dns_1", DNSRecordMutationInput{
		DNSCredentialID: "credential_1",
		Zone:            "new-zone",
		RecordName:      "new.example.com",
		RecordType:      "A",
		DesiredValues:   []string{"192.0.2.1"},
	})
	if err != nil {
		t.Fatalf("update dns record: %v", err)
	}
	if provider.calls() != 2 {
		t.Fatalf("expected old provider delete and new provider apply, got %d calls", provider.calls())
	}
	if input := provider.inputs[0]; input.Zone != "old-zone" || input.RecordName != "old.example.com" || len(input.Values) != 0 {
		t.Fatalf("expected first call to delete old provider record, got %#v", input)
	}
	if input := provider.inputs[1]; input.Zone != "new-zone" || input.RecordName != "new.example.com" || len(input.Values) != 1 || input.Values[0] != "192.0.2.1" {
		t.Fatalf("expected second call to apply new provider record, got %#v", input)
	}
	if store.record.Zone != "new-zone" || store.record.LastAppliedValuesJSON != `["192.0.2.1"]` || store.record.LastAppliedAt == "" {
		t.Fatalf("expected updated record to track new applied state, got %#v", store.record)
	}
}

func TestUpdateDNSRecordIdentityApplyFailureForcesRetry(t *testing.T) {
	providerErr := errors.New("provider apply failed")
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
	provider := &healthDNSTestProvider{err: providerErr, errAt: 2}
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
		t.Fatalf("expected provider error, got %v", err)
	}
	if provider.calls() != 2 {
		t.Fatalf("expected retire and failed apply calls, got %d", provider.calls())
	}
	if store.record.Zone != "new-zone" || store.record.LastAppliedValuesJSON != "[]" || store.record.LastAppliedAt != "" {
		t.Fatalf("identity-change apply failure must leave new identity retryable, got %#v", store.record)
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
		t.Fatalf("expected retry to apply new provider record, got %d calls", provider.calls())
	}
	if input := provider.inputs[2]; input.Zone != "new-zone" || input.RecordName != "new.example.com" || len(input.Values) != 1 || input.Values[0] != "192.0.2.1" {
		t.Fatalf("expected retry to recreate new provider record, got %#v", input)
	}
	if store.record.LastAppliedValuesJSON != `["192.0.2.1"]` || store.record.LastAppliedAt == "" {
		t.Fatalf("expected retry to persist applied state, got %#v", store.record)
	}
}

func TestUpdateDNSRecordDoesNotApplyProviderBeforeLocalStateCommits(t *testing.T) {
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
		},
		updateDNSRecordErr: repo.ErrConflict,
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

	_, err = control.UpdateDNSRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "dns_1", DNSRecordMutationInput{
		DNSCredentialID: "credential_1",
		Zone:            "new-zone",
		RecordName:      "new.example.com",
		RecordType:      "A",
		DesiredValues:   []string{"192.0.2.1"},
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	if provider.calls() != 0 {
		t.Fatalf("provider must not be called before local state commits, got %d calls", provider.calls())
	}
}

func TestRecordMonitorHealthResultsDeleteOfflinePreservesHealthyDNSValues(t *testing.T) {
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
			DesiredValuesJSON:     `["192.0.2.1","192.0.2.2"]`,
			LastAppliedValuesJSON: `["192.0.2.1","192.0.2.2"]`,
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
		Status:              "ONLINE",
		ObservedAt:          "2026-06-20T00:00:00Z",
	}}); err != nil {
		t.Fatalf("record monitor health results: %v", err)
	}
	if provider.calls() != 1 {
		t.Fatalf("expected DNS provider update, got %d calls", provider.calls())
	}
	if input := provider.lastInput(); len(input.Values) != 1 || input.Values[0] != "192.0.2.2" {
		t.Fatalf("expected only offline target value to be removed, got %#v", input.Values)
	}
	if store.record.LastAppliedValuesJSON != `["192.0.2.2"]` {
		t.Fatalf("expected last applied values to preserve healthy target, got %q", store.record.LastAppliedValuesJSON)
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
	if provider.calls() != 0 {
		t.Fatalf("DNS_DELETE_ALL must not delete healthy records, got %d provider calls", provider.calls())
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
