package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func (service *ControlService) lockDNSManagedRecords(ctx context.Context, repositories repo.Repositories, organizationID string, recordIDs ...string) error {
	unique := make(map[string]bool)
	ordered := make([]string, 0, len(recordIDs))
	for _, recordID := range recordIDs {
		recordID = strings.TrimSpace(recordID)
		if recordID == "" || unique[recordID] {
			continue
		}
		unique[recordID] = true
		ordered = append(ordered, recordID)
	}
	sort.Strings(ordered)
	for _, recordID := range ordered {
		if err := repositories.DNSRecords().LockDNSManagedRecordEvaluation(ctx, organizationID, recordID); err != nil {
			return err
		}
	}
	return nil
}

func (service *ControlService) markDNSManagedRecordPending(ctx context.Context, repositories repo.Repositories, organizationID string, recordID string, updatedAt string, code string) (bool, error) {
	if strings.TrimSpace(recordID) == "" {
		return false, nil
	}
	if err := repositories.DNSRecords().LockDNSManagedRecordEvaluation(ctx, organizationID, recordID); err != nil {
		return false, err
	}
	record, err := repositories.DNSRecords().FindDNSManagedRecordByID(ctx, organizationID, recordID)
	if err != nil {
		return false, err
	}
	if strings.ToUpper(strings.TrimSpace(record.LastEvaluationStatus)) == "DELETE_PENDING" {
		return true, nil
	}
	if code == "" {
		code = "DNS_POLICY_INPUT_CHANGED"
	}
	record.LastEvaluationStatus = "PENDING"
	record.LastEvaluationError = ""
	record.LastDiagnosticsJSON = diagnosticsJSON([]DNSDiagnosticPayload{{Code: code, Message: "DNS policy changed; re-evaluation is required."}})
	record.UpdatedAt = updatedAt
	if err := repositories.DNSRecords().UpdateDNSManagedRecordEvaluation(ctx, record); err != nil {
		return false, err
	}
	return true, nil
}

func (service *ControlService) markDNSManagedRecordsPending(ctx context.Context, repositories repo.Repositories, organizationID string, recordIDs []string, updatedAt string, code string) ([]string, error) {
	unique := map[string]bool{}
	for _, recordID := range recordIDs {
		recordID = strings.TrimSpace(recordID)
		if recordID != "" {
			unique[recordID] = true
		}
	}
	ordered := sortedStringSetKeys(unique)
	if len(ordered) == 0 {
		return nil, nil
	}
	if err := service.lockDNSManagedRecords(ctx, repositories, organizationID, ordered...); err != nil {
		return nil, err
	}
	if code == "" {
		code = "DNS_POLICY_INPUT_CHANGED"
	}
	marked := make([]string, 0, len(ordered))
	for _, recordID := range ordered {
		record, err := repositories.DNSRecords().FindDNSManagedRecordByID(ctx, organizationID, recordID)
		if err != nil {
			return nil, err
		}
		if strings.ToUpper(strings.TrimSpace(record.LastEvaluationStatus)) == "DELETE_PENDING" {
			marked = append(marked, recordID)
			continue
		}
		record.LastEvaluationStatus = "PENDING"
		record.LastEvaluationError = ""
		record.LastDiagnosticsJSON = diagnosticsJSON([]DNSDiagnosticPayload{{Code: code, Message: "DNS policy changed; re-evaluation is required."}})
		record.UpdatedAt = updatedAt
		if err := repositories.DNSRecords().UpdateDNSManagedRecordEvaluation(ctx, record); err != nil {
			return nil, err
		}
		marked = append(marked, recordID)
	}
	return marked, nil
}

func (service *ControlService) ensureCanEvaluateDNSManagedRecord(ctx context.Context, repositories repo.Repositories, identity InternalIdentity, record repo.DNSManagedRecordRecord) error {
	for _, instance := range record.Instances {
		if !instance.Enabled || instance.DeletedAt != "" {
			continue
		}
		if err := service.ensureDNSInstanceUseScope(ctx, repositories, identity, instance, map[string]bool{}, 0, false); err != nil {
			return err
		}
	}
	return nil
}

