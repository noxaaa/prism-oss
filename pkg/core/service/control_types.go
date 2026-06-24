package service

import (
	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
)

type BootstrapInput struct {
	OrganizationName string
	OrganizationSlug string
	SourceIP         string
}

type WebUserIdentity struct {
	UserID string
	Email  string
	Name   string
}

type InternalIdentity struct {
	UserID         string
	OrganizationID string
	MemberID       string
	Roles          []string
	Permissions    []string
	ResourceScopes []ResourceScopePayload
	SourceIP       string
}

type SessionResult struct {
	Created        bool                   `json:"created"`
	User           UserPayload            `json:"user"`
	Organization   OrganizationPayload    `json:"organization"`
	Organizations  []OrganizationPayload  `json:"organizations"`
	Member         MemberPayload          `json:"member"`
	Roles          []RolePayload          `json:"roles"`
	Permissions    []string               `json:"permissions"`
	ResourceScopes []ResourceScopePayload `json:"resource_scopes"`
}

type UserPayload struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type OrganizationPayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type MemberPayload struct {
	ID      string   `json:"id"`
	UserID  string   `json:"user_id"`
	Email   string   `json:"email"`
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	RoleIDs []string `json:"role_ids"`
}

type RolePayload struct {
	ID             string                 `json:"id"`
	Key            string                 `json:"key"`
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	IsSystem       bool                   `json:"is_system"`
	Permissions    []string               `json:"permissions"`
	ResourceScopes []ResourceScopePayload `json:"resource_scopes"`
}

type ResourceScopePayload struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	AccessLevel  string `json:"access_level"`
}

type MemberMutationInput struct {
	Email           string
	RoleIDs         []string
	RoleIDsProvided bool
	Status          string
}

type RoleMutationInput struct {
	Name           string
	Description    string
	Permissions    []string
	ResourceScopes []ResourceScopePayload
}

type GroupMutationInput struct {
	Name        string
	Description string
}

type NodeMutationInput struct {
	Name                        string
	NameProvided                bool
	GroupIDs                    []string
	GroupIDsProvided            bool
	ListenIPs                   []NodeListenIPInput
	ListenIPsProvided           bool
	PortRanges                  []NodePortRangeInput
	PortRangesProvided          bool
	DNSPublishAddresses         []NodeDNSPublishAddressInput
	DNSPublishAddressesProvided bool
	PublicDescription           string
	PublicDescriptionProvided   bool
}

type NodeListenIPInput struct {
	ListenIP    string
	DisplayName string
}

type NodePortRangeInput struct {
	Protocol  string
	StartPort int
	EndPort   int
}

type NodeDNSPublishAddressInput struct {
	AddressType string
	Address     string
	Enabled     bool
}

type MonitorMutationInput struct {
	Name             string
	NameProvided     bool
	GroupIDs         []string
	GroupIDsProvided bool
}

type HealthCheckMutationInput struct {
	Name            string
	ProbeType       string
	IntervalSeconds int
	TimeoutSeconds  int
	Enabled         bool
	TargetScope     HealthTargetScopeInput
	MonitorScope    HealthMonitorScopeInput
	ConfigJSON      string
}

type HealthTargetScopeInput struct {
	Type          string
	TargetIDs     []string
	TargetGroupID string
}

type HealthMonitorScopeInput struct {
	Type           string
	MonitorID      string
	MonitorGroupID string
}

type HealthResultInput struct {
	HealthCheckID       string
	HealthCheckTargetID string
	TargetID            string
	Status              string
	LatencyMS           int
	ErrorMessage        string
	ObservedAt          string
}

type HealthEvaluationRuleMutationInput struct {
	HealthCheckID  string
	Name           string
	Enabled        bool
	ExpressionJSON string
	Events         []HealthEventMutationInput
}

type HealthEventMutationInput struct {
	EventType  string
	ConfigJSON string
	Secret     string
	Enabled    bool
}

