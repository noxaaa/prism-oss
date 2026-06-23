package service

import (
	"context"
	"errors"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func (service *ControlService) ListHealthChecks(ctx context.Context, identity InternalIdentity) ([]HealthCheckPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionHealthChecksRead)) && !service.hasPermission(identity, string(domain.PermissionHealthChecksManage)) {
		return nil, ErrForbidden
	}
	var result []HealthCheckPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		checks, err := repositories.HealthChecks().ListHealthChecksByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		latestResults, err := service.latestHealthResultsByCheck(ctx, repositories, identity.OrganizationID, checks)
		if err != nil {
			return err
		}
		result = toHealthCheckPayloads(checks, latestResults)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) CreateHealthCheck(ctx context.Context, identity InternalIdentity, input HealthCheckMutationInput) (HealthCheckPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionHealthChecksManage)) {
		return HealthCheckPayload{}, ErrForbidden
	}
	var result HealthCheckPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		targets, monitorScopes, err := service.buildHealthBindings(ctx, repositories, identity.OrganizationID, input)
		if err != nil {
			return err
		}
		now := service.timestamp()
		check := repo.HealthCheckRecord{
			ID:              service.newID(),
			OrganizationID:  identity.OrganizationID,
			Name:            input.Name,
			ProbeType:       input.ProbeType,
			IntervalSeconds: input.IntervalSeconds,
			TimeoutSeconds:  input.TimeoutSeconds,
			ConfigJSON:      normalizedConfigJSON(input.ConfigJSON),
			Enabled:         input.Enabled,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := repositories.HealthChecks().CreateHealthCheck(ctx, check, targets, monitorScopes, now, service.newID); err != nil {
			return err
		}
		check, err = repositories.HealthChecks().FindHealthCheckByID(ctx, identity.OrganizationID, check.ID)
		if err != nil {
			return err
		}
		result = toHealthCheckPayload(check)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "health_checks.create", "HEALTH_CHECK", check.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) UpdateHealthCheck(ctx context.Context, identity InternalIdentity, healthCheckID string, input HealthCheckMutationInput) (HealthCheckPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionHealthChecksManage)) {
		return HealthCheckPayload{}, ErrForbidden
	}
	var result HealthCheckPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		check, err := repositories.HealthChecks().FindHealthCheckByID(ctx, identity.OrganizationID, healthCheckID)
		if err != nil {
			return err
		}
		targets, monitorScopes, err := service.buildHealthBindings(ctx, repositories, identity.OrganizationID, input)
		if err != nil {
			return err
		}
		check.Name = input.Name
		check.ProbeType = input.ProbeType
		check.IntervalSeconds = input.IntervalSeconds
		check.TimeoutSeconds = input.TimeoutSeconds
		check.ConfigJSON = normalizedConfigJSON(input.ConfigJSON)
		check.Enabled = input.Enabled
		check.UpdatedAt = service.timestamp()
		if err := repositories.HealthChecks().UpdateHealthCheck(ctx, check, targets, monitorScopes, check.UpdatedAt, service.newID); err != nil {
			return err
		}
		check, err = repositories.HealthChecks().FindHealthCheckByID(ctx, identity.OrganizationID, check.ID)
		if err != nil {
			return err
		}
		result = toHealthCheckPayload(check)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "health_checks.update", "HEALTH_CHECK", check.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) DeleteHealthCheck(ctx context.Context, identity InternalIdentity, healthCheckID string) error {
	if !service.hasPermission(identity, string(domain.PermissionHealthChecksManage)) {
		return ErrForbidden
	}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if err := ensureHealthCheckHasNoActions(ctx, repositories, identity.OrganizationID, healthCheckID); err != nil {
			return err
		}
		deletedAt := service.timestamp()
		if err := repositories.HealthChecks().DeleteHealthCheck(ctx, identity.OrganizationID, healthCheckID, deletedAt); err != nil {
			return err
		}
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "health_checks.delete", "HEALTH_CHECK", healthCheckID, ""))
	})
	return mapServiceError(err)
}

