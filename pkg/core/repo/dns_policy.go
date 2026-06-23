package repo

import (
	"context"
	"database/sql"
)

func (store *PostgresStore) ListDNSManagedRecordsByOrganization(ctx context.Context, organizationID string) ([]DNSManagedRecordRecord, error) {
	rows, err := store.db.QueryContext(ctx, dnsManagedRecordSelect()+`
		WHERE organization_id = ? AND deleted_at IS NULL
		ORDER BY zone_name, record_name, record_type
	`, organizationID)
	if err != nil {
		return nil, err
	}
	records := make([]DNSManagedRecordRecord, 0)
	for rows.Next() {
		record, err := scanDNSManagedRecordRows(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range records {
		instances, err := store.ListDNSInstancesByManagedRecord(ctx, organizationID, records[index].ID)
		if err != nil {
			return nil, err
		}
		records[index].Instances = instances
	}
	return records, nil
}

func (store *PostgresStore) FindDNSManagedRecordByID(ctx context.Context, organizationID string, recordID string) (DNSManagedRecordRecord, error) {
	row := store.db.QueryRowContext(ctx, dnsManagedRecordSelect()+`
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, organizationID, recordID)
	record, err := scanDNSManagedRecord(row)
	if err != nil {
		return DNSManagedRecordRecord{}, err
	}
	instances, err := store.ListDNSInstancesByManagedRecord(ctx, organizationID, record.ID)
	if err != nil {
		return DNSManagedRecordRecord{}, err
	}
	record.Instances = instances
	return record, nil
}

func (store *PostgresStore) LockDNSManagedRecordEvaluation(ctx context.Context, organizationID string, recordID string) error {
	_, err := store.db.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext(?), hashtext(?))`, organizationID, recordID)
	return err
}

func (store *PostgresStore) LockDNSInstanceMutation(ctx context.Context, organizationID string, instanceID string) error {
	_, err := store.db.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext(?), hashtext(?))`, organizationID, "dns_instance:"+instanceID)
	return err
}

func (store *PostgresStore) CreateDNSManagedRecord(ctx context.Context, record DNSManagedRecordRecord) error {
	_, err := store.db.ExecContext(ctx, `
			INSERT INTO dns_managed_records (
				id, organization_id, dns_credential_id, credential_zone_id, zone_id, zone_name,
				record_host, record_name, record_type, ttl, proxied,
				last_applied_values_json, provider_retirements_json, last_evaluation_status, last_evaluation_error, last_diagnostics_json,
				created_at, updated_at
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::jsonb, ?::jsonb, ?, ?, ?::jsonb, ?, ?)
		`, record.ID, record.OrganizationID, record.DNSCredentialID, record.CredentialZoneID, record.ZoneID, record.ZoneName,
		record.RecordHost, record.RecordName, record.RecordType, record.TTL, boolToDB(record.Proxied),
		nonEmptyJSONList(record.LastAppliedValuesJSON), nonEmptyJSONList(record.ProviderRetirementsJSON), defaultString(record.LastEvaluationStatus, "PENDING"), record.LastEvaluationError, nonEmptyJSONList(record.LastDiagnosticsJSON),
		record.CreatedAt, record.UpdatedAt)
	return mapWriteError(err)
}

func (store *PostgresStore) UpdateDNSManagedRecord(ctx context.Context, record DNSManagedRecordRecord) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE dns_managed_records
		SET dns_credential_id = ?,
		    credential_zone_id = ?,
		    zone_id = ?,
		    zone_name = ?,
		    record_host = ?,
		    record_name = ?,
		    record_type = ?,
		    ttl = ?,
			    proxied = ?,
			    last_applied_values_json = ?::jsonb,
			    provider_retirements_json = ?::jsonb,
			    last_evaluation_status = ?,
		    last_evaluation_error = ?,
		    last_diagnostics_json = ?::jsonb,
		    last_evaluated_at = NULLIF(?, '')::timestamptz,
		    last_applied_at = NULLIF(?, '')::timestamptz,
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
		`, record.DNSCredentialID, record.CredentialZoneID, record.ZoneID, record.ZoneName, record.RecordHost, record.RecordName, record.RecordType, record.TTL, boolToDB(record.Proxied),
		nonEmptyJSONList(record.LastAppliedValuesJSON), nonEmptyJSONList(record.ProviderRetirementsJSON), defaultString(record.LastEvaluationStatus, "PENDING"), record.LastEvaluationError, nonEmptyJSONList(record.LastDiagnosticsJSON),
		record.LastEvaluatedAt, record.LastAppliedAt, record.UpdatedAt, record.OrganizationID, record.ID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) DeleteDNSManagedRecord(ctx context.Context, organizationID string, recordID string, deletedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE dns_managed_records
		SET deleted_at = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, deletedAt, deletedAt, organizationID, recordID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) UpdateDNSManagedRecordEvaluation(ctx context.Context, record DNSManagedRecordRecord) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE dns_managed_records
		SET active_instance_id = NULLIF(?, '')::uuid,
			    last_applied_values_json = ?::jsonb,
			    provider_retirements_json = ?::jsonb,
			    last_evaluation_status = ?,
		    last_evaluation_error = ?,
		    last_diagnostics_json = ?::jsonb,
		    last_evaluated_at = NULLIF(?, '')::timestamptz,
		    last_applied_at = NULLIF(?, '')::timestamptz,
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
		`, record.ActiveInstanceID, nonEmptyJSONList(record.LastAppliedValuesJSON), nonEmptyJSONList(record.ProviderRetirementsJSON), record.LastEvaluationStatus, record.LastEvaluationError, nonEmptyJSONList(record.LastDiagnosticsJSON),
		record.LastEvaluatedAt, record.LastAppliedAt, record.UpdatedAt, record.OrganizationID, record.ID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) ListDNSInstancesByOrganization(ctx context.Context, organizationID string) ([]DNSInstanceRecord, error) {
	rows, err := store.db.QueryContext(ctx, dnsInstanceSelect()+`
		JOIN dns_managed_records
		  ON dns_managed_records.organization_id = dns_instances.organization_id
		 AND dns_managed_records.id = dns_instances.managed_record_id
		 AND dns_managed_records.deleted_at IS NULL
		WHERE dns_instances.organization_id = ? AND dns_instances.deleted_at IS NULL
		ORDER BY dns_instances.priority, dns_instances.name, dns_instances.id
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanDNSInstanceList(rows)
}

