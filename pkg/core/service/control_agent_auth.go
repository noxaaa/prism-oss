package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
	"github.com/noxaaa/prism-oss/pkg/edition"
)

func (service *ControlService) ListRegistrationTokens(ctx context.Context, identity InternalIdentity, agentType string, agentID string) ([]RegistrationTokenPayload, error) {
	var result []RegistrationTokenPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if err := service.ensureCanManageAgent(ctx, repositories, identity, agentType, agentID); err != nil {
			return err
		}
		tokens, err := repositories.AgentRegistrationTokens().ListRegistrationTokens(ctx, identity.OrganizationID, agentType, agentID)
		if err != nil {
			return err
		}
		result = toRegistrationTokenPayloads(tokens)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) CreateRegistrationToken(ctx context.Context, identity InternalIdentity, agentType string, agentID string, input RegistrationTokenInput) (RegistrationTokenPayload, error) {
	if len(service.agentTokenSigningSecret) == 0 || service.appName == "" || service.controlPlaneURL == "" {
		return RegistrationTokenPayload{}, ErrInvalidInput
	}
	ttlHours := input.TTLHours
	if ttlHours == 0 {
		ttlHours = 24
	}
	if ttlHours < 1 || ttlHours > 24*7 {
		return RegistrationTokenPayload{}, ErrInvalidInput
	}
	plaintext, err := randomRegistrationToken()
	if err != nil {
		return RegistrationTokenPayload{}, err
	}
	tokenHash := hmacTokenHash(service.agentTokenSigningSecret, plaintext)
	var result RegistrationTokenPayload
	err = service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if err := service.ensureCanManageAgent(ctx, repositories, identity, agentType, agentID); err != nil {
			return err
		}
		now := service.timestamp()
		expiresAt := service.now().UTC().Add(time.Duration(ttlHours) * time.Hour).Format(time.RFC3339Nano)
		if err := repositories.AgentRegistrationTokens().RevokeActiveUnusedRegistrationTokens(ctx, identity.OrganizationID, agentType, agentID, now); err != nil {
			return err
		}
		record := repo.AgentRegistrationTokenRecord{
			ID:              service.newID(),
			OrganizationID:  identity.OrganizationID,
			AgentType:       agentType,
			AgentID:         agentID,
			TokenHash:       tokenHash,
			ExpiresAt:       expiresAt,
			CreatedAt:       now,
			CreatedByUserID: identity.UserID,
		}
		if err := repositories.AgentRegistrationTokens().CreateRegistrationToken(ctx, record); err != nil {
			return err
		}
		result = toRegistrationTokenPayload(record)
		result.Token = plaintext
		result.InstallCommand = service.installCommand(agentType, plaintext)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, strings.ToLower(agentType)+"_registration_tokens.create", "AGENT_REGISTRATION_TOKEN", record.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) RevokeRegistrationToken(ctx context.Context, identity InternalIdentity, agentType string, agentID string, tokenID string) error {
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if err := service.ensureCanManageAgent(ctx, repositories, identity, agentType, agentID); err != nil {
			return err
		}
		now := service.timestamp()
		if err := repositories.AgentRegistrationTokens().RevokeRegistrationToken(ctx, identity.OrganizationID, agentType, agentID, tokenID, now); err != nil {
			return err
		}
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, strings.ToLower(agentType)+"_registration_tokens.revoke", "AGENT_REGISTRATION_TOKEN", tokenID, ""))
	})
	return mapServiceError(err)
}

func (service *ControlService) AuthenticateAgentToken(ctx context.Context, agentType string, token string) (AgentAuthResult, error) {
	return service.authenticateAgentToken(ctx, agentType, token, true)
}

func (service *ControlService) ValidateAgentToken(ctx context.Context, agentType string, token string) (AgentAuthResult, error) {
	return service.authenticateAgentToken(ctx, agentType, token, false)
}

