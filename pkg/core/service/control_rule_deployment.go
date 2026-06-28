package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

const (
	FailurePolicyKeepEnabled               = "KEEP_ENABLED"
	FailurePolicyDisableWhenAllNodesFailed = "DISABLE_WHEN_ALL_NODES_FAILED"
	DataplanePreferenceAuto                = "AUTO"
	DataplanePreferenceNative              = "NATIVE"
	DataplanePreferenceHAProxy             = "HAPROXY"
	DataplanePreferenceNFTables            = "NFTABLES"
	RuleDeploymentAggregateDisabled        = "DISABLED"
	RuleDeploymentStatusPending            = "PENDING"
	RuleDeploymentStatusApplied            = "APPLIED"
	RuleDeploymentStatusFailed             = "FAILED"
	RuleDeploymentAggregateNoNodes         = "NO_NODES"
	RuleDeploymentAggregateDeployFailed    = "DEPLOY_FAILED"
)

func defaultFailurePolicy(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return FailurePolicyKeepEnabled
	}
	return value
}

func defaultDataplanePreference(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	switch value {
	case DataplanePreferenceNative, DataplanePreferenceHAProxy, DataplanePreferenceNFTables:
		return value
	default:
		return DataplanePreferenceAuto
	}
}

func normalizeDataplanePreferenceForMutation(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if err := validateDataplanePreference(value); err != nil {
		return "", err
	}
	if value == "" {
		return DataplanePreferenceAuto, nil
	}
	return value, nil
}

func validateDataplanePreference(value string) error {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "":
		return nil
	case DataplanePreferenceAuto, DataplanePreferenceNative, DataplanePreferenceHAProxy, DataplanePreferenceNFTables:
		return nil
	default:
		return validationFieldError("dataplane_preference", "Unsupported rule dataplane preference.", map[string]any{
			"actual": value,
		})
	}
}

func validateFailurePolicy(value string) error {
	switch defaultFailurePolicy(value) {
	case FailurePolicyKeepEnabled, FailurePolicyDisableWhenAllNodesFailed:
		return nil
	default:
		return validationFieldError("failure_policy", "Unsupported rule failure policy.", map[string]any{
			"actual": value,
		})
	}
}

func nodeConfigBackoffActive(node repo.NodeRecord, now time.Time) bool {
	if node.ConfigStatus != "FAILED" || node.ConfigStatusConfigVersion != node.DesiredConfigVersion || strings.TrimSpace(node.ConfigNextRetryAt) == "" {
		return false
	}
	nextRetryAt, err := parseNodeConfigRetryAt(node.ConfigNextRetryAt)
	if err != nil {
		return false
	}
	return now.UTC().Before(nextRetryAt)
}

