package service

import (
	"context"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func (service *ControlService) ListNodeGroupOptions(ctx context.Context, identity InternalIdentity, accessLevel string) ([]ResourceOption, error) {
	if accessLevel != string(domain.AccessLevelUse) && accessLevel != string(domain.AccessLevelManage) {
		return nil, ErrInvalidInput
	}
	if accessLevel == string(domain.AccessLevelManage) && !service.hasPermission(identity, string(domain.PermissionNodesManage)) {
		return nil, ErrForbidden
	}
	if accessLevel == string(domain.AccessLevelUse) &&
		!service.canListUseNodeGroupOptions(identity) {
		return nil, ErrForbidden
	}
	var result []ResourceOption
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		nodeGroups, err := repositories.NodeGroups().ListNodeGroupsByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		allowed := service.allowedNodeGroupIDs(identity, accessLevel)
		result = make([]ResourceOption, 0)
		for _, nodeGroup := range nodeGroups {
			if !allowed["*"] && !allowed[nodeGroup.ID] {
				continue
			}
			result = append(result, ResourceOption{Value: nodeGroup.ID, Label: nodeGroup.Name})
		}
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) ListNodeGroups(ctx context.Context, identity InternalIdentity) ([]NodeGroupPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionNodesRead)) && !service.hasPermission(identity, string(domain.PermissionNodesManage)) {
		return nil, ErrForbidden
	}
	var result []NodeGroupPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		nodeGroups, err := repositories.NodeGroups().ListNodeGroupsByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		allowed := service.allowedNodeGroupIDs(identity, string(domain.AccessLevelUse))
		for _, nodeGroup := range nodeGroups {
			if !allowed["*"] && !allowed[nodeGroup.ID] {
				continue
			}
			result = append(result, toNodeGroupPayload(nodeGroup))
		}
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) CreateNodeGroup(ctx context.Context, identity InternalIdentity, input GroupMutationInput) (NodeGroupPayload, error) {
	if !service.canManageAllNodeGroups(identity) {
		return NodeGroupPayload{}, ErrForbidden
	}
	var result NodeGroupPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		now := service.timestamp()
		nodeGroup := repo.NodeGroupRecord{
			ID:             service.newID(),
			OrganizationID: identity.OrganizationID,
			Name:           input.Name,
			Description:    input.Description,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := repositories.NodeGroups().CreateNodeGroup(ctx, nodeGroup); err != nil {
			return err
		}
		result = toNodeGroupPayload(nodeGroup)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "node_groups.create", "NODE_GROUP", nodeGroup.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) UpdateNodeGroup(ctx context.Context, identity InternalIdentity, nodeGroupID string, input GroupMutationInput) (NodeGroupPayload, error) {
	if !service.canManageNodeGroup(identity, nodeGroupID) {
		return NodeGroupPayload{}, ErrForbidden
	}
	var result NodeGroupPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		nodeGroup, err := repositories.NodeGroups().FindNodeGroupByID(ctx, identity.OrganizationID, nodeGroupID)
		if err != nil {
			return err
		}
		nodeGroup.Name = input.Name
		nodeGroup.Description = input.Description
		nodeGroup.UpdatedAt = service.timestamp()
		if err := repositories.NodeGroups().UpdateNodeGroup(ctx, nodeGroup); err != nil {
			return err
		}
		result = toNodeGroupPayload(nodeGroup)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "node_groups.update", "NODE_GROUP", nodeGroup.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) DeleteNodeGroup(ctx context.Context, identity InternalIdentity, nodeGroupID string) error {
	if !service.canManageNodeGroup(identity, nodeGroupID) {
		return ErrForbidden
	}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if _, err := repositories.NodeGroups().FindNodeGroupByID(ctx, identity.OrganizationID, nodeGroupID); err != nil {
			return err
		}
		if err := ensureNoRulesForNodeGroup(ctx, repositories, identity.OrganizationID, nodeGroupID); err != nil {
			return err
		}
		if err := ensureNoNodesForNodeGroup(ctx, repositories, identity.OrganizationID, nodeGroupID); err != nil {
			return err
		}
		if err := ensureNoDNSInstancesForNodeGroup(ctx, repositories, identity.OrganizationID, nodeGroupID); err != nil {
			return err
		}
		if err := ensureNoNodeEnrollmentProfilesForNodeGroup(ctx, repositories, identity.OrganizationID, nodeGroupID); err != nil {
			return err
		}
		deletedAt := service.timestamp()
		if err := repositories.NodeGroups().DeleteNodeGroup(ctx, identity.OrganizationID, nodeGroupID, deletedAt); err != nil {
			return err
		}
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "node_groups.delete", "NODE_GROUP", nodeGroupID, ""))
	})
	return mapServiceError(err)
}

