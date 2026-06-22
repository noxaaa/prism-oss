export type InitialUser = {
  id: string;
  email: string;
  name: string;
} | null;

export type APIEnvelope<T> = {
  data?: T;
  error?: {
    code: string;
    message?: string;
    details?: Record<string, unknown>;
  };
};

export type ResourceOption = {
  value: string;
  label: string;
  disabled?: boolean;
  disabled_reason?: string;
  disabledReason?: string;
};

export type Organization = {
  id: string;
  name: string;
  slug: string;
};

export type Member = {
  id: string;
  user_id: string;
  email: string;
  name: string;
  status: string;
  role_ids?: string[];
};

export type ResourceScope = {
  resource_type: string;
  resource_id: string;
  access_level: string;
};

export type Role = {
  id: string;
  key: string;
  name: string;
  description: string;
  is_system: boolean;
  permissions: string[];
  resource_scopes: ResourceScope[];
};

export type ControlSession = {
  user: {
    id: string;
    email: string;
    name: string;
  };
  organization?: Organization;
  organizations?: Organization[];
  member?: Member;
  roles?: Role[];
  permissions?: string[];
  resource_scopes?: ResourceScope[];
};

export type NodeGroup = {
  id: string;
  name: string;
  description: string;
};

export type NodeListenIP = {
  id?: string;
  listen_ip: string;
  display_name: string;
  enabled: boolean;
};

export type NodePortRange = {
  id?: string;
  protocol: "TCP" | "UDP" | string;
  start_port: number;
  end_port: number;
  enabled: boolean;
};

export type NodeResource = {
  id: string;
  name: string;
  status: string;
  public_description: string;
  desired_config_version: number;
  applied_config_version: number;
  config_status: string;
  config_error_message?: string;
  config_status_updated_at?: string;
  last_seen_at?: string;
  registered_at?: string;
  agent_version: string;
  agent_commit: string;
  agent_build_time: string;
  agent_auto_update_enabled: boolean;
  desired_agent_version: string;
  agent_update_status: string;
  agent_update_error?: string;
  agent_update_started_at?: string;
  agent_update_finished_at?: string;
  group_ids: string[];
  listen_ips: NodeListenIP[];
  port_ranges: NodePortRange[];
};

export type MonitorGroup = {
  id: string;
  name: string;
  description: string;
};

export type Monitor = {
  id: string;
  name: string;
  status: string;
  desired_config_version: number;
  applied_config_version: number;
  last_seen_at?: string;
  registered_at?: string;
  group_ids: string[];
};

export type RegistrationToken = {
  token_id: string;
  token?: string;
  agent_type: string;
  agent_id: string;
  expires_at: string;
  used_at?: string;
  revoked_at?: string;
  created_at: string;
  created_by_user_id?: string;
  install_command?: string;
};

export type HealthCheck = {
  id: string;
  name: string;
  probe_type: "ICMP" | "TCP_PORT" | "HTTP" | string;
  interval_seconds: number;
  timeout_seconds: number;
  config: Record<string, unknown>;
  enabled: boolean;
  targets: HealthCheckTarget[];
  monitor_scopes: HealthMonitorScope[];
};

export type HealthCheckTarget = {
  id: string;
  scope_type: string;
  target_id: string;
  target_group_id?: string;
  target_name: string;
  target_host: string;
  target_port: number;
};

export type HealthMonitorScope = {
  id: string;
  scope_type: string;
  monitor_id?: string;
  monitor_group_id?: string;
};

export type HealthResult = {
  id: string;
  health_check_id: string;
  health_check_target_id: string;
  monitor_id: string;
  target_id: string;
  status: string;
  latency_ms?: number;
  error_message?: string;
  observed_at: string;
  created_at: string;
};

export type DNSCredential = {
  id: string;
  name: string;
  provider: string;
};

export type DNSRecord = {
  id: string;
  dns_credential_id: string;
  zone: string;
  record_name: string;
  record_type: string;
  managed_mode: string;
  desired_values: string[];
  last_applied_values: string[];
  last_applied_at?: string;
};

export type Target = {
  id: string;
  name: string;
  host: string;
  port: number;
  enabled: boolean;
};

export type TargetGroupMember = {
  target_id: string;
  priority: number;
  enabled: boolean;
};

