package service

import (
	"context"
	"errors"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/dns"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestDeleteDNSManagedRecordDeletesAppliedProviderRecord(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer:              healthDNSTestAuthorizer{},
		DNSSecretEncryptionKey:  "test-secret-key",
		DNSProviders:            dns.StaticProviderRegistry{"CLOUDFLARE": provider},
		HealthActionExecutors:   nil,
		HealthEventExecutors:    nil,
		TargetGroupSchedulers:   nil,
		AgentTokenSigningSecret: nil,
	})
	encrypted, err := control.encryptDNSSecret("provider-token")
	if err != nil {
		t.Fatalf("encrypt dns secret: %v", err)
	}
	store.credential = repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE", EncryptedSecret: encrypted}
	store.managedRecords = []repo.DNSManagedRecordRecord{{
		ID:                    "record_1",
		OrganizationID:        "org_1",
		DNSCredentialID:       "credential_1",
		ZoneID:                "zone_1",
		RecordName:            "app.example.com",
		RecordType:            "A",
		TTL:                   120,
		Proxied:               true,
		LastAppliedValuesJSON: `["192.0.2.10"]`,
	}}

	err = control.DeleteDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if err != nil {
		t.Fatalf("delete managed record: %v", err)
	}
	if store.deletedDNSManagedRecordID != "record_1" {
		t.Fatalf("expected local record to be deleted, got %q", store.deletedDNSManagedRecordID)
	}
	if len(provider.actions) != 1 {
		t.Fatalf("expected one provider delete action, got %#v", provider.actions)
	}
	action := provider.actions[0]
	if action.Zone != "zone_1" || action.RecordName != "app.example.com" || action.RecordType != "A" || len(action.Values) != 0 || action.TTL != 120 || !action.Proxied {
		t.Fatalf("unexpected provider delete action %#v", action)
	}
	if len(store.lockedDNSManagedRecords) == 0 || store.lockedDNSManagedRecords[0] != "record_1" {
		t.Fatalf("delete should share the DNS evaluation lock, got %#v", store.lockedDNSManagedRecords)
	}
}

func TestUpdateDNSManagedRecordUsesEvaluationLock(t *testing.T) {
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		credentialZones: []repo.DNSCredentialZoneRecord{{
			ID: "zone_ref_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", ZoneID: "zone_1", ZoneName: "example.com", Status: "ACTIVE",
		}},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", CredentialZoneID: "zone_ref_1", ZoneID: "zone_1", ZoneName: "example.com", RecordHost: "www", RecordName: "www.example.com", RecordType: "A", TTL: 60,
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	_, err := control.UpdateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1", DNSManagedRecordMutationInput{
		DNSCredentialID:  "credential_1",
		CredentialZoneID: "zone_ref_1",
		RecordHost:       "www",
		RecordType:       "A",
		TTL:              120,
	})
	if err != nil {
		t.Fatalf("update managed record: %v", err)
	}
	if len(store.lockedDNSManagedRecords) == 0 || store.lockedDNSManagedRecords[0] != "record_1" {
		t.Fatalf("update should share the DNS evaluation lock, got %#v", store.lockedDNSManagedRecords)
	}
}

func TestEvaluateDNSManagedRecordReportsEqualPriorityConflict(t *testing.T) {
	store := &healthDNSTestStore{managedRecords: []repo.DNSManagedRecordRecord{{
		ID:             "record_1",
		OrganizationID: "org_1",
		RecordType:     "A",
		Instances: []repo.DNSInstanceRecord{
			{ID: "instance_1", OrganizationID: "org_1", ManagedRecordID: "record_1", Priority: 10, Enabled: true, ConditionJSON: `{}`, ActionJSON: `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.1"]}`},
			{ID: "instance_2", OrganizationID: "org_1", ManagedRecordID: "record_1", Priority: 10, Enabled: true, ConditionJSON: `{}`, ActionJSON: `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.2"]}`},
		},
	}}}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	payload, err := control.EvaluateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if err != nil {
		t.Fatalf("evaluate managed record: %v", err)
	}
	if payload.LastEvaluationStatus != "CONFLICT" {
		t.Fatalf("expected conflict status, got %#v", payload)
	}
	if len(payload.LastDiagnostics) != 1 || payload.LastDiagnostics[0].Code != "AMBIGUOUS_PRIORITY" {
		t.Fatalf("expected ambiguous priority diagnostic, got %#v", payload.LastDiagnostics)
	}
}

