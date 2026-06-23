package service

import (
	"encoding/json"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func toHealthCheckPayloads(checks []repo.HealthCheckRecord, latestResults map[string][]repo.HealthResultRecord) []HealthCheckPayload {
	payloads := make([]HealthCheckPayload, 0, len(checks))
	for _, check := range checks {
		payloads = append(payloads, toHealthCheckPayloadWithLatestResults(check, latestResults[check.ID]))
	}
	return payloads
}

func toHealthCheckPayload(check repo.HealthCheckRecord) HealthCheckPayload {
	return toHealthCheckPayloadWithLatestResults(check, nil)
}

func toHealthCheckPayloadWithLatestResults(check repo.HealthCheckRecord, latestResults []repo.HealthResultRecord) HealthCheckPayload {
	config := map[string]any{}
	_ = json.Unmarshal([]byte(normalizedConfigJSON(check.ConfigJSON)), &config)
	targetScope := HealthTargetScopePayload{Type: "TARGETS"}
	targets := make([]HealthCheckTargetPayload, 0, len(check.Targets))
	for _, target := range check.Targets {
		if target.ScopeType == "TARGET_GROUP" && target.TargetGroupID != "" {
			targetScope = HealthTargetScopePayload{Type: "TARGET_GROUP", TargetGroupID: target.TargetGroupID}
		}
		if target.TargetID == "" {
			continue
		}
		if target.ScopeType == "TARGET" {
			targetScope.TargetIDs = append(targetScope.TargetIDs, target.TargetID)
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
		TargetScope:     targetScope,
		Targets:         targets,
		MonitorScopes:   scopes,
		LatestResults:   toHealthResultPayloads(latestResults),
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

func toDNSCredentialPayloads(credentials []repo.DNSCredentialRecord, zones []repo.DNSCredentialZoneRecord) []DNSCredentialPayload {
	zonesByCredential := make(map[string][]repo.DNSCredentialZoneRecord, len(credentials))
	for _, zone := range zones {
		zonesByCredential[zone.DNSCredentialID] = append(zonesByCredential[zone.DNSCredentialID], zone)
	}
	payloads := make([]DNSCredentialPayload, 0, len(credentials))
	for _, credential := range credentials {
		payloads = append(payloads, toDNSCredentialPayload(credential, zonesByCredential[credential.ID]))
	}
	return payloads
}

func toDNSCredentialPayload(credential repo.DNSCredentialRecord, zones []repo.DNSCredentialZoneRecord) DNSCredentialPayload {
	payload := DNSCredentialPayload{ID: credential.ID, Name: credential.Name, Provider: credential.Provider, Zones: make([]DNSCredentialZonePayload, 0, len(zones))}
	for _, zone := range zones {
		payload.Zones = append(payload.Zones, DNSCredentialZonePayload{
			ID:           zone.ID,
			ZoneID:       zone.ZoneID,
			ZoneName:     zone.ZoneName,
			Status:       zone.Status,
			LastSyncedAt: zone.LastSyncedAt,
		})
	}
	return payload
}

func parseStringListJSON(value string) []string {
	var values []string
	if err := json.Unmarshal([]byte(value), &values); err != nil {
		return nil
	}
	return values
}
