package repo

import (
	"context"
	"database/sql"
)

func (store *PostgresStore) ListHealthChecksByOrganization(ctx context.Context, organizationID string) ([]HealthCheckRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, name, probe_type, interval_seconds, timeout_seconds, config_json::text, enabled, created_at, updated_at, COALESCE(deleted_at::text, '')
		FROM health_checks
		WHERE organization_id = ? AND deleted_at IS NULL
		ORDER BY name
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	checks := make([]HealthCheckRecord, 0)
	for rows.Next() {
		check, err := scanHealthCheckRows(rows)
		if err != nil {
			return nil, err
		}
		checks = append(checks, check)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range checks {
		if err := store.attachHealthCheckChildren(ctx, &checks[index]); err != nil {
			return nil, err
		}
	}
	return checks, nil
}

func (store *PostgresStore) FindHealthCheckByID(ctx context.Context, organizationID string, healthCheckID string) (HealthCheckRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT id, organization_id, name, probe_type, interval_seconds, timeout_seconds, config_json::text, enabled, created_at, updated_at, COALESCE(deleted_at::text, '')
		FROM health_checks
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, organizationID, healthCheckID)
	check, err := scanHealthCheck(row)
	if err != nil {
		return HealthCheckRecord{}, err
	}
	if err := store.attachHealthCheckChildren(ctx, &check); err != nil {
		return HealthCheckRecord{}, err
	}
	return check, nil
}

func (store *PostgresStore) CreateHealthCheck(ctx context.Context, healthCheck HealthCheckRecord, targets []HealthCheckTargetRecord, monitorScopes []HealthCheckMonitorScopeRecord, now string, nextID func() string) error {
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO health_checks (id, organization_id, name, probe_type, interval_seconds, timeout_seconds, config_json, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?::jsonb, ?, ?, ?)
	`, healthCheck.ID, healthCheck.OrganizationID, healthCheck.Name, healthCheck.ProbeType, healthCheck.IntervalSeconds, healthCheck.TimeoutSeconds, healthCheck.ConfigJSON, healthCheck.Enabled, healthCheck.CreatedAt, healthCheck.UpdatedAt); err != nil {
		return err
	}
	return store.replaceHealthCheckChildren(ctx, healthCheck.OrganizationID, healthCheck.ID, targets, monitorScopes, now, nextID)
}

func (store *PostgresStore) UpdateHealthCheck(ctx context.Context, healthCheck HealthCheckRecord, targets []HealthCheckTargetRecord, monitorScopes []HealthCheckMonitorScopeRecord, now string, nextID func() string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE health_checks
		SET name = ?, probe_type = ?, interval_seconds = ?, timeout_seconds = ?, config_json = ?::jsonb, enabled = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, healthCheck.Name, healthCheck.ProbeType, healthCheck.IntervalSeconds, healthCheck.TimeoutSeconds, healthCheck.ConfigJSON, healthCheck.Enabled, healthCheck.UpdatedAt, healthCheck.OrganizationID, healthCheck.ID)
	if err != nil {
		return err
	}
	if err := requireAffected(result); err != nil {
		return err
	}
	return store.replaceHealthCheckChildren(ctx, healthCheck.OrganizationID, healthCheck.ID, targets, monitorScopes, now, nextID)
}

func (store *PostgresStore) SyncHealthCheckTargets(ctx context.Context, organizationID string, healthCheckID string, targets []HealthCheckTargetRecord, now string, nextID func() string) error {
	current, err := store.listHealthCheckTargets(ctx, organizationID, healthCheckID)
	if err != nil {
		return err
	}
	desired := make(map[string]HealthCheckTargetRecord, len(targets))
	for _, target := range targets {
		if target.ScopeType != "TARGET_GROUP" {
			continue
		}
		desired[healthCheckTargetGroupKey(target.TargetID, target.TargetGroupID)] = target
	}
	for _, target := range current {
		if target.ScopeType != "TARGET_GROUP" {
			continue
		}
		if _, ok := desired[healthCheckTargetGroupKey(target.TargetID, target.TargetGroupID)]; ok {
			continue
		}
		if _, err := store.db.ExecContext(ctx, `
			DELETE FROM health_check_targets
			WHERE organization_id = ? AND health_check_id = ? AND scope_type = 'TARGET_GROUP' AND target_group_id = ?::uuid
			  AND ((target_id IS NULL AND ? = '') OR target_id = NULLIF(?, '')::uuid)
		`, organizationID, healthCheckID, target.TargetGroupID, target.TargetID, target.TargetID); err != nil {
			return mapWriteError(err)
		}
	}
	for _, target := range desired {
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO health_check_targets (id, organization_id, health_check_id, scope_type, target_id, target_group_id, created_at)
			VALUES (?, ?, ?, 'TARGET_GROUP', NULLIF(?, '')::uuid, ?::uuid, ?)
			ON CONFLICT DO NOTHING
		`, nextID(), organizationID, healthCheckID, target.TargetID, target.TargetGroupID, now); err != nil {
			return mapWriteError(err)
		}
	}
	return nil
}

