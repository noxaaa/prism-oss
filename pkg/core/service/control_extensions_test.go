package service

import (
	"context"
	"errors"
	"testing"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func TestControlServiceUsesInjectedAuthorizer(t *testing.T) {
	control := NewControlServiceWithOptions(nil, ControlServiceOptions{
		Authorizer: testAuthorizer{allowed: true},
	})
	if !control.hasPermission(InternalIdentity{}, "members.read") {
		t.Fatalf("expected injected authorizer to grant permission")
	}
}

func TestControlServiceDelegatesSessionBackend(t *testing.T) {
	control := NewControlServiceWithOptions(nil, ControlServiceOptions{
		SessionBackend: &testSessionBackend{
			result: SessionResult{User: UserPayload{ID: "user_1"}},
		},
	})
	result, err := control.SessionForWebUser(context.Background(), WebUserIdentity{UserID: "user_1"})
	if err != nil {
		t.Fatalf("session for web user: %v", err)
	}
	if result.User.ID != "user_1" {
		t.Fatalf("expected delegated session result, got %#v", result)
	}
}

func TestControlServiceRequiresOrganizationReadBeforeDelegatingInternalSession(t *testing.T) {
	backend := &testSessionBackend{
		result: SessionResult{User: UserPayload{ID: "user_1"}},
	}
	control := NewControlServiceWithOptions(nil, ControlServiceOptions{
		Authorizer:     testAuthorizer{allowed: false},
		SessionBackend: backend,
	})
	result, err := control.SessionForInternalIdentity(context.Background(), InternalIdentity{
		UserID:      "user_1",
		Permissions: []string{string(domain.PermissionNodesRead)},
	})
	if err != ErrForbidden {
		t.Fatalf("expected forbidden before delegated session lookup, got result=%#v err=%v", result, err)
	}
	if backend.internalCalls != 0 {
		t.Fatalf("expected unauthorized internal session lookup to skip backend, got %d calls", backend.internalCalls)
	}
}

func TestControlServiceDelegatesInternalSessionAfterOrganizationRead(t *testing.T) {
	backend := &testSessionBackend{
		result: SessionResult{User: UserPayload{ID: "user_1"}},
	}
	control := NewControlServiceWithOptions(nil, ControlServiceOptions{
		Authorizer:     testAuthorizer{allowed: true},
		SessionBackend: backend,
	})
	result, err := control.SessionForInternalIdentity(context.Background(), InternalIdentity{
		UserID:      "user_1",
		Permissions: []string{string(domain.PermissionOrganizationRead)},
	})
	if err != nil {
		t.Fatalf("internal session lookup: %v", err)
	}
	if result.User.ID != "user_1" || backend.internalCalls != 1 {
		t.Fatalf("expected delegated internal session, got result=%#v calls=%d", result, backend.internalCalls)
	}
}

func TestControlServiceMapsSessionBackendErrors(t *testing.T) {
	control := NewControlServiceWithOptions(nil, ControlServiceOptions{
		Authorizer: testAuthorizer{allowed: true},
		SessionBackend: &testSessionBackend{
			err: repo.ErrNotFound,
		},
	})
	for name, run := range map[string]func() (SessionResult, error){
		"bootstrap": func() (SessionResult, error) {
			return control.Bootstrap(context.Background(), WebUserIdentity{UserID: "user_1"}, BootstrapInput{})
		},
		"web": func() (SessionResult, error) {
			return control.SessionForWebUser(context.Background(), WebUserIdentity{UserID: "user_1"})
		},
		"internal": func() (SessionResult, error) {
			return control.SessionForInternalIdentity(context.Background(), InternalIdentity{
				UserID:      "user_1",
				Permissions: []string{string(domain.PermissionOrganizationRead)},
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := run(); !errors.Is(err, ErrNotFound) {
				t.Fatalf("expected mapped not found error, got %v", err)
			}
		})
	}
}

func TestControlServiceDelegatesRBACBackend(t *testing.T) {
	control := NewControlServiceWithOptions(nil, ControlServiceOptions{
		RBACBackend: testRBACBackend{
			members: []MemberPayload{{ID: "member_1"}},
		},
	})
	members, err := control.ListMembers(context.Background(), InternalIdentity{})
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 1 || members[0].ID != "member_1" {
		t.Fatalf("expected delegated members, got %#v", members)
	}
}

func TestControlServiceMapsRBACBackendErrors(t *testing.T) {
	control := NewControlServiceWithOptions(nil, ControlServiceOptions{
		RBACBackend: testRBACBackend{
			err: repo.ErrConflict,
		},
	})
	for name, run := range map[string]func() error{
		"list_members": func() error {
			_, err := control.ListMembers(context.Background(), InternalIdentity{})
			return err
		},
		"add_member": func() error {
			_, err := control.AddMember(context.Background(), InternalIdentity{}, MemberMutationInput{})
			return err
		},
		"update_member": func() error {
			_, err := control.UpdateMember(context.Background(), InternalIdentity{}, "member_1", MemberMutationInput{})
			return err
		},
		"delete_member": func() error {
			return control.DeleteMember(context.Background(), InternalIdentity{}, "member_1")
		},
		"list_roles": func() error {
			_, err := control.ListRoles(context.Background(), InternalIdentity{})
			return err
		},
		"create_role": func() error {
			_, err := control.CreateRole(context.Background(), InternalIdentity{}, RoleMutationInput{})
			return err
		},
		"update_role": func() error {
			_, err := control.UpdateRole(context.Background(), InternalIdentity{}, "role_1", RoleMutationInput{})
			return err
		},
		"delete_role": func() error {
			return control.DeleteRole(context.Background(), InternalIdentity{}, "role_1")
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(); !errors.Is(err, ErrConflict) {
				t.Fatalf("expected mapped conflict error, got %v", err)
			}
		})
	}
}

type testAuthorizer struct {
	allowed bool
}

func (authorizer testAuthorizer) HasPermission(InternalIdentity, string) bool {
	return authorizer.allowed
}

func (authorizer testAuthorizer) AllowedNodeGroupIDs(InternalIdentity, string) map[string]bool {
	return map[string]bool{}
}

func (authorizer testAuthorizer) EnsureCanDelegateRoleScopes(context.Context, repo.Repositories, InternalIdentity, []repo.ResourceScopeRecord) error {
	return nil
}

type testSessionBackend struct {
	result        SessionResult
	err           error
	internalCalls int
}

func (backend *testSessionBackend) Bootstrap(context.Context, WebUserIdentity, BootstrapInput) (SessionResult, error) {
	return backend.result, backend.err
}

func (backend *testSessionBackend) SessionForWebUser(context.Context, WebUserIdentity) (SessionResult, error) {
	return backend.result, backend.err
}

func (backend *testSessionBackend) SessionForInternalIdentity(context.Context, InternalIdentity) (SessionResult, error) {
	backend.internalCalls++
	return backend.result, backend.err
}

type testRBACBackend struct {
	members []MemberPayload
	err     error
}

func (backend testRBACBackend) ListMembers(context.Context, InternalIdentity) ([]MemberPayload, error) {
	return backend.members, backend.err
}

func (backend testRBACBackend) AddMember(context.Context, InternalIdentity, MemberMutationInput) (MemberPayload, error) {
	return MemberPayload{}, backend.err
}

func (backend testRBACBackend) UpdateMember(context.Context, InternalIdentity, string, MemberMutationInput) (MemberPayload, error) {
	return MemberPayload{}, backend.err
}

func (backend testRBACBackend) DeleteMember(context.Context, InternalIdentity, string) error {
	return backend.err
}

func (backend testRBACBackend) ListRoles(context.Context, InternalIdentity) ([]RolePayload, error) {
	return nil, backend.err
}

func (backend testRBACBackend) CreateRole(context.Context, InternalIdentity, RoleMutationInput) (RolePayload, error) {
	return RolePayload{}, backend.err
}

func (backend testRBACBackend) UpdateRole(context.Context, InternalIdentity, string, RoleMutationInput) (RolePayload, error) {
	return RolePayload{}, backend.err
}

func (backend testRBACBackend) DeleteRole(context.Context, InternalIdentity, string) error {
	return backend.err
}