func TestBestEffortDNSEvaluationUsesDetachedTimeoutContext(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID:              "record_1",
			OrganizationID:  "org_1",
			DNSCredentialID: "credential_1",
			ZoneID:          "zone_1",
			RecordName:      "app.example.com",
			RecordType:      "A",
			TTL:             60,
			Instances: []repo.DNSInstanceRecord{{
				ID:              "instance_1",
				OrganizationID:  "org_1",
				ManagedRecordID: "record_1",
				Priority:        10,
				Enabled:         true,
				ConditionJSON:   `{}`,
				ActionJSON:      `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
			}},
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer:             healthDNSTestAuthorizer{},
		DNSSecretEncryptionKey: "test-secret-key",
		DNSProviders:           dns.StaticProviderRegistry{"CLOUDFLARE": provider},
	})
	encrypted, err := control.encryptDNSSecret("provider-token")
	if err != nil {
		t.Fatalf("encrypt dns secret: %v", err)
	}
	store.credential.EncryptedSecret = encrypted
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	control.evaluateDNSManagedRecordsBestEffort(canceled, "org_1", []string{"record_1"})

	if len(provider.actions) != 1 {
		t.Fatalf("expected provider apply despite canceled caller context, got %#v", provider.actions)
	}
	if provider.contextErr != nil {
		t.Fatalf("expected detached provider context, got %v", provider.contextErr)
	}
	if !provider.hasDeadline {
		t.Fatalf("expected detached provider context to retain a timeout deadline")
	}
}

func TestEvaluateDNSManagedRecordFailsWhenOnlineNodesHaveNoPublishAddresses(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{
		nodes: []repo.NodeRecord{{
			ID:             "node_1",
			OrganizationID: "org_1",
			Status:         "ONLINE",
			GroupIDs:       []string{"node_group_1"},
		}},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID:                    "record_1",
			OrganizationID:        "org_1",
			RecordType:            "A",
			LastAppliedValuesJSON: `["192.0.2.10"]`,
			Instances: []repo.DNSInstanceRecord{{
				ID:               "instance_1",
				OrganizationID:   "org_1",
				ManagedRecordID:  "record_1",
				Priority:         10,
				Enabled:          true,
				NodeGroupIDsJSON: `["node_group_1"]`,
				AnswerCount:      -1,
				ConditionJSON:    `{}`,
				ActionJSON:       `{"type":"ROTATE_ONLINE_NODES"}`,
			}},
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}, DNSProviders: dns.StaticProviderRegistry{"CLOUDFLARE": provider}})

	payload, err := control.EvaluateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if err != nil {
		t.Fatalf("evaluate managed record: %v", err)
	}
	if payload.LastEvaluationStatus != "FAILED" {
		t.Fatalf("expected failed status, got %#v", payload)
	}
	if len(payload.LastAppliedValues) != 1 || payload.LastAppliedValues[0] != "192.0.2.10" {
		t.Fatalf("previous applied values should be preserved, got %#v", payload.LastAppliedValues)
	}
	if len(provider.actions) != 0 {
		t.Fatalf("provider must not be called for missing publish addresses, got %#v", provider.actions)
	}
	if len(payload.LastDiagnostics) == 0 || payload.LastDiagnostics[0].Code != "NO_ONLINE_NODE_ADDRESSES" {
		t.Fatalf("expected no online addresses diagnostic, got %#v", payload.LastDiagnostics)
	}
}

func TestEvaluateDNSManagedRecordMarksInstanceFailedAfterProviderError(t *testing.T) {
	provider := &recordingDNSProvider{err: errors.New("provider rejected update")}
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID:                    "record_1",
			OrganizationID:        "org_1",
			DNSCredentialID:       "credential_1",
			ZoneID:                "zone_1",
			RecordName:            "app.example.com",
			RecordType:            "A",
			LastAppliedValuesJSON: `["192.0.2.10"]`,
			LastEvaluationStatus:  "APPLIED",
			Instances: []repo.DNSInstanceRecord{{
				ID:              "instance_1",
				OrganizationID:  "org_1",
				ManagedRecordID: "record_1",
				Priority:        10,
				Enabled:         true,
				ConditionJSON:   `{}`,
				ActionJSON:      `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.20"]}`,
			}},
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer:             healthDNSTestAuthorizer{},
		DNSSecretEncryptionKey: "test-secret-key",
		DNSProviders:           dns.StaticProviderRegistry{"CLOUDFLARE": provider},
	})
	encrypted, err := control.encryptDNSSecret("provider-token")
	if err != nil {
		t.Fatalf("encrypt dns secret: %v", err)
	}
	store.credential.EncryptedSecret = encrypted

	payload, err := control.EvaluateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if err != nil {
		t.Fatalf("evaluate managed record: %v", err)
	}
	if payload.LastEvaluationStatus != "FAILED" {
		t.Fatalf("expected failed record status, got %#v", payload)
	}
	instance := store.managedRecords[0].Instances[0]
	if instance.LastStatus != "FAILED" {
		t.Fatalf("expected failed instance status, got %#v", instance)
	}
}