func ensureHealthCheckHasNoActions(ctx context.Context, repositories repo.Repositories, organizationID string, healthCheckID string) error {
	rules, err := repositories.HealthChecks().ListHealthEvaluationRulesByCheck(ctx, organizationID, healthCheckID)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		for _, event := range rule.Events {
			if event.Enabled {
				return ErrConflict
			}
		}
	}
	return nil
}

func (service *ControlService) ListHealthResults(ctx context.Context, identity InternalIdentity, healthCheckID string) ([]HealthResultPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionHealthChecksRead)) && !service.hasPermission(identity, string(domain.PermissionHealthChecksManage)) {
		return nil, ErrForbidden
	}
	var result []HealthResultPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		results, err := repositories.HealthChecks().ListHealthResults(ctx, identity.OrganizationID, healthCheckID, 100)
		if err != nil {
			return err
		}
		result = toHealthResultPayloads(results)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) RecordMonitorHealthResults(ctx context.Context, organizationID string, monitorID string, results []HealthResultInput) error {
	var actions []pendingHealthAction
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		now := service.timestamp()
		records := make([]repo.HealthResultRecord, 0, len(results))
		if err := service.authorizeMonitorHealthResults(ctx, repositories, organizationID, monitorID, results); err != nil {
			return err
		}
		for _, result := range results {
			observedAt := clampFutureObservedAt(result.ObservedAt, now)
			records = append(records, repo.HealthResultRecord{
				ID:                  service.newID(),
				OrganizationID:      organizationID,
				HealthCheckID:       result.HealthCheckID,
				HealthCheckTargetID: result.HealthCheckTargetID,
				MonitorID:           monitorID,
				TargetID:            result.TargetID,
				Status:              result.Status,
				LatencyMS:           result.LatencyMS,
				ErrorMessage:        result.ErrorMessage,
				ObservedAt:          observedAt,
				CreatedAt:           now,
			})
		}
		if err := repositories.HealthChecks().RecordHealthResults(ctx, organizationID, records); err != nil {
			return err
		}
		var err error
		actions, err = service.buildHealthActions(ctx, repositories, organizationID, records)
		return err
	})
	if err != nil {
		return mapServiceError(err)
	}
	for _, action := range actions {
		if err := action.executor.Execute(ctx, action.payload); err != nil {
			return mapServiceError(err)
		}
	}
	return nil
}

func (service *ControlService) authorizeMonitorHealthResults(ctx context.Context, repositories repo.Repositories, organizationID string, monitorID string, results []HealthResultInput) error {
	if len(results) == 0 {
		return nil
	}
	monitor, err := repositories.Monitors().FindMonitorByID(ctx, organizationID, monitorID)
	if err != nil {
		return err
	}
	activeMonitorGroupIDs, err := activeMonitorGroupIDSet(ctx, repositories, organizationID)
	if err != nil {
		return err
	}
	checks := make(map[string]repo.HealthCheckRecord)
	for _, result := range results {
		check, ok := checks[result.HealthCheckID]
		if !ok {
			check, err = repositories.HealthChecks().FindHealthCheckByID(ctx, organizationID, result.HealthCheckID)
			if err != nil {
				return err
			}
			check, err = service.syncHealthCheckTargetGroupBindings(ctx, repositories, organizationID, check)
			if err != nil {
				return err
			}
			checks[result.HealthCheckID] = check
		}
		if !check.Enabled || !healthCheckTargetsMonitor(check, monitor, activeMonitorGroupIDs) || !healthCheckIncludesResultTarget(check, result) {
			return ErrForbidden
		}
	}
	return nil
}

func healthCheckIncludesResultTarget(check repo.HealthCheckRecord, result HealthResultInput) bool {
	for _, target := range check.Targets {
		if target.ID == result.HealthCheckTargetID && target.TargetID == result.TargetID {
			return true
		}
	}
	return false
}