func ensureNoNodesForNodeGroup(ctx context.Context, repositories repo.Repositories, organizationID string, nodeGroupID string) error {
	nodes, err := repositories.Nodes().ListNodesByOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		for _, groupID := range node.GroupIDs {
			if groupID == nodeGroupID {
				return &controlServiceError{
					Code:    "NODE_GROUP_IN_USE",
					Message: "The node group is still assigned to one or more nodes.",
					Details: map[string]any{
						"node_group_id": nodeGroupID,
						"node_id":       node.ID,
					},
					Cause: ErrConflict,
				}
			}
		}
	}
	return nil
}

func ensureNoNodeEnrollmentProfilesForNodeGroup(ctx context.Context, repositories repo.Repositories, organizationID string, nodeGroupID string) error {
	profiles, err := repositories.NodeEnrollmentProfiles().ListNodeEnrollmentProfiles(ctx, organizationID)
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		for _, groupID := range decodeJSONStringList(profile.GroupIDsJSON) {
			if groupID == nodeGroupID {
				return &controlServiceError{
					Code:    "NODE_GROUP_IN_USE",
					Message: "The node group is still referenced by one or more node enrollment profiles.",
					Details: map[string]any{
						"node_group_id":                  nodeGroupID,
						"node_enrollment_profile_id":     profile.ID,
						"node_enrollment_profile_name":   profile.Name,
						"node_enrollment_profile_active": profile.Enabled && profile.RevokedAt == "",
					},
					Cause: ErrConflict,
				}
			}
		}
	}
	return nil
}

func ensureNoDNSInstancesForNodeGroup(ctx context.Context, repositories repo.Repositories, organizationID string, nodeGroupID string) error {
	instances, err := repositories.DNSRecords().ListDNSInstancesByOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	for _, instance := range instances {
		for _, groupID := range parseStringListJSON(instance.NodeGroupIDsJSON) {
			if groupID == nodeGroupID {
				return &controlServiceError{
					Code:    "NODE_GROUP_IN_USE",
					Message: "The node group is still referenced by one or more DNS policies.",
					Details: map[string]any{
						"node_group_id":   nodeGroupID,
						"dns_instance_id": instance.ID,
					},
					Cause: ErrConflict,
				}
			}
		}
	}
	return nil
}

func (service *ControlService) ListNodes(ctx context.Context, identity InternalIdentity) ([]NodePayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionNodesRead)) {
		return nil, ErrForbidden
	}
	var result []NodePayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		nodes, err := repositories.Nodes().ListNodesByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		for _, node := range nodes {
			if !service.canUseAnyNodeGroup(identity, node.GroupIDs) {
				continue
			}
			result = append(result, service.toNodePayload(node))
		}
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) GetNode(ctx context.Context, identity InternalIdentity, nodeID string) (NodePayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionNodesRead)) {
		return NodePayload{}, ErrForbidden
	}
	var result NodePayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		node, err := repositories.Nodes().FindNodeByID(ctx, identity.OrganizationID, nodeID)
		if err != nil {
			return err
		}
		if !service.canUseAnyNodeGroup(identity, node.GroupIDs) {
			return ErrForbidden
		}
		result = service.toNodePayload(node)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) AuthorizeNodeMetricsStream(ctx context.Context, identity InternalIdentity, nodeID string) error {
	if !service.hasPermission(identity, string(domain.PermissionNodesRead)) || !service.hasPermission(identity, string(domain.PermissionTrafficReadAll)) {
		return ErrForbidden
	}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		node, err := repositories.Nodes().FindNodeByID(ctx, identity.OrganizationID, nodeID)
		if err != nil {
			return err
		}
		if !service.canUseAnyNodeGroup(identity, node.GroupIDs) {
			return ErrForbidden
		}
		return nil
	})
	return mapServiceError(err)
}

func (service *ControlService) AuthorizeOrganizationNodeMetricsStream(_ context.Context, identity InternalIdentity) error {
	if !service.hasPermission(identity, string(domain.PermissionNodesRead)) || !service.hasPermission(identity, string(domain.PermissionTrafficReadAll)) {
		return ErrForbidden
	}
	return nil
}

