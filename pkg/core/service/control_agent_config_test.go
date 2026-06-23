package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/buildinfo"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestTargetAgentVersionNormalizesLatestToBuildInfoVersion(t *testing.T) {
	previousVersion := buildinfo.Version
	buildinfo.Version = "v9.9.9"
	defer func() {
		buildinfo.Version = previousVersion
	}()

	service := NewControlServiceWithOptions(nil, ControlServiceOptions{AgentReleaseVersion: "latest"})
	if got := service.targetAgentVersion(); got != "v9.9.9" {
		t.Fatalf("expected latest to resolve to release build version, got %q", got)
	}
}

func TestTargetAgentVersionLeavesLatestUnresolvedForDevBuilds(t *testing.T) {
	previousVersion := buildinfo.Version
	buildinfo.Version = "dev"
	defer func() {
		buildinfo.Version = previousVersion
	}()

	service := NewControlServiceWithOptions(nil, ControlServiceOptions{AgentReleaseVersion: "latest"})
	targetVersion := service.targetAgentVersion()
	if targetVersion != "latest" {
		t.Fatalf("expected unresolved latest for dev build, got %q", targetVersion)
	}
	if shouldRequestAgentAutoUpdate("v0.1.9", targetVersion) {
		t.Fatalf("unresolved latest must not trigger automatic agent updates")
	}
}

func TestShouldRequestAgentAutoUpdateRequiresConcreteTarget(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		targetVersion  string
		want           bool
	}{
		{name: "blank current", currentVersion: "", targetVersion: "v1.2.3", want: false},
		{name: "blank target", currentVersion: "v1.0.0", targetVersion: "", want: false},
		{name: "latest target", currentVersion: "v1.0.0", targetVersion: "latest", want: false},
		{name: "dev target", currentVersion: "v1.0.0", targetVersion: "dev", want: false},
		{name: "unknown target", currentVersion: "v1.0.0", targetVersion: "unknown", want: false},
		{name: "same concrete target", currentVersion: "v1.2.3", targetVersion: "v1.2.3", want: false},
		{name: "different concrete target", currentVersion: "v1.0.0", targetVersion: "v1.2.3", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldRequestAgentAutoUpdate(tt.currentVersion, tt.targetVersion)
			if got != tt.want {
				t.Fatalf("shouldRequestAgentAutoUpdate(%q, %q) = %v, want %v", tt.currentVersion, tt.targetVersion, got, tt.want)
			}
		})
	}
}

func TestPublicRemoteIPRejectsCarrierGradeNAT(t *testing.T) {
	for _, remoteAddr := range []string{
		"100.64.0.1:443",
		"100.127.255.254",
	} {
		if ip := publicRemoteIP(remoteAddr); ip != nil {
			t.Fatalf("expected CGNAT remote %q to be rejected, got %s", remoteAddr, ip.String())
		}
	}
	if ip := publicRemoteIP("203.0.113.10:443"); ip == nil || ip.String() != "203.0.113.10" {
		t.Fatalf("expected public remote to be accepted, got %v", ip)
	}
}

func TestRecordNodeAgentHelloMarksMatchedPendingUpdateSucceeded(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	store := &agentHelloTestStore{
		node: repo.NodeRecord{
			ID:                     "node_1",
			OrganizationID:         "org_1",
			Name:                   "node",
			Status:                 "ONLINE",
			AgentAutoUpdateEnabled: true,
			DesiredAgentVersion:    "v1.2.3",
			AgentUpdateStatus:      "RUNNING",
			AgentUpdateError:       "",
		},
	}
	service := NewControlServiceWithOptions(store, ControlServiceOptions{AgentReleaseVersion: "v1.2.3"})
	service.now = func() time.Time { return now }

	payload, shouldUpdate, err := service.RecordNodeAgentHello(context.Background(), "org_1", "node_1", AgentHelloInput{
		Version:   "v1.2.3",
		Commit:    "abc123",
		BuildTime: "2026-01-02T03:04:05Z",
	})
	if err != nil {
		t.Fatalf("record hello: %v", err)
	}
	if shouldUpdate {
		t.Fatalf("matching desired version must not queue another update")
	}
	if store.updateRequested {
		t.Fatalf("matching desired version must not call MarkNodeAgentUpdateRequested")
	}
	if store.node.AgentUpdateStatus != "SUCCEEDED" {
		t.Fatalf("expected stored update status SUCCEEDED, got %q", store.node.AgentUpdateStatus)
	}
	if store.node.AgentUpdateFinishedAt != now.Format(time.RFC3339Nano) {
		t.Fatalf("expected finished timestamp to be recorded, got %q", store.node.AgentUpdateFinishedAt)
	}
	if payload.AgentUpdateStatus != "SUCCEEDED" {
		t.Fatalf("expected payload update status SUCCEEDED, got %q", payload.AgentUpdateStatus)
	}
}

