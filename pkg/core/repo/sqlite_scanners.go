package repo

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (UserRecord, error) {
	var user UserRecord
	if err := row.Scan(&user.ID, &user.Email, &user.Name); err != nil {
		return UserRecord{}, mapReadError(err)
	}
	return user, nil
}

func scanOrganization(row rowScanner) (OrganizationRecord, error) {
	var organization OrganizationRecord
	if err := row.Scan(&organization.ID, &organization.Name, &organization.Slug, &organization.OwnerUserID, &organization.DefaultRuleLimit, &organization.DefaultTrafficLimitBytes, &organization.DefaultTrafficLimitMode, &organization.CreatedAt, &organization.UpdatedAt, &organization.DeletedAt); err != nil {
		return OrganizationRecord{}, mapReadError(err)
	}
	return organization, nil
}

func scanOrganizationRows(rows *sql.Rows) (OrganizationRecord, error) {
	return scanOrganization(rows)
}

func scanMember(row rowScanner) (MemberRecord, error) {
	var member MemberRecord
	if err := row.Scan(&member.ID, &member.OrganizationID, &member.UserID, &member.UserEmail, &member.UserName, &member.Status, &member.CreatedAt, &member.UpdatedAt); err != nil {
		return MemberRecord{}, mapReadError(err)
	}
	return member, nil
}

func scanMemberRows(rows *sql.Rows) (MemberRecord, error) {
	return scanMember(rows)
}

func scanRole(row rowScanner) (RoleRecord, error) {
	var role RoleRecord
	var isSystem int
	if err := row.Scan(&role.ID, &role.OrganizationID, &role.Key, &role.Name, &role.Description, &isSystem, &role.CreatedAt, &role.UpdatedAt, &role.DeletedAt); err != nil {
		return RoleRecord{}, mapReadError(err)
	}
	role.IsSystem = isSystem == 1
	return role, nil
}

func scanRoleRows(rows *sql.Rows) (RoleRecord, error) {
	return scanRole(rows)
}

func scanNodeGroup(row rowScanner) (NodeGroupRecord, error) {
	var nodeGroup NodeGroupRecord
	if err := row.Scan(&nodeGroup.ID, &nodeGroup.OrganizationID, &nodeGroup.Name, &nodeGroup.Description, &nodeGroup.CreatedAt, &nodeGroup.UpdatedAt, &nodeGroup.DeletedAt); err != nil {
		return NodeGroupRecord{}, mapReadError(err)
	}
	return nodeGroup, nil
}

func scanNodeGroupRows(rows *sql.Rows) (NodeGroupRecord, error) {
	return scanNodeGroup(rows)
}

func scanNode(row rowScanner) (NodeRecord, error) {
	var node NodeRecord
	var autoUpdate int
	if err := row.Scan(
		&node.ID,
		&node.OrganizationID,
		&node.Name,
		&node.Status,
		&node.PublicDescription,
		&node.DesiredConfigVersion,
		&node.AppliedConfigVersion,
		&node.ConfigStatus,
		&node.ConfigErrorMessage,
		&node.ConfigStatusUpdatedAt,
		&node.LastSeenAt,
		&node.RegisteredAt,
		&node.AgentVersion,
		&node.AgentCommit,
		&node.AgentBuildTime,
		&autoUpdate,
		&node.DesiredAgentVersion,
		&node.AgentUpdateStatus,
		&node.AgentUpdateError,
		&node.AgentUpdateStartedAt,
		&node.AgentUpdateFinishedAt,
		&node.CreatedAt,
		&node.UpdatedAt,
		&node.DeletedAt,
	); err != nil {
		return NodeRecord{}, mapReadError(err)
	}
	node.AgentAutoUpdateEnabled = autoUpdate == 1
	return node, nil
}

func scanNodeRows(rows *sql.Rows) (NodeRecord, error) {
	return scanNode(rows)
}

func scanNodeListenIP(row rowScanner) (NodeListenIPRecord, error) {
	var listenIP NodeListenIPRecord
	var enabled int
	if err := row.Scan(&listenIP.ID, &listenIP.OrganizationID, &listenIP.NodeID, &listenIP.ListenIP, &listenIP.DisplayName, &enabled, &listenIP.CreatedAt, &listenIP.UpdatedAt); err != nil {
		return NodeListenIPRecord{}, mapReadError(err)
	}
	listenIP.Enabled = enabled == 1
	return listenIP, nil
}

func scanNodeListenIPRows(rows *sql.Rows) (NodeListenIPRecord, error) {
	return scanNodeListenIP(rows)
}

func scanNodePortRange(row rowScanner) (NodePortRangeRecord, error) {
	var portRange NodePortRangeRecord
	var enabled int
	if err := row.Scan(&portRange.ID, &portRange.OrganizationID, &portRange.NodeID, &portRange.Protocol, &portRange.StartPort, &portRange.EndPort, &enabled, &portRange.CreatedAt, &portRange.UpdatedAt); err != nil {
		return NodePortRangeRecord{}, mapReadError(err)
	}
	portRange.Enabled = enabled == 1
	return portRange, nil
}

func scanNodePortRangeRows(rows *sql.Rows) (NodePortRangeRecord, error) {
	return scanNodePortRange(rows)
}

func scanMonitorGroup(row rowScanner) (MonitorGroupRecord, error) {
	var monitorGroup MonitorGroupRecord
	if err := row.Scan(&monitorGroup.ID, &monitorGroup.OrganizationID, &monitorGroup.Name, &monitorGroup.Description, &monitorGroup.CreatedAt, &monitorGroup.UpdatedAt, &monitorGroup.DeletedAt); err != nil {
		return MonitorGroupRecord{}, mapReadError(err)
	}
	return monitorGroup, nil
}

