package service

import (
	"context"
	"errors"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/dns"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestDeleteDNSManagedRecordRejectsInstancesOutsideUseScope(t *testing.T) {
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
			LastEvaluationStatus:  "APPLIED",
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
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer:   healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
		DNSProviders: dns.StaticProviderRegistry{"CLOUDFLARE": provider},
	})

	err := control.DeleteDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden for deleting out-of-scope DNS instance, got %v", err)
	}
	if len(provider.actions) != 0 {
		t.Fatalf("provider must not be called for forbidden delete, got %#v", provider.actions)
	}
	if store.managedRecords[0].LastEvaluationStatus == "DELETE_PENDING" {
		t.Fatalf("forbidden delete must not persist delete intent")
	}
}

func TestListDNSInstancesFiltersInstancesOutsideUseScope(t *testing.T) {
	store := &healthDNSTestStore{
		managedRecords: []repo.DNSManagedRecordRecord{
			{
				ID: "record_visible", OrganizationID: "org_1", RecordType: "A",
				Instances: []repo.DNSInstanceRecord{{
					ID:               "instance_visible",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_visible",
					Priority:         10,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_1"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
				}},
			},
			{
				ID: "record_hidden", OrganizationID: "org_1", RecordType: "A",
				Instances: []repo.DNSInstanceRecord{{
					ID:               "instance_hidden",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_hidden",
					Priority:         10,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_2"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.20"]}`,
				}},
			},
		},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer: healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
	})

	instances, err := control.ListDNSInstances(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSRead)))
	if err != nil {
		t.Fatalf("list dns instances: %v", err)
	}
	if len(instances) != 1 || instances[0].ID != "instance_visible" {
		t.Fatalf("expected only in-scope DNS instance, got %#v", instances)
	}
}

func TestListDNSManagedRecordsFiltersEmbeddedInstancesOutsideUseScope(t *testing.T) {
	store := &healthDNSTestStore{
		managedRecords: []repo.DNSManagedRecordRecord{
			{
				ID: "record_visible", OrganizationID: "org_1", RecordType: "A",
				Instances: []repo.DNSInstanceRecord{
					{
						ID:               "instance_visible",
						OrganizationID:   "org_1",
						ManagedRecordID:  "record_visible",
						Priority:         10,
						Enabled:          true,
						NodeGroupIDsJSON: `["node_group_1"]`,
						ConditionJSON:    `{}`,
						ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
					},
					{
						ID:               "instance_hidden",
						OrganizationID:   "org_1",
						ManagedRecordID:  "record_visible",
						Priority:         20,
						Enabled:          true,
						NodeGroupIDsJSON: `["node_group_2"]`,
						ConditionJSON:    `{}`,
						ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.20"]}`,
					},
				},
			},
			{
				ID: "record_hidden", OrganizationID: "org_1", RecordType: "A",
				Instances: []repo.DNSInstanceRecord{{
					ID:               "instance_only_hidden",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_hidden",
					Priority:         10,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_2"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.30"]}`,
				}},
			},
		},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer: healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
	})

	records, err := control.ListDNSManagedRecords(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSRead)))
	if err != nil {
		t.Fatalf("list dns managed records: %v", err)
	}
	if len(records) != 1 || records[0].ID != "record_visible" {
		t.Fatalf("expected only records with visible instances, got %#v", records)
	}
	if len(records[0].Instances) != 1 || records[0].Instances[0].ID != "instance_visible" {
		t.Fatalf("expected embedded instances to be scope-filtered, got %#v", records[0].Instances)
	}
}

func TestListDNSManagedRecordsRedactsActiveStateWhenActiveInstanceIsOutsideUseScope(t *testing.T) {
	store := &healthDNSTestStore{
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID:                    "record_visible",
			OrganizationID:        "org_1",
			RecordType:            "A",
			ActiveInstanceID:      "instance_hidden",
			LastAppliedValuesJSON: `["192.0.2.99"]`,
			LastAppliedAt:         "2026-06-19T00:00:00Z",
			LastEvaluationStatus:  "APPLIED",
			LastEvaluatedAt:       "2026-06-19T00:00:01Z",
			Instances: []repo.DNSInstanceRecord{
				{
					ID:               "instance_visible",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_visible",
					Priority:         20,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_1"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
				},
				{
					ID:               "instance_hidden",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_visible",
					Priority:         10,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_2"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.99"]}`,
				},
			},
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer: healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
	})

	records, err := control.ListDNSManagedRecords(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSRead)))
	if err != nil {
		t.Fatalf("list dns managed records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected scoped record to remain visible, got %#v", records)
	}
	if records[0].ActiveInstanceID != "" || len(records[0].LastAppliedValues) != 0 || records[0].LastAppliedAt != "" || records[0].LastEvaluationStatus != "" {
		t.Fatalf("expected hidden active state to be redacted, got %#v", records[0])
	}
	if len(records[0].Instances) != 1 || records[0].Instances[0].ID != "instance_visible" {
		t.Fatalf("expected visible instance to remain embedded, got %#v", records[0].Instances)
	}
}