func (service *ControlService) CreateNode(ctx context.Context, identity InternalIdentity, input NodeMutationInput) (NodePayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionNodesManage)) {
		return NodePayload{}, ErrForbidden
	}
	dataplaneMode, err := normalizeNodeDataplaneModeForMutation(input.DataplaneMode)
	if err != nil {
		return NodePayload{}, err
	}
	input.DataplaneMode = dataplaneMode
	input.DataplaneConflictPolicy = defaultNodeDataplaneConflictPolicy(input.DataplaneConflictPolicy)
	if err := validateNodeDataplaneConflictPolicy(input.DataplaneConflictPolicy); err != nil {
		return NodePayload{}, err
	}
	if len(input.GroupIDs) == 0 && !service.canManageAllNodeGroups(identity) {
		return NodePayload{}, ErrForbidden
	}
	if err := service.ensureCanManageNodeGroups(identity, input.GroupIDs); err != nil {
		return NodePayload{}, err
	}
	var result NodePayload
	var affectedDNSRecordIDs []string
	err = service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if err := ensureNodeGroupsExist(ctx, repositories, identity.OrganizationID, input.GroupIDs); err != nil {
			return err
		}
		now := service.timestamp()
		node := repo.NodeRecord{
			ID:                      service.newID(),
			OrganizationID:          identity.OrganizationID,
			Name:                    input.Name,
			Status:                  "PENDING",
			PublicDescription:       input.PublicDescription,
			ConfigStatus:            "PENDING",
			ConfigErrorMessage:      "",
			AgentAutoUpdateEnabled:  true,
			DataplaneMode:           input.DataplaneMode,
			DataplaneConflictPolicy: input.DataplaneConflictPolicy,
			CreatedAt:               now,
			UpdatedAt:               now,
			GroupIDs:                append([]string(nil), input.GroupIDs...),
			ListenIPs:               toNodeListenIPRecords(input.ListenIPs),
			SendIPs:                 toNodeSendIPRecords(input.SendIPs),
			PortRanges:              toNodePortRangeRecords(input.PortRanges),
			MaxRulePorts:            defaultMaxRulePorts(input.MaxRulePorts),
			DNSPublishAddresses:     toNodeDNSPublishAddressRecords(input.DNSPublishAddresses),
		}
		nodes, err := repositories.Nodes().ListNodesByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		if err := validateEnabledRulesForNodeSet(ctx, repositories, identity.OrganizationID, replaceNodeInSet(nodes, node)); err != nil {
			return err
		}
		if err := repositories.Nodes().CreateNode(ctx, node, input.GroupIDs, node.ListenIPs, node.PortRanges, now, service.newID); err != nil {
			return err
		}
		if err := replaceManualNodeDNSPublishAddresses(ctx, repositories, identity.OrganizationID, node.ID, node.DNSPublishAddresses, now, service.newID); err != nil {
			return err
		}
		affectedDNSRecordIDs, err = service.markDNSRecordsDependingOnNodeGroupsPending(ctx, repositories, identity.OrganizationID, node.GroupIDs, now)
		if err != nil {
			return err
		}
		node, err = repositories.Nodes().FindNodeByID(ctx, identity.OrganizationID, node.ID)
		if err != nil {
			return err
		}
		result = service.toNodePayload(node)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "nodes.create", "NODE", node.ID, ""))
	})
	if err == nil {
		service.evaluateDNSManagedRecordsBestEffort(ctx, identity.OrganizationID, affectedDNSRecordIDs)
	}
	return result, mapServiceError(err)
}

