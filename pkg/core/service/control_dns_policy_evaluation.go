package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/notification"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

type dnsPolicyEvaluation struct {
	Status         string
	ErrorMessage   string
	Diagnostics    []DNSDiagnosticPayload
	Values         []string
	ApplyProvider  bool
	ActiveInstance *repo.DNSInstanceRecord
}

type dnsPolicyPreviousState struct {
	ActiveInstanceID string
	Status           string
	ValuesJSON       string
}

func (service *ControlService) evaluateDNSManagedRecord(ctx context.Context, repositories repo.Repositories, organizationID string, record repo.DNSManagedRecordRecord) (dnsPolicyEvaluation, error) {
	instances := make([]repo.DNSInstanceRecord, 0)
	for _, instance := range record.Instances {
		if instance.Enabled && instance.DeletedAt == "" {
			instances = append(instances, instance)
		}
	}
	sort.Slice(instances, func(left, right int) bool {
		if instances[left].Priority == instances[right].Priority {
			return instances[left].ID < instances[right].ID
		}
		return instances[left].Priority < instances[right].Priority
	})
	var selected *repo.DNSInstanceRecord
	diagnostics := []DNSDiagnosticPayload{}
	for index := range instances {
		instance := instances[index]
		context, err := service.dnsConditionContext(ctx, repositories, organizationID, instance)
		if err != nil {
			return dnsPolicyEvaluation{}, err
		}
		matched, conditionDiagnostics := evaluateDNSCondition(instance.ConditionJSON, context)
		diagnostics = append(diagnostics, conditionDiagnostics...)
		if !matched {
			continue
		}
		if selected != nil && selected.Priority == instance.Priority {
			return dnsPolicyEvaluation{Status: "CONFLICT", Diagnostics: append(diagnostics, DNSDiagnosticPayload{Code: "AMBIGUOUS_PRIORITY", Message: "Multiple DNS instances matched at the same priority."})}, nil
		}
		if selected != nil && selected.Priority < instance.Priority {
			break
		}
		selected = &instances[index]
	}
	if selected == nil {
		return dnsPolicyEvaluation{Status: "NO_MATCH", Values: parseStringListJSON(record.LastAppliedValuesJSON), Diagnostics: append(diagnostics, DNSDiagnosticPayload{Code: "NO_MATCHED_INSTANCE", Message: "No DNS instance matched this record."})}, nil
	}
	values, actionDiagnostics, err := service.evaluateDNSAction(ctx, repositories, organizationID, record, *selected, map[string]bool{}, 0)
	diagnostics = append(diagnostics, actionDiagnostics...)
	if err != nil {
		return dnsPolicyEvaluation{Status: "FAILED", ErrorMessage: err.Error(), Diagnostics: diagnostics, ActiveInstance: selected}, nil
	}
	previous := stringListJSON(parseStringListJSON(record.LastAppliedValuesJSON))
	next := stringListJSON(values)
	return dnsPolicyEvaluation{Status: "APPLIED", Values: values, ApplyProvider: previous != next || strings.ToUpper(record.LastEvaluationStatus) != "APPLIED", Diagnostics: diagnostics, ActiveInstance: selected}, nil
}

type dnsConditionContext struct {
	TotalNodes   int
	OnlineNodes  int
	OfflineNodes int
}

func (service *ControlService) dnsConditionContext(ctx context.Context, repositories repo.Repositories, organizationID string, instance repo.DNSInstanceRecord) (dnsConditionContext, error) {
	nodeGroupIDs := parseStringListJSON(instance.NodeGroupIDsJSON)
	nodes, err := repositories.Nodes().ListNodesByOrganization(ctx, organizationID)
	if err != nil {
		return dnsConditionContext{}, err
	}
	groupSet := map[string]bool{}
	for _, groupID := range nodeGroupIDs {
		groupSet[groupID] = true
	}
	result := dnsConditionContext{}
	for _, node := range nodes {
		if !nodeMatchesGroupSet(node, groupSet) {
			continue
		}
		result.TotalNodes++
		switch strings.ToUpper(strings.TrimSpace(node.Status)) {
		case "ONLINE":
			result.OnlineNodes++
		case "OFFLINE":
			result.OfflineNodes++
		}
	}
	return result, nil
}

