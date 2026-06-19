package service

import (
	"context"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
	"github.com/noxaaa/prism-oss/pkg/edition"
)

type singleUserAuthorizer struct{}

func defaultControlEdition() edition.Provider {
	return edition.OSSProvider()
}

func defaultControlAuthorizer() controlAuthorizer {
	return singleUserAuthorizer{}
}

func (singleUserAuthorizer) HasPermission(identity InternalIdentity, permission string) bool {
	return stringSliceHas(singleUserPermissions(), permission)
}

func (singleUserAuthorizer) AllowedNodeGroupIDs(identity InternalIdentity, requestedAccess string) map[string]bool {
	switch requestedAccess {
	case string(domain.AccessLevelUse), string(domain.AccessLevelManage):
		return map[string]bool{"*": true}
	default:
		return map[string]bool{}
	}
}

func (singleUserAuthorizer) EnsureCanDelegateRoleScopes(context.Context, repo.Repositories, InternalIdentity, []repo.ResourceScopeRecord) error {
	return ErrForbidden
}
