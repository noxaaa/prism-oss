package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func (service *ControlService) CreateHealthEvaluationRule(ctx context.Context, identity InternalIdentity, input HealthEvaluationRuleMutationInput) (HealthEvaluationRulePayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionHealthChecksManage)) {
		return HealthEvaluationRulePayload{}, ErrForbidden
	}
	var result HealthEvaluationRulePayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		rule, err := service.createHealthEvaluationRule(ctx, repositories, identity.OrganizationID, input, service.timestamp())
		if err != nil {
			return err
		}
		result = toHealthEvaluationRulePayload(rule)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) createHealthEvaluationRule(ctx context.Context, repositories repo.Repositories, organizationID string, input HealthEvaluationRuleMutationInput, now string) (repo.HealthEvaluationRuleRecord, error) {
	healthCheckID := strings.TrimSpace(input.HealthCheckID)
	name := strings.TrimSpace(input.Name)
	if healthCheckID == "" || name == "" || len(name) > 120 || len(input.Events) == 0 {
		return repo.HealthEvaluationRuleRecord{}, ErrInvalidInput
	}
	if _, err := repositories.HealthChecks().FindHealthCheckByID(ctx, organizationID, healthCheckID); err != nil {
		return repo.HealthEvaluationRuleRecord{}, err
	}
	expressionJSON, err := normalizeHealthActionConfigJSON(input.ExpressionJSON, `{"mode":"latest_result"}`)
	if err != nil {
		return repo.HealthEvaluationRuleRecord{}, err
	}
	rule := repo.HealthEvaluationRuleRecord{
		ID:             service.newID(),
		OrganizationID: organizationID,
		HealthCheckID:  healthCheckID,
		Name:           name,
		Enabled:        input.Enabled,
		ExpressionJSON: expressionJSON,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	events := make([]repo.HealthEventRecord, 0, len(input.Events))
	for _, eventInput := range input.Events {
		eventType := normalizeHealthActionType(eventInput.EventType)
		if eventType == "" || service.healthActionExecutorForType(eventType) == nil {
			return repo.HealthEvaluationRuleRecord{}, ErrInvalidInput
		}
		configJSON, err := normalizeHealthActionConfigJSON(eventInput.ConfigJSON, `{}`)
		if err != nil {
			return repo.HealthEvaluationRuleRecord{}, err
		}
		events = append(events, repo.HealthEventRecord{
			ID:                     service.newID(),
			OrganizationID:         organizationID,
			HealthEvaluationRuleID: rule.ID,
			EventType:              eventType,
			ConfigJSON:             configJSON,
			Enabled:                eventInput.Enabled,
			CreatedAt:              now,
			UpdatedAt:              now,
		})
	}
	if err := repositories.HealthChecks().CreateHealthEvaluationRule(ctx, rule, events); err != nil {
		return repo.HealthEvaluationRuleRecord{}, err
	}
	rule.Events = events
	return rule, nil
}

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
		evaluation, err := service.latestHealthEvaluation(ctx, repositories, organizationID, check, result)
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
					Result:         evaluation.Result,
					Results:        evaluation.Results,
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

type latestHealthEvaluation struct {
	Result  repo.HealthResultRecord
	Results []repo.HealthResultRecord
}

func (service *ControlService) latestHealthEvaluation(ctx context.Context, repositories repo.Repositories, organizationID string, check repo.HealthCheckRecord, fallback repo.HealthResultRecord) (latestHealthEvaluation, error) {
	latest, err := repositories.HealthChecks().ListLatestHealthResultsByCheck(ctx, organizationID, check.ID)
	if err != nil {
		return latestHealthEvaluation{}, err
	}
	activeMonitorGroupIDs, err := activeMonitorGroupIDSet(ctx, repositories, organizationID)
	if err != nil {
		return latestHealthEvaluation{}, err
	}
	monitors, err := repositories.Monitors().ListMonitorsByOrganization(ctx, organizationID)
	if err != nil {
		return latestHealthEvaluation{}, err
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
		return latestHealthEvaluation{Result: fallback, Results: []repo.HealthResultRecord{fallback}}, nil
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
	return latestHealthEvaluation{Result: selected, Results: candidates}, nil
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

func normalizeHealthActionConfigJSON(raw string, fallback string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = fallback
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return "", ErrInvalidInput
	}
	if decoded == nil {
		return "", ErrInvalidInput
	}
	return raw, nil
}

func toHealthEvaluationRulePayload(rule repo.HealthEvaluationRuleRecord) HealthEvaluationRulePayload {
	events := make([]HealthEventPayload, 0, len(rule.Events))
	for _, event := range rule.Events {
		events = append(events, HealthEventPayload{
			ID:         event.ID,
			EventType:  event.EventType,
			ConfigJSON: event.ConfigJSON,
			Enabled:    event.Enabled,
			CreatedAt:  event.CreatedAt,
			UpdatedAt:  event.UpdatedAt,
		})
	}
	return HealthEvaluationRulePayload{
		ID:             rule.ID,
		HealthCheckID:  rule.HealthCheckID,
		Name:           rule.Name,
		Enabled:        rule.Enabled,
		ExpressionJSON: rule.ExpressionJSON,
		Events:         events,
		CreatedAt:      rule.CreatedAt,
		UpdatedAt:      rule.UpdatedAt,
	}
}
