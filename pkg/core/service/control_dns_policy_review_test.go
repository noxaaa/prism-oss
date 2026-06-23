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

func TestComposeDNSManagedRecordNameRejectsInvalidRelativeHost(t *testing.T) {
	for _, recordHost := range []string{"bad/path", "bad host", "-bad", "bad-", "bad..host"} {
		if _, _, err := composeDNSManagedRecordName(recordHost, "", "example.com"); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input for host %q, got %v", recordHost, err)
		}
	}
}

func TestComposeDNSManagedRecordNameRejectsOnlyFullyQualifiedRecordHost(t *testing.T) {
	host, name, err := composeDNSManagedRecordName("myexample.com", "", "example.com")
	if err != nil {
		t.Fatalf("expected relative host containing zone text to be valid: %v", err)
	}
	if host != "myexample.com" || name != "myexample.com.example.com" {
		t.Fatalf("unexpected composed relative host=%q name=%q", host, name)
	}
	if _, _, err := composeDNSManagedRecordName("www.example.com", "", "example.com"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected fully qualified host to be rejected, got %v", err)
	}
}

func TestComposeDNSManagedRecordNameAllowsUnderscoreOwnerLabels(t *testing.T) {
	host, name, err := composeDNSManagedRecordName("_acme-challenge.www", "", "example.com")
	if err != nil {
		t.Fatalf("compose underscore owner label: %v", err)
	}
	if host != "_acme-challenge.www" || name != "_acme-challenge.www.example.com" {
		t.Fatalf("unexpected composed owner label host=%q name=%q", host, name)
	}
}

