package repo

import "context"

const nodeEnrollmentProfileSelect = `
	SELECT id, organization_id, name, description, token_hash, enabled,
	       coalesce(to_char(expires_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'), ''), max_uses, used_count, node_name_template,
	       group_ids_json::text, listen_ips_json::text, send_ips_json::text, port_ranges_json::text, max_rule_ports,
	       dns_publish_addresses_json::text,
	       dataplane_mode, dataplane_conflict_policy, auto_update_enabled,
	       allowed_cidrs_json::text, metadata_json::text, coalesce(created_by_user_id, ''),
	       created_at, updated_at,
	       coalesce(to_char(revoked_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'), ''),
	       coalesce(to_char(deleted_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'), '')
	FROM node_enrollment_profiles
`

func (store *PostgresStore) ListNodeEnrollmentProfiles(ctx context.Context, organizationID string) ([]NodeEnrollmentProfileRecord, error) {
	rows, err := store.db.QueryContext(ctx, nodeEnrollmentProfileSelect+`
		WHERE organization_id = ? AND deleted_at IS NULL
		ORDER BY name, id
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	profiles := make([]NodeEnrollmentProfileRecord, 0)
	for rows.Next() {
		profile, err := scanNodeEnrollmentProfileRows(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

func (store *PostgresStore) FindNodeEnrollmentProfileByID(ctx context.Context, organizationID string, profileID string) (NodeEnrollmentProfileRecord, error) {
	row := store.db.QueryRowContext(ctx, nodeEnrollmentProfileSelect+`
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, organizationID, profileID)
	return scanNodeEnrollmentProfile(row)
}

func (store *PostgresStore) FindNodeEnrollmentProfileByIDForUpdate(ctx context.Context, organizationID string, profileID string) (NodeEnrollmentProfileRecord, error) {
	row := store.db.QueryRowContext(ctx, nodeEnrollmentProfileSelect+`
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
		FOR UPDATE
	`, organizationID, profileID)
	return scanNodeEnrollmentProfile(row)
}

func (store *PostgresStore) FindNodeEnrollmentProfileByTokenHashForUpdate(ctx context.Context, tokenHash string) (NodeEnrollmentProfileRecord, error) {
	row := store.db.QueryRowContext(ctx, nodeEnrollmentProfileSelect+`
		WHERE token_hash = ? AND deleted_at IS NULL
		FOR UPDATE
	`, tokenHash)
	return scanNodeEnrollmentProfile(row)
}

