package repo

import "context"

func (store *PostgresStore) FindUserByID(ctx context.Context, userID string) (UserRecord, error) {
	row := store.db.QueryRowContext(ctx, `SELECT id, email, name FROM "user" WHERE id = ?`, userID)
	return scanUser(row)
}

func (store *PostgresStore) FindUserByEmail(ctx context.Context, email string) (UserRecord, error) {
	row := store.db.QueryRowContext(ctx, `SELECT id, email, name FROM "user" WHERE lower(email) = lower(?)`, email)
	return scanUser(row)
}

func (store *PostgresStore) CountOrganizations(ctx context.Context) (int, error) {
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM organizations WHERE deleted_at IS NULL`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (store *PostgresStore) CreateOrganization(ctx context.Context, organization OrganizationRecord) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO organizations (
			id, name, slug, owner_user_id, default_rule_limit, default_traffic_limit_bytes, default_traffic_limit_mode, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, organization.ID, organization.Name, organization.Slug, organization.OwnerUserID, organization.DefaultRuleLimit, organization.DefaultTrafficLimitBytes, organization.DefaultTrafficLimitMode, organization.CreatedAt, organization.UpdatedAt)
	return mapWriteError(err)
}

func (store *PostgresStore) UpdateOrganization(ctx context.Context, organization OrganizationRecord) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE organizations
		SET name = ?, slug = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`, organization.Name, organization.Slug, organization.UpdatedAt, organization.ID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) ListOrganizations(ctx context.Context) ([]OrganizationRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, name, slug, coalesce(owner_user_id, ''), default_rule_limit, default_traffic_limit_bytes, default_traffic_limit_mode, created_at, updated_at, coalesce(deleted_at::text, '')
		FROM organizations
		WHERE deleted_at IS NULL
		ORDER BY created_at, id
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	organizations := make([]OrganizationRecord, 0)
	for rows.Next() {
		organization, err := scanOrganizationRows(rows)
		if err != nil {
			return nil, err
		}
		organizations = append(organizations, organization)
	}
	return organizations, rows.Err()
}

func (store *PostgresStore) ListOrganizationsByUserID(ctx context.Context, userID string) ([]OrganizationRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT organizations.id, organizations.name, organizations.slug,
		       coalesce(organizations.owner_user_id, ''),
		       organizations.default_rule_limit, organizations.default_traffic_limit_bytes, organizations.default_traffic_limit_mode,
		       organizations.created_at, organizations.updated_at, coalesce(organizations.deleted_at::text, '')
		FROM organizations
		JOIN organization_members ON organization_members.organization_id = organizations.id
		WHERE organization_members.user_id = ?
		  AND organization_members.status = 'ACTIVE'
		  AND organizations.deleted_at IS NULL
		ORDER BY organizations.created_at, organizations.id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	organizations := make([]OrganizationRecord, 0)
	for rows.Next() {
		organization, err := scanOrganizationRows(rows)
		if err != nil {
			return nil, err
		}
		organizations = append(organizations, organization)
	}
	return organizations, rows.Err()
}

func (store *PostgresStore) FindOrganizationByID(ctx context.Context, organizationID string) (OrganizationRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT id, name, slug, coalesce(owner_user_id, ''), default_rule_limit, default_traffic_limit_bytes, default_traffic_limit_mode, created_at, updated_at, coalesce(deleted_at::text, '')
		FROM organizations
		WHERE id = ? AND deleted_at IS NULL
	`, organizationID)
	return scanOrganization(row)
}

func (store *PostgresStore) FindMemberByUserAndOrganization(ctx context.Context, organizationID string, userID string) (MemberRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT organization_members.id, organization_members.organization_id, organization_members.user_id,
		       "user".email, "user".name, organization_members.status,
		       organization_members.created_at, organization_members.updated_at
		FROM organization_members
		JOIN "user" ON "user".id = organization_members.user_id
		WHERE organization_members.organization_id = ? AND organization_members.user_id = ?
	`, organizationID, userID)
	return scanMember(row)
}

func (store *PostgresStore) ListMembersByOrganization(ctx context.Context, organizationID string) ([]MemberRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT organization_members.id, organization_members.organization_id, organization_members.user_id,
		       "user".email, "user".name, organization_members.status,
		       organization_members.created_at, organization_members.updated_at
		FROM organization_members
		JOIN "user" ON "user".id = organization_members.user_id
		WHERE organization_members.organization_id = ?
		ORDER BY organization_members.created_at, organization_members.id
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	members := make([]MemberRecord, 0)
	for rows.Next() {
		member, err := scanMemberRows(rows)
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func (store *PostgresStore) CreateMember(ctx context.Context, member MemberRecord) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO organization_members (id, organization_id, user_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, member.ID, member.OrganizationID, member.UserID, member.Status, member.CreatedAt, member.UpdatedAt)
	return mapWriteError(err)
}

func (store *PostgresStore) UpdateMemberStatus(ctx context.Context, organizationID string, memberID string, status string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE organization_members
		SET status = ?, updated_at = clock_timestamp()
		WHERE organization_id = ? AND id = ?
	`, status, organizationID, memberID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) DeleteMember(ctx context.Context, organizationID string, memberID string) error {
	result, err := store.db.ExecContext(ctx, `
		DELETE FROM organization_members
		WHERE organization_id = ? AND id = ?
	`, organizationID, memberID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) ListRolesByOrganization(ctx context.Context, organizationID string) ([]RoleRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, key, name, description, is_system, created_at, updated_at, coalesce(deleted_at::text, '')
		FROM roles
		WHERE organization_id = ? AND deleted_at IS NULL
		ORDER BY key
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	roles := make([]RoleRecord, 0)
	for rows.Next() {
		role, err := scanRoleRows(rows)
		if err != nil {
			return nil, err
		}
		if err := store.loadRoleDetails(ctx, &role); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (store *PostgresStore) FindRoleByID(ctx context.Context, organizationID string, roleID string) (RoleRecord, error) {
	row := store.db.QueryRowContext(ctx, `
		SELECT id, organization_id, key, name, description, is_system, created_at, updated_at, coalesce(deleted_at::text, '')
		FROM roles
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, organizationID, roleID)
	role, err := scanRole(row)
	if err != nil {
		return RoleRecord{}, err
	}
	if err := store.loadRoleDetails(ctx, &role); err != nil {
		return RoleRecord{}, err
	}
	return role, nil
}