func TestUpdateDNSManagedRecordRejectsSameTypeConflictBeforeProviderDelete(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		credentialZones: []repo.DNSCredentialZoneRecord{{
			ID: "zone_ref_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", ZoneID: "zone_1", ZoneName: "example.com", Status: "ACTIVE",
		}},
		managedRecords: []repo.DNSManagedRecordRecord{
			{ID: "record_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", CredentialZoneID: "zone_ref_1", ZoneID: "zone_1", ZoneName: "example.com", RecordHost: "old", RecordName: "old.example.com", RecordType: "A", TTL: 60, LastAppliedValuesJSON: `["192.0.2.10"]`},
			{ID: "record_2", OrganizationID: "org_1", DNSCredentialID: "credential_1", CredentialZoneID: "zone_ref_1", ZoneID: "zone_1", ZoneName: "example.com", RecordHost: "dup", RecordName: "dup.example.com", RecordType: "A", TTL: 60},
		},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer:             healthDNSTestAuthorizer{},
		DNSSecretEncryptionKey: "test-secret-key",
		DNSProviders:           dns.StaticProviderRegistry{"CLOUDFLARE": provider},
	})
	encrypted, err := control.encryptDNSSecret("provider-token")
	if err != nil {
		t.Fatalf("encrypt dns secret: %v", err)
	}
	store.credential.EncryptedSecret = encrypted

	_, err = control.UpdateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1", DNSManagedRecordMutationInput{
		DNSCredentialID:  "credential_1",
		CredentialZoneID: "zone_ref_1",
		RecordHost:       "dup",
		RecordType:       "A",
		TTL:              60,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for duplicate record, got %v", err)
	}
	if len(provider.actions) != 0 {
		t.Fatalf("provider delete must not run when local validation fails, got %#v", provider.actions)
	}
}

func TestUpdateDNSManagedRecordPreservesCurrentUnavailableZone(t *testing.T) {
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		credentialZones: []repo.DNSCredentialZoneRecord{{
			ID: "zone_ref_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", ZoneID: "zone_1", ZoneName: "legacy.example", Status: "UNKNOWN",
		}},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", CredentialZoneID: "zone_ref_1", ZoneID: "zone_1", ZoneName: "legacy.example", RecordHost: "www", RecordName: "www.legacy.example", RecordType: "A", TTL: 60,
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	payload, err := control.UpdateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1", DNSManagedRecordMutationInput{
		DNSCredentialID:  "credential_1",
		CredentialZoneID: "zone_ref_1",
		RecordHost:       "www",
		RecordType:       "A",
		TTL:              120,
	})
	if err != nil {
		t.Fatalf("update record with current unavailable zone: %v", err)
	}
	if payload.TTL != 120 || payload.CredentialZoneID != "zone_ref_1" {
		t.Fatalf("expected update to preserve current zone, got %#v", payload)
	}
}

func TestUpdateDNSManagedRecordPreservesAppliedValuesForSettingsOnlyChange(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		credentialZones: []repo.DNSCredentialZoneRecord{{
			ID: "zone_ref_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", ZoneID: "zone_1", ZoneName: "example.com", Status: "ACTIVE",
		}},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", CredentialZoneID: "zone_ref_1", ZoneID: "zone_1", ZoneName: "example.com", RecordHost: "www", RecordName: "www.example.com", RecordType: "A", TTL: 60, Proxied: false, LastAppliedValuesJSON: `["192.0.2.10"]`, LastEvaluationStatus: "APPLIED",
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer:             healthDNSTestAuthorizer{},
		DNSSecretEncryptionKey: "test-secret-key",
		DNSProviders:           dns.StaticProviderRegistry{"CLOUDFLARE": provider},
	})
	encrypted, err := control.encryptDNSSecret("provider-token")
	if err != nil {
		t.Fatalf("encrypt dns secret: %v", err)
	}
	store.credential.EncryptedSecret = encrypted

	payload, err := control.UpdateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1", DNSManagedRecordMutationInput{
		DNSCredentialID:  "credential_1",
		CredentialZoneID: "zone_ref_1",
		RecordHost:       "www",
		RecordType:       "A",
		TTL:              120,
		Proxied:          true,
	})
	if err != nil {
		t.Fatalf("update record settings: %v", err)
	}
	if payload.LastEvaluationStatus != "PENDING" || len(payload.LastAppliedValues) != 1 || payload.LastAppliedValues[0] != "192.0.2.10" {
		t.Fatalf("settings-only update should preserve applied values and mark pending, got %#v", payload)
	}
	if len(provider.actions) != 0 {
		t.Fatalf("settings-only update should not delete provider record immediately, got %#v", provider.actions)
	}
}