func (service *ControlService) authenticateAgentToken(ctx context.Context, agentType string, token string, exchangeRegistration bool) (AgentAuthResult, error) {
	agentType = strings.ToUpper(strings.TrimSpace(agentType))
	token = strings.TrimSpace(token)
	if agentType != "NODE" && agentType != "MONITOR" {
		return AgentAuthResult{}, ErrInvalidInput
	}
	if agentType == "MONITOR" && !service.edition.Has(edition.CapabilityMonitors) {
		return AgentAuthResult{}, ErrForbidden
	}
	if token == "" || len(service.agentTokenSigningSecret) == 0 {
		return AgentAuthResult{}, ErrForbidden
	}
	tokenHash := hmacTokenHash(service.agentTokenSigningSecret, token)

	var result AgentAuthResult
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if credential, err := repositories.AgentCredentials().FindCredentialByHash(ctx, tokenHash); err == nil {
			if credential.AgentType != agentType || credential.RevokedAt != "" {
				return ErrForbidden
			}
			if credential.ActivatedAt == "" {
				if credential.RegistrationTokenID == "" {
					return ErrForbidden
				}
				if exchangeRegistration {
					now := service.timestamp()
					if err := repositories.AgentRegistrationTokens().ClaimRegistrationToken(ctx, credential.OrganizationID, credential.RegistrationTokenID, now); err != nil {
						if errors.Is(err, repo.ErrNotFound) {
							return ErrForbidden
						}
						return err
					}
					if err := repositories.AgentCredentials().ActivateCredential(ctx, credential.OrganizationID, credential.ID, now); err != nil {
						return err
					}
					if err := repositories.AgentCredentials().RevokeActiveCredentialsExcept(ctx, credential.OrganizationID, credential.AgentType, credential.AgentID, credential.ID, now); err != nil {
						return err
					}
				}
			}
			result = AgentAuthResult{
				OrganizationID: credential.OrganizationID,
				AgentType:      credential.AgentType,
				AgentID:        credential.AgentID,
			}
			return nil
		} else if !errors.Is(err, repo.ErrNotFound) {
			return err
		}

		registration, err := repositories.AgentRegistrationTokens().FindRegistrationTokenByHash(ctx, tokenHash)
		if err != nil {
			return err
		}
		if registration.AgentType != agentType || registration.UsedAt != "" || registration.RevokedAt != "" {
			return ErrForbidden
		}
		expiresAt, err := time.Parse(time.RFC3339Nano, registration.ExpiresAt)
		if err != nil || !expiresAt.After(service.now()) {
			return ErrForbidden
		}
		pendingCredential, pendingErr := repositories.AgentCredentials().FindPendingCredentialByRegistrationToken(ctx, registration.OrganizationID, registration.ID)
		pendingCredentialIsStale := false
		if pendingErr == nil {
			pendingCredentialIsStale = service.pendingCredentialIsStale(pendingCredential)
			if !pendingCredentialIsStale {
				return ErrForbidden
			}
		} else if !errors.Is(pendingErr, repo.ErrNotFound) {
			return pendingErr
		}
		if !exchangeRegistration {
			result = AgentAuthResult{
				OrganizationID:      registration.OrganizationID,
				AgentType:           registration.AgentType,
				AgentID:             registration.AgentID,
				RegisteredWithToken: true,
			}
			return nil
		}
		now := service.timestamp()
		if pendingErr == nil && pendingCredentialIsStale {
			if err := repositories.AgentCredentials().RevokeCredential(ctx, registration.OrganizationID, pendingCredential.ID, now); err != nil {
				return err
			}
		}
		plaintext, err := randomRegistrationToken()
		if err != nil {
			return err
		}
		credential := repo.AgentCredentialRecord{
			ID:                  service.newID(),
			OrganizationID:      registration.OrganizationID,
			AgentType:           registration.AgentType,
			AgentID:             registration.AgentID,
			CredentialHash:      hmacTokenHash(service.agentTokenSigningSecret, plaintext),
			RegistrationTokenID: registration.ID,
			CreatedAt:           now,
		}
		if err := repositories.AgentCredentials().CreateCredential(ctx, credential); err != nil {
			return err
		}
		result = AgentAuthResult{
			OrganizationID:          registration.OrganizationID,
			AgentType:               registration.AgentType,
			AgentID:                 registration.AgentID,
			RegisteredWithToken:     true,
			RegistrationTokenID:     registration.ID,
			AgentCredentialID:       credential.ID,
			AgentCredential:         plaintext,
			AgentCredentialFileHint: "agent-credential.json",
		}
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) FinalizeAgentRegistrationDelivery(ctx context.Context, authResult AgentAuthResult) error {
	if !authResult.RegisteredWithToken || authResult.RegistrationTokenID == "" || authResult.AgentCredentialID == "" {
		return nil
	}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		now := service.timestamp()
		if err := repositories.AgentRegistrationTokens().ClaimRegistrationToken(ctx, authResult.OrganizationID, authResult.RegistrationTokenID, now); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return ErrForbidden
			}
			return err
		}
		if err := repositories.AgentCredentials().ActivateCredential(ctx, authResult.OrganizationID, authResult.AgentCredentialID, now); err != nil {
			return err
		}
		return repositories.AgentCredentials().RevokeActiveCredentialsExcept(
			ctx,
			authResult.OrganizationID,
			authResult.AgentType,
			authResult.AgentID,
			authResult.AgentCredentialID,
			now,
		)
	})
	return mapServiceError(err)
}

