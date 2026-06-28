package service

import (
	"context"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestDeleteDNSManagedRecordAllowsCredentialDeleteWhenNoProviderState(t *testing.T) {
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
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	err := control.DeleteDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if err != nil {
		t.Fatalf("delete managed record: %v", err)
	}
	if store.managedRecords[0].DeletedAt == "" {
		t.Fatalf("expected managed record to be tombstoned, got %#v", store.managedRecords[0])
	}
	err = control.DeleteDNSCredential(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "credential_1")
	if err != nil {
		t.Fatalf("delete credential after managed record delete: %v", err)
	}
	if store.deletedCredentialID != "credential_1" {
		t.Fatalf("expected credential to be deleted, got %q", store.deletedCredentialID)
	}
}

func TestDNSManagedRecordEvaluationSnapshotMatchesPostgresTimestampText(t *testing.T) {
	snapshot := newDNSManagedRecordEvaluationSnapshot(repo.DNSManagedRecordRecord{
		ID:                    "record_1",
		OrganizationID:        "org_1",
		DNSCredentialID:       "credential_1",
		CredentialZoneID:      "zone_ref_1",
		ZoneID:                "zone_1",
		ZoneName:              "example.com",
		RecordHost:            "app",
		RecordName:            "app.example.com",
		RecordType:            "A",
		TTL:                   60,
		LastAppliedValuesJSON: "[]",
		LastEvaluationStatus:  "DELETE_PENDING",
		UpdatedAt:             "2026-06-23T04:30:45.123456789Z",
	})
	if !snapshot.matches(repo.DNSManagedRecordRecord{
		ID:                    "record_1",
		OrganizationID:        "org_1",
		DNSCredentialID:       "credential_1",
		CredentialZoneID:      "zone_ref_1",
		ZoneID:                "zone_1",
		ZoneName:              "example.com",
		RecordHost:            "app",
		RecordName:            "app.example.com",
		RecordType:            "A",
		TTL:                   60,
		LastAppliedValuesJSON: "[]",
		LastEvaluationStatus:  "DELETE_PENDING",
		UpdatedAt:             "2026-06-23 04:30:45.123457+00",
	}) {
		t.Fatalf("expected snapshot comparison to normalize PostgreSQL timestamptz text and microsecond precision")
	}
}

func TestDNSManagedRecordEvaluationSnapshotNormalizesEmptyProviderRetirements(t *testing.T) {
	snapshot := newDNSManagedRecordEvaluationSnapshot(repo.DNSManagedRecordRecord{
		ID:                      "record_1",
		OrganizationID:          "org_1",
		DNSCredentialID:         "credential_1",
		CredentialZoneID:        "zone_ref_1",
		ZoneID:                  "zone_1",
		ZoneName:                "example.com",
		RecordHost:              "app",
		RecordName:              "app.example.com",
		RecordType:              "A",
		TTL:                     60,
		LastAppliedValuesJSON:   "[]",
		LastEvaluationStatus:    "DELETE_PENDING",
		ProviderRetirementsJSON: "",
	})
	if !snapshot.matches(repo.DNSManagedRecordRecord{
		ID:                      "record_1",
		OrganizationID:          "org_1",
		DNSCredentialID:         "credential_1",
		CredentialZoneID:        "zone_ref_1",
		ZoneID:                  "zone_1",
		ZoneName:                "example.com",
		RecordHost:              "app",
		RecordName:              "app.example.com",
		RecordType:              "A",
		TTL:                     60,
		LastAppliedValuesJSON:   "[]",
		LastEvaluationStatus:    "DELETE_PENDING",
		ProviderRetirementsJSON: "[]",
	}) {
		t.Fatalf("expected snapshot comparison to normalize empty provider retirements")
	}
}
