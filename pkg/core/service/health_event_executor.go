package service

import (
	"context"
	"sort"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

// HealthActionExecutor is the extension point for side effects triggered by health evaluation rules.
// DNS failover is the OSS default action; webhook, email, and other action types can register here
// without changing monitor result ingestion.
type HealthActionExecutionInput struct {
	OrganizationID string
	HealthCheck    repo.HealthCheckRecord
	Rule           repo.HealthEvaluationRuleRecord
	Event          repo.HealthEventRecord
	Result         repo.HealthResultRecord
}

type HealthActionExecutor interface {
	Supports(eventType string) bool
	BuildAction(ctx context.Context, repositories repo.Repositories, input HealthActionExecutionInput) (any, bool, error)
	Execute(ctx context.Context, action any) error
}

// HealthActionTypesProvider lets action executors advertise concrete event types
// for API/UI capability discovery without coupling health evaluation to the event implementation.
type HealthActionTypesProvider interface {
	HealthActionTypes() []string
}

type HealthEventExecutionInput = HealthActionExecutionInput
type HealthEventExecutor = HealthActionExecutor

// HealthActionRegistry owns the extension boundary between health evaluation and action side effects.
type HealthActionRegistry struct {
	executors []HealthActionExecutor
}

// NewHealthActionRegistry builds an ordered health action registry.
func NewHealthActionRegistry(executors ...HealthActionExecutor) HealthActionRegistry {
	registry := HealthActionRegistry{}
	for _, executor := range executors {
		registry.Register(executor)
	}
	return registry
}

// Register appends an executor to the registry. Earlier executors win when multiple support a type.
func (registry *HealthActionRegistry) Register(executor HealthActionExecutor) {
	if executor == nil {
		return
	}
	registry.executors = append(registry.executors, executor)
}

// ExecutorForType returns the first executor that supports the normalized event type.
func (registry HealthActionRegistry) ExecutorForType(eventType string) HealthActionExecutor {
	eventType = normalizeHealthActionType(eventType)
	for _, executor := range registry.executors {
		if executor != nil && executor.Supports(eventType) {
			return executor
		}
	}
	return nil
}

// SupportedHealthActionTypes reports the stable set of advertised action types.
func (registry HealthActionRegistry) SupportedHealthActionTypes() []string {
	seen := map[string]bool{}
	for _, executor := range registry.executors {
		provider, ok := executor.(HealthActionTypesProvider)
		if !ok {
			continue
		}
		for _, actionType := range provider.HealthActionTypes() {
			actionType = normalizeHealthActionType(actionType)
			if actionType != "" {
				seen[actionType] = true
			}
		}
	}
	actionTypes := make([]string, 0, len(seen))
	for actionType := range seen {
		actionTypes = append(actionTypes, actionType)
	}
	sort.Strings(actionTypes)
	return actionTypes
}

type pendingHealthAction struct {
	executor HealthActionExecutor
	payload  any
}

func (service *ControlService) healthActionExecutorForType(eventType string) HealthActionExecutor {
	return service.healthActionRegistry.ExecutorForType(eventType)
}

// SupportedHealthActionTypes reports health action types available in this control service.
func (service *ControlService) SupportedHealthActionTypes() []string {
	return service.healthActionRegistry.SupportedHealthActionTypes()
}

func normalizeHealthActionType(actionType string) string {
	return strings.ToUpper(strings.TrimSpace(actionType))
}