func TestEvaluateDNSManagedRecordReconcilesSettingsOnlyPendingState(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID:                    "record_1",
			OrganizationID:        "org_1",
			DNSCredentialID:       "credential_1",
			ZoneID:                "zone_1",
			RecordName:            "www.example.com",
			RecordType:            "A",
			TTL:                   120,
			Proxied:               true,
			LastAppliedValuesJSON: `["192.0.2.10"]`,
			LastEvaluationStatus:  "PENDING",
			Instances: []repo.DNSInstanceRecord{{
				ID:              "instance_1",
				OrganizationID:  "org_1",
				ManagedRecordID: "record_1",
				Priority:        10,
				Enabled:         true,
				ConditionJSON:   `{}`,
				ActionJSON:      `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
			}},
		}},
	}
	provider.inTx = func() bool { return store.txDepth > 0 }
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer:             healthDNSTestAuthorizer{},
		DNSSecretEncryptionKey: "test-secret-key",
		DNSProviders:           dns.StaticProviderRegistry{"CLOUDFLARE": provider},
	})
	encrypted, err := control.encryptDNSSecret("provider-token")
	if err != nil {
		t.Fatalf("encrypt dns secret: %v", err)
	}
	store.credential.EncryptedSecret = encrypted

	payload, err := control.EvaluateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if err != nil {
		t.Fatalf("evaluate settings-only pending record: %v", err)
	}
	if payload.LastEvaluationStatus != "APPLIED" {
		t.Fatalf("expected applied status, got %#v", payload)
	}
	if len(provider.actions) != 1 || provider.actions[0].TTL != 120 || !provider.actions[0].Proxied || len(provider.actions[0].Values) != 1 || provider.actions[0].Values[0] != "192.0.2.10" {
		t.Fatalf("expected provider reconcile with same values and new settings, got %#v", provider.actions)
	}
	if provider.calledInTx {
		t.Fatalf("provider apply must run outside the database transaction")
	}
}

func TestUpdateDNSManagedRecordRejectsTypeChangeWithIncompatibleInstances(t *testing.T) {
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		credentialZones: []repo.DNSCredentialZoneRecord{{
			ID: "zone_ref_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", ZoneID: "zone_1", ZoneName: "example.com", Status: "ACTIVE",
		}},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", CredentialZoneID: "zone_ref_1", ZoneID: "zone_1", ZoneName: "example.com", RecordHost: "www", RecordName: "www.example.com", RecordType: "A", TTL: 60,
			Instances: []repo.DNSInstanceRecord{{
				ID:              "instance_1",
				OrganizationID:  "org_1",
				ManagedRecordID: "record_1",
				Enabled:         true,
				ActionJSON:      `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
			}},
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	_, err := control.UpdateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1", DNSManagedRecordMutationInput{
		DNSCredentialID:  "credential_1",
		CredentialZoneID: "zone_ref_1",
		RecordHost:       "www",
		RecordType:       "CNAME",
		TTL:              60,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for incompatible type change, got %v", err)
	}
	if store.managedRecords[0].RecordType != "A" {
		t.Fatalf("record type should not change after rejected update: %#v", store.managedRecords[0])
	}
}

func TestDeleteDNSInstanceClearsActiveManagedRecordReference(t *testing.T) {
	store := &healthDNSTestStore{managedRecords: []repo.DNSManagedRecordRecord{{
		ID: "record_1", OrganizationID: "org_1", ActiveInstanceID: "instance_1",
		Instances: []repo.DNSInstanceRecord{{ID: "instance_1", OrganizationID: "org_1", ManagedRecordID: "record_1", Enabled: true}},
	}}}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	err := control.DeleteDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "instance_1")
	if err != nil {
		t.Fatalf("delete DNS instance: %v", err)
	}
	if store.managedRecords[0].ActiveInstanceID != "" {
		t.Fatalf("expected active instance to be cleared, got %#v", store.managedRecords[0])
	}
	if store.managedRecords[0].Instances[0].DeletedAt == "" {
		t.Fatalf("expected instance to be soft-deleted")
	}
}

