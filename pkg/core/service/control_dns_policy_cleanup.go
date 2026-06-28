package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func dnsManagedRecordProviderIdentityChanged(left repo.DNSManagedRecordRecord, right repo.DNSManagedRecordRecord) bool {
	return left.DNSCredentialID != right.DNSCredentialID ||
		left.CredentialZoneID != right.CredentialZoneID ||
		dnsManagedRecordProviderTargetChanged(left, right)
}

func dnsManagedRecordProviderTargetChanged(left repo.DNSManagedRecordRecord, right repo.DNSManagedRecordRecord) bool {
	return left.ZoneID != right.ZoneID ||
		left.RecordName != right.RecordName ||
		left.RecordType != right.RecordType
}

func dnsManagedRecordProviderSettingsChanged(left repo.DNSManagedRecordRecord, right repo.DNSManagedRecordRecord) bool {
	return left.TTL != right.TTL || left.Proxied != right.Proxied
}

func dnsProviderTargetConflicts(record repo.DNSManagedRecordRecord, zoneID string, recordName string, recordType string) bool {
	if record.ZoneID != zoneID || !strings.EqualFold(record.RecordName, recordName) {
		return false
	}
	return strings.EqualFold(record.RecordType, recordType) || strings.EqualFold(record.RecordType, "CNAME") || strings.EqualFold(recordType, "CNAME")
}

func dnsProviderRetirementTargetConflicts(raw string, zoneID string, recordName string, recordType string) bool {
	for _, retirement := range parseDNSProviderRetirements(raw) {
		if retirement.Zone != zoneID || !strings.EqualFold(retirement.RecordName, recordName) {
			continue
		}
		if strings.EqualFold(retirement.RecordType, recordType) || strings.EqualFold(retirement.RecordType, "CNAME") || strings.EqualFold(recordType, "CNAME") {
			return true
		}
	}
	return false
}

type dnsManagedRecordEvaluationSnapshot struct {
	DNSCredentialID         string
	CredentialZoneID        string
	ZoneID                  string
	ZoneName                string
	RecordHost              string
	RecordName              string
	RecordType              string
	TTL                     int
	Proxied                 bool
	ActiveInstanceID        string
	LastAppliedValuesJSON   string
	LastEvaluationStatus    string
	ProviderRetirementsJSON string
	UpdatedAt               string
	InstancesJSON           string
}

type dnsInstanceEvaluationSnapshot struct {
	ID                         string `json:"id"`
	ManagedRecordID            string `json:"managed_record_id"`
	Priority                   int    `json:"priority"`
	Enabled                    bool   `json:"enabled"`
	NodeGroupIDsJSON           string `json:"node_group_ids_json"`
	AnswerCount                int    `json:"answer_count"`
	ConditionJSON              string `json:"condition_json"`
	ActionJSON                 string `json:"action_json"`
	NotificationChannelIDsJSON string `json:"notification_channel_ids_json"`
	DeletedAt                  string `json:"deleted_at"`
}

func newDNSManagedRecordEvaluationSnapshot(record repo.DNSManagedRecordRecord) dnsManagedRecordEvaluationSnapshot {
	instances := make([]dnsInstanceEvaluationSnapshot, 0, len(record.Instances))
	for _, instance := range record.Instances {
		instances = append(instances, dnsInstanceEvaluationSnapshot{
			ID:                         instance.ID,
			ManagedRecordID:            instance.ManagedRecordID,
			Priority:                   instance.Priority,
			Enabled:                    instance.Enabled,
			NodeGroupIDsJSON:           instance.NodeGroupIDsJSON,
			AnswerCount:                instance.AnswerCount,
			ConditionJSON:              instance.ConditionJSON,
			ActionJSON:                 instance.ActionJSON,
			NotificationChannelIDsJSON: instance.NotificationChannelIDsJSON,
			DeletedAt:                  instance.DeletedAt,
		})
	}
	instancesJSON, err := json.Marshal(instances)
	if err != nil {
		instancesJSON = []byte("[]")
	}
	return dnsManagedRecordEvaluationSnapshot{
		DNSCredentialID:         record.DNSCredentialID,
		CredentialZoneID:        record.CredentialZoneID,
		ZoneID:                  record.ZoneID,
		ZoneName:                record.ZoneName,
		RecordHost:              record.RecordHost,
		RecordName:              record.RecordName,
		RecordType:              record.RecordType,
		TTL:                     record.TTL,
		Proxied:                 record.Proxied,
		ActiveInstanceID:        record.ActiveInstanceID,
		LastAppliedValuesJSON:   stringListJSON(parseStringListJSON(record.LastAppliedValuesJSON)),
		LastEvaluationStatus:    record.LastEvaluationStatus,
		ProviderRetirementsJSON: dnsProviderRetirementsJSON(parseDNSProviderRetirements(record.ProviderRetirementsJSON)),
		UpdatedAt:               normalizeDNSSnapshotTimestamp(record.UpdatedAt),
		InstancesJSON:           string(instancesJSON),
	}
}

