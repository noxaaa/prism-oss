package service

import (
	"context"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

type Authorizer interface {
	HasPermission(identity InternalIdentity, permission string) bool
	AllowedNodeGroupIDs(identity InternalIdentity, requestedAccess string) map[string]bool
	EnsureCanDelegateRoleScopes(ctx context.Context, repositories repo.Repositories, identity InternalIdentity, scopes []repo.ResourceScopeRecord) error
}

func (service *ControlService) hasPermission(identity InternalIdentity, permission string) bool {
	return service.authorizer.HasPermission(identity, permission)
}

func (service *ControlService) allowedNodeGroupIDs(identity InternalIdentity, requestedAccess string) map[string]bool {
	return service.authorizer.AllowedNodeGroupIDs(identity, requestedAccess)
}

func (service *ControlService) canManageAllNodeGroups(identity InternalIdentity) bool {
	if !service.hasPermission(identity, string(domain.PermissionNodesManage)) {
		return false
	}
	return service.allowedNodeGroupIDs(identity, string(domain.AccessLevelManage))["*"]
}

func (service *ControlService) canManageNodeGroup(identity InternalIdentity, nodeGroupID string) bool {
	if !service.hasPermission(identity, string(domain.PermissionNodesManage)) {
		return false
	}
	allowed := service.allowedNodeGroupIDs(identity, string(domain.AccessLevelManage))
	return allowed["*"] || allowed[nodeGroupID]
}

func (service *ControlService) canUseNodeGroup(identity InternalIdentity, nodeGroupID string) bool {
	return service.canUseAnyNodeGroup(identity, []string{nodeGroupID})
}

func (service *ControlService) canUseAnyNodeGroup(identity InternalIdentity, groupIDs []string) bool {
	allowed := service.allowedNodeGroupIDs(identity, string(domain.AccessLevelUse))
	if allowed["*"] {
		return true
	}
	if len(groupIDs) == 0 {
		return false
	}
	for _, groupID := range groupIDs {
		if allowed[groupID] {
			return true
		}
	}
	return false
}

func (service *ControlService) ensureCanManageNodeGroups(identity InternalIdentity, groupIDs []string) error {
	allowed := service.allowedNodeGroupIDs(identity, string(domain.AccessLevelManage))
	if allowed["*"] {
		return nil
	}
	if len(groupIDs) == 0 {
		return ErrForbidden
	}
	for _, groupID := range groupIDs {
		if !allowed[groupID] {
			return ErrForbidden
		}
	}
	return nil
}

func stringSliceHas(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