type DNSCredentialMutationInput struct {
	Name     string
	Provider string
	Secret   string
}

type DNSManagedRecordMutationInput struct {
	DNSCredentialID  string
	CredentialZoneID string
	RecordHost       string
	RecordName       string
	RecordType       string
	TTL              int
	Proxied          bool
}

type DNSInstanceMutationInput struct {
	ManagedRecordID        string
	Name                   string
	Priority               int
	Enabled                bool
	NodeGroupIDs           []string
	AnswerCount            int
	Condition              map[string]any
	Action                 map[string]any
	NotificationChannelIDs []string
}

type NotificationChannelMutationInput struct {
	Name        string
	ChannelType string
	Config      map[string]any
	Secret      string
	Enabled     bool
}

type RegistrationTokenInput struct {
	TTLHours int
}

type AgentAuthResult struct {
	OrganizationID          string
	AgentType               string
	AgentID                 string
	RegisteredWithToken     bool
	RegistrationTokenID     string
	AgentCredentialID       string
	AgentCredential         string
	AgentCredentialFileHint string
}

type TargetMutationInput struct {
	Name                   string
	Host                   string
	Port                   int
	Enabled                bool
	TargetGroupIDs         []string
	TargetGroupIDsProvided bool
}

type TargetGroupMutationInput struct {
	Name        string
	Description string
	Scheduler   string
	Members     []TargetGroupMemberInput
}

type TargetGroupMemberInput struct {
	TargetID string
	Priority int
	Enabled  bool
}

type RuleMutationInput struct {
	Name           string
	Tags           []string
	NodeGroupID    string
	ListenIP       string
	FailurePolicy  string
	ForwardingType string
	Protocol       string
	Port           int
	Match          RuleMatchInput
	ProxyProtocol  RuleProxyProtocolInput
	Upstream       RuleUpstreamInput
	Enabled        bool
	EnabledSet     bool
}

type RuleMatchInput struct {
	Type        string
	SNIHostname string
}

type RuleProxyProtocolInput struct {
	In  string `json:"in"`
	Out string `json:"out"`
}

type RuleUpstreamInput struct {
	Type          string `json:"type"`
	TargetID      string `json:"target_id"`
	TargetGroupID string `json:"target_group_id"`
}

type RuleCopyInput struct {
	Name    string
	Tags    []string
	TagsSet bool
}

type RuleImportInput struct {
	DryRun     bool
	Entry      RuleImportEntry
	Format     string
	SourceText string
}

type RuleImportEntry struct {
	NodeGroupID string `json:"node_group_id"`
	ListenIP    string `json:"listen_ip"`
}

type RuleBatchInput struct {
	Action  string
	RuleIDs []string
}

type ErrorPayload struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type RuleBatchResult struct {
	Action    string                `json:"action"`
	Total     int                   `json:"total"`
	Succeeded int                   `json:"succeeded"`
	Failed    int                   `json:"failed"`
	Results   []RuleBatchItemResult `json:"results"`
}

type RuleBatchItemResult struct {
	RuleID  string        `json:"rule_id"`
	Status  string        `json:"status"`
	Error   *ErrorPayload `json:"error,omitempty"`
	Warning string        `json:"warning,omitempty"`
}

type OrganizationUpdateInput struct {
	Name string
	Slug string
}

type ResourceOption struct {
	Value          string `json:"value"`
	Label          string `json:"label"`
	Disabled       bool   `json:"disabled"`
	DisabledReason string `json:"disabled_reason,omitempty"`
}

