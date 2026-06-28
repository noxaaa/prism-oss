package repo

import "context"

func (store *PostgresStore) ListNodeGroupsByOrganization(ctx context.Context, organizationID string) ([]NodeGroupRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, name, description, created_at, updated_at, coalesce(deleted_at::text, '')
		FROM node_groups
		WHERE organization_id = ? AND deleted_at IS NULL
		ORDER BY node_groups.name, node_groups.id
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	nodeGroups := make([]NodeGroupRecord, 0)
	for rows.Next() {
		nodeGroup, err := scanNodeGroupRows(rows)
		if err != nil {
			return nil, err
		}
		nodeGroups = append(nodeGroups, nodeGroup)
	}
	return nodeGroups, rows.Err()
}

func (store *PostgresStore) FindNodeGroupByID(ctx context.Context, organizationID string, nodeGroupID string) (NodeGroupRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT id, organization_id, name, description, created_at, updated_at, coalesce(deleted_at::text, '')
		FROM node_groups
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, organizationID, nodeGroupID)
	return scanNodeGroup(row)
}

func (store *PostgresStore) CreateNodeGroup(ctx context.Context, nodeGroup NodeGroupRecord) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO node_groups (id, organization_id, name, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, nodeGroup.ID, nodeGroup.OrganizationID, nodeGroup.Name, nodeGroup.Description, nodeGroup.CreatedAt, nodeGroup.UpdatedAt)
	return mapWriteError(err)
}