func evaluateDNSCondition(raw string, context dnsConditionContext) (bool, []DNSDiagnosticPayload) {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "{}" {
		return true, nil
	}
	var expression map[string]any
	if err := json.Unmarshal([]byte(raw), &expression); err != nil {
		return false, []DNSDiagnosticPayload{{Code: "INVALID_CONDITION", Message: "Condition JSON is invalid."}}
	}
	if context.TotalNodes == 0 {
		return false, []DNSDiagnosticPayload{{Code: "NO_NODE_GROUP_MEMBERS", Message: "Bound node groups have no nodes."}}
	}
	matched, diagnostics := evalDNSConditionNode(expression, context)
	return matched, diagnostics
}

func evalDNSConditionNode(node map[string]any, context dnsConditionContext) (bool, []DNSDiagnosticPayload) {
	op := strings.ToUpper(strings.TrimSpace(stringValue(node["op"])))
	if op == "AND" || op == "OR" {
		children, _ := node["children"].([]any)
		if len(children) == 0 {
			return false, []DNSDiagnosticPayload{{Code: "EMPTY_CONDITION_GROUP", Message: "Condition group has no children."}}
		}
		diagnostics := []DNSDiagnosticPayload{}
		if op == "AND" {
			for _, child := range children {
				childMap, ok := child.(map[string]any)
				if !ok {
					return false, append(diagnostics, DNSDiagnosticPayload{Code: "INVALID_CONDITION", Message: "Condition child is invalid."})
				}
				matched, childDiagnostics := evalDNSConditionNode(childMap, context)
				diagnostics = append(diagnostics, childDiagnostics...)
				if !matched {
					return false, diagnostics
				}
			}
			return true, diagnostics
		}
		for _, child := range children {
			childMap, ok := child.(map[string]any)
			if !ok {
				return false, append(diagnostics, DNSDiagnosticPayload{Code: "INVALID_CONDITION", Message: "Condition child is invalid."})
			}
			matched, childDiagnostics := evalDNSConditionNode(childMap, context)
			diagnostics = append(diagnostics, childDiagnostics...)
			if matched {
				return true, diagnostics
			}
		}
		return false, diagnostics
	}
	metric := strings.ToLower(strings.TrimSpace(stringValue(node["metric"])))
	comparator := strings.TrimSpace(stringValue(node["comparator"]))
	actual, ok := dnsConditionMetric(metric, context)
	if !ok {
		return false, []DNSDiagnosticPayload{{Code: "UNSUPPORTED_CONDITION_METRIC", Message: "Condition metric is not supported.", Details: map[string]any{"metric": metric}}}
	}
	expected, ok := numericValue(node["value"])
	if !ok {
		return false, []DNSDiagnosticPayload{{Code: "INVALID_CONDITION_VALUE", Message: "Condition value must be numeric."}}
	}
	switch comparator {
	case ">":
		return actual > expected, nil
	case "<":
		return actual < expected, nil
	case ">=":
		return actual >= expected, nil
	case "<=":
		return actual <= expected, nil
	case "=":
		return actual == expected, nil
	default:
		return false, []DNSDiagnosticPayload{{Code: "UNSUPPORTED_CONDITION_COMPARATOR", Message: "Condition comparator is not supported."}}
	}
}

func dnsConditionMetric(metric string, context dnsConditionContext) (float64, bool) {
	switch metric {
	case "offline_node_count":
		return float64(context.OfflineNodes), true
	case "online_node_count":
		return float64(context.OnlineNodes), true
	case "offline_node_percent":
		return float64(context.OfflineNodes) * 100 / float64(context.TotalNodes), true
	case "online_node_percent":
		return float64(context.OnlineNodes) * 100 / float64(context.TotalNodes), true
	default:
		return 0, false
	}
}

