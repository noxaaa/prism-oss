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
	return service.AuthenticateAgentTokenWithMetadata(ctx, agentType, token, AgentEnrollmentMetadata{})
}

func (service *ControlService) AuthenticateAgentTokenWithMetadata(ctx context.Context, agentType string, token string, metadata AgentEnrollmentMetadata) (AgentAuthResult, error) {
	return service.authenticateAgentToken(ctx, agentType, token, true, metadata)
}

func (service *ControlService) ValidateAgentToken(ctx context.Context, agentType string, token string) (AgentAuthResult, error) {
	return service.authenticateAgentToken(ctx, agentType, token, false, AgentEnrollmentMetadata{})
}

func (service *ControlService) ValidateAgentTokenWithMetadata(ctx context.Context, agentType string, token string, metadata AgentEnrollmentMetadata) (AgentAuthResult, error) {
	return service.authenticateAgentToken(ctx, agentType, token, false, metadata)
}

func (service *ControlService) authenticateAgentToken(ctx context.Context, agentType string, token string, exchangeRegistration bool, metadata AgentEnrollmentMetadata) (AgentAuthResult, error) {
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
	var committedAuthError error
	var committedAuthErrorOrganizationID string
	var committedAuthErrorAffectedDNSRecordIDs []string
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if credential, err := repositories.AgentCredentials().FindCredentialByHash(ctx, tokenHash); err == nil {
			if credential.AgentType != agentType || credential.RevokedAt != "" {
				return ErrForbidden
			}
			if credential.ActivatedAt == "" {
				if credential.RegistrationTokenID == "" && credential.EnrollmentProfileID == "" {
					return ErrForbidden
				}
				var enrollmentProfile repo.NodeEnrollmentProfileRecord
				if credential.EnrollmentProfileID != "" {
					profile, err := service.validatePendingEnrollmentCredentialForActivation(ctx, repositories, credential, metadata)
					if err != nil {
						return err
					}
					enrollmentProfile = profile
				}
				if exchangeRegistration {
					now := service.timestamp()
					if credential.RegistrationTokenID != "" {
						if err := repositories.AgentRegistrationTokens().ClaimRegistrationToken(ctx, credential.OrganizationID, credential.RegistrationTokenID, now); err != nil {
							if errors.Is(err, repo.ErrNotFound) {
								return ErrForbidden
							}
							return err
						}
					}
					if err := repositories.AgentCredentials().ActivateCredential(ctx, credential.OrganizationID, credential.ID, now); err != nil {
						return err
					}
					if err := repositories.AgentCredentials().RevokeActiveCredentialsExcept(ctx, credential.OrganizationID, credential.AgentType, credential.AgentID, credential.ID, now); err != nil {
						return err
					}
					if enrollmentProfile.ID != "" {
						if err := service.recordNodeEnrollmentEvent(ctx, repositories, enrollmentProfile, credential.AgentID, "SUCCEEDED", "", "Node enrolled.", metadata); err != nil {
							return err
						}
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
			if errors.Is(err, repo.ErrNotFound) && agentType == "NODE" {
				enrollmentResult, found, enrollmentErr := service.authenticateNodeEnrollmentTokenInTx(ctx, repositories, tokenHash, exchangeRegistration, metadata)
				if enrollmentErr != nil {
					if authErr, ok := enrollmentAuthError(enrollmentErr); ok && authErr.commitFailure {
						committedAuthError = authErr.cause
						committedAuthErrorOrganizationID = authErr.profile.OrganizationID
						committedAuthErrorAffectedDNSRecordIDs = authErr.affectedDNSRecordIDs
						return nil
					}
					return enrollmentErr
				}
				if found {
					result = enrollmentResult
					return nil
				}
			}
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
	if committedAuthError != nil {
		if len(committedAuthErrorAffectedDNSRecordIDs) > 0 {
			service.evaluateDNSManagedRecordsBestEffort(ctx, committedAuthErrorOrganizationID, committedAuthErrorAffectedDNSRecordIDs)
		}
		return AgentAuthResult{}, mapServiceError(committedAuthError)
	}
	if authErr, ok := enrollmentAuthError(err); ok {
		service.recordNodeEnrollmentFailureBestEffort(ctx, authErr.profile, authErr.cause, authErr.metadata)
		return AgentAuthResult{}, mapServiceError(authErr.cause)
	}
	if err == nil && len(result.AffectedDNSRecordIDs) > 0 {
		service.evaluateDNSManagedRecordsBestEffort(ctx, result.OrganizationID, result.AffectedDNSRecordIDs)
	}
	return result, mapServiceError(err)
}

func (service *ControlService) FinalizeAgentRegistrationDelivery(ctx context.Context, authResult AgentAuthResult) error {
	if !authResult.RegisteredWithToken || authResult.AgentCredentialID == "" {
		return nil
	}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		now := service.timestamp()
		if authResult.RegistrationTokenID != "" {
			if err := repositories.AgentRegistrationTokens().ClaimRegistrationToken(ctx, authResult.OrganizationID, authResult.RegistrationTokenID, now); err != nil {
				if errors.Is(err, repo.ErrNotFound) {
					return ErrForbidden
				}
				return err
			}
		}
		var enrollmentProfile repo.NodeEnrollmentProfileRecord
		if authResult.EnrollmentProfileID != "" && authResult.RegistrationTokenID == "" {
			profile, err := service.validatePendingEnrollmentCredentialForActivation(ctx, repositories, repo.AgentCredentialRecord{
				ID:                  authResult.AgentCredentialID,
				OrganizationID:      authResult.OrganizationID,
				AgentType:           authResult.AgentType,
				AgentID:             authResult.AgentID,
				EnrollmentProfileID: authResult.EnrollmentProfileID,
				EnrollmentTokenHash: authResult.EnrollmentTokenHash,
			}, authResult.EnrollmentMetadata)
			if err != nil {
				return err
			}
			enrollmentProfile = profile
		}
		if err := repositories.AgentCredentials().ActivateCredential(ctx, authResult.OrganizationID, authResult.AgentCredentialID, now); err != nil {
			return err
		}
		if err := repositories.AgentCredentials().RevokeActiveCredentialsExcept(
			ctx,
			authResult.OrganizationID,
			authResult.AgentType,
			authResult.AgentID,
			authResult.AgentCredentialID,
			now,
		); err != nil {
			return err
		}
		if enrollmentProfile.ID != "" {
			if err := service.recordNodeEnrollmentEvent(ctx, repositories, enrollmentProfile, authResult.AgentID, "SUCCEEDED", "", "Node enrolled.", authResult.EnrollmentMetadata); err != nil {
				return err
			}
		}
		return nil
	})
	return mapServiceError(err)
}

func (service *ControlService) validatePendingEnrollmentCredentialForActivation(ctx context.Context, repositories repo.Repositories, credential repo.AgentCredentialRecord, metadata AgentEnrollmentMetadata) (repo.NodeEnrollmentProfileRecord, error) {
	profile, err := repositories.NodeEnrollmentProfiles().FindNodeEnrollmentProfileByTokenHashForUpdate(ctx, credential.EnrollmentTokenHash)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return repo.NodeEnrollmentProfileRecord{}, ErrForbidden
		}
		return repo.NodeEnrollmentProfileRecord{}, err
	}
	if profile.OrganizationID != credential.OrganizationID || profile.ID != credential.EnrollmentProfileID {
		return repo.NodeEnrollmentProfileRecord{}, ErrForbidden
	}
	if !profile.Enabled || strings.TrimSpace(profile.RevokedAt) != "" {
		return repo.NodeEnrollmentProfileRecord{}, ErrForbidden
	}
	if strings.TrimSpace(profile.ExpiresAt) != "" {
		expiresAt, err := time.Parse(time.RFC3339Nano, profile.ExpiresAt)
		if err != nil || !expiresAt.After(service.now()) {
			return repo.NodeEnrollmentProfileRecord{}, ErrForbidden
		}
	}
	if !remoteIPAllowed(metadata.RemoteIP, decodeJSONStringList(profile.AllowedCIDRsJSON)) {
		return repo.NodeEnrollmentProfileRecord{}, ErrForbidden
	}
	return profile, nil
}

