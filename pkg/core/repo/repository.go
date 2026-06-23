package repo

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("repository record not found")
var ErrConflict = errors.New("repository conflict")

type UnitOfWork interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context, repositories Repositories) error) error
}

type Repositories interface {
	Users() UserRepository
	Organizations() OrganizationRepository
	Members() MemberRepository
	Roles() RoleRepository
	NodeGroups() NodeGroupRepository
	Nodes() NodeRepository
	MonitorGroups() MonitorGroupRepository
	Monitors() MonitorRepository
	HealthChecks() HealthCheckRepository
	DNSCredentials() DNSCredentialRepository
	DNSRecords() DNSRecordRepository
	Targets() TargetRepository
	TargetGroups() TargetGroupRepository
	Rules() RuleRepository
	Quotas() QuotaRepository
	AgentRegistrationTokens() AgentRegistrationTokenRepository
	AgentCredentials() AgentCredentialRepository
	AuditLogs() AuditLogRepository
}

type UserRepository interface {
	FindUserByID(ctx context.Context, userID string) (UserRecord, error)
	FindUserByEmail(ctx context.Context, email string) (UserRecord, error)
}

type OrganizationRepository interface {
	CountOrganizations(ctx context.Context) (int, error)
	FindOrganizationByID(ctx context.Context, organizationID string) (OrganizationRecord, error)
	CreateOrganization(ctx context.Context, organization OrganizationRecord) error
	UpdateOrganization(ctx context.Context, organization OrganizationRecord) error
	ListOrganizations(ctx context.Context) ([]OrganizationRecord, error)
	ListOrganizationsByUserID(ctx context.Context, userID string) ([]OrganizationRecord, error)
}

type MemberRepository interface {
	FindMemberByUserAndOrganization(ctx context.Context, organizationID string, userID string) (MemberRecord, error)
	ListMembersByOrganization(ctx context.Context, organizationID string) ([]MemberRecord, error)
	CreateMember(ctx context.Context, member MemberRecord) error
	UpdateMemberStatus(ctx context.Context, organizationID string, memberID string, status string) error
	DeleteMember(ctx context.Context, organizationID string, memberID string) error
}

type RoleRepository interface {
	ListRolesByOrganization(ctx context.Context, organizationID string) ([]RoleRecord, error)
	FindRoleByID(ctx context.Context, organizationID string, roleID string) (RoleRecord, error)
	CreateRole(ctx context.Context, role RoleRecord) error
	UpdateRole(ctx context.Context, role RoleRecord) error
	DeleteRole(ctx context.Context, organizationID string, roleID string) error
	ReplacePermissions(ctx context.Context, organizationID string, roleID string, permissions []string, now string, nextID func() string) error
	ReplaceResourceScopes(ctx context.Context, organizationID string, roleID string, scopes []ResourceScopeRecord, now string, nextID func() string) error
	ReplaceMemberRoles(ctx context.Context, organizationID string, memberID string, roleIDs []string, now string, nextID func() string) error
	ListForMember(ctx context.Context, organizationID string, memberID string) ([]RoleRecord, error)
}

type NodeGroupRepository interface {
	ListNodeGroupsByOrganization(ctx context.Context, organizationID string) ([]NodeGroupRecord, error)
	FindNodeGroupByID(ctx context.Context, organizationID string, nodeGroupID string) (NodeGroupRecord, error)
	CreateNodeGroup(ctx context.Context, nodeGroup NodeGroupRecord) error
	UpdateNodeGroup(ctx context.Context, nodeGroup NodeGroupRecord) error
	DeleteNodeGroup(ctx context.Context, organizationID string, nodeGroupID string, deletedAt string) error
}