func parseNodeConfigRetryAt(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	formats := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999-07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05.999999-07",
		"2006-01-02 15:04:05-07",
	}
	var lastErr error
	for _, format := range formats {
		parsed, err := time.Parse(format, value)
		if err == nil {
			return parsed, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}

func nodeConfigFailedAck(node repo.NodeRecord, configVersion int, errorMessage string, now time.Time) repo.NodeConfigAckRecord {
	retryCount := 1
	if node.ConfigStatus == "FAILED" && node.ConfigStatusConfigVersion == configVersion {
		retryCount = node.ConfigRetryCount + 1
	}
	return repo.NodeConfigAckRecord{
		ConfigVersion: configVersion,
		Status:        "FAILED",
		ErrorMessage:  errorMessage,
		RetryCount:    retryCount,
		NextRetryAt:   now.UTC().Add(configRetryBackoff(retryCount)).Format(time.RFC3339Nano),
	}
}

func configRetryBackoff(retryCount int) time.Duration {
	switch {
	case retryCount <= 1:
		return 15 * time.Second
	case retryCount == 2:
		return 30 * time.Second
	case retryCount == 3:
		return time.Minute
	case retryCount == 4:
		return 2 * time.Minute
	default:
		return 5 * time.Minute
	}
}

func ruleDeploymentPayload(rule repo.RuleRecord, nodes []repo.NodeRecord, deployments []repo.RuleDeploymentRecord, includeNodeDetails bool) RuleDeploymentPayload {
	if !rule.Enabled {
		return RuleDeploymentPayload{Status: RuleDeploymentAggregateDisabled}
	}
	deploymentsByNode := make(map[string]repo.RuleDeploymentRecord, len(deployments))
	for _, deployment := range deployments {
		if deployment.RuleID == rule.ID {
			deploymentsByNode[deployment.NodeID] = deployment
		}
	}
	payload := RuleDeploymentPayload{
		Status: RuleDeploymentAggregateNoNodes,
		Total:  len(nodes),
		Nodes:  make([]RuleDeploymentNodePayload, 0, len(nodes)),
	}
	for _, node := range nodes {
		nodePayload := RuleDeploymentNodePayload{
			NodeID:   node.ID,
			NodeName: node.Name,
			Status:   RuleDeploymentStatusPending,
		}
		if deployment, ok := deploymentsByNode[node.ID]; ok {
			nodePayload.Status = deployment.Status
			nodePayload.ErrorCode = deployment.ErrorCode
			nodePayload.ErrorMessage = deployment.ErrorMessage
			nodePayload.Protocol = deployment.Protocol
			nodePayload.ListenIP = deployment.ListenIP
			nodePayload.Port = deployment.Port
			nodePayload.ExpectedDataplane = deployment.ExpectedDataplane
			nodePayload.ActualDataplane = deployment.ActualDataplane
			nodePayload.Owner = deployment.Owner
			nodePayload.DriftStatus = deployment.DriftStatus
			nodePayload.ExternalResource = deployment.ExternalResource
			nodePayload.UpdatedAt = deployment.UpdatedAt
		}
		switch nodePayload.Status {
		case RuleDeploymentStatusApplied:
			payload.Applied++
		case RuleDeploymentStatusFailed:
			payload.Failed++
		default:
			payload.Pending++
		}
		if includeNodeDetails {
			payload.Nodes = append(payload.Nodes, nodePayload)
		}
	}
	sort.SliceStable(payload.Nodes, func(i int, j int) bool {
		if payload.Nodes[i].Status != payload.Nodes[j].Status {
			return deploymentStatusRank(payload.Nodes[i].Status) < deploymentStatusRank(payload.Nodes[j].Status)
		}
		return payload.Nodes[i].NodeName < payload.Nodes[j].NodeName
	})
	switch {
	case payload.Total == 0:
		payload.Status = RuleDeploymentAggregateNoNodes
	case payload.Failed > 0:
		payload.Status = RuleDeploymentAggregateDeployFailed
	case payload.Pending > 0:
		payload.Status = RuleDeploymentStatusPending
	default:
		payload.Status = RuleDeploymentStatusApplied
	}
	return payload
}

func pendingDeploymentsForNodes(nodes []repo.NodeRecord) []repo.RuleDeploymentPendingRecord {
	deployments := make([]repo.RuleDeploymentPendingRecord, 0, len(nodes))
	for _, node := range nodes {
		if strings.TrimSpace(node.ID) == "" {
			continue
		}
		deployments = append(deployments, repo.RuleDeploymentPendingRecord{
			NodeID:        node.ID,
			ConfigVersion: node.DesiredConfigVersion,
		})
	}
	sort.SliceStable(deployments, func(i int, j int) bool {
		return deployments[i].NodeID < deployments[j].NodeID
	})
	return deployments
}

func deploymentStatusRank(status string) int {
	switch status {
	case RuleDeploymentStatusFailed:
		return 0
	case RuleDeploymentStatusPending:
		return 1
	case RuleDeploymentStatusApplied:
		return 2
	default:
		return 3
	}
}

func syncRuleDeploymentPending(ctx context.Context, repositories repo.Repositories, organizationID string, rule repo.RuleRecord, now string, nextID func() string) error {
	if !rule.Enabled {
		return repositories.Rules().DeleteRuleDeployments(ctx, organizationID, rule.ID)
	}
	nodes, err := nodesInGroup(ctx, repositories, organizationID, rule.Binding.NodeGroupID)
	if err != nil {
		return err
	}
	return repositories.Rules().ReplaceRuleDeploymentPending(ctx, organizationID, rule, pendingDeploymentsForNodes(nodes), now, nextID)
}

func syncRuleDeploymentsForNodeMembershipChange(ctx context.Context, repositories repo.Repositories, organizationID string, node repo.NodeRecord, previousGroupIDs []string, nextGroupIDs []string, now string, nextID func() string) error {
	if strings.TrimSpace(node.ID) == "" {
		return nil
	}
	rules, err := repositories.Rules().ListRulesByOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	previous := stringSet(previousGroupIDs)
	next := stringSet(nextGroupIDs)
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		_, wasInGroup := previous[rule.Binding.NodeGroupID]
		_, isInGroup := next[rule.Binding.NodeGroupID]
		switch {
		case wasInGroup && !isInGroup:
			if err := repositories.Rules().DeleteRuleDeploymentForNode(ctx, organizationID, rule.ID, node.ID); err != nil {
				return err
			}
		case !wasInGroup && isInGroup:
			if err := repositories.Rules().UpsertRuleDeploymentPending(ctx, organizationID, rule, repo.RuleDeploymentPendingRecord{
				NodeID:        node.ID,
				ConfigVersion: node.DesiredConfigVersion,
			}, now, nextID); err != nil {
				return err
			}
		}
	}
	return nil
}