func (service *ControlService) ReleaseAgentRegistrationCredential(ctx context.Context, authResult AgentAuthResult) error {
	if !authResult.RegisteredWithToken || authResult.RegistrationTokenID == "" || authResult.AgentCredentialID == "" {
		return nil
	}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		now := service.timestamp()
		if err := repositories.AgentCredentials().RevokeCredential(ctx, authResult.OrganizationID, authResult.AgentCredentialID, now); err != nil {
			if !errors.Is(err, repo.ErrNotFound) {
				return err
			}
		}
		return nil
	})
	return mapServiceError(err)
}

func (service *ControlService) pendingCredentialIsStale(credential repo.AgentCredentialRecord) bool {
	createdAt, err := time.Parse(time.RFC3339Nano, credential.CreatedAt)
	if err != nil {
		return false
	}
	return !createdAt.Add(agentPendingCredentialTTL).After(service.now())
}

func (service *ControlService) ensureCanManageAgent(ctx context.Context, repositories repo.Repositories, identity InternalIdentity, agentType string, agentID string) error {
	switch agentType {
	case "NODE":
		if !service.hasPermission(identity, string(domain.PermissionNodesManage)) {
			return ErrForbidden
		}
		node, err := repositories.Nodes().FindNodeByID(ctx, identity.OrganizationID, agentID)
		if err != nil {
			return err
		}
		return service.ensureCanManageNodeGroups(identity, node.GroupIDs)
	case "MONITOR":
		if !service.hasPermission(identity, string(domain.PermissionMonitorsManage)) {
			return ErrForbidden
		}
		_, err := repositories.Monitors().FindMonitorByID(ctx, identity.OrganizationID, agentID)
		return err
	default:
		return ErrInvalidInput
	}
}

func randomRegistrationToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func hmacTokenHash(secret []byte, token string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(token))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (service *ControlService) installCommand(agentType string, token string) string {
	if agentType == "MONITOR" {
		return fmt.Sprintf("APP_NAME=%s ./monitor-agent install --control-url %s --registration-token %s --credential-file agent-credential.json", shellQuote(service.appName), shellQuote(service.controlPlaneURL), shellQuote(token))
	}
	releaseVersion := service.nodeAgentInstallReleaseVersion()
	return fmt.Sprintf(
		"(tmp=$(mktemp) && curl -fsSL %s -o \"$tmp\" && sudo env APP_NAME=%s sh \"$tmp\" --version %s --control-url %s --registration-token %s --credential-file agent-credential.json; status=$?; rm -f \"${tmp:-}\"; exit \"$status\")",
		shellQuote(nodeAgentInstallerURL(releaseVersion)),
		shellQuote(service.appName),
		shellQuote(releaseVersion),
		shellQuote(service.controlPlaneURL),
		shellQuote(token),
	)
}

func (service *ControlService) nodeAgentReleaseVersion() string {
	if service.agentReleaseVersion == "" {
		return "latest"
	}
	return service.agentReleaseVersion
}

func (service *ControlService) nodeAgentInstallReleaseVersion() string {
	targetVersion := service.targetAgentVersion()
	if agentUpdateTargetIsConcrete(targetVersion) {
		return targetVersion
	}
	return service.nodeAgentReleaseVersion()
}

func nodeAgentInstallerURL(version string) string {
	if version == "" || version == "latest" {
		return "https://github.com/noxaaa/prism-oss/releases/latest/download/install-node-agent.sh"
	}
	return "https://github.com/noxaaa/prism-oss/releases/download/" + url.PathEscape(version) + "/install-node-agent.sh"
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