func (store *PostgresStore) UpdateNodeGroup(ctx context.Context, nodeGroup NodeGroupRecord) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE node_groups
		SET name = ?, description = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, nodeGroup.Name, nodeGroup.Description, nodeGroup.UpdatedAt, nodeGroup.OrganizationID, nodeGroup.ID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) DeleteNodeGroup(ctx context.Context, organizationID string, nodeGroupID string, deletedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE node_groups
		SET deleted_at = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, deletedAt, deletedAt, organizationID, nodeGroupID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) ListNodesByOrganization(ctx context.Context, organizationID string) ([]NodeRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT nodes.id, nodes.organization_id, nodes.name, nodes.status, nodes.public_description, nodes.desired_config_version, nodes.applied_config_version,
		       nodes.config_status, nodes.config_error_message, nodes.config_status_config_version, nodes.config_retry_count, coalesce(to_char(config_next_retry_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'), ''),
		       coalesce(nodes.config_status_updated_at::text, ''),
		       coalesce(nodes.last_seen_at::text, ''), coalesce(nodes.registered_at::text, ''),
		       nodes.agent_version, nodes.agent_commit, nodes.agent_build_time, nodes.agent_auto_update_enabled, nodes.desired_agent_version,
		       nodes.agent_update_status, nodes.agent_update_error, coalesce(nodes.agent_update_started_at::text, ''), coalesce(nodes.agent_update_finished_at::text, ''),
		       nodes.dataplane_mode, nodes.dataplane_conflict_policy, nodes.dataplane_instance_id, nodes.dataplane_status, nodes.dataplane_error, nodes.dataplane_last_hash,
		       coalesce(nodes.dataplane_last_applied_at::text, ''),
		       coalesce(nodes.enrollment_profile_id::text, ''), coalesce(node_enrollment_profiles.name, ''),
		       nodes.max_rule_ports,
		       nodes.created_at, nodes.updated_at, coalesce(nodes.deleted_at::text, '')
		FROM nodes
		LEFT JOIN node_enrollment_profiles
		  ON node_enrollment_profiles.organization_id = nodes.organization_id
		 AND node_enrollment_profiles.id = nodes.enrollment_profile_id
		WHERE nodes.organization_id = ? AND nodes.deleted_at IS NULL
		ORDER BY nodes.name, nodes.id
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	nodes := make([]NodeRecord, 0)
	for rows.Next() {
		node, err := scanNodeRows(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range nodes {
		if err := store.loadNodeDetails(ctx, &nodes[index]); err != nil {
			return nil, err
		}
	}
	return nodes, nil
}

func (store *PostgresStore) FindNodeByID(ctx context.Context, organizationID string, nodeID string) (NodeRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT nodes.id, nodes.organization_id, nodes.name, nodes.status, nodes.public_description, nodes.desired_config_version, nodes.applied_config_version,
		       nodes.config_status, nodes.config_error_message, nodes.config_status_config_version, nodes.config_retry_count, coalesce(to_char(config_next_retry_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'), ''),
		       coalesce(nodes.config_status_updated_at::text, ''),
		       coalesce(nodes.last_seen_at::text, ''), coalesce(nodes.registered_at::text, ''),
		       nodes.agent_version, nodes.agent_commit, nodes.agent_build_time, nodes.agent_auto_update_enabled, nodes.desired_agent_version,
		       nodes.agent_update_status, nodes.agent_update_error, coalesce(nodes.agent_update_started_at::text, ''), coalesce(nodes.agent_update_finished_at::text, ''),
		       nodes.dataplane_mode, nodes.dataplane_conflict_policy, nodes.dataplane_instance_id, nodes.dataplane_status, nodes.dataplane_error, nodes.dataplane_last_hash,
		       coalesce(nodes.dataplane_last_applied_at::text, ''),
		       coalesce(nodes.enrollment_profile_id::text, ''), coalesce(node_enrollment_profiles.name, ''),
		       nodes.max_rule_ports,
		       nodes.created_at, nodes.updated_at, coalesce(nodes.deleted_at::text, '')
		FROM nodes
		LEFT JOIN node_enrollment_profiles
		  ON node_enrollment_profiles.organization_id = nodes.organization_id
		 AND node_enrollment_profiles.id = nodes.enrollment_profile_id
		WHERE nodes.organization_id = ? AND nodes.id = ? AND nodes.deleted_at IS NULL
	`, organizationID, nodeID)
	node, err := scanNode(row)
	if err != nil {
		return NodeRecord{}, err
	}
	if err := store.loadNodeDetails(ctx, &node); err != nil {
		return NodeRecord{}, err
	}
	return node, nil
}

func (store *PostgresStore) CreateNode(ctx context.Context, node NodeRecord, groupIDs []string, listenIPs []NodeListenIPRecord, portRanges []NodePortRangeRecord, now string, nextID func() string) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO nodes (
			id, organization_id, name, status, public_description, desired_config_version, applied_config_version,
			config_status, config_error_message, config_status_updated_at, agent_auto_update_enabled, max_rule_ports,
			dataplane_mode, dataplane_conflict_policy, enrollment_profile_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, node.ID, node.OrganizationID, node.Name, node.Status, node.PublicDescription, node.DesiredConfigVersion, node.AppliedConfigVersion, node.ConfigStatus, node.ConfigErrorMessage, nullable(node.ConfigStatusUpdatedAt), node.AgentAutoUpdateEnabled, defaultMaxRulePorts(node.MaxRulePorts), normalizeNodeDataplaneMode(node.DataplaneMode), normalizeNodeDataplaneConflictPolicy(node.DataplaneConflictPolicy), nullable(node.EnrollmentProfileID), node.CreatedAt, node.UpdatedAt)
	if err != nil {
		return mapWriteError(err)
	}
	if err := store.replaceNodeGroups(ctx, node.OrganizationID, node.ID, groupIDs, now, nextID); err != nil {
		return err
	}
	if err := store.replaceNodeListenIPs(ctx, node.OrganizationID, node.ID, listenIPs, now, nextID); err != nil {
		return err
	}
	if err := store.replaceNodeSendIPs(ctx, node.OrganizationID, node.ID, node.SendIPs, now, nextID); err != nil {
		return err
	}
	return store.replaceNodePortRanges(ctx, node.OrganizationID, node.ID, portRanges, now, nextID)
}

func (store *PostgresStore) UpdateNode(ctx context.Context, node NodeRecord, replaceGroups bool, groupIDs []string, replaceListenIPs bool, listenIPs []NodeListenIPRecord, replacePortRanges bool, portRanges []NodePortRangeRecord, now string, nextID func() string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE nodes
		SET name = ?, public_description = ?, dataplane_mode = ?, dataplane_conflict_policy = ?, max_rule_ports = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, node.Name, node.PublicDescription, normalizeNodeDataplaneMode(node.DataplaneMode), normalizeNodeDataplaneConflictPolicy(node.DataplaneConflictPolicy), defaultMaxRulePorts(node.MaxRulePorts), node.UpdatedAt, node.OrganizationID, node.ID)
	if err != nil {
		return mapWriteError(err)
	}
	if err := requireAffected(result); err != nil {
		return err
	}
	if replaceGroups {
		if err := store.replaceNodeGroups(ctx, node.OrganizationID, node.ID, groupIDs, now, nextID); err != nil {
			return err
		}
	}
	if replaceListenIPs {
		if err := store.replaceNodeListenIPs(ctx, node.OrganizationID, node.ID, listenIPs, now, nextID); err != nil {
			return err
		}
	}
	if err := store.replaceNodeSendIPs(ctx, node.OrganizationID, node.ID, node.SendIPs, now, nextID); err != nil {
		return err
	}
	if replacePortRanges {
		if err := store.replaceNodePortRanges(ctx, node.OrganizationID, node.ID, portRanges, now, nextID); err != nil {
			return err
		}
	}
	return nil
}

func (store *PostgresStore) MarkNodeAgentConnected(ctx context.Context, organizationID string, nodeID string, now string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE nodes
		SET status = 'ONLINE',
		    last_seen_at = ?,
		    registered_at = COALESCE(registered_at, ?),
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, now, now, now, organizationID, nodeID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) UpdateNodeAgentVersion(ctx context.Context, organizationID string, nodeID string, version NodeAgentVersionRecord, now string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE nodes
		SET agent_version = ?,
		    agent_commit = ?,
		    agent_build_time = ?,
		    last_seen_at = ?,
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, version.Version, version.Commit, version.BuildTime, now, now, organizationID, nodeID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) UpdateNodeAgentUpdatePolicy(ctx context.Context, organizationID string, nodeID string, enabled bool, now string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE nodes
		SET agent_auto_update_enabled = ?,
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, enabled, now, organizationID, nodeID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) MarkNodeAgentUpdateRequested(ctx context.Context, organizationID string, nodeID string, targetVersion string, now string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE nodes
		SET desired_agent_version = ?,
		    agent_update_status = 'PENDING',
		    agent_update_error = '',
		    agent_update_started_at = ?,
		    agent_update_finished_at = NULL,
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, targetVersion, now, now, organizationID, nodeID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) MarkNodeAgentUpdateSatisfied(ctx context.Context, organizationID string, nodeID string, targetVersion string, now string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE nodes
		SET desired_agent_version = ?,
		    agent_update_status = 'SUCCEEDED',
		    agent_update_error = '',
		    agent_update_started_at = COALESCE(agent_update_started_at, ?),
		    agent_update_finished_at = ?,
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, targetVersion, now, now, now, organizationID, nodeID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) RecordNodeAgentUpdateResult(ctx context.Context, organizationID string, nodeID string, status string, errorMessage string, now string) error {
	finishedAt := any(now)
	if status == "RUNNING" {
		finishedAt = nil
	}
	result, err := store.db.ExecContext(ctx, `
		UPDATE nodes
		SET agent_update_status = ?,
		    agent_update_error = ?,
		    agent_update_finished_at = ?,
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, status, errorMessage, finishedAt, now, organizationID, nodeID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) MarkNodeAgentDisconnected(ctx context.Context, organizationID string, nodeID string, now string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE nodes
		SET status = 'OFFLINE',
		    last_seen_at = ?,
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, now, now, organizationID, nodeID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) RecordNodeConfigAck(ctx context.Context, organizationID string, nodeID string, ack NodeConfigAckRecord, now string) error {
	if ack.Status == "APPLIED" {
		_, err := store.db.ExecContext(ctx, `
		UPDATE nodes
		SET applied_config_version = ?,
		    config_status = CASE
		      WHEN desired_config_version <= ? THEN 'APPLIED'
		      ELSE 'PENDING'
		    END,
		    config_error_message = '',
		    config_status_config_version = ?,
		    config_retry_count = 0,
		    config_next_retry_at = NULL,
		    config_status_updated_at = ?,
		    dataplane_status = CASE
		      WHEN desired_config_version <= ? THEN ?
		      ELSE dataplane_status
		    END,
		    dataplane_error = CASE
		      WHEN desired_config_version <= ? THEN ''
		      ELSE dataplane_error
		    END,
		    dataplane_last_hash = CASE
		      WHEN desired_config_version <= ? THEN ?
		      ELSE dataplane_last_hash
		    END,
		    dataplane_last_applied_at = CASE
		      WHEN desired_config_version <= ? THEN ?
		      ELSE dataplane_last_applied_at
		    END,
		    last_seen_at = ?,
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL AND applied_config_version <= ?
		  AND ? <= desired_config_version
	`, ack.ConfigVersion, ack.ConfigVersion, ack.ConfigVersion, now, ack.ConfigVersion, ack.DataplaneStatus, ack.ConfigVersion, ack.ConfigVersion, ack.DataplaneLastHash, ack.ConfigVersion, now, now, now, organizationID, nodeID, ack.ConfigVersion, ack.ConfigVersion)
		if err != nil {
			return mapWriteError(err)
		}
		return nil
	}
	_, err := store.db.ExecContext(ctx, `
		UPDATE nodes
		SET config_status = CASE
		      WHEN desired_config_version <= ? THEN 'FAILED'
		      ELSE 'PENDING'
		    END,
		    config_error_message = CASE
		      WHEN desired_config_version <= ? THEN ?
		      ELSE ''
		    END,
		    config_status_config_version = ?,
		    config_retry_count = ?,
		    config_next_retry_at = ?,
		    config_status_updated_at = ?,
		    dataplane_status = CASE
		      WHEN desired_config_version <= ? THEN ?
		      ELSE dataplane_status
		    END,
		    dataplane_error = CASE
		      WHEN desired_config_version <= ? THEN ?
		      ELSE dataplane_error
		    END,
		    last_seen_at = ?,
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL AND applied_config_version <= ?
		  AND ? <= desired_config_version
	`, ack.ConfigVersion, ack.ConfigVersion, ack.ErrorMessage, ack.ConfigVersion, ack.RetryCount, nullable(ack.NextRetryAt), now, ack.ConfigVersion, ack.DataplaneStatus, ack.ConfigVersion, ack.DataplaneError, now, now, organizationID, nodeID, ack.ConfigVersion, ack.ConfigVersion)
	return mapWriteError(err)
}

func (store *PostgresStore) EnsureDesiredConfigVersionAtLeast(ctx context.Context, organizationID string, nodeID string, configVersion int, now string) error {
	_, err := store.db.ExecContext(ctx, `
		UPDATE nodes
		SET desired_config_version = ?,
		    config_status = 'PENDING',
		    config_error_message = '',
		    config_status_config_version = ?,
		    config_retry_count = 0,
		    config_next_retry_at = NULL,
		    config_status_updated_at = ?,
		    updated_at = ?
		WHERE organization_id = ?
		  AND id = ?
		  AND deleted_at IS NULL
		  AND desired_config_version < ?
	`, configVersion, configVersion, now, now, organizationID, nodeID, configVersion)
	return mapWriteError(err)
}

func (store *PostgresStore) IncrementDesiredConfigForNode(ctx context.Context, organizationID string, nodeID string, now string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE nodes
		SET desired_config_version =
		      CASE
		        WHEN desired_config_version >= applied_config_version THEN desired_config_version + 1
		        ELSE applied_config_version + 1
		      END,
		    config_status = 'PENDING',
		    config_error_message = '',
		    config_status_config_version =
		      CASE
		        WHEN desired_config_version >= applied_config_version THEN desired_config_version + 1
		        ELSE applied_config_version + 1
		      END,
		    config_retry_count = 0,
		    config_next_retry_at = NULL,
		    config_status_updated_at = ?,
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, now, now, organizationID, nodeID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) IncrementDesiredConfigForNodeGroup(ctx context.Context, organizationID string, nodeGroupID string, now string) error {
	_, err := store.db.ExecContext(ctx, `
		UPDATE nodes
		SET desired_config_version =
		      CASE
		        WHEN desired_config_version >= applied_config_version THEN desired_config_version + 1
		        ELSE applied_config_version + 1
		      END,
		    config_status = 'PENDING',
		    config_error_message = '',
		    config_status_config_version =
		      CASE
		        WHEN desired_config_version >= applied_config_version THEN desired_config_version + 1
		        ELSE applied_config_version + 1
		      END,
		    config_retry_count = 0,
		    config_next_retry_at = NULL,
		    config_status_updated_at = ?,
		    updated_at = ?
		WHERE organization_id = ?
		  AND deleted_at IS NULL
		  AND id IN (
		    SELECT node_id
		    FROM node_group_members
		    WHERE organization_id = ? AND node_group_id = ?
		  )
	`, now, now, organizationID, organizationID, nodeGroupID)
	return mapWriteError(err)
}

func (store *PostgresStore) DeleteNode(ctx context.Context, organizationID string, nodeID string, deletedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE nodes
		SET deleted_at = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, deletedAt, deletedAt, organizationID, nodeID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) ListMonitorGroupsByOrganization(ctx context.Context, organizationID string) ([]MonitorGroupRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, name, description, created_at, updated_at, coalesce(deleted_at::text, '')
		FROM monitor_groups
		WHERE organization_id = ? AND deleted_at IS NULL
		ORDER BY name, id
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	monitorGroups := make([]MonitorGroupRecord, 0)
	for rows.Next() {
		monitorGroup, err := scanMonitorGroupRows(rows)
		if err != nil {
			return nil, err
		}
		monitorGroups = append(monitorGroups, monitorGroup)
	}
	return monitorGroups, rows.Err()
}

func (store *PostgresStore) FindMonitorGroupByID(ctx context.Context, organizationID string, monitorGroupID string) (MonitorGroupRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT id, organization_id, name, description, created_at, updated_at, coalesce(deleted_at::text, '')
		FROM monitor_groups
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, organizationID, monitorGroupID)
	return scanMonitorGroup(row)
}

func (store *PostgresStore) CreateMonitorGroup(ctx context.Context, monitorGroup MonitorGroupRecord) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO monitor_groups (id, organization_id, name, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, monitorGroup.ID, monitorGroup.OrganizationID, monitorGroup.Name, monitorGroup.Description, monitorGroup.CreatedAt, monitorGroup.UpdatedAt)
	return mapWriteError(err)
}

func (store *PostgresStore) UpdateMonitorGroup(ctx context.Context, monitorGroup MonitorGroupRecord) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE monitor_groups
		SET name = ?, description = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, monitorGroup.Name, monitorGroup.Description, monitorGroup.UpdatedAt, monitorGroup.OrganizationID, monitorGroup.ID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) DeleteMonitorGroup(ctx context.Context, organizationID string, monitorGroupID string, deletedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE monitor_groups
		SET deleted_at = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, deletedAt, deletedAt, organizationID, monitorGroupID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) ListMonitorsByOrganization(ctx context.Context, organizationID string) ([]MonitorRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, name, status, desired_config_version, applied_config_version,
		       coalesce(last_seen_at::text, ''), coalesce(registered_at::text, ''), created_at, updated_at, coalesce(deleted_at::text, '')
		FROM monitors
		WHERE organization_id = ? AND deleted_at IS NULL
		ORDER BY name, id
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	monitors := make([]MonitorRecord, 0)
	for rows.Next() {
		monitor, err := scanMonitorRows(rows)
		if err != nil {
			return nil, err
		}
		monitors = append(monitors, monitor)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range monitors {
		groupIDs, err := store.listMonitorGroupIDs(ctx, monitors[index].OrganizationID, monitors[index].ID)
		if err != nil {
			return nil, err
		}
		monitors[index].GroupIDs = groupIDs
	}
	return monitors, nil
}

func (store *PostgresStore) FindMonitorByID(ctx context.Context, organizationID string, monitorID string) (MonitorRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT id, organization_id, name, status, desired_config_version, applied_config_version,
		       coalesce(last_seen_at::text, ''), coalesce(registered_at::text, ''), created_at, updated_at, coalesce(deleted_at::text, '')
		FROM monitors
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, organizationID, monitorID)
	monitor, err := scanMonitor(row)
	if err != nil {
		return MonitorRecord{}, err
	}
	groupIDs, err := store.listMonitorGroupIDs(ctx, monitor.OrganizationID, monitor.ID)
	if err != nil {
		return MonitorRecord{}, err
	}
	monitor.GroupIDs = groupIDs
	return monitor, nil
}

func (store *PostgresStore) CreateMonitor(ctx context.Context, monitor MonitorRecord, groupIDs []string, now string, nextID func() string) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO monitors (
			id, organization_id, name, status, desired_config_version, applied_config_version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, monitor.ID, monitor.OrganizationID, monitor.Name, monitor.Status, monitor.DesiredConfigVersion, monitor.AppliedConfigVersion, monitor.CreatedAt, monitor.UpdatedAt)
	if err != nil {
		return mapWriteError(err)
	}
	return store.replaceMonitorGroups(ctx, monitor.OrganizationID, monitor.ID, groupIDs, now, nextID)
}

func (store *PostgresStore) UpdateMonitor(ctx context.Context, monitor MonitorRecord, replaceGroups bool, groupIDs []string, now string, nextID func() string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE monitors
		SET name = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, monitor.Name, monitor.UpdatedAt, monitor.OrganizationID, monitor.ID)
	if err != nil {
		return mapWriteError(err)
	}
	if err := requireAffected(result); err != nil {
		return err
	}
	if replaceGroups {
		return store.replaceMonitorGroups(ctx, monitor.OrganizationID, monitor.ID, groupIDs, now, nextID)
	}
	return nil
}

func (store *PostgresStore) MarkMonitorAgentConnected(ctx context.Context, organizationID string, monitorID string, now string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE monitors
		SET status = 'ONLINE',
		    last_seen_at = ?,
		    registered_at = COALESCE(registered_at, ?),
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, now, now, now, organizationID, monitorID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) MarkMonitorAgentDisconnected(ctx context.Context, organizationID string, monitorID string, now string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE monitors
		SET status = 'OFFLINE',
		    last_seen_at = ?,
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, now, now, organizationID, monitorID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) RecordMonitorConfigAck(ctx context.Context, organizationID string, monitorID string, configVersion int, now string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE monitors
		SET applied_config_version = ?,
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, configVersion, now, organizationID, monitorID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) DeleteMonitor(ctx context.Context, organizationID string, monitorID string, deletedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE monitors
		SET deleted_at = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, deletedAt, deletedAt, organizationID, monitorID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) loadNodeDetails(ctx context.Context, node *NodeRecord) error {
	groupIDs, err := store.listNodeGroupIDs(ctx, node.OrganizationID, node.ID)
	if err != nil {
		return err
	}
	listenIPs, err := store.listNodeListenIPs(ctx, node.OrganizationID, node.ID)
	if err != nil {
		return err
	}
	sendIPs, err := store.listNodeSendIPs(ctx, node.OrganizationID, node.ID)
	if err != nil {
		return err
	}
	portRanges, err := store.listNodePortRanges(ctx, node.OrganizationID, node.ID)
	if err != nil {
		return err
	}
	dnsPublishAddresses, err := store.ListNodeDNSPublishAddresses(ctx, node.OrganizationID, node.ID)
	if err != nil {
		return err
	}
	node.GroupIDs = groupIDs
	node.ListenIPs = listenIPs
	node.SendIPs = sendIPs
	node.PortRanges = portRanges
	node.DNSPublishAddresses = dnsPublishAddresses
	return nil
}

func (store *PostgresStore) listNodeGroupIDs(ctx context.Context, organizationID string, nodeID string) ([]string, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT node_group_members.node_group_id
		FROM node_group_members
		JOIN node_groups
		  ON node_groups.organization_id = node_group_members.organization_id
		 AND node_groups.id = node_group_members.node_group_id
		 AND node_groups.deleted_at IS NULL
		WHERE node_group_members.organization_id = ? AND node_group_members.node_id = ?
		ORDER BY node_group_members.node_group_id
	`, organizationID, nodeID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	groupIDs := make([]string, 0)
	for rows.Next() {
		var groupID string
		if err := rows.Scan(&groupID); err != nil {
			return nil, err
		}
		groupIDs = append(groupIDs, groupID)
	}
	return groupIDs, rows.Err()
}

func (store *PostgresStore) listNodeListenIPs(ctx context.Context, organizationID string, nodeID string) ([]NodeListenIPRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, node_id, listen_ip, display_name, enabled, created_at, updated_at
		FROM node_listen_ips
		WHERE organization_id = ? AND node_id = ?
		ORDER BY listen_ip, id
	`, organizationID, nodeID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	listenIPs := make([]NodeListenIPRecord, 0)
	for rows.Next() {
		listenIP, err := scanNodeListenIPRows(rows)
		if err != nil {
			return nil, err
		}
		listenIPs = append(listenIPs, listenIP)
	}
	return listenIPs, rows.Err()
}

func (store *PostgresStore) listNodeSendIPs(ctx context.Context, organizationID string, nodeID string) ([]NodeSendIPRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, node_id, send_ip, display_name, enabled, created_at, updated_at
		FROM node_send_ips
		WHERE organization_id = ? AND node_id = ?
		ORDER BY send_ip, id
	`, organizationID, nodeID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	sendIPs := make([]NodeSendIPRecord, 0)
	for rows.Next() {
		sendIP, err := scanNodeSendIPRows(rows)
		if err != nil {
			return nil, err
		}
		sendIPs = append(sendIPs, sendIP)
	}
	return sendIPs, rows.Err()
}

func (store *PostgresStore) listNodePortRanges(ctx context.Context, organizationID string, nodeID string) ([]NodePortRangeRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, node_id, protocol, start_port, end_port, enabled, created_at, updated_at
		FROM node_port_ranges
		WHERE organization_id = ? AND node_id = ?
		ORDER BY protocol, start_port, end_port, id
	`, organizationID, nodeID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	portRanges := make([]NodePortRangeRecord, 0)
	for rows.Next() {
		portRange, err := scanNodePortRangeRows(rows)
		if err != nil {
			return nil, err
		}
		portRanges = append(portRanges, portRange)
	}
	return portRanges, rows.Err()
}

func (store *PostgresStore) ListNodeDNSPublishAddresses(ctx context.Context, organizationID string, nodeID string) ([]NodeDNSPublishAddressRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, node_id, address_type, address, source, enabled, COALESCE(observed_at::text, ''), created_at, updated_at
		FROM dns_publish_addresses
		WHERE organization_id = ? AND node_id = ?
		ORDER BY source, address_type, address, id
	`, organizationID, nodeID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	addresses := make([]NodeDNSPublishAddressRecord, 0)
	for rows.Next() {
		address, err := scanNodeDNSPublishAddressRows(rows)
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, address)
	}
	return addresses, rows.Err()
}

func (store *PostgresStore) ReplaceManualNodeDNSPublishAddresses(ctx context.Context, organizationID string, nodeID string, addresses []NodeDNSPublishAddressRecord, now string, nextID func() string) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM dns_publish_addresses WHERE organization_id = ? AND node_id = ? AND source = 'MANUAL'`, organizationID, nodeID); err != nil {
		return mapWriteError(err)
	}
	for _, address := range addresses {
		id := address.ID
		if id == "" {
			id = nextID()
		}
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO dns_publish_addresses (id, organization_id, node_id, address_type, address, source, enabled, observed_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 'MANUAL', ?, NULLIF(?, '')::timestamptz, ?, ?)
		`, id, organizationID, nodeID, address.AddressType, address.Address, boolToDB(address.Enabled), address.ObservedAt, now, now); err != nil {
			return mapWriteError(err)
		}
	}
	return nil
}

func (store *PostgresStore) UpsertAutoNodeDNSPublishAddress(ctx context.Context, organizationID string, nodeID string, addressType string, address string, now string, nextID func() string) error {
	if _, err := store.db.ExecContext(ctx, `
		UPDATE dns_publish_addresses
		SET enabled = false, updated_at = ?
		WHERE organization_id = ?
		  AND node_id = ?
		  AND source = 'AUTO'
		  AND address_type = ?
		  AND address <> ?
		  AND enabled = true
	`, now, organizationID, nodeID, addressType, address); err != nil {
		return mapWriteError(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO dns_publish_addresses (id, organization_id, node_id, address_type, address, source, enabled, observed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'AUTO', true, ?, ?, ?)
		ON CONFLICT (organization_id, node_id, source, address_type, address)
		DO UPDATE SET enabled = true, observed_at = EXCLUDED.observed_at, updated_at = EXCLUDED.updated_at
	`, nextID(), organizationID, nodeID, addressType, address, now, now, now); err != nil {
		return mapWriteError(err)
	}
	return nil
}

