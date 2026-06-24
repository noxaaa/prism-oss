package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

const dnsBestEffortEvaluationTimeout = 30 * time.Second

func (service *ControlService) ListDNSManagedRecords(ctx context.Context, identity InternalIdentity) ([]DNSManagedRecordPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionDNSRead)) && !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return nil, ErrForbidden
	}
	var result []DNSManagedRecordPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		records, err := repositories.DNSRecords().ListDNSManagedRecordsByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		records = service.filterDNSManagedRecordsForScope(ctx, repositories, identity, records)
		result = toDNSManagedRecordPayloads(records)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) CreateDNSManagedRecord(ctx context.Context, identity InternalIdentity, input DNSManagedRecordMutationInput) (DNSManagedRecordPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return DNSManagedRecordPayload{}, ErrForbidden
	}
	var result DNSManagedRecordPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		record, err := service.buildDNSManagedRecord(ctx, repositories, identity.OrganizationID, repo.DNSManagedRecordRecord{
			ID:             service.newID(),
			OrganizationID: identity.OrganizationID,
		}, input)
		if err != nil {
			return err
		}
		now := service.timestamp()
		record.CreatedAt = now
		record.UpdatedAt = now
		record.LastEvaluationStatus = "PENDING"
		record.LastAppliedValuesJSON = "[]"
		record.LastDiagnosticsJSON = "[]"
		if err := repositories.DNSRecords().CreateDNSManagedRecord(ctx, record); err != nil {
			if errors.Is(err, repo.ErrConflict) {
				return ErrInvalidInput
			}
			return err
		}
		result = toDNSManagedRecordPayload(record)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "dns_managed_records.create", "DNS_MANAGED_RECORD", record.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) UpdateDNSManagedRecord(ctx context.Context, identity InternalIdentity, recordID string, input DNSManagedRecordMutationInput) (DNSManagedRecordPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return DNSManagedRecordPayload{}, ErrForbidden
	}
	var result DNSManagedRecordPayload
	needsImmediateEvaluation := false
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		record, err := repositories.DNSRecords().FindDNSManagedRecordByID(ctx, identity.OrganizationID, recordID)
		if err != nil {
			return err
		}
		recordsToLock := map[string]bool{record.ID: true}
		seenReferences := map[string]bool{}
		for _, instance := range record.Instances {
			if instance.DeletedAt != "" {
				continue
			}
			referenced, err := service.collectDNSRecordsReferencingInstance(ctx, repositories, identity.OrganizationID, instance.ID, seenReferences, 0)
			if err != nil {
				return err
			}
			for referencedRecordID := range referenced {
				recordsToLock[referencedRecordID] = true
			}
		}
		if err := service.lockDNSManagedRecords(ctx, repositories, identity.OrganizationID, sortedStringSetKeys(recordsToLock)...); err != nil {
			return err
		}
		record, err = repositories.DNSRecords().FindDNSManagedRecordByID(ctx, identity.OrganizationID, recordID)
		if err != nil {
			return err
		}
		if err := service.ensureCanManageDNSManagedRecord(ctx, repositories, identity, record); err != nil {
			return err
		}
		previous := record
		record, err = service.buildDNSManagedRecord(ctx, repositories, identity.OrganizationID, record, input)
		if err != nil {
			return err
		}
		if previous.RecordType != record.RecordType {
			if err := service.validateExistingDNSInstancesForManagedRecord(ctx, repositories, identity.OrganizationID, record); err != nil {
				return err
			}
		}
		providerIdentityChanged := dnsManagedRecordProviderIdentityChanged(previous, record)
		providerTargetChanged := dnsManagedRecordProviderTargetChanged(previous, record)
		providerSettingsChanged := dnsManagedRecordProviderSettingsChanged(previous, record)
		needsImmediateEvaluation = providerIdentityChanged
		if providerIdentityChanged || providerSettingsChanged {
			if providerTargetChanged {
				record.LastAppliedValuesJSON = "[]"
				record.LastAppliedAt = ""
			}
			record.LastEvaluationStatus = "PENDING"
			record.LastEvaluationError = ""
			record.LastDiagnosticsJSON = "[]"
		}
		now := service.timestamp()
		if providerTargetChanged && len(parseStringListJSON(previous.LastAppliedValuesJSON)) > 0 {
			credential, err := repositories.DNSCredentials().FindDNSCredentialByID(ctx, identity.OrganizationID, previous.DNSCredentialID)
			if err != nil {
				return err
			}
			record.ProviderRetirementsJSON = appendDNSProviderRetirement(record.ProviderRetirementsJSON, buildDNSProviderRetirement(previous, credential, now))
		}
		if previous.RecordType != record.RecordType {
			for _, instance := range previous.Instances {
				if instance.DeletedAt != "" {
					continue
				}
				if _, err := service.markDNSRecordsReferencingInstancePending(ctx, repositories, identity.OrganizationID, instance.ID, now); err != nil {
					return err
				}
			}
		}
		record.UpdatedAt = now
		if err := repositories.DNSRecords().UpdateDNSManagedRecord(ctx, record); err != nil {
			if errors.Is(err, repo.ErrConflict) {
				return ErrInvalidInput
			}
			return err
		}
		record, err = repositories.DNSRecords().FindDNSManagedRecordByID(ctx, identity.OrganizationID, record.ID)
		if err != nil {
			return err
		}
		result = toDNSManagedRecordPayload(record)
		if err := service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "dns_managed_records.update", "DNS_MANAGED_RECORD", record.ID, "")); err != nil {
			return err
		}
		return nil
	})
	if err == nil && needsImmediateEvaluation {
		evaluationCtx, cancel := context.WithTimeout(context.Background(), dnsBestEffortEvaluationTimeout)
		defer cancel()
		if evaluated, evaluationErr := service.evaluateDNSManagedRecordForOrganization(evaluationCtx, identity.OrganizationID, recordID, nil); evaluationErr == nil {
			result = evaluated
		}
	}
	return result, mapServiceError(err)
}

