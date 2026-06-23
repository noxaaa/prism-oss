package service

import (
	"context"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/dns"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestEvaluateDNSManagedRecordDoesNotTreatPendingNodesAsOffline(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		nodes: []repo.NodeRecord{{
			ID:             "node_1",
			OrganizationID: "org_1",
			Status:         "PENDING",
			GroupIDs:       []string{"node_group_1"},
		}},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID:                    "record_1",
			OrganizationID:        "org_1",
			DNSCredentialID:       "credential_1",
			ZoneID:                "zone_1",
			RecordHost:            "app",
			RecordName:            "app.example.com",
			RecordType:            "A",
			TTL:                   60,
			ActiveInstanceID:      "instance_default",
			LastAppliedValuesJSON: `["198.51.100.20"]`,
			LastEvaluationStatus:  "APPLIED",
			Instances: []repo.DNSInstanceRecord{
				{
					ID:               "instance_failover",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_1",
					Priority:         10,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_1"]`,
					ConditionJSON:    `{"metric":"offline_node_count","comparator":">=","value":1}`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
				},
				{
					ID:               "instance_default",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_1",
					Priority:         20,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_1"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["198.51.100.20"]}`,
				},
			},
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
	if payload.ActiveInstanceID != "instance_default" {
		t.Fatalf("pending node should not trigger offline failover, got active instance %q payload %#v", payload.ActiveInstanceID, payload)
	}
	if len(provider.actions) != 0 {
		t.Fatalf("default output already applied; pending node should not force provider write, got %#v", provider.actions)
	}
}

func TestUpdateDNSInstanceLocksInstanceBeforeRecordOwners(t *testing.T) {
	store := &healthDNSTestStore{
		managedRecords: []repo.DNSManagedRecordRecord{
			{
				ID:             "record_1",
				OrganizationID: "org_1",
				RecordName:     "old.example.com",
				RecordType:     "A",
				Instances: []repo.DNSInstanceRecord{{
					ID:              "instance_1",
					OrganizationID:  "org_1",
					ManagedRecordID: "record_1",
					Name:            "policy",
					Priority:        10,
					Enabled:         true,
					ConditionJSON:   `{}`,
					ActionJSON:      `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
				}},
			},
			{ID: "record_2", OrganizationID: "org_1", RecordName: "new.example.com", RecordType: "A"},
		},
	}
	store.onLockDNSManagedRecord = func(recordID string) {
		if !testStringSliceContains(store.lockedDNSInstances, "instance_1") {
			t.Fatalf("record %s was locked before the DNS instance mutation lock", recordID)
		}
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	_, err := control.UpdateDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "instance_1", DNSInstanceMutationInput{
		ManagedRecordID: "record_2",
		Name:            "policy",
		Priority:        10,
		Enabled:         true,
		Condition:       map[string]any{},
		Action:          map[string]any{"type": "SET_STATIC_ADDRESSES", "values": []any{"192.0.2.20"}},
	})
	if err != nil {
		t.Fatalf("update dns instance: %v", err)
	}
	if !testStringSliceContains(store.lockedDNSInstances, "instance_1") {
		t.Fatalf("expected DNS instance mutation lock, got %#v", store.lockedDNSInstances)
	}
	if len(store.lockedDNSManagedRecords) < 2 || store.lockedDNSManagedRecords[0] != "record_1" || store.lockedDNSManagedRecords[1] != "record_2" {
		t.Fatalf("expected sorted current/target record locks after instance lock, got %#v", store.lockedDNSManagedRecords)
	}
	if len(store.managedRecords[1].Instances) != 1 || store.managedRecords[1].Instances[0].ID != "instance_1" {
		t.Fatalf("expected instance to move to requested record_2, got %#v", store.managedRecords)
	}
}

func TestListDNSManagedRecordsKeepsStaleReferencedInstanceVisibleForRepair(t *testing.T) {
	store := &healthDNSTestStore{
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID:             "record_1",
			OrganizationID: "org_1",
			RecordName:     "app.example.com",
			RecordType:     "A",
			Instances: []repo.DNSInstanceRecord{{
				ID:               "instance_stale",
				OrganizationID:   "org_1",
				ManagedRecordID:  "record_1",
				Name:             "stale",
				Priority:         10,
				Enabled:          true,
				NodeGroupIDsJSON: `["node_group_1"]`,
				ConditionJSON:    `{}`,
				ActionJSON:       `{"type":"USE_INSTANCE_OUTPUT","instance_id":"missing_instance"}`,
			}},
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	records, err := control.ListDNSManagedRecords(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSRead)))
	if err != nil {
		t.Fatalf("list managed records: %v", err)
	}
	if len(records) != 1 || len(records[0].Instances) != 1 || records[0].Instances[0].ID != "instance_stale" {
		t.Fatalf("expected stale reference to remain visible for repair, got %#v", records)
	}
}

func TestListDNSManagedRecordsStillHidesExistingReferencedInstanceOutsideScope(t *testing.T) {
	store := &healthDNSTestStore{
		managedRecords: []repo.DNSManagedRecordRecord{
			{
				ID: "record_source", OrganizationID: "org_1", RecordName: "source.example.com", RecordType: "A",
				Instances: []repo.DNSInstanceRecord{{
					ID:               "instance_source",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_source",
					Name:             "source",
					Priority:         10,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_2"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.20"]}`,
				}},
			},
			{
				ID: "record_consumer", OrganizationID: "org_1", RecordName: "consumer.example.com", RecordType: "A",
				Instances: []repo.DNSInstanceRecord{{
					ID:               "instance_consumer",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_consumer",
					Name:             "consumer",
					Priority:         10,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_1"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"USE_INSTANCE_OUTPUT","instance_id":"instance_source"}`,
				}},
			},
		},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer: healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
	})

	records, err := control.ListDNSManagedRecords(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSRead)))
	if err != nil {
		t.Fatalf("list managed records: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected existing out-of-scope reference to stay hidden, got %#v", records)
	}
}