func (service *ControlService) evaluateDNSAction(ctx context.Context, repositories repo.Repositories, organizationID string, record repo.DNSManagedRecordRecord, instance repo.DNSInstanceRecord, seen map[string]bool, depth int) ([]string, []DNSDiagnosticPayload, error) {
	if depth > 8 {
		return nil, []DNSDiagnosticPayload{{Code: "INSTANCE_REFERENCE_DEPTH_EXCEEDED", Message: "DNS instance reference depth exceeded."}}, ErrInvalidInput
	}
	if seen[instance.ID] {
		return nil, []DNSDiagnosticPayload{{Code: "INSTANCE_REFERENCE_CYCLE", Message: "DNS instance references form a cycle."}}, ErrInvalidInput
	}
	seen[instance.ID] = true
	var action dnsPolicyAction
	if err := json.Unmarshal([]byte(instance.ActionJSON), &action); err != nil {
		return nil, []DNSDiagnosticPayload{{Code: "INVALID_ACTION", Message: "DNS action JSON is invalid."}}, err
	}
	switch normalizeDNSActionType(action.Type) {
	case "ROTATE_ONLINE_NODES":
		values, diagnostics, err := service.dnsOnlineNodeValues(ctx, repositories, organizationID, record.RecordType, instance)
		if err != nil {
			return nil, diagnostics, err
		}
		return limitDNSAnswers(values, instance.AnswerCount), diagnostics, nil
	case "SET_STATIC_A", "SET_STATIC_AAAA", "SET_STATIC_ADDRESSES":
		values := normalizeDNSValues(action.Values)
		if len(values) == 0 || !dnsValuesMatchRecordType(values, record.RecordType) {
			return nil, []DNSDiagnosticPayload{{Code: "INVALID_STATIC_DNS_VALUES", Message: "Static DNS values are invalid for this record type."}}, ErrInvalidInput
		}
		return values, nil, nil
	case "SET_STATIC_CNAME":
		value := strings.Trim(strings.TrimSpace(action.Value), ".")
		if value == "" {
			return nil, nil, ErrInvalidInput
		}
		return []string{value}, nil, nil
	case "USE_INSTANCE_OUTPUT":
		target, err := repositories.DNSRecords().FindDNSInstanceByID(ctx, organizationID, action.InstanceID)
		if err != nil {
			return nil, nil, err
		}
		if !target.Enabled {
			return nil, []DNSDiagnosticPayload{{Code: "REFERENCED_INSTANCE_DISABLED", Message: "Referenced DNS instance is disabled."}}, ErrInvalidInput
		}
		targetRecord, err := repositories.DNSRecords().FindDNSManagedRecordByID(ctx, organizationID, target.ManagedRecordID)
		if err != nil {
			return nil, nil, err
		}
		if strings.ToUpper(strings.TrimSpace(targetRecord.LastEvaluationStatus)) == "DELETE_PENDING" {
			return nil, []DNSDiagnosticPayload{{Code: "REFERENCED_INSTANCE_DELETE_PENDING", Message: "Referenced DNS instance output is being deleted."}}, ErrInvalidInput
		}
		if targetRecord.RecordType != record.RecordType {
			return nil, []DNSDiagnosticPayload{{Code: "REFERENCED_INSTANCE_RECORD_TYPE_MISMATCH", Message: "Referenced DNS instance output is not compatible with this managed record type."}}, ErrInvalidInput
		}
		return service.evaluateDNSAction(ctx, repositories, organizationID, record, target, seen, depth+1)
	default:
		return nil, []DNSDiagnosticPayload{{Code: "UNSUPPORTED_ACTION", Message: "DNS action is not supported."}}, ErrInvalidInput
	}
}

func (service *ControlService) dnsOnlineNodeValues(ctx context.Context, repositories repo.Repositories, organizationID string, recordType string, instance repo.DNSInstanceRecord) ([]string, []DNSDiagnosticPayload, error) {
	nodes, err := repositories.Nodes().ListNodesByOrganization(ctx, organizationID)
	if err != nil {
		return nil, nil, err
	}
	groupSet := map[string]bool{}
	for _, groupID := range parseStringListJSON(instance.NodeGroupIDsJSON) {
		groupSet[groupID] = true
	}
	values := []string{}
	for _, node := range nodes {
		if strings.ToUpper(strings.TrimSpace(node.Status)) != "ONLINE" || !nodeMatchesGroupSet(node, groupSet) {
			continue
		}
		for _, address := range node.DNSPublishAddresses {
			if !address.Enabled || address.AddressType != recordType {
				continue
			}
			values = append(values, address.Address)
		}
	}
	values = normalizeDNSValues(values)
	if len(values) == 0 {
		return nil, []DNSDiagnosticPayload{{Code: "NO_ONLINE_NODE_ADDRESSES", Message: "No online nodes have usable DNS publish addresses."}}, ErrInvalidInput
	}
	return values, nil, nil
}

type preparedDNSNotification struct {
	channel   repo.NotificationChannelRecord
	delivery  repo.NotificationDeliveryRecord
	payload   []byte
	createdAt string
}