func (store *PostgresStore) CreateRole(ctx context.Context, role RoleRecord) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO roles (id, organization_id, key, name, description, is_system, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, role.ID, role.OrganizationID, role.Key, role.Name, role.Description, boolToDB(role.IsSystem), role.CreatedAt, role.UpdatedAt)
	return mapWriteError(err)
}

func (store *PostgresStore) UpdateRole(ctx context.Context, role RoleRecord) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE roles
		SET name = ?, description = ?, updated_at = ?
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, role.Name, role.Description, role.UpdatedAt, role.OrganizationID, role.ID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) DeleteRole(ctx context.Context, organizationID string, roleID string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE roles
		SET deleted_at = clock_timestamp()
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
	`, organizationID, roleID)
	if err != nil {
		return mapWriteError(err)
	}
	return requireAffected(result)
}

func (store *PostgresStore) ReplacePermissions(ctx context.Context, organizationID string, roleID string, permissions []string, now string, nextID func() string) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM role_permissions WHERE organization_id = ? AND role_id = ?`, organizationID, roleID); err != nil {
		return mapWriteError(err)
	}
	for _, permission := range permissions {
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO role_permissions (id, organization_id, role_id, permission, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, nextID(), organizationID, roleID, permission, now); err != nil {
			return mapWriteError(err)
		}
	}
	return nil
}

func (store *PostgresStore) ReplaceResourceScopes(ctx context.Context, organizationID string, roleID string, scopes []ResourceScopeRecord, now string, nextID func() string) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM role_resource_scopes WHERE organization_id = ? AND role_id = ?`, organizationID, roleID); err != nil {
		return mapWriteError(err)
	}
	for _, scope := range scopes {
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO role_resource_scopes (id, organization_id, role_id, resource_type, resource_id, access_level, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, nextID(), organizationID, roleID, scope.ResourceType, scope.ResourceID, scope.AccessLevel, now); err != nil {
			return mapWriteError(err)
		}
	}
	return nil
}

func (store *PostgresStore) ReplaceMemberRoles(ctx context.Context, organizationID string, memberID string, roleIDs []string, now string, nextID func() string) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM member_roles WHERE organization_id = ? AND member_id = ?`, organizationID, memberID); err != nil {
		return mapWriteError(err)
	}
	for _, roleID := range roleIDs {
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO member_roles (id, organization_id, member_id, role_id, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, nextID(), organizationID, memberID, roleID, now); err != nil {
			return mapWriteError(err)
		}
	}
	return nil
}

func (store *PostgresStore) ListForMember(ctx context.Context, organizationID string, memberID string) ([]RoleRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT roles.id, roles.organization_id, roles.key, roles.name, roles.description, roles.is_system,
		       roles.created_at, roles.updated_at, coalesce(roles.deleted_at::text, '')
		FROM roles
		JOIN member_roles ON member_roles.role_id = roles.id AND member_roles.organization_id = roles.organization_id
		WHERE member_roles.organization_id = ? AND member_roles.member_id = ? AND roles.deleted_at IS NULL
		ORDER BY roles.key
	`, organizationID, memberID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	roles := make([]RoleRecord, 0)
	for rows.Next() {
		role, err := scanRoleRows(rows)
		if err != nil {
			return nil, err
		}
		if err := store.loadRoleDetails(ctx, &role); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (store *PostgresStore) loadRoleDetails(ctx context.Context, role *RoleRecord) error {
	permissions, err := store.listRolePermissions(ctx, role.OrganizationID, role.ID)
	if err != nil {
		return err
	}
	scopes, err := store.listRoleScopes(ctx, role.OrganizationID, role.ID)
	if err != nil {
		return err
	}
	role.Permissions = permissions
	role.ResourceScopes = scopes
	return nil
}

func (store *PostgresStore) listRolePermissions(ctx context.Context, organizationID string, roleID string) ([]string, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT permission
		FROM role_permissions
		WHERE organization_id = ? AND role_id = ?
		ORDER BY permission
	`, organizationID, roleID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	permissions := make([]string, 0)
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}
	return permissions, rows.Err()
}

func (store *PostgresStore) listRoleScopes(ctx context.Context, organizationID string, roleID string) ([]ResourceScopeRecord, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, organization_id, role_id, resource_type, resource_id, access_level, created_at
		FROM role_resource_scopes
		WHERE organization_id = ? AND role_id = ?
		ORDER BY resource_type, resource_id, access_level
	`, organizationID, roleID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	scopes := make([]ResourceScopeRecord, 0)
	for rows.Next() {
		var scope ResourceScopeRecord
		if err := rows.Scan(&scope.ID, &scope.OrganizationID, &scope.RoleID, &scope.ResourceType, &scope.ResourceID, &scope.AccessLevel, &scope.CreatedAt); err != nil {
			return nil, err
		}
		scopes = append(scopes, scope)
	}
	return scopes, rows.Err()
}
