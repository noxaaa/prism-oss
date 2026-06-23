package service

import (
	"encoding/json"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func jsonObjectString(value map[string]any) (string, error) {
	if value == nil {
		value = map[string]any{}
	}
	data, err := json.Marshal(value)
	return string(data), err
}

func jsonStringList(values []string) (string, error) {
	normalized := normalizeDNSValues(values)
	data, err := json.Marshal(normalized)
	return string(data), err
}

func diagnosticsJSON(values []DNSDiagnosticPayload) string {
	if values == nil {
		values = []DNSDiagnosticPayload{}
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case json.Number:
		number, err := typed.Float64()
		return number, err == nil
	default:
		return 0, false
	}
}

func parseJSONObject(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return map[string]any{}
	}
	return value
}

func parseDNSDiagnostics(raw string) []DNSDiagnosticPayload {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var values []DNSDiagnosticPayload
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return values
}

func toDNSManagedRecordPayloads(records []repo.DNSManagedRecordRecord) []DNSManagedRecordPayload {
	payloads := make([]DNSManagedRecordPayload, 0, len(records))
	for _, record := range records {
		payloads = append(payloads, toDNSManagedRecordPayload(record))
	}
	return payloads
}

func toDNSManagedRecordPayload(record repo.DNSManagedRecordRecord) DNSManagedRecordPayload {
	return DNSManagedRecordPayload{
		ID:                   record.ID,
		DNSCredentialID:      record.DNSCredentialID,
		CredentialZoneID:     record.CredentialZoneID,
		ZoneID:               record.ZoneID,
		ZoneName:             record.ZoneName,
		RecordHost:           record.RecordHost,
		RecordName:           record.RecordName,
		RecordType:           record.RecordType,
		TTL:                  record.TTL,
		Proxied:              record.Proxied,
		ActiveInstanceID:     record.ActiveInstanceID,
		LastAppliedValues:    parseStringListJSON(record.LastAppliedValuesJSON),
		LastEvaluationStatus: record.LastEvaluationStatus,
		LastEvaluationError:  record.LastEvaluationError,
		LastDiagnostics:      parseDNSDiagnostics(record.LastDiagnosticsJSON),
		LastEvaluatedAt:      record.LastEvaluatedAt,
		LastAppliedAt:        record.LastAppliedAt,
		Instances:            toDNSInstancePayloads(record.Instances),
	}
}

func toDNSInstancePayloads(instances []repo.DNSInstanceRecord) []DNSInstancePayload {
	payloads := make([]DNSInstancePayload, 0, len(instances))
	for _, instance := range instances {
		payloads = append(payloads, toDNSInstancePayload(instance))
	}
	return payloads
}

func toDNSInstancePayload(instance repo.DNSInstanceRecord) DNSInstancePayload {
	return DNSInstancePayload{
		ID:                     instance.ID,
		ManagedRecordID:        instance.ManagedRecordID,
		Name:                   instance.Name,
		Priority:               instance.Priority,
		Enabled:                instance.Enabled,
		NodeGroupIDs:           parseStringListJSON(instance.NodeGroupIDsJSON),
		AnswerCount:            instance.AnswerCount,
		Condition:              parseJSONObject(instance.ConditionJSON),
		Action:                 parseJSONObject(instance.ActionJSON),
		NotificationChannelIDs: parseStringListJSON(instance.NotificationChannelIDsJSON),
		LastOutputValues:       parseStringListJSON(instance.LastOutputValuesJSON),
		LastStatus:             instance.LastStatus,
		LastDiagnostics:        parseDNSDiagnostics(instance.LastDiagnosticsJSON),
		LastEvaluatedAt:        instance.LastEvaluatedAt,
	}
}

func toNotificationChannelPayload(channel repo.NotificationChannelRecord) NotificationChannelPayload {
	return NotificationChannelPayload{ID: channel.ID, Name: channel.Name, ChannelType: channel.ChannelType, Config: parseJSONObject(channel.ConfigJSON), Enabled: channel.Enabled}
}

func toNotificationChannelPayloads(channels []repo.NotificationChannelRecord) []NotificationChannelPayload {
	payloads := make([]NotificationChannelPayload, 0, len(channels))
	for _, channel := range channels {
		payloads = append(payloads, toNotificationChannelPayload(channel))
	}
	return payloads
}