func (store *PostgresStore) DisableAutoNodeDNSPublishAddresses(ctx context.Context, organizationID string, nodeID string, now string) error {
	if _, err := store.db.ExecContext(ctx, `
		UPDATE dns_publish_addresses
		SET enabled = false, updated_at = ?
		WHERE organization_id = ?
		  AND node_id = ?
		  AND source = 'AUTO'
		  AND enabled = true
	`, now, organizationID, nodeID); err != nil {
		return mapWriteError(err)
	}
	return nil
}

func defaultMaxRulePorts(value int) int {
	if value <= 0 {
		return 256
	}
	return value
}

func (store *PostgresStore) replaceNodeGroups(ctx context.Context, organizationID string, nodeID string, groupIDs []string, now string, nextID func() string) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM node_group_members WHERE organization_id = ? AND node_id = ?`, organizationID, nodeID); err != nil {
		return mapWriteError(err)
	}
	for _, groupID := range groupIDs {
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO node_group_members (id, organization_id, node_id, node_group_id, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, nextID(), organizationID, nodeID, groupID, now); err != nil {
			return mapWriteError(err)
		}
	}
	return nil
}

func (store *PostgresStore) replaceNodeListenIPs(ctx context.Context, organizationID string, nodeID string, listenIPs []NodeListenIPRecord, now string, nextID func() string) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM node_listen_ips WHERE organization_id = ? AND node_id = ?`, organizationID, nodeID); err != nil {
		return mapWriteError(err)
	}
	for _, listenIP := range listenIPs {
		id := listenIP.ID
		if id == "" {
			id = nextID()
		}
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO node_listen_ips (id, organization_id, node_id, listen_ip, display_name, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, id, organizationID, nodeID, listenIP.ListenIP, listenIP.DisplayName, boolToDB(listenIP.Enabled), now, now); err != nil {
			return mapWriteError(err)
		}
	}
	return nil
}