func TestUpdateDNSInstanceEvaluatesOldActiveReferenceWhenDisabledOrMoved(t *testing.T) {
	store := &healthDNSTestStore{managedRecords: []repo.DNSManagedRecordRecord{
		{
			ID: "record_1", OrganizationID: "org_1", ActiveInstanceID: "instance_1", RecordType: "A", LastEvaluationStatus: "APPLIED",
			Instances: []repo.DNSInstanceRecord{{
				ID:              "instance_1",
				OrganizationID:  "org_1",
				ManagedRecordID: "record_1",
				Name:            "active",
				Priority:        10,
				Enabled:         true,
				ConditionJSON:   `{}`,
				ActionJSON:      `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
			}},
		},
		{ID: "record_2", OrganizationID: "org_1", RecordType: "A"},
	}}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	_, err := control.UpdateDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "instance_1", DNSInstanceMutationInput{
		ManagedRecordID: "record_2",
		Name:            "moved",
		Priority:        10,
		Enabled:         false,
		AnswerCount:     -1,
		Condition:       map[string]any{},
		Action:          map[string]any{"type": "SET_STATIC_ADDRESSES", "values": []any{"192.0.2.20"}},
	})
	if err != nil {
		t.Fatalf("update DNS instance: %v", err)
	}
	if store.managedRecords[0].ActiveInstanceID != "" {
		t.Fatalf("old managed record active instance should be cleared, got %#v", store.managedRecords[0])
	}
	if store.managedRecords[0].LastEvaluationStatus != "NO_MATCH" {
		t.Fatalf("old managed record should be re-evaluated after active instance moves, got %#v", store.managedRecords[0])
	}
}

func TestUpdateInactiveDNSInstanceMarksRecordPending(t *testing.T) {
	store := &healthDNSTestStore{managedRecords: []repo.DNSManagedRecordRecord{{
		ID: "record_1", OrganizationID: "org_1", ActiveInstanceID: "instance_1", RecordType: "A", LastEvaluationStatus: "APPLIED", LastAppliedValuesJSON: `["192.0.2.10"]`,
		Instances: []repo.DNSInstanceRecord{
			{ID: "instance_1", OrganizationID: "org_1", ManagedRecordID: "record_1", Name: "active", Priority: 10, Enabled: true, ConditionJSON: `{}`, ActionJSON: `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`},
			{ID: "instance_2", OrganizationID: "org_1", ManagedRecordID: "record_1", Name: "inactive", Priority: 20, Enabled: true, ConditionJSON: `{}`, ActionJSON: `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.20"]}`},
		},
	}}}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	_, err := control.UpdateDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "instance_2", DNSInstanceMutationInput{
		ManagedRecordID: "record_1",
		Name:            "inactive",
		Priority:        5,
		Enabled:         true,
		AnswerCount:     -1,
		Condition:       map[string]any{},
		Action:          map[string]any{"type": "SET_STATIC_ADDRESSES", "values": []any{"192.0.2.20"}},
	})
	if err != nil {
		t.Fatalf("update inactive DNS instance: %v", err)
	}
	if store.managedRecords[0].LastEvaluationStatus != "PENDING" {
		t.Fatalf("managed record should be pending after inactive instance becomes eligible, got %#v", store.managedRecords[0])
	}
}

func TestUpdateDNSInstanceMarksReferencingRecordsPending(t *testing.T) {
	store := &healthDNSTestStore{managedRecords: []repo.DNSManagedRecordRecord{
		{
			ID: "record_1", OrganizationID: "org_1", RecordType: "A", LastEvaluationStatus: "APPLIED",
			Instances: []repo.DNSInstanceRecord{{ID: "instance_1", OrganizationID: "org_1", ManagedRecordID: "record_1", Name: "source", Priority: 10, Enabled: true, ConditionJSON: `{}`, ActionJSON: `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`}},
		},
		{
			ID: "record_2", OrganizationID: "org_1", RecordType: "A", LastEvaluationStatus: "APPLIED",
			Instances: []repo.DNSInstanceRecord{{ID: "instance_2", OrganizationID: "org_1", ManagedRecordID: "record_2", Name: "consumer", Priority: 10, Enabled: true, ConditionJSON: `{}`, ActionJSON: `{"type":"USE_INSTANCE_OUTPUT","instance_id":"instance_1"}`}},
		},
	}}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	_, err := control.UpdateDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "instance_1", DNSInstanceMutationInput{
		ManagedRecordID: "record_1",
		Name:            "source",
		Priority:        10,
		Enabled:         true,
		AnswerCount:     -1,
		Condition:       map[string]any{},
		Action:          map[string]any{"type": "SET_STATIC_ADDRESSES", "values": []any{"192.0.2.30"}},
	})
	if err != nil {
		t.Fatalf("update referenced DNS instance: %v", err)
	}
	if store.managedRecords[1].LastEvaluationStatus != "PENDING" {
		t.Fatalf("consumer managed record should be pending after referenced instance changes, got %#v", store.managedRecords[1])
	}
}