func TestUpdateDNSInstanceRejectsCurrentParentOutsideUseScope(t *testing.T) {
	store := &healthDNSTestStore{
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_mixed", OrganizationID: "org_1", RecordName: "mixed.example.com", RecordType: "A",
			Instances: []repo.DNSInstanceRecord{
				{
					ID:               "instance_visible",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_mixed",
					Name:             "visible",
					Priority:         10,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_1"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
				},
				{
					ID:               "instance_hidden",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_mixed",
					Name:             "hidden",
					Priority:         20,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_2"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.20"]}`,
				},
			},
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer: healthDNSTestAuthorizer{allowedNodeGroups: map[string]bool{"node_group_1": true}},
	})

	_, err := control.UpdateDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "instance_visible", DNSInstanceMutationInput{
		ManagedRecordID: "record_mixed",
		Name:            "visible edited",
		Priority:        10,
		Enabled:         true,
		NodeGroupIDs:    []string{"node_group_1"},
		AnswerCount:     -1,
		Condition:       map[string]any{},
		Action:          map[string]any{"type": "SET_STATIC_ADDRESSES", "values": []any{"192.0.2.11"}},
	})
	if err != ErrForbidden {
		t.Fatalf("expected forbidden for updating instance under partially out-of-scope parent, got %v", err)
	}
	if store.managedRecords[0].Instances[0].Name != "visible" {
		t.Fatalf("forbidden update must not mutate instance, got %#v", store.managedRecords[0].Instances[0])
	}
}

func TestUpdateDNSInstanceLocksOwnerAndConsumersInSortedOrder(t *testing.T) {
	store := &healthDNSTestStore{
		managedRecords: []repo.DNSManagedRecordRecord{
			{
				ID: "record_z", OrganizationID: "org_1", RecordName: "source.example.com", RecordType: "A",
				Instances: []repo.DNSInstanceRecord{{
					ID:              "instance_source",
					OrganizationID:  "org_1",
					ManagedRecordID: "record_z",
					Name:            "source",
					Priority:        10,
					Enabled:         true,
					ConditionJSON:   `{}`,
					ActionJSON:      `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
				}},
			},
			{
				ID: "record_a", OrganizationID: "org_1", RecordName: "consumer.example.com", RecordType: "A",
				Instances: []repo.DNSInstanceRecord{{
					ID:              "instance_consumer",
					OrganizationID:  "org_1",
					ManagedRecordID: "record_a",
					Name:            "consumer",
					Priority:        10,
					Enabled:         true,
					ConditionJSON:   `{}`,
					ActionJSON:      `{"type":"USE_INSTANCE_OUTPUT","instance_id":"instance_source"}`,
				}},
			},
		},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	_, err := control.UpdateDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "instance_source", DNSInstanceMutationInput{
		ManagedRecordID: "record_z",
		Name:            "source",
		Priority:        10,
		Enabled:         true,
		Condition:       map[string]any{},
		Action:          map[string]any{"type": "SET_STATIC_ADDRESSES", "values": []any{"192.0.2.11"}},
	})
	if err != nil {
		t.Fatalf("update dns instance: %v", err)
	}
	if len(store.lockedDNSManagedRecords) < 2 || store.lockedDNSManagedRecords[0] != "record_a" || store.lockedDNSManagedRecords[1] != "record_z" {
		t.Fatalf("expected owner and consumer records to be locked in sorted order, got %#v", store.lockedDNSManagedRecords)
	}
}

