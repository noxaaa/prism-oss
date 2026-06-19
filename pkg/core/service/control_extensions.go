package service

import "context"

type SessionBackend interface {
	Bootstrap(ctx context.Context, identity WebUserIdentity, input BootstrapInput) (SessionResult, error)
	SessionForWebUser(ctx context.Context, identity WebUserIdentity) (SessionResult, error)
	SessionForInternalIdentity(ctx context.Context, identity InternalIdentity) (SessionResult, error)
}

type RBACBackend interface {
	ListMembers(ctx context.Context, identity InternalIdentity) ([]MemberPayload, error)
	AddMember(ctx context.Context, identity InternalIdentity, input MemberMutationInput) (MemberPayload, error)
	UpdateMember(ctx context.Context, identity InternalIdentity, memberID string, input MemberMutationInput) (MemberPayload, error)
	DeleteMember(ctx context.Context, identity InternalIdentity, memberID string) error
	ListRoles(ctx context.Context, identity InternalIdentity) ([]RolePayload, error)
	CreateRole(ctx context.Context, identity InternalIdentity, input RoleMutationInput) (RolePayload, error)
	UpdateRole(ctx context.Context, identity InternalIdentity, roleID string, input RoleMutationInput) (RolePayload, error)
	DeleteRole(ctx context.Context, identity InternalIdentity, roleID string) error
}