func TestEvaluateDNSManagedRecordRejectsDisabledReferencedInstance(t *testing.T) {
	store := &healthDNSTestStore{managedRecords: []repo.DNSManagedRecordRecord{
		{
			ID: "record_1", OrganizationID: "org_1", RecordType: "A",
			Instances: []repo.DNSInstanceRecord{{
				ID:              "instance_1",
				OrganizationID:  "org_1",
				ManagedRecordID: "record_1",
				Priority:        10,
				Enabled:         true,
				ConditionJSON:   `{}`,
				ActionJSON:      `{"type":"USE_INSTANCE_OUTPUT","instance_id":"instance_2"}`,
			}},
		},
		{
			ID: "record_2", OrganizationID: "org_1", RecordType: "A",
			Instances: []repo.DNSInstanceRecord{{
				ID:              "instance_2",
				OrganizationID:  "org_1",
				ManagedRecordID: "record_2",
				Priority:        10,
				Enabled:         false,
				ConditionJSON:   `{}`,
				ActionJSON:      `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.20"]}`,
			}},
		},
	}}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	payload, err := control.EvaluateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if err != nil {
		t.Fatalf("evaluate managed record: %v", err)
	}
	if payload.LastEvaluationStatus != "FAILED" {
		t.Fatalf("expected failed evaluation, got %#v", payload)
	}
	if len(payload.LastDiagnostics) == 0 || payload.LastDiagnostics[len(payload.LastDiagnostics)-1].Code != "REFERENCED_INSTANCE_DISABLED" {
		t.Fatalf("expected disabled reference diagnostic, got %#v", payload.LastDiagnostics)
	}
}

func TestCreateDNSInstanceRejectsBlankStaticAddressValues(t *testing.T) {
	store := &healthDNSTestStore{managedRecords: []repo.DNSManagedRecordRecord{{
		ID: "record_1", OrganizationID: "org_1", RecordType: "A",
	}}}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	_, err := control.CreateDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), DNSInstanceMutationInput{
		ManagedRecordID: "record_1",
		Name:            "blank static",
		Priority:        10,
		Enabled:         true,
		AnswerCount:     -1,
		Condition:       map[string]any{},
		Action:          map[string]any{"type": "SET_STATIC_ADDRESSES", "values": []any{" ", ""}},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for blank static values, got %v", err)
	}
}

func TestCreateDNSInstanceRequiresUseScopeForAllNodeGroups(t *testing.T) {
	store := &healthDNSTestStore{managedRecords: []repo.DNSManagedRecordRecord{{
		ID: "record_1", OrganizationID: "org_1", RecordType: "A",
	}}}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer: healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
	})

	_, err := control.CreateDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), DNSInstanceMutationInput{
		ManagedRecordID: "record_1",
		Name:            "all groups",
		Priority:        10,
		Enabled:         true,
		AnswerCount:     -1,
		Condition:       map[string]any{},
		Action:          map[string]any{"type": "SET_STATIC_ADDRESSES", "values": []any{"192.0.2.10"}},
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden for all-node DNS instance without wildcard scope, got %v", err)
	}
}

func TestCreateDNSInstanceRejectsNodeGroupsOutsideUseScope(t *testing.T) {
	store := &healthDNSTestStore{
		nodeGroups: map[string]repo.NodeGroupRecord{
			"node_group_1": {ID: "node_group_1", OrganizationID: "org_1"},
			"node_group_2": {ID: "node_group_2", OrganizationID: "org_1"},
		},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", RecordType: "A",
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer: healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
	})

	_, err := control.CreateDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), DNSInstanceMutationInput{
		ManagedRecordID: "record_1",
		Name:            "scoped groups",
		Priority:        10,
		Enabled:         true,
		NodeGroupIDs:    []string{"node_group_1", "node_group_2"},
		AnswerCount:     -1,
		Condition:       map[string]any{},
		Action:          map[string]any{"type": "SET_STATIC_ADDRESSES", "values": []any{"192.0.2.10"}},
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden for DNS instance outside node group scope, got %v", err)
	}
}

