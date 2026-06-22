package repo

import (
	"context"
	"strings"
)

func (store *PostgresStore) ListTargetsByOrganization(ctx context.Context, organizationID string) ([]TargetRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, name, host, port, enabled, created_at, updated_at, coalesce(deleted_at::text, '')
		FROM targets
		WHERE organization_id = ? AND deleted_at IS NULL
		ORDER BY name, id
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	targets := make([]TargetRecord, 0)
	for rows.Next() {
		target, err := scanTargetRows(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, rows.Err()
}

func (store *PostgresStore) FindTargetByID(ctx context.Context, organizationID string, targetID string) (TargetRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT id, organization_id, name, host, port, enabled, created_at, updated_at, coalesce(deleted_at::text, '')
		FROM targets
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, organizationID, targetID)
	return scanTarget(row)
}

func (store *PostgresStore) CreateTarget(ctx context.Context, target TargetRecord) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO targets (id, organization_id, name, host, port, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, target.ID, target.OrganizationID, target.Name, target.Host, target.Port, boolToDB(target.Enabled), target.CreatedAt, target.UpdatedAt)
	return mapWriteError(err)
}

func (store *PostgresStore) UpdateTarget(ctx context.Context, target TargetRecord) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE targets
		SET name = ?, host = ?, port = ?, enabled = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, target.Name, target.Host, target.Port, boolToDB(target.Enabled), target.UpdatedAt, target.OrganizationID, target.ID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) DeleteTarget(ctx context.Context, organizationID string, targetID string, deletedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE targets
		SET deleted_at = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, deletedAt, deletedAt, organizationID, targetID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) ListTargetGroupsByOrganization(ctx context.Context, organizationID string) ([]TargetGroupRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, name, description, scheduler, created_at, updated_at, coalesce(deleted_at::text, '')
		FROM target_groups
		WHERE organization_id = ? AND deleted_at IS NULL
		ORDER BY name, id
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	groups := make([]TargetGroupRecord, 0)
	for rows.Next() {
		group, err := scanTargetGroupRows(rows)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range groups {
		if err := store.loadTargetGroupMembers(ctx, &groups[index]); err != nil {
			return nil, err
		}
	}
	return groups, nil
}

func (store *PostgresStore) FindTargetGroupByID(ctx context.Context, organizationID string, targetGroupID string) (TargetGroupRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT id, organization_id, name, description, scheduler, created_at, updated_at, coalesce(deleted_at::text, '')
		FROM target_groups
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, organizationID, targetGroupID)
	group, err := scanTargetGroup(row)
	if err != nil {
		return TargetGroupRecord{}, err
	}
	if err := store.loadTargetGroupMembers(ctx, &group); err != nil {
		return TargetGroupRecord{}, err
	}
	return group, nil
}

func (store *PostgresStore) CreateTargetGroup(ctx context.Context, targetGroup TargetGroupRecord, members []TargetGroupMemberRecord, now string, nextID func() string) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO target_groups (id, organization_id, name, description, scheduler, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, targetGroup.ID, targetGroup.OrganizationID, targetGroup.Name, targetGroup.Description, targetGroup.Scheduler, targetGroup.CreatedAt, targetGroup.UpdatedAt)
	if err != nil {
		return mapWriteError(err)
	}
	return store.replaceTargetGroupMembers(ctx, targetGroup.OrganizationID, targetGroup.ID, members, now, nextID)
}

func (store *PostgresStore) UpdateTargetGroup(ctx context.Context, targetGroup TargetGroupRecord, members []TargetGroupMemberRecord, now string, nextID func() string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE target_groups
		SET name = ?, description = ?, scheduler = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, targetGroup.Name, targetGroup.Description, targetGroup.Scheduler, targetGroup.UpdatedAt, targetGroup.OrganizationID, targetGroup.ID)
	if err != nil {
		return mapWriteError(err)
	}
	if err := requireAffected(result); err != nil {
		return err
	}
	return store.replaceTargetGroupMembers(ctx, targetGroup.OrganizationID, targetGroup.ID, members, now, nextID)
}

