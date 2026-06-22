package service

import (
	"context"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func (service *ControlService) buildHealthActions(ctx context.Context, repositories repo.Repositories, organizationID string, results []repo.HealthResultRecord) ([]pendingHealthAction, error) {
	actions := make([]pendingHealthAction, 0)
	seenRulesByCheck := map[string][]repo.HealthEvaluationRuleRecord{}
	seenChecksByID := map[string]repo.HealthCheckRecord{}
	for _, result := range aggregateHealthResultsByCheck(results) {
		rules, ok := seenRulesByCheck[result.HealthCheckID]
		if !ok {
			var err error
			rules, err = repositories.HealthChecks().ListHealthEvaluationRulesByCheck(ctx, organizationID, result.HealthCheckID)
			if err != nil {
				return nil, err
			}
			seenRulesByCheck[result.HealthCheckID] = rules
		}
		if len(rules) == 0 {
			continue
		}
		check, ok := seenChecksByID[result.HealthCheckID]
		if !ok {
			var err error
			check, err = repositories.HealthChecks().FindHealthCheckByID(ctx, organizationID, result.HealthCheckID)
			if err != nil {
				return nil, err
			}
			seenChecksByID[result.HealthCheckID] = check
		}
		evaluationResult, err := service.latestHealthEvaluationResult(ctx, repositories, organizationID, check, result)
		if err != nil {
			return nil, err
		}
		for _, rule := range rules {
			if !rule.Enabled {
				continue
			}
			for _, event := range rule.Events {
				if !event.Enabled {
					continue
				}
				executor := service.healthActionExecutorForType(event.EventType)
				if executor == nil {
					return nil, ErrInvalidInput
				}
				payload, ok, err := executor.BuildAction(ctx, repositories, HealthActionExecutionInput{
					OrganizationID: organizationID,
					HealthCheck:    check,
					Rule:           rule,
					Event:          event,
					Result:         evaluationResult,
				})
				if err != nil {
					return nil, err
				}
				if !ok {
					continue
				}
				actions = append(actions, pendingHealthAction{executor: executor, payload: payload})
			}
		}
	}
	return actions, nil
}

func (service *ControlService) latestHealthEvaluationResult(ctx context.Context, repositories repo.Repositories, organizationID string, check repo.HealthCheckRecord, fallback repo.HealthResultRecord) (repo.HealthResultRecord, error) {
	latest, err := repositories.HealthChecks().ListLatestHealthResultsByCheck(ctx, organizationID, check.ID)
	if err != nil {
		return repo.HealthResultRecord{}, err
	}
	activeMonitorGroupIDs, err := activeMonitorGroupIDSet(ctx, repositories, organizationID)
	if err != nil {
		return repo.HealthResultRecord{}, err
	}
	monitors, err := repositories.Monitors().ListMonitorsByOrganization(ctx, organizationID)
	if err != nil {
		return repo.HealthResultRecord{}, err
	}
	scopedMonitors := make(map[string]bool, len(monitors))
	for _, monitor := range monitors {
		if strings.EqualFold(strings.TrimSpace(monitor.Status), "OFFLINE") {
			continue
		}
		if healthCheckTargetsMonitor(check, monitor, activeMonitorGroupIDs) {
			scopedMonitors[monitor.ID] = true
		}
	}
	scopedTargets := make(map[string]bool, len(check.Targets))
	for _, target := range check.Targets {
		if target.TargetID == "" {
			continue
		}
		scopedTargets[target.ID+"\x00"+target.TargetID] = true
	}
	candidates := make([]repo.HealthResultRecord, 0, len(latest))
	for _, result := range latest {
		status := strings.ToUpper(strings.TrimSpace(result.Status))
		if status != "ONLINE" && status != "OFFLINE" {
			continue
		}
		if !scopedMonitors[result.MonitorID] || !scopedTargets[result.HealthCheckTargetID+"\x00"+result.TargetID] {
			continue
		}
		result.Status = status
		candidates = append(candidates, result)
	}
	if len(candidates) == 0 {
		fallback.Status = strings.ToUpper(strings.TrimSpace(fallback.Status))
		return fallback, nil
	}
	selected := candidates[0]
	for _, result := range candidates {
		if result.Status == "OFFLINE" {
			selected = result
			break
		}
	}
	if selected.Status != "OFFLINE" {
		selected.Status = "ONLINE"
	}
	return selected, nil
}

func aggregateHealthResultsByCheck(results []repo.HealthResultRecord) []repo.HealthResultRecord {
	byCheck := make(map[string]repo.HealthResultRecord)
	order := make([]string, 0)
	for _, result := range results {
		status := strings.ToUpper(strings.TrimSpace(result.Status))
		if status != "ONLINE" && status != "OFFLINE" {
			continue
		}
		result.Status = status
		current, ok := byCheck[result.HealthCheckID]
		if !ok {
			byCheck[result.HealthCheckID] = result
			order = append(order, result.HealthCheckID)
			continue
		}
		if status == "OFFLINE" || current.Status != "OFFLINE" {
			byCheck[result.HealthCheckID] = result
		}
	}
	aggregated := make([]repo.HealthResultRecord, 0, len(order))
	for _, healthCheckID := range order {
		aggregated = append(aggregated, byCheck[healthCheckID])
	}
	return aggregated
}
