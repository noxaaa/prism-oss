package service

import (
	"context"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/dns"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestRecordNodeAgentHelloMarksDNSPolicyPendingWhenStaleAutoAddressIsDisabled(t *testing.T) {
	store := &healthDNSTestStore{
		nodes: []repo.NodeRecord{{
			ID:             "node_1",
			OrganizationID: "org_1",
			Status:         "ONLINE",
			GroupIDs:       []string{"node_group_1"},
			DNSPublishAddresses: []repo.NodeDNSPublishAddressRecord{
				{ID: "address_1", OrganizationID: "org_1", NodeID: "node_1", AddressType: "A", Address: "203.0.113.25", Source: "AUTO", Enabled: true},
				{ID: "address_2", OrganizationID: "org_1", NodeID: "node_1", AddressType: "A", Address: "198.51.100.10", Source: "AUTO", Enabled: true},
			},
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
		t.Fatalf("DNS policy should be pending after stale auto address is disabled, got %#v", store.managedRecords[0])
	}
	if store.nodes[0].DNSPublishAddresses[1].Enabled {
		t.Fatalf("stale same-type AUTO address should be disabled")
	}
}

func TestRecordNodeAgentHelloDisablesAutoAddressWithoutTrustedRemote(t *testing.T) {
	store := &healthDNSTestStore{
		nodes: []repo.NodeRecord{{
			ID:             "node_1",
			OrganizationID: "org_1",
			Status:         "ONLINE",
			GroupIDs:       []string{"node_group_1"},
			DNSPublishAddresses: []repo.NodeDNSPublishAddressRecord{{
				ID: "address_1", OrganizationID: "org_1", NodeID: "node_1", AddressType: "A", Address: "203.0.113.25", Source: "AUTO", Enabled: true,
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

	_, _, err := control.RecordNodeAgentHello(context.Background(), "org_1", "node_1", AgentHelloInput{RemoteAddr: "10.0.0.1:443", Version: "v0.1.0"})
	if err != nil {
		t.Fatalf("record node hello: %v", err)
	}
	if store.nodes[0].DNSPublishAddresses[0].Enabled {
		t.Fatalf("AUTO address should be disabled when remote address is not trustworthy")
	}
	if store.managedRecords[0].LastEvaluationStatus != "FAILED" || store.managedRecords[0].LastEvaluationError == "" {
		t.Fatalf("DNS policy should be re-evaluated after AUTO address is disabled, got %#v", store.managedRecords[0])
	}
}

func TestNodeDNSInputChangeEvaluatesReferencedDNSPolicyConsumers(t *testing.T) {
	store := &healthDNSTestStore{
		nodes: []repo.NodeRecord{{
			ID:             "node_1",
			OrganizationID: "org_1",
			Status:         "ONLINE",
			GroupIDs:       []string{"node_group_1"},
		}},
		managedRecords: []repo.DNSManagedRecordRecord{
			{
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
			},
			{
				ID: "record_2", OrganizationID: "org_1", RecordType: "A", LastEvaluationStatus: "APPLIED",
				Instances: []repo.DNSInstanceRecord{{
					ID:               "instance_2",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_2",
					Priority:         10,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_2"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"USE_INSTANCE_OUTPUT","instance_id":"instance_1"}`,
				}},
			},
		},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	err := control.MarkNodeAgentDisconnected(context.Background(), "org_1", "node_1")
	if err != nil {
		t.Fatalf("mark node disconnected: %v", err)
	}
	for _, record := range store.managedRecords {
		if record.LastEvaluationStatus != "FAILED" {
			t.Fatalf("record %s should be evaluated after node-driven DNS input change, got %#v", record.ID, record)
		}
	}
}

func TestPendingNodeDisconnectEvaluatesDNSPolicies(t *testing.T) {
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

	if err := control.MarkNodeAgentDisconnected(context.Background(), "org_1", "node_1"); err != nil {
		t.Fatalf("mark node disconnected: %v", err)
	}
	if len(provider.actions) != 1 {
		t.Fatalf("expected DNS provider apply after pending node becomes offline, got %d", len(provider.actions))
	}
	if got := provider.actions[0].Values; len(got) != 1 || got[0] != "192.0.2.10" {
		t.Fatalf("expected failover value to be applied, got %#v", got)
	}
	if store.managedRecords[0].ActiveInstanceID != "instance_failover" {
		t.Fatalf("expected offline condition to activate failover instance, got %#v", store.managedRecords[0])
	}
}

func TestNodeDNSInputChangeLocksAffectedRecordsInSortedOrder(t *testing.T) {
	store := &healthDNSTestStore{
		managedRecords: []repo.DNSManagedRecordRecord{
			{
				ID: "record_z", OrganizationID: "org_1", RecordType: "A", LastEvaluationStatus: "APPLIED",
				Instances: []repo.DNSInstanceRecord{{
					ID:               "instance_source",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_z",
					Priority:         10,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_1"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"ROTATE_ONLINE_NODES"}`,
				}},
			},
			{
				ID: "record_a", OrganizationID: "org_1", RecordType: "A", LastEvaluationStatus: "APPLIED",
				Instances: []repo.DNSInstanceRecord{{
					ID:              "instance_consumer",
					OrganizationID:  "org_1",
					ManagedRecordID: "record_a",
					Priority:        10,
					Enabled:         true,
					ConditionJSON:   `{}`,
					ActionJSON:      `{"type":"USE_INSTANCE_OUTPUT","instance_id":"instance_source"}`,
				}},
			},
		},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	err := store.WithinTx(context.Background(), func(ctx context.Context, repositories repo.Repositories) error {
		_, err := control.markDNSRecordsDependingOnNodeGroupsPending(ctx, repositories, "org_1", []string{"node_group_1"}, "now")
		return err
	})
	if err != nil {
		t.Fatalf("mark DNS records pending: %v", err)
	}
	if len(store.lockedDNSManagedRecords) != 2 || store.lockedDNSManagedRecords[0] != "record_a" || store.lockedDNSManagedRecords[1] != "record_z" {
		t.Fatalf("expected affected records to be locked once in sorted order, got %#v", store.lockedDNSManagedRecords)
	}
}

func TestGrouplessNodeDNSInputChangeDoesNotAffectGroupScopedInstances(t *testing.T) {
	store := &healthDNSTestStore{
		nodes: []repo.NodeRecord{{
			ID:             "node_1",
			OrganizationID: "org_1",
			Status:         "ONLINE",
			GroupIDs:       nil,
		}},
		managedRecords: []repo.DNSManagedRecordRecord{
			{
				ID: "record_grouped", OrganizationID: "org_1", RecordType: "A", LastEvaluationStatus: "APPLIED",
				Instances: []repo.DNSInstanceRecord{{
					ID:               "instance_grouped",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_grouped",
					Priority:         10,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_1"]`,
					ConditionJSON:    `{}`,
					ActionJSON:       `{"type":"ROTATE_ONLINE_NODES"}`,
				}},
			},
			{
				ID: "record_groupless", OrganizationID: "org_1", RecordType: "A", LastEvaluationStatus: "APPLIED",
				Instances: []repo.DNSInstanceRecord{{
					ID:              "instance_groupless",
					OrganizationID:  "org_1",
					ManagedRecordID: "record_groupless",
					Priority:        10,
					Enabled:         true,
					ConditionJSON:   `{}`,
					ActionJSON:      `{"type":"ROTATE_ONLINE_NODES"}`,
				}},
			},
		},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	if err := control.MarkNodeAgentDisconnected(context.Background(), "org_1", "node_1"); err != nil {
		t.Fatalf("mark node disconnected: %v", err)
	}
	if store.managedRecords[0].LastEvaluationStatus != "APPLIED" {
		t.Fatalf("group-scoped record should not be touched by groupless node changes, got %#v", store.managedRecords[0])
	}
	if store.managedRecords[1].LastEvaluationStatus != "FAILED" {
		t.Fatalf("groupless record should be evaluated after groupless node change, got %#v", store.managedRecords[1])
	}
}

func TestNodeDNSInputChangeAppliesMatchedDNSFailoverInstance(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		nodes: []repo.NodeRecord{{
			ID:             "node_1",
			OrganizationID: "org_1",
			Status:         "ONLINE",
			GroupIDs:       []string{"node_group_1"},
			DNSPublishAddresses: []repo.NodeDNSPublishAddressRecord{{
				ID: "address_1", OrganizationID: "org_1", NodeID: "node_1", AddressType: "A", Address: "203.0.113.25", Source: "AUTO", Enabled: true,
			}},
		}},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID:                    "record_1",
			OrganizationID:        "org_1",
			DNSCredentialID:       "credential_1",
			ZoneID:                "zone_1",
			RecordName:            "app.example.com",
			RecordType:            "A",
			TTL:                   60,
			ActiveInstanceID:      "instance_1",
			LastAppliedValuesJSON: `["203.0.113.25"]`,
			LastEvaluationStatus:  "APPLIED",
			Instances: []repo.DNSInstanceRecord{
				{
					ID:               "instance_1",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_1",
					Priority:         10,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_1"]`,
					ConditionJSON:    `{"metric":"online_node_count","comparator":">=","value":1}`,
					ActionJSON:       `{"type":"ROTATE_ONLINE_NODES"}`,
				},
				{
					ID:               "instance_2",
					OrganizationID:   "org_1",
					ManagedRecordID:  "record_1",
					Priority:         20,
					Enabled:          true,
					NodeGroupIDsJSON: `["node_group_1"]`,
					ConditionJSON:    `{"metric":"offline_node_count","comparator":">=","value":1}`,
					ActionJSON:       `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
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

	if err := control.MarkNodeAgentDisconnected(context.Background(), "org_1", "node_1"); err != nil {
		t.Fatalf("mark node disconnected: %v", err)
	}
	if len(provider.actions) != 1 {
		t.Fatalf("expected DNS provider apply after node input change, got %d", len(provider.actions))
	}
	if got := provider.actions[0].Values; len(got) != 1 || got[0] != "192.0.2.10" {
		t.Fatalf("expected failover value to be applied, got %#v", got)
	}
	if store.managedRecords[0].LastEvaluationStatus != "APPLIED" || store.managedRecords[0].ActiveInstanceID != "instance_2" {
		t.Fatalf("expected failover instance to become active, got %#v", store.managedRecords[0])
	}
}

func TestDeleteDNSManagedRecordMarksReferencedConsumersPending(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		managedRecords: []repo.DNSManagedRecordRecord{
			{
				ID: "record_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", ZoneID: "zone_1", RecordName: "source.example.com", RecordType: "A",
				Instances: []repo.DNSInstanceRecord{{
					ID:              "instance_1",
					OrganizationID:  "org_1",
					ManagedRecordID: "record_1",
					Priority:        10,
					Enabled:         true,
					ConditionJSON:   `{}`,
					ActionJSON:      `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
				}},
			},
			{
				ID: "record_2", OrganizationID: "org_1", RecordType: "A", LastEvaluationStatus: "APPLIED",
				Instances: []repo.DNSInstanceRecord{{
					ID:              "instance_2",
					OrganizationID:  "org_1",
					ManagedRecordID: "record_2",
					Priority:        10,
					Enabled:         true,
					ConditionJSON:   `{}`,
					ActionJSON:      `{"type":"USE_INSTANCE_OUTPUT","instance_id":"instance_1"}`,
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

	err = control.DeleteDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if err != nil {
		t.Fatalf("delete managed record: %v", err)
	}
	if store.managedRecords[1].LastEvaluationStatus != "PENDING" {
		t.Fatalf("referencing DNS record should be pending after referenced record delete, got %#v", store.managedRecords[1])
	}
}

func TestDeleteNodeEvaluatesDNSPolicies(t *testing.T) {
	store := &healthDNSTestStore{
		nodes: []repo.NodeRecord{{
			ID:             "node_1",
			OrganizationID: "org_1",
			Status:         "ONLINE",
			GroupIDs:       []string{"node_group_1"},
			DNSPublishAddresses: []repo.NodeDNSPublishAddressRecord{{
				ID: "address_1", OrganizationID: "org_1", NodeID: "node_1", AddressType: "A", Address: "203.0.113.25", Source: "AUTO", Enabled: true,
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

	err := control.DeleteNode(context.Background(), healthDNSTestIdentity(string(domain.PermissionNodesManage)), "node_1")
	if err != nil {
		t.Fatalf("delete node: %v", err)
	}
	if store.managedRecords[0].LastEvaluationStatus != "FAILED" {
		t.Fatalf("DNS policy should be evaluated after node delete, got %#v", store.managedRecords[0])
	}
}