type NodeGroupPayload struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type NodePayload struct {
	ID                     string                         `json:"id"`
	Name                   string                         `json:"name"`
	Status                 string                         `json:"status"`
	PublicDescription      string                         `json:"public_description"`
	DesiredConfigVersion   int                            `json:"desired_config_version"`
	AppliedConfigVersion   int                            `json:"applied_config_version"`
	ConfigStatus           string                         `json:"config_status"`
	ConfigErrorMessage     string                         `json:"config_error_message,omitempty"`
	ConfigStatusUpdatedAt  string                         `json:"config_status_updated_at,omitempty"`
	LastSeenAt             string                         `json:"last_seen_at,omitempty"`
	RegisteredAt           string                         `json:"registered_at,omitempty"`
	AgentVersion           string                         `json:"agent_version"`
	AgentCommit            string                         `json:"agent_commit"`
	AgentBuildTime         string                         `json:"agent_build_time"`
	AgentAutoUpdateEnabled bool                           `json:"agent_auto_update_enabled"`
	DesiredAgentVersion    string                         `json:"desired_agent_version"`
	AgentUpdateStatus      string                         `json:"agent_update_status"`
	AgentUpdateError       string                         `json:"agent_update_error,omitempty"`
	AgentUpdateStartedAt   string                         `json:"agent_update_started_at,omitempty"`
	AgentUpdateFinishedAt  string                         `json:"agent_update_finished_at,omitempty"`
	GeoIP                  NodeGeoIPPayload               `json:"geoip"`
	GroupIDs               []string                       `json:"group_ids"`
	ListenIPs              []NodeListenIPPayload          `json:"listen_ips"`
	PortRanges             []NodePortRangePayload         `json:"port_ranges"`
	DNSPublishAddresses    []NodeDNSPublishAddressPayload `json:"dns_publish_addresses"`
}