func (store *PostgresStore) replaceNodeSendIPs(ctx context.Context, organizationID string, nodeID string, sendIPs []NodeSendIPRecord, now string, nextID func() string) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM node_send_ips WHERE organization_id = ? AND node_id = ?`, organizationID, nodeID); err != nil {
		return mapWriteError(err)
	}
	for _, sendIP := range sendIPs {
		id := sendIP.ID
		if id == "" {
			id = nextID()
		}
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO node_send_ips (id, organization_id, node_id, send_ip, display_name, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, id, organizationID, nodeID, sendIP.SendIP, sendIP.DisplayName, boolToDB(sendIP.Enabled), now, now); err != nil {
			return mapWriteError(err)
		}
	}
	return nil
}

func (store *PostgresStore) replaceNodePortRanges(ctx context.Context, organizationID string, nodeID string, portRanges []NodePortRangeRecord, now string, nextID func() string) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM node_port_ranges WHERE organization_id = ? AND node_id = ?`, organizationID, nodeID); err != nil {
		return mapWriteError(err)
	}
	for _, portRange := range portRanges {
		id := portRange.ID
		if id == "" {
			id = nextID()
		}
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO node_port_ranges (id, organization_id, node_id, protocol, start_port, end_port, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, id, organizationID, nodeID, portRange.Protocol, portRange.StartPort, portRange.EndPort, boolToDB(portRange.Enabled), now, now); err != nil {
			return mapWriteError(err)
		}
	}
	return nil
}