func TestListDNSManagedRecordsRedactsProviderStateWhenActiveSourceIsUnknownAndHiddenInstancesExist(t *testing.T) {
	store := &healthDNSTestStore{
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID:                    "record_visible",
			OrganizationID:        "org_1",
			RecordType:            "A",
			LastAppliedValuesJSON: `["192.0.2.99"]`,
			LastAppliedAt:         "2026-06-19T00:00:00Z",
			LastEvaluationStatus:  "NO_MATCH",
			LastDiagnosticsJSON:   `[{"code":"NO_MATCHED_INSTANCE","message":"No DNS instance matched this record."}]`,
			LastEvaluatedAt:       "2026-06-19T00:00:01Z",
			Instances: []repo.DNSInstanceRecord{
				{
					ID:               "instance_visible",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_visible",
					Priority:         20,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_1"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
				},
				{
					ID:               "instance_hidden",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_visible",
					Priority:         10,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_2"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.99"]}`,
				},
			},
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer: healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
	})

	records, err := control.ListDNSManagedRecords(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSRead)))
	if err != nil {
		t.Fatalf("list dns managed records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected scoped record to remain visible, got %#v", records)
	}
	if len(records[0].LastAppliedValues) != 0 || records[0].LastAppliedAt != "" || records[0].LastEvaluationStatus != "" || len(records[0].LastDiagnostics) != 0 {
		t.Fatalf("expected unknown-source provider state to be redacted, got %#v", records[0])
	}
	if len(records[0].Instances) != 1 || records[0].Instances[0].ID != "instance_visible" {
		t.Fatalf("expected visible instance to remain embedded, got %#v", records[0].Instances)
	}
}

func TestListDNSManagedRecordsRedactsDiagnosticsWhenHiddenInstancesExist(t *testing.T) {
	store := &healthDNSTestStore{
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID:                    "record_visible",
			OrganizationID:        "org_1",
			RecordType:            "A",
			ActiveInstanceID:      "instance_visible",
			LastAppliedValuesJSON: `["192.0.2.10"]`,
			LastAppliedAt:         "2026-06-19T00:00:00Z",
			LastEvaluationStatus:  "APPLIED",
			LastDiagnosticsJSON:   `[{"code":"HIDDEN_CONDITION_DETAIL","message":"Hidden policy branch detail."}]`,
			LastEvaluatedAt:       "2026-06-19T00:00:01Z",
			Instances: []repo.DNSInstanceRecord{
				{
					ID:               "instance_visible",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_visible",
					Priority:         20,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_1"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
				},
				{
					ID:               "instance_hidden",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_visible",
					Priority:         10,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_2"]`,
					ConditionJSON:    `{"metric":"offline_node_count","comparator":">=","value":1}`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.99"]}`,
				},
			},
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer: healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
	})

	records, err := control.ListDNSManagedRecords(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSRead)))
	if err != nil {
		t.Fatalf("list dns managed records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected scoped record to remain visible, got %#v", records)
	}
	if records[0].ActiveInstanceID != "" || len(records[0].LastAppliedValues) != 0 || records[0].LastEvaluationStatus != "" || len(records[0].LastDiagnostics) != 0 {
		t.Fatalf("expected provider state with hidden diagnostics to be redacted, got %#v", records[0])
	}
	if len(records[0].Instances) != 1 || records[0].Instances[0].ID != "instance_visible" {
		t.Fatalf("expected visible instance to remain embedded, got %#v", records[0].Instances)
	}
}

