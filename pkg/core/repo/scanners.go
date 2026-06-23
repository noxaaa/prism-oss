package repo

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
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
	if err := row.Scan(&role.ID, &role.OrganizationID, &role.Key, &role.Name, &role.Description, &role.IsSystem, &role.CreatedAt, &role.UpdatedAt, &role.DeletedAt); err != nil {
		return RoleRecord{}, mapReadError(err)
	}
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
		&node.ConfigStatusConfigVersion,
		&node.ConfigRetryCount,
		&node.ConfigNextRetryAt,
		&node.ConfigStatusUpdatedAt,
		&node.LastSeenAt,
		&node.RegisteredAt,
		&node.AgentVersion,
		&node.AgentCommit,
		&node.AgentBuildTime,
		&node.AgentAutoUpdateEnabled,
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
	return node, nil
}

func scanNodeRows(rows *sql.Rows) (NodeRecord, error) {
	return scanNode(rows)
}

func scanNodeListenIP(row rowScanner) (NodeListenIPRecord, error) {
	var listenIP NodeListenIPRecord
	if err := row.Scan(&listenIP.ID, &listenIP.OrganizationID, &listenIP.NodeID, &listenIP.ListenIP, &listenIP.DisplayName, &listenIP.Enabled, &listenIP.CreatedAt, &listenIP.UpdatedAt); err != nil {
		return NodeListenIPRecord{}, mapReadError(err)
	}
	return listenIP, nil
}

func scanNodeListenIPRows(rows *sql.Rows) (NodeListenIPRecord, error) {
	return scanNodeListenIP(rows)
}

func scanNodePortRange(row rowScanner) (NodePortRangeRecord, error) {
	var portRange NodePortRangeRecord
	if err := row.Scan(&portRange.ID, &portRange.OrganizationID, &portRange.NodeID, &portRange.Protocol, &portRange.StartPort, &portRange.EndPort, &portRange.Enabled, &portRange.CreatedAt, &portRange.UpdatedAt); err != nil {
		return NodePortRangeRecord{}, mapReadError(err)
	}
	return portRange, nil
}

func scanNodePortRangeRows(rows *sql.Rows) (NodePortRangeRecord, error) {
	return scanNodePortRange(rows)
}

func scanNodeDNSPublishAddress(row rowScanner) (NodeDNSPublishAddressRecord, error) {
	var address NodeDNSPublishAddressRecord
	if err := row.Scan(&address.ID, &address.OrganizationID, &address.NodeID, &address.AddressType, &address.Address, &address.Source, &address.Enabled, &address.ObservedAt, &address.CreatedAt, &address.UpdatedAt); err != nil {
		return NodeDNSPublishAddressRecord{}, mapReadError(err)
	}
	return address, nil
}

func scanNodeDNSPublishAddressRows(rows *sql.Rows) (NodeDNSPublishAddressRecord, error) {
	return scanNodeDNSPublishAddress(rows)
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
	if err := row.Scan(&target.ID, &target.OrganizationID, &target.Name, &target.Host, &target.Port, &target.Enabled, &target.CreatedAt, &target.UpdatedAt, &target.DeletedAt); err != nil {
		return TargetRecord{}, mapReadError(err)
	}
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
	if err := row.Scan(&member.ID, &member.OrganizationID, &member.TargetGroupID, &member.TargetID, &member.Priority, &member.Enabled, &member.CreatedAt, &member.UpdatedAt); err != nil {
		return TargetGroupMemberRecord{}, mapReadError(err)
	}
	return member, nil
}

func scanTargetGroupMemberRows(rows *sql.Rows) (TargetGroupMemberRecord, error) {
	return scanTargetGroupMember(rows)
}

func scanRule(row rowScanner) (RuleRecord, error) {
	var rule RuleRecord
	if err := row.Scan(
		&rule.ID,
		&rule.OrganizationID,
		&rule.OwnerUserID,
		&rule.Name,
		&rule.Enabled,
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
		&rule.FailurePolicy,
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
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505", "23503", "23514", "23P01":
			return fmt.Errorf("%w: %v", ErrConflict, err)
		}
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

func boolToDB(value bool) bool {
	return value
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