func (service *ControlService) resolveDNSNotificationChannels(ctx context.Context, repositories repo.Repositories, organizationID string, evaluation dnsPolicyEvaluation) ([]repo.NotificationChannelRecord, error) {
	if evaluation.ActiveInstance == nil {
		return nil, nil
	}
	channelIDs := parseStringListJSON(evaluation.ActiveInstance.NotificationChannelIDsJSON)
	if len(channelIDs) == 0 {
		return nil, nil
	}
	channels := make([]repo.NotificationChannelRecord, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		channel, err := repositories.DNSRecords().FindNotificationChannelByID(ctx, organizationID, channelID)
		if errors.Is(err, repo.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !channel.Enabled {
			continue
		}
		channels = append(channels, channel)
	}
	return channels, nil
}

func (service *ControlService) prepareDNSNotifications(record repo.DNSManagedRecordRecord, evaluation dnsPolicyEvaluation, previous dnsPolicyPreviousState, channels []repo.NotificationChannelRecord, now string) []preparedDNSNotification {
	if evaluation.ActiveInstance == nil || len(channels) == 0 {
		return nil
	}
	eventType := dnsPolicyNotificationEventType(evaluation, previous)
	if eventType == "" {
		return nil
	}
	payload := map[string]any{
		"record_id":   record.ID,
		"record_name": record.RecordName,
		"record_type": record.RecordType,
		"instance_id": evaluation.ActiveInstance.ID,
		"status":      evaluation.Status,
		"values":      evaluation.Values,
		"event_type":  eventType,
	}
	payloadJSON, _ := json.Marshal(payload)
	notifications := make([]preparedDNSNotification, 0, len(channels))
	for _, channel := range channels {
		notifications = append(notifications, preparedDNSNotification{
			channel:   channel,
			payload:   payloadJSON,
			createdAt: now,
			delivery: repo.NotificationDeliveryRecord{
				ID:                 service.newID(),
				OrganizationID:     record.OrganizationID,
				ChannelID:          channel.ID,
				DNSManagedRecordID: record.ID,
				DNSInstanceID:      evaluation.ActiveInstance.ID,
				EventType:          eventType,
				PayloadJSON:        string(payloadJSON),
				CreatedAt:          now,
			},
		})
	}
	return notifications
}

func (service *ControlService) dispatchPreparedDNSNotificationsBestEffort(_ context.Context, notifications []preparedDNSNotification) {
	if len(notifications) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), dnsBestEffortEvaluationTimeout)
	defer cancel()
	for _, prepared := range notifications {
		delivery := prepared.delivery
		delivery.Status = "SUCCEEDED"
		delivery.DeliveredAt = prepared.createdAt
		if err := service.sendNotification(ctx, prepared.channel, prepared.payload); err != nil {
			delivery.Status = "FAILED"
			delivery.ErrorMessage = err.Error()
			delivery.DeliveredAt = ""
		}
		_ = service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
			return repositories.DNSRecords().CreateNotificationDelivery(ctx, delivery)
		})
	}
}

func (service *ControlService) sendNotification(ctx context.Context, channel repo.NotificationChannelRecord, payload []byte) error {
	secret := ""
	if channel.EncryptedSecret != "" {
		var err error
		secret, err = service.decryptDNSSecret(channel.EncryptedSecret)
		if err != nil {
			return err
		}
	}
	return notification.Send(ctx, channel.ChannelType, channel.ConfigJSON, secret, payload)
}

func dnsPolicyNotificationEventType(evaluation dnsPolicyEvaluation, previous dnsPolicyPreviousState) string {
	activeInstanceID := ""
	if evaluation.ActiveInstance != nil {
		activeInstanceID = evaluation.ActiveInstance.ID
	}
	if activeInstanceID != previous.ActiveInstanceID {
		return "DNS_ACTIVE_INSTANCE_CHANGED"
	}
	if evaluation.Status == "FAILED" && previous.Status != "FAILED" {
		return "DNS_POLICY_FAILED"
	}
	if evaluation.Status != "FAILED" && previous.Status == "FAILED" {
		return "DNS_POLICY_RECOVERED"
	}
	if stringListJSON(evaluation.Values) != previous.ValuesJSON {
		return "DNS_OUTPUT_CHANGED"
	}
	return ""
}