func (service *ControlService) UpdateNode(ctx context.Context, identity InternalIdentity, nodeID string, input NodeMutationInput) (NodePayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionNodesManage)) {
		return NodePayload{}, ErrForbidden
	}
	if input.DataplaneModeProvided {
		dataplaneMode, err := normalizeNodeDataplaneModeForMutation(input.DataplaneMode)
		if err != nil {
			return NodePayload{}, err
		}
		input.DataplaneMode = dataplaneMode
	}
	if input.DataplaneConflictPolicyProvided {
		input.DataplaneConflictPolicy = defaultNodeDataplaneConflictPolicy(input.DataplaneConflictPolicy)
		if err := validateNodeDataplaneConflictPolicy(input.DataplaneConflictPolicy); err != nil {
			return NodePayload{}, err
		}
	}
	var result NodePayload
	var affectedDNSRecordIDs []string
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		node, err := repositories.Nodes().FindNodeByID(ctx, identity.OrganizationID, nodeID)
		if err != nil {
			return err
		}
		if err := service.ensureCanManageNodeGroups(identity, node.GroupIDs); err != nil {
			return err
		}
		previousGroupIDs := append([]string(nil), node.GroupIDs...)
		targetGroupIDs := node.GroupIDs
		if input.GroupIDsProvided {
			targetGroupIDs = input.GroupIDs
			if len(targetGroupIDs) == 0 && !service.canManageAllNodeGroups(identity) {
				return ErrForbidden
			}
			if err := service.ensureCanManageNodeGroups(identity, targetGroupIDs); err != nil {
				return err
			}
			if err := ensureNodeGroupsExist(ctx, repositories, identity.OrganizationID, targetGroupIDs); err != nil {
				return err
			}
		}
		if input.NameProvided {
			node.Name = input.Name
		}
		if input.PublicDescriptionProvided {
			node.PublicDescription = input.PublicDescription
		}
		dataplaneConfigChanged := false
		if input.DataplaneModeProvided && node.DataplaneMode != input.DataplaneMode {
			node.DataplaneMode = input.DataplaneMode
			dataplaneConfigChanged = true
		}
		if input.DataplaneConflictPolicyProvided && node.DataplaneConflictPolicy != input.DataplaneConflictPolicy {
			node.DataplaneConflictPolicy = input.DataplaneConflictPolicy
			dataplaneConfigChanged = true
		}
		if input.ListenIPsProvided {
			node.ListenIPs = toNodeListenIPRecords(input.ListenIPs)
		}
		if input.SendIPsProvided {
			node.SendIPs = toNodeSendIPRecords(input.SendIPs)
		}
		if input.PortRangesProvided {
			node.PortRanges = toNodePortRangeRecords(input.PortRanges)
		}
		if input.MaxRulePortsProvided {
			node.MaxRulePorts = defaultMaxRulePorts(input.MaxRulePorts)
		}
		if input.DNSPublishAddressesProvided {
			node.DNSPublishAddresses = toNodeDNSPublishAddressRecords(input.DNSPublishAddresses)
		}
		node.GroupIDs = targetGroupIDs
		nodes, err := repositories.Nodes().ListNodesByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		if err := validateEnabledRulesForNodeSet(ctx, repositories, identity.OrganizationID, replaceNodeInSet(nodes, node)); err != nil {
			return err
		}
		node.UpdatedAt = service.timestamp()
		if err := repositories.Nodes().UpdateNode(
			ctx,
			node,
			input.GroupIDsProvided,
			targetGroupIDs,
			input.ListenIPsProvided,
			toNodeListenIPRecords(input.ListenIPs),
			input.PortRangesProvided,
			toNodePortRangeRecords(input.PortRanges),
			node.UpdatedAt,
			service.newID,
		); err != nil {
			return err
		}
		if input.DNSPublishAddressesProvided {
			if err := replaceManualNodeDNSPublishAddresses(ctx, repositories, identity.OrganizationID, node.ID, node.DNSPublishAddresses, node.UpdatedAt, service.newID); err != nil {
				return err
			}
		}
		if input.DNSPublishAddressesProvided || (input.GroupIDsProvided && !sameStringSet(previousGroupIDs, targetGroupIDs)) {
			affectedDNSRecordIDs, err = service.markDNSRecordsDependingOnNodeGroupsPending(ctx, repositories, identity.OrganizationID, mergeStringSets(previousGroupIDs, targetGroupIDs), node.UpdatedAt)
			if err != nil {
				return err
			}
		}
		groupMembershipChanged := input.GroupIDsProvided && !sameStringSet(previousGroupIDs, targetGroupIDs)
		if dataplaneConfigChanged || groupMembershipChanged {
			if err := repositories.Nodes().IncrementDesiredConfigForNode(ctx, identity.OrganizationID, node.ID, node.UpdatedAt); err != nil {
				return err
			}
			node, err = repositories.Nodes().FindNodeByID(ctx, identity.OrganizationID, node.ID)
			if err != nil {
				return err
			}
			if groupMembershipChanged {
				if err := syncRuleDeploymentsForNodeMembershipChange(ctx, repositories, identity.OrganizationID, node, previousGroupIDs, targetGroupIDs, node.UpdatedAt, service.newID); err != nil {
					return err
				}
			}
			if err := syncRuleDeploymentsForNodeConfigChange(ctx, repositories, identity.OrganizationID, node, node.UpdatedAt, service.newID); err != nil {
				return err
			}
		}
		node, err = repositories.Nodes().FindNodeByID(ctx, identity.OrganizationID, node.ID)
		if err != nil {
			return err
		}
		result = service.toNodePayload(node)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "nodes.update", "NODE", node.ID, ""))
	})
	if err == nil {
		service.evaluateDNSManagedRecordsBestEffort(ctx, identity.OrganizationID, affectedDNSRecordIDs)
	}
	return result, mapServiceError(err)
}

func (service *ControlService) DeleteNode(ctx context.Context, identity InternalIdentity, nodeID string) error {
	if !service.hasPermission(identity, string(domain.PermissionNodesManage)) {
		return ErrForbidden
	}
	var affectedDNSRecordIDs []string
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		node, err := repositories.Nodes().FindNodeByID(ctx, identity.OrganizationID, nodeID)
		if err != nil {
			return err
		}
		if err := service.ensureCanManageNodeGroups(identity, node.GroupIDs); err != nil {
			return err
		}
		nodes, err := repositories.Nodes().ListNodesByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		if err := validateEnabledRulesForNodeSet(ctx, repositories, identity.OrganizationID, removeNodeFromSet(nodes, nodeID)); err != nil {
			return err
		}
		deletedAt := service.timestamp()
		releasedDNSRecordIDs, released, err := service.releasePendingNodeEnrollmentForNode(ctx, repositories, node, deletedAt)
		if err != nil {
			return err
		}
		if released {
			affectedDNSRecordIDs = releasedDNSRecordIDs
			return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "nodes.delete", "NODE", nodeID, ""))
		}
		if err := repositories.Nodes().DeleteNode(ctx, identity.OrganizationID, nodeID, deletedAt); err != nil {
			return err
		}
		affectedDNSRecordIDs, err = service.markDNSRecordsDependingOnNodeGroupsPending(ctx, repositories, identity.OrganizationID, node.GroupIDs, deletedAt)
		if err != nil {
			return err
		}
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "nodes.delete", "NODE", nodeID, ""))
	})
	if err == nil {
		service.evaluateDNSManagedRecordsBestEffort(ctx, identity.OrganizationID, affectedDNSRecordIDs)
	}
	return mapServiceError(err)
}