func (store *PostgresStore) DeleteHealthCheck(ctx context.Context, organizationID string, healthCheckID string, deletedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE health_checks
		SET deleted_at = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, deletedAt, deletedAt, organizationID, healthCheckID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (store *PostgresStore) ListHealthResults(ctx context.Context, organizationID string, healthCheckID string, limit int) ([]HealthResultRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, health_check_id, health_check_target_id, monitor_id, target_id, status, COALESCE(latency_ms, -1), error_message, observed_at, created_at
		FROM health_results
		WHERE organization_id = ? AND (? = '' OR health_check_id = ?)
		ORDER BY observed_at DESC
		LIMIT ?
	`, organizationID, healthCheckID, healthCheckID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	results := make([]HealthResultRecord, 0)
	for rows.Next() {
		result, err := scanHealthResultRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

func (store *PostgresStore) RecordHealthResults(ctx context.Context, organizationID string, results []HealthResultRecord) error {
	for _, result := range results {
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO health_results (
				id, organization_id, health_check_id, health_check_target_id, monitor_id, target_id,
				status, latency_ms, error_message, observed_at, created_at
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, -1), ?, ?, ?)
		`, result.ID, organizationID, result.HealthCheckID, result.HealthCheckTargetID, result.MonitorID, result.TargetID, result.Status, result.LatencyMS, result.ErrorMessage, result.ObservedAt, result.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

func (store *PostgresStore) ListHealthEvaluationRulesByCheck(ctx context.Context, organizationID string, healthCheckID string) ([]HealthEvaluationRuleRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, health_check_id, name, enabled, expression_json::text, created_at, updated_at, COALESCE(deleted_at::text, '')
		FROM health_evaluation_rules
		WHERE organization_id = ? AND health_check_id = ? AND deleted_at IS NULL
		ORDER BY name, id
	`, organizationID, healthCheckID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	rules := make([]HealthEvaluationRuleRecord, 0)
	for rows.Next() {
		rule, err := scanHealthEvaluationRuleRows(rows)
		if err != nil {
			return nil, err
		}
		events, err := store.listHealthEventsByRule(ctx, rule.OrganizationID, rule.ID)
		if err != nil {
			return nil, err
		}
		rule.Events = events
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (store *PostgresStore) CreateHealthEvaluationRule(ctx context.Context, rule HealthEvaluationRuleRecord, events []HealthEventRecord) error {
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO health_evaluation_rules (id, organization_id, health_check_id, name, enabled, expression_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?::jsonb, ?, ?)
	`, rule.ID, rule.OrganizationID, rule.HealthCheckID, rule.Name, rule.Enabled, rule.ExpressionJSON, rule.CreatedAt, rule.UpdatedAt); err != nil {
		return err
	}
	for _, event := range events {
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO health_events (id, organization_id, health_evaluation_rule_id, event_type, config_json, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?::jsonb, ?, ?, ?)
		`, event.ID, rule.OrganizationID, rule.ID, event.EventType, event.ConfigJSON, event.Enabled, event.CreatedAt, event.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}

func (store *PostgresStore) DeleteHealthEvaluationRulesForDNSRecord(ctx context.Context, organizationID string, dnsRecordID string, deletedAt string) error {
	_, err := store.db.ExecContext(ctx, `
		UPDATE health_events
		SET deleted_at = ?, updated_at = ?
		WHERE organization_id = ?
		  AND deleted_at IS NULL
		  AND config_json->>'dns_record_id' = ?
	`, deletedAt, deletedAt, organizationID, dnsRecordID)
	if err != nil {
		return err
	}
	_, err = store.db.ExecContext(ctx, `
		UPDATE health_evaluation_rules
		SET deleted_at = ?, updated_at = ?
		WHERE organization_id = ?
		  AND deleted_at IS NULL
		  AND id IN (
		    SELECT health_evaluation_rule_id
		    FROM health_events
		    WHERE organization_id = ? AND config_json->>'dns_record_id' = ?
		  )
	`, deletedAt, deletedAt, organizationID, organizationID, dnsRecordID)
	return err
}

func (store *PostgresStore) listHealthEventsByRule(ctx context.Context, organizationID string, ruleID string) ([]HealthEventRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, health_evaluation_rule_id, event_type, config_json::text, enabled, created_at, updated_at, COALESCE(deleted_at::text, '')
		FROM health_events
		WHERE organization_id = ? AND health_evaluation_rule_id = ? AND deleted_at IS NULL
		ORDER BY event_type, id
	`, organizationID, ruleID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	events := make([]HealthEventRecord, 0)
	for rows.Next() {
		event, err := scanHealthEventRows(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (store *PostgresStore) attachHealthCheckChildren(ctx context.Context, healthCheck *HealthCheckRecord) error {
	targets, err := store.listHealthCheckTargets(ctx, healthCheck.OrganizationID, healthCheck.ID)
	if err != nil {
		return err
	}
	scopes, err := store.listHealthCheckMonitorScopes(ctx, healthCheck.OrganizationID, healthCheck.ID)
	if err != nil {
		return err
	}
	healthCheck.Targets = targets
	healthCheck.MonitorScopes = scopes
	return nil
}

func (store *PostgresStore) listHealthCheckTargets(ctx context.Context, organizationID string, healthCheckID string) ([]HealthCheckTargetRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT hct.id, hct.organization_id, hct.health_check_id, hct.scope_type, COALESCE(hct.target_id::text, ''), COALESCE(hct.target_group_id::text, ''), COALESCE(targets.name, ''), COALESCE(targets.host, ''), COALESCE(targets.port, 0), hct.created_at
		FROM health_check_targets hct
		LEFT JOIN targets ON targets.organization_id = hct.organization_id AND targets.id = hct.target_id AND targets.deleted_at IS NULL
		WHERE hct.organization_id = ? AND hct.health_check_id = ?
		  AND (hct.target_id IS NULL OR targets.id IS NOT NULL)
		ORDER BY targets.name NULLS FIRST, hct.target_id
	`, organizationID, healthCheckID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	targets := make([]HealthCheckTargetRecord, 0)
	for rows.Next() {
		target, err := scanHealthCheckTargetRows(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, rows.Err()
}

func (store *PostgresStore) listHealthCheckMonitorScopes(ctx context.Context, organizationID string, healthCheckID string) ([]HealthCheckMonitorScopeRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, health_check_id, scope_type, COALESCE(monitor_id::text, ''), COALESCE(monitor_group_id::text, ''), created_at
		FROM health_check_monitor_scopes
		WHERE organization_id = ? AND health_check_id = ?
		ORDER BY scope_type, monitor_id, monitor_group_id
	`, organizationID, healthCheckID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	scopes := make([]HealthCheckMonitorScopeRecord, 0)
	for rows.Next() {
		scope, err := scanHealthCheckMonitorScopeRows(rows)
		if err != nil {
			return nil, err
		}
		scopes = append(scopes, scope)
	}
	return scopes, rows.Err()
}

func (store *PostgresStore) replaceHealthCheckChildren(ctx context.Context, organizationID string, healthCheckID string, targets []HealthCheckTargetRecord, monitorScopes []HealthCheckMonitorScopeRecord, now string, nextID func() string) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM health_check_targets WHERE organization_id = ? AND health_check_id = ?`, organizationID, healthCheckID); err != nil {
		return err
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM health_check_monitor_scopes WHERE organization_id = ? AND health_check_id = ?`, organizationID, healthCheckID); err != nil {
		return err
	}
	for _, target := range targets {
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO health_check_targets (id, organization_id, health_check_id, scope_type, target_id, target_group_id, created_at)
			VALUES (?, ?, ?, ?, NULLIF(?, '')::uuid, NULLIF(?, '')::uuid, ?)
		`, nextID(), organizationID, healthCheckID, target.ScopeType, target.TargetID, target.TargetGroupID, now); err != nil {
			return err
		}
	}
	for _, scope := range monitorScopes {
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO health_check_monitor_scopes (id, organization_id, health_check_id, scope_type, monitor_id, monitor_group_id, created_at)
			VALUES (?, ?, ?, ?, NULLIF(?, '')::uuid, NULLIF(?, '')::uuid, ?)
		`, nextID(), organizationID, healthCheckID, scope.ScopeType, scope.MonitorID, scope.MonitorGroupID, now); err != nil {
			return err
		}
	}
	return nil
}

func healthCheckTargetGroupKey(targetID string, targetGroupID string) string {
	return targetID + "\x00" + targetGroupID
}

func scanHealthCheck(row rowScanner) (HealthCheckRecord, error) {
	var check HealthCheckRecord
	if err := row.Scan(&check.ID, &check.OrganizationID, &check.Name, &check.ProbeType, &check.IntervalSeconds, &check.TimeoutSeconds, &check.ConfigJSON, &check.Enabled, &check.CreatedAt, &check.UpdatedAt, &check.DeletedAt); err != nil {
		return HealthCheckRecord{}, mapReadError(err)
	}
	return check, nil
}

func scanHealthCheckRows(rows *sql.Rows) (HealthCheckRecord, error) {
	return scanHealthCheck(rows)
}

func scanHealthCheckTargetRows(rows *sql.Rows) (HealthCheckTargetRecord, error) {
	var target HealthCheckTargetRecord
	if err := rows.Scan(&target.ID, &target.OrganizationID, &target.HealthCheckID, &target.ScopeType, &target.TargetID, &target.TargetGroupID, &target.TargetName, &target.TargetHost, &target.TargetPort, &target.CreatedAt); err != nil {
		return HealthCheckTargetRecord{}, mapReadError(err)
	}
	return target, nil
}

func scanHealthCheckMonitorScopeRows(rows *sql.Rows) (HealthCheckMonitorScopeRecord, error) {
	var scope HealthCheckMonitorScopeRecord
	if err := rows.Scan(&scope.ID, &scope.OrganizationID, &scope.HealthCheckID, &scope.ScopeType, &scope.MonitorID, &scope.MonitorGroupID, &scope.CreatedAt); err != nil {
		return HealthCheckMonitorScopeRecord{}, mapReadError(err)
	}
	return scope, nil
}

func scanHealthResultRows(rows *sql.Rows) (HealthResultRecord, error) {
	var result HealthResultRecord
	if err := rows.Scan(&result.ID, &result.OrganizationID, &result.HealthCheckID, &result.HealthCheckTargetID, &result.MonitorID, &result.TargetID, &result.Status, &result.LatencyMS, &result.ErrorMessage, &result.ObservedAt, &result.CreatedAt); err != nil {
		return HealthResultRecord{}, mapReadError(err)
	}
	return result, nil
}

func scanHealthEvaluationRuleRows(rows *sql.Rows) (HealthEvaluationRuleRecord, error) {
	var rule HealthEvaluationRuleRecord
	if err := rows.Scan(&rule.ID, &rule.OrganizationID, &rule.HealthCheckID, &rule.Name, &rule.Enabled, &rule.ExpressionJSON, &rule.CreatedAt, &rule.UpdatedAt, &rule.DeletedAt); err != nil {
		return HealthEvaluationRuleRecord{}, mapReadError(err)
	}
	return rule, nil
}

func scanHealthEventRows(rows *sql.Rows) (HealthEventRecord, error) {
	var event HealthEventRecord
	if err := rows.Scan(&event.ID, &event.OrganizationID, &event.HealthEvaluationRuleID, &event.EventType, &event.ConfigJSON, &event.Enabled, &event.CreatedAt, &event.UpdatedAt, &event.DeletedAt); err != nil {
		return HealthEventRecord{}, mapReadError(err)
	}
	return event, nil
}

func (store *PostgresStore) ListDNSCredentialsByOrganization(ctx context.Context, organizationID string) ([]DNSCredentialRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, provider, name, encrypted_secret, created_at, updated_at, COALESCE(deleted_at::text, '')
		FROM dns_credentials
		WHERE organization_id = ? AND deleted_at IS NULL
		ORDER BY name
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	credentials := make([]DNSCredentialRecord, 0)
	for rows.Next() {
		credential, err := scanDNSCredentialRows(rows)
		if err != nil {
			return nil, err
		}
		credentials = append(credentials, credential)
	}
	return credentials, rows.Err()
}

func (store *PostgresStore) FindDNSCredentialByID(ctx context.Context, organizationID string, credentialID string) (DNSCredentialRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT id, organization_id, provider, name, encrypted_secret, created_at, updated_at, COALESCE(deleted_at::text, '')
		FROM dns_credentials
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, organizationID, credentialID)
	return scanDNSCredential(row)
}

func (store *PostgresStore) CreateDNSCredential(ctx context.Context, credential DNSCredentialRecord) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO dns_credentials (id, organization_id, provider, name, encrypted_secret, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, credential.ID, credential.OrganizationID, credential.Provider, credential.Name, credential.EncryptedSecret, credential.CreatedAt, credential.UpdatedAt)
	return err
}