func TestRecordNodeAgentHelloMarksMatchedFailedUpdateSucceeded(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	store := &agentHelloTestStore{
		node: repo.NodeRecord{
			ID:                     "node_1",
			OrganizationID:         "org_1",
			Name:                   "node",
			Status:                 "ONLINE",
			AgentAutoUpdateEnabled: true,
			DesiredAgentVersion:    "v1.2.3",
			AgentUpdateStatus:      "FAILED",
			AgentUpdateError:       "restart failed",
		},
	}
	service := NewControlServiceWithOptions(store, ControlServiceOptions{AgentReleaseVersion: "v1.2.3"})
	service.now = func() time.Time { return now }

	payload, shouldUpdate, err := service.RecordNodeAgentHello(context.Background(), "org_1", "node_1", AgentHelloInput{
		Version:   "v1.2.3",
		Commit:    "abc123",
		BuildTime: "2026-01-02T03:04:05Z",
	})
	if err != nil {
		t.Fatalf("record hello: %v", err)
	}
	if shouldUpdate {
		t.Fatalf("matching desired version must not queue another update")
	}
	if store.updateRequested {
		t.Fatalf("matching desired version must not call MarkNodeAgentUpdateRequested")
	}
	if store.node.AgentUpdateStatus != "SUCCEEDED" {
		t.Fatalf("expected failed update to be satisfied, got %q", store.node.AgentUpdateStatus)
	}
	if store.node.AgentUpdateError != "" {
		t.Fatalf("expected failed update error to be cleared, got %q", store.node.AgentUpdateError)
	}
	if payload.AgentUpdateStatus != "SUCCEEDED" {
		t.Fatalf("expected payload update status SUCCEEDED, got %q", payload.AgentUpdateStatus)
	}
}

func TestRequestNodeAgentUpgradeMarksCurrentVersionSatisfied(t *testing.T) {
	now := "2026-01-02T03:04:05Z"
	store := &agentHelloTestStore{
		node: repo.NodeRecord{
			ID:                  "node_1",
			OrganizationID:      "org_1",
			AgentVersion:        "v1.2.3",
			DesiredAgentVersion: "",
			AgentUpdateStatus:   "IDLE",
		},
	}

	node, err := requestNodeAgentUpgrade(context.Background(), agentHelloTestNodeRepository{store: store}, "org_1", store.node, "v1.2.3", now)
	if err != nil {
		t.Fatalf("request node agent upgrade: %v", err)
	}
	if store.updateRequested {
		t.Fatalf("same-version upgrade must not be queued as PENDING")
	}
	if !store.updateSatisfied {
		t.Fatalf("same-version upgrade must be marked satisfied")
	}
	if node.AgentUpdateStatus != "SUCCEEDED" {
		t.Fatalf("expected returned status SUCCEEDED, got %q", node.AgentUpdateStatus)
	}
	if node.DesiredAgentVersion != "v1.2.3" {
		t.Fatalf("expected desired version to be recorded, got %q", node.DesiredAgentVersion)
	}
}