func syncRuleDeploymentsForNodeConfigChange(ctx context.Context, repositories repo.Repositories, organizationID string, node repo.NodeRecord, now string, nextID func() string) error {
	if strings.TrimSpace(node.ID) == "" {
		return nil
	}
	rules, err := repositories.Rules().ListRulesByOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	nodeGroups := stringSet(node.GroupIDs)
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if _, ok := nodeGroups[rule.Binding.NodeGroupID]; !ok {
			continue
		}
		if err := repositories.Rules().UpsertRuleDeploymentPending(ctx, organizationID, rule, repo.RuleDeploymentPendingRecord{
			NodeID:        node.ID,
			ConfigVersion: node.DesiredConfigVersion,
		}, now, nextID); err != nil {
			return err
		}
	}
	return nil
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	return set
}

func ruleDeploymentAppliedRecordsForNode(ctx context.Context, repositories repo.Repositories, organizationID string, node repo.NodeRecord, rules []repo.RuleRecord, protocolVersion agent.ProtocolVersion, protocolKnown bool, resolvedDataplanes map[string]string) ([]repo.RuleDeploymentAppliedRecord, bool, error) {
	candidateRules := executableRulesForNode(node, rules)
	targets, err := repositories.Targets().ListTargetsByOrganization(ctx, organizationID)
	if err != nil {
		return nil, false, err
	}
	targetGroups, err := repositories.TargetGroups().ListTargetGroupsByOrganization(ctx, organizationID)
	if err != nil {
		return nil, false, err
	}
	ruleConfigs, err := toRuleConfigs(candidateRules, targets, targetGroups)
	if err != nil {
		return nil, false, err
	}
	compiled, err := BasicAgentConfigCompiler{}.Compile(ctx, AgentConfigInput{
		NodeID:                  node.ID,
		NodeGroups:              node.GroupIDs,
		AgentProtocolVersion:    protocolVersion,
		AgentProtocolKnown:      protocolKnown,
		DataplaneMode:           node.DataplaneMode,
		DataplaneInstanceID:     node.DataplaneInstanceID,
		DataplaneConflictPolicy: node.DataplaneConflictPolicy,
		Rules:                   ruleConfigs,
	})
	if err != nil {
		return nil, false, err
	}
	applied := make([]repo.RuleDeploymentAppliedRecord, 0, len(compiled.Rules))
	appliedRuleIDs := make(map[string]bool)
	for _, rule := range compiled.Rules {
		if appliedRuleIDs[rule.ID] {
			continue
		}
		appliedRuleIDs[rule.ID] = true
		actualDataplane := defaultDataplanePreference(rule.Dataplane)
		if resolved := strings.TrimSpace(resolvedDataplanes[rule.ID]); resolved != "" {
			actualDataplane = defaultDataplanePreference(resolved)
		}
		applied = append(applied, repo.RuleDeploymentAppliedRecord{
			RuleID:            rule.ID,
			RuleConfigVersion: rule.ConfigVersion,
			ExpectedDataplane: defaultDataplanePreference(rule.Dataplane),
			ActualDataplane:   actualDataplane,
		})
	}
	return applied, logicalRuleIDCount(compiled.Rules) == logicalRuleIDCount(ruleConfigs), nil
}

func logicalRuleIDCount(rules []RuleConfig) int {
	seen := make(map[string]bool, len(rules))
	for _, rule := range rules {
		if strings.TrimSpace(rule.ID) == "" {
			continue
		}
		seen[rule.ID] = true
	}
	return len(seen)
}