func (service *ControlService) buildHealthBindings(ctx context.Context, repositories repo.Repositories, organizationID string, input HealthCheckMutationInput) ([]repo.HealthCheckTargetRecord, []repo.HealthCheckMonitorScopeRecord, error) {
	targets := make([]repo.HealthCheckTargetRecord, 0)
	switch input.TargetScope.Type {
	case "TARGETS":
		for _, targetID := range input.TargetScope.TargetIDs {
			target, err := repositories.Targets().FindTargetByID(ctx, organizationID, targetID)
			if err != nil {
				return nil, nil, err
			}
			if !target.Enabled {
				return nil, nil, ErrInvalidInput
			}
			targets = append(targets, repo.HealthCheckTargetRecord{ScopeType: "TARGET", TargetID: targetID})
		}
	case "TARGET_GROUP":
		targetGroup, err := repositories.TargetGroups().FindTargetGroupByID(ctx, organizationID, input.TargetScope.TargetGroupID)
		if err != nil {
			return nil, nil, err
		}
		targets, err = service.appendTargetGroupHealthBindings(ctx, repositories, organizationID, targets, targetGroup)
		if err != nil {
			return nil, nil, err
		}
	default:
		return nil, nil, ErrInvalidInput
	}
	if len(targets) == 0 {
		return nil, nil, ErrInvalidInput
	}

	var scopes []repo.HealthCheckMonitorScopeRecord
	switch input.MonitorScope.Type {
	case "MONITOR":
		if _, err := repositories.Monitors().FindMonitorByID(ctx, organizationID, input.MonitorScope.MonitorID); err != nil {
			return nil, nil, err
		}
		scopes = []repo.HealthCheckMonitorScopeRecord{{ScopeType: "MONITOR", MonitorID: input.MonitorScope.MonitorID}}
	case "MONITOR_GROUP":
		if _, err := repositories.MonitorGroups().FindMonitorGroupByID(ctx, organizationID, input.MonitorScope.MonitorGroupID); err != nil {
			return nil, nil, err
		}
		scopes = []repo.HealthCheckMonitorScopeRecord{{ScopeType: "MONITOR_GROUP", MonitorGroupID: input.MonitorScope.MonitorGroupID}}
	default:
		return nil, nil, ErrInvalidInput
	}
	return targets, scopes, nil
}

func (service *ControlService) appendTargetGroupHealthBindings(ctx context.Context, repositories repo.Repositories, organizationID string, targets []repo.HealthCheckTargetRecord, targetGroup repo.TargetGroupRecord) ([]repo.HealthCheckTargetRecord, error) {
	targets = append(targets, repo.HealthCheckTargetRecord{
		ScopeType:     "TARGET_GROUP",
		TargetGroupID: targetGroup.ID,
	})
	for _, member := range targetGroup.Members {
		if !member.Enabled {
			continue
		}
		target, err := repositories.Targets().FindTargetByID(ctx, organizationID, member.TargetID)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				continue
			}
			return nil, err
		}
		if !target.Enabled {
			continue
		}
		targets = append(targets, repo.HealthCheckTargetRecord{
			ScopeType:     "TARGET_GROUP",
			TargetID:      target.ID,
			TargetGroupID: targetGroup.ID,
		})
	}
	return targets, nil
}