func TestUpdateNodeAgentUpdatePolicyWritesAudit(t *testing.T) {
	store := &agentHelloTestStore{
		node: repo.NodeRecord{
			ID:                     "node_1",
			OrganizationID:         "org_1",
			Name:                   "node",
			AgentAutoUpdateEnabled: true,
		},
	}
	service := NewControlService(store)

	payload, err := service.UpdateNodeAgentUpdatePolicy(context.Background(), agentConfigTestIdentity(), "node_1", AgentUpdatePolicyInput{Enabled: false})
	if err != nil {
		t.Fatalf("update node agent policy: %v", err)
	}
	if payload.AgentAutoUpdateEnabled {
		t.Fatalf("expected auto update to be disabled")
	}
	requireSingleAudit(t, store, "nodes.agent_update_policy", "node_1")
}

func TestUpdateNodeAgentUpdatePolicyQueuesBehindNodeWhenEnabled(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	store := &agentHelloTestStore{
		node: repo.NodeRecord{
			ID:                     "node_1",
			OrganizationID:         "org_1",
			Name:                   "node",
			AgentVersion:           "v1.0.0",
			AgentAutoUpdateEnabled: false,
			AgentUpdateStatus:      "IDLE",
		},
	}
	service := NewControlServiceWithOptions(store, ControlServiceOptions{AgentReleaseVersion: "v1.2.3"})
	service.now = func() time.Time { return now }

	payload, err := service.UpdateNodeAgentUpdatePolicy(context.Background(), agentConfigTestIdentity(), "node_1", AgentUpdatePolicyInput{Enabled: true})
	if err != nil {
		t.Fatalf("update node agent policy: %v", err)
	}
	if !payload.AgentAutoUpdateEnabled {
		t.Fatalf("expected auto update to be enabled")
	}
	if !store.updateRequested {
		t.Fatalf("enabling auto update for a behind node must queue an update")
	}
	if payload.DesiredAgentVersion != "v1.2.3" {
		t.Fatalf("expected desired agent version v1.2.3, got %q", payload.DesiredAgentVersion)
	}
	if payload.AgentUpdateStatus != "PENDING" {
		t.Fatalf("expected pending update, got %q", payload.AgentUpdateStatus)
	}
	if store.node.AgentUpdateStartedAt != now.Format(time.RFC3339Nano) {
		t.Fatalf("expected update started timestamp to be recorded, got %q", store.node.AgentUpdateStartedAt)
	}
	requireSingleAudit(t, store, "nodes.agent_update_policy", "node_1")
}

func TestRequestNodeAgentUpgradeWritesAudit(t *testing.T) {
	store := &agentHelloTestStore{
		node: repo.NodeRecord{
			ID:                "node_1",
			OrganizationID:    "org_1",
			Name:              "node",
			AgentVersion:      "v1.0.0",
			AgentUpdateStatus: "IDLE",
		},
	}
	service := NewControlServiceWithOptions(store, ControlServiceOptions{AgentReleaseVersion: "v1.2.3"})

	payload, err := service.RequestNodeAgentUpgrade(context.Background(), agentConfigTestIdentity(), "node_1")
	if err != nil {
		t.Fatalf("request node agent upgrade: %v", err)
	}
	if payload.AgentUpdateStatus != "PENDING" {
		t.Fatalf("expected upgrade to be pending, got %q", payload.AgentUpdateStatus)
	}
	requireSingleAudit(t, store, "nodes.agent_upgrade", "node_1")
}

func TestRequestNodeAgentUpgradesWritesAuditForBatchNodes(t *testing.T) {
	store := &agentHelloTestStore{
		node: repo.NodeRecord{
			ID:                "node_1",
			OrganizationID:    "org_1",
			Name:              "node",
			AgentVersion:      "v1.0.0",
			AgentUpdateStatus: "IDLE",
		},
	}
	service := NewControlServiceWithOptions(store, ControlServiceOptions{AgentReleaseVersion: "v1.2.3"})

	payloads, err := service.RequestNodeAgentUpgrades(context.Background(), agentConfigTestIdentity(), AgentUpgradeBatchInput{NodeIDs: []string{"node_1"}})
	if err != nil {
		t.Fatalf("request node agent upgrades: %v", err)
	}
	if len(payloads) != 1 {
		t.Fatalf("expected one upgraded node, got %d", len(payloads))
	}
	requireSingleAudit(t, store, "nodes.agent_upgrade", "node_1")
}