func TestEvaluateDNSManagedRecordFiltersHiddenDisabledInstancesFromResponse(t *testing.T) {
	store := &healthDNSTestStore{
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID:                   "record_shared",
			OrganizationID:       "org_1",
			RecordType:           "A",
			LastEvaluationStatus: "DELETE_PENDING",
			Instances: []repo.DNSInstanceRecord{
				{
					ID:               "instance_visible",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_shared",
					Name:             "visible",
					Priority:         10,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_1"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
				},
				{
					ID:                         "instance_hidden_disabled",
					OrganizationID:             "org_1",
					ManagedRecordID:            "record_shared",
					Name:                       "hidden disabled",
					Priority:                   20,
					Enabled:                    false,
					NodeGroupIDsJSON:           `["node_group_2"]`,
					ConditionJSON:              `{}`,
					ActionJSON:                 `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.99"]}`,
					NotificationChannelIDsJSON: `["notification_hidden"]`,
				},
			},
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer: healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
	})

	payload, err := control.EvaluateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_shared")
	if err != nil {
		t.Fatalf("evaluate dns managed record: %v", err)
	}
	if len(payload.Instances) != 1 || payload.Instances[0].ID != "instance_visible" {
		t.Fatalf("expected evaluate response to filter hidden disabled instances, got %#v", payload.Instances)
	}
}

func TestUpdateDNSManagedRecordRejectsExistingInstancesOutsideUseScope(t *testing.T) {
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		credentialZones: []repo.DNSCredentialZoneRecord{{
			ID: "zone_ref_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", ZoneID: "zone_1", ZoneName: "example.com", Status: "ACTIVE",
		}},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", CredentialZoneID: "zone_ref_1", ZoneID: "zone_1", ZoneName: "example.com", RecordHost: "app", RecordName: "app.example.com", RecordType: "A", TTL: 60,
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
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer: healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
	})

	_, err := control.UpdateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1", DNSManagedRecordMutationInput{
		DNSCredentialID:  "credential_1",
		CredentialZoneID: "zone_ref_1",
		RecordHost:       "app",
		RecordType:       "A",
		TTL:              120,
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden for editing out-of-scope DNS instance parent, got %v", err)
	}
	if store.managedRecords[0].TTL != 60 {
		t.Fatalf("forbidden update must not mutate managed record, got %#v", store.managedRecords[0])
	}
}

