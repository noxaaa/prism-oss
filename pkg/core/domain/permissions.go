package domain

type Permission string

const (
	PermissionOrganizationRead   Permission = "organization.read"
	PermissionOrganizationUpdate Permission = "organization.update"
	PermissionQuotasManage       Permission = "quotas.manage"
	PermissionNodesRead          Permission = "nodes.read"
	PermissionNodesManage        Permission = "nodes.manage"
	PermissionMonitorsRead       Permission = "monitors.read"
	PermissionMonitorsManage     Permission = "monitors.manage"
	PermissionTargetsRead        Permission = "targets.read"
	PermissionTargetsManage      Permission = "targets.manage"
	PermissionRulesReadOwn       Permission = "rules.read_own"
	PermissionRulesManageOwn     Permission = "rules.manage_own"
	PermissionRulesReadAll       Permission = "rules.read_all"
	PermissionRulesManageAll     Permission = "rules.manage_all"
	PermissionTrafficReadOwn     Permission = "traffic.read_own"
	PermissionTrafficReadAll     Permission = "traffic.read_all"
	PermissionAuditLogsRead      Permission = "audit_logs.read"
)

type ResourceType string

const (
	ResourceTypeNodeGroup ResourceType = "NODE_GROUP"
)

type AccessLevel string

const (
	AccessLevelUse    AccessLevel = "USE"
	AccessLevelManage AccessLevel = "MANAGE"
)