func (service *ControlService) ListMonitorGroups(ctx context.Context, identity InternalIdentity) ([]MonitorGroupPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionMonitorsRead)) && !service.hasPermission(identity, string(domain.PermissionMonitorsManage)) {
		return nil, ErrForbidden
	}
	var result []MonitorGroupPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		monitorGroups, err := repositories.MonitorGroups().ListMonitorGroupsByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		result = toMonitorGroupPayloads(monitorGroups)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) CreateMonitorGroup(ctx context.Context, identity InternalIdentity, input GroupMutationInput) (MonitorGroupPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionMonitorsManage)) {
		return MonitorGroupPayload{}, ErrForbidden
	}
	var result MonitorGroupPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		now := service.timestamp()
		monitorGroup := repo.MonitorGroupRecord{
			ID:             service.newID(),
			OrganizationID: identity.OrganizationID,
			Name:           input.Name,
			Description:    input.Description,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := repositories.MonitorGroups().CreateMonitorGroup(ctx, monitorGroup); err != nil {
			return err
		}
		result = toMonitorGroupPayload(monitorGroup)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "monitor_groups.create", "MONITOR_GROUP", monitorGroup.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) UpdateMonitorGroup(ctx context.Context, identity InternalIdentity, monitorGroupID string, input GroupMutationInput) (MonitorGroupPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionMonitorsManage)) {
		return MonitorGroupPayload{}, ErrForbidden
	}
	var result MonitorGroupPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		monitorGroup, err := repositories.MonitorGroups().FindMonitorGroupByID(ctx, identity.OrganizationID, monitorGroupID)
		if err != nil {
			return err
		}
		monitorGroup.Name = input.Name
		monitorGroup.Description = input.Description
		monitorGroup.UpdatedAt = service.timestamp()
		if err := repositories.MonitorGroups().UpdateMonitorGroup(ctx, monitorGroup); err != nil {
			return err
		}
		result = toMonitorGroupPayload(monitorGroup)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "monitor_groups.update", "MONITOR_GROUP", monitorGroup.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) DeleteMonitorGroup(ctx context.Context, identity InternalIdentity, monitorGroupID string) error {
	if !service.hasPermission(identity, string(domain.PermissionMonitorsManage)) {
		return ErrForbidden
	}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if _, err := repositories.MonitorGroups().FindMonitorGroupByID(ctx, identity.OrganizationID, monitorGroupID); err != nil {
			return err
		}
		if err := ensureMonitorGroupNotUsedByHealthChecks(ctx, repositories, identity.OrganizationID, monitorGroupID); err != nil {
			return err
		}
		deletedAt := service.timestamp()
		if err := repositories.MonitorGroups().DeleteMonitorGroup(ctx, identity.OrganizationID, monitorGroupID, deletedAt); err != nil {
			return err
		}
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "monitor_groups.delete", "MONITOR_GROUP", monitorGroupID, ""))
	})
	return mapServiceError(err)
}