func (store *PostgresStore) DeleteTargetGroup(ctx context.Context, organizationID string, targetGroupID string, deletedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE target_groups
		SET deleted_at = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, deletedAt, deletedAt, organizationID, targetGroupID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) ListRulesByOrganization(ctx context.Context, organizationID string) ([]RuleRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT forwarding_rules.id, forwarding_rules.organization_id, owner_user_id, name, enabled, status,
		       forwarding_type, forwarding_rules.protocol, forwarding_rules.match_type, inbound_binding_id, coalesce(sni_hostname, ''),
		       target_type, coalesce(target_id::text, ''), coalesce(target_group_id::text, ''), proxy_protocol_in, proxy_protocol_out,
		       failure_policy, config_version, forwarding_rules.created_at, forwarding_rules.updated_at, coalesce(forwarding_rules.deleted_at::text, ''),
		       inbound_bindings.id, inbound_bindings.organization_id, inbound_bindings.node_group_id, inbound_bindings.listen_ip,
		       inbound_bindings.protocol, inbound_bindings.port, inbound_bindings.match_type, inbound_bindings.created_at
		FROM forwarding_rules
		JOIN inbound_bindings
		  ON inbound_bindings.organization_id = forwarding_rules.organization_id
		 AND inbound_bindings.id = forwarding_rules.inbound_binding_id
		WHERE forwarding_rules.organization_id = ? AND forwarding_rules.deleted_at IS NULL
		ORDER BY forwarding_rules.name, forwarding_rules.id
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	rules := make([]RuleRecord, 0)
	for rows.Next() {
		rule, err := scanRuleRows(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range rules {
		if err := store.loadRuleTags(ctx, &rules[index]); err != nil {
			return nil, err
		}
	}
	return rules, nil
}

func (store *PostgresStore) FindRuleByID(ctx context.Context, organizationID string, ruleID string) (RuleRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT forwarding_rules.id, forwarding_rules.organization_id, owner_user_id, name, enabled, status,
		       forwarding_type, forwarding_rules.protocol, forwarding_rules.match_type, inbound_binding_id, coalesce(sni_hostname, ''),
		       target_type, coalesce(target_id::text, ''), coalesce(target_group_id::text, ''), proxy_protocol_in, proxy_protocol_out,
		       failure_policy, config_version, forwarding_rules.created_at, forwarding_rules.updated_at, coalesce(forwarding_rules.deleted_at::text, ''),
		       inbound_bindings.id, inbound_bindings.organization_id, inbound_bindings.node_group_id, inbound_bindings.listen_ip,
		       inbound_bindings.protocol, inbound_bindings.port, inbound_bindings.match_type, inbound_bindings.created_at
		FROM forwarding_rules
		JOIN inbound_bindings
		  ON inbound_bindings.organization_id = forwarding_rules.organization_id
		 AND inbound_bindings.id = forwarding_rules.inbound_binding_id
		WHERE forwarding_rules.organization_id = ? AND forwarding_rules.id = ? AND forwarding_rules.deleted_at IS NULL
	`, organizationID, ruleID)
	rule, err := scanRule(row)
	if err != nil {
		return RuleRecord{}, err
	}
	if err := store.loadRuleTags(ctx, &rule); err != nil {
		return RuleRecord{}, err
	}
	return rule, nil
}

func (store *PostgresStore) CreateRule(ctx context.Context, rule RuleRecord, binding InboundBindingRecord, tags []string, now string, nextID func() string) error {
	if err := store.upsertInboundBinding(ctx, binding); err != nil {
		return err
	}
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, status, forwarding_type, protocol, match_type,
			inbound_binding_id, sni_hostname, target_type, target_id, target_group_id,
			proxy_protocol_in, proxy_protocol_out, failure_policy, config_version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, nullif(?, '')::uuid, nullif(?, '')::uuid, ?, ?, ?, ?, ?, ?)
	`, rule.ID, rule.OrganizationID, rule.OwnerUserID, rule.Name, boolToDB(rule.Enabled), rule.Status, rule.ForwardingType, rule.Protocol, rule.MatchType,
		binding.ID, nullIfEmpty(rule.SNIHostname), rule.TargetType, rule.TargetID, rule.TargetGroupID,
		rule.ProxyProtocolIn, rule.ProxyProtocolOut, rule.FailurePolicy, rule.ConfigVersion, rule.CreatedAt, rule.UpdatedAt)
	if err != nil {
		return mapWriteError(err)
	}
	return store.replaceRuleTags(ctx, rule.OrganizationID, rule.ID, tags, now, nextID)
}

func (store *PostgresStore) UpdateRule(ctx context.Context, rule RuleRecord, binding InboundBindingRecord, tags []string, now string, nextID func() string) error {
	if err := store.upsertInboundBinding(ctx, binding); err != nil {
		return err
	}
	result, err := store.db.ExecContext(ctx, `
		UPDATE forwarding_rules
		SET name = ?, enabled = ?, status = ?, forwarding_type = ?, protocol = ?, match_type = ?, inbound_binding_id = ?,
		    sni_hostname = ?, target_type = ?, target_id = nullif(?, '')::uuid, target_group_id = nullif(?, '')::uuid,
		    proxy_protocol_in = ?, proxy_protocol_out = ?, failure_policy = ?, config_version = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, rule.Name, boolToDB(rule.Enabled), rule.Status, rule.ForwardingType, rule.Protocol, rule.MatchType, binding.ID,
		nullIfEmpty(rule.SNIHostname), rule.TargetType, rule.TargetID, rule.TargetGroupID,
		rule.ProxyProtocolIn, rule.ProxyProtocolOut, rule.FailurePolicy, rule.ConfigVersion, rule.UpdatedAt, rule.OrganizationID, rule.ID)
	if err != nil {
		return mapWriteError(err)
	}
	if err := requireAffected(result); err != nil {
		return err
	}
	return store.replaceRuleTags(ctx, rule.OrganizationID, rule.ID, tags, now, nextID)
}

func (store *PostgresStore) DeleteRule(ctx context.Context, organizationID string, ruleID string, deletedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE forwarding_rules
		SET deleted_at = ?, updated_at = ?, enabled = false, status = 'DISABLED'
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, deletedAt, deletedAt, organizationID, ruleID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) ListEnabledInboundBindings(ctx context.Context, organizationID string) ([]RuleRecord, error) {
	rules, err := store.ListRulesByOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	filtered := make([]RuleRecord, 0, len(rules))
	for _, rule := range rules {
		if rule.Enabled && rule.Status == "ENABLED" {
			filtered = append(filtered, rule)
		}
	}
	return filtered, nil
}

func (store *PostgresStore) CountRulesByOrganization(ctx context.Context, organizationID string) (int, error) {
	var count int
	if err := store.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM forwarding_rules
		WHERE organization_id = ? AND deleted_at IS NULL
	`, organizationID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (store *PostgresStore) CountRulesByOwner(ctx context.Context, organizationID string, ownerUserID string) (int, error) {
	var count int
	if err := store.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM forwarding_rules
		WHERE organization_id = ? AND owner_user_id = ? AND deleted_at IS NULL
	`, organizationID, ownerUserID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (store *PostgresStore) SumRuleTraffic(ctx context.Context, organizationID string, ruleID string) (RuleTrafficRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT coalesce(sum(upload_bytes), 0), coalesce(sum(download_bytes), 0),
		       coalesce(sum(tcp_connections), 0), coalesce(sum(udp_packets), 0)
		FROM rule_traffic_counters
		WHERE organization_id = ? AND rule_id = ?
	`, organizationID, ruleID)
	var traffic RuleTrafficRecord
	if err := row.Scan(&traffic.UploadBytes, &traffic.DownloadBytes, &traffic.TCPConnections, &traffic.UDPPackets); err != nil {
		return RuleTrafficRecord{}, err
	}
	return traffic, nil
}

func (store *PostgresStore) RecordNodeRuleTrafficAssignments(ctx context.Context, organizationID string, nodeID string, ruleIDs []string, now string) error {
	seen := make(map[string]struct{}, len(ruleIDs))
	for _, ruleID := range ruleIDs {
		ruleID = strings.TrimSpace(ruleID)
		if ruleID == "" {
			continue
		}
		if _, ok := seen[ruleID]; ok {
			continue
		}
		seen[ruleID] = struct{}{}
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO node_rule_traffic_assignments (organization_id, node_id, rule_id, first_seen_at, last_seen_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (organization_id, node_id, rule_id)
			DO UPDATE SET last_seen_at = EXCLUDED.last_seen_at
		`, organizationID, nodeID, ruleID, now, now); err != nil {
			return mapWriteError(err)
		}
	}
	return nil
}

func (store *PostgresStore) RecordRuleTrafficReport(ctx context.Context, organizationID string, agentID string, report RuleTrafficReportRecord, deltas []RuleTrafficDeltaRecord, now string, nextID func() string) (bool, error) {
	reportID := strings.TrimSpace(report.ReportID)
	if reportID == "" || len(deltas) == 0 {
		return false, nil
	}
	if report.ID == "" {
		report.ID = nextID()
	}
	if report.OrganizationID == "" {
		report.OrganizationID = organizationID
	}
	if report.AgentID == "" {
		report.AgentID = agentID
	}
	if report.CreatedAt == "" {
		report.CreatedAt = now
	}
	result, err := store.db.ExecContext(ctx, `
		INSERT INTO rule_traffic_reports (id, organization_id, agent_id, report_id, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (organization_id, agent_id, report_id) DO NOTHING
	`, report.ID, report.OrganizationID, report.AgentID, reportID, report.CreatedAt)
	if err != nil {
		return false, mapWriteError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, nil
	}
	for _, delta := range mergeRuleTrafficDeltas(deltas) {
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO rule_traffic_counters (
				id, organization_id, rule_id, period_start, period_granularity,
				upload_bytes, download_bytes, tcp_connections, udp_packets, updated_at
			)
			SELECT ?, ?, ?, '1970-01-01T00:00:00Z', 'ALL_TIME', ?, ?, ?, ?, ?
			WHERE EXISTS (
				SELECT 1
				FROM node_rule_traffic_assignments
				WHERE organization_id = ? AND node_id = ? AND rule_id = ?
			)
			ON CONFLICT (rule_id, period_start, period_granularity)
			DO UPDATE SET
				upload_bytes = rule_traffic_counters.upload_bytes + EXCLUDED.upload_bytes,
				download_bytes = rule_traffic_counters.download_bytes + EXCLUDED.download_bytes,
				tcp_connections = rule_traffic_counters.tcp_connections + EXCLUDED.tcp_connections,
				udp_packets = rule_traffic_counters.udp_packets + EXCLUDED.udp_packets,
				updated_at = EXCLUDED.updated_at
		`, nextID(), organizationID, delta.RuleID, delta.UploadBytes, delta.DownloadBytes, delta.TCPConnections, delta.UDPPackets, now, organizationID, agentID, delta.RuleID); err != nil {
			return false, mapWriteError(err)
		}
	}
	return true, nil
}

func (store *PostgresStore) ListRuleDeploymentsByOrganization(ctx context.Context, organizationID string) ([]RuleDeploymentRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, rule_id, node_id, config_version, rule_config_version, status,
		       error_code, error_message, protocol, listen_ip, port, updated_at
		FROM rule_deployment_statuses
		WHERE organization_id = ?
		ORDER BY rule_id, node_id
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	deployments := make([]RuleDeploymentRecord, 0)
	for rows.Next() {
		var deployment RuleDeploymentRecord
		if err := rows.Scan(
			&deployment.ID,
			&deployment.OrganizationID,
			&deployment.RuleID,
			&deployment.NodeID,
			&deployment.ConfigVersion,
			&deployment.RuleConfigVersion,
			&deployment.Status,
			&deployment.ErrorCode,
			&deployment.ErrorMessage,
			&deployment.Protocol,
			&deployment.ListenIP,
			&deployment.Port,
			&deployment.UpdatedAt,
		); err != nil {
			return nil, mapReadError(err)
		}
		deployments = append(deployments, deployment)
	}
	return deployments, rows.Err()
}

func (store *PostgresStore) ReplaceRuleDeploymentPending(ctx context.Context, organizationID string, rule RuleRecord, deployments []RuleDeploymentPendingRecord, now string, nextID func() string) error {
	if err := store.DeleteRuleDeployments(ctx, organizationID, rule.ID); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(deployments))
	for _, deployment := range deployments {
		nodeID := strings.TrimSpace(deployment.NodeID)
		if nodeID == "" {
			continue
		}
		if _, ok := seen[nodeID]; ok {
			continue
		}
		seen[nodeID] = struct{}{}
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO rule_deployment_statuses (
				id, organization_id, rule_id, node_id, config_version, rule_config_version, status,
				error_code, error_message, protocol, listen_ip, port, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, 'PENDING', '', '', '', '', 0, ?)
		`, nextID(), organizationID, rule.ID, nodeID, deployment.ConfigVersion, rule.ConfigVersion, now); err != nil {
			return mapWriteError(err)
		}
	}
	return nil
}

func (store *PostgresStore) UpsertRuleDeploymentPending(ctx context.Context, organizationID string, rule RuleRecord, deployment RuleDeploymentPendingRecord, now string, nextID func() string) error {
	nodeID := strings.TrimSpace(deployment.NodeID)
	if nodeID == "" {
		return nil
	}
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO rule_deployment_statuses (
			id, organization_id, rule_id, node_id, config_version, rule_config_version, status,
			error_code, error_message, protocol, listen_ip, port, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, 'PENDING', '', '', '', '', 0, ?)
		ON CONFLICT (organization_id, rule_id, node_id)
		DO UPDATE SET
			config_version = EXCLUDED.config_version,
			rule_config_version = EXCLUDED.rule_config_version,
			status = 'PENDING',
			error_code = '',
			error_message = '',
			protocol = '',
			listen_ip = '',
			port = 0,
			updated_at = EXCLUDED.updated_at
	`, nextID(), organizationID, rule.ID, nodeID, deployment.ConfigVersion, rule.ConfigVersion, now)
	return mapWriteError(err)
}

func (store *PostgresStore) RecordRuleDeploymentApplied(ctx context.Context, organizationID string, nodeID string, configVersion int, deployments []RuleDeploymentAppliedRecord, now string, nextID func() string) error {
	seen := make(map[string]struct{}, len(deployments))
	for _, deployment := range deployments {
		ruleID := strings.TrimSpace(deployment.RuleID)
		if ruleID == "" {
			continue
		}
		if _, ok := seen[ruleID]; ok {
			continue
		}
		seen[ruleID] = struct{}{}
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO rule_deployment_statuses (
				id, organization_id, rule_id, node_id, config_version, rule_config_version, status,
				error_code, error_message, protocol, listen_ip, port, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, 'APPLIED', '', '', '', '', 0, ?)
			ON CONFLICT (organization_id, rule_id, node_id)
			DO UPDATE SET
				config_version = EXCLUDED.config_version,
				rule_config_version = EXCLUDED.rule_config_version,
				status = 'APPLIED',
				error_code = '',
				error_message = '',
				protocol = '',
				listen_ip = '',
				port = 0,
				updated_at = EXCLUDED.updated_at
			WHERE rule_deployment_statuses.config_version <= EXCLUDED.config_version
		`, nextID(), organizationID, ruleID, nodeID, configVersion, deployment.RuleConfigVersion, now); err != nil {
			return mapWriteError(err)
		}
	}
	return nil
}

func (store *PostgresStore) RecordRuleDeploymentFailures(ctx context.Context, organizationID string, nodeID string, configVersion int, failures []RuleDeploymentFailureRecord, now string, nextID func() string) error {
	for _, failure := range failures {
		ruleID := strings.TrimSpace(failure.RuleID)
		if ruleID == "" {
			continue
		}
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO rule_deployment_statuses (
				id, organization_id, rule_id, node_id, config_version, rule_config_version, status,
				error_code, error_message, protocol, listen_ip, port, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, 'FAILED', ?, ?, ?, ?, ?, ?)
			ON CONFLICT (organization_id, rule_id, node_id)
			DO UPDATE SET
				config_version = EXCLUDED.config_version,
				rule_config_version = EXCLUDED.rule_config_version,
				status = 'FAILED',
				error_code = EXCLUDED.error_code,
				error_message = EXCLUDED.error_message,
				protocol = EXCLUDED.protocol,
				listen_ip = EXCLUDED.listen_ip,
				port = EXCLUDED.port,
				updated_at = EXCLUDED.updated_at
			WHERE rule_deployment_statuses.config_version = EXCLUDED.config_version
			  AND rule_deployment_statuses.rule_config_version = EXCLUDED.rule_config_version
		`, nextID(), organizationID, ruleID, nodeID, configVersion, failure.RuleConfigVersion,
			failure.ErrorCode, failure.ErrorMessage, failure.Protocol, failure.ListenIP, failure.Port, now); err != nil {
			return mapWriteError(err)
		}
	}
	return nil
}

func (store *PostgresStore) DeleteRuleDeploymentForNode(ctx context.Context, organizationID string, ruleID string, nodeID string) error {
	_, err := store.db.ExecContext(ctx, `
		DELETE FROM rule_deployment_statuses
		WHERE organization_id = ? AND rule_id = ? AND node_id = ?
	`, organizationID, ruleID, nodeID)
	return mapWriteError(err)
}

func (store *PostgresStore) DeleteRuleDeployments(ctx context.Context, organizationID string, ruleID string) error {
	_, err := store.db.ExecContext(ctx, `
		DELETE FROM rule_deployment_statuses
		WHERE organization_id = ? AND rule_id = ?
	`, organizationID, ruleID)
	return mapWriteError(err)
}

func mergeRuleTrafficDeltas(deltas []RuleTrafficDeltaRecord) []RuleTrafficDeltaRecord {
	byRule := make(map[string]RuleTrafficDeltaRecord)
	order := make([]string, 0, len(deltas))
	for _, delta := range deltas {
		ruleID := strings.TrimSpace(delta.RuleID)
		if ruleID == "" {
			continue
		}
		current, ok := byRule[ruleID]
		if !ok {
			order = append(order, ruleID)
			current.RuleID = ruleID
		}
		current.UploadBytes += positiveRuleTrafficDelta(delta.UploadBytes)
		current.DownloadBytes += positiveRuleTrafficDelta(delta.DownloadBytes)
		current.TCPConnections += positiveRuleTrafficDelta(delta.TCPConnections)
		current.UDPPackets += positiveRuleTrafficDelta(delta.UDPPackets)
		byRule[ruleID] = current
	}
	merged := make([]RuleTrafficDeltaRecord, 0, len(byRule))
	for _, ruleID := range order {
		delta := byRule[ruleID]
		if delta.UploadBytes == 0 && delta.DownloadBytes == 0 && delta.TCPConnections == 0 && delta.UDPPackets == 0 {
			continue
		}
		merged = append(merged, delta)
	}
	return merged
}

func positiveRuleTrafficDelta(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func (store *PostgresStore) loadTargetGroupMembers(ctx context.Context, group *TargetGroupRecord) error {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, target_group_id, target_id, priority, enabled, created_at, updated_at
		FROM target_group_members
		WHERE organization_id = ? AND target_group_id = ?
		ORDER BY priority, target_id
	`, group.OrganizationID, group.ID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	members := make([]TargetGroupMemberRecord, 0)
	for rows.Next() {
		member, err := scanTargetGroupMemberRows(rows)
		if err != nil {
			return err
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	group.Members = members
	return nil
}

func (store *PostgresStore) replaceTargetGroupMembers(ctx context.Context, organizationID string, targetGroupID string, members []TargetGroupMemberRecord, now string, nextID func() string) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM target_group_members WHERE organization_id = ? AND target_group_id = ?`, organizationID, targetGroupID); err != nil {
		return mapWriteError(err)
	}
	for _, member := range members {
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO target_group_members (id, organization_id, target_group_id, target_id, priority, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, nextID(), organizationID, targetGroupID, member.TargetID, member.Priority, boolToDB(member.Enabled), now, now); err != nil {
			return mapWriteError(err)
		}
	}
	return nil
}

func (store *PostgresStore) upsertInboundBinding(ctx context.Context, binding InboundBindingRecord) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(organization_id, node_group_id, listen_ip, protocol, port, match_type) DO UPDATE SET
			match_type = excluded.match_type
	`, binding.ID, binding.OrganizationID, binding.NodeGroupID, binding.ListenIP, binding.Protocol, binding.Port, binding.MatchType, binding.CreatedAt)
	return mapWriteError(err)
}

func (store *PostgresStore) loadRuleTags(ctx context.Context, rule *RuleRecord) error {
	rows, err := store.db.QueryContext(ctx, `
		SELECT tag
		FROM rule_tags
		WHERE organization_id = ? AND rule_id = ?
		ORDER BY tag
	`, rule.OrganizationID, rule.ID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	tags := make([]string, 0)
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return err
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rule.Tags = tags
	return nil
}

func (store *PostgresStore) replaceRuleTags(ctx context.Context, organizationID string, ruleID string, tags []string, now string, nextID func() string) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM rule_tags WHERE organization_id = ? AND rule_id = ?`, organizationID, ruleID); err != nil {
		return mapWriteError(err)
	}
	for _, tag := range tags {
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO rule_tags (id, organization_id, rule_id, tag, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, nextID(), organizationID, ruleID, tag, now); err != nil {
			return mapWriteError(err)
		}
	}
	return nil
}
