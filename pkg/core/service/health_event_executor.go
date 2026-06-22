package service

import (
	"context"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

type HealthEventExecutionInput struct {
	OrganizationID string
	Event          repo.HealthEventRecord
	Result         repo.HealthResultRecord
}

type HealthEventExecutor interface {
	Supports(eventType string) bool
	BuildAction(ctx context.Context, repositories repo.Repositories, input HealthEventExecutionInput) (any, bool, error)
	Execute(ctx context.Context, action any) error
}

type healthEventAction struct {
	executor HealthEventExecutor
	payload  any
}

func (service *ControlService) healthEventExecutorForType(eventType string) HealthEventExecutor {
	eventType = strings.ToUpper(strings.TrimSpace(eventType))
	for _, executor := range service.healthEventExecutors {
		if executor != nil && executor.Supports(eventType) {
			return executor
		}
	}
	return nil
}