func TestDispatchDNSNotificationsUsesFreshContext(t *testing.T) {
	store := &healthDNSTestStore{respectContextCancellation: true}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	control.dispatchPreparedDNSNotificationsBestEffort(ctx, []preparedDNSNotification{{
		channel: repo.NotificationChannelRecord{
			ID:             "channel_1",
			OrganizationID: "org_1",
			ChannelType:    "UNSUPPORTED",
			Enabled:        true,
		},
		payload:   []byte(`{"event_type":"DNS_POLICY_FAILED"}`),
		createdAt: "2026-06-23T00:00:00Z",
		delivery: repo.NotificationDeliveryRecord{
			ID:                 "delivery_1",
			OrganizationID:     "org_1",
			ChannelID:          "channel_1",
			DNSManagedRecordID: "record_1",
			DNSInstanceID:      "instance_1",
			EventType:          "DNS_POLICY_FAILED",
			PayloadJSON:        `{"event_type":"DNS_POLICY_FAILED"}`,
			CreatedAt:          "2026-06-23T00:00:00Z",
		},
	}})
	if len(store.notificationDeliveries) != 1 {
		t.Fatalf("expected notification delivery to be recorded with a fresh context, got %#v", store.notificationDeliveries)
	}
	if store.notificationDeliveries[0].Status != "FAILED" {
		t.Fatalf("expected unsupported notification send to be recorded as failed, got %#v", store.notificationDeliveries[0])
	}
}

func TestUpdateDNSManagedRecordLocksOwnerAndConsumersInSortedOrder(t *testing.T) {
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		credentialZones: []repo.DNSCredentialZoneRecord{{
			ID: "zone_ref_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", ZoneID: "zone_1", ZoneName: "example.com", Status: "ACTIVE",
		}},
		managedRecords: []repo.DNSManagedRecordRecord{
			{
				ID: "record_z", OrganizationID: "org_1", DNSCredentialID: "credential_1", CredentialZoneID: "zone_ref_1", ZoneID: "zone_1", ZoneName: "example.com", RecordHost: "source", RecordName: "source.example.com", RecordType: "A", TTL: 60,
				Instances: []repo.DNSInstanceRecord{{
					ID:              "instance_source",
					OrganizationID:  "org_1",
					ManagedRecordID: "record_z",
					Name:            "source",
					Priority:        10,
					Enabled:         true,
					ConditionJSON:   `{}`,
					ActionJSON:      `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
				}},
			},
			{
				ID: "record_a", OrganizationID: "org_1", RecordName: "consumer.example.com", RecordType: "A",
				Instances: []repo.DNSInstanceRecord{{
					ID:              "instance_consumer",
					OrganizationID:  "org_1",
					ManagedRecordID: "record_a",
					Name:            "consumer",
					Priority:        10,
					Enabled:         true,
					ConditionJSON:   `{}`,
					ActionJSON:      `{"type":"USE_INSTANCE_OUTPUT","instance_id":"instance_source"}`,
				}},
			},
		},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	_, err := control.UpdateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_z", DNSManagedRecordMutationInput{
		DNSCredentialID:  "credential_1",
		CredentialZoneID: "zone_ref_1",
		RecordHost:       "source",
		RecordType:       "A",
		TTL:              120,
	})
	if err != nil {
		t.Fatalf("update managed record: %v", err)
	}
	if len(store.lockedDNSManagedRecords) < 2 || store.lockedDNSManagedRecords[0] != "record_a" || store.lockedDNSManagedRecords[1] != "record_z" {
		t.Fatalf("expected owner and consumer records to be locked in sorted order, got %#v", store.lockedDNSManagedRecords)
	}
}

func TestDeleteDNSManagedRecordLocksOwnerAndConsumersInSortedOrder(t *testing.T) {
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		managedRecords: []repo.DNSManagedRecordRecord{
			{
				ID: "record_z", OrganizationID: "org_1", DNSCredentialID: "credential_1", ZoneID: "zone_1", RecordName: "source.example.com", RecordType: "A", LastAppliedValuesJSON: "[]",
				Instances: []repo.DNSInstanceRecord{{
					ID:              "instance_source",
					OrganizationID:  "org_1",
					ManagedRecordID: "record_z",
					Name:            "source",
					Priority:        10,
					Enabled:         true,
					ConditionJSON:   `{}`,
					ActionJSON:      `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
				}},
			},
			{
				ID: "record_a", OrganizationID: "org_1", RecordName: "consumer.example.com", RecordType: "A",
				Instances: []repo.DNSInstanceRecord{{
					ID:              "instance_consumer",
					OrganizationID:  "org_1",
					ManagedRecordID: "record_a",
					Name:            "consumer",
					Priority:        10,
					Enabled:         true,
					ConditionJSON:   `{}`,
					ActionJSON:      `{"type":"USE_INSTANCE_OUTPUT","instance_id":"instance_source"}`,
				}},
			},
		},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	err := control.DeleteDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_z")
	if err != nil {
		t.Fatalf("delete managed record: %v", err)
	}
	if len(store.lockedDNSManagedRecords) < 2 || store.lockedDNSManagedRecords[0] != "record_a" || store.lockedDNSManagedRecords[1] != "record_z" {
		t.Fatalf("expected owner and consumer records to be locked in sorted order, got %#v", store.lockedDNSManagedRecords)
	}
}

func testStringSliceContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