func TestEvaluateDNSManagedRecordRejectsReferencedInstanceRecordTypeMismatch(t *testing.T) {
	store := &healthDNSTestStore{
		managedRecords: []repo.DNSManagedRecordRecord{
			{
				ID: "record_1", OrganizationID: "org_1", RecordName: "alias.example.com", RecordType: "CNAME",
				Instances: []repo.DNSInstanceRecord{{
					ID:              "instance_1",
					OrganizationID:  "org_1",
					ManagedRecordID: "record_1",
					Priority:        10,
					Enabled:         true,
					ConditionJSON:   `{}`,
					ActionJSON:      `{"type":"SET_STATIC_CNAME","value":"fallback.example.com"}`,
				}},
			},
			{
				ID: "record_2", OrganizationID: "org_1", RecordName: "app.example.com", RecordType: "A",
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
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	_, err := control.EvaluateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_2")
	if err != nil {
		t.Fatalf("evaluate managed record: %v", err)
	}
	if store.managedRecords[1].LastEvaluationStatus != "FAILED" {
		t.Fatalf("expected incompatible referenced output to fail evaluation, got %#v", store.managedRecords[1])
	}
	if !strings.Contains(store.managedRecords[1].LastDiagnosticsJSON, "REFERENCED_INSTANCE_RECORD_TYPE_MISMATCH") {
		t.Fatalf("expected mismatch diagnostic, got %s", store.managedRecords[1].LastDiagnosticsJSON)
	}
}

func TestDispatchDNSNotificationsReturnsUnexpectedLookupErrors(t *testing.T) {
	store := &healthDNSTestStore{
		notificationLookupErr: errors.New("notification repository unavailable"),
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID:                    "record_1",
			OrganizationID:        "org_1",
			RecordName:            "app.example.com",
			RecordType:            "A",
			LastAppliedValuesJSON: `["192.0.2.10"]`,
			LastEvaluationStatus:  "APPLIED",
			Instances: []repo.DNSInstanceRecord{{
				ID:                         "instance_1",
				OrganizationID:             "org_1",
				ManagedRecordID:            "record_1",
				Priority:                   10,
				Enabled:                    true,
				ConditionJSON:              `{}`,
				ActionJSON:                 `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
				NotificationChannelIDsJSON: `["channel_1"]`,
			}},
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	_, err := control.EvaluateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if err == nil || !strings.Contains(err.Error(), "notification repository unavailable") {
		t.Fatalf("expected notification lookup error to be returned, got %v", err)
	}
}

func TestEvaluateDNSManagedRecordKeepsProviderApplyWhenNotificationDeliveryFails(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{
		notificationDeliveryErr: errors.New("delivery table unavailable"),
		notificationChannels: []repo.NotificationChannelRecord{{
			ID:             "channel_1",
			OrganizationID: "org_1",
			ChannelType:    "UNSUPPORTED",
			Enabled:        true,
		}},
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
				ID:                         "instance_1",
				OrganizationID:             "org_1",
				ManagedRecordID:            "record_1",
				Priority:                   10,
				Enabled:                    true,
				ConditionJSON:              `{}`,
				ActionJSON:                 `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
				NotificationChannelIDsJSON: `["channel_1"]`,
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

	_, err = control.EvaluateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if err != nil {
		t.Fatalf("evaluate managed record: %v", err)
	}
	if len(provider.actions) != 1 || len(provider.actions[0].Values) != 1 || provider.actions[0].Values[0] != "192.0.2.10" {
		t.Fatalf("expected provider apply before notification delivery failure, got %#v", provider.actions)
	}
	if store.managedRecords[0].LastEvaluationStatus != "APPLIED" || store.managedRecords[0].LastAppliedValuesJSON != `["192.0.2.10"]` {
		t.Fatalf("expected applied DNS state to persist, got %#v", store.managedRecords[0])
	}
}

func TestDeleteNodeGroupRejectsDNSInstanceReferences(t *testing.T) {
	store := &healthDNSTestStore{
		nodeGroups: map[string]repo.NodeGroupRecord{
			"node_group_1": {ID: "node_group_1", OrganizationID: "org_1", Name: "primary"},
		},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", RecordType: "A",
			Instances: []repo.DNSInstanceRecord{{
				ID:               "instance_1",
				OrganizationID:   "org_1",
				ManagedRecordID:  "record_1",
				Enabled:          true,
				NodeGroupIDsJSON: `["node_group_1"]`,
				ActionJSON:       `{"type":"ROTATE_ONLINE_NODES"}`,
			}},
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	err := control.DeleteNodeGroup(context.Background(), healthDNSTestIdentity(string(domain.PermissionNodesManage)), "node_group_1")
	if err == nil || !strings.Contains(err.Error(), "node group") {
		t.Fatalf("expected node group delete to be blocked by DNS policy reference, got %v", err)
	}
}

func TestDeleteDNSManagedRecordKeepsLocalRecordRetryableWhenProviderCleanupFails(t *testing.T) {
	provider := &recordingDNSProvider{err: errors.New("provider cleanup failed")}
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

	err = control.DeleteDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if err == nil || !strings.Contains(err.Error(), "provider cleanup failed") {
		t.Fatalf("expected provider cleanup failure, got %v", err)
	}
	if store.deletedDNSManagedRecordID != "" || store.managedRecords[0].DeletedAt != "" {
		t.Fatalf("expected local record to remain retryable after provider cleanup failure, got %#v", store.managedRecords[0])
	}
}

func TestDeleteDNSManagedRecordPersistsDeleteIntentBeforeProviderCleanup(t *testing.T) {
	var statusDuringProviderCall string
	var deletedDuringProviderCall string
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
			TTL:                   60,
			LastAppliedValuesJSON: `["192.0.2.10"]`,
			LastEvaluationStatus:  "APPLIED",
		}},
	}
	provider.afterApply = func(input dns.ApplyRecordInput) {
		if len(input.Values) != 0 {
			return
		}
		statusDuringProviderCall = store.managedRecords[0].LastEvaluationStatus
		deletedDuringProviderCall = store.managedRecords[0].DeletedAt
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
	if statusDuringProviderCall != "DELETE_PENDING" || deletedDuringProviderCall != "" {
		t.Fatalf("provider cleanup must run after durable delete intent and before local tombstone, status=%q deleted_at=%q", statusDuringProviderCall, deletedDuringProviderCall)
	}
	if store.deletedDNSManagedRecordID != "record_1" || store.managedRecords[0].DeletedAt == "" {
		t.Fatalf("expected local record to be tombstoned after provider cleanup, got %#v", store.managedRecords[0])
	}
}

func TestDeleteDNSManagedRecordBlocksConcurrentEvaluationBeforeTombstone(t *testing.T) {
	var evaluationStatusDuringProviderCall string
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
			TTL:                   60,
			LastAppliedValuesJSON: `["192.0.2.10"]`,
			LastEvaluationStatus:  "APPLIED",
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
	provider.afterApply = func(input dns.ApplyRecordInput) {
		if len(input.Values) != 0 {
			return
		}
		payload, err := control.EvaluateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
		if err != nil {
			t.Fatalf("concurrent evaluate while delete pending: %v", err)
		}
		evaluationStatusDuringProviderCall = payload.LastEvaluationStatus
	}

	err = control.DeleteDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if err != nil {
		t.Fatalf("delete managed record: %v", err)
	}
	if evaluationStatusDuringProviderCall != "DELETE_PENDING" {
		t.Fatalf("concurrent evaluation should observe delete pending without re-applying provider, got %q", evaluationStatusDuringProviderCall)
	}
	if len(provider.actions) != 1 {
		t.Fatalf("delete should not allow concurrent evaluation to re-apply provider values, got actions %#v", provider.actions)
	}
}

func TestMarkDNSManagedRecordPendingDoesNotDowngradeDeletePending(t *testing.T) {
	store := &healthDNSTestStore{
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID:                   "record_1",
			OrganizationID:       "org_1",
			RecordType:           "A",
			LastEvaluationStatus: "DELETE_PENDING",
			UpdatedAt:            "2026-01-01T00:00:00Z",
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	err := store.WithinTx(context.Background(), func(ctx context.Context, repositories repo.Repositories) error {
		_, err := control.markDNSManagedRecordPending(ctx, repositories, "org_1", "record_1", "2026-01-01T00:00:01Z", "NODE_DNS_INPUT_CHANGED")
		return err
	})
	if err != nil {
		t.Fatalf("mark managed record pending: %v", err)
	}
	if store.managedRecords[0].LastEvaluationStatus != "DELETE_PENDING" || store.managedRecords[0].UpdatedAt != "2026-01-01T00:00:00Z" {
		t.Fatalf("delete pending record should not be downgraded by invalidation, got %#v", store.managedRecords[0])
	}
}

func TestDeleteDNSManagedRecordClearsRetirementsBeforeCurrentProviderRecord(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID:                    "record_1",
			OrganizationID:        "org_1",
			DNSCredentialID:       "credential_1",
			ZoneID:                "zone_1",
			RecordName:            "current.example.com",
			RecordType:            "A",
			TTL:                   60,
			LastAppliedValuesJSON: `["192.0.2.10"]`,
			ProviderRetirementsJSON: `[{
				"provider":"CLOUDFLARE",
				"encrypted_secret":"",
				"zone":"zone_1",
				"record_name":"old.example.com",
				"record_type":"A",
				"ttl":60,
				"proxied":false,
				"created_at":"2026-06-19T00:00:00Z"
			}]`,
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
	store.managedRecords[0].ProviderRetirementsJSON = strings.ReplaceAll(store.managedRecords[0].ProviderRetirementsJSON, `"encrypted_secret":""`, `"encrypted_secret":"`+encrypted+`"`)

	err = control.DeleteDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if err != nil {
		t.Fatalf("delete managed record: %v", err)
	}
	if len(provider.actions) != 2 {
		t.Fatalf("expected pending retirement and current delete actions, got %#v", provider.actions)
	}
	if provider.actions[0].RecordName != "old.example.com" || len(provider.actions[0].Values) != 0 {
		t.Fatalf("first provider action must clear pending retirement, got %#v", provider.actions[0])
	}
	if provider.actions[1].RecordName != "current.example.com" || len(provider.actions[1].Values) != 0 {
		t.Fatalf("second provider action must delete current record, got %#v", provider.actions[1])
	}
	if store.managedRecords[0].ProviderRetirementsJSON != "[]" {
		t.Fatalf("pending retirements should be cleared before local delete, got %s", store.managedRecords[0].ProviderRetirementsJSON)
	}
}

func TestDeleteDNSManagedRecordKeepsRetirementRetryableWhenCleanupFails(t *testing.T) {
	provider := &recordingDNSProvider{errOnDelete: errors.New("provider retirement cleanup failed")}
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID:                    "record_1",
			OrganizationID:        "org_1",
			DNSCredentialID:       "credential_1",
			ZoneID:                "zone_1",
			RecordName:            "current.example.com",
			RecordType:            "A",
			TTL:                   60,
			LastAppliedValuesJSON: `["192.0.2.10"]`,
			ProviderRetirementsJSON: `[{
				"provider":"CLOUDFLARE",
				"encrypted_secret":"",
				"zone":"zone_1",
				"record_name":"old.example.com",
				"record_type":"A",
				"ttl":60,
				"proxied":false,
				"created_at":"2026-06-19T00:00:00Z"
			}]`,
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
	store.managedRecords[0].ProviderRetirementsJSON = strings.ReplaceAll(store.managedRecords[0].ProviderRetirementsJSON, `"encrypted_secret":""`, `"encrypted_secret":"`+encrypted+`"`)

	err = control.DeleteDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if err == nil || !strings.Contains(err.Error(), "provider retirement cleanup failed") {
		t.Fatalf("expected provider retirement cleanup failure, got %v", err)
	}
	if store.deletedDNSManagedRecordID != "" || store.managedRecords[0].DeletedAt != "" {
		t.Fatalf("expected local record to remain visible after cleanup failure, got %#v", store.managedRecords[0])
	}
	if !strings.Contains(store.managedRecords[0].ProviderRetirementsJSON, "old.example.com") {
		t.Fatalf("pending retirement must remain retryable, got %s", store.managedRecords[0].ProviderRetirementsJSON)
	}
}

func TestUpdateDNSManagedRecordAppliesNewIdentityBeforeRetiringOldIdentity(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		credentialZones: []repo.DNSCredentialZoneRecord{{
			ID: "zone_ref_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", ZoneID: "zone_1", ZoneName: "example.com", Status: "ACTIVE",
		}},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", CredentialZoneID: "zone_ref_1", ZoneID: "zone_1", ZoneName: "example.com", RecordHost: "www", RecordName: "www.example.com", RecordType: "A", TTL: 60, LastAppliedValuesJSON: `["192.0.2.10"]`, LastEvaluationStatus: "APPLIED",
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

	payload, err := control.UpdateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1", DNSManagedRecordMutationInput{
		DNSCredentialID:  "credential_1",
		CredentialZoneID: "zone_ref_1",
		RecordHost:       "api",
		RecordType:       "A",
		TTL:              60,
	})
	if err != nil {
		t.Fatalf("update managed record: %v", err)
	}
	if payload.RecordName != "api.example.com" || payload.LastEvaluationStatus != "APPLIED" {
		t.Fatalf("expected new identity to be evaluated immediately, got %#v", payload)
	}
	if len(provider.actions) != 2 {
		t.Fatalf("expected apply-new then delete-old provider actions, got %#v", provider.actions)
	}
	if provider.actions[0].RecordName != "api.example.com" || len(provider.actions[0].Values) != 1 || provider.actions[0].Values[0] != "192.0.2.10" {
		t.Fatalf("first provider action must apply the new identity, got %#v", provider.actions[0])
	}
	if provider.actions[1].RecordName != "www.example.com" || len(provider.actions[1].Values) != 0 {
		t.Fatalf("second provider action must retire the old identity, got %#v", provider.actions[1])
	}
	if store.managedRecords[0].ProviderRetirementsJSON != "[]" {
		t.Fatalf("provider retirements should be cleared after successful cleanup, got %s", store.managedRecords[0].ProviderRetirementsJSON)
	}
}

func TestUpdateDNSManagedRecordDoesNotRetireSameProviderTargetOnCredentialRotation(t *testing.T) {
	provider := &recordingDNSProvider{}
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
		t.Fatalf("rotate managed record credential: %v", err)
	}
	if payload.RecordName != "www.example.com" || payload.LastEvaluationStatus != "APPLIED" {
		t.Fatalf("expected same provider target to remain applied after credential rotation, got %#v", payload)
	}
	if len(provider.actions) != 1 {
		t.Fatalf("credential rotation should re-apply once without deleting the same provider record, got %#v", provider.actions)
	}
	if len(provider.actions[0].Values) != 1 || provider.actions[0].Values[0] != "192.0.2.10" {
		t.Fatalf("credential rotation should only apply current values, got %#v", provider.actions[0])
	}
	if store.managedRecords[0].ProviderRetirementsJSON != "[]" {
		t.Fatalf("credential-only rotation must not enqueue provider retirement for the same record, got %s", store.managedRecords[0].ProviderRetirementsJSON)
	}
}

func TestCreateDNSManagedRecordRejectsTargetWithPendingProviderRetirement(t *testing.T) {
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		credentialZones: []repo.DNSCredentialZoneRecord{{
			ID: "zone_ref_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", ZoneID: "zone_1", ZoneName: "example.com", Status: "ACTIVE",
		}},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", CredentialZoneID: "zone_ref_1", ZoneID: "zone_1", ZoneName: "example.com", RecordHost: "api", RecordName: "api.example.com", RecordType: "A", TTL: 60, LastAppliedValuesJSON: `["192.0.2.20"]`, LastEvaluationStatus: "PENDING",
			ProviderRetirementsJSON: `[{"provider":"CLOUDFLARE","encrypted_secret":"secret","zone":"zone_1","record_name":"www.example.com","record_type":"A","ttl":60,"proxied":false,"created_at":"now"}]`,
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{Authorizer: healthDNSTestAuthorizer{}})

	_, err := control.CreateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), DNSManagedRecordMutationInput{
		DNSCredentialID:  "credential_1",
		CredentialZoneID: "zone_ref_1",
		RecordHost:       "www",
		RecordType:       "A",
		TTL:              60,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected pending provider retirement to reserve old target, got %v", err)
	}
	if len(store.managedRecords) != 1 {
		t.Fatalf("conflicting managed record must not be created, got %#v", store.managedRecords)
	}
}

func TestUpdateDNSManagedRecordKeepsProviderRetirementRetryableWhenCleanupFails(t *testing.T) {
	provider := &recordingDNSProvider{errOnDelete: errors.New("provider cleanup failed")}
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		credentialZones: []repo.DNSCredentialZoneRecord{{
			ID: "zone_ref_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", ZoneID: "zone_1", ZoneName: "example.com", Status: "ACTIVE",
		}},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", CredentialZoneID: "zone_ref_1", ZoneID: "zone_1", ZoneName: "example.com", RecordHost: "www", RecordName: "www.example.com", RecordType: "A", TTL: 60, LastAppliedValuesJSON: `["192.0.2.10"]`, LastEvaluationStatus: "APPLIED",
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

	payload, err := control.UpdateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1", DNSManagedRecordMutationInput{
		DNSCredentialID:  "credential_1",
		CredentialZoneID: "zone_ref_1",
		RecordHost:       "api",
		RecordType:       "A",
		TTL:              60,
	})
	if err != nil {
		t.Fatalf("update should keep new identity durable while marking cleanup failure: %v", err)
	}
	if store.managedRecords[0].RecordName != "api.example.com" {
		t.Fatalf("identity update should be durable before cleanup retry, got %#v", store.managedRecords[0])
	}
	if payload.LastEvaluationStatus != "FAILED" || !strings.Contains(payload.LastEvaluationError, "provider cleanup failed") {
		t.Fatalf("cleanup failure must be visible in evaluation status, got %#v", payload)
	}
	if !strings.Contains(store.managedRecords[0].ProviderRetirementsJSON, "www.example.com") {
		t.Fatalf("old provider identity must remain retryable, got %s", store.managedRecords[0].ProviderRetirementsJSON)
	}
	if len(provider.actions) != 2 || provider.actions[0].RecordName != "api.example.com" || provider.actions[1].RecordName != "www.example.com" || len(provider.actions[1].Values) != 0 {
		t.Fatalf("expected new apply before old cleanup attempt, got %#v", provider.actions)
	}

	provider.err = nil
	provider.errOnDelete = nil
	_, err = control.EvaluateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if err != nil {
		t.Fatalf("evaluate should retry pending provider retirement: %v", err)
	}
	if store.managedRecords[0].ProviderRetirementsJSON != "[]" {
		t.Fatalf("provider retirement should be cleared after retry succeeds, got %s", store.managedRecords[0].ProviderRetirementsJSON)
	}
}