func (service *ControlService) DeleteDNSManagedRecord(ctx context.Context, identity InternalIdentity, recordID string) error {
	if !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return ErrForbidden
	}
	var deleteSnapshot dnsManagedRecordEvaluationSnapshot
	var currentDeleteAction *dnsProviderApplyAction
	var retirementActions []dnsProviderApplyAction
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		record, err := repositories.DNSRecords().FindDNSManagedRecordByID(ctx, identity.OrganizationID, recordID)
		if err != nil {
			return err
		}
		recordsToLock := map[string]bool{record.ID: true}
		seenReferences := map[string]bool{}
		for _, instance := range record.Instances {
			if instance.DeletedAt != "" {
				continue
			}
			referenced, err := service.collectDNSRecordsReferencingInstance(ctx, repositories, identity.OrganizationID, instance.ID, seenReferences, 0)
			if err != nil {
				return err
			}
			for referencedRecordID := range referenced {
				recordsToLock[referencedRecordID] = true
			}
		}
		if err := service.lockDNSManagedRecords(ctx, repositories, identity.OrganizationID, sortedStringSetKeys(recordsToLock)...); err != nil {
			return err
		}
		record, err = repositories.DNSRecords().FindDNSManagedRecordByID(ctx, identity.OrganizationID, recordID)
		if err != nil {
			return err
		}
		if err := service.ensureCanManageDNSManagedRecord(ctx, repositories, identity, record); err != nil {
			return err
		}
		deleteSnapshot = newDNSManagedRecordEvaluationSnapshot(record)
		retirementActions = dnsProviderRetirementActions(record.ProviderRetirementsJSON)
		if len(parseStringListJSON(record.LastAppliedValuesJSON)) > 0 {
			credential, err := repositories.DNSCredentials().FindDNSCredentialByID(ctx, identity.OrganizationID, record.DNSCredentialID)
			if err != nil {
				return err
			}
			currentDeleteAction = &dnsProviderApplyAction{
				Provider:        credential.Provider,
				EncryptedSecret: credential.EncryptedSecret,
				Zone:            record.ZoneID,
				RecordName:      record.RecordName,
				RecordType:      record.RecordType,
				Values:          nil,
				TTL:             record.TTL,
				Proxied:         record.Proxied,
			}
		}
		now := service.timestamp()
		record.LastEvaluationStatus = "DELETE_PENDING"
		record.LastEvaluationError = ""
		record.LastDiagnosticsJSON = diagnosticsJSON([]DNSDiagnosticPayload{{
			Code:    "DELETE_PENDING",
			Message: "DNS provider cleanup is pending before the managed record is deleted locally.",
		}})
		record.LastEvaluatedAt = now
		record.UpdatedAt = now
		deleteSnapshot = newDNSManagedRecordEvaluationSnapshot(record)
		if err := repositories.DNSRecords().UpdateDNSManagedRecordEvaluation(ctx, record); err != nil {
			return err
		}
		for _, instance := range record.Instances {
			if instance.DeletedAt != "" {
				continue
			}
			if _, err := service.markDNSRecordsReferencingInstancePending(ctx, repositories, identity.OrganizationID, instance.ID, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return mapServiceError(err)
	}
	if err := service.executeDNSProviderRetirementActions(ctx, retirementActions); err != nil {
		return mapServiceError(err)
	}
	if currentDeleteAction != nil {
		if err := service.executeDNSProviderAction(ctx, *currentDeleteAction); err != nil {
			return mapServiceError(err)
		}
	}
	err = service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if err := repositories.DNSRecords().LockDNSManagedRecordEvaluation(ctx, identity.OrganizationID, recordID); err != nil {
			return err
		}
		record, err := repositories.DNSRecords().FindDNSManagedRecordByID(ctx, identity.OrganizationID, recordID)
		if err != nil {
			return err
		}
		if !deleteSnapshot.matches(record) {
			record.LastEvaluationStatus = "PENDING"
			record.LastEvaluationError = ""
			record.LastDiagnosticsJSON = diagnosticsJSON([]DNSDiagnosticPayload{{
				Code:    "STALE_DELETE_SKIPPED",
				Message: "DNS record changed while provider cleanup was running; delete must be retried.",
			}})
			record.LastEvaluatedAt = service.timestamp()
			record.UpdatedAt = record.LastEvaluatedAt
			return repositories.DNSRecords().UpdateDNSManagedRecordEvaluation(ctx, record)
		}
		now := service.timestamp()
		record.ProviderRetirementsJSON = "[]"
		record.LastAppliedValuesJSON = "[]"
		record.UpdatedAt = now
		if err := repositories.DNSRecords().UpdateDNSManagedRecordEvaluation(ctx, record); err != nil {
			return err
		}
		if err := repositories.DNSRecords().DeleteDNSManagedRecord(ctx, identity.OrganizationID, record.ID, now); err != nil {
			return err
		}
		if err := service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "dns_managed_records.delete", "DNS_MANAGED_RECORD", record.ID, "")); err != nil {
			return err
		}
		return nil
	})
	return mapServiceError(err)
}