func (store *PostgresStore) ListDNSInstancesByManagedRecord(ctx context.Context, organizationID string, recordID string) ([]DNSInstanceRecord, error) {
	rows, err := store.db.QueryContext(ctx, dnsInstanceSelect()+`
		JOIN dns_managed_records
		  ON dns_managed_records.organization_id = dns_instances.organization_id
		 AND dns_managed_records.id = dns_instances.managed_record_id
		 AND dns_managed_records.deleted_at IS NULL
		WHERE dns_instances.organization_id = ? AND dns_instances.managed_record_id = ? AND dns_instances.deleted_at IS NULL
		ORDER BY dns_instances.priority, dns_instances.name, dns_instances.id
	`, organizationID, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanDNSInstanceList(rows)
}

func (store *PostgresStore) FindDNSInstanceByID(ctx context.Context, organizationID string, instanceID string) (DNSInstanceRecord, error) {
	row := store.db.QueryRowContext(ctx, dnsInstanceSelect()+`
		JOIN dns_managed_records
		  ON dns_managed_records.organization_id = dns_instances.organization_id
		 AND dns_managed_records.id = dns_instances.managed_record_id
		 AND dns_managed_records.deleted_at IS NULL
		WHERE dns_instances.organization_id = ? AND dns_instances.id = ? AND dns_instances.deleted_at IS NULL
	`, organizationID, instanceID)
	return scanDNSInstance(row)
}

func (store *PostgresStore) CreateDNSInstance(ctx context.Context, instance DNSInstanceRecord) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO dns_instances (
			id, organization_id, managed_record_id, name, priority, enabled, node_group_ids_json,
			answer_count, condition_json, action_json, notification_channel_ids_json,
			last_output_values_json, last_status, last_diagnostics_json, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?::jsonb, ?, ?::jsonb, ?::jsonb, ?::jsonb, ?::jsonb, ?, ?::jsonb, ?, ?)
	`, instance.ID, instance.OrganizationID, instance.ManagedRecordID, instance.Name, instance.Priority, boolToDB(instance.Enabled), nonEmptyJSONList(instance.NodeGroupIDsJSON),
		instance.AnswerCount, nonEmptyJSONObject(instance.ConditionJSON), nonEmptyJSONObject(instance.ActionJSON), nonEmptyJSONList(instance.NotificationChannelIDsJSON),
		nonEmptyJSONList(instance.LastOutputValuesJSON), defaultString(instance.LastStatus, "PENDING"), nonEmptyJSONList(instance.LastDiagnosticsJSON), instance.CreatedAt, instance.UpdatedAt)
	return mapWriteError(err)
}

func (store *PostgresStore) UpdateDNSInstance(ctx context.Context, instance DNSInstanceRecord) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE dns_instances
		SET managed_record_id = ?,
		    name = ?,
		    priority = ?,
		    enabled = ?,
		    node_group_ids_json = ?::jsonb,
		    answer_count = ?,
		    condition_json = ?::jsonb,
		    action_json = ?::jsonb,
		    notification_channel_ids_json = ?::jsonb,
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, instance.ManagedRecordID, instance.Name, instance.Priority, boolToDB(instance.Enabled), nonEmptyJSONList(instance.NodeGroupIDsJSON), instance.AnswerCount,
		nonEmptyJSONObject(instance.ConditionJSON), nonEmptyJSONObject(instance.ActionJSON), nonEmptyJSONList(instance.NotificationChannelIDsJSON), instance.UpdatedAt, instance.OrganizationID, instance.ID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) DeleteDNSInstance(ctx context.Context, organizationID string, instanceID string, deletedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE dns_instances
		SET deleted_at = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, deletedAt, deletedAt, organizationID, instanceID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) ClearDNSManagedRecordActiveInstance(ctx context.Context, organizationID string, instanceID string, updatedAt string) error {
	_, err := store.db.ExecContext(ctx, `
		UPDATE dns_managed_records
		SET active_instance_id = NULL,
		    last_evaluation_status = 'PENDING',
		    last_evaluation_error = '',
		    last_diagnostics_json = '[{"code":"STALE_ACTIVE_INSTANCE_CLEARED","message":"Active DNS instance changed; re-evaluation is required."}]'::jsonb,
		    updated_at = ?
		WHERE organization_id = ?
		  AND active_instance_id = NULLIF(?, '')::uuid
		  AND deleted_at IS NULL
	`, updatedAt, organizationID, instanceID)
	return mapWriteError(err)
}

func (store *PostgresStore) UpdateDNSInstanceEvaluation(ctx context.Context, instance DNSInstanceRecord) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE dns_instances
		SET last_output_values_json = ?::jsonb,
		    last_status = ?,
		    last_diagnostics_json = ?::jsonb,
		    last_evaluated_at = NULLIF(?, '')::timestamptz,
		    updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, nonEmptyJSONList(instance.LastOutputValuesJSON), instance.LastStatus, nonEmptyJSONList(instance.LastDiagnosticsJSON), instance.LastEvaluatedAt, instance.UpdatedAt, instance.OrganizationID, instance.ID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) ListNotificationChannelsByOrganization(ctx context.Context, organizationID string) ([]NotificationChannelRecord, error) {
	rows, err := store.db.QueryContext(ctx, notificationChannelSelect()+`
		WHERE organization_id = ? AND deleted_at IS NULL
		ORDER BY name, id
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	channels := make([]NotificationChannelRecord, 0)
	for rows.Next() {
		channel, err := scanNotificationChannelRows(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

func (store *PostgresStore) FindNotificationChannelByID(ctx context.Context, organizationID string, channelID string) (NotificationChannelRecord, error) {
	row := store.db.QueryRowContext(ctx, notificationChannelSelect()+`
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, organizationID, channelID)
	return scanNotificationChannel(row)
}

func (store *PostgresStore) CreateNotificationChannel(ctx context.Context, channel NotificationChannelRecord) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO notification_channels (id, organization_id, name, channel_type, config_json, encrypted_secret, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?::jsonb, ?, ?, ?, ?)
	`, channel.ID, channel.OrganizationID, channel.Name, channel.ChannelType, nonEmptyJSONObject(channel.ConfigJSON), channel.EncryptedSecret, boolToDB(channel.Enabled), channel.CreatedAt, channel.UpdatedAt)
	return mapWriteError(err)
}

func (store *PostgresStore) UpdateNotificationChannel(ctx context.Context, channel NotificationChannelRecord, replaceSecret bool) error {
	query := `
		UPDATE notification_channels
		SET name = ?, channel_type = ?, config_json = ?::jsonb, enabled = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`
	args := []any{channel.Name, channel.ChannelType, nonEmptyJSONObject(channel.ConfigJSON), boolToDB(channel.Enabled), channel.UpdatedAt, channel.OrganizationID, channel.ID}
	if replaceSecret {
		query = `
			UPDATE notification_channels
			SET name = ?, channel_type = ?, config_json = ?::jsonb, encrypted_secret = ?, enabled = ?, updated_at = ?
			WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
		`
		args = []any{channel.Name, channel.ChannelType, nonEmptyJSONObject(channel.ConfigJSON), channel.EncryptedSecret, boolToDB(channel.Enabled), channel.UpdatedAt, channel.OrganizationID, channel.ID}
	}
	result, err := store.db.ExecContext(ctx, query, args...)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) DeleteNotificationChannel(ctx context.Context, organizationID string, channelID string, deletedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE notification_channels
		SET deleted_at = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, deletedAt, deletedAt, organizationID, channelID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) CreateNotificationDelivery(ctx context.Context, delivery NotificationDeliveryRecord) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO notification_deliveries (
			id, organization_id, channel_id, dns_managed_record_id, dns_instance_id,
			event_type, status, error_message, payload_json, created_at, delivered_at
		)
		VALUES (?, ?, ?, NULLIF(?, '')::uuid, NULLIF(?, '')::uuid, ?, ?, ?, ?::jsonb, ?, NULLIF(?, '')::timestamptz)
	`, delivery.ID, delivery.OrganizationID, delivery.ChannelID, delivery.DNSManagedRecordID, delivery.DNSInstanceID, delivery.EventType, delivery.Status, delivery.ErrorMessage, nonEmptyJSONObject(delivery.PayloadJSON), delivery.CreatedAt, delivery.DeliveredAt)
	return mapWriteError(err)
}

func dnsManagedRecordSelect() string {
	return `
			SELECT id, organization_id, dns_credential_id, credential_zone_id, zone_id, zone_name,
			       record_host, record_name, record_type, ttl, proxied, COALESCE(active_instance_id::text, ''),
			       last_applied_values_json::text, provider_retirements_json::text, last_evaluation_status, last_evaluation_error, last_diagnostics_json::text,
			       COALESCE(last_evaluated_at::text, ''), COALESCE(last_applied_at::text, ''),
		       created_at, updated_at, COALESCE(deleted_at::text, '')
		FROM dns_managed_records
	`
}

func scanDNSManagedRecord(row rowScanner) (DNSManagedRecordRecord, error) {
	var record DNSManagedRecordRecord
	if err := row.Scan(&record.ID, &record.OrganizationID, &record.DNSCredentialID, &record.CredentialZoneID, &record.ZoneID, &record.ZoneName,
		&record.RecordHost, &record.RecordName, &record.RecordType, &record.TTL, &record.Proxied, &record.ActiveInstanceID,
		&record.LastAppliedValuesJSON, &record.ProviderRetirementsJSON, &record.LastEvaluationStatus, &record.LastEvaluationError, &record.LastDiagnosticsJSON,
		&record.LastEvaluatedAt, &record.LastAppliedAt, &record.CreatedAt, &record.UpdatedAt, &record.DeletedAt); err != nil {
		return DNSManagedRecordRecord{}, mapReadError(err)
	}
	return record, nil
}

func scanDNSManagedRecordRows(rows *sql.Rows) (DNSManagedRecordRecord, error) {
	return scanDNSManagedRecord(rows)
}

func dnsInstanceSelect() string {
	return `
		SELECT dns_instances.id, dns_instances.organization_id, dns_instances.managed_record_id, dns_instances.name, dns_instances.priority,
		       dns_instances.enabled, dns_instances.node_group_ids_json::text,
		       dns_instances.answer_count, dns_instances.condition_json::text, dns_instances.action_json::text,
		       dns_instances.notification_channel_ids_json::text,
		       dns_instances.last_output_values_json::text, dns_instances.last_status, dns_instances.last_diagnostics_json::text,
		       COALESCE(dns_instances.last_evaluated_at::text, ''), dns_instances.created_at, dns_instances.updated_at, COALESCE(dns_instances.deleted_at::text, '')
		FROM dns_instances
	`
}

func scanDNSInstance(row rowScanner) (DNSInstanceRecord, error) {
	var instance DNSInstanceRecord
	if err := row.Scan(&instance.ID, &instance.OrganizationID, &instance.ManagedRecordID, &instance.Name, &instance.Priority, &instance.Enabled, &instance.NodeGroupIDsJSON,
		&instance.AnswerCount, &instance.ConditionJSON, &instance.ActionJSON, &instance.NotificationChannelIDsJSON,
		&instance.LastOutputValuesJSON, &instance.LastStatus, &instance.LastDiagnosticsJSON,
		&instance.LastEvaluatedAt, &instance.CreatedAt, &instance.UpdatedAt, &instance.DeletedAt); err != nil {
		return DNSInstanceRecord{}, mapReadError(err)
	}
	return instance, nil
}

func scanDNSInstanceList(rows *sql.Rows) ([]DNSInstanceRecord, error) {
	instances := make([]DNSInstanceRecord, 0)
	for rows.Next() {
		instance, err := scanDNSInstance(rows)
		if err != nil {
			return nil, err
		}
		instances = append(instances, instance)
	}
	return instances, rows.Err()
}

func notificationChannelSelect() string {
	return `
		SELECT id, organization_id, name, channel_type, config_json::text, encrypted_secret, enabled, created_at, updated_at, COALESCE(deleted_at::text, '')
		FROM notification_channels
	`
}

func scanNotificationChannel(row rowScanner) (NotificationChannelRecord, error) {
	var channel NotificationChannelRecord
	if err := row.Scan(&channel.ID, &channel.OrganizationID, &channel.Name, &channel.ChannelType, &channel.ConfigJSON, &channel.EncryptedSecret, &channel.Enabled, &channel.CreatedAt, &channel.UpdatedAt, &channel.DeletedAt); err != nil {
		return NotificationChannelRecord{}, mapReadError(err)
	}
	return channel, nil
}

func scanNotificationChannelRows(rows *sql.Rows) (NotificationChannelRecord, error) {
	return scanNotificationChannel(rows)
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func nonEmptyJSONObject(value string) string {
	if value == "" {
		return "{}"
	}
	return value
}