type NodeGeoIPPayload struct {
	IP          string `json:"ip,omitempty"`
	Source      string `json:"source,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
	CountryName string `json:"country_name,omitempty"`
	FlagEmoji   string `json:"flag_emoji,omitempty"`
	Attribution string `json:"attribution,omitempty"`
}

type NodeListenIPPayload struct {
	ID          string `json:"id,omitempty"`
	ListenIP    string `json:"listen_ip"`
	DisplayName string `json:"display_name"`
	Enabled     bool   `json:"enabled"`
}

type NodePortRangePayload struct {
	ID        string `json:"id,omitempty"`
	Protocol  string `json:"protocol"`
	StartPort int    `json:"start_port"`
	EndPort   int    `json:"end_port"`
	Enabled   bool   `json:"enabled"`
}

type NodeDNSPublishAddressPayload struct {
	ID          string           `json:"id,omitempty"`
	AddressType string           `json:"address_type"`
	Address     string           `json:"address"`
	Source      string           `json:"source"`
	Enabled     bool             `json:"enabled"`
	ObservedAt  string           `json:"observed_at,omitempty"`
	GeoIP       NodeGeoIPPayload `json:"geoip"`
}

type MonitorGroupPayload struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type MonitorPayload struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Status               string   `json:"status"`
	DesiredConfigVersion int      `json:"desired_config_version"`
	AppliedConfigVersion int      `json:"applied_config_version"`
	LastSeenAt           string   `json:"last_seen_at,omitempty"`
	RegisteredAt         string   `json:"registered_at,omitempty"`
	GroupIDs             []string `json:"group_ids"`
}

type HealthCheckPayload struct {
	ID              string                      `json:"id"`
	Name            string                      `json:"name"`
	ProbeType       string                      `json:"probe_type"`
	IntervalSeconds int                         `json:"interval_seconds"`
	TimeoutSeconds  int                         `json:"timeout_seconds"`
	Config          map[string]any              `json:"config"`
	Enabled         bool                        `json:"enabled"`
	TargetScope     HealthTargetScopePayload    `json:"target_scope"`
	Targets         []HealthCheckTargetPayload  `json:"targets"`
	MonitorScopes   []HealthMonitorScopePayload `json:"monitor_scopes"`
	LatestResults   []HealthResultPayload       `json:"latest_results"`
}

type HealthTargetScopePayload struct {
	Type          string   `json:"type"`
	TargetIDs     []string `json:"target_ids,omitempty"`
	TargetGroupID string   `json:"target_group_id,omitempty"`
}

type HealthCheckTargetPayload struct {
	ID            string `json:"id"`
	ScopeType     string `json:"scope_type"`
	TargetID      string `json:"target_id"`
	TargetGroupID string `json:"target_group_id,omitempty"`
	TargetName    string `json:"target_name"`
	TargetHost    string `json:"target_host"`
	TargetPort    int    `json:"target_port"`
}

type HealthMonitorScopePayload struct {
	ID             string `json:"id"`
	ScopeType      string `json:"scope_type"`
	MonitorID      string `json:"monitor_id,omitempty"`
	MonitorGroupID string `json:"monitor_group_id,omitempty"`
}

type HealthResultPayload struct {
	ID                  string `json:"id"`
	HealthCheckID       string `json:"health_check_id"`
	HealthCheckTargetID string `json:"health_check_target_id"`
	MonitorID           string `json:"monitor_id"`
	TargetID            string `json:"target_id"`
	Status              string `json:"status"`
	LatencyMS           int    `json:"latency_ms"`
	ErrorMessage        string `json:"error_message,omitempty"`
	ObservedAt          string `json:"observed_at"`
	CreatedAt           string `json:"created_at"`
}

type HealthEvaluationRulePayload struct {
	ID             string               `json:"id"`
	HealthCheckID  string               `json:"health_check_id"`
	Name           string               `json:"name"`
	Enabled        bool                 `json:"enabled"`
	ExpressionJSON string               `json:"expression_json"`
	Events         []HealthEventPayload `json:"events"`
	CreatedAt      string               `json:"created_at"`
	UpdatedAt      string               `json:"updated_at"`
}

type HealthEventPayload struct {
	ID         string `json:"id"`
	EventType  string `json:"event_type"`
	ConfigJSON string `json:"config_json"`
	Enabled    bool   `json:"enabled"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type DNSCredentialPayload struct {
	ID       string                     `json:"id"`
	Name     string                     `json:"name"`
	Provider string                     `json:"provider"`
	Zones    []DNSCredentialZonePayload `json:"zones"`
}

type DNSCredentialZonePayload struct {
	ID           string `json:"id"`
	ZoneID       string `json:"zone_id"`
	ZoneName     string `json:"zone_name"`
	Status       string `json:"status"`
	LastSyncedAt string `json:"last_synced_at"`
}

type DNSManagedRecordPayload struct {
	ID                   string                 `json:"id"`
	DNSCredentialID      string                 `json:"dns_credential_id"`
	CredentialZoneID     string                 `json:"credential_zone_id"`
	ZoneID               string                 `json:"zone_id"`
	ZoneName             string                 `json:"zone_name"`
	RecordHost           string                 `json:"record_host"`
	RecordName           string                 `json:"record_name"`
	RecordType           string                 `json:"record_type"`
	TTL                  int                    `json:"ttl"`
	Proxied              bool                   `json:"proxied"`
	ActiveInstanceID     string                 `json:"active_instance_id,omitempty"`
	LastAppliedValues    []string               `json:"last_applied_values"`
	LastEvaluationStatus string                 `json:"last_evaluation_status"`
	LastEvaluationError  string                 `json:"last_evaluation_error,omitempty"`
	LastDiagnostics      []DNSDiagnosticPayload `json:"last_diagnostics"`
	LastEvaluatedAt      string                 `json:"last_evaluated_at,omitempty"`
	LastAppliedAt        string                 `json:"last_applied_at,omitempty"`
	Instances            []DNSInstancePayload   `json:"instances"`
}

type DNSInstancePayload struct {
	ID                     string                 `json:"id"`
	ManagedRecordID        string                 `json:"managed_record_id"`
	Name                   string                 `json:"name"`
	Priority               int                    `json:"priority"`
	Enabled                bool                   `json:"enabled"`
	NodeGroupIDs           []string               `json:"node_group_ids"`
	AnswerCount            int                    `json:"answer_count"`
	Condition              map[string]any         `json:"condition"`
	Action                 map[string]any         `json:"action"`
	NotificationChannelIDs []string               `json:"notification_channel_ids"`
	LastOutputValues       []string               `json:"last_output_values"`
	LastStatus             string                 `json:"last_status"`
	LastDiagnostics        []DNSDiagnosticPayload `json:"last_diagnostics"`
	LastEvaluatedAt        string                 `json:"last_evaluated_at,omitempty"`
}

type DNSDiagnosticPayload struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type NotificationChannelPayload struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	ChannelType string         `json:"channel_type"`
	Config      map[string]any `json:"config"`
	Enabled     bool           `json:"enabled"`
}