func (service *ControlService) buildDNSManagedRecord(ctx context.Context, repositories repo.Repositories, organizationID string, record repo.DNSManagedRecordRecord, input DNSManagedRecordMutationInput) (repo.DNSManagedRecordRecord, error) {
	credential, err := repositories.DNSCredentials().FindDNSCredentialByID(ctx, organizationID, input.DNSCredentialID)
	if err != nil {
		return repo.DNSManagedRecordRecord{}, err
	}
	zone, err := repositories.DNSCredentials().FindDNSCredentialZoneByID(ctx, organizationID, input.CredentialZoneID)
	if err != nil {
		return repo.DNSManagedRecordRecord{}, err
	}
	preservingCurrentZone := record.ID != "" && record.DNSCredentialID == credential.ID && record.CredentialZoneID == zone.ID
	if zone.DNSCredentialID != credential.ID || (!dnsCredentialZoneWritable(zone.Status) && !preservingCurrentZone) {
		return repo.DNSManagedRecordRecord{}, ErrInvalidInput
	}
	recordType := strings.ToUpper(strings.TrimSpace(input.RecordType))
	if recordType != "A" && recordType != "AAAA" && recordType != "CNAME" {
		return repo.DNSManagedRecordRecord{}, ErrInvalidInput
	}
	recordHost, recordName, err := composeDNSManagedRecordName(input.RecordHost, input.RecordName, zone.ZoneName)
	if err != nil {
		return repo.DNSManagedRecordRecord{}, err
	}
	if err := service.ensureDNSManagedRecordTypeCompatible(ctx, repositories, organizationID, record.ID, zone.ZoneID, recordName, recordType); err != nil {
		return repo.DNSManagedRecordRecord{}, err
	}
	ttl := input.TTL
	if ttl <= 0 {
		ttl = 60
	}
	record.DNSCredentialID = credential.ID
	record.CredentialZoneID = zone.ID
	record.ZoneID = zone.ZoneID
	record.ZoneName = zone.ZoneName
	record.RecordHost = recordHost
	record.RecordName = recordName
	record.RecordType = recordType
	record.TTL = ttl
	record.Proxied = input.Proxied
	return record, nil
}

func (service *ControlService) ensureDNSManagedRecordTypeCompatible(ctx context.Context, repositories repo.Repositories, organizationID string, currentRecordID string, zoneID string, recordName string, recordType string) error {
	records, err := repositories.DNSRecords().ListDNSManagedRecordsByOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	for _, existing := range records {
		if dnsProviderRetirementTargetConflicts(existing.ProviderRetirementsJSON, zoneID, recordName, recordType) {
			return ErrInvalidInput
		}
		if existing.ID == currentRecordID {
			continue
		}
		if dnsProviderTargetConflicts(existing, zoneID, recordName, recordType) {
			return ErrInvalidInput
		}
	}
	return nil
}

func (service *ControlService) ListDNSInstances(ctx context.Context, identity InternalIdentity) ([]DNSInstancePayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionDNSRead)) && !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return nil, ErrForbidden
	}
	var result []DNSInstancePayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		instances, err := repositories.DNSRecords().ListDNSInstancesByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		instances = service.filterDNSInstancesForScope(ctx, repositories, identity, instances)
		result = toDNSInstancePayloads(instances)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) CreateDNSInstance(ctx context.Context, identity InternalIdentity, input DNSInstanceMutationInput) (DNSInstancePayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return DNSInstancePayload{}, ErrForbidden
	}
	var result DNSInstancePayload
	affectedDNSRecordIDs := map[string]bool{}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if err := repositories.DNSRecords().LockDNSManagedRecordEvaluation(ctx, identity.OrganizationID, input.ManagedRecordID); err != nil {
			return err
		}
		record, err := repositories.DNSRecords().FindDNSManagedRecordByID(ctx, identity.OrganizationID, input.ManagedRecordID)
		if err != nil {
			return err
		}
		if err := service.ensureCanManageDNSManagedRecord(ctx, repositories, identity, record); err != nil {
			return err
		}
		instance, err := service.buildDNSInstance(ctx, repositories, identity, repo.DNSInstanceRecord{
			ID:             service.newID(),
			OrganizationID: identity.OrganizationID,
		}, input)
		if err != nil {
			return err
		}
		now := service.timestamp()
		instance.CreatedAt = now
		instance.UpdatedAt = now
		instance.LastStatus = "PENDING"
		instance.LastOutputValuesJSON = "[]"
		instance.LastDiagnosticsJSON = "[]"
		if err := repositories.DNSRecords().CreateDNSInstance(ctx, instance); err != nil {
			return err
		}
		if instance.Enabled {
			marked, err := service.markDNSManagedRecordPending(ctx, repositories, identity.OrganizationID, instance.ManagedRecordID, now, "DNS_INSTANCE_CHANGED")
			if err != nil {
				return err
			}
			if marked {
				affectedDNSRecordIDs[instance.ManagedRecordID] = true
			}
		}
		result = toDNSInstancePayload(instance)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "dns_instances.create", "DNS_INSTANCE", instance.ID, ""))
	})
	if err == nil {
		service.evaluateDNSManagedRecordsBestEffort(ctx, identity.OrganizationID, sortedStringSetKeys(affectedDNSRecordIDs))
		if refreshed, refreshErr := service.findDNSInstancePayload(ctx, identity.OrganizationID, result.ID); refreshErr == nil {
			result = refreshed
		}
	}
	return result, mapServiceError(err)
}