func (snapshot dnsManagedRecordEvaluationSnapshot) matches(record repo.DNSManagedRecordRecord) bool {
	return snapshot == newDNSManagedRecordEvaluationSnapshot(record)
}

func normalizeDNSSnapshotTimestamp(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	formats := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999-07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05.999999-07",
		"2006-01-02 15:04:05-07",
	}
	for _, format := range formats {
		parsed, err := time.Parse(format, value)
		if err == nil {
			return parsed.UTC().Round(time.Microsecond).Format(time.RFC3339Nano)
		}
	}
	return value
}

type dnsProviderRetirement struct {
	Provider        string `json:"provider"`
	EncryptedSecret string `json:"encrypted_secret"`
	Zone            string `json:"zone"`
	RecordName      string `json:"record_name"`
	RecordType      string `json:"record_type"`
	TTL             int    `json:"ttl"`
	Proxied         bool   `json:"proxied"`
	CreatedAt       string `json:"created_at"`
}

func buildDNSProviderRetirement(record repo.DNSManagedRecordRecord, credential repo.DNSCredentialRecord, now string) dnsProviderRetirement {
	return dnsProviderRetirement{
		Provider:        credential.Provider,
		EncryptedSecret: credential.EncryptedSecret,
		Zone:            record.ZoneID,
		RecordName:      record.RecordName,
		RecordType:      record.RecordType,
		TTL:             record.TTL,
		Proxied:         record.Proxied,
		CreatedAt:       now,
	}
}

func buildDNSProviderRetirementFromAction(action dnsProviderApplyAction, now string) dnsProviderRetirement {
	return dnsProviderRetirement{
		Provider:        action.Provider,
		EncryptedSecret: action.EncryptedSecret,
		Zone:            action.Zone,
		RecordName:      action.RecordName,
		RecordType:      action.RecordType,
		TTL:             action.TTL,
		Proxied:         action.Proxied,
		CreatedAt:       now,
	}
}

func dnsProviderActionTargetsRecord(action dnsProviderApplyAction, record repo.DNSManagedRecordRecord) bool {
	return action.Zone == record.ZoneID &&
		strings.EqualFold(action.RecordName, record.RecordName) &&
		strings.EqualFold(action.RecordType, record.RecordType)
}

func appendDNSProviderRetirement(raw string, retirement dnsProviderRetirement) string {
	retirements := parseDNSProviderRetirements(raw)
	retirements = append(retirements, retirement)
	return dnsProviderRetirementsJSON(retirements)
}

func parseDNSProviderRetirements(raw string) []dnsProviderRetirement {
	var retirements []dnsProviderRetirement
	if err := json.Unmarshal([]byte(raw), &retirements); err != nil {
		return nil
	}
	return retirements
}

func dnsProviderRetirementsJSON(retirements []dnsProviderRetirement) string {
	if retirements == nil {
		retirements = []dnsProviderRetirement{}
	}
	data, err := json.Marshal(retirements)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func shouldCleanupDNSProviderRetirements(status string) bool {
	switch status {
	case "APPLIED", "NO_MATCH":
		return true
	default:
		return false
	}
}

func dnsProviderRetirementActions(raw string) []dnsProviderApplyAction {
	retirements := parseDNSProviderRetirements(raw)
	actions := make([]dnsProviderApplyAction, 0, len(retirements))
	for _, retirement := range retirements {
		actions = append(actions, dnsProviderApplyAction{
			Provider:        retirement.Provider,
			EncryptedSecret: retirement.EncryptedSecret,
			Zone:            retirement.Zone,
			RecordName:      retirement.RecordName,
			RecordType:      retirement.RecordType,
			Values:          nil,
			TTL:             retirement.TTL,
			Proxied:         retirement.Proxied,
		})
	}
	return actions
}

func (service *ControlService) executeDNSProviderRetirementActions(ctx context.Context, actions []dnsProviderApplyAction) error {
	for _, action := range actions {
		if err := service.executeDNSProviderAction(ctx, action); err != nil {
			return err
		}
	}
	return nil
}
