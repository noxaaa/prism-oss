package service

import (
	"context"

	"github.com/noxaaa/prism-oss/internal/domain"
	"github.com/noxaaa/prism-oss/internal/repo"
)

const (
	ossSingleUserMemberID = "oss-single-user-member"
	ossSingleUserRoleID   = "oss-single-user-owner"
)

func (service *ControlService) bootstrapSingleUser(ctx context.Context, repositories repo.Repositories, identity WebUserIdentity, input BootstrapInput) (SessionResult, error) {
	organizations, err := repositories.Organizations().ListOrganizations(ctx)
	if err != nil {
		return SessionResult{}, err
	}
	if len(organizations) > 0 {
		return service.loadSingleUserSession(ctx, repositories, identity, organizations[0].ID)
	}

	now := service.timestamp()
	organization := repo.OrganizationRecord{
		ID:                      service.newID(),
		Name:                    input.OrganizationName,
		Slug:                    input.OrganizationSlug,
		OwnerUserID:             identity.UserID,
		DefaultTrafficLimitMode: "TOTAL",
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := repositories.Organizations().CreateOrganization(ctx, organization); err != nil {
		return SessionResult{}, err
	}
	permissions := singleUserPermissions()
	if err := service.writeAudit(ctx, repositories, auditInput{
		OrganizationID: organization.ID,
		ActorUserID:    identity.UserID,
		Action:         "organization.bootstrap",
		ResourceType:   "ORGANIZATION",
		ResourceID:     organization.ID,
		Roles:          []string{ossSingleUserRoleID},
		Permissions:    permissions,
		SourceIP:       input.SourceIP,
	}); err != nil {
		return SessionResult{}, err
	}
	session := singleUserSession(identity, organization, []repo.OrganizationRecord{organization})
	session.Created = true
	return session, nil
}

func (service *ControlService) sessionForSingleUser(ctx context.Context, repositories repo.Repositories, identity WebUserIdentity) (SessionResult, error) {
	organizations, err := repositories.Organizations().ListOrganizations(ctx)
	if err != nil {
		return SessionResult{}, err
	}
	if len(organizations) == 0 {
		return SessionResult{User: UserPayload{ID: identity.UserID, Email: identity.Email, Name: identity.Name}}, nil
	}
	if organizations[0].OwnerUserID != identity.UserID {
		return SessionResult{}, ossOwnerRequiredError()
	}
	return singleUserSession(identity, organizations[0], organizations), nil
}

func (service *ControlService) loadSingleUserSession(ctx context.Context, repositories repo.Repositories, identity WebUserIdentity, organizationID string) (SessionResult, error) {
	organization, err := repositories.Organizations().FindOrganizationByID(ctx, organizationID)
	if err != nil {
		return SessionResult{}, err
	}
	if organization.OwnerUserID != identity.UserID {
		return SessionResult{}, ossOwnerRequiredError()
	}
	organizations, err := repositories.Organizations().ListOrganizations(ctx)
	if err != nil {
		return SessionResult{}, err
	}
	return singleUserSession(identity, organization, organizations), nil
}

func singleUserSession(identity WebUserIdentity, organization repo.OrganizationRecord, organizations []repo.OrganizationRecord) SessionResult {
	permissions := singleUserPermissions()
	scopes := []ResourceScopePayload{{
		ResourceType: string(domain.ResourceTypeNodeGroup),
		ResourceID:   "*",
		AccessLevel:  string(domain.AccessLevelManage),
	}}
	role := RolePayload{
		ID:          ossSingleUserRoleID,
		Key:         "owner",
		Name:        "Owner",
		Description: "Single-user owner",
		IsSystem:    true,
		Permissions: append([]string(nil), permissions...),
		ResourceScopes: []ResourceScopePayload{{
			ResourceType: string(domain.ResourceTypeNodeGroup),
			ResourceID:   "*",
			AccessLevel:  string(domain.AccessLevelManage),
		}},
	}
	return SessionResult{
		User:          UserPayload{ID: identity.UserID, Email: identity.Email, Name: identity.Name},
		Organization:  toOrganizationPayload(organization),
		Organizations: toOrganizationPayloads(organizations),
		Member: MemberPayload{
			ID:      ossSingleUserMemberID,
			UserID:  identity.UserID,
			Email:   identity.Email,
			Name:    identity.Name,
			Status:  MemberStatusActive,
			RoleIDs: []string{ossSingleUserRoleID},
		},
		Roles:          []RolePayload{role},
		Permissions:    permissions,
		ResourceScopes: scopes,
	}
}

func ossOwnerRequiredError() error {
	return &controlServiceError{
		Code:    "OSS_OWNER_REQUIRED",
		Message: "Only the owner can access this OSS instance.",
		Details: map[string]any{
			"edition": "oss",
		},
		Cause: ErrForbidden,
	}
}

func singleUserPermissions() []string {
	return []string{
		string(domain.PermissionAuditLogsRead),
		string(domain.PermissionMonitorsManage),
		string(domain.PermissionMonitorsRead),
		string(domain.PermissionNodesManage),
		string(domain.PermissionNodesRead),
		string(domain.PermissionOrganizationRead),
		string(domain.PermissionOrganizationUpdate),
		string(domain.PermissionQuotasManage),
		string(domain.PermissionRulesManageAll),
		string(domain.PermissionRulesManageOwn),
		string(domain.PermissionRulesReadAll),
		string(domain.PermissionRulesReadOwn),
		string(domain.PermissionTargetsManage),
		string(domain.PermissionTargetsRead),
		string(domain.PermissionTrafficReadAll),
		string(domain.PermissionTrafficReadOwn),
	}
}
