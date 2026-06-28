package repo

import "context"

func (store *PostgresStore) ListQuotasByOrganization(ctx context.Context, organizationID string) ([]QuotaRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, scope, coalesce(subject_user_id, ''), coalesce(subject_rule_id::text, ''),
		       rule_limit, traffic_limit_bytes, traffic_limit_mode, over_limit_action, created_at, updated_at
		FROM quotas
		WHERE organization_id = ?
		ORDER BY scope, id
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	quotas := make([]QuotaRecord, 0)
	for rows.Next() {
		var quota QuotaRecord
		if err := rows.Scan(
			&quota.ID,
			&quota.OrganizationID,
			&quota.Scope,
			&quota.SubjectUserID,
			&quota.SubjectRuleID,
			&quota.RuleLimit,
			&quota.TrafficLimitBytes,
			&quota.TrafficLimitMode,
			&quota.OverLimitAction,
			&quota.CreatedAt,
			&quota.UpdatedAt,
		); err != nil {
			return nil, err
		}
		quotas = append(quotas, quota)
	}
	return quotas, rows.Err()
}

func (store *PostgresStore) ListRegistrationTokens(ctx context.Context, organizationID string, agentType string, agentID string) ([]AgentRegistrationTokenRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, agent_type, agent_id, token_hash, expires_at,
		       coalesce(used_at::text, ''), coalesce(revoked_at::text, ''), created_at, coalesce(created_by_user_id, '')
		FROM agent_registration_tokens
		WHERE organization_id = ? AND agent_type = ? AND agent_id = ?
		ORDER BY created_at DESC, id DESC
	`, organizationID, agentType, agentID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	tokens := make([]AgentRegistrationTokenRecord, 0)
	for rows.Next() {
		token, err := scanRegistrationTokenRows(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

func (store *PostgresStore) FindRegistrationTokenByHash(ctx context.Context, tokenHash string) (AgentRegistrationTokenRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT id, organization_id, agent_type, agent_id, token_hash, expires_at,
		       coalesce(used_at::text, ''), coalesce(revoked_at::text, ''), created_at, coalesce(created_by_user_id, '')
		FROM agent_registration_tokens
		WHERE token_hash = ?
	`, tokenHash)
	return scanRegistrationToken(row)
}

func (store *PostgresStore) CreateRegistrationToken(ctx context.Context, token AgentRegistrationTokenRecord) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO agent_registration_tokens (
			id, organization_id, agent_type, agent_id, token_hash, expires_at, created_at, created_by_user_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, token.ID, token.OrganizationID, token.AgentType, token.AgentID, token.TokenHash, token.ExpiresAt, token.CreatedAt, nullable(token.CreatedByUserID))
	return mapWriteError(err)
}

func (store *PostgresStore) ClaimRegistrationToken(ctx context.Context, organizationID string, tokenID string, claimedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE agent_registration_tokens
		SET used_at = ?
		WHERE organization_id = ? AND id = ? AND used_at IS NULL AND revoked_at IS NULL
		  AND expires_at > ?
	`, claimedAt, organizationID, tokenID, claimedAt)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) ReleaseRegistrationTokenUse(ctx context.Context, organizationID string, tokenID string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE agent_registration_tokens
		SET used_at = NULL
		WHERE organization_id = ? AND id = ? AND used_at IS NOT NULL AND revoked_at IS NULL
	`, organizationID, tokenID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) RevokeActiveUnusedRegistrationTokens(ctx context.Context, organizationID string, agentType string, agentID string, revokedAt string) error {
	_, err := store.db.ExecContext(ctx, `
		UPDATE agent_registration_tokens
		SET revoked_at = ?
		WHERE organization_id = ? AND agent_type = ? AND agent_id = ?
		  AND used_at IS NULL AND revoked_at IS NULL
	`, revokedAt, organizationID, agentType, agentID)
	return mapWriteError(err)
}

func (store *PostgresStore) RevokeRegistrationToken(ctx context.Context, organizationID string, agentType string, agentID string, tokenID string, revokedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE agent_registration_tokens
		SET revoked_at = ?
		WHERE organization_id = ? AND agent_type = ? AND agent_id = ? AND id = ?
		  AND revoked_at IS NULL
	`, revokedAt, organizationID, agentType, agentID, tokenID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) FindCredentialByHash(ctx context.Context, credentialHash string) (AgentCredentialRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT id, organization_id, agent_type, agent_id, credential_hash,
		       coalesce(registration_token_id::text, ''), coalesce(enrollment_profile_id::text, ''), coalesce(enrollment_token_hash, ''), coalesce(activated_at::text, ''),
		       coalesce(revoked_at::text, ''), created_at, coalesce(rotated_at::text, '')
		FROM agent_credentials
		WHERE credential_hash = ?
	`, credentialHash)
	return scanAgentCredential(row)
}