func TestRequestNodeAgentUpgradeRejectsUnresolvedLatestTarget(t *testing.T) {
	previousVersion := buildinfo.Version
	buildinfo.Version = "dev"
	defer func() {
		buildinfo.Version = previousVersion
	}()

	store := &agentHelloTestStore{
		node: repo.NodeRecord{
			ID:                "node_1",
			OrganizationID:    "org_1",
			Name:              "node",
			AgentVersion:      "v1.0.0",
			AgentUpdateStatus: "IDLE",
		},
	}
	service := NewControlServiceWithOptions(store, ControlServiceOptions{AgentReleaseVersion: "latest"})

	_, err := service.RequestNodeAgentUpgrade(context.Background(), agentConfigTestIdentity(), "node_1")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	if store.updateRequested {
		t.Fatalf("unresolved latest must not queue a manual agent update")
	}
	if store.node.DesiredAgentVersion != "" {
		t.Fatalf("unresolved latest must not be persisted as desired version, got %q", store.node.DesiredAgentVersion)
	}
	if len(store.auditLogs) != 0 {
		t.Fatalf("failed unresolved upgrade must not write audit log, got %d", len(store.auditLogs))
	}
}

func TestRequestNodeAgentUpgradesRejectsUnresolvedLatestTarget(t *testing.T) {
	previousVersion := buildinfo.Version
	buildinfo.Version = "dev"
	defer func() {
		buildinfo.Version = previousVersion
	}()

	store := &agentHelloTestStore{
		node: repo.NodeRecord{
			ID:                "node_1",
			OrganizationID:    "org_1",
			Name:              "node",
			AgentVersion:      "v1.0.0",
			AgentUpdateStatus: "IDLE",
		},
	}
	service := NewControlServiceWithOptions(store, ControlServiceOptions{AgentReleaseVersion: "latest"})

	_, err := service.RequestNodeAgentUpgrades(context.Background(), agentConfigTestIdentity(), AgentUpgradeBatchInput{NodeIDs: []string{"node_1"}})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	if store.updateRequested {
		t.Fatalf("unresolved latest must not queue batch agent updates")
	}
	if store.node.DesiredAgentVersion != "" {
		t.Fatalf("unresolved latest must not be persisted as desired version, got %q", store.node.DesiredAgentVersion)
	}
	if len(store.auditLogs) != 0 {
		t.Fatalf("failed unresolved batch upgrade must not write audit log, got %d", len(store.auditLogs))
	}
}

func TestPendingNodeAgentUpdateDoesNotRepeatRunningUpdate(t *testing.T) {
	store := &agentHelloTestStore{
		node: repo.NodeRecord{
			ID:                  "node_1",
			OrganizationID:      "org_1",
			AgentVersion:        "v1.0.0",
			DesiredAgentVersion: "v1.2.3",
			AgentUpdateStatus:   "RUNNING",
		},
	}
	service := NewControlService(store)

	targetVersion, pending, err := service.PendingNodeAgentUpdate(context.Background(), "org_1", "node_1")
	if err != nil {
		t.Fatalf("pending node agent update: %v", err)
	}
	if pending {
		t.Fatalf("RUNNING update must not emit another agent_update_request for %q", targetVersion)
	}
}

type agentHelloTestStore struct {
	node            repo.NodeRecord
	updateRequested bool
	updateSatisfied bool
	auditLogs       []repo.AuditLogRecord
}

func (store *agentHelloTestStore) WithinTx(ctx context.Context, fn func(context.Context, repo.Repositories) error) error {
	return fn(ctx, agentHelloTestRepositories{store: store})
}