type RegistrationTokenPayload struct {
	TokenID         string `json:"token_id"`
	Token           string `json:"token,omitempty"`
	AgentType       string `json:"agent_type"`
	AgentID         string `json:"agent_id"`
	ExpiresAt       string `json:"expires_at"`
	UsedAt          string `json:"used_at,omitempty"`
	RevokedAt       string `json:"revoked_at,omitempty"`
	CreatedAt       string `json:"created_at"`
	CreatedByUserID string `json:"created_by_user_id,omitempty"`
	InstallCommand  string `json:"install_command,omitempty"`
}

type AgentUpdatePolicyInput struct {
	Enabled bool `json:"enabled"`
}

type AgentUpgradeBatchInput struct {
	NodeIDs []string `json:"node_ids"`
}

type TargetPayload struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Enabled bool   `json:"enabled"`
}

type TargetGroupPayload struct {
	ID          string                     `json:"id"`
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	Scheduler   string                     `json:"scheduler"`
	Members     []TargetGroupMemberPayload `json:"members"`
}

type TargetGroupMemberPayload struct {
	TargetID string `json:"target_id"`
	Priority int    `json:"priority"`
	Enabled  bool   `json:"enabled"`
}

type RulePayload struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Status         string                 `json:"status"`
	Enabled        bool                   `json:"enabled"`
	Tags           []string               `json:"tags"`
	NodeGroupID    string                 `json:"node_group_id"`
	ListenIP       string                 `json:"listen_ip"`
	FailurePolicy  string                 `json:"failure_policy"`
	ForwardingType string                 `json:"forwarding_type"`
	Protocol       string                 `json:"protocol"`
	Port           int                    `json:"port"`
	Match          RuleMatchPayload       `json:"match"`
	ProxyProtocol  RuleProxyProtocolInput `json:"proxy_protocol"`
	Upstream       RuleUpstreamInput      `json:"upstream"`
	OwnerUserID    string                 `json:"owner_user_id"`
	ConfigVersion  int                    `json:"config_version"`
	ConnectInfo    RuleConnectInfoPayload `json:"connect_info"`
	Deployment     RuleDeploymentPayload  `json:"deployment"`
}

type RuleDeploymentPayload struct {
	Status  string                      `json:"status"`
	Total   int                         `json:"total"`
	Applied int                         `json:"applied"`
	Failed  int                         `json:"failed"`
	Pending int                         `json:"pending"`
	Nodes   []RuleDeploymentNodePayload `json:"nodes"`
}