func ruleDeploymentFailuresFromApplyErrors(rulesByID map[string]repo.RuleRecord, errors []ConfigApplyErrorInput) []repo.RuleDeploymentFailureRecord {
	failuresByRule := make(map[string]repo.RuleDeploymentFailureRecord)
	order := make([]string, 0)
	for _, applyErr := range errors {
		for _, ruleID := range applyErr.RuleIDs {
			ruleID = strings.TrimSpace(ruleID)
			rule, ok := rulesByID[ruleID]
			if !ok {
				continue
			}
			if _, exists := failuresByRule[ruleID]; !exists {
				order = append(order, ruleID)
			}
			failuresByRule[ruleID] = repo.RuleDeploymentFailureRecord{
				RuleID:            ruleID,
				RuleConfigVersion: rule.ConfigVersion,
				ErrorCode:         truncateString(strings.TrimSpace(applyErr.Code), 120),
				ErrorMessage:      truncateString(strings.TrimSpace(applyErr.Message), 1000),
				Protocol:          string(applyErr.Protocol),
				ListenIP:          truncateString(strings.TrimSpace(applyErr.ListenIP), 255),
				Port:              applyErr.Port,
				ExpectedDataplane: truncateString(defaultDataplanePreference(rule.DataplanePreference), 40),
				ActualDataplane:   truncateString(defaultDataplanePreference(applyErr.Dataplane), 40),
				Owner:             truncateString(strings.TrimSpace(applyErr.Owner), 255),
				DriftStatus:       truncateString(strings.TrimSpace(applyErr.DriftStatus), 120),
				ExternalResource:  truncateString(strings.TrimSpace(applyErr.ExternalResource), 255),
			}
		}
	}
	failures := make([]repo.RuleDeploymentFailureRecord, 0, len(failuresByRule))
	for _, ruleID := range order {
		failures = append(failures, failuresByRule[ruleID])
	}
	return failures
}

func truncateString(value string, maxLength int) string {
	if maxLength <= 0 || len(value) <= maxLength {
		return value
	}
	return value[:maxLength]
}

func (service *ControlService) applyRuleFailurePolicies(ctx context.Context, repositories repo.Repositories, organizationID string, failedRuleIDs []string, now string) error {
	if len(failedRuleIDs) == 0 {
		return nil
	}
	deployments, err := repositories.Rules().ListRuleDeploymentsByOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	deploymentsByRuleNode := make(map[string]repo.RuleDeploymentRecord, len(deployments))
	for _, deployment := range deployments {
		deploymentsByRuleNode[deployment.RuleID+"|"+deployment.NodeID] = deployment
	}
	seen := make(map[string]struct{}, len(failedRuleIDs))
	for _, ruleID := range failedRuleIDs {
		ruleID = strings.TrimSpace(ruleID)
		if ruleID == "" {
			continue
		}
		if _, ok := seen[ruleID]; ok {
			continue
		}
		seen[ruleID] = struct{}{}
		rule, err := repositories.Rules().FindRuleByID(ctx, organizationID, ruleID)
		if err != nil {
			return err
		}
		if !rule.Enabled || defaultFailurePolicy(rule.FailurePolicy) != FailurePolicyDisableWhenAllNodesFailed {
			continue
		}
		nodes, err := nodesInGroup(ctx, repositories, organizationID, rule.Binding.NodeGroupID)
		if err != nil {
			return err
		}
		if len(nodes) == 0 || !allCurrentRuleNodesFailed(rule, nodes, deploymentsByRuleNode) {
			continue
		}
		rule.Enabled = false
		rule.Status = "DISABLED"
		rule.ConfigVersion++
		rule.UpdatedAt = now
		if err := repositories.Rules().UpdateRule(ctx, rule, rule.Binding, rule.Tags, now, service.newID); err != nil {
			return err
		}
		if err := bumpDesiredConfigForNodeGroup(ctx, repositories, organizationID, rule.Binding.NodeGroupID, now); err != nil {
			return err
		}
		if err := service.writeAudit(ctx, repositories, auditInput{
			OrganizationID: organizationID,
			Action:         "rules.auto_disable_deploy_failure",
			ResourceType:   "FORWARDING_RULE",
			ResourceID:     rule.ID,
		}); err != nil {
			return err
		}
	}
	return nil
}

func allCurrentRuleNodesFailed(rule repo.RuleRecord, nodes []repo.NodeRecord, deploymentsByRuleNode map[string]repo.RuleDeploymentRecord) bool {
	for _, node := range nodes {
		deployment, ok := deploymentsByRuleNode[rule.ID+"|"+node.ID]
		if !ok ||
			deployment.ConfigVersion != node.DesiredConfigVersion ||
			deployment.RuleConfigVersion != rule.ConfigVersion ||
			deployment.Status != RuleDeploymentStatusFailed {
			return false
		}
	}
	return true
}
