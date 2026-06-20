package repo

import "context"

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
		if err := store.loadTargetGroupMembers(ctx, &group); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
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
		       config_version, forwarding_rules.created_at, forwarding_rules.updated_at, coalesce(forwarding_rules.deleted_at::text, ''),
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
		if err := store.loadRuleTags(ctx, &rule); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (store *PostgresStore) FindRuleByID(ctx context.Context, organizationID string, ruleID string) (RuleRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT forwarding_rules.id, forwarding_rules.organization_id, owner_user_id, name, enabled, status,
		       forwarding_type, forwarding_rules.protocol, forwarding_rules.match_type, inbound_binding_id, coalesce(sni_hostname, ''),
		       target_type, coalesce(target_id::text, ''), coalesce(target_group_id::text, ''), proxy_protocol_in, proxy_protocol_out,
		       config_version, forwarding_rules.created_at, forwarding_rules.updated_at, coalesce(forwarding_rules.deleted_at::text, ''),
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
			proxy_protocol_in, proxy_protocol_out, config_version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, nullif(?, '')::uuid, nullif(?, '')::uuid, ?, ?, ?, ?, ?)
	`, rule.ID, rule.OrganizationID, rule.OwnerUserID, rule.Name, boolToDB(rule.Enabled), rule.Status, rule.ForwardingType, rule.Protocol, rule.MatchType,
		binding.ID, nullIfEmpty(rule.SNIHostname), rule.TargetType, rule.TargetID, rule.TargetGroupID,
		rule.ProxyProtocolIn, rule.ProxyProtocolOut, rule.ConfigVersion, rule.CreatedAt, rule.UpdatedAt)
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
		    proxy_protocol_in = ?, proxy_protocol_out = ?, config_version = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, rule.Name, boolToDB(rule.Enabled), rule.Status, rule.ForwardingType, rule.Protocol, rule.MatchType, binding.ID,
		nullIfEmpty(rule.SNIHostname), rule.TargetType, rule.TargetID, rule.TargetGroupID,
		rule.ProxyProtocolIn, rule.ProxyProtocolOut, rule.ConfigVersion, rule.UpdatedAt, rule.OrganizationID, rule.ID)
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