type RuleDeploymentNodePayload struct {
	NodeID       string `json:"node_id"`
	NodeName     string `json:"node_name"`
	Status       string `json:"status"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	Protocol     string `json:"protocol,omitempty"`
	ListenIP     string `json:"listen_ip,omitempty"`
	Port         int    `json:"port,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

type RuleMatchPayload struct {
	Type        string `json:"type"`
	SNIHostname string `json:"sni_hostname,omitempty"`
}

type RuleConnectInfoPayload struct {
	Protocol         string   `json:"protocol"`
	ListenPort       int      `json:"listen_port"`
	ListenIP         string   `json:"listen_ip"`
	NodeDescriptions []string `json:"node_descriptions"`
}

type RuleTrafficPayload struct {
	UploadBytes    int64  `json:"upload_bytes"`
	DownloadBytes  int64  `json:"download_bytes"`
	TCPConnections int64  `json:"tcp_connections"`
	UDPPackets     int64  `json:"udp_packets"`
	LimitBytes     int64  `json:"limit_bytes"`
	LimitMode      string `json:"limit_mode"`
}

type AgentRuntimeMetricsInput struct {
	AgentID    string
	Status     string
	LastSeenAt string
	Metrics    agent.MetricsPayload
}

type AgentTrafficReportInput struct {
	ReportID string
	Deltas   []agent.RuleTrafficDelta
}

type ConfigApplyErrorInput struct {
	Code     string
	RuleIDs  []string
	Protocol domain.Protocol
	ListenIP string
	Port     int
	Message  string
}

type RuleDiagnosticsPayload struct {
	RuleID        string                         `json:"rule_id"`
	GeneratedAt   string                         `json:"generated_at"`
	BandwidthBps  int64                          `json:"bandwidth_bps"`
	UploadBytes   int64                          `json:"upload_bytes"`
	DownloadBytes int64                          `json:"download_bytes"`
	Targets       []RuleTargetDiagnosticsPayload `json:"targets"`
}

type RuleTargetDiagnosticsPayload struct {
	TargetID            string `json:"target_id"`
	Name                string `json:"name"`
	Address             string `json:"address"`
	Status              string `json:"status"`
	LastSeenAt          string `json:"last_seen_at,omitempty"`
	LatencyMS           *int64 `json:"latency_ms"`
	BandwidthBps        *int64 `json:"bandwidth_bps"`
	UploadBytes         int64  `json:"upload_bytes"`
	DownloadBytes       int64  `json:"download_bytes"`
	TCPConnections      int64  `json:"tcp_connections"`
	UDPPacketsPerSecond int64  `json:"udp_packets_per_second"`
}

type RulesExportPayload struct {
	SchemaVersion string                       `json:"schema_version"`
	ExportedAt    string                       `json:"exported_at"`
	Rules         []PortableRulePayload        `json:"rules"`
	Targets       []PortableTargetPayload      `json:"targets"`
	TargetGroups  []PortableTargetGroupPayload `json:"target_groups"`
}

type PortableRulePayload struct {
	Name           string                      `json:"name"`
	Tags           []string                    `json:"tags"`
	FailurePolicy  string                      `json:"failure_policy,omitempty"`
	ForwardingType string                      `json:"forwarding_type"`
	Protocol       string                      `json:"protocol"`
	Port           int                         `json:"port"`
	Match          RuleMatchPayload            `json:"match"`
	ProxyProtocol  RuleProxyProtocolInput      `json:"proxy_protocol"`
	Upstream       PortableRuleUpstreamPayload `json:"upstream"`
}

type PortableRuleUpstreamPayload struct {
	Type           string `json:"type"`
	TargetRef      string `json:"target_ref,omitempty"`
	TargetGroupRef string `json:"target_group_ref,omitempty"`
}

type PortableTargetPayload struct {
	Ref     string `json:"ref"`
	Name    string `json:"name"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Enabled bool   `json:"enabled"`
}

type PortableTargetGroupPayload struct {
	Ref         string                             `json:"ref"`
	Name        string                             `json:"name"`
	Description string                             `json:"description"`
	Scheduler   string                             `json:"scheduler"`
	Members     []PortableTargetGroupMemberPayload `json:"members"`
}

type PortableTargetGroupMemberPayload struct {
	TargetRef string `json:"target_ref"`
	Priority  int    `json:"priority"`
	Enabled   bool   `json:"enabled"`
}

type RulesImportResult struct {
	DryRun   bool              `json:"dry_run"`
	Created  int               `json:"created"`
	Skipped  int               `json:"skipped"`
	Errors   []RuleImportIssue `json:"errors"`
	Warnings []RuleImportIssue `json:"warnings"`
}

type RuleImportIssue struct {
	Code    string         `json:"code"`
	Scope   string         `json:"scope"`
	Index   *int           `json:"index,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}