func TestCreateDNSInstanceRejectsReferencedInstanceOutsideUseScope(t *testing.T) {
	store := &healthDNSTestStore{
		nodeGroups: map[string]repo.NodeGroupRecord{
			"node_group_1": {ID: "node_group_1", OrganizationID: "org_1"},
			"node_group_2": {ID: "node_group_2", OrganizationID: "org_1"},
		},
		managedRecords: []repo.DNSManagedRecordRecord{
			{ID: "record_1", OrganizationID: "org_1", RecordType: "A"},
			{
				ID: "record_2", OrganizationID: "org_1", RecordType: "A",
				Instances: []repo.DNSInstanceRecord{{
					ID:               "instance_2",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_2",
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_2"]`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.20"]}`,
				}},
			},
		},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer: healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
	})

	_, err := control.CreateDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), DNSInstanceMutationInput{
		ManagedRecordID: "record_1",
		Name:            "referenced output",
		Priority:        10,
		Enabled:         true,
		NodeGroupIDs:    []string{"node_group_1"},
		AnswerCount:     -1,
		Condition:       map[string]any{},
		Action:          map[string]any{"type": "USE_INSTANCE_OUTPUT", "instance_id": "instance_2"},
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden for referenced instance outside node group scope, got %v", err)
	}
}

func TestUpdateDNSInstanceRejectsExistingNodeGroupsOutsideUseScope(t *testing.T) {
	store := &healthDNSTestStore{
		nodeGroups: map[string]repo.NodeGroupRecord{
			"node_group_1": {ID: "node_group_1", OrganizationID: "org_1"},
			"node_group_2": {ID: "node_group_2", OrganizationID: "org_1"},
		},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", RecordType: "A",
			Instances: []repo.DNSInstanceRecord{{
				ID:               "instance_1",
				OrganizationID:   "org_1",
				ManagedRecordID:  "record_1",
				Name:             "scoped",
				Priority:         10,
				Enabled:          true,
				NodeGroupIDsJSON: `["node_group_2"]`,
				ConditionJSON:    `{}`,
				ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.20"]}`,
			}},
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer: healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
	})

	_, err := control.UpdateDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "instance_1", DNSInstanceMutationInput{
		ManagedRecordID: "record_1",
		Name:            "move scoped",
		Priority:        10,
		Enabled:         true,
		NodeGroupIDs:    []string{"node_group_1"},
		AnswerCount:     -1,
		Condition:       map[string]any{},
		Action:          map[string]any{"type": "SET_STATIC_ADDRESSES", "values": []any{"192.0.2.10"}},
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden for updating existing DNS instance outside node group scope, got %v", err)
	}
}

func TestEvaluateDNSManagedRecordRejectsNestedReferencedInstanceOutsideUseScope(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{managedRecords: []repo.DNSManagedRecordRecord{
		{
			ID: "record_1", OrganizationID: "org_1", RecordType: "A",
			Instances: []repo.DNSInstanceRecord{{
				ID:               "instance_1",
				OrganizationID:   "org_1",
				ManagedRecordID:  "record_1",
				Priority:         10,
				Enabled:          true,
				NodeGroupIDsJSON: `["node_group_1"]`,
				ConditionJSON:    `{}`,
				ActionJSON:       `{"type":"USE_INSTANCE_OUTPUT","instance_id":"instance_2"}`,
			}},
		},
		{
			ID: "record_2", OrganizationID: "org_1", RecordType: "A",
			Instances: []repo.DNSInstanceRecord{{
				ID:               "instance_2",
				OrganizationID:   "org_1",
				ManagedRecordID:  "record_2",
				Priority:         10,
				Enabled:          true,
				NodeGroupIDsJSON: `["node_group_1"]`,
				ConditionJSON:    `{}`,
				ActionJSON:       `{"type":"USE_INSTANCE_OUTPUT","instance_id":"instance_3"}`,
			}},
		},
		{
			ID: "record_3", OrganizationID: "org_1", RecordType: "A",
			Instances: []repo.DNSInstanceRecord{{
				ID:               "instance_3",
				OrganizationID:   "org_1",
				ManagedRecordID:  "record_3",
				Priority:         10,
				Enabled:          true,
				NodeGroupIDsJSON: `["node_group_2"]`,
				ConditionJSON:    `{}`,
				ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.30"]}`,
			}},
		},
	}}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer:   healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
		DNSProviders: dns.StaticProviderRegistry{"CLOUDFLARE": provider},
	})

	_, err := control.EvaluateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden for nested referenced instance outside scope, got %v", err)
	}
	if len(provider.actions) != 0 {
		t.Fatalf("provider must not be called for forbidden nested reference, got %#v", provider.actions)
	}
}