type NodeRepository interface {
	ListNodesByOrganization(ctx context.Context, organizationID string) ([]NodeRecord, error)
	FindNodeByID(ctx context.Context, organizationID string, nodeID string) (NodeRecord, error)
	CreateNode(ctx context.Context, node NodeRecord, groupIDs []string, listenIPs []NodeListenIPRecord, portRanges []NodePortRangeRecord, now string, nextID func() string) error
	UpdateNode(ctx context.Context, node NodeRecord, replaceGroups bool, groupIDs []string, replaceListenIPs bool, listenIPs []NodeListenIPRecord, replacePortRanges bool, portRanges []NodePortRangeRecord, now string, nextID func() string) error
	MarkNodeAgentConnected(ctx context.Context, organizationID string, nodeID string, now string) error
	UpdateNodeAgentVersion(ctx context.Context, organizationID string, nodeID string, version NodeAgentVersionRecord, now string) error
	UpdateNodeAgentUpdatePolicy(ctx context.Context, organizationID string, nodeID string, enabled bool, now string) error
	MarkNodeAgentUpdateRequested(ctx context.Context, organizationID string, nodeID string, targetVersion string, now string) error
	MarkNodeAgentUpdateSatisfied(ctx context.Context, organizationID string, nodeID string, targetVersion string, now string) error
	RecordNodeAgentUpdateResult(ctx context.Context, organizationID string, nodeID string, status string, errorMessage string, now string) error
	MarkNodeAgentDisconnected(ctx context.Context, organizationID string, nodeID string, now string) error
	RecordNodeConfigAck(ctx context.Context, organizationID string, nodeID string, ack NodeConfigAckRecord, now string) error
	EnsureDesiredConfigVersionAtLeast(ctx context.Context, organizationID string, nodeID string, configVersion int, now string) error
	IncrementDesiredConfigForNode(ctx context.Context, organizationID string, nodeID string, now string) error
	IncrementDesiredConfigForNodeGroup(ctx context.Context, organizationID string, nodeGroupID string, now string) error
	DeleteNode(ctx context.Context, organizationID string, nodeID string, deletedAt string) error
}

type MonitorGroupRepository interface {
	ListMonitorGroupsByOrganization(ctx context.Context, organizationID string) ([]MonitorGroupRecord, error)
	FindMonitorGroupByID(ctx context.Context, organizationID string, monitorGroupID string) (MonitorGroupRecord, error)
	CreateMonitorGroup(ctx context.Context, monitorGroup MonitorGroupRecord) error
	UpdateMonitorGroup(ctx context.Context, monitorGroup MonitorGroupRecord) error
	DeleteMonitorGroup(ctx context.Context, organizationID string, monitorGroupID string, deletedAt string) error
}

type MonitorRepository interface {
	ListMonitorsByOrganization(ctx context.Context, organizationID string) ([]MonitorRecord, error)
	FindMonitorByID(ctx context.Context, organizationID string, monitorID string) (MonitorRecord, error)
	CreateMonitor(ctx context.Context, monitor MonitorRecord, groupIDs []string, now string, nextID func() string) error
	UpdateMonitor(ctx context.Context, monitor MonitorRecord, replaceGroups bool, groupIDs []string, now string, nextID func() string) error
	MarkMonitorAgentConnected(ctx context.Context, organizationID string, monitorID string, now string) error
	MarkMonitorAgentDisconnected(ctx context.Context, organizationID string, monitorID string, now string) error
	RecordMonitorConfigAck(ctx context.Context, organizationID string, monitorID string, configVersion int, now string) error
	DeleteMonitor(ctx context.Context, organizationID string, monitorID string, deletedAt string) error
}

type HealthCheckRepository interface {
	ListHealthChecksByOrganization(ctx context.Context, organizationID string) ([]HealthCheckRecord, error)
	FindHealthCheckByID(ctx context.Context, organizationID string, healthCheckID string) (HealthCheckRecord, error)
	CreateHealthCheck(ctx context.Context, healthCheck HealthCheckRecord, targets []HealthCheckTargetRecord, monitorScopes []HealthCheckMonitorScopeRecord, now string, nextID func() string) error
	UpdateHealthCheck(ctx context.Context, healthCheck HealthCheckRecord, targets []HealthCheckTargetRecord, monitorScopes []HealthCheckMonitorScopeRecord, now string, nextID func() string) error
	SyncHealthCheckTargets(ctx context.Context, organizationID string, healthCheckID string, targets []HealthCheckTargetRecord, now string, nextID func() string) error
	DeleteHealthCheck(ctx context.Context, organizationID string, healthCheckID string, deletedAt string) error
	ListHealthResults(ctx context.Context, organizationID string, healthCheckID string, limit int) ([]HealthResultRecord, error)
	ListLatestHealthResultsByCheck(ctx context.Context, organizationID string, healthCheckID string) ([]HealthResultRecord, error)
	ListLatestHealthResultsByChecks(ctx context.Context, organizationID string, healthCheckIDs []string) (map[string][]HealthResultRecord, error)
	RecordHealthResults(ctx context.Context, organizationID string, results []HealthResultRecord) error
	ListHealthEvaluationRulesByCheck(ctx context.Context, organizationID string, healthCheckID string) ([]HealthEvaluationRuleRecord, error)
	CreateHealthEvaluationRule(ctx context.Context, rule HealthEvaluationRuleRecord, events []HealthEventRecord) error
}