func (service *ControlService) findDNSInstancePayload(ctx context.Context, organizationID string, instanceID string) (DNSInstancePayload, error) {
	var result DNSInstancePayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		instance, err := repositories.DNSRecords().FindDNSInstanceByID(ctx, organizationID, instanceID)
		if err != nil {
			return err
		}
		result = toDNSInstancePayload(instance)
		return nil
	})
	return result, err
}

func (service *ControlService) UpdateDNSInstance(ctx context.Context, identity InternalIdentity, instanceID string, input DNSInstanceMutationInput) (DNSInstancePayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return DNSInstancePayload{}, ErrForbidden
	}
	var result DNSInstancePayload
	affectedDNSRecordIDs := map[string]bool{}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if err := repositories.DNSRecords().LockDNSInstanceMutation(ctx, identity.OrganizationID, instanceID); err != nil {
			return err
		}
		instance, err := repositories.DNSRecords().FindDNSInstanceByID(ctx, identity.OrganizationID, instanceID)
		if err != nil {
			return err
		}
		recordsToLock := map[string]bool{
			instance.ManagedRecordID: true,
			input.ManagedRecordID:    true,
		}
		referenced, err := service.collectDNSRecordsReferencingInstance(ctx, repositories, identity.OrganizationID, instance.ID, map[string]bool{}, 0)
		if err != nil {
			return err
		}
		for referencedRecordID := range referenced {
			recordsToLock[referencedRecordID] = true
		}
		if err := service.lockDNSManagedRecords(ctx, repositories, identity.OrganizationID, sortedStringSetKeys(recordsToLock)...); err != nil {
			return err
		}
		instance, err = repositories.DNSRecords().FindDNSInstanceByID(ctx, identity.OrganizationID, instanceID)
		if err != nil {
			return err
		}
		currentRecord, err := repositories.DNSRecords().FindDNSManagedRecordByID(ctx, identity.OrganizationID, instance.ManagedRecordID)
		if err != nil {
			return err
		}
		if err := service.ensureCanManageDNSManagedRecord(ctx, repositories, identity, currentRecord); err != nil {
			return err
		}
		if input.ManagedRecordID != instance.ManagedRecordID {
			record, err := repositories.DNSRecords().FindDNSManagedRecordByID(ctx, identity.OrganizationID, input.ManagedRecordID)
			if err != nil {
				return err
			}
			if err := service.ensureCanManageDNSManagedRecord(ctx, repositories, identity, record); err != nil {
				return err
			}
		}
		previous := instance
		instance, err = service.buildDNSInstance(ctx, repositories, identity, instance, input)
		if err != nil {
			return err
		}
		instance.UpdatedAt = service.timestamp()
		if err := repositories.DNSRecords().UpdateDNSInstance(ctx, instance); err != nil {
			return err
		}
		evaluationInputsChanged := dnsInstanceEvaluationInputsChanged(previous, instance)
		if previous.Enabled && (previous.ManagedRecordID != instance.ManagedRecordID || !instance.Enabled) {
			if err := repositories.DNSRecords().ClearDNSManagedRecordActiveInstance(ctx, identity.OrganizationID, instance.ID, instance.UpdatedAt); err != nil {
				return err
			}
			if previous.ManagedRecordID != "" {
				marked, err := service.markDNSManagedRecordPending(ctx, repositories, identity.OrganizationID, previous.ManagedRecordID, instance.UpdatedAt, "DNS_INSTANCE_CHANGED")
				if err != nil {
					return err
				}
				if marked {
					affectedDNSRecordIDs[previous.ManagedRecordID] = true
				}
			}
		}
		if instance.Enabled && evaluationInputsChanged {
			code := "DNS_INSTANCE_CHANGED"
			if previous.ManagedRecordID == instance.ManagedRecordID {
				record, err := repositories.DNSRecords().FindDNSManagedRecordByID(ctx, identity.OrganizationID, instance.ManagedRecordID)
				if err != nil {
					return err
				}
				if record.ActiveInstanceID == instance.ID {
					code = "ACTIVE_INSTANCE_CHANGED"
				}
			}
			marked, err := service.markDNSManagedRecordPending(ctx, repositories, identity.OrganizationID, instance.ManagedRecordID, instance.UpdatedAt, code)
			if err != nil {
				return err
			}
			if marked {
				affectedDNSRecordIDs[instance.ManagedRecordID] = true
			}
		}
		if evaluationInputsChanged {
			referencedRecordIDs, err := service.markDNSRecordsReferencingInstancePending(ctx, repositories, identity.OrganizationID, instance.ID, instance.UpdatedAt)
			if err != nil {
				return err
			}
			for _, recordID := range referencedRecordIDs {
				affectedDNSRecordIDs[recordID] = true
			}
		}
		result = toDNSInstancePayload(instance)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "dns_instances.update", "DNS_INSTANCE", instance.ID, ""))
	})
	if err == nil {
		service.evaluateDNSManagedRecordsBestEffort(ctx, identity.OrganizationID, sortedStringSetKeys(affectedDNSRecordIDs))
		if refreshed, refreshErr := service.findDNSInstancePayload(ctx, identity.OrganizationID, result.ID); refreshErr == nil {
			result = refreshed
		}
	}
	return result, mapServiceError(err)
}

