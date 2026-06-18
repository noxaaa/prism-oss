import type { RuleImportIssue } from "@/components/console/types";

export const localeStorageKey = "console_locale";

export type Locale = "zh-CN" | "en";
export type MessageKey = string;
export type MessageParams = Record<string, string | number | boolean | null | undefined>;

const commonMessages = {
  "common.language": { en: "Language", zh: "语言" },
  "common.chinese": { en: "Chinese", zh: "中文" },
  "common.english": { en: "English", zh: "English" },
  "common.refresh": { en: "Refresh", zh: "刷新" },
  "common.options": { en: "Options", zh: "操作" },
  "common.actions": { en: "Actions", zh: "操作" },
  "common.create": { en: "Create", zh: "创建" },
  "common.save": { en: "Save", zh: "保存" },
  "common.delete": { en: "Delete", zh: "删除" },
  "common.edit": { en: "Edit", zh: "编辑" },
  "common.copy": { en: "Copy", zh: "复制" },
  "common.export": { en: "Export", zh: "导出" },
  "common.import": { en: "Import", zh: "导入" },
  "common.enable": { en: "Enable", zh: "开启" },
  "common.disable": { en: "Disable", zh: "关闭" },
  "common.enabled": { en: "Enabled", zh: "已开启" },
  "common.disabled": { en: "Disabled", zh: "已关闭" },
  "common.loading": { en: "Loading", zh: "加载中" },
  "common.never": { en: "Never", zh: "从未" },
  "common.noTags": { en: "No tags", zh: "无标签" },
  "common.notLoaded": { en: "Not loaded", zh: "未加载" },
  "common.copied": { en: "Copied.", zh: "已复制。" },
  "common.permissions": { en: "{count} permissions", zh: "{count} 个权限" },
  "common.scopes": { en: "{count} scopes", zh: "{count} 个资源范围" },
  "common.selected": { en: "Selected", zh: "已选择" },
  "common.yes": { en: "Yes", zh: "是" },
  "common.no": { en: "No", zh: "否" },
  "common.existing": { en: "Existing", zh: "已有" },
  "common.new": { en: "New", zh: "新增" },
  "common.removed": { en: "Removed", zh: "移除" },
  "common.none": { en: "None", zh: "无" },
  "common.unknown": { en: "Unknown", zh: "未知" },
  "nav.overview": { en: "Overview", zh: "总览" },
  "nav.nodes": { en: "Nodes", zh: "节点" },
  "nav.monitors": { en: "Monitors", zh: "监控" },
  "nav.targets": { en: "Targets", zh: "目标" },
  "nav.rules": { en: "Rules", zh: "规则" },
  "nav.settings": { en: "Settings", zh: "设置" },
  "nav.myRules": { en: "My Rules", zh: "我的规则" },
  "nav.usage": { en: "Usage", zh: "用量" },
  "nav.availableNodes": { en: "Available Nodes", zh: "可用节点" },
  "page.myForwardingRules": { en: "My forwarding rules", zh: "我的转发规则" },
  "overview.nodes": { en: "Nodes", zh: "节点" },
  "overview.monitors": { en: "Monitors", zh: "监控" },
  "overview.rules": { en: "Rules", zh: "规则" },
  "overview.targets": { en: "Targets", zh: "目标" },
  "overview.runtimeStatus": { en: "Runtime status", zh: "运行状态" },
  "overview.currentOrganization": { en: "Current organization", zh: "当前组织" },
  "overview.node": { en: "Node", zh: "节点" },
  "overview.status": { en: "Status", zh: "状态" },
  "overview.desired": { en: "Desired", zh: "期望配置" },
  "overview.applied": { en: "Applied", zh: "已应用配置" },
  "overview.lastSeen": { en: "Last seen", zh: "最后在线" },
  "usage.upload": { en: "Upload", zh: "上传" },
  "usage.download": { en: "Download", zh: "下载" },
  "usage.tcpConnections": { en: "TCP connections", zh: "TCP 连接" },
  "usage.usageByRule": { en: "Usage by rule", zh: "按规则查看用量" },
  "usage.refreshTraffic": { en: "Refresh traffic", zh: "刷新流量" },
  "usage.rule": { en: "Rule", zh: "规则" },
  "usage.udpPackets": { en: "UDP packets", zh: "UDP 包" },
  "settings.organization": { en: "Organization", zh: "组织" },
  "settings.editOrganization": { en: "Edit organization", zh: "编辑组织" },
  "settings.editOrganizationDescription": { en: "Update the current organization display name and slug.", zh: "更新当前组织的显示名称和标识。" },
  "settings.organizationUpdated": { en: "Organization updated.", zh: "组织已更新。" },
  "settings.slug": { en: "Slug", zh: "标识" },
  "settings.organizationNamePlaceholder": { en: "Network Operations", zh: "网络运营" },
  "rules.title": { en: "Rules", zh: "规则" },
  "rules.inventory": { en: "Rule inventory", zh: "规则清单" },
  "rules.description": { en: "Enabled rules are the only rules reserving listener ports.", zh: "只有已开启的规则会占用监听端口。" },
  "rules.createRule": { en: "Create rule", zh: "创建规则" },
  "rules.importRules": { en: "Import rules", zh: "导入规则" },
  "rules.enableSelected": { en: "Enable selected", zh: "开启选中规则" },
  "rules.disableSelected": { en: "Disable selected", zh: "关闭选中规则" },
  "rules.deleteSelected": { en: "Delete selected", zh: "删除选中规则" },
  "rules.exportSelected": { en: "Export selected", zh: "导出选中规则" },
  "rules.exportAll": { en: "Export all", zh: "导出全部规则" },
  "rules.selectedRules": { en: "Selected rules", zh: "选中规则" },
  "rules.allRules": { en: "All rules", zh: "全部规则" },
  "rules.batchResult": { en: "Batch {action} result", zh: "批量{action}结果" },
  "rules.batchSummary": { en: "{succeeded} succeeded, {failed} failed.", zh: "{succeeded} 条成功，{failed} 条失败。" },
  "rules.batchUpdated": { en: "{count} rules updated.", zh: "{count} 条规则已更新。" },
  "rules.name": { en: "Name", zh: "名称" },
  "rules.status": { en: "Status", zh: "状态" },
  "rules.listener": { en: "Listener", zh: "监听" },
  "rules.match": { en: "Match", zh: "匹配" },
  "rules.upstream": { en: "Upstream", zh: "上游" },
  "rules.traffic": { en: "Traffic", zh: "流量" },
  "rules.selectAll": { en: "Select all rules", zh: "选择全部规则" },
  "rules.selectRule": { en: "Select {name}", zh: "选择 {name}" },
  "rules.trafficValue": { en: "{upload} up / {download} down", zh: "上传 {upload} / 下载 {download}" },
  "rules.trafficButton": { en: "Traffic", zh: "流量" },
  "rules.copyRequested": { en: "Rule copy requested.", zh: "已请求复制规则。" },
  "rules.enableRequested": { en: "Rule enable requested.", zh: "已请求开启规则。" },
  "rules.disableRequested": { en: "Rule disable requested.", zh: "已请求关闭规则。" },
  "rules.exported": { en: "{label} exported.", zh: "{label}已导出。" },
  "rules.deleted": { en: "Rule deleted.", zh: "规则已删除。" },
  "rules.updated": { en: "Rule updated.", zh: "规则已更新。" },
  "rules.created": { en: "Rule created.", zh: "规则已创建。" },
  "rules.activeRules": { en: "Active rules", zh: "活跃规则" },
  "rules.totalRules": { en: "Total rules", zh: "规则总数" },
  "rules.availableNodeGroups": { en: "Available node groups", zh: "可用节点组" },
  "rules.scopes": { en: "Scopes", zh: "资源范围" },
  "rules.createDescription": { en: "Resource references come from authorized options only.", zh: "资源引用只能来自已授权选项。" },
  "rules.editRule": { en: "Edit rule", zh: "编辑规则" },
  "rules.editDescription": { en: "Changes are validated against the selected entry and upstream resources.", zh: "更改会按所选入口和上游资源重新校验。" },
  "rules.saveRule": { en: "Save rule", zh: "保存规则" },
  "rules.deleteRule": { en: "Delete rule", zh: "删除规则" },
  "rules.deleteQuestion": { en: "Delete {name}? This cannot be undone.", zh: "删除 {name}？此操作不可撤销。" },
  "rules.deleteThisQuestion": { en: "Delete this rule?", zh: "删除这条规则？" },
  "rules.deleteSelectedQuestion": { en: "Delete {count} selected rules? This cannot be undone.", zh: "删除选中的 {count} 条规则？此操作不可撤销。" },
  "rules.forwardingType": { en: "Forwarding type", zh: "转发方式" },
  "rules.protocol": { en: "Protocol", zh: "协议" },
  "rules.port": { en: "Port", zh: "端口" },
  "rules.nodeGroup": { en: "Node group", zh: "节点组" },
  "rules.listenIP": { en: "Listen IP", zh: "监听 IP" },
  "rules.matchType": { en: "Match type", zh: "匹配方式" },
  "rules.sniHostname": { en: "SNI hostname", zh: "SNI 主机名" },
  "rules.proxyProtocolIn": { en: "Proxy protocol in", zh: "入口 Proxy Protocol" },
  "rules.proxyProtocolOut": { en: "Proxy protocol out", zh: "出口 Proxy Protocol" },
  "rules.upstreamType": { en: "Upstream type", zh: "上游类型" },
  "rules.target": { en: "Target", zh: "目标" },
  "rules.targetGroup": { en: "Target group", zh: "目标组" },
  "rules.exportTitle": { en: "Rule export", zh: "规则导出" },
  "rules.exportDescription": { en: "Export uses the versioned rules export schema.", zh: "导出使用版本化规则导出 schema。" },
  "rules.exportJson": { en: "Export JSON", zh: "导出 JSON" },
  "rules.exportCount": { en: "{rules} rules, {targets} targets, {targetGroups} target groups.", zh: "{rules} 条规则，{targets} 个目标，{targetGroups} 个目标组。" },
  "rules.exportChooseAction": { en: "Choose a rule export action from the table options.", zh: "请从表格操作中选择导出动作。" },
  "rules.copyJson": { en: "Copy JSON", zh: "复制 JSON" },
  "rules.importDescription": { en: "Import validates permissions, scopes, listener availability, and upstream availability.", zh: "导入会校验权限、资源范围、监听可用性和上游可用性。" },
  "rules.importSource": { en: "Import source", zh: "导入内容" },
  "rules.importPortableDescription": { en: "Paste a complete rules.export.v1 payload.", zh: "粘贴完整的 rules.export.v1 内容。" },
  "rules.importNyanpassDescription": { en: "Paste one Nyanpass JSON object, a JSON array, or newline-delimited JSON objects. Rules with tls are skipped with errors.", zh: "粘贴一个 Nyanpass JSON 对象、JSON 数组或多行 JSON 对象。包含 tls 的规则会跳过并显示错误。" },
  "rules.importFailed": { en: "Import failed", zh: "导入失败" },
  "rules.dryRun": { en: "Dry run", zh: "试运行" },
  "rules.importResult": { en: "{mode} result", zh: "{mode}结果" },
  "rules.importSummary": { en: "{created} created, {skipped} skipped, {errors} errors, {warnings} warnings.", zh: "创建 {created} 条，跳过 {skipped} 条，错误 {errors} 条，警告 {warnings} 条。" },
  "rules.importErrors": { en: "Errors", zh: "错误" },
  "rules.importWarnings": { en: "Warnings", zh: "警告" },
  "rules.importErrorsToast": { en: "{count} import errors. {first}", zh: "{count} 条导入错误。{first}" },
  "rules.importWarningsToast": { en: "{count} import warnings. {first}", zh: "{count} 条导入警告。{first}" },
  "rules.dryRunCompleted": { en: "Dry-run completed.", zh: "试运行完成。" },
  "rules.importCompleted": { en: "Import completed.", zh: "导入完成。" },
  "rules.diagnostics": { en: "Diagnostics", zh: "诊断" },
  "rules.diagnosticsDescription": { en: "Realtime diagnostics for {name}.", zh: "{name} 的实时诊断。" },
  "rules.diagnosticsFailed": { en: "Diagnostics failed", zh: "诊断失败" },
  "rules.currentBandwidth": { en: "Current bandwidth", zh: "当前带宽" },
  "rules.latency": { en: "Latency", zh: "延迟" },
  "rules.latencyValue": { en: "{value} ms", zh: "{value} ms" },
  "shell.adminConsole": { en: "Admin Console", zh: "管理控制台" },
  "shell.userWorkspace": { en: "User Workspace", zh: "用户工作区" },
  "shell.user": { en: "User", zh: "用户" },
  "shell.admin": { en: "Admin", zh: "管理" },
  "shell.adminPermissionRequired": { en: "Additional permissions required.", zh: "需要额外权限。" },
  "shell.signOut": { en: "Sign out", zh: "退出登录" },
  "auth.signInDescription": { en: "Sign in with email and password.", zh: "使用邮箱和密码登录。" },
  "auth.signUpDescription": { en: "Create the first local account.", zh: "创建第一个本地账号。" },
  "auth.name": { en: "Name", zh: "姓名" },
  "auth.namePlaceholder": { en: "Operator", zh: "操作员" },
  "auth.email": { en: "Email", zh: "邮箱" },
  "auth.password": { en: "Password", zh: "密码" },
  "auth.signIn": { en: "Sign in", zh: "登录" },
  "auth.signUp": { en: "Sign up", zh: "注册" },
  "auth.createAccount": { en: "Create account", zh: "创建账号" },
  "auth.useExistingAccount": { en: "Use existing account", zh: "使用已有账号" },
  "auth.failedTitle": { en: "Authentication failed", zh: "认证失败" },
  "auth.failedDescription": { en: "Check the credentials and try again.", zh: "请检查账号密码后重试。" },
  "auth.signupDisabledTitle": { en: "This OSS instance has already been initialized.", zh: "此 OSS 实例已完成初始化。" },
  "auth.signupDisabledDescription": { en: "Only the owner account created during setup can sign in.", zh: "只有初始化时创建的 owner 账号可以登录。" },
  "auth.ownerOnlyTitle": { en: "Owner account required", zh: "需要 owner 账号" },
  "auth.ownerOnlyDescription": { en: "This OSS instance only allows the owner account created during setup.", zh: "此 OSS 实例仅允许初始化时创建的 owner 账号访问。" },
  "setup.organizationName": { en: "Organization name", zh: "组织名称" },
  "setup.organizationSlug": { en: "Organization slug", zh: "组织标识" },
  "setup.organizationNamePlaceholder": { en: "Network Operations", zh: "网络运营" },
  "setup.organizationSlugHelp": { en: "Lowercase letters, numbers, and hyphens.", zh: "只能使用小写字母、数字和连字符。" },
  "setup.createOrganization": { en: "Create organization", zh: "创建组织" },
  "setup.organizationCreated": { en: "Organization created.", zh: "组织已创建。" },
  "resource.selectResource": { en: "Select resource", zh: "选择资源" },
  "resource.noAvailableOptions": { en: "No available options", zh: "没有可用选项" },
  "resource.noAuthorizedResources": { en: "No authorized resources are currently available.", zh: "当前没有已授权且可用的资源。" },
  "field.name": { en: "Name", zh: "名称" },
  "field.description": { en: "Description", zh: "描述" },
  "field.tags": { en: "Tags", zh: "标签" },
  "field.format": { en: "Import format", zh: "导入格式" },
  "field.source_text": { en: "Import source", zh: "导入内容" },
  "field.entry.node_group_id_present": { en: "Node group", zh: "节点组" },
  "field.entry.listen_ip_present": { en: "Listen IP", zh: "监听 IP" },
  "field.node_group_id": { en: "Node group", zh: "节点组" },
  "field.listen_ip": { en: "Listen IP", zh: "监听 IP" },
  "field.protocol": { en: "Protocol", zh: "协议" },
  "field.port": { en: "Port", zh: "端口" },
  "field.forwarding_type": { en: "Forwarding type", zh: "转发方式" },
  "field.match.type": { en: "Match type", zh: "匹配方式" },
  "field.match.sni_hostname": { en: "SNI hostname", zh: "SNI 主机名" },
  "field.proxy_protocol.in": { en: "Proxy Protocol in", zh: "入口 Proxy Protocol" },
  "field.proxy_protocol.out": { en: "Proxy Protocol out", zh: "出口 Proxy Protocol" },
  "field.upstream.type": { en: "Upstream type", zh: "上游类型" },
  "nodes.nodeGroups": { en: "Node groups", zh: "节点组" },
  "nodes.nodes": { en: "Nodes", zh: "节点" },
  "nodes.online": { en: "Online", zh: "在线" },
  "nodes.installCommand": { en: "Install command", zh: "安装命令" },
  "nodes.availableNodes": { en: "Available nodes", zh: "可用节点" },
  "nodes.groups": { en: "Groups", zh: "分组" },
  "nodes.listenIPs": { en: "Listen IPs", zh: "监听 IP" },
  "nodes.ports": { en: "Ports", zh: "端口" },
  "nodes.config": { en: "Config", zh: "配置" },
  "nodes.token": { en: "Token", zh: "令牌" },
  "nodes.createNodeGroup": { en: "Create node group", zh: "创建节点组" },
  "nodes.createNodeGroupDescription": { en: "Node groups define where rules can listen.", zh: "节点组定义规则可以监听的位置。" },
  "nodes.nodeGroupNamePlaceholder": { en: "Edge Group A", zh: "入口节点组 A" },
  "nodes.nodeGroupDescriptionPlaceholder": { en: "Customer-facing entry nodes", zh: "面向客户的入口节点" },
  "nodes.createNode": { en: "Create node", zh: "创建节点" },
  "nodes.createNodeDescription": { en: "Listeners and port ranges are validated by the control plane.", zh: "监听地址和端口范围由控制面校验。" },
  "nodes.nodeGroupCreated": { en: "Node group created.", zh: "节点组已创建。" },
  "nodes.nodeCreated": { en: "Node created.", zh: "节点已创建。" },
  "nodes.registrationTokenCreated": { en: "Registration token created.", zh: "注册令牌已创建。" },
  "nodes.listenIPLabel": { en: "Listen IP label", zh: "监听 IP 标签" },
  "nodes.startPort": { en: "Start port", zh: "起始端口" },
  "nodes.endPort": { en: "End port", zh: "结束端口" },
  "nodes.publicDescription": { en: "Public description", zh: "公开说明" },
  "nodes.nodeMetrics": { en: "Node metrics", zh: "节点指标" },
  "nodes.nodeMetricsDescription": { en: "Latest realtime metrics from every visible node.", zh: "每个可见节点的最新实时指标。" },
  "nodes.bandwidth": { en: "Bandwidth", zh: "带宽" },
  "nodes.uptime": { en: "Uptime", zh: "运行时间" },
  "nodes.bootTime": { en: "Boot time", zh: "启动时间" },
  "monitors.registrationTokenCreated": { en: "Registration token created.", zh: "注册令牌已创建。" },
  "monitors.installCommand": { en: "Install command", zh: "安装命令" },
  "monitors.monitors": { en: "Monitors", zh: "监控" },
  "monitors.groups": { en: "Groups", zh: "分组" },
  "monitors.config": { en: "Config", zh: "配置" },
  "monitors.token": { en: "Token", zh: "令牌" },
  "monitors.createGroup": { en: "Create monitor group", zh: "创建监控组" },
  "monitors.createGroupDescription": { en: "Monitor groups organize probe agents.", zh: "监控组用于组织探测 Agent。" },
  "monitors.groupNamePlaceholder": { en: "Probe Group A", zh: "探测组 A" },
  "monitors.groupDescriptionPlaceholder": { en: "Regional monitors", zh: "区域监控节点" },
  "monitors.createMonitor": { en: "Create monitor", zh: "创建监控" },
  "monitors.createMonitorDescription": { en: "Monitor agents connect back to the control plane after registration.", zh: "监控 Agent 注册后会连接回控制面。" },
  "monitors.groupCreated": { en: "Monitor group created.", zh: "监控组已创建。" },
  "monitors.monitorCreated": { en: "Monitor created.", zh: "监控已创建。" },
  "monitors.monitorGroups": { en: "Monitor groups", zh: "监控组" },
  "targets.targets": { en: "Targets", zh: "目标" },
  "targets.visibleDescription": { en: "Targets visible in this organization.", zh: "当前组织可见的目标。" },
  "targets.upstreamDescription": { en: "External upstream endpoints.", zh: "外部上游端点。" },
  "targets.address": { en: "Address", zh: "地址" },
  "targets.groups": { en: "Groups", zh: "分组" },
  "targets.noGroups": { en: "No groups", zh: "无分组" },
  "targets.targetGroups": { en: "Target groups", zh: "目标组" },
  "targets.poolDescription": { en: "Priority + iphash target pools.", zh: "基于优先级和 iphash 的目标池。" },
  "targets.scheduler": { en: "Scheduler", zh: "调度器" },
  "targets.members": { en: "Members", zh: "成员" },
  "targets.noTargets": { en: "No targets", zh: "无目标" },
  "targets.unknownTarget": { en: "Unknown target", zh: "未知目标" },
  "targets.memberDisabled": { en: "disabled", zh: "已关闭" },
  "targets.createTarget": { en: "Create target", zh: "创建目标" },
  "targets.createTargetDescription": { en: "External upstream endpoints can be entered directly.", zh: "外部上游端点可以直接填写。" },
  "targets.createTargetGroup": { en: "Create target group", zh: "创建目标组" },
  "targets.createTargetGroupDescription": { en: "Existing targets are selected from authorized options.", zh: "已有目标必须从授权选项中选择。" },
  "targets.targetCreated": { en: "Target created.", zh: "目标已创建。" },
  "targets.targetGroupCreated": { en: "Target group created.", zh: "目标组已创建。" },
  "targets.editTarget": { en: "Edit target", zh: "编辑目标" },
  "targets.editTargetDescription": { en: "Update the upstream endpoint and group memberships.", zh: "更新上游端点和目标组归属。" },
  "targets.membershipLoading": { en: "Target group memberships are still loading.", zh: "目标组归属仍在加载。" },
  "targets.targetUpdated": { en: "Target updated.", zh: "目标已更新。" },
  "targets.deleteTarget": { en: "Delete target", zh: "删除目标" },
  "targets.deleteTargetQuestion": { en: "Delete {name}? Rules or groups using it may block deletion.", zh: "删除 {name}？使用它的规则或目标组可能会阻止删除。" },
  "targets.targetDeleted": { en: "Target deleted.", zh: "目标已删除。" },
  "targets.editTargetGroup": { en: "Edit target group", zh: "编辑目标组" },
  "targets.editTargetGroupDescription": { en: "Update the target group name, description, and members.", zh: "更新目标组名称、描述和成员。" },
  "targets.targetGroupUpdated": { en: "Target group updated.", zh: "目标组已更新。" },
  "targets.deleteTargetGroup": { en: "Delete target group", zh: "删除目标组" },
  "targets.deleteTargetGroupQuestion": { en: "Delete {name}? Rules using it may block deletion.", zh: "删除 {name}？使用它的规则可能会阻止删除。" },
  "targets.targetGroupDeleted": { en: "Target group deleted.", zh: "目标组已删除。" },
  "targets.host": { en: "Host", zh: "主机" },
  "targets.groupNamePlaceholder": { en: "origin-pool", zh: "origin-pool" },
  "targets.groupDescriptionPlaceholder": { en: "Primary pool", zh: "主目标池" },
  "targets.defaultMemberPriority": { en: "Default member priority", zh: "默认成员优先级" },
  "targets.createGroup": { en: "Create group", zh: "创建组" },
  "enum.PORTABLE_EXPORT": { en: "Portable export", zh: "便携导出" },
  "enum.NYANPASS": { en: "Nyanpass", zh: "Nyanpass" },
  "enum.DIRECT": { en: "Direct", zh: "直连" },
  "enum.TUNNEL": { en: "Tunnel", zh: "隧道" },
  "enum.TCP": { en: "TCP", zh: "TCP" },
  "enum.UDP": { en: "UDP", zh: "UDP" },
  "enum.TCP_UDP": { en: "TCP + UDP", zh: "TCP + UDP" },
  "enum.ANY_INBOUND": { en: "Any inbound", zh: "任意入站" },
  "enum.TLS_SNI": { en: "TLS SNI", zh: "TLS SNI" },
  "enum.TARGET": { en: "Target", zh: "目标" },
  "enum.TARGET_GROUP": { en: "Target group", zh: "目标组" },
  "enum.ENABLE": { en: "enable", zh: "开启" },
  "enum.DISABLE": { en: "disable", zh: "关闭" },
  "enum.DELETE": { en: "delete", zh: "删除" },
  "enum.NONE": { en: "None", zh: "无" },
  "enum.V1": { en: "V1", zh: "V1" },
  "enum.V2": { en: "V2", zh: "V2" },
  "status.ONLINE": { en: "Online", zh: "在线" },
  "status.OFFLINE": { en: "Offline", zh: "离线" },
  "status.ENABLED": { en: "Enabled", zh: "已开启" },
  "status.DISABLED": { en: "Disabled", zh: "已关闭" },
  "status.ACTIVE": { en: "Active", zh: "活跃" },
  "status.PENDING": { en: "Pending", zh: "待连接" },
  "status.FAILED": { en: "Failed", zh: "失败" },
  "status.SUCCEEDED": { en: "Succeeded", zh: "成功" },
  "status.OK": { en: "OK", zh: "正常" },
  "status.NO_RUNTIME_METRICS": { en: "No runtime metrics", zh: "无运行态指标" },
  "error.requestFailed": { en: "The request failed.", zh: "请求失败。" },
  "error.withCode": { en: "The request failed. Code: {code}.", zh: "请求失败。错误码：{code}。" },
  "error.UNAUTHENTICATED": { en: "Authentication is required.", zh: "请先登录。" },
  "error.FORBIDDEN": { en: "You do not have permission to perform this action.", zh: "你没有权限执行此操作。" },
  "error.NOT_FOUND": { en: "The requested resource was not found.", zh: "请求的资源不存在。" },
  "error.CONFLICT": { en: "The request conflicts with the current environment state.", zh: "请求与当前环境状态冲突。" },
  "error.VALIDATION_FAILED": { en: "The request payload is invalid.", zh: "请求内容无效。" },
  "error.QUOTA_EXCEEDED": { en: "The request exceeds the configured quota.", zh: "请求超出了当前配额。" },
  "error.OSS_SIGNUP_DISABLED": { en: "This OSS instance has already been initialized. New account registration is closed.", zh: "此 OSS 实例已完成初始化，不能再注册新账号。" },
  "error.OSS_OWNER_REQUIRED": { en: "This OSS instance only allows the owner account created during setup.", zh: "此 OSS 实例仅允许初始化时创建的 owner 登录。" },
  "error.RULE_PORT_CONFLICT": { en: "The listen port is already reserved by another enabled rule.", zh: "监听端口已被其他开启中的规则占用。" },
  "error.RULE_DUPLICATE_SNI": { en: "Another enabled rule already uses this SNI on the selected listener.", zh: "所选入口上已有开启中的规则使用该 SNI。" },
  "error.INTERNAL_ERROR": { en: "An internal error occurred.", zh: "服务内部错误。" },
  "error.validationField": { en: "{field} is invalid.", zh: "{field}填写无效。" },
  "error.validationRequired": { en: "{field} is required.", zh: "请填写{field}。" },
  "error.supportedFormats": { en: "Supported formats: {formats}.", zh: "支持格式：{formats}。" },
  "error.actualValue": { en: "Actual value: {value}.", zh: "当前值：{value}。" },
  "error.expectedValue": { en: "Expected value: {value}.", zh: "期望值：{value}。" },
  "error.portRange": { en: "Allowed port range: {min}-{max}.", zh: "允许端口范围：{min}-{max}。" },
  "import.issue.nyanpass": { en: "Nyanpass rule {index} was not imported: {reason}", zh: "第 {index} 条 Nyanpass 规则未导入：{reason}" },
  "import.issue.rules": { en: "Rule {index} was not imported: {reason}", zh: "第 {index} 条规则未导入：{reason}" },
  "import.issue.rulesDisabled": { en: "Rule {index} was imported disabled: {reason}", zh: "第 {index} 条规则已导入但已关闭：{reason}" },
  "import.issue.targets": { en: "Target {index} was not imported: {reason}", zh: "第 {index} 个目标未导入：{reason}" },
  "import.issue.targetGroups": { en: "Target group {index} was not imported: {reason}", zh: "第 {index} 个目标组未导入：{reason}" },
  "import.issue.generic": { en: "Import issue {index}: {reason}", zh: "第 {index} 条导入问题：{reason}" },
  "import.reason.IMPORT_NYANPASS_TLS_UNSUPPORTED": { en: "the current runtime does not support Nyanpass TLS/origin fetch semantics.", zh: "当前运行态不支持 Nyanpass TLS/origin fetch 语义。" },
  "import.reason.IMPORT_NYANPASS_INVALID_DEST": { en: "dest must be host:port with port 1-65535.", zh: "目标地址必须是 host:port，端口必须在 1-65535 之间。" },
  "import.reason.IMPORT_NYANPASS_UNSUPPORTED_DEST_POLICY": { en: "dest_policy only supports ip_hash.", zh: "dest_policy 仅支持 ip_hash。" },
  "import.reason.IMPORT_NYANPASS_INVALID_ACCEPT_PROXY_PROTOCOL": { en: "accept_proxy_protocol must be 0, 1, or 2.", zh: "accept_proxy_protocol 只能是 0、1 或 2。" },
  "import.reason.IMPORT_NYANPASS_INVALID_PROXY_PROTOCOL": { en: "proxy_protocol must be 0, 1, or 2.", zh: "proxy_protocol 只能是 0、1 或 2。" },
  "import.reason.IMPORT_NYANPASS_INVALID_NAME": { en: "rule name is required and must be at most 120 characters.", zh: "规则名称不能为空，且长度不能超过 120 个字符。" },
  "import.reason.IMPORT_NYANPASS_INVALID_LISTEN_PORT": { en: "listen_port must be between 1 and 65535.", zh: "listen_port 必须在 1-65535 之间。" },
  "import.reason.IMPORT_NYANPASS_DEST_REQUIRED": { en: "dest must contain at least one host:port.", zh: "dest 至少需要一个 host:port。" },
  "import.reason.IMPORT_RULE_DISABLED": { en: "the rule conflicts with the selected entry or existing enabled rules.", zh: "规则与所选入口或已有开启规则冲突。" },
  "import.reason.IMPORT_RULE_INVALID": { en: "the rule payload could not be resolved in this environment.", zh: "规则内容无法在当前环境解析。" },
  "import.reason.IMPORT_TARGET_INVALID": { en: "target data is invalid.", zh: "目标数据无效。" },
  "import.reason.IMPORT_TARGET_DUPLICATE_REF": { en: "target ref is duplicated.", zh: "目标 ref 重复。" },
  "import.reason.IMPORT_TARGET_CREATE_FORBIDDEN": { en: "you do not have permission to create missing targets.", zh: "你没有权限创建缺失目标。" },
  "import.reason.IMPORT_TARGET_GROUP_INVALID": { en: "target group data is invalid.", zh: "目标组数据无效。" },
  "import.reason.IMPORT_TARGET_GROUP_DUPLICATE_REF": { en: "target group ref is duplicated.", zh: "目标组 ref 重复。" },
  "import.reason.IMPORT_TARGET_GROUP_CREATE_FORBIDDEN": { en: "you do not have permission to create missing target groups.", zh: "你没有权限创建缺失目标组。" },
  "import.reason.IMPORT_TARGET_GROUP_MEMBER_INVALID": { en: "target group member references an unresolved or duplicate target.", zh: "目标组成员引用了无法解析或重复的目标。" },
  "import.reason.FORBIDDEN": { en: "permission denied.", zh: "权限不足。" },
  "import.reason.VALIDATION_FAILED": { en: "validation failed.", zh: "校验失败。" },
  "import.reason.QUOTA_EXCEEDED": { en: "quota exceeded.", zh: "超出配额。" },
  "import.reason.RULE_PORT_CONFLICT": { en: "the port is already reserved.", zh: "端口已被占用。" },
  "import.reason.RULE_DUPLICATE_SNI": { en: "the SNI conflicts with another enabled rule.", zh: "SNI 与其他开启规则冲突。" },
};