type DNSCredentialRepository interface {
	ListDNSCredentialsByOrganization(ctx context.Context, organizationID string) ([]DNSCredentialRecord, error)
	FindDNSCredentialByID(ctx context.Context, organizationID string, credentialID string) (DNSCredentialRecord, error)
	ListDNSCredentialZonesByOrganization(ctx context.Context, organizationID string) ([]DNSCredentialZoneRecord, error)
	ListDNSCredentialZonesByCredential(ctx context.Context, organizationID string, credentialID string) ([]DNSCredentialZoneRecord, error)
	FindDNSCredentialZoneByID(ctx context.Context, organizationID string, credentialZoneID string) (DNSCredentialZoneRecord, error)
	CreateDNSCredential(ctx context.Context, credential DNSCredentialRecord) error
	UpdateDNSCredential(ctx context.Context, credential DNSCredentialRecord, replaceSecret bool) error
	ReplaceDNSCredentialZones(ctx context.Context, organizationID string, credentialID string, zones []DNSCredentialZoneRecord, now string, nextID func() string) error
	DeleteDNSCredential(ctx context.Context, organizationID string, credentialID string, deletedAt string) error
}

type DNSRecordRepository interface {
	ListDNSManagedRecordsByOrganization(ctx context.Context, organizationID string) ([]DNSManagedRecordRecord, error)
	FindDNSManagedRecordByID(ctx context.Context, organizationID string, recordID string) (DNSManagedRecordRecord, error)
	LockDNSManagedRecordEvaluation(ctx context.Context, organizationID string, recordID string) error
	CreateDNSManagedRecord(ctx context.Context, record DNSManagedRecordRecord) error
	UpdateDNSManagedRecord(ctx context.Context, record DNSManagedRecordRecord) error
	DeleteDNSManagedRecord(ctx context.Context, organizationID string, recordID string, deletedAt string) error
	ListDNSInstancesByOrganization(ctx context.Context, organizationID string) ([]DNSInstanceRecord, error)
	ListDNSInstancesByManagedRecord(ctx context.Context, organizationID string, recordID string) ([]DNSInstanceRecord, error)
	FindDNSInstanceByID(ctx context.Context, organizationID string, instanceID string) (DNSInstanceRecord, error)
	LockDNSInstanceMutation(ctx context.Context, organizationID string, instanceID string) error
	CreateDNSInstance(ctx context.Context, instance DNSInstanceRecord) error
	UpdateDNSInstance(ctx context.Context, instance DNSInstanceRecord) error
	DeleteDNSInstance(ctx context.Context, organizationID string, instanceID string, deletedAt string) error
	ClearDNSManagedRecordActiveInstance(ctx context.Context, organizationID string, instanceID string, updatedAt string) error
	UpdateDNSManagedRecordEvaluation(ctx context.Context, record DNSManagedRecordRecord) error
	UpdateDNSInstanceEvaluation(ctx context.Context, instance DNSInstanceRecord) error
	ListNotificationChannelsByOrganization(ctx context.Context, organizationID string) ([]NotificationChannelRecord, error)
	FindNotificationChannelByID(ctx context.Context, organizationID string, channelID string) (NotificationChannelRecord, error)
	CreateNotificationChannel(ctx context.Context, channel NotificationChannelRecord) error
	UpdateNotificationChannel(ctx context.Context, channel NotificationChannelRecord, replaceSecret bool) error
	DeleteNotificationChannel(ctx context.Context, organizationID string, channelID string, deletedAt string) error
	CreateNotificationDelivery(ctx context.Context, delivery NotificationDeliveryRecord) error
}