func dnsInstanceEvaluationInputsChanged(previous repo.DNSInstanceRecord, next repo.DNSInstanceRecord) bool {
	return previous.ManagedRecordID != next.ManagedRecordID ||
		previous.Priority != next.Priority ||
		previous.Enabled != next.Enabled ||
		previous.NodeGroupIDsJSON != next.NodeGroupIDsJSON ||
		previous.AnswerCount != next.AnswerCount ||
		previous.ConditionJSON != next.ConditionJSON ||
		previous.ActionJSON != next.ActionJSON
}

func (service *ControlService) DeleteDNSInstance(ctx context.Context, identity InternalIdentity, instanceID string) error {
	if !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return ErrForbidden
	}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if err := repositories.DNSRecords().LockDNSInstanceMutation(ctx, identity.OrganizationID, instanceID); err != nil {
			return err
		}
		instance, err := repositories.DNSRecords().FindDNSInstanceByID(ctx, identity.OrganizationID, instanceID)
		if err != nil {
			return err
		}
		if err := repositories.DNSRecords().LockDNSManagedRecordEvaluation(ctx, identity.OrganizationID, instance.ManagedRecordID); err != nil {
			return err
		}
		instance, err = repositories.DNSRecords().FindDNSInstanceByID(ctx, identity.OrganizationID, instanceID)
		if err != nil {
			return err
		}
		record, err := repositories.DNSRecords().FindDNSManagedRecordByID(ctx, identity.OrganizationID, instance.ManagedRecordID)
		if err != nil {
			return err
		}
		if err := service.ensureCanManageDNSManagedRecord(ctx, repositories, identity, record); err != nil {
			return err
		}
		if err := service.ensureDNSInstanceUseScope(ctx, repositories, identity, instance, map[string]bool{}, 0, true); err != nil {
			return err
		}
		now := service.timestamp()
		if err := repositories.DNSRecords().DeleteDNSInstance(ctx, identity.OrganizationID, instance.ID, now); err != nil {
			return err
		}
		if err := repositories.DNSRecords().ClearDNSManagedRecordActiveInstance(ctx, identity.OrganizationID, instance.ID, now); err != nil {
			return err
		}
		if _, err := service.markDNSManagedRecordPending(ctx, repositories, identity.OrganizationID, instance.ManagedRecordID, now, "DNS_INSTANCE_CHANGED"); err != nil {
			return err
		}
		if _, err := service.markDNSRecordsReferencingInstancePending(ctx, repositories, identity.OrganizationID, instance.ID, now); err != nil {
			return err
		}
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "dns_instances.delete", "DNS_INSTANCE", instance.ID, ""))
	})
	return mapServiceError(err)
}