const zhCNMessages = Object.fromEntries(Object.entries(commonMessages).map(([key, value]) => [key, value.zh])) as Record<string, string>;
const enMessages = Object.fromEntries(Object.entries(commonMessages).map(([key, value]) => [key, value.en])) as Record<string, string>;

export const messages: Record<Locale, Record<MessageKey, string>> = {
  "zh-CN": zhCNMessages,
  en: enMessages,
};

export function resolveLocale(explicitLocale?: string | null, browserLocales: readonly string[] = []): Locale {
  const explicit = normalizeLocale(explicitLocale);
  if (explicit) {
    return explicit;
  }
  for (const browserLocale of browserLocales) {
    const locale = normalizeLocale(browserLocale);
    if (locale) {
      return locale;
    }
  }
  return "zh-CN";
}

export function localeCandidatesFromAcceptLanguage(value?: string | null): string[] {
  if (!value) {
    return [];
  }
  return value
    .split(",")
    .map((part) => part.trim().split(";")[0]?.trim())
    .filter((part): part is string => Boolean(part));
}

export function formatMessage(locale: Locale, key: MessageKey, params: MessageParams = {}): string {
  const template = messages[locale][key] ?? messages.en[key] ?? key;
  return template.replace(/\{([a-zA-Z0-9_]+)\}/g, (_, name: string) => String(params[name] ?? ""));
}