type TargetRepository interface {
	ListTargetsByOrganization(ctx context.Context, organizationID string) ([]TargetRecord, error)
	FindTargetByID(ctx context.Context, organizationID string, targetID string) (TargetRecord, error)
	CreateTarget(ctx context.Context, target TargetRecord) error
	UpdateTarget(ctx context.Context, target TargetRecord) error
	DeleteTarget(ctx context.Context, organizationID string, targetID string, deletedAt string) error
}

type TargetGroupRepository interface {
	ListTargetGroupsByOrganization(ctx context.Context, organizationID string) ([]TargetGroupRecord, error)
	FindTargetGroupByID(ctx context.Context, organizationID string, targetGroupID string) (TargetGroupRecord, error)
	CreateTargetGroup(ctx context.Context, targetGroup TargetGroupRecord, members []TargetGroupMemberRecord, now string, nextID func() string) error
	UpdateTargetGroup(ctx context.Context, targetGroup TargetGroupRecord, members []TargetGroupMemberRecord, now string, nextID func() string) error
	DeleteTargetGroup(ctx context.Context, organizationID string, targetGroupID string, deletedAt string) error
}

type RuleRepository interface {
	ListRulesByOrganization(ctx context.Context, organizationID string) ([]RuleRecord, error)
	FindRuleByID(ctx context.Context, organizationID string, ruleID string) (RuleRecord, error)
	CreateRule(ctx context.Context, rule RuleRecord, binding InboundBindingRecord, tags []string, now string, nextID func() string) error
	UpdateRule(ctx context.Context, rule RuleRecord, binding InboundBindingRecord, tags []string, now string, nextID func() string) error
	DeleteRule(ctx context.Context, organizationID string, ruleID string, deletedAt string) error
	ListEnabledInboundBindings(ctx context.Context, organizationID string) ([]RuleRecord, error)
	CountRulesByOrganization(ctx context.Context, organizationID string) (int, error)
	CountRulesByOwner(ctx context.Context, organizationID string, ownerUserID string) (int, error)
	SumRuleTraffic(ctx context.Context, organizationID string, ruleID string) (RuleTrafficRecord, error)
	RecordNodeRuleTrafficAssignments(ctx context.Context, organizationID string, nodeID string, ruleIDs []string, now string) error
	RecordRuleTrafficReport(ctx context.Context, organizationID string, agentID string, report RuleTrafficReportRecord, deltas []RuleTrafficDeltaRecord, now string, nextID func() string) (bool, error)
	ListRuleDeploymentsByOrganization(ctx context.Context, organizationID string) ([]RuleDeploymentRecord, error)
	ReplaceRuleDeploymentPending(ctx context.Context, organizationID string, rule RuleRecord, deployments []RuleDeploymentPendingRecord, now string, nextID func() string) error
	UpsertRuleDeploymentPending(ctx context.Context, organizationID string, rule RuleRecord, deployment RuleDeploymentPendingRecord, now string, nextID func() string) error
	RecordRuleDeploymentApplied(ctx context.Context, organizationID string, nodeID string, configVersion int, deployments []RuleDeploymentAppliedRecord, now string, nextID func() string) error
	RecordRuleDeploymentFailures(ctx context.Context, organizationID string, nodeID string, configVersion int, failures []RuleDeploymentFailureRecord, now string, nextID func() string) error
	DeleteRuleDeploymentForNode(ctx context.Context, organizationID string, ruleID string, nodeID string) error
	DeleteRuleDeployments(ctx context.Context, organizationID string, ruleID string) error
}

type QuotaRepository interface {
	ListQuotasByOrganization(ctx context.Context, organizationID string) ([]QuotaRecord, error)
}

type AgentRegistrationTokenRepository interface {
	ListRegistrationTokens(ctx context.Context, organizationID string, agentType string, agentID string) ([]AgentRegistrationTokenRecord, error)
	FindRegistrationTokenByHash(ctx context.Context, tokenHash string) (AgentRegistrationTokenRecord, error)
	CreateRegistrationToken(ctx context.Context, token AgentRegistrationTokenRecord) error
	ClaimRegistrationToken(ctx context.Context, organizationID string, tokenID string, claimedAt string) error
	ReleaseRegistrationTokenUse(ctx context.Context, organizationID string, tokenID string) error
	RevokeActiveUnusedRegistrationTokens(ctx context.Context, organizationID string, agentType string, agentID string, revokedAt string) error
	RevokeRegistrationToken(ctx context.Context, organizationID string, agentType string, agentID string, tokenID string, revokedAt string) error
}