type agentHelloTestRepositories struct {
	store *agentHelloTestStore
}

func (repositories agentHelloTestRepositories) Users() repo.UserRepository { return nil }
func (repositories agentHelloTestRepositories) Organizations() repo.OrganizationRepository {
	return nil
}
func (repositories agentHelloTestRepositories) Members() repo.MemberRepository { return nil }
func (repositories agentHelloTestRepositories) Roles() repo.RoleRepository     { return nil }
func (repositories agentHelloTestRepositories) NodeGroups() repo.NodeGroupRepository {
	return nil
}
func (repositories agentHelloTestRepositories) Nodes() repo.NodeRepository {
	return agentHelloTestNodeRepository(repositories)
}
func (repositories agentHelloTestRepositories) MonitorGroups() repo.MonitorGroupRepository {
	return nil
}
func (repositories agentHelloTestRepositories) Monitors() repo.MonitorRepository { return nil }
func (repositories agentHelloTestRepositories) HealthChecks() repo.HealthCheckRepository {
	return nil
}
func (repositories agentHelloTestRepositories) DNSCredentials() repo.DNSCredentialRepository {
	return nil
}
func (repositories agentHelloTestRepositories) DNSRecords() repo.DNSRecordRepository {
	return nil
}
func (repositories agentHelloTestRepositories) Targets() repo.TargetRepository { return nil }
func (repositories agentHelloTestRepositories) TargetGroups() repo.TargetGroupRepository {
	return nil
}
func (repositories agentHelloTestRepositories) Rules() repo.RuleRepository   { return nil }
func (repositories agentHelloTestRepositories) Quotas() repo.QuotaRepository { return nil }
func (repositories agentHelloTestRepositories) AgentRegistrationTokens() repo.AgentRegistrationTokenRepository {
	return nil
}
func (repositories agentHelloTestRepositories) AgentCredentials() repo.AgentCredentialRepository {
	return nil
}
func (repositories agentHelloTestRepositories) AuditLogs() repo.AuditLogRepository {
	return agentHelloTestAuditRepository(repositories)
}

type agentHelloTestNodeRepository struct {
	store *agentHelloTestStore
}

func (nodes agentHelloTestNodeRepository) ListNodesByOrganization(context.Context, string) ([]repo.NodeRecord, error) {
	return []repo.NodeRecord{nodes.store.node}, nil
}

func (nodes agentHelloTestNodeRepository) FindNodeByID(context.Context, string, string) (repo.NodeRecord, error) {
	return nodes.store.node, nil
}

func (nodes agentHelloTestNodeRepository) CreateNode(context.Context, repo.NodeRecord, []string, []repo.NodeListenIPRecord, []repo.NodePortRangeRecord, string, func() string) error {
	return nil
}

func (nodes agentHelloTestNodeRepository) UpdateNode(context.Context, repo.NodeRecord, bool, []string, bool, []repo.NodeListenIPRecord, bool, []repo.NodePortRangeRecord, string, func() string) error {
	return nil
}

func (nodes agentHelloTestNodeRepository) MarkNodeAgentConnected(_ context.Context, _ string, _ string, now string) error {
	nodes.store.node.Status = "ONLINE"
	nodes.store.node.LastSeenAt = now
	return nil
}

func (nodes agentHelloTestNodeRepository) UpdateNodeAgentVersion(_ context.Context, _ string, _ string, version repo.NodeAgentVersionRecord, now string) error {
	nodes.store.node.AgentVersion = version.Version
	nodes.store.node.AgentCommit = version.Commit
	nodes.store.node.AgentBuildTime = version.BuildTime
	nodes.store.node.LastSeenAt = now
	return nil
}

func (nodes agentHelloTestNodeRepository) UpdateNodeAgentUpdatePolicy(_ context.Context, _ string, _ string, enabled bool, now string) error {
	nodes.store.node.AgentAutoUpdateEnabled = enabled
	nodes.store.node.UpdatedAt = now
	return nil
}