export function localizeEnum(value: string | undefined | null, locale: Locale): string {
  if (!value) {
    return formatMessage(locale, "common.none");
  }
  const key = `enum.${value}` as MessageKey;
  return messages[locale][key] ?? value;
}

export function localizeStatus(value: string | undefined | null, locale: Locale): string {
  if (!value) {
    return formatMessage(locale, "common.unknown");
  }
  const key = `status.${value.toUpperCase()}` as MessageKey;
  return messages[locale][key] ?? value;
}

export function localizeControlError(error: unknown, locale: Locale): string {
  if (isControlAPIErrorLike(error)) {
    const base = localizeErrorCode(error.code, locale);
    const detail = localizeControlErrorDetails(error.details, locale);
    return detail ? `${base}${locale === "zh-CN" ? "" : " "}${detail}` : base;
  }
  return formatMessage(locale, "error.requestFailed");
}

export function localizeImportIssue(issue: RuleImportIssue, locale: Locale): string {
  const reason = localizeImportReason(issue, locale);
  const index = typeof issue.index === "number" ? issue.index + 1 : "?";
  if (issue.code === "IMPORT_RULE_DISABLED") {
    return formatMessage(locale, "import.issue.rulesDisabled", { index, reason });
  }
  switch (issue.scope) {
    case "nyanpass":
      return formatMessage(locale, "import.issue.nyanpass", { index, reason });
    case "rules":
      return formatMessage(locale, "import.issue.rules", { index, reason });
    case "targets":
      return formatMessage(locale, "import.issue.targets", { index, reason });
    case "target_groups":
      return formatMessage(locale, "import.issue.targetGroups", { index, reason });
    default:
      return formatMessage(locale, "import.issue.generic", { index, reason });
  }
}

