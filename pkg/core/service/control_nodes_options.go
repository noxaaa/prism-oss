package service

import "github.com/noxaaa/prism-oss/pkg/core/domain"

func (service *ControlService) canListUseNodeGroupOptions(identity InternalIdentity) bool {
	return service.hasPermission(identity, string(domain.PermissionRulesManageOwn)) ||
		service.hasPermission(identity, string(domain.PermissionRulesManageAll)) ||
		service.hasPermission(identity, string(domain.PermissionDNSRead)) ||
		service.hasPermission(identity, string(domain.PermissionDNSManage)) ||
		service.hasPermission(identity, string(domain.PermissionNodesManage)) ||
		service.canListUseNodeGroupOptionsForCommercial(identity)
}