func TestEvaluateDNSManagedRecordCleansRetirementsWhenNoInstanceMatches(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID:                    "record_1",
			OrganizationID:        "org_1",
			DNSCredentialID:       "credential_1",
			ZoneID:                "zone_1",
			RecordName:            "api.example.com",
			RecordType:            "A",
			TTL:                   60,
			LastAppliedValuesJSON: `[]`,
			LastEvaluationStatus:  "PENDING",
			ProviderRetirementsJSON: `[{
				"provider":"CLOUDFLARE",
				"encrypted_secret":"",
				"zone":"zone_1",
				"record_name":"old.example.com",
				"record_type":"A",
				"ttl":60,
				"proxied":false,
				"created_at":"2026-06-19T00:00:00Z"
			}]`,
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
	store.managedRecords[0].ProviderRetirementsJSON = strings.ReplaceAll(store.managedRecords[0].ProviderRetirementsJSON, `"encrypted_secret":""`, `"encrypted_secret":"`+encrypted+`"`)

	payload, err := control.EvaluateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1")
	if err != nil {
		t.Fatalf("evaluate managed record: %v", err)
	}
	if payload.LastEvaluationStatus != "NO_MATCH" {
		t.Fatalf("expected no-match evaluation, got %#v", payload)
	}
	if len(provider.actions) != 1 || provider.actions[0].RecordName != "old.example.com" || len(provider.actions[0].Values) != 0 {
		t.Fatalf("no-match evaluation should retire old provider record, got %#v", provider.actions)
	}
	if store.managedRecords[0].ProviderRetirementsJSON != "[]" {
		t.Fatalf("provider retirement should be cleared after no-match cleanup, got %s", store.managedRecords[0].ProviderRetirementsJSON)
	}
}

