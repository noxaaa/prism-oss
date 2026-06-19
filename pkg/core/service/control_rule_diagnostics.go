package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func (service *ControlService) RuleDiagnostics(ctx context.Context, identity InternalIdentity, ruleID string, runtimeStates []AgentRuntimeMetricsInput) (RuleDiagnosticsPayload, error) {
	var result RuleDiagnosticsPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		rule, err := repositories.Rules().FindRuleByID(ctx, identity.OrganizationID, ruleID)
		if err != nil {
			return err
		}
		if !service.canReadRule(identity, rule) {
			return ErrForbidden
		}
		canReadAllTraffic := service.hasPermission(identity, string(domain.PermissionTrafficReadAll))
		canReadOwnTraffic := service.hasPermission(identity, string(domain.PermissionTrafficReadOwn)) && rule.OwnerUserID == identity.UserID
		if !canReadAllTraffic && !canReadOwnTraffic {
			return ErrForbidden
		}
		if !service.canUseNodeGroup(identity, rule.Binding.NodeGroupID) {
			return ErrForbidden
		}
		nodes, err := nodesInGroup(ctx, repositories, identity.OrganizationID, rule.Binding.NodeGroupID)
		if err != nil {
			return err
		}
		nodeIDs := make(map[string]bool, len(nodes))
		for _, node := range nodes {
			nodeIDs[node.ID] = true
		}
		targets, err := diagnosticsTargetsForRule(ctx, repositories, identity.OrganizationID, rule)
		if err != nil {
			return err
		}
		result = RuleDiagnosticsPayload{
			RuleID:      rule.ID,
			GeneratedAt: service.timestamp(),
			Targets:     diagnosticsTargetPayloads(targets),
		}
		targetIndexes := diagnosticsTargetIndexes(result.Targets)
		for _, state := range runtimeStates {
			if !nodeIDs[state.AgentID] {
				continue
			}
			if state.Status != "ONLINE" {
				continue
			}
			mergeRuleTargetMetrics(&result, targetIndexes, rule.ID, state)
		}
		return nil
	})
	return result, mapServiceError(err)
}

func diagnosticsTargetPayloads(targets []repo.TargetRecord) []RuleTargetDiagnosticsPayload {
	payloads := make([]RuleTargetDiagnosticsPayload, 0, len(targets))
	for _, target := range targets {
		payloads = append(payloads, RuleTargetDiagnosticsPayload{
			TargetID: target.ID,
			Name:     target.Name,
			Address:  target.Host + ":" + fmt.Sprint(target.Port),
			Status:   "NO_RUNTIME_METRICS",
		})
	}
	return payloads
}

func diagnosticsTargetIndexes(targets []RuleTargetDiagnosticsPayload) map[string]int {
	indexes := make(map[string]int, len(targets))
	for index, target := range targets {
		indexes[target.TargetID] = index
	}
	return indexes
}

func mergeRuleTargetMetrics(result *RuleDiagnosticsPayload, targetIndexes map[string]int, ruleID string, state AgentRuntimeMetricsInput) {
	for _, targetMetrics := range state.Metrics.Targets {
		if targetMetrics.RuleID != ruleID {
			continue
		}
		index, ok := targetIndexes[targetMetrics.TargetID]
		if !ok {
			continue
		}
		result.BandwidthBps += targetMetrics.BandwidthBps
		result.UploadBytes += targetMetrics.UploadBytes
		result.DownloadBytes += targetMetrics.DownloadBytes
		targetPayload := &result.Targets[index]
		targetPayload.Status = "OK"
		if isNewerObservation(state.LastSeenAt, targetPayload.LastSeenAt) {
			targetPayload.LastSeenAt = state.LastSeenAt
		}
		targetPayload.UploadBytes += targetMetrics.UploadBytes
		targetPayload.DownloadBytes += targetMetrics.DownloadBytes
		targetPayload.TCPConnections += targetMetrics.TCPConnections
		targetPayload.UDPPacketsPerSecond += targetMetrics.UDPPacketsPerSecond
		bandwidthBps := targetMetrics.BandwidthBps
		if targetPayload.BandwidthBps != nil {
			bandwidthBps += *targetPayload.BandwidthBps
		}
		targetPayload.BandwidthBps = &bandwidthBps
		if targetMetrics.LatencyMS > 0 {
			latencyMS := targetMetrics.LatencyMS
			if targetPayload.LatencyMS == nil || latencyMS < *targetPayload.LatencyMS {
				targetPayload.LatencyMS = &latencyMS
			}
		}
	}
}

func isNewerObservation(candidate string, current string) bool {
	if candidate == "" {
		return false
	}
	if current == "" {
		return true
	}
	candidateTime, candidateErr := time.Parse(time.RFC3339Nano, candidate)
	currentTime, currentErr := time.Parse(time.RFC3339Nano, current)
	if candidateErr == nil && currentErr == nil {
		return candidateTime.After(currentTime)
	}
	if candidateErr == nil {
		return true
	}
	if currentErr == nil {
		return false
	}
	return candidate > current
}

func diagnosticsTargetsForRule(ctx context.Context, repositories repo.Repositories, organizationID string, rule repo.RuleRecord) ([]repo.TargetRecord, error) {
	if rule.TargetType == "TARGET" {
		target, err := repositories.Targets().FindTargetByID(ctx, organizationID, rule.TargetID)
		if err != nil {
			return nil, err
		}
		return []repo.TargetRecord{target}, nil
	}
	if rule.TargetType != "TARGET_GROUP" {
		return []repo.TargetRecord{}, nil
	}
	targetGroup, err := repositories.TargetGroups().FindTargetGroupByID(ctx, organizationID, rule.TargetGroupID)
	if err != nil {
		return nil, err
	}
	targets, err := repositories.Targets().ListTargetsByOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	targetsByID := make(map[string]repo.TargetRecord, len(targets))
	for _, target := range targets {
		targetsByID[target.ID] = target
	}
	members := append([]repo.TargetGroupMemberRecord(nil), targetGroup.Members...)
	sort.SliceStable(members, func(left int, right int) bool {
		if members[left].Priority != members[right].Priority {
			return members[left].Priority < members[right].Priority
		}
		return members[left].TargetID < members[right].TargetID
	})
	result := make([]repo.TargetRecord, 0, len(members))
	for _, member := range members {
		target, ok := targetsByID[member.TargetID]
		if ok {
			result = append(result, target)
		}
	}
	return result, nil
}