func (service *ControlService) buildDNSInstance(ctx context.Context, repositories repo.Repositories, identity InternalIdentity, instance repo.DNSInstanceRecord, input DNSInstanceMutationInput) (repo.DNSInstanceRecord, error) {
	if strings.TrimSpace(input.Name) == "" || len(input.Name) > 120 {
		return repo.DNSInstanceRecord{}, ErrInvalidInput
	}
	if err := service.ensureCanUseNodeGroups(identity, input.NodeGroupIDs); err != nil {
		return repo.DNSInstanceRecord{}, err
	}
	record, err := repositories.DNSRecords().FindDNSManagedRecordByID(ctx, identity.OrganizationID, input.ManagedRecordID)
	if err != nil {
		return repo.DNSInstanceRecord{}, err
	}
	for _, groupID := range input.NodeGroupIDs {
		if _, err := repositories.NodeGroups().FindNodeGroupByID(ctx, identity.OrganizationID, groupID); err != nil {
			return repo.DNSInstanceRecord{}, err
		}
	}
	if input.AnswerCount == 0 {
		input.AnswerCount = -1
	}
	if input.AnswerCount < -1 {
		return repo.DNSInstanceRecord{}, ErrInvalidInput
	}
	conditionJSON, err := jsonObjectString(input.Condition)
	if err != nil {
		return repo.DNSInstanceRecord{}, err
	}
	actionJSON, err := jsonObjectString(input.Action)
	if err != nil {
		return repo.DNSInstanceRecord{}, err
	}
	channelIDsJSON, err := jsonStringList(input.NotificationChannelIDs)
	if err != nil {
		return repo.DNSInstanceRecord{}, err
	}
	for _, channelID := range input.NotificationChannelIDs {
		if _, err := repositories.DNSRecords().FindNotificationChannelByID(ctx, identity.OrganizationID, channelID); err != nil {
			return repo.DNSInstanceRecord{}, err
		}
	}
	if err := service.ensureDNSInstanceActionScope(ctx, repositories, identity, actionJSON, instance.ID); err != nil {
		return repo.DNSInstanceRecord{}, err
	}
	if err := service.validateDNSInstanceAction(ctx, repositories, identity.OrganizationID, record, actionJSON, instance.ID); err != nil {
		return repo.DNSInstanceRecord{}, err
	}
	instance.ManagedRecordID = record.ID
	instance.Name = strings.TrimSpace(input.Name)
	instance.Priority = input.Priority
	if instance.Priority < 0 {
		return repo.DNSInstanceRecord{}, ErrInvalidInput
	}
	instance.Enabled = input.Enabled
	instance.NodeGroupIDsJSON, err = jsonStringList(input.NodeGroupIDs)
	if err != nil {
		return repo.DNSInstanceRecord{}, err
	}
	instance.AnswerCount = input.AnswerCount
	instance.ConditionJSON = conditionJSON
	instance.ActionJSON = actionJSON
	instance.NotificationChannelIDsJSON = channelIDsJSON
	return instance, nil
}

func (service *ControlService) ensureDNSInstanceActionScope(ctx context.Context, repositories repo.Repositories, identity InternalIdentity, rawAction string, currentInstanceID string) error {
	var action dnsPolicyAction
	if err := json.Unmarshal([]byte(rawAction), &action); err != nil {
		return ErrInvalidInput
	}
	if normalizeDNSActionType(action.Type) != "USE_INSTANCE_OUTPUT" {
		return nil
	}
	if strings.TrimSpace(action.InstanceID) == "" || strings.TrimSpace(action.InstanceID) == currentInstanceID {
		return nil
	}
	target, err := repositories.DNSRecords().FindDNSInstanceByID(ctx, identity.OrganizationID, action.InstanceID)
	if err != nil {
		return err
	}
	return service.ensureDNSInstanceUseScope(ctx, repositories, identity, target, map[string]bool{currentInstanceID: true}, 0, false)
}

func (service *ControlService) validateExistingDNSInstancesForManagedRecord(ctx context.Context, repositories repo.Repositories, organizationID string, record repo.DNSManagedRecordRecord) error {
	instances, err := repositories.DNSRecords().ListDNSInstancesByManagedRecord(ctx, organizationID, record.ID)
	if err != nil {
		return err
	}
	for _, instance := range instances {
		if err := service.validateDNSInstanceAction(ctx, repositories, organizationID, record, instance.ActionJSON, instance.ID); err != nil {
			return err
		}
	}
	return nil
}

func (service *ControlService) validateDNSInstanceAction(ctx context.Context, repositories repo.Repositories, organizationID string, record repo.DNSManagedRecordRecord, rawAction string, currentInstanceID string) error {
	var action dnsPolicyAction
	if err := json.Unmarshal([]byte(rawAction), &action); err != nil {
		return ErrInvalidInput
	}
	actionType := normalizeDNSActionType(action.Type)
	switch actionType {
	case "ROTATE_ONLINE_NODES":
		if record.RecordType == "CNAME" {
			return ErrInvalidInput
		}
	case "SET_STATIC_A":
		values := normalizeDNSValues(action.Values)
		if record.RecordType != "A" || len(values) == 0 || !dnsValuesMatchRecordType(values, record.RecordType) {
			return ErrInvalidInput
		}
	case "SET_STATIC_AAAA":
		values := normalizeDNSValues(action.Values)
		if record.RecordType != "AAAA" || len(values) == 0 || !dnsValuesMatchRecordType(values, record.RecordType) {
			return ErrInvalidInput
		}
	case "SET_STATIC_ADDRESSES":
		values := normalizeDNSValues(action.Values)
		if record.RecordType == "CNAME" || len(values) == 0 || !dnsValuesMatchRecordType(values, record.RecordType) {
			return ErrInvalidInput
		}
	case "SET_STATIC_CNAME":
		if record.RecordType != "CNAME" || strings.TrimSpace(action.Value) == "" {
			return ErrInvalidInput
		}
	case "USE_INSTANCE_OUTPUT":
		if strings.TrimSpace(action.InstanceID) == "" || strings.TrimSpace(action.InstanceID) == currentInstanceID {
			return ErrInvalidInput
		}
		target, err := repositories.DNSRecords().FindDNSInstanceByID(ctx, organizationID, action.InstanceID)
		if err != nil {
			return err
		}
		if !target.Enabled {
			return ErrInvalidInput
		}
		if target.ManagedRecordID == record.ID {
			return ErrInvalidInput
		}
		targetRecord, err := repositories.DNSRecords().FindDNSManagedRecordByID(ctx, organizationID, target.ManagedRecordID)
		if err != nil {
			return err
		}
		if targetRecord.RecordType != record.RecordType {
			return ErrInvalidInput
		}
	default:
		return ErrInvalidInput
	}
	return nil
}