func (nodes agentHelloTestNodeRepository) MarkNodeAgentUpdateRequested(_ context.Context, _ string, _ string, targetVersion string, now string) error {
	nodes.store.updateRequested = true
	nodes.store.node.DesiredAgentVersion = targetVersion
	nodes.store.node.AgentUpdateStatus = "PENDING"
	nodes.store.node.AgentUpdateError = ""
	nodes.store.node.AgentUpdateStartedAt = now
	nodes.store.node.AgentUpdateFinishedAt = ""
	return nil
}

func (nodes agentHelloTestNodeRepository) MarkNodeAgentUpdateSatisfied(_ context.Context, _ string, _ string, targetVersion string, now string) error {
	nodes.store.updateSatisfied = true
	nodes.store.node.DesiredAgentVersion = targetVersion
	nodes.store.node.AgentUpdateStatus = "SUCCEEDED"
	nodes.store.node.AgentUpdateError = ""
	nodes.store.node.AgentUpdateStartedAt = defaultString(nodes.store.node.AgentUpdateStartedAt, now)
	nodes.store.node.AgentUpdateFinishedAt = now
	return nil
}

func (nodes agentHelloTestNodeRepository) RecordNodeAgentUpdateResult(_ context.Context, _ string, _ string, status string, errorMessage string, now string) error {
	nodes.store.node.AgentUpdateStatus = status
	nodes.store.node.AgentUpdateError = errorMessage
	nodes.store.node.AgentUpdateFinishedAt = now
	return nil
}

func (nodes agentHelloTestNodeRepository) MarkNodeAgentDisconnected(context.Context, string, string, string) error {
	return nil
}

func (nodes agentHelloTestNodeRepository) RecordNodeConfigAck(context.Context, string, string, repo.NodeConfigAckRecord, string) error {
	return nil
}

func (nodes agentHelloTestNodeRepository) EnsureDesiredConfigVersionAtLeast(context.Context, string, string, int, string) error {
	return nil
}

func (nodes agentHelloTestNodeRepository) IncrementDesiredConfigForNode(context.Context, string, string, string) error {
	return nil
}

func (nodes agentHelloTestNodeRepository) IncrementDesiredConfigForNodeGroup(context.Context, string, string, string) error {
	return nil
}

func (nodes agentHelloTestNodeRepository) DeleteNode(context.Context, string, string, string) error {
	return nil
}

type agentHelloTestAuditRepository struct {
	store *agentHelloTestStore
}

func (audits agentHelloTestAuditRepository) CreateAuditLog(_ context.Context, audit repo.AuditLogRecord) error {
	audits.store.auditLogs = append(audits.store.auditLogs, audit)
	return nil
}

func agentConfigTestIdentity() InternalIdentity {
	return InternalIdentity{
		UserID:         "user_1",
		OrganizationID: "org_1",
		Permissions:    []string{string(domain.PermissionNodesManage)},
		SourceIP:       "203.0.113.10",
	}
}

func requireSingleAudit(t *testing.T, store *agentHelloTestStore, action string, resourceID string) {
	t.Helper()
	if len(store.auditLogs) != 1 {
		t.Fatalf("expected one audit log, got %d", len(store.auditLogs))
	}
	audit := store.auditLogs[0]
	if audit.Action != action {
		t.Fatalf("audit action = %q, want %q", audit.Action, action)
	}
	if audit.ResourceType != "NODE" {
		t.Fatalf("audit resource type = %q, want NODE", audit.ResourceType)
	}
	if audit.ResourceID != resourceID {
		t.Fatalf("audit resource id = %q, want %q", audit.ResourceID, resourceID)
	}
	if audit.OrganizationID != "org_1" {
		t.Fatalf("audit organization = %q, want org_1", audit.OrganizationID)
	}
	if audit.ActorUserID != "user_1" {
		t.Fatalf("audit actor = %q, want user_1", audit.ActorUserID)
	}
	if audit.Result != "SUCCESS" {
		t.Fatalf("audit result = %q, want SUCCESS", audit.Result)
	}
}
