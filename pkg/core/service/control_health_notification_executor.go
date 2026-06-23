package service

import (
	"context"
	"encoding/json"

	"github.com/noxaaa/prism-oss/pkg/core/notification"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

type healthNotificationAction struct {
	channelType string
	configJSON  string
	secret      string
	payload     []byte
}

type healthNotificationSender func(context.Context, string, string, string, []byte) error

type healthSecretDecryptor func(string) (string, error)

func (service *ControlService) defaultHealthActionExecutors() []HealthActionExecutor {
	return []HealthActionExecutor{healthNotificationActionExecutor{
		decryptSecret: service.decryptDNSSecret,
		send:          notification.Send,
	}}
}

type healthNotificationActionExecutor struct {
	decryptSecret healthSecretDecryptor
	send          healthNotificationSender
}

func (executor healthNotificationActionExecutor) Supports(eventType string) bool {
	eventType = normalizeHealthActionType(eventType)
	return eventType == "WEBHOOK" || eventType == "EMAIL"
}

func (executor healthNotificationActionExecutor) HealthActionTypes() []string {
	return []string{"EMAIL", "WEBHOOK"}
}

func (executor healthNotificationActionExecutor) BuildAction(_ context.Context, _ repo.Repositories, input HealthActionExecutionInput) (any, bool, error) {
	secret := ""
	if input.Event.EncryptedSecret != "" {
		if executor.decryptSecret == nil {
			return nil, false, ErrInvalidInput
		}
		var err error
		secret, err = executor.decryptSecret(input.Event.EncryptedSecret)
		if err != nil {
			return nil, false, err
		}
	}
	payload, err := json.Marshal(map[string]any{
		"organization_id":        input.OrganizationID,
		"health_check_id":        input.HealthCheck.ID,
		"health_check_name":      input.HealthCheck.Name,
		"health_evaluation_rule": input.Rule.ID,
		"health_event_id":        input.Event.ID,
		"health_event_type":      input.Event.EventType,
		"status":                 input.Result.Status,
		"health_check_target_id": input.Result.HealthCheckTargetID,
		"target_id":              input.Result.TargetID,
		"monitor_id":             input.Result.MonitorID,
		"latency_ms":             input.Result.LatencyMS,
		"error_message":          input.Result.ErrorMessage,
		"observed_at":            input.Result.ObservedAt,
	})
	if err != nil {
		return nil, false, err
	}
	return healthNotificationAction{channelType: input.Event.EventType, configJSON: input.Event.ConfigJSON, secret: secret, payload: payload}, true, nil
}

func (executor healthNotificationActionExecutor) Execute(ctx context.Context, action any) error {
	notificationAction, ok := action.(healthNotificationAction)
	if !ok {
		return ErrInvalidInput
	}
	send := executor.send
	if send == nil {
		send = notification.Send
	}
	return send(ctx, notificationAction.channelType, notificationAction.configJSON, notificationAction.secret, notificationAction.payload)
}