func (service *ControlService) ensureCanManageDNSManagedRecord(ctx context.Context, repositories repo.Repositories, identity InternalIdentity, record repo.DNSManagedRecordRecord) error {
	for _, instance := range record.Instances {
		if instance.DeletedAt != "" {
			continue
		}
		if err := service.ensureDNSInstanceUseScope(ctx, repositories, identity, instance, map[string]bool{}, 0, true); err != nil {
			return err
		}
	}
	return nil
}

func (service *ControlService) filterDNSManagedRecordsForScope(ctx context.Context, repositories repo.Repositories, identity InternalIdentity, records []repo.DNSManagedRecordRecord) []repo.DNSManagedRecordRecord {
	filtered := make([]repo.DNSManagedRecordRecord, 0, len(records))
	for _, record := range records {
		instances := service.filterDNSInstancesForScope(ctx, repositories, identity, record.Instances)
		hasInstances := false
		for _, instance := range record.Instances {
			if instance.DeletedAt == "" {
				hasInstances = true
				break
			}
		}
		if hasInstances && len(instances) == 0 {
			continue
		}
		if shouldRedactDNSManagedRecordProviderState(record, instances) {
			redactDNSManagedRecordProviderState(&record)
		}
		record.Instances = instances
		filtered = append(filtered, record)
	}
	return filtered
}

func shouldRedactDNSManagedRecordProviderState(record repo.DNSManagedRecordRecord, visibleInstances []repo.DNSInstanceRecord) bool {
	activeInstances := 0
	for _, instance := range record.Instances {
		if instance.DeletedAt == "" {
			activeInstances++
		}
	}
	hasHiddenInstances := len(visibleInstances) < activeInstances
	if !hasHiddenInstances {
		return false
	}
	if len(parseDNSDiagnostics(record.LastDiagnosticsJSON)) > 0 {
		return true
	}
	if record.ActiveInstanceID == "" {
		return len(parseStringListJSON(record.LastAppliedValuesJSON)) > 0 ||
			strings.TrimSpace(record.LastAppliedAt) != "" ||
			strings.TrimSpace(record.LastEvaluationStatus) != "" ||
			strings.TrimSpace(record.LastEvaluationError) != "" ||
			len(parseDNSDiagnostics(record.LastDiagnosticsJSON)) > 0 ||
			strings.TrimSpace(record.LastEvaluatedAt) != ""
	}
	return !dnsInstanceSliceContainsID(visibleInstances, record.ActiveInstanceID)
}

func redactDNSManagedRecordProviderState(record *repo.DNSManagedRecordRecord) {
	record.ActiveInstanceID = ""
	record.LastAppliedValuesJSON = "[]"
	record.LastAppliedAt = ""
	record.LastEvaluationStatus = ""
	record.LastEvaluationError = ""
	record.LastDiagnosticsJSON = "[]"
	record.LastEvaluatedAt = ""
}

func dnsInstanceSliceContainsID(instances []repo.DNSInstanceRecord, instanceID string) bool {
	for _, instance := range instances {
		if instance.ID == instanceID {
			return true
		}
	}
	return false
}

func (service *ControlService) filterDNSInstancesForScope(ctx context.Context, repositories repo.Repositories, identity InternalIdentity, instances []repo.DNSInstanceRecord) []repo.DNSInstanceRecord {
	filtered := make([]repo.DNSInstanceRecord, 0, len(instances))
	for _, instance := range instances {
		if instance.DeletedAt != "" {
			continue
		}
		if err := service.ensureDNSInstanceUseScope(ctx, repositories, identity, instance, map[string]bool{}, 0, true); err == nil {
			filtered = append(filtered, instance)
		}
	}
	return filtered
}