type AgentCredentialRepository interface {
	FindCredentialByHash(ctx context.Context, credentialHash string) (AgentCredentialRecord, error)
	FindPendingCredentialByRegistrationToken(ctx context.Context, organizationID string, registrationTokenID string) (AgentCredentialRecord, error)
	CreateCredential(ctx context.Context, credential AgentCredentialRecord) error
	ActivateCredential(ctx context.Context, organizationID string, credentialID string, activatedAt string) error
	RevokeActiveCredentialsExcept(ctx context.Context, organizationID string, agentType string, agentID string, keepCredentialID string, revokedAt string) error
	RevokeCredential(ctx context.Context, organizationID string, credentialID string, revokedAt string) error
}

type AuditLogRepository interface {
	CreateAuditLog(ctx context.Context, audit AuditLogRecord) error
}

type UserRecord struct {
	ID    string
	Email string
	Name  string
}

type OrganizationRecord struct {
	ID                       string
	Name                     string
	Slug                     string
	OwnerUserID              string
	DefaultRuleLimit         int
	DefaultTrafficLimitBytes int64
	DefaultTrafficLimitMode  string
	CreatedAt                string
	UpdatedAt                string
	DeletedAt                string
}

type MemberRecord struct {
	ID             string
	OrganizationID string
	UserID         string
	UserEmail      string
	UserName       string
	Status         string
	CreatedAt      string
	UpdatedAt      string
}

type RoleRecord struct {
	ID             string
	OrganizationID string
	Key            string
	Name           string
	Description    string
	IsSystem       bool
	Permissions    []string
	ResourceScopes []ResourceScopeRecord
	CreatedAt      string
	UpdatedAt      string
	DeletedAt      string
}

type ResourceScopeRecord struct {
	ID             string
	OrganizationID string
	RoleID         string
	ResourceType   string
	ResourceID     string
	AccessLevel    string
	CreatedAt      string
}

type NodeGroupRecord struct {
	ID             string
	OrganizationID string
	Name           string
	Description    string
	CreatedAt      string
	UpdatedAt      string
	DeletedAt      string
}

type NodeRecord struct {
	ID                        string
	OrganizationID            string
	Name                      string
	Status                    string
	PublicDescription         string
	DesiredConfigVersion      int
	AppliedConfigVersion      int
	ConfigStatus              string
	ConfigErrorMessage        string
	ConfigStatusConfigVersion int
	ConfigRetryCount          int
	ConfigNextRetryAt         string
	ConfigStatusUpdatedAt     string
	LastSeenAt                string
	RegisteredAt              string
	AgentVersion              string
	AgentCommit               string
	AgentBuildTime            string
	AgentAutoUpdateEnabled    bool
	DesiredAgentVersion       string
	AgentUpdateStatus         string
	AgentUpdateError          string
	AgentUpdateStartedAt      string
	AgentUpdateFinishedAt     string
	CreatedAt                 string
	UpdatedAt                 string
	DeletedAt                 string
	GroupIDs                  []string
	ListenIPs                 []NodeListenIPRecord
	PortRanges                []NodePortRangeRecord
	DNSPublishAddresses       []NodeDNSPublishAddressRecord
}

type NodeConfigAckRecord struct {
	ConfigVersion int
	Status        string
	ErrorMessage  string
	RetryCount    int
	NextRetryAt   string
}

type NodeAgentVersionRecord struct {
	Version   string
	Commit    string
	BuildTime string
}

type NodeListenIPRecord struct {
	ID             string
	OrganizationID string
	NodeID         string
	ListenIP       string
	DisplayName    string
	Enabled        bool
	CreatedAt      string
	UpdatedAt      string
}

type NodePortRangeRecord struct {
	ID             string
	OrganizationID string
	NodeID         string
	Protocol       string
	StartPort      int
	EndPort        int
	Enabled        bool
	CreatedAt      string
	UpdatedAt      string
}

type NodeDNSPublishAddressRecord struct {
	ID             string
	OrganizationID string
	NodeID         string
	AddressType    string
	Address        string
	Source         string
	Enabled        bool
	ObservedAt     string
	CreatedAt      string
	UpdatedAt      string
}

