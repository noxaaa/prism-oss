package service

import (
	"encoding/json"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func toHealthCheckPayloads(checks []repo.HealthCheckRecord) []HealthCheckPayload {
	payloads := make([]HealthCheckPayload, 0, len(checks))
	for _, check := range checks {
		payloads = append(payloads, toHealthCheckPayload(check))
	}
	return payloads
}

func toHealthCheckPayload(check repo.HealthCheckRecord) HealthCheckPayload {
	config := map[string]any{}
	_ = json.Unmarshal([]byte(normalizedConfigJSON(check.ConfigJSON)), &config)
	targets := make([]HealthCheckTargetPayload, 0, len(check.Targets))
	for _, target := range check.Targets {
		if target.TargetID == "" {
			continue
		}
		targets = append(targets, HealthCheckTargetPayload{
			ID:            target.ID,
			ScopeType:     target.ScopeType,
			TargetID:      target.TargetID,
			TargetGroupID: target.TargetGroupID,
			TargetName:    target.TargetName,
			TargetHost:    target.TargetHost,
			TargetPort:    target.TargetPort,
		})
	}
	scopes := make([]HealthMonitorScopePayload, 0, len(check.MonitorScopes))
	for _, scope := range check.MonitorScopes {
		scopes = append(scopes, HealthMonitorScopePayload{ID: scope.ID, ScopeType: scope.ScopeType, MonitorID: scope.MonitorID, MonitorGroupID: scope.MonitorGroupID})
	}
	return HealthCheckPayload{
		ID:              check.ID,
		Name:            check.Name,
		ProbeType:       check.ProbeType,
		IntervalSeconds: check.IntervalSeconds,
		TimeoutSeconds:  check.TimeoutSeconds,
		Config:          config,
		Enabled:         check.Enabled,
		Targets:         targets,
		MonitorScopes:   scopes,
	}
}

func toHealthResultPayloads(results []repo.HealthResultRecord) []HealthResultPayload {
	payloads := make([]HealthResultPayload, 0, len(results))
	for _, result := range results {
		payloads = append(payloads, HealthResultPayload{
			ID:                  result.ID,
			HealthCheckID:       result.HealthCheckID,
			HealthCheckTargetID: result.HealthCheckTargetID,
			MonitorID:           result.MonitorID,
			TargetID:            result.TargetID,
			Status:              result.Status,
			LatencyMS:           result.LatencyMS,
			ErrorMessage:        result.ErrorMessage,
			ObservedAt:          result.ObservedAt,
			CreatedAt:           result.CreatedAt,
		})
	}
	return payloads
}

func toDNSCredentialPayloads(credentials []repo.DNSCredentialRecord) []DNSCredentialPayload {
	payloads := make([]DNSCredentialPayload, 0, len(credentials))
	for _, credential := range credentials {
		payloads = append(payloads, toDNSCredentialPayload(credential))
	}
	return payloads
}

func toDNSCredentialPayload(credential repo.DNSCredentialRecord) DNSCredentialPayload {
	return DNSCredentialPayload{ID: credential.ID, Name: credential.Name, Provider: credential.Provider}
}

func toDNSRecordPayloads(records []repo.DNSRecordRecord) []DNSRecordPayload {
	payloads := make([]DNSRecordPayload, 0, len(records))
	for _, record := range records {
		payloads = append(payloads, toDNSRecordPayload(record))
	}
	return payloads
}

func toDNSRecordPayload(record repo.DNSRecordRecord) DNSRecordPayload {
	return DNSRecordPayload{
		ID:                record.ID,
		DNSCredentialID:   record.DNSCredentialID,
		Zone:              record.Zone,
		RecordName:        record.RecordName,
		RecordType:        record.RecordType,
		ManagedMode:       record.ManagedMode,
		DesiredValues:     parseStringListJSON(record.DesiredValuesJSON),
		LastAppliedValues: parseStringListJSON(record.LastAppliedValuesJSON),
		LastAppliedAt:     record.LastAppliedAt,
	}
}

func parseStringListJSON(value string) []string {
	var values []string
	if err := json.Unmarshal([]byte(value), &values); err != nil {
		return nil
	}
	return values
}