func (service *ControlService) ensureDNSInstanceUseScope(ctx context.Context, repositories repo.Repositories, identity InternalIdentity, instance repo.DNSInstanceRecord, seen map[string]bool, depth int, allowMissingReferences bool) error {
	if depth > 8 {
		return ErrInvalidInput
	}
	if seen[instance.ID] {
		return ErrInvalidInput
	}
	seen[instance.ID] = true
	if err := service.ensureCanUseNodeGroups(identity, parseStringListJSON(instance.NodeGroupIDsJSON)); err != nil {
		return err
	}
	var action dnsPolicyAction
	if err := json.Unmarshal([]byte(instance.ActionJSON), &action); err != nil {
		if allowMissingReferences {
			return nil
		}
		return ErrInvalidInput
	}
	if normalizeDNSActionType(action.Type) != "USE_INSTANCE_OUTPUT" {
		return nil
	}
	if strings.TrimSpace(action.InstanceID) == "" {
		return ErrInvalidInput
	}
	target, err := repositories.DNSRecords().FindDNSInstanceByID(ctx, identity.OrganizationID, action.InstanceID)
	if allowMissingReferences && errors.Is(err, repo.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return service.ensureDNSInstanceUseScope(ctx, repositories, identity, target, seen, depth+1, allowMissingReferences)
}

func (service *ControlService) markDNSRecordsReferencingInstancePending(ctx context.Context, repositories repo.Repositories, organizationID string, instanceID string, updatedAt string) ([]string, error) {
	affected, err := service.collectDNSRecordsReferencingInstance(ctx, repositories, organizationID, instanceID, map[string]bool{}, 0)
	if err != nil {
		return nil, err
	}
	return service.markDNSManagedRecordsPending(ctx, repositories, organizationID, sortedStringSetKeys(affected), updatedAt, "REFERENCED_DNS_INSTANCE_CHANGED")
}

func (service *ControlService) collectDNSRecordsReferencingInstance(ctx context.Context, repositories repo.Repositories, organizationID string, instanceID string, seen map[string]bool, depth int) (map[string]bool, error) {
	affected := map[string]bool{}
	if depth > 8 || seen[instanceID] {
		return affected, nil
	}
	seen[instanceID] = true
	instances, err := repositories.DNSRecords().ListDNSInstancesByOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	for _, candidate := range instances {
		var action dnsPolicyAction
		if err := json.Unmarshal([]byte(candidate.ActionJSON), &action); err != nil {
			continue
		}
		if normalizeDNSActionType(action.Type) != "USE_INSTANCE_OUTPUT" || strings.TrimSpace(action.InstanceID) != instanceID {
			continue
		}
		affected[candidate.ManagedRecordID] = true
		childAffected, err := service.collectDNSRecordsReferencingInstance(ctx, repositories, organizationID, candidate.ID, seen, depth+1)
		if err != nil {
			return nil, err
		}
		for recordID := range childAffected {
			affected[recordID] = true
		}
	}
	return affected, nil
}

func (service *ControlService) markDNSRecordsDependingOnNodeGroupsPending(ctx context.Context, repositories repo.Repositories, organizationID string, groupIDs []string, updatedAt string) ([]string, error) {
	records, err := repositories.DNSRecords().ListDNSManagedRecordsByOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	changed := make(map[string]bool)
	for _, groupID := range groupIDs {
		changed[groupID] = true
	}
	seenReferences := map[string]bool{}
	affected := map[string]bool{}
	for _, record := range records {
		recordMarked := false
		for _, instance := range record.Instances {
			if !instance.Enabled || instance.DeletedAt != "" {
				continue
			}
			if dnsInstanceDependsOnNodeGroups(instance, changed) {
				if !recordMarked {
					affected[record.ID] = true
					recordMarked = true
				}
				referenced, err := service.collectDNSRecordsReferencingInstance(ctx, repositories, organizationID, instance.ID, seenReferences, 0)
				if err != nil {
					return nil, err
				}
				for recordID := range referenced {
					affected[recordID] = true
				}
			}
		}
	}
	return service.markDNSManagedRecordsPending(ctx, repositories, organizationID, sortedStringSetKeys(affected), updatedAt, "NODE_DNS_INPUT_CHANGED")
}

func dnsInstanceDependsOnNodeGroups(instance repo.DNSInstanceRecord, groupIDs map[string]bool) bool {
	instanceGroups := parseStringListJSON(instance.NodeGroupIDsJSON)
	if len(instanceGroups) == 0 {
		return true
	}
	if len(groupIDs) == 0 {
		return false
	}
	for _, groupID := range instanceGroups {
		if groupIDs[groupID] {
			return true
		}
	}
	return false
}

func sortedStringSetKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