func (store *PostgresStore) FindPendingCredentialByRegistrationToken(ctx context.Context, organizationID string, registrationTokenID string) (AgentCredentialRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT id, organization_id, agent_type, agent_id, credential_hash,
		       coalesce(registration_token_id::text, ''), coalesce(enrollment_profile_id::text, ''), coalesce(enrollment_token_hash, ''), coalesce(activated_at::text, ''),
		       coalesce(revoked_at::text, ''), created_at, coalesce(rotated_at::text, '')
		FROM agent_credentials
		WHERE organization_id = ? AND registration_token_id = ?
		  AND activated_at IS NULL AND revoked_at IS NULL
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, organizationID, registrationTokenID)
	return scanAgentCredential(row)
}

func (store *PostgresStore) ListPendingCredentialsByEnrollmentProfile(ctx context.Context, organizationID string, enrollmentProfileID string) ([]AgentCredentialRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, agent_type, agent_id, credential_hash,
		       coalesce(registration_token_id::text, ''), coalesce(enrollment_profile_id::text, ''), coalesce(enrollment_token_hash, ''), coalesce(activated_at::text, ''),
		       coalesce(revoked_at::text, ''), created_at, coalesce(rotated_at::text, '')
		FROM agent_credentials
		WHERE organization_id = ? AND enrollment_profile_id = ?
		  AND activated_at IS NULL AND revoked_at IS NULL
		ORDER BY created_at ASC, id ASC
	`, organizationID, enrollmentProfileID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	credentials := make([]AgentCredentialRecord, 0)
	for rows.Next() {
		credential, err := scanAgentCredentialRows(rows)
		if err != nil {
			return nil, err
		}
		credentials = append(credentials, credential)
	}
	return credentials, rows.Err()
}

func (store *PostgresStore) CreateCredential(ctx context.Context, credential AgentCredentialRecord) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO agent_credentials (
			id, organization_id, agent_type, agent_id, credential_hash, registration_token_id, enrollment_profile_id, enrollment_token_hash, activated_at, created_at, rotated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, credential.ID, credential.OrganizationID, credential.AgentType, credential.AgentID, credential.CredentialHash, nullable(credential.RegistrationTokenID), nullable(credential.EnrollmentProfileID), credential.EnrollmentTokenHash, nullable(credential.ActivatedAt), credential.CreatedAt, nullable(credential.RotatedAt))
	return mapWriteError(err)
}

func (store *PostgresStore) ActivateCredential(ctx context.Context, organizationID string, credentialID string, activatedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE agent_credentials
		SET activated_at = ?
		WHERE organization_id = ? AND id = ? AND activated_at IS NULL AND revoked_at IS NULL
	`, activatedAt, organizationID, credentialID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) RevokeActiveCredentialsExcept(ctx context.Context, organizationID string, agentType string, agentID string, keepCredentialID string, revokedAt string) error {
	_, err := store.db.ExecContext(ctx, `
		UPDATE agent_credentials
		SET revoked_at = ?,
		    rotated_at = ?
		WHERE organization_id = ? AND agent_type = ? AND agent_id = ?
		  AND id <> ?
		  AND revoked_at IS NULL
	`, revokedAt, revokedAt, organizationID, agentType, agentID, keepCredentialID)
	return mapWriteError(err)
}

func (store *PostgresStore) RevokeCredential(ctx context.Context, organizationID string, credentialID string, revokedAt string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE agent_credentials
		SET revoked_at = ?,
		    rotated_at = ?
		WHERE organization_id = ? AND id = ? AND revoked_at IS NULL
		  AND activated_at IS NULL
	`, revokedAt, revokedAt, organizationID, credentialID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) CreateAuditLog(ctx context.Context, audit AuditLogRecord) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO audit_logs (
			id, organization_id, actor_user_id, actor_roles_json, actor_permissions_json,
			action, resource_type, resource_id, result, error_message, metadata_json, source_ip, created_at
		) VALUES (?, ?, ?, ?::jsonb, ?::jsonb, ?, ?, ?, ?, ?, ?::jsonb, ?, ?)
	`, audit.ID, audit.OrganizationID, nullable(audit.ActorUserID), audit.ActorRolesJSON, audit.ActorPermissionsJSON, audit.Action, audit.ResourceType, audit.ResourceID, audit.Result, nullable(audit.ErrorMessage), audit.MetadataJSON, nullable(audit.SourceIP), audit.CreatedAt)
	return mapWriteError(err)
}