export type TargetGroup = {
  id: string;
  name: string;
  description: string;
  scheduler: string;
  members: TargetGroupMember[] | null;
};

export type Rule = {
  id: string;
  name: string;
  status: string;
  enabled: boolean;
  tags: string[] | null;
  node_group_id: string;
  listen_ip: string;
  forwarding_type: "DIRECT" | "TUNNEL" | string;
  protocol: "TCP" | "UDP" | "TCP_UDP" | string;
  port: number;
  match: {
    type: "ANY_INBOUND" | "TLS_SNI" | "FEATURE" | string;
    sni_hostname?: string;
  };
  proxy_protocol: {
    in: string;
    out: string;
  };
  upstream: {
    type: "TARGET" | "TARGET_GROUP" | string;
    target_id?: string;
    target_group_id?: string;
  };
  owner_user_id: string;
  config_version: number;
  connect_info?: {
    protocol: string;
    listen_port: number;
    listen_ip: string;
    node_descriptions: string[] | null;
  };
};

export type RuleTraffic = {
  upload_bytes: number;
  download_bytes: number;
  tcp_connections: number;
  udp_packets: number;
  limit_bytes: number;
  limit_mode: string;
};

export type RuleDiagnostics = {
  rule_id: string;
  generated_at: string;
  bandwidth_bps: number;
  upload_bytes: number;
  download_bytes: number;
  targets: RuleTargetDiagnostics[] | null;
};

export type RuleTargetDiagnostics = {
  target_id: string;
  name: string;
  address: string;
  status: string;
  last_seen_at?: string;
  latency_ms: number | null;
  bandwidth_bps: number | null;
  upload_bytes: number;
  download_bytes: number;
  tcp_connections: number;
  udp_packets_per_second: number;
};

export type RuleExportPayload = {
  schema_version: string;
  exported_at: string;
  rules: PortableRule[];
  targets: PortableTarget[];
  target_groups: PortableTargetGroup[];
};

export type PortableRule = {
  name: string;
  tags: string[] | null;
  forwarding_type: "DIRECT" | "TUNNEL" | string;
  protocol: "TCP" | "UDP" | "TCP_UDP" | string;
  port: number;
  match: {
    type: "ANY_INBOUND" | "TLS_SNI" | string;
    sni_hostname?: string;
  };
  proxy_protocol: {
    in: string;
    out: string;
  };
  upstream: {
    type: "TARGET" | "TARGET_GROUP" | string;
    target_ref?: string;
    target_group_ref?: string;
  };
};

export type PortableTarget = {
  ref: string;
  name: string;
  host: string;
  port: number;
  enabled: boolean;
};

export type PortableTargetGroup = {
  ref: string;
  name: string;
  description: string;
  scheduler: string;
  members: Array<{
    target_ref: string;
    priority: number;
    enabled: boolean;
  }> | null;
};

export type RuleImportRequest = {
  entry: {
    node_group_id: string;
    listen_ip: string;
  };
  format: "PORTABLE_EXPORT" | "NYANPASS" | string;
  source_text: string;
};

export type RuleImportResult = {
  dry_run: boolean;
  created: number;
  skipped: number;
  errors: RuleImportIssue[] | null;
  warnings: RuleImportIssue[] | null;
};

export type RuleImportIssue = {
  code: string;
  scope: string;
  index?: number;
  details?: Record<string, unknown>;
};

export type RuleBatchResult = {
  action: "ENABLE" | "DISABLE" | "DELETE" | string;
  total: number;
  succeeded: number;
  failed: number;
  results: Array<{
    rule_id: string;
    status: "SUCCEEDED" | "FAILED" | string;
    error?: {
      code: string;
      message: string;
      details?: Record<string, unknown>;
    };
    warning?: string;
  }> | null;
};

export type AgentMetrics = {
  status?: string;
  tcp_connections?: number;
  udp_packets_per_second?: number;
  bandwidth_bps?: number;
  cpu_percent?: number;
  ram_used_bytes?: number;
  ram_total_bytes?: number;
  upload_bytes?: number;
  download_bytes?: number;
  uptime_seconds?: number;
  boot_time?: string;
  last_seen_at?: string;
  desired_config_version?: number;
  applied_config_version?: number;
};