func TestEvaluateDNSManagedRecordSkipsStaleProviderResultAfterConcurrentRecordEdit(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{
		credential: repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID:                    "record_1",
			OrganizationID:        "org_1",
			DNSCredentialID:       "credential_1",
			ZoneID:                "zone_1",
			RecordHost:            "app",
			RecordName:            "app.example.com",
			RecordType:            "A",
			TTL:                   60,
			LastAppliedValuesJSON: `[]`,
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
	provider.afterApply = func(input dns.ApplyRecordInput) {
		if input.RecordName != "app.example.com" {
			return
		}
		store.managedRecords[0].RecordHost = "api"
		store.managedRecords[0].RecordName = "api.example.com"
		store.managedRecords[0].LastEvaluationStatus = "PENDING"
		store.managedRecords[0].LastAppliedValuesJSON = `[]`
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
	if payload.RecordName != "api.example.com" {
		t.Fatalf("expected payload to reflect concurrent edit, got %#v", payload)
	}
	if payload.LastEvaluationStatus == "APPLIED" || store.managedRecords[0].LastAppliedValuesJSON != `[]` {
		t.Fatalf("stale provider result must not be persisted onto edited record, payload=%#v stored=%#v", payload, store.managedRecords[0])
	}
	if !strings.Contains(store.managedRecords[0].LastDiagnosticsJSON, "STALE_EVALUATION_SKIPPED") {
		t.Fatalf("expected stale evaluation diagnostic, got %s", store.managedRecords[0].LastDiagnosticsJSON)
	}
}

func TestEvaluateDNSManagedRecordSkipsStaleProviderResultAfterNodeInputInvalidation(t *testing.T) {
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
			ID:                   "record_1",
			OrganizationID:       "org_1",
			DNSCredentialID:      "credential_1",
			ZoneID:               "zone_1",
			RecordHost:           "app",
			RecordName:           "app.example.com",
			RecordType:           "A",
			TTL:                  60,
			LastEvaluationStatus: "PENDING",
			UpdatedAt:            "2026-01-01T00:00:00Z",
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
	provider.afterApply = func(input dns.ApplyRecordInput) {
		if input.RecordName != "app.example.com" {
			return
		}
		store.managedRecords[0].LastEvaluationStatus = "PENDING"
		store.managedRecords[0].UpdatedAt = "2026-01-01T00:00:01Z"
		store.managedRecords[0].LastDiagnosticsJSON = diagnosticsJSON([]DNSDiagnosticPayload{{Code: "NODE_DNS_INPUT_CHANGED", Message: "DNS policy changed; re-evaluation is required."}})
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
	if payload.LastEvaluationStatus != "PENDING" {
		t.Fatalf("stale node-input evaluation should leave record pending, got %#v", payload)
	}
	if store.managedRecords[0].LastAppliedValuesJSON != "" && store.managedRecords[0].LastAppliedValuesJSON != "[]" {
		t.Fatalf("stale provider result should not be saved as applied values, got %#v", store.managedRecords[0])
	}
	if !strings.Contains(store.managedRecords[0].LastDiagnosticsJSON, "STALE_EVALUATION_SKIPPED") {
		t.Fatalf("expected stale evaluation diagnostic, got %s", store.managedRecords[0].LastDiagnosticsJSON)
	}
}

func TestUpdateDNSManagedRecordIgnoresPostCommitEvaluationError(t *testing.T) {
	provider := &recordingDNSProvider{}
	store := &healthDNSTestStore{
		notificationLookupErr: errors.New("notification lookup unavailable"),
		credential:            repo.DNSCredentialRecord{ID: "credential_1", OrganizationID: "org_1", Provider: "CLOUDFLARE"},
		credentialZones: []repo.DNSCredentialZoneRecord{{
			ID: "zone_ref_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", ZoneID: "zone_1", ZoneName: "example.com", Status: "ACTIVE",
		}},
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", DNSCredentialID: "credential_1", CredentialZoneID: "zone_ref_1", ZoneID: "zone_1", ZoneName: "example.com", RecordHost: "www", RecordName: "www.example.com", RecordType: "A", TTL: 60, LastAppliedValuesJSON: `["192.0.2.10"]`, LastEvaluationStatus: "APPLIED",
			Instances: []repo.DNSInstanceRecord{{
				ID:                         "instance_1",
				OrganizationID:             "org_1",
				ManagedRecordID:            "record_1",
				Priority:                   10,
				Enabled:                    true,
				ConditionJSON:              `{}`,
				ActionJSON:                 `{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]}`,
				NotificationChannelIDsJSON: `["channel_1"]`,
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

	payload, err := control.UpdateDNSManagedRecord(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "record_1", DNSManagedRecordMutationInput{
		DNSCredentialID:  "credential_1",
		CredentialZoneID: "zone_ref_1",
		RecordHost:       "api",
		RecordType:       "A",
		TTL:              60,
	})
	if err != nil {
		t.Fatalf("update should not fail after the local record mutation commits: %v", err)
	}
	if payload.RecordName != "api.example.com" || store.managedRecords[0].RecordName != "api.example.com" {
		t.Fatalf("record identity update should be durable, payload=%#v stored=%#v", payload, store.managedRecords[0])
	}
	if len(provider.actions) != 0 {
		t.Fatalf("post-commit evaluation failed before provider apply; provider should not run, got %#v", provider.actions)
	}
}

func TestUpdateDNSInstanceAllowsRepairingStaleReferencedAction(t *testing.T) {
	store := &healthDNSTestStore{
		managedRecords: []repo.DNSManagedRecordRecord{{
			ID: "record_1", OrganizationID: "org_1", RecordType: "A",
			Instances: []repo.DNSInstanceRecord{{
				ID:              "instance_1",
				OrganizationID:  "org_1",
				ManagedRecordID: "record_1",
				Name:            "stale reference",
				Priority:        10,
				Enabled:         true,
				ConditionJSON:   `{}`,
				ActionJSON:      `{"type":"USE_INSTANCE_OUTPUT","instance_id":"missing_instance"}`,
			}},
		}},
	}
	control := NewControlServiceWithOptions(store, ControlServiceOptions{
		Authorizer: healthDNSTestAuthorizer{},
	})

	payload, err := control.UpdateDNSInstance(context.Background(), healthDNSTestIdentity(string(domain.PermissionDNSManage)), "instance_1", DNSInstanceMutationInput{
		ManagedRecordID: "record_1",
		Name:            "static repair",
		Priority:        10,
		Enabled:         true,
		AnswerCount:     -1,
		Condition:       map[string]any{},
		Action:          map[string]any{"type": "SET_STATIC_ADDRESSES", "values": []any{"192.0.2.10"}},
	})
	if err != nil {
		t.Fatalf("stale reference should be repairable: %v", err)
	}
	if payload.Name != "static repair" || !strings.Contains(store.managedRecords[0].Instances[0].ActionJSON, "SET_STATIC_ADDRESSES") {
		t.Fatalf("expected stale action to be replaced, payload=%#v stored=%#v", payload, store.managedRecords[0].Instances[0])
	}
}