func (store *PostgresStore) UpdateDNSCredential(ctx context.Context, credential DNSCredentialRecord, replaceSecret bool) error {
	var result sql.Result
	var err error
	if replaceSecret {
		result, err = store.db.ExecContext(ctx, `
			UPDATE dns_credentials
			SET provider = ?, name = ?, encrypted_secret = ?, updated_at = ?
			WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
		`, credential.Provider, credential.Name, credential.EncryptedSecret, credential.UpdatedAt, credential.OrganizationID, credential.ID)
	} else {
		result, err = store.db.ExecContext(ctx, `
			UPDATE dns_credentials
			SET provider = ?, name = ?, updated_at = ?
			WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
		`, credential.Provider, credential.Name, credential.UpdatedAt, credential.OrganizationID, credential.ID)
	}
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (store *PostgresStore) DeleteDNSCredential(ctx context.Context, organizationID string, credentialID string, deletedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE dns_credentials
		SET deleted_at = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, deletedAt, deletedAt, organizationID, credentialID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return ErrNotFound
	}
	return nil
}

func scanDNSCredential(row rowScanner) (DNSCredentialRecord, error) {
	var credential DNSCredentialRecord
	if err := row.Scan(&credential.ID, &credential.OrganizationID, &credential.Provider, &credential.Name, &credential.EncryptedSecret, &credential.CreatedAt, &credential.UpdatedAt, &credential.DeletedAt); err != nil {
		return DNSCredentialRecord{}, mapReadError(err)
	}
	return credential, nil
}