func (service *ControlService) ListMonitors(ctx context.Context, identity InternalIdentity) ([]MonitorPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionMonitorsRead)) {
		return nil, ErrForbidden
	}
	var result []MonitorPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		monitors, err := repositories.Monitors().ListMonitorsByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		result = toMonitorPayloads(monitors)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) GetMonitor(ctx context.Context, identity InternalIdentity, monitorID string) (MonitorPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionMonitorsRead)) {
		return MonitorPayload{}, ErrForbidden
	}
	var result MonitorPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		monitor, err := repositories.Monitors().FindMonitorByID(ctx, identity.OrganizationID, monitorID)
		if err != nil {
			return err
		}
		result = toMonitorPayload(monitor)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) CreateMonitor(ctx context.Context, identity InternalIdentity, input MonitorMutationInput) (MonitorPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionMonitorsManage)) {
		return MonitorPayload{}, ErrForbidden
	}
	var result MonitorPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if err := ensureMonitorGroupsExist(ctx, repositories, identity.OrganizationID, input.GroupIDs); err != nil {
			return err
		}
		now := service.timestamp()
		monitor := repo.MonitorRecord{
			ID:             service.newID(),
			OrganizationID: identity.OrganizationID,
			Name:           input.Name,
			Status:         "PENDING",
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := repositories.Monitors().CreateMonitor(ctx, monitor, input.GroupIDs, now, service.newID); err != nil {
			return err
		}
		monitor, err := repositories.Monitors().FindMonitorByID(ctx, identity.OrganizationID, monitor.ID)
		if err != nil {
			return err
		}
		result = toMonitorPayload(monitor)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "monitors.create", "MONITOR", monitor.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) UpdateMonitor(ctx context.Context, identity InternalIdentity, monitorID string, input MonitorMutationInput) (MonitorPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionMonitorsManage)) {
		return MonitorPayload{}, ErrForbidden
	}
	var result MonitorPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		monitor, err := repositories.Monitors().FindMonitorByID(ctx, identity.OrganizationID, monitorID)
		if err != nil {
			return err
		}
		if input.NameProvided {
			monitor.Name = input.Name
		}
		targetGroups := monitor.GroupIDs
		if input.GroupIDsProvided {
			targetGroups = input.GroupIDs
			if err := ensureMonitorGroupsExist(ctx, repositories, identity.OrganizationID, targetGroups); err != nil {
				return err
			}
		}
		monitor.UpdatedAt = service.timestamp()
		if err := repositories.Monitors().UpdateMonitor(ctx, monitor, input.GroupIDsProvided, targetGroups, monitor.UpdatedAt, service.newID); err != nil {
			return err
		}
		monitor, err = repositories.Monitors().FindMonitorByID(ctx, identity.OrganizationID, monitor.ID)
		if err != nil {
			return err
		}
		result = toMonitorPayload(monitor)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "monitors.update", "MONITOR", monitor.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) DeleteMonitor(ctx context.Context, identity InternalIdentity, monitorID string) error {
	if !service.hasPermission(identity, string(domain.PermissionMonitorsManage)) {
		return ErrForbidden
	}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if _, err := repositories.Monitors().FindMonitorByID(ctx, identity.OrganizationID, monitorID); err != nil {
			return err
		}
		if err := ensureMonitorNotUsedByHealthChecks(ctx, repositories, identity.OrganizationID, monitorID); err != nil {
			return err
		}
		deletedAt := service.timestamp()
		if err := repositories.Monitors().DeleteMonitor(ctx, identity.OrganizationID, monitorID, deletedAt); err != nil {
			return err
		}
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "monitors.delete", "MONITOR", monitorID, ""))
	})
	return mapServiceError(err)
}

func ensureMonitorNotUsedByHealthChecks(ctx context.Context, repositories repo.Repositories, organizationID string, monitorID string) error {
	checks, err := repositories.HealthChecks().ListHealthChecksByOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	for _, check := range checks {
		for _, scope := range check.MonitorScopes {
			if scope.ScopeType != "MONITOR" || scope.MonitorID != monitorID {
				continue
			}
			return &controlServiceError{
				Code:    "MONITOR_IN_USE",
				Message: "The monitor is still assigned to one or more health checks.",
				Details: map[string]any{
					"monitor_id":      monitorID,
					"health_check_id": check.ID,
				},
				Cause: ErrConflict,
			}
		}
	}
	return nil
}

func ensureMonitorGroupNotUsedByHealthChecks(ctx context.Context, repositories repo.Repositories, organizationID string, monitorGroupID string) error {
	checks, err := repositories.HealthChecks().ListHealthChecksByOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	for _, check := range checks {
		for _, scope := range check.MonitorScopes {
			if scope.ScopeType != "MONITOR_GROUP" || scope.MonitorGroupID != monitorGroupID {
				continue
			}
			return &controlServiceError{
				Code:    "MONITOR_GROUP_IN_USE",
				Message: "The monitor group is still assigned to one or more health checks.",
				Details: map[string]any{
					"monitor_group_id": monitorGroupID,
					"health_check_id":  check.ID,
				},
				Cause: ErrConflict,
			}
		}
	}
	return nil
}

func ensureNodeGroupsExist(ctx context.Context, repositories repo.Repositories, organizationID string, groupIDs []string) error {
	for _, groupID := range groupIDs {
		if _, err := repositories.NodeGroups().FindNodeGroupByID(ctx, organizationID, groupID); err != nil {
			return err
		}
	}
	return nil
}

func ensureMonitorGroupsExist(ctx context.Context, repositories repo.Repositories, organizationID string, groupIDs []string) error {
	for _, groupID := range groupIDs {
		if _, err := repositories.MonitorGroups().FindMonitorGroupByID(ctx, organizationID, groupID); err != nil {
			return err
		}
	}
	return nil
}

func toNodeListenIPRecords(inputs []NodeListenIPInput) []repo.NodeListenIPRecord {
	records := make([]repo.NodeListenIPRecord, 0, len(inputs))
	for _, input := range inputs {
		records = append(records, repo.NodeListenIPRecord{ListenIP: input.ListenIP, DisplayName: input.DisplayName, Enabled: true})
	}
	return records
}