function normalizeLocale(value?: string | null): Locale | null {
  const locale = value?.trim().toLowerCase();
  if (!locale) {
    return null;
  }
  if (locale === "en" || locale.startsWith("en-")) {
    return "en";
  }
  if (locale === "zh" || locale.startsWith("zh-")) {
    return "zh-CN";
  }
  return null;
}

function isControlAPIErrorLike(error: unknown): error is { code: string; message?: string; details?: Record<string, unknown> } {
  return typeof error === "object" && error !== null && "code" in error && typeof (error as { code?: unknown }).code === "string";
}

function localizeErrorCode(code: string, locale: Locale): string {
  const key = `error.${code}` as MessageKey;
  return messages[locale][key] ?? formatMessage(locale, "error.withCode", { code });
}

function localizeControlErrorDetails(details: Record<string, unknown> | undefined, locale: Locale): string {
  if (!details) {
    return "";
  }
  const pieces: string[] = [];
  const field = typeof details.field === "string" ? details.field : "";
  if (field) {
    pieces.push(formatMessage(locale, "error.validationField", { field: localizeField(field, locale) }));
  }
  if (Array.isArray(details.supported_formats)) {
    pieces.push(formatMessage(locale, "error.supportedFormats", { formats: details.supported_formats.map((value) => localizeEnum(String(value), locale)).join(locale === "zh-CN" ? "、" : ", ") }));
  }
  if (details.actual !== undefined && details.actual !== null && details.actual !== "") {
    pieces.push(formatMessage(locale, "error.actualValue", { value: localizeDetailValue(details.actual, locale) }));
  }
  if (details.expected !== undefined && details.expected !== null && details.expected !== "") {
    pieces.push(formatMessage(locale, "error.expectedValue", { value: localizeDetailValue(details.expected, locale) }));
  }
  if (details.min !== undefined && details.max !== undefined) {
    pieces.push(formatMessage(locale, "error.portRange", { min: localizeDetailValue(details.min, locale), max: localizeDetailValue(details.max, locale) }));
  }
  return pieces.join(locale === "zh-CN" ? "" : " ");
}

function localizeField(field: string, locale: Locale): string {
  const key = `field.${field}` as MessageKey;
  return messages[locale][key] ?? field;
}

function localizeImportReason(issue: RuleImportIssue, locale: Locale): string {
  const reasonCode = typeof issue.details?.reason_code === "string" ? issue.details.reason_code : issue.code;
  const importReasonKey = `import.reason.${reasonCode}` as MessageKey;
  const issueReasonKey = `import.reason.${issue.code}` as MessageKey;
  const reason = messages[locale][importReasonKey] ?? messages.en[importReasonKey] ?? messages[locale][issueReasonKey] ?? messages.en[issueReasonKey];
  if (reason) {
    return reason;
  }
  return formatMessage(locale, "error.withCode", { code: issue.code });
}

function localizeDetailValue(value: unknown, locale: Locale): string {
  if (Array.isArray(value)) {
    return value.map((item) => localizeDetailValue(item, locale)).join(locale === "zh-CN" ? "、" : ", ");
  }
  if (typeof value === "string") {
    return localizeEnum(value, locale);
  }
  return String(value);
}