func TestEvaluateDNSManagedRecordRejectsInstancesOutsideUseScope(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{managedRecords: []repo.DNSManagedRecordRecord{{
		ID: "record_1", OrganizationID: "org_1", RecordType: "A",
		Instances: []repo.DNSInstanceRecord{{
			ID:               "instance_1",
			OrganizationID:   "org_1",
			ManagedRecordID:  "record_1",
			Priority:         10,
			Enabled:          true,
			NodeGroupIDsJSON: `["node_group_2"]`,
			ConditionJSON:    `{}`,
			ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
		}},
	}}}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer:   healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
		DNSProviders: dns.StaticProviderRegistry{"CLOUDFLARE": provider},
	})

	_, err := control.EvaluateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden for evaluating out-of-scope DNS instance, got %v", err)
	}
	if len(provider.actions) != 0 {
		t.Fatalf("provider must not be called for forbidden evaluate, got %#v", provider.actions)
	}
}

func TestRecordNodeAgentHelloMarksDNSPolicyPendingForNewAutoAddress(t *testing.T) {
	store := &healthDNSTestStore{
		nodes: []repo.NodeRecord{{
			ID:             "node_1",
			OrganizationID: "org_1",
			Status:         "ONLINE",
			GroupIDs:       []string{"node_group_1"},
			DNSPublishAddresses: []repo.NodeDNSPublishAddressRecord{{
				ID: "address_1", OrganizationID: "org_1", NodeID: "node_1", AddressType: "A", Address: "198.51.100.10", Source: "AUTO", Enabled: true,
			}},
		}},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", RecordType: "A", LastEvaluationStatus: "APPLIED",
			Instances: []repo.DNSInstanceRecord{{
				ID:               "instance_1",
				OrganizationID:   "org_1",
				ManagedRecordID:  "record_1",
				Priority:         10,
				Enabled:          true,
				NodeGroupIDsJSON: `["node_group_1"]`,
				ConditionJSON:    `{}`,
				ActionJSON:       `{"type":"ROTATE_ONLINE_NODES"}`,
			}},
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	_, _, err := control.RecordNodeAgentHello(context.Background(), "org_1", "node_1", AgentHelloInput{RemoteAddr: "203.0.113.25:443", Version: "v0.1.0"})
	if err != nil {
		t.Fatalf("record node hello: %v", err)
	}
	if store.managedRecords[0].LastEvaluationStatus != "PENDING" {
		t.Fatalf("DNS policy should be pending after auto publish address changes, got %#v", store.managedRecords[0])
	}
}

func TestEvaluateDNSManagedRecordReloadsAfterEvaluationLock(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID:                    "record_1",
			OrganizationID:        "org_1",
			DNSCredentialID:       "credential_1",
			ZoneID:                "zone_1",
			RecordName:            "app.example.com",
			RecordType:            "A",
			LastAppliedValuesJSON: `["192.0.2.10"]`,
			Instances: []repo.DNSInstanceRecord{{
				ID:              "instance_1",
				OrganizationID:  "org_1",
				ManagedRecordID: "record_1",
				Priority:        10,
				Enabled:         true,
				ConditionJSON:   `{}`,
				ActionJSON:      `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.20"]}`,
			}},
		}},
	}
	store.onLockDNSManagedRecord = func(recordID string) {
		if recordID == "record_1" {
			store.managedRecords[0].LastAppliedValuesJSON = `["192.0.2.20"]`
			store.managedRecords[0].LastEvaluationStatus = "APPLIED"
		}
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer:             healthDNSTestAuthorizer{},
		DNSSecretEncryptionKey: "test-secret-key",
		DNSProviders:           dns.StaticProviderRegistry{"CLOUDFLARE": provider},
	})
	encrypted, err := control.encryptDNSSecret("provider-token")
	if err != nil {
		t.Fatalf("encrypt dns secret: %v", err)
	}
	store.credential.EncryptedSecret = encrypted

	payload, err := control.EvaluateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if err != nil {
		t.Fatalf("evaluate managed record: %v", err)
	}
	if payload.LastEvaluationStatus != "APPLIED" {
		t.Fatalf("expected applied status, got %#v", payload)
	}
	if len(provider.actions) != 0 {
		t.Fatalf("stale pre-lock state should not trigger provider apply, got %#v", provider.actions)
	}
}