func toNodePortRangeRecords(inputs []NodePortRangeInput) []repo.NodePortRangeRecord {
	if len(inputs) == 0 {
		inputs = []NodePortRangeInput{
			{Protocol: string(domain.ProtocolTCP), StartPort: 1, EndPort: 65535},
			{Protocol: string(domain.ProtocolUDP), StartPort: 1, EndPort: 65535},
		}
	}
	records := make([]repo.NodePortRangeRecord, 0, len(inputs))
	for _, input := range inputs {
		records = append(records, repo.NodePortRangeRecord{Protocol: input.Protocol, StartPort: input.StartPort, EndPort: input.EndPort, Enabled: true})
	}
	return records
}

type manualNodeDNSPublisher interface {
	ReplaceManualNodeDNSPublishAddresses(ctx context.Context, organizationID string, nodeID string, addresses []repo.NodeDNSPublishAddressRecord, now string, nextID func() string) error
}

func replaceManualNodeDNSPublishAddresses(ctx context.Context, repositories repo.Repositories, organizationID string, nodeID string, addresses []repo.NodeDNSPublishAddressRecord, now string, nextID func() string) error {
	publisher, ok := repositories.Nodes().(manualNodeDNSPublisher)
	if !ok {
		return nil
	}
	return publisher.ReplaceManualNodeDNSPublishAddresses(ctx, organizationID, nodeID, addresses, now, nextID)
}

func toNodeDNSPublishAddressRecords(inputs []NodeDNSPublishAddressInput) []repo.NodeDNSPublishAddressRecord {
	records := make([]repo.NodeDNSPublishAddressRecord, 0, len(inputs))
	for _, input := range inputs {
		records = append(records, repo.NodeDNSPublishAddressRecord{AddressType: input.AddressType, Address: input.Address, Source: "MANUAL", Enabled: input.Enabled})
	}
	return records
}

func toNodeGroupPayload(nodeGroup repo.NodeGroupRecord) NodeGroupPayload {
	return NodeGroupPayload{ID: nodeGroup.ID, Name: nodeGroup.Name, Description: nodeGroup.Description}
}

func toNodePayload(node repo.NodeRecord) NodePayload {
	return nodePayloadWithoutGeoIP(node)
}

func nodePayloadWithoutGeoIP(node repo.NodeRecord) NodePayload {
	return NodePayload{
		ID:                      node.ID,
		Name:                    node.Name,
		Status:                  node.Status,
		PublicDescription:       node.PublicDescription,
		DesiredConfigVersion:    node.DesiredConfigVersion,
		AppliedConfigVersion:    node.AppliedConfigVersion,
		ConfigStatus:            node.ConfigStatus,
		ConfigErrorMessage:      node.ConfigErrorMessage,
		ConfigStatusUpdatedAt:   node.ConfigStatusUpdatedAt,
		LastSeenAt:              node.LastSeenAt,
		RegisteredAt:            node.RegisteredAt,
		AgentVersion:            node.AgentVersion,
		AgentCommit:             node.AgentCommit,
		AgentBuildTime:          node.AgentBuildTime,
		AgentAutoUpdateEnabled:  node.AgentAutoUpdateEnabled,
		DesiredAgentVersion:     node.DesiredAgentVersion,
		AgentUpdateStatus:       node.AgentUpdateStatus,
		AgentUpdateError:        node.AgentUpdateError,
		AgentUpdateStartedAt:    node.AgentUpdateStartedAt,
		AgentUpdateFinishedAt:   node.AgentUpdateFinishedAt,
		DataplaneMode:           defaultNodeDataplaneMode(node.DataplaneMode),
		DataplaneConflictPolicy: defaultNodeDataplaneConflictPolicy(node.DataplaneConflictPolicy),
		DataplaneInstanceID:     node.DataplaneInstanceID,
		DataplaneStatus:         defaultNodeDataplaneStatus(node.DataplaneStatus),
		DataplaneError:          node.DataplaneError,
		DataplaneLastHash:       node.DataplaneLastHash,
		DataplaneLastAppliedAt:  node.DataplaneLastAppliedAt,
		RegistrationSource:      nodeRegistrationSource(node),
		EnrollmentProfile:       nodeEnrollmentProfileRef(node),
		GroupIDs:                stringSlicePayload(node.GroupIDs),
		ListenIPs:               toNodeListenIPPayloads(node.ListenIPs),
		SendIPs:                 toNodeSendIPPayloads(node.SendIPs),
		PortRanges:              toNodePortRangePayloads(node.PortRanges),
		MaxRulePorts:            defaultMaxRulePorts(node.MaxRulePorts),
		DNSPublishAddresses:     toNodeDNSPublishAddressPayloads(node.DNSPublishAddresses),
	}
}