func (store *PostgresStore) CreateNodeEnrollmentProfile(ctx context.Context, profile NodeEnrollmentProfileRecord) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO node_enrollment_profiles (
			id, organization_id, name, description, token_hash, enabled, expires_at, max_uses, used_count,
			node_name_template, group_ids_json, listen_ips_json, send_ips_json, port_ranges_json, max_rule_ports, dns_publish_addresses_json,
			dataplane_mode, dataplane_conflict_policy, auto_update_enabled, allowed_cidrs_json, metadata_json,
			created_by_user_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?::timestamptz, ?, ?, ?, ?::jsonb, ?::jsonb, ?::jsonb, ?::jsonb, ?, ?::jsonb, ?, ?, ?, ?::jsonb, ?::jsonb, ?, ?, ?)
	`, profile.ID, profile.OrganizationID, profile.Name, profile.Description, profile.TokenHash, profile.Enabled, nullable(profile.ExpiresAt), profile.MaxUses, profile.UsedCount, profile.NodeNameTemplate, profile.GroupIDsJSON, profile.ListenIPsJSON, profile.SendIPsJSON, profile.PortRangesJSON, defaultMaxRulePorts(profile.MaxRulePorts), profile.DNSPublishAddressesJSON, profile.DataplaneMode, profile.DataplaneConflictPolicy, profile.AutoUpdateEnabled, profile.AllowedCIDRsJSON, profile.MetadataJSON, nullable(profile.CreatedByUserID), profile.CreatedAt, profile.UpdatedAt)
	return mapWriteError(err)
}

func (store *PostgresStore) UpdateNodeEnrollmentProfile(ctx context.Context, profile NodeEnrollmentProfileRecord, replaceToken bool) error {
	query := `
		UPDATE node_enrollment_profiles
		SET name = ?, description = ?, enabled = ?, expires_at = ?::timestamptz, max_uses = ?,
		    node_name_template = ?, group_ids_json = ?::jsonb, listen_ips_json = ?::jsonb,
		    send_ips_json = ?::jsonb, port_ranges_json = ?::jsonb, max_rule_ports = ?, dns_publish_addresses_json = ?::jsonb,
		    dataplane_mode = ?, dataplane_conflict_policy = ?, auto_update_enabled = ?,
		    allowed_cidrs_json = ?::jsonb, metadata_json = ?::jsonb, revoked_at = ?::timestamptz, updated_at = ?
	`
	args := []any{profile.Name, profile.Description, profile.Enabled, nullable(profile.ExpiresAt), profile.MaxUses, profile.NodeNameTemplate, profile.GroupIDsJSON, profile.ListenIPsJSON, profile.SendIPsJSON, profile.PortRangesJSON, defaultMaxRulePorts(profile.MaxRulePorts), profile.DNSPublishAddressesJSON, profile.DataplaneMode, profile.DataplaneConflictPolicy, profile.AutoUpdateEnabled, profile.AllowedCIDRsJSON, profile.MetadataJSON, nullable(profile.RevokedAt), profile.UpdatedAt}
	if replaceToken {
		query += `, token_hash = ?, used_count = 0`
		args = append(args, profile.TokenHash)
	}
	query += ` WHERE organization_id = ? AND id = ? AND deleted_at IS NULL`
	args = append(args, profile.OrganizationID, profile.ID)
	result, err := store.db.ExecContext(ctx, query, args...)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) IncrementNodeEnrollmentProfileUsedCount(ctx context.Context, organizationID string, profileID string, now string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE node_enrollment_profiles
		SET used_count = used_count + 1, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, now, organizationID, profileID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) DecrementNodeEnrollmentProfileUsedCount(ctx context.Context, organizationID string, profileID string, now string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE node_enrollment_profiles
		SET used_count = GREATEST(used_count - 1, 0), updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, now, organizationID, profileID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) DeleteNodeEnrollmentProfile(ctx context.Context, organizationID string, profileID string, deletedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE node_enrollment_profiles
		SET deleted_at = ?, revoked_at = COALESCE(revoked_at, ?), enabled = false, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, deletedAt, deletedAt, deletedAt, organizationID, profileID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) ListNodeEnrollmentEvents(ctx context.Context, organizationID string, profileID string, limit int) ([]NodeEnrollmentEventRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, enrollment_profile_id, coalesce(node_id::text, ''), status, reason_code,
		       message, remote_ip, hostname, metadata_json::text, created_at
		FROM node_enrollment_events
		WHERE organization_id = ? AND enrollment_profile_id = ?
		ORDER BY created_at DESC, id DESC
		LIMIT ?
	`, organizationID, profileID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	events := make([]NodeEnrollmentEventRecord, 0)
	for rows.Next() {
		event, err := scanNodeEnrollmentEventRows(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (store *PostgresStore) CreateNodeEnrollmentEvent(ctx context.Context, event NodeEnrollmentEventRecord) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO node_enrollment_events (
			id, organization_id, enrollment_profile_id, node_id, status, reason_code, message, remote_ip, hostname, metadata_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?::jsonb, ?)
	`, event.ID, event.OrganizationID, event.EnrollmentProfileID, nullable(event.NodeID), event.Status, event.ReasonCode, event.Message, event.RemoteIP, event.Hostname, event.MetadataJSON, event.CreatedAt)
	return mapWriteError(err)
}
