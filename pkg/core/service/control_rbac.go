package service

import "context"

func (service *ControlService) ListMembers(ctx context.Context, identity InternalIdentity) ([]MemberPayload, error) {
	if service.rbacBackend == nil {
		return nil, ErrForbidden
	}
	result, err := service.rbacBackend.ListMembers(ctx, identity)
	return result, mapServiceError(err)
}

func (service *ControlService) AddMember(ctx context.Context, identity InternalIdentity, input MemberMutationInput) (MemberPayload, error) {
	if service.rbacBackend == nil {
		return MemberPayload{}, ErrForbidden
	}
	result, err := service.rbacBackend.AddMember(ctx, identity, input)
	return result, mapServiceError(err)
}

func (service *ControlService) UpdateMember(ctx context.Context, identity InternalIdentity, memberID string, input MemberMutationInput) (MemberPayload, error) {
	if service.rbacBackend == nil {
		return MemberPayload{}, ErrForbidden
	}
	result, err := service.rbacBackend.UpdateMember(ctx, identity, memberID, input)
	return result, mapServiceError(err)
}

func (service *ControlService) DeleteMember(ctx context.Context, identity InternalIdentity, memberID string) error {
	if service.rbacBackend == nil {
		return ErrForbidden
	}
	return mapServiceError(service.rbacBackend.DeleteMember(ctx, identity, memberID))
}

func (service *ControlService) ListRoles(ctx context.Context, identity InternalIdentity) ([]RolePayload, error) {
	if service.rbacBackend == nil {
		return nil, ErrForbidden
	}
	result, err := service.rbacBackend.ListRoles(ctx, identity)
	return result, mapServiceError(err)
}

func (service *ControlService) CreateRole(ctx context.Context, identity InternalIdentity, input RoleMutationInput) (RolePayload, error) {
	if service.rbacBackend == nil {
		return RolePayload{}, ErrForbidden
	}
	result, err := service.rbacBackend.CreateRole(ctx, identity, input)
	return result, mapServiceError(err)
}

func (service *ControlService) UpdateRole(ctx context.Context, identity InternalIdentity, roleID string, input RoleMutationInput) (RolePayload, error) {
	if service.rbacBackend == nil {
		return RolePayload{}, ErrForbidden
	}
	result, err := service.rbacBackend.UpdateRole(ctx, identity, roleID, input)
	return result, mapServiceError(err)
}

func (service *ControlService) DeleteRole(ctx context.Context, identity InternalIdentity, roleID string) error {
	if service.rbacBackend == nil {
		return ErrForbidden
	}
	return mapServiceError(service.rbacBackend.DeleteRole(ctx, identity, roleID))
}