func (service *ControlService) CompileMonitorAgentConfig(ctx context.Context, organizationID string, monitorID string) (agent.MonitorConfigSnapshot, error) {
	var snapshot agent.MonitorConfigSnapshot
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		monitor, err := repositories.Monitors().FindMonitorByID(ctx, organizationID, monitorID)
		if err != nil {
			return err
		}
		checks, err := repositories.HealthChecks().ListHealthChecksByOrganization(ctx, organizationID)
		if err != nil {
			return err
		}
		snapshot = agent.MonitorConfigSnapshot{
			MonitorID:      monitor.ID,
			ConfigVersion:  monitor.DesiredConfigVersion,
			HealthChecks:   make([]agent.MonitorHealthCheck, 0),
			GeneratedAtUTC: service.timestamp(),
		}
		activeMonitorGroupIDs, err := activeMonitorGroupIDSet(ctx, repositories, organizationID)
		if err != nil {
			return err
		}
		for _, check := range checks {
			if !check.Enabled || !healthCheckTargetsMonitor(check, monitor, activeMonitorGroupIDs) {
				continue
			}
			check, err = service.syncHealthCheckTargetGroupBindings(ctx, repositories, organizationID, check)
			if err != nil {
				return err
			}
			compiled := agent.MonitorHealthCheck{
				ID:              check.ID,
				ProbeType:       check.ProbeType,
				IntervalSeconds: check.IntervalSeconds,
				TimeoutSeconds:  check.TimeoutSeconds,
				ConfigJSON:      check.ConfigJSON,
				Targets:         make([]agent.MonitorHealthTarget, 0, len(check.Targets)),
			}
			for _, target := range check.Targets {
				if target.TargetID == "" {
					continue
				}
				compiled.Targets = append(compiled.Targets, agent.MonitorHealthTarget{
					HealthCheckTargetID: target.ID,
					TargetID:            target.TargetID,
					Name:                target.TargetName,
					Host:                target.TargetHost,
					Port:                target.TargetPort,
				})
			}
			snapshot.HealthChecks = append(snapshot.HealthChecks, compiled)
		}
		return nil
	})
	return snapshot, mapServiceError(err)
}

func (service *ControlService) syncHealthCheckTargetGroupBindings(ctx context.Context, repositories repo.Repositories, organizationID string, check repo.HealthCheckRecord) (repo.HealthCheckRecord, error) {
	targetGroupIDs := healthCheckTargetGroupIDs(check)
	if len(targetGroupIDs) == 0 {
		return check, nil
	}
	targets := make([]repo.HealthCheckTargetRecord, 0)
	for _, targetGroupID := range targetGroupIDs {
		group, err := repositories.TargetGroups().FindTargetGroupByID(ctx, organizationID, targetGroupID)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				targets = append(targets, repo.HealthCheckTargetRecord{
					ScopeType:     "TARGET_GROUP",
					TargetGroupID: targetGroupID,
				})
				continue
			}
			return repo.HealthCheckRecord{}, err
		}
		targets, err = service.appendTargetGroupHealthBindings(ctx, repositories, organizationID, targets, group)
		if err != nil {
			return repo.HealthCheckRecord{}, err
		}
	}
	now := service.timestamp()
	if err := repositories.HealthChecks().SyncHealthCheckTargets(ctx, organizationID, check.ID, targets, now, service.newID); err != nil {
		return repo.HealthCheckRecord{}, err
	}
	return repositories.HealthChecks().FindHealthCheckByID(ctx, organizationID, check.ID)
}

func healthCheckTargetGroupIDs(check repo.HealthCheckRecord) []string {
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, target := range check.Targets {
		if target.ScopeType != "TARGET_GROUP" || target.TargetGroupID == "" || seen[target.TargetGroupID] {
			continue
		}
		seen[target.TargetGroupID] = true
		result = append(result, target.TargetGroupID)
	}
	return result
}

func (service *ControlService) AcknowledgeMonitorAgentConfig(ctx context.Context, organizationID string, monitorID string, configVersion int) error {
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		return repositories.Monitors().RecordMonitorConfigAck(ctx, organizationID, monitorID, configVersion, service.timestamp())
	})
	return mapServiceError(err)
}

func dnsCredentialZoneWritable(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "ACTIVE", "PENDING":
		return true
	default:
		return false
	}
}

func normalizedConfigJSON(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "{}"
	}
	return value
}
