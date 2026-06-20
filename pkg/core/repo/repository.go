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
	RecordNodeConfigAck(ctx context.Context, organizationID string, nodeID string, configVersion int, status string, errorMessage string, now string) error
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
	DeleteMonitor(ctx context.Context, organizationID string, monitorID string, deletedAt string) error
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
	ID                     string
	OrganizationID         string
	Name                   string
	Status                 string
	PublicDescription      string
	DesiredConfigVersion   int
	AppliedConfigVersion   int
	ConfigStatus           string
	ConfigErrorMessage     string
	ConfigStatusUpdatedAt  string
	LastSeenAt             string
	RegisteredAt           string
	AgentVersion           string
	AgentCommit            string
	AgentBuildTime         string
	AgentAutoUpdateEnabled bool
	DesiredAgentVersion    string
	AgentUpdateStatus      string
	AgentUpdateError       string
	AgentUpdateStartedAt   string
	AgentUpdateFinishedAt  string
	CreatedAt              string
	UpdatedAt              string
	DeletedAt              string
	GroupIDs               []string
	ListenIPs              []NodeListenIPRecord
	PortRanges             []NodePortRangeRecord
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
