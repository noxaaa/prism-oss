package repo

import "context"

func (store *PostgresStore) ListNodeGroupsByOrganization(ctx context.Context, organizationID string) ([]NodeGroupRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, name, description, created_at, updated_at, coalesce(deleted_at::text, '')
		FROM node_groups
		WHERE organization_id = ? AND deleted_at IS NULL
		ORDER BY name, id
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
		SELECT id, organization_id, name, status, public_description, desired_config_version, applied_config_version,
		       config_status, config_error_message, coalesce(config_status_updated_at::text, ''),
		       coalesce(last_seen_at::text, ''), coalesce(registered_at::text, ''),
		       agent_version, agent_commit, agent_build_time, agent_auto_update_enabled, desired_agent_version,
		       agent_update_status, agent_update_error, coalesce(agent_update_started_at::text, ''), coalesce(agent_update_finished_at::text, ''),
		       created_at, updated_at, coalesce(deleted_at::text, '')
		FROM nodes
		WHERE organization_id = ? AND deleted_at IS NULL
		ORDER BY name, id
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
		if err := store.loadNodeDetails(ctx, &node); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func (store *PostgresStore) FindNodeByID(ctx context.Context, organizationID string, nodeID string) (NodeRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT id, organization_id, name, status, public_description, desired_config_version, applied_config_version,
		       config_status, config_error_message, coalesce(config_status_updated_at::text, ''),
		       coalesce(last_seen_at::text, ''), coalesce(registered_at::text, ''),
		       agent_version, agent_commit, agent_build_time, agent_auto_update_enabled, desired_agent_version,
		       agent_update_status, agent_update_error, coalesce(agent_update_started_at::text, ''), coalesce(agent_update_finished_at::text, ''),
		       created_at, updated_at, coalesce(deleted_at::text, '')
		FROM nodes
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
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
			config_status, config_error_message, config_status_updated_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, node.ID, node.OrganizationID, node.Name, node.Status, node.PublicDescription, node.DesiredConfigVersion, node.AppliedConfigVersion, node.ConfigStatus, node.ConfigErrorMessage, nullable(node.ConfigStatusUpdatedAt), node.CreatedAt, node.UpdatedAt)
	if err != nil {
		return mapWriteError(err)
	}
	if err := store.replaceNodeGroups(ctx, node.OrganizationID, node.ID, groupIDs, now, nextID); err != nil {
		return err
	}
	if err := store.replaceNodeListenIPs(ctx, node.OrganizationID, node.ID, listenIPs, now, nextID); err != nil {
		return err
	}
	return store.replaceNodePortRanges(ctx, node.OrganizationID, node.ID, portRanges, now, nextID)
}

func (store *PostgresStore) UpdateNode(ctx context.Context, node NodeRecord, replaceGroups bool, groupIDs []string, replaceListenIPs bool, listenIPs []NodeListenIPRecord, replacePortRanges bool, portRanges []NodePortRangeRecord, now string, nextID func() string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE nodes
		SET name = ?, public_description = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, node.Name, node.PublicDescription, node.UpdatedAt, node.OrganizationID, node.ID)
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

func (store *PostgresStore) RecordNodeConfigAck(ctx context.Context, organizationID string, nodeID string, configVersion int, status string, errorMessage string, now string) error {
	if status == "APPLIED" {
		_, err := store.db.ExecContext(ctx, `
		UPDATE nodes
		SET applied_config_version = ?,
		    config_status = CASE
		      WHEN desired_config_version <= ? THEN 'APPLIED'
		      ELSE 'PENDING'
		    END,
		    config_error_message = '',
		    config_status_updated_at = ?,
		    last_seen_at = ?,
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL AND applied_config_version <= ?
		  AND ? <= desired_config_version
	`, configVersion, configVersion, now, now, now, organizationID, nodeID, configVersion, configVersion)
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
		    config_status_updated_at = ?,
		    last_seen_at = ?,
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL AND applied_config_version <= ?
		  AND ? <= desired_config_version
	`, configVersion, configVersion, errorMessage, now, now, now, organizationID, nodeID, configVersion, configVersion)
	return mapWriteError(err)
}

func (store *PostgresStore) EnsureDesiredConfigVersionAtLeast(ctx context.Context, organizationID string, nodeID string, configVersion int, now string) error {
	_, err := store.db.ExecContext(ctx, `
		UPDATE nodes
		SET desired_config_version = ?,
		    config_status = 'PENDING',
		    config_error_message = '',
		    config_status_updated_at = ?,
		    updated_at = ?
		WHERE organization_id = ?
		  AND id = ?
		  AND deleted_at IS NULL
		  AND desired_config_version < ?
	`, configVersion, now, now, organizationID, nodeID, configVersion)
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
		groupIDs, err := store.listMonitorGroupIDs(ctx, monitor.OrganizationID, monitor.ID)
		if err != nil {
			return nil, err
		}
		monitor.GroupIDs = groupIDs
		monitors = append(monitors, monitor)
	}
	return monitors, rows.Err()
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
	portRanges, err := store.listNodePortRanges(ctx, node.OrganizationID, node.ID)
	if err != nil {
		return err
	}
	node.GroupIDs = groupIDs
	node.ListenIPs = listenIPs
	node.PortRanges = portRanges
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
		SELECT monitor_group_id
		FROM monitor_group_members
		WHERE organization_id = ? AND monitor_id = ?
		ORDER BY monitor_group_id
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
