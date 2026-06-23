package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/dns"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestUpdateDNSManagedRecordPreservesAppliedValuesWhenCredentialRotationApplyFails(t *testing.T) {
	provider := &recordingDNSProvider{err: errors.New("new credential cannot write")}
	store := &healthDNSTestStore{
		credential:        repo.DNSCredentialRecord{ID: "credential_old", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		createdCredential: repo.DNSCredentialRecord{ID: "credential_new", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		credentialZones: []repo.DNSCredentialZoneRecord{
			{ID: "zone_ref_old", OrganizationID: "org_1", DNSCredentialID: "credential_old", ZoneID: "zone_1", ZoneName: "example.com", Status: "ACTIVE"},
			{ID: "zone_ref_new", OrganizationID: "org_1", DNSCredentialID: "credential_new", ZoneID: "zone_1", ZoneName: "example.com", Status: "ACTIVE"},
		},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", DNSCredentialID: "credential_old", CredentialZoneID: "zone_ref_old", ZoneID: "zone_1", ZoneName: "example.com", RecordHost: "www", RecordName: "www.example.com", RecordType: "A", TTL: 60, LastAppliedValuesJSON: `["192.0.2.10"]`, ProviderRetirementsJSON: "[]", LastEvaluationStatus: "APPLIED",
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
	encryptedOld, err := control.encryptDNSSecret("old-provider-token")
	if err != nil {
		t.Fatalf("encrypt old dns secret: %v", err)
	}
	encryptedNew, err := control.encryptDNSSecret("new-provider-token")
	if err != nil {
		t.Fatalf("encrypt new dns secret: %v", err)
	}
	store.credential.EncryptedSecret = encryptedOld
	store.createdCredential.EncryptedSecret = encryptedNew

	payload, err := control.UpdateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1", DNSManagedRecordMutationInput{
		DNSCredentialID:  "credential_new",
		CredentialZoneID: "zone_ref_new",
		RecordHost:       "www",
		RecordType:       "A",
		TTL:              60,
	})
	if err != nil {
		t.Fatalf("credential rotation should keep config durable despite provider failure: %v", err)
	}
	if payload.LastEvaluationStatus != "FAILED" || !strings.Contains(payload.LastEvaluationError, "new credential cannot write") {
		t.Fatalf("expected provider failure to be visible, got %#v", payload)
	}
	if store.managedRecords[0].LastAppliedValuesJSON != `["192.0.2.10"]` {
		t.Fatalf("credential-only rotation failure must preserve old applied values for cleanup, got %s", store.managedRecords[0].LastAppliedValuesJSON)
	}
	if store.managedRecords[0].ProviderRetirementsJSON != "[]" {
		t.Fatalf("same provider target should not enqueue retirement on credential rotation, got %s", store.managedRecords[0].ProviderRetirementsJSON)
	}
}

func TestEvaluateDNSManagedRecordTracksProviderWriteWhenSnapshotBecomesStale(t *testing.T) {
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", ZoneID: "zone_1", ZoneName: "example.com", RecordHost: "www", RecordName: "www.example.com", RecordType: "A", TTL: 60, LastAppliedValuesJSON: `[]`, ProviderRetirementsJSON: "[]", LastEvaluationStatus: "PENDING",
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
	provider := &recordingDNSProvider{afterApply: func(dns.ApplyRecordInput) {
		store.managedRecords[0].RecordHost = "api"
		store.managedRecords[0].RecordName = "api.example.com"
		store.managedRecords[0].UpdatedAt = "2026-06-23T10:30:00Z"
	}}
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
	if payload.LastEvaluationStatus != "PENDING" {
		t.Fatalf("stale evaluation should leave record pending, got %#v", payload)
	}
	if store.managedRecords[0].LastAppliedValuesJSON != `[]` {
		t.Fatalf("stale write to old target must not be tracked as current values, got %s", store.managedRecords[0].LastAppliedValuesJSON)
	}
	if !strings.Contains(store.managedRecords[0].ProviderRetirementsJSON, "www.example.com") {
		t.Fatalf("stale provider write must be tracked for cleanup, got %s", store.managedRecords[0].ProviderRetirementsJSON)
	}
}
