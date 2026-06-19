package service

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func (service *ControlService) Bootstrap(ctx context.Context, identity WebUserIdentity, input BootstrapInput) (SessionResult, error) {
	var result SessionResult
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		session, err := service.bootstrapSingleUser(ctx, repositories, identity, input)
		if err != nil {
			return err
		}
		result = session
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) SessionForWebUser(ctx context.Context, identity WebUserIdentity) (SessionResult, error) {
	var result SessionResult
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		session, err := service.sessionForSingleUser(ctx, repositories, identity)
		if err != nil {
			return err
		}
		result = session
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) SessionForInternalIdentity(ctx context.Context, identity InternalIdentity) (SessionResult, error) {
	if !service.hasPermission(identity, string(domain.PermissionOrganizationRead)) {
		return SessionResult{}, ErrForbidden
	}
	var result SessionResult
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		user, err := repositories.Users().FindUserByID(ctx, identity.UserID)
		if err != nil {
			return err
		}
		session, err := service.loadSingleUserSession(ctx, repositories, WebUserIdentity{UserID: user.ID, Email: user.Email, Name: user.Name}, identity.OrganizationID)
		if err != nil {
			return err
		}
		result = session
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) UpdateOrganization(ctx context.Context, identity InternalIdentity, input OrganizationUpdateInput) (OrganizationPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionOrganizationUpdate)) {
		return OrganizationPayload{}, ErrForbidden
	}
	var result OrganizationPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		organization, err := repositories.Organizations().FindOrganizationByID(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		organization.Name = input.Name
		organization.Slug = input.Slug
		organization.UpdatedAt = service.timestamp()
		if err := repositories.Organizations().UpdateOrganization(ctx, organization); err != nil {
			return err
		}
		result = toOrganizationPayload(organization)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "organization.update", "ORGANIZATION", organization.ID, ""))
	})
	return result, mapServiceError(err)
}

type auditInput struct {
	OrganizationID string
	ActorUserID    string
	Action         string
	ResourceType   string
	ResourceID     string
	Roles          []string
	Permissions    []string
	SourceIP       string
}

func (service *ControlService) auditForIdentity(identity InternalIdentity, action string, resourceType string, resourceID string, sourceIP string) auditInput {
	if sourceIP == "" {
		sourceIP = identity.SourceIP
	}
	return auditInput{
		OrganizationID: identity.OrganizationID,
		ActorUserID:    identity.UserID,
		Action:         action,
		ResourceType:   resourceType,
		ResourceID:     resourceID,
		Roles:          identity.Roles,
		Permissions:    identity.Permissions,
		SourceIP:       sourceIP,
	}
}

func (service *ControlService) writeAudit(ctx context.Context, repositories repo.Repositories, input auditInput) error {
	rolesJSON, err := json.Marshal(input.Roles)
	if err != nil {
		return err
	}
	permissionsJSON, err := json.Marshal(input.Permissions)
	if err != nil {
		return err
	}
	return repositories.AuditLogs().CreateAuditLog(ctx, repo.AuditLogRecord{
		ID:                   service.newID(),
		OrganizationID:       input.OrganizationID,
		ActorUserID:          input.ActorUserID,
		ActorRolesJSON:       string(rolesJSON),
		ActorPermissionsJSON: string(permissionsJSON),
		Action:               input.Action,
		ResourceType:         input.ResourceType,
		ResourceID:           input.ResourceID,
		Result:               "SUCCESS",
		MetadataJSON:         "{}",
		SourceIP:             input.SourceIP,
		CreatedAt:            service.timestamp(),
	})
}

func sameStringSet(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	leftCopy := append([]string(nil), left...)
	rightCopy := append([]string(nil), right...)
	sort.Strings(leftCopy)
	sort.Strings(rightCopy)
	for index := range leftCopy {
		if leftCopy[index] != rightCopy[index] {
			return false
		}
	}
	return true
}

func toOrganizationPayload(organization repo.OrganizationRecord) OrganizationPayload {
	return OrganizationPayload{ID: organization.ID, Name: organization.Name, Slug: organization.Slug}
}

func toOrganizationPayloads(organizations []repo.OrganizationRecord) []OrganizationPayload {
	payloads := make([]OrganizationPayload, 0, len(organizations))
	for _, organization := range organizations {
		payloads = append(payloads, toOrganizationPayload(organization))
	}
	return payloads
}