func scanDNSCredentialRows(rows *sql.Rows) (DNSCredentialRecord, error) {
	return scanDNSCredential(rows)
}

func (store *PostgresStore) ListDNSRecordsByOrganization(ctx context.Context, organizationID string) ([]DNSRecordRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, dns_credential_id, zone, record_name, record_type, managed_mode, desired_values_json::text, last_applied_values_json::text, COALESCE(last_applied_at::text, ''), created_at, updated_at, COALESCE(deleted_at::text, '')
		FROM dns_records
		WHERE organization_id = ? AND deleted_at IS NULL
		ORDER BY zone, record_name, record_type
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	records := make([]DNSRecordRecord, 0)
	for rows.Next() {
		record, err := scanDNSRecordRows(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (store *PostgresStore) FindDNSRecordByID(ctx context.Context, organizationID string, recordID string) (DNSRecordRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT id, organization_id, dns_credential_id, zone, record_name, record_type, managed_mode, desired_values_json::text, last_applied_values_json::text, COALESCE(last_applied_at::text, ''), created_at, updated_at, COALESCE(deleted_at::text, '')
		FROM dns_records
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, organizationID, recordID)
	return scanDNSRecord(row)
}

func (store *PostgresStore) CreateDNSRecord(ctx context.Context, record DNSRecordRecord) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO dns_records (
			id, organization_id, dns_credential_id, zone, record_name, record_type,
			managed_mode, desired_values_json, last_applied_values_json, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?::jsonb, ?::jsonb, ?, ?)
	`, record.ID, record.OrganizationID, record.DNSCredentialID, record.Zone, record.RecordName, record.RecordType, record.ManagedMode, record.DesiredValuesJSON, record.LastAppliedValuesJSON, record.CreatedAt, record.UpdatedAt)
	return err
}

func (store *PostgresStore) UpdateDNSRecord(ctx context.Context, record DNSRecordRecord) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE dns_records
		SET dns_credential_id = ?, zone = ?, record_name = ?, record_type = ?, managed_mode = ?, desired_values_json = ?::jsonb, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, record.DNSCredentialID, record.Zone, record.RecordName, record.RecordType, record.ManagedMode, record.DesiredValuesJSON, record.UpdatedAt, record.OrganizationID, record.ID)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (store *PostgresStore) UpdateDNSRecordLastApplied(ctx context.Context, organizationID string, recordID string, lastAppliedValuesJSON string, lastAppliedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE dns_records
		SET last_applied_values_json = ?::jsonb, last_applied_at = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, lastAppliedValuesJSON, lastAppliedAt, lastAppliedAt, organizationID, recordID)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (store *PostgresStore) DeleteDNSRecord(ctx context.Context, organizationID string, recordID string, deletedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE dns_records
		SET deleted_at = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, deletedAt, deletedAt, organizationID, recordID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return ErrNotFound
	}
	return nil
}

func scanDNSRecord(row rowScanner) (DNSRecordRecord, error) {
	var record DNSRecordRecord
	if err := row.Scan(&record.ID, &record.OrganizationID, &record.DNSCredentialID, &record.Zone, &record.RecordName, &record.RecordType, &record.ManagedMode, &record.DesiredValuesJSON, &record.LastAppliedValuesJSON, &record.LastAppliedAt, &record.CreatedAt, &record.UpdatedAt, &record.DeletedAt); err != nil {
		return DNSRecordRecord{}, mapReadError(err)
	}
	return record, nil
}

func scanDNSRecordRows(rows *sql.Rows) (DNSRecordRecord, error) {
	return scanDNSRecord(rows)
}