func (service *ControlService) ReleaseAgentRegistrationCredential(ctx context.Context, authResult AgentAuthResult) error {
	if !authResult.RegisteredWithToken || authResult.AgentCredentialID == "" {
		return nil
	}
	var affectedDNSRecordIDs []string
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		now := service.timestamp()
		releasedDNSRecordIDs, _, releaseErr := service.releaseAgentRegistrationCredentialInTx(ctx, repositories, authResult, now)
		affectedDNSRecordIDs = releasedDNSRecordIDs
		return releaseErr
	})
	if err == nil && len(affectedDNSRecordIDs) > 0 {
		service.evaluateDNSManagedRecordsBestEffort(ctx, authResult.OrganizationID, affectedDNSRecordIDs)
	}
	return mapServiceError(err)
}

func (service *ControlService) releaseAgentRegistrationCredentialInTx(ctx context.Context, repositories repo.Repositories, authResult AgentAuthResult, now string) ([]string, bool, error) {
	credentialRevoked := true
	if err := repositories.AgentCredentials().RevokeCredential(ctx, authResult.OrganizationID, authResult.AgentCredentialID, now); err != nil {
		if !errors.Is(err, repo.ErrNotFound) {
			return nil, false, err
		}
		credentialRevoked = false
	}
	if authResult.EnrollmentProfileID == "" {
		return nil, credentialRevoked, nil
	}
	var affectedDNSRecordIDs []string
	if credentialRevoked && authResult.AgentID != "" {
		node, err := repositories.Nodes().FindNodeByID(ctx, authResult.OrganizationID, authResult.AgentID)
		if err != nil && !errors.Is(err, repo.ErrNotFound) {
			return nil, false, err
		}
		if err == nil {
			nodes, err := repositories.Nodes().ListNodesByOrganization(ctx, authResult.OrganizationID)
			if err != nil {
				return nil, false, err
			}
			if err := validateEnabledRulesForNodeSet(ctx, repositories, authResult.OrganizationID, removeNodeFromSet(nodes, authResult.AgentID)); err != nil {
				return nil, false, err
			}
			if err := repositories.Nodes().DeleteNode(ctx, authResult.OrganizationID, authResult.AgentID, now); err != nil && !errors.Is(err, repo.ErrNotFound) {
				return nil, false, err
			}
			affectedDNSRecordIDs, err = service.markDNSRecordsDependingOnNodeGroupsPending(ctx, repositories, authResult.OrganizationID, node.GroupIDs, now)
			if err != nil {
				return nil, false, err
			}
		}
	}
	profile, err := repositories.NodeEnrollmentProfiles().FindNodeEnrollmentProfileByID(ctx, authResult.OrganizationID, authResult.EnrollmentProfileID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return affectedDNSRecordIDs, credentialRevoked, nil
		}
		return nil, false, err
	}
	if credentialRevoked && profile.TokenHash == authResult.EnrollmentTokenHash {
		if err := repositories.NodeEnrollmentProfiles().DecrementNodeEnrollmentProfileUsedCount(ctx, authResult.OrganizationID, authResult.EnrollmentProfileID, now); err != nil && !errors.Is(err, repo.ErrNotFound) {
			return nil, false, err
		}
	}
	return affectedDNSRecordIDs, credentialRevoked, nil
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
	releaseVersion := service.nodeAgentInstallReleaseVersion()
	if agentType == "MONITOR" {
		return fmt.Sprintf(
			"(tmp=$(mktemp) && curl -fsSL %s -o \"$tmp\" && sudo env APP_NAME=%s sh \"$tmp\" --version %s --control-url %s --registration-token %s --credential-file agent-credential.json; status=$?; rm -f \"${tmp:-}\"; exit \"$status\")",
			shellQuote(monitorAgentInstallerURL(releaseVersion)),
			shellQuote(service.appName),
			shellQuote(releaseVersion),
			shellQuote(service.controlPlaneURL),
			shellQuote(token),
		)
	}
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

func monitorAgentInstallerURL(version string) string {
	if version == "" || version == "latest" {
		return "https://github.com/noxaaa/prism-oss/releases/latest/download/install-monitor-agent.sh"
	}
	return "https://github.com/noxaaa/prism-oss/releases/download/" + url.PathEscape(version) + "/install-monitor-agent.sh"
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
