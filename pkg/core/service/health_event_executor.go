package service

import (
	"context"
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

type HealthEventExecutionInput = HealthActionExecutionInput
type HealthEventExecutor = HealthActionExecutor

type pendingHealthAction struct {
	executor HealthActionExecutor
	payload  any
}

func (service *ControlService) healthActionExecutorForType(eventType string) HealthActionExecutor {
	eventType = strings.ToUpper(strings.TrimSpace(eventType))
	for _, executor := range service.healthActionExecutors {
		if executor != nil && executor.Supports(eventType) {
			return executor
		}
	}
	return nil
}
