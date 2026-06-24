package service

import (
	"context"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/dns"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestCreateDNSInstanceEvaluatesManagedRecord(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", ZoneID: "zone_1", RecordName: "www.example.com", RecordType: "A", TTL: 60, LastEvaluationStatus: "APPLIED", LastAppliedValuesJSON: `["192.0.2.10"]`,
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

	instance, err := control.CreateDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), DNSInstanceMutationInput{
		ManagedRecordID: "record_1",
		Name:            "new policy",
		Priority:        5,
		Enabled:         true,
		AnswerCount:     -1,
		Condition:       map[string]any{},
		Action:          map[string]any{"type": "SET_STATIC_ADDRESSES", "values": []any{"192.0.2.20"}},
	})
	if err != nil {
		t.Fatalf("create DNS instance: %v", err)
	}
	if instance.LastStatus != "APPLIED" || len(instance.LastOutputValues) != 1 || instance.LastOutputValues[0] != "192.0.2.20" {
		t.Fatalf("create DNS instance response should include evaluated state, got %#v", instance)
	}
	if store.managedRecords[0].LastEvaluationStatus != "APPLIED" || store.managedRecords[0].LastAppliedValuesJSON != `["192.0.2.20"]` {
		t.Fatalf("managed record should be evaluated after enabled instance creation, got %#v", store.managedRecords[0])
	}
	if len(provider.actions) != 1 || len(provider.actions[0].Values) != 1 || provider.actions[0].Values[0] != "192.0.2.20" {
		t.Fatalf("expected provider apply after instance creation, got %#v", provider.actions)
	}
}

func TestUpdateDNSInstanceEvaluatesActiveManagedRecord(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{managedRecords: []repo.DNSManagedRecordRecord{{
		ID: "record_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", ZoneID: "zone_1", RecordName: "www.example.com", ActiveInstanceID: "instance_1", RecordType: "A", TTL: 60, LastEvaluationStatus: "APPLIED", LastAppliedValuesJSON: `["192.0.2.10"]`,
		Instances: []repo.DNSInstanceRecord{{
			ID:              "instance_1",
			OrganizationID:  "org_1",
			ManagedRecordID: "record_1",
			Name:            "active",
			Priority:        10,
			Enabled:         true,
			AnswerCount:     -1,
			ConditionJSON:   `{}`,
			ActionJSON:      `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
		}},
	}}}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer:             healthDNSTestAuthorizer{},
		DNSSecretEncryptionKey: "test-secret-key",
		DNSProviders:           dns.StaticProviderRegistry{"CLOUDFLARE": provider},
	})
	encrypted, err := control.encryptDNSSecret("provider-token")
	if err != nil {
		t.Fatalf("encrypt dns secret: %v", err)
	}
	store.credential = repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE", EncryptedSecret: encrypted}

	instance, err := control.UpdateDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "instance_1", DNSInstanceMutationInput{
		ManagedRecordID: "record_1",
		Name:            "active",
		Priority:        10,
		Enabled:         true,
		AnswerCount:     -1,
		Condition:       map[string]any{},
		Action:          map[string]any{"type": "SET_STATIC_ADDRESSES", "values": []any{"192.0.2.20"}},
	})
	if err != nil {
		t.Fatalf("update DNS instance: %v", err)
	}
	if instance.LastStatus != "APPLIED" || len(instance.LastOutputValues) != 1 || instance.LastOutputValues[0] != "192.0.2.20" {
		t.Fatalf("update DNS instance response should include evaluated state, got %#v", instance)
	}
	record := store.managedRecords[0]
	if record.ActiveInstanceID != "instance_1" || record.LastEvaluationStatus != "APPLIED" || record.LastAppliedValuesJSON != `["192.0.2.20"]` {
		t.Fatalf("active record should be re-evaluated after instance update, got %#v", record)
	}
	if len(provider.actions) != 1 || len(provider.actions[0].Values) != 1 || provider.actions[0].Values[0] != "192.0.2.20" {
		t.Fatalf("expected provider apply after instance update, got %#v", provider.actions)
	}
}

func TestUpdateDNSInstanceMetadataOnlyDoesNotReapplyManagedRecord(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{managedRecords: []repo.DNSManagedRecordRecord{{
		ID: "record_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", ZoneID: "zone_1", RecordName: "www.example.com", ActiveInstanceID: "instance_1", RecordType: "A", TTL: 60, LastEvaluationStatus: "APPLIED", LastAppliedValuesJSON: `["192.0.2.10"]`,
		Instances: []repo.DNSInstanceRecord{{
			ID:              "instance_1",
			OrganizationID:  "org_1",
			ManagedRecordID: "record_1",
			Name:            "active",
			Priority:        10,
			Enabled:         true,
			NodeGroupIDsJSON: `[]`,
			AnswerCount:     -1,
			ConditionJSON:   `{}`,
			ActionJSON:      `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
		}},
	}}}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer:             healthDNSTestAuthorizer{},
		DNSSecretEncryptionKey: "test-secret-key",
		DNSProviders:           dns.StaticProviderRegistry{"CLOUDFLARE": provider},
	})
	encrypted, err := control.encryptDNSSecret("provider-token")
	if err != nil {
		t.Fatalf("encrypt dns secret: %v", err)
	}
	store.credential = repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE", EncryptedSecret: encrypted}

	_, err = control.UpdateDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "instance_1", DNSInstanceMutationInput{
		ManagedRecordID: "record_1",
		Name:            "renamed",
		Priority:        10,
		Enabled:         true,
		AnswerCount:     -1,
		Condition:       map[string]any{},
		Action:          map[string]any{"type": "SET_STATIC_ADDRESSES", "values": []any{"192.0.2.10"}},
	})
	if err != nil {
		t.Fatalf("update DNS instance: %v", err)
	}
	record := store.managedRecords[0]
	if record.LastEvaluationStatus != "APPLIED" || record.LastAppliedValuesJSON != `["192.0.2.10"]` {
		t.Fatalf("metadata-only update should preserve applied managed record state, got %#v", record)
	}
	if len(provider.actions) != 0 {
		t.Fatalf("metadata-only update should not call DNS provider, got %#v", provider.actions)
	}
}