type MonitorGroupRecord struct {
	ID             string
	OrganizationID string
	Name           string
	Description    string
	CreatedAt      string
	UpdatedAt      string
	DeletedAt      string
}

type MonitorRecord struct {
	ID                   string
	OrganizationID       string
	Name                 string
	Status               string
	DesiredConfigVersion int
	AppliedConfigVersion int
	LastSeenAt           string
	RegisteredAt         string
	CreatedAt            string
	UpdatedAt            string
	DeletedAt            string
	GroupIDs             []string
}

type HealthCheckRecord struct {
	ID              string
	OrganizationID  string
	Name            string
	ProbeType       string
	IntervalSeconds int
	TimeoutSeconds  int
	ConfigJSON      string
	Enabled         bool
	CreatedAt       string
	UpdatedAt       string
	DeletedAt       string
	Targets         []HealthCheckTargetRecord
	MonitorScopes   []HealthCheckMonitorScopeRecord
}

type HealthCheckTargetRecord struct {
	ID             string
	OrganizationID string
	HealthCheckID  string
	ScopeType      string
	TargetID       string
	TargetGroupID  string
	TargetName     string
	TargetHost     string
	TargetPort     int
	CreatedAt      string
}

type HealthCheckMonitorScopeRecord struct {
	ID             string
	OrganizationID string
	HealthCheckID  string
	ScopeType      string
	MonitorID      string
	MonitorGroupID string
	CreatedAt      string
}

type HealthResultRecord struct {
	ID                  string
	OrganizationID      string
	HealthCheckID       string
	HealthCheckTargetID string
	MonitorID           string
	TargetID            string
	Status              string
	LatencyMS           int
	ErrorMessage        string
	ObservedAt          string
	CreatedAt           string
}

type HealthEvaluationRuleRecord struct {
	ID             string
	OrganizationID string
	HealthCheckID  string
	Name           string
	Enabled        bool
	ExpressionJSON string
	Events         []HealthEventRecord
	CreatedAt      string
	UpdatedAt      string
	DeletedAt      string
}

type HealthEventRecord struct {
	ID                     string
	OrganizationID         string
	HealthEvaluationRuleID string
	EventType              string
	ConfigJSON             string
	EncryptedSecret        string
	Enabled                bool
	CreatedAt              string
	UpdatedAt              string
	DeletedAt              string
}

type DNSCredentialRecord struct {
	ID              string
	OrganizationID  string
	Provider        string
	Name            string
	EncryptedSecret string
	CreatedAt       string
	UpdatedAt       string
	DeletedAt       string
}

type DNSCredentialZoneRecord struct {
	ID              string
	OrganizationID  string
	DNSCredentialID string
	ZoneID          string
	ZoneName        string
	Status          string
	LastSyncedAt    string
	CreatedAt       string
	UpdatedAt       string
}

type DNSManagedRecordRecord struct {
	ID                      string
	OrganizationID          string
	DNSCredentialID         string
	CredentialZoneID        string
	ZoneID                  string
	ZoneName                string
	RecordHost              string
	RecordName              string
	RecordType              string
	TTL                     int
	Proxied                 bool
	ActiveInstanceID        string
	LastAppliedValuesJSON   string
	ProviderRetirementsJSON string
	LastEvaluationStatus    string
	LastEvaluationError     string
	LastDiagnosticsJSON     string
	LastEvaluatedAt         string
	LastAppliedAt           string
	CreatedAt               string
	UpdatedAt               string
	DeletedAt               string
	Instances               []DNSInstanceRecord
}

type DNSInstanceRecord struct {
	ID                         string
	OrganizationID             string
	ManagedRecordID            string
	Name                       string
	Priority                   int
	Enabled                    bool
	NodeGroupIDsJSON           string
	AnswerCount                int
	ConditionJSON              string
	ActionJSON                 string
	NotificationChannelIDsJSON string
	LastOutputValuesJSON       string
	LastStatus                 string
	LastDiagnosticsJSON        string
	LastEvaluatedAt            string
	CreatedAt                  string
	UpdatedAt                  string
	DeletedAt                  string
}

type NotificationChannelRecord struct {
	ID              string
	OrganizationID  string
	Name            string
	ChannelType     string
	ConfigJSON      string
	EncryptedSecret string
	Enabled         bool
	CreatedAt       string
	UpdatedAt       string
	DeletedAt       string
}