func scanMonitorGroupRows(rows *sql.Rows) (MonitorGroupRecord, error) {
	return scanMonitorGroup(rows)
}

func scanMonitor(row rowScanner) (MonitorRecord, error) {
	var monitor MonitorRecord
	if err := row.Scan(
		&monitor.ID,
		&monitor.OrganizationID,
		&monitor.Name,
		&monitor.Status,
		&monitor.DesiredConfigVersion,
		&monitor.AppliedConfigVersion,
		&monitor.LastSeenAt,
		&monitor.RegisteredAt,
		&monitor.CreatedAt,
		&monitor.UpdatedAt,
		&monitor.DeletedAt,
	); err != nil {
		return MonitorRecord{}, mapReadError(err)
	}
	return monitor, nil
}

func scanMonitorRows(rows *sql.Rows) (MonitorRecord, error) {
	return scanMonitor(rows)
}

func scanTarget(row rowScanner) (TargetRecord, error) {
	var target TargetRecord
	var enabled int
	if err := row.Scan(&target.ID, &target.OrganizationID, &target.Name, &target.Host, &target.Port, &enabled, &target.CreatedAt, &target.UpdatedAt, &target.DeletedAt); err != nil {
		return TargetRecord{}, mapReadError(err)
	}
	target.Enabled = enabled == 1
	return target, nil
}

func scanTargetRows(rows *sql.Rows) (TargetRecord, error) {
	return scanTarget(rows)
}

func scanTargetGroup(row rowScanner) (TargetGroupRecord, error) {
	var targetGroup TargetGroupRecord
	if err := row.Scan(&targetGroup.ID, &targetGroup.OrganizationID, &targetGroup.Name, &targetGroup.Description, &targetGroup.Scheduler, &targetGroup.CreatedAt, &targetGroup.UpdatedAt, &targetGroup.DeletedAt); err != nil {
		return TargetGroupRecord{}, mapReadError(err)
	}
	return targetGroup, nil
}

func scanTargetGroupRows(rows *sql.Rows) (TargetGroupRecord, error) {
	return scanTargetGroup(rows)
}

func scanTargetGroupMember(row rowScanner) (TargetGroupMemberRecord, error) {
	var member TargetGroupMemberRecord
	var enabled int
	if err := row.Scan(&member.ID, &member.OrganizationID, &member.TargetGroupID, &member.TargetID, &member.Priority, &enabled, &member.CreatedAt, &member.UpdatedAt); err != nil {
		return TargetGroupMemberRecord{}, mapReadError(err)
	}
	member.Enabled = enabled == 1
	return member, nil
}

func scanTargetGroupMemberRows(rows *sql.Rows) (TargetGroupMemberRecord, error) {
	return scanTargetGroupMember(rows)
}

func scanRule(row rowScanner) (RuleRecord, error) {
	var rule RuleRecord
	var enabled int
	if err := row.Scan(
		&rule.ID,
		&rule.OrganizationID,
		&rule.OwnerUserID,
		&rule.Name,
		&enabled,
		&rule.Status,
		&rule.ForwardingType,
		&rule.Protocol,
		&rule.MatchType,
		&rule.InboundBindingID,
		&rule.SNIHostname,
		&rule.TargetType,
		&rule.TargetID,
		&rule.TargetGroupID,
		&rule.ProxyProtocolIn,
		&rule.ProxyProtocolOut,
		&rule.ConfigVersion,
		&rule.CreatedAt,
		&rule.UpdatedAt,
		&rule.DeletedAt,
		&rule.Binding.ID,
		&rule.Binding.OrganizationID,
		&rule.Binding.NodeGroupID,
		&rule.Binding.ListenIP,
		&rule.Binding.Protocol,
		&rule.Binding.Port,
		&rule.Binding.MatchType,
		&rule.Binding.CreatedAt,
	); err != nil {
		return RuleRecord{}, mapReadError(err)
	}
	rule.Enabled = enabled == 1
	return rule, nil
}

func scanRuleRows(rows *sql.Rows) (RuleRecord, error) {
	return scanRule(rows)
}

func scanRegistrationToken(row rowScanner) (AgentRegistrationTokenRecord, error) {
	var token AgentRegistrationTokenRecord
	if err := row.Scan(
		&token.ID,
		&token.OrganizationID,
		&token.AgentType,
		&token.AgentID,
		&token.TokenHash,
		&token.ExpiresAt,
		&token.UsedAt,
		&token.RevokedAt,
		&token.CreatedAt,
		&token.CreatedByUserID,
	); err != nil {
		return AgentRegistrationTokenRecord{}, mapReadError(err)
	}
	return token, nil
}

func scanRegistrationTokenRows(rows *sql.Rows) (AgentRegistrationTokenRecord, error) {
	return scanRegistrationToken(rows)
}

func scanAgentCredential(row rowScanner) (AgentCredentialRecord, error) {
	var credential AgentCredentialRecord
	if err := row.Scan(
		&credential.ID,
		&credential.OrganizationID,
		&credential.AgentType,
		&credential.AgentID,
		&credential.CredentialHash,
		&credential.RegistrationTokenID,
		&credential.ActivatedAt,
		&credential.RevokedAt,
		&credential.CreatedAt,
		&credential.RotatedAt,
	); err != nil {
		return AgentCredentialRecord{}, mapReadError(err)
	}
	return credential, nil
}

func mapReadError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func mapWriteError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique") || strings.Contains(message, "constraint") {
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return err
}

func requireAffected(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullIfEmpty(value string) any {
	return nullable(value)
}