func TestCreateDNSInstanceRejectsTargetRecordOutsideUseScope(t *testing.T) {
	store := &healthDNSTestStore{
		nodeGroups: map[string]repo.NodeGroupRecord{
			"node_group_1": {ID: "node_group_1", OrganizationID: "org_1"},
			"node_group_2": {ID: "node_group_2", OrganizationID: "org_1"},
		},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", RecordType: "A",
			Instances: []repo.DNSInstanceRecord{{
				ID:               "instance_hidden",
				OrganizationID:   "org_1",
				ManagedRecordID:  "record_1",
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

	_, err := control.CreateDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), DNSInstanceMutationInput{
		ManagedRecordID: "record_1",
		Name:            "visible",
		Priority:        1,
		Enabled:         true,
		NodeGroupIDs:    []string{"node_group_1"},
		AnswerCount:     -1,
		Condition:       map[string]any{},
		Action:          map[string]any{"type": "SET_STATIC_ADDRESSES", "values": []string{"192.0.2.10"}},
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden for attaching instance to out-of-scope parent record, got %v", err)
	}
	if len(store.managedRecords[0].Instances) != 1 {
		t.Fatalf("forbidden create must not attach instance, got %#v", store.managedRecords[0].Instances)
	}
}

func TestUpdateDNSInstanceRejectsMoveIntoTargetRecordOutsideUseScope(t *testing.T) {
	store := &healthDNSTestStore{
		nodeGroups: map[string]repo.NodeGroupRecord{
			"node_group_1": {ID: "node_group_1", OrganizationID: "org_1"},
			"node_group_2": {ID: "node_group_2", OrganizationID: "org_1"},
		},
		managedRecords: []repo.DNSManagedRecordRecord{
			{
				ID: "record_visible", OrganizationID: "org_1", RecordType: "A",
				Instances: []repo.DNSInstanceRecord{{
					ID:               "instance_visible",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_visible",
					Name:             "visible",
					Priority:         10,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_1"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
				}},
			},
			{
				ID: "record_hidden_parent", OrganizationID: "org_1", RecordType: "A",
				Instances: []repo.DNSInstanceRecord{{
					ID:               "instance_hidden",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_hidden_parent",
					Name:             "hidden",
					Priority:         10,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_2"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.20"]}`,
				}},
			},
		},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer: healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
	})

	_, err := control.UpdateDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "instance_visible", DNSInstanceMutationInput{
		ManagedRecordID: "record_hidden_parent",
		Name:            "visible",
		Priority:        1,
		Enabled:         true,
		NodeGroupIDs:    []string{"node_group_1"},
		AnswerCount:     -1,
		Condition:       map[string]any{},
		Action:          map[string]any{"type": "SET_STATIC_ADDRESSES", "values": []string{"192.0.2.10"}},
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden for moving instance into out-of-scope parent record, got %v", err)
	}
	if store.managedRecords[0].Instances[0].ManagedRecordID != "record_visible" || len(store.managedRecords[1].Instances) != 1 {
		t.Fatalf("forbidden move must not mutate instances, got %#v", store.managedRecords)
	}
}

func TestDeleteDNSInstanceRejectsInstanceOutsideUseScope(t *testing.T) {
	store := &healthDNSTestStore{
		managedRecords: []repo.DNSManagedRecordRecord{{
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
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer: healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
	})

	err := control.DeleteDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "instance_1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden for deleting out-of-scope DNS instance, got %v", err)
	}
	if store.managedRecords[0].Instances[0].DeletedAt != "" {
		t.Fatalf("forbidden delete must not mark instance deleted, got %#v", store.managedRecords[0].Instances[0])
	}
}

func TestDeleteDNSInstanceRejectsParentRecordOutsideUseScope(t *testing.T) {
	store := &healthDNSTestStore{
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_shared", OrganizationID: "org_1", RecordType: "A",
			Instances: []repo.DNSInstanceRecord{{
				ID:               "instance_visible",
				OrganizationID:   "org_1",
				ManagedRecordID:  "record_shared",
				Priority:         10,
				Enabled:          true,
				NodeGroupIDsJSON: `["node_group_1"]`,
				ConditionJSON:    `{}`,
				ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
			}, {
				ID:               "instance_hidden",
				OrganizationID:   "org_1",
				ManagedRecordID:  "record_shared",
				Priority:         20,
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

	err := control.DeleteDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "instance_visible")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden for deleting instance from out-of-scope parent record, got %v", err)
	}
	if store.managedRecords[0].Instances[0].DeletedAt != "" || store.managedRecords[0].Instances[1].DeletedAt != "" {
		t.Fatalf("forbidden delete must not mark shared parent instances deleted, got %#v", store.managedRecords[0].Instances)
	}
	if store.managedRecords[0].LastEvaluationStatus == "PENDING" {
		t.Fatalf("forbidden delete must not mark shared parent pending")
	}
}

func TestDeleteDNSManagedRecordRejectsDisabledInstancesOutsideUseScope(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", ZoneID: "zone_1", RecordName: "app.example.com", RecordType: "A", LastAppliedValuesJSON: "[]",
			Instances: []repo.DNSInstanceRecord{{
				ID:               "instance_1",
				OrganizationID:   "org_1",
				ManagedRecordID:  "record_1",
				Priority:         10,
				Enabled:          false,
				NodeGroupIDsJSON: `["node_group_2"]`,
				ConditionJSON:    `{}`,
				ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
			}},
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer:   healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
		DNSProviders: dns.StaticProviderRegistry{"CLOUDFLARE": provider},
	})

	err := control.DeleteDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden for deleting disabled out-of-scope DNS instance parent, got %v", err)
	}
	if store.managedRecords[0].DeletedAt != "" || store.managedRecords[0].LastEvaluationStatus == "DELETE_PENDING" {
		t.Fatalf("forbidden delete must not persist delete state, got %#v", store.managedRecords[0])
	}
	if len(provider.actions) != 0 {
		t.Fatalf("provider must not be called for forbidden delete, got %#v", provider.actions)
	}
}

func TestEvaluateDNSManagedRecordRejectsDeletePendingReferencedInstanceOutput(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		managedRecords: []repo.DNSManagedRecordRecord{
			{
				ID:                   "source_record",
				OrganizationID:       "org_1",
				DNSCredentialID:      "credential_1",
				ZoneID:               "zone_1",
				RecordName:           "source.example.com",
				RecordType:           "A",
				LastEvaluationStatus: "DELETE_PENDING",
				Instances: []repo.DNSInstanceRecord{{
					ID:              "source_instance",
					OrganizationID:  "org_1",
					ManagedRecordID: "source_record",
					Priority:        10,
					Enabled:         true,
					ConditionJSON:   `{}`,
					ActionJSON:      `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
				}},
			},
			{
				ID:                    "dependent_record",
				OrganizationID:        "org_1",
				DNSCredentialID:       "credential_1",
				ZoneID:                "zone_1",
				RecordName:            "app.example.com",
				RecordType:            "A",
				TTL:                   60,
				LastAppliedValuesJSON: `["192.0.2.99"]`,
				LastEvaluationStatus:  "PENDING",
				Instances: []repo.DNSInstanceRecord{{
					ID:              "dependent_instance",
					OrganizationID:  "org_1",
					ManagedRecordID: "dependent_record",
					Priority:        10,
					Enabled:         true,
					ConditionJSON:   `{}`,
					ActionJSON:      `{"type":"USE_INSTANCE_OUTPUT","instance_id":"source_instance"}`,
				}},
			},
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

	payload, err := control.EvaluateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "dependent_record")
	if err != nil {
		t.Fatalf("evaluate dependent managed record: %v", err)
	}
	if payload.LastEvaluationStatus != "FAILED" {
		t.Fatalf("expected failed evaluation for delete-pending reference, got %#v", payload)
	}
	if len(payload.LastDiagnostics) == 0 || payload.LastDiagnostics[len(payload.LastDiagnostics)-1].Code != "REFERENCED_INSTANCE_DELETE_PENDING" {
		t.Fatalf("expected delete-pending reference diagnostic, got %#v", payload.LastDiagnostics)
	}
	if len(provider.actions) != 0 {
		t.Fatalf("provider must not be called for delete-pending reference, got %#v", provider.actions)
	}
	if store.managedRecords[1].LastAppliedValuesJSON != `["192.0.2.99"]` {
		t.Fatalf("failed evaluation must preserve previous applied values, got %#v", store.managedRecords[1])
	}
}
