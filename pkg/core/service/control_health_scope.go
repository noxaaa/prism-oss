package service

import (
	"context"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func healthCheckTargetsMonitor(check repo.HealthCheckRecord, monitor repo.MonitorRecord, activeMonitorGroupIDs map[string]bool) bool {
	groupIDs := make(map[string]bool, len(monitor.GroupIDs))
	for _, groupID := range monitor.GroupIDs {
		if activeMonitorGroupIDs != nil && !activeMonitorGroupIDs[groupID] {
			continue
		}
		groupIDs[groupID] = true
	}
	for _, scope := range check.MonitorScopes {
		if scope.ScopeType == "MONITOR" && scope.MonitorID == monitor.ID {
			return true
		}
		if scope.ScopeType == "MONITOR_GROUP" && groupIDs[scope.MonitorGroupID] {
			return true
		}
	}
	return false
}

func activeMonitorGroupIDSet(ctx context.Context, repositories repo.Repositories, organizationID string) (map[string]bool, error) {
	groups, err := repositories.MonitorGroups().ListMonitorGroupsByOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]bool, len(groups))
	for _, group := range groups {
		result[group.ID] = true
	}
	return result, nil
}