func (service *ControlService) EvaluateDNSManagedRecord(ctx context.Context, identity InternalIdentity, recordID string) (DNSManagedRecordPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return DNSManagedRecordPayload{}, ErrForbidden
	}
	return service.evaluateDNSManagedRecordForOrganization(ctx, identity.OrganizationID, recordID, &identity)
}

func (service *ControlService) evaluateDNSManagedRecordForOrganization(ctx context.Context, organizationID string, recordID string, identity *InternalIdentity) (DNSManagedRecordPayload, error) {
	var result DNSManagedRecordPayload
	var notifications []preparedDNSNotification
	var providerAction *dnsProviderApplyAction
	var providerApplied bool
	var retirementActions []dnsProviderApplyAction
	var retirementCleaned bool
	var evaluation dnsPolicyEvaluation
	var evaluationSnapshot dnsManagedRecordEvaluationSnapshot
	var previous dnsPolicyPreviousState
	var notificationChannels []repo.NotificationChannelRecord
	skipEvaluation := false

	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if err := repositories.DNSRecords().LockDNSManagedRecordEvaluation(ctx, organizationID, recordID); err != nil {
			return err
		}
		record, err := repositories.DNSRecords().FindDNSManagedRecordByID(ctx, organizationID, recordID)
		if err != nil {
			return err
		}
		if identity != nil {
			if err := service.ensureCanEvaluateDNSManagedRecord(ctx, repositories, *identity, record); err != nil {
				return err
			}
		}
		if strings.ToUpper(strings.TrimSpace(record.LastEvaluationStatus)) == "DELETE_PENDING" {
			result, err = service.dnsManagedRecordPayloadForIdentity(ctx, repositories, identity, record)
			skipEvaluation = true
			return err
		}
		evaluationSnapshot = newDNSManagedRecordEvaluationSnapshot(record)
		previous = dnsPolicyPreviousState{
			ActiveInstanceID: record.ActiveInstanceID,
			Status:           record.LastEvaluationStatus,
			ValuesJSON:       stringListJSON(parseStringListJSON(record.LastAppliedValuesJSON)),
		}
		evaluation, err = service.evaluateDNSManagedRecord(ctx, repositories, organizationID, record)
		if err != nil {
			record.LastEvaluationStatus = "FAILED"
			record.LastEvaluationError = err.Error()
			record.LastDiagnosticsJSON = diagnosticsJSON([]DNSDiagnosticPayload{{Code: "EVALUATION_FAILED", Message: "DNS evaluation failed."}})
			record.LastEvaluatedAt = service.timestamp()
			record.UpdatedAt = record.LastEvaluatedAt
			_ = repositories.DNSRecords().UpdateDNSManagedRecordEvaluation(ctx, record)
			return err
		}
		notificationChannels, err = service.resolveDNSNotificationChannels(ctx, repositories, organizationID, evaluation)
		if err != nil {
			return err
		}
		if evaluation.ApplyProvider {
			credential, err := repositories.DNSCredentials().FindDNSCredentialByID(ctx, organizationID, record.DNSCredentialID)
			if err != nil {
				return err
			}
			providerAction = &dnsProviderApplyAction{
				Provider:        credential.Provider,
				EncryptedSecret: credential.EncryptedSecret,
				Zone:            record.ZoneID,
				RecordName:      record.RecordName,
				RecordType:      record.RecordType,
				Values:          evaluation.Values,
				TTL:             record.TTL,
				Proxied:         record.Proxied,
			}
		}
		if shouldCleanupDNSProviderRetirements(evaluation.Status) {
			retirementActions = dnsProviderRetirementActions(record.ProviderRetirementsJSON)
		}
		return nil
	})
	if err != nil {
		return result, mapServiceError(err)
	}
	if skipEvaluation {
		return result, nil
	}
	if providerAction != nil {
		if err := service.executeDNSProviderAction(ctx, *providerAction); err != nil {
			evaluation.Status = "FAILED"
			evaluation.Diagnostics = append(evaluation.Diagnostics, DNSDiagnosticPayload{Code: "PROVIDER_APPLY_FAILED", Message: "DNS provider apply failed."})
			evaluation.ErrorMessage = err.Error()
		} else {
			providerApplied = true
		}
	}
	if providerApplied || providerAction == nil {
		if err := service.executeDNSProviderRetirementActions(ctx, retirementActions); err != nil {
			evaluation.Status = "FAILED"
			evaluation.Diagnostics = append(evaluation.Diagnostics, DNSDiagnosticPayload{Code: "PROVIDER_RETIREMENT_CLEANUP_FAILED", Message: "DNS provider retirement cleanup failed."})
			evaluation.ErrorMessage = err.Error()
		} else if len(retirementActions) > 0 {
			retirementCleaned = true
		}
	}
	err = service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if err := repositories.DNSRecords().LockDNSManagedRecordEvaluation(ctx, organizationID, recordID); err != nil {
			return err
		}
		record, err := repositories.DNSRecords().FindDNSManagedRecordByID(ctx, organizationID, recordID)
		if err != nil {
			return err
		}
		now := service.timestamp()
		if !evaluationSnapshot.matches(record) {
			if providerApplied && providerAction != nil {
				if dnsProviderActionTargetsRecord(*providerAction, record) {
					record.LastAppliedAt = now
					record.LastAppliedValuesJSON = stringListJSON(evaluation.Values)
				} else if len(providerAction.Values) > 0 {
					record.ProviderRetirementsJSON = appendDNSProviderRetirement(record.ProviderRetirementsJSON, buildDNSProviderRetirementFromAction(*providerAction, now))
				}
			}
			record.LastEvaluationStatus = "PENDING"
			record.LastEvaluationError = ""
			record.LastDiagnosticsJSON = diagnosticsJSON([]DNSDiagnosticPayload{{
				Code:    "STALE_EVALUATION_SKIPPED",
				Message: "DNS record changed while the provider operation was running; re-evaluation is required.",
			}})
			record.LastEvaluatedAt = now
			record.UpdatedAt = now
			if err := repositories.DNSRecords().UpdateDNSManagedRecordEvaluation(ctx, record); err != nil {
				return err
			}
			record, err = repositories.DNSRecords().FindDNSManagedRecordByID(ctx, organizationID, record.ID)
			if err != nil {
				return err
			}
			result, err = service.dnsManagedRecordPayloadForIdentity(ctx, repositories, identity, record)
			return err
		}
		if providerApplied {
			record.LastAppliedAt = now
			record.LastAppliedValuesJSON = stringListJSON(evaluation.Values)
		}
		if retirementCleaned {
			record.ProviderRetirementsJSON = "[]"
		}
		if evaluation.ActiveInstance != nil {
			evaluation.ActiveInstance.LastOutputValuesJSON = stringListJSON(evaluation.Values)
			evaluation.ActiveInstance.LastStatus = evaluation.Status
			evaluation.ActiveInstance.LastDiagnosticsJSON = diagnosticsJSON(evaluation.Diagnostics)
			evaluation.ActiveInstance.LastEvaluatedAt = now
			evaluation.ActiveInstance.UpdatedAt = now
			if err := repositories.DNSRecords().UpdateDNSInstanceEvaluation(ctx, *evaluation.ActiveInstance); err != nil {
				return err
			}
		}
		if evaluation.ActiveInstance != nil {
			record.ActiveInstanceID = evaluation.ActiveInstance.ID
		} else {
			record.ActiveInstanceID = ""
		}
		record.LastEvaluationStatus = evaluation.Status
		record.LastEvaluationError = evaluation.ErrorMessage
		record.LastDiagnosticsJSON = diagnosticsJSON(evaluation.Diagnostics)
		record.LastEvaluatedAt = now
		record.UpdatedAt = now
		if err := repositories.DNSRecords().UpdateDNSManagedRecordEvaluation(ctx, record); err != nil {
			return err
		}
		notifications = service.prepareDNSNotifications(record, evaluation, previous, notificationChannels, now)
		record, err = repositories.DNSRecords().FindDNSManagedRecordByID(ctx, organizationID, record.ID)
		if err != nil {
			return err
		}
		result, err = service.dnsManagedRecordPayloadForIdentity(ctx, repositories, identity, record)
		return err
	})
	if err != nil {
		return result, mapServiceError(err)
	}
	service.dispatchPreparedDNSNotificationsBestEffort(ctx, notifications)
	return result, nil
}