func (store *PostgresStore) listMonitorGroupIDs(ctx context.Context, organizationID string, monitorID string) ([]string, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT monitor_group_members.monitor_group_id
		FROM monitor_group_members
		JOIN monitor_groups
		  ON monitor_groups.organization_id = monitor_group_members.organization_id
		 AND monitor_groups.id = monitor_group_members.monitor_group_id
		 AND monitor_groups.deleted_at IS NULL
		WHERE monitor_group_members.organization_id = ? AND monitor_group_members.monitor_id = ?
		ORDER BY monitor_group_members.monitor_group_id
	`, organizationID, monitorID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	groupIDs := make([]string, 0)
	for rows.Next() {
		var groupID string
		if err := rows.Scan(&groupID); err != nil {
			return nil, err
		}
		groupIDs = append(groupIDs, groupID)
	}
	return groupIDs, rows.Err()
}

func (store *PostgresStore) replaceMonitorGroups(ctx context.Context, organizationID string, monitorID string, groupIDs []string, now string, nextID func() string) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM monitor_group_members WHERE organization_id = ? AND monitor_id = ?`, organizationID, monitorID); err != nil {
		return mapWriteError(err)
	}
	for _, groupID := range groupIDs {
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO monitor_group_members (id, organization_id, monitor_id, monitor_group_id, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, nextID(), organizationID, monitorID, groupID, now); err != nil {
			return mapWriteError(err)
		}
	}
	return nil
}