type NotificationDeliveryRecord struct {
	ID                 string
	OrganizationID     string
	ChannelID          string
	DNSManagedRecordID string
	DNSInstanceID      string
	EventType          string
	Status             string
	ErrorMessage       string
	PayloadJSON        string
	CreatedAt          string
	DeliveredAt        string
}

type TargetRecord struct {
	ID             string
	OrganizationID string
	Name           string
	Host           string
	Port           int
	Enabled        bool
	CreatedAt      string
	UpdatedAt      string
	DeletedAt      string
}

type TargetGroupRecord struct {
	ID             string
	OrganizationID string
	Name           string
	Description    string
	Scheduler      string
	CreatedAt      string
	UpdatedAt      string
	DeletedAt      string
	Members        []TargetGroupMemberRecord
}

type TargetGroupMemberRecord struct {
	ID             string
	OrganizationID string
	TargetGroupID  string
	TargetID       string
	Priority       int
	Enabled        bool
	CreatedAt      string
	UpdatedAt      string
}

type InboundBindingRecord struct {
	ID             string
	OrganizationID string
	NodeGroupID    string
	ListenIP       string
	Protocol       string
	Port           int
	MatchType      string
	CreatedAt      string
}

type RuleRecord struct {
	ID               string
	OrganizationID   string
	OwnerUserID      string
	Name             string
	Enabled          bool
	Status           string
	ForwardingType   string
	Protocol         string
	MatchType        string
	InboundBindingID string
	SNIHostname      string
	TargetType       string
	TargetID         string
	TargetGroupID    string
	ProxyProtocolIn  string
	ProxyProtocolOut string
	FailurePolicy    string
	ConfigVersion    int
	CreatedAt        string
	UpdatedAt        string
	DeletedAt        string
	Binding          InboundBindingRecord
	Tags             []string
}

type RuleTrafficRecord struct {
	UploadBytes    int64
	DownloadBytes  int64
	TCPConnections int64
	UDPPackets     int64
}

type RuleTrafficReportRecord struct {
	ID             string
	OrganizationID string
	AgentID        string
	ReportID       string
	CreatedAt      string
}

type RuleTrafficDeltaRecord struct {
	RuleID         string
	UploadBytes    int64
	DownloadBytes  int64
	TCPConnections int64
	UDPPackets     int64
}

type RuleDeploymentRecord struct {
	ID                string
	OrganizationID    string
	RuleID            string
	NodeID            string
	ConfigVersion     int
	RuleConfigVersion int
	Status            string
	ErrorCode         string
	ErrorMessage      string
	Protocol          string
	ListenIP          string
	Port              int
	UpdatedAt         string
}

type RuleDeploymentPendingRecord struct {
	NodeID        string
	ConfigVersion int
}

type RuleDeploymentAppliedRecord struct {
	RuleID            string
	RuleConfigVersion int
}

type RuleDeploymentFailureRecord struct {
	RuleID            string
	RuleConfigVersion int
	ErrorCode         string
	ErrorMessage      string
	Protocol          string
	ListenIP          string
	Port              int
}

type QuotaRecord struct {
	ID                string
	OrganizationID    string
	Scope             string
	SubjectUserID     string
	SubjectRuleID     string
	RuleLimit         int
	TrafficLimitBytes int64
	TrafficLimitMode  string
	OverLimitAction   string
	CreatedAt         string
	UpdatedAt         string
}

type AgentRegistrationTokenRecord struct {
	ID              string
	OrganizationID  string
	AgentType       string
	AgentID         string
	TokenHash       string
	ExpiresAt       string
	UsedAt          string
	RevokedAt       string
	CreatedAt       string
	CreatedByUserID string
}

type AgentCredentialRecord struct {
	ID                  string
	OrganizationID      string
	AgentType           string
	AgentID             string
	CredentialHash      string
	RegistrationTokenID string
	ActivatedAt         string
	RevokedAt           string
	CreatedAt           string
	RotatedAt           string
}

type AuditLogRecord struct {
	ID                   string
	OrganizationID       string
	ActorUserID          string
	ActorRolesJSON       string
	ActorPermissionsJSON string
	Action               string
	ResourceType         string
	ResourceID           string
	Result               string
	ErrorMessage         string
	MetadataJSON         string
	SourceIP             string
	CreatedAt            string
}