func (service *ControlService) dnsManagedRecordPayloadForIdentity(ctx context.Context, repositories repo.Repositories, identity *InternalIdentity, record repo.DNSManagedRecordRecord) (DNSManagedRecordPayload, error) {
	if identity == nil {
		return toDNSManagedRecordPayload(record), nil
	}
	records := service.filterDNSManagedRecordsForScope(ctx, repositories, *identity, []repo.DNSManagedRecordRecord{record})
	if len(records) == 0 {
		return DNSManagedRecordPayload{}, ErrForbidden
	}
	return toDNSManagedRecordPayload(records[0]), nil
}

func (service *ControlService) evaluateDNSManagedRecordsBestEffort(_ context.Context, organizationID string, recordIDs []string) {
	if len(recordIDs) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), dnsBestEffortEvaluationTimeout)
	defer cancel()
	seen := map[string]bool{}
	for _, recordID := range recordIDs {
		recordID = strings.TrimSpace(recordID)
		if recordID == "" || seen[recordID] {
			continue
		}
		seen[recordID] = true
		_, _ = service.evaluateDNSManagedRecordForOrganization(ctx, organizationID, recordID, nil)
	}
}

func (service *ControlService) ListNotificationChannels(ctx context.Context, identity InternalIdentity) ([]NotificationChannelPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionDNSRead)) && !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return nil, ErrForbidden
	}
	var result []NotificationChannelPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		channels, err := repositories.DNSRecords().ListNotificationChannelsByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		result = toNotificationChannelPayloads(channels)
		return nil
	})
	return result, mapServiceError(err)
}

type dnsPolicyAction struct {
	Type       string   `json:"type"`
	Values     []string `json:"values"`
	Value      string   `json:"value"`
	InstanceID string   `json:"instance_id"`
}