func nodeRegistrationSource(node repo.NodeRecord) string {
	if strings.TrimSpace(node.EnrollmentProfileID) != "" {
		return "ENROLLMENT_PROFILE"
	}
	return "MANUAL"
}

func nodeEnrollmentProfileRef(node repo.NodeRecord) *NodeEnrollmentProfileRef {
	if strings.TrimSpace(node.EnrollmentProfileID) == "" {
		return nil
	}
	return &NodeEnrollmentProfileRef{ID: node.EnrollmentProfileID, Name: node.EnrollmentProfileName}
}

func toNodeListenIPPayloads(listenIPs []repo.NodeListenIPRecord) []NodeListenIPPayload {
	payloads := make([]NodeListenIPPayload, 0, len(listenIPs))
	for _, listenIP := range listenIPs {
		payloads = append(payloads, NodeListenIPPayload{ID: listenIP.ID, ListenIP: listenIP.ListenIP, DisplayName: listenIP.DisplayName, Enabled: listenIP.Enabled})
	}
	return payloads
}

func toNodePortRangePayloads(portRanges []repo.NodePortRangeRecord) []NodePortRangePayload {
	payloads := make([]NodePortRangePayload, 0, len(portRanges))
	for _, portRange := range portRanges {
		payloads = append(payloads, NodePortRangePayload{ID: portRange.ID, Protocol: portRange.Protocol, StartPort: portRange.StartPort, EndPort: portRange.EndPort, Enabled: portRange.Enabled})
	}
	return payloads
}

func toNodeDNSPublishAddressPayloads(addresses []repo.NodeDNSPublishAddressRecord) []NodeDNSPublishAddressPayload {
	payloads := make([]NodeDNSPublishAddressPayload, 0, len(addresses))
	for _, address := range addresses {
		payloads = append(payloads, toNodeDNSPublishAddressPayload(address))
	}
	return payloads
}

func toNodeDNSPublishAddressPayload(address repo.NodeDNSPublishAddressRecord) NodeDNSPublishAddressPayload {
	return NodeDNSPublishAddressPayload{ID: address.ID, AddressType: address.AddressType, Address: address.Address, Source: address.Source, Enabled: address.Enabled, ObservedAt: address.ObservedAt}
}

func mergeStringSets(left []string, right []string) []string {
	seen := make(map[string]bool, len(left)+len(right))
	merged := make([]string, 0, len(left)+len(right))
	for _, value := range append(append([]string{}, left...), right...) {
		if seen[value] {
			continue
		}
		seen[value] = true
		merged = append(merged, value)
	}
	return merged
}

func toMonitorGroupPayload(monitorGroup repo.MonitorGroupRecord) MonitorGroupPayload {
	return MonitorGroupPayload{ID: monitorGroup.ID, Name: monitorGroup.Name, Description: monitorGroup.Description}
}

func toMonitorGroupPayloads(monitorGroups []repo.MonitorGroupRecord) []MonitorGroupPayload {
	payloads := make([]MonitorGroupPayload, 0, len(monitorGroups))
	for _, monitorGroup := range monitorGroups {
		payloads = append(payloads, toMonitorGroupPayload(monitorGroup))
	}
	return payloads
}

func toMonitorPayload(monitor repo.MonitorRecord) MonitorPayload {
	return MonitorPayload{
		ID:                   monitor.ID,
		Name:                 monitor.Name,
		Status:               monitor.Status,
		DesiredConfigVersion: monitor.DesiredConfigVersion,
		AppliedConfigVersion: monitor.AppliedConfigVersion,
		LastSeenAt:           monitor.LastSeenAt,
		RegisteredAt:         monitor.RegisteredAt,
		GroupIDs:             stringSlicePayload(monitor.GroupIDs),
	}
}

func stringSlicePayload(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}

func toMonitorPayloads(monitors []repo.MonitorRecord) []MonitorPayload {
	payloads := make([]MonitorPayload, 0, len(monitors))
	for _, monitor := range monitors {
		payloads = append(payloads, toMonitorPayload(monitor))
	}
	return payloads
}

func toRegistrationTokenPayload(token repo.AgentRegistrationTokenRecord) RegistrationTokenPayload {
	return RegistrationTokenPayload{
		TokenID:         token.ID,
		AgentType:       token.AgentType,
		AgentID:         token.AgentID,
		ExpiresAt:       token.ExpiresAt,
		UsedAt:          token.UsedAt,
		RevokedAt:       token.RevokedAt,
		CreatedAt:       token.CreatedAt,
		CreatedByUserID: token.CreatedByUserID,
	}
}

func toRegistrationTokenPayloads(tokens []repo.AgentRegistrationTokenRecord) []RegistrationTokenPayload {
	payloads := make([]RegistrationTokenPayload, 0, len(tokens))
	for _, token := range tokens {
		payloads = append(payloads, toRegistrationTokenPayload(token))
	}
	return payloads
}
