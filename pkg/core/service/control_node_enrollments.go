package service

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

type AgentEnrollmentMetadata struct {
	Hostname string
	RemoteIP string
}

type nodeEnrollmentAuthError struct {
	profile              repo.NodeEnrollmentProfileRecord
	metadata             AgentEnrollmentMetadata
	cause                error
	commitFailure        bool
	affectedDNSRecordIDs []string
}

func (err nodeEnrollmentAuthError) Error() string {
	return err.cause.Error()
}

func (err nodeEnrollmentAuthError) Unwrap() error {
	return err.cause
}

func (service *ControlService) ListNodeEnrollmentProfiles(ctx context.Context, identity InternalIdentity) ([]NodeEnrollmentProfilePayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionNodesRead)) {
		return nil, ErrForbidden
	}
	var result []NodeEnrollmentProfilePayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		profiles, err := repositories.NodeEnrollmentProfiles().ListNodeEnrollmentProfiles(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		result = make([]NodeEnrollmentProfilePayload, 0, len(profiles))
		for _, profile := range profiles {
			if err := service.ensureCanUseNodeGroups(identity, decodeJSONStringList(profile.GroupIDsJSON)); err != nil {
				continue
			}
			result = append(result, service.toNodeEnrollmentProfilePayload(profile, ""))
		}
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) GetNodeEnrollmentProfile(ctx context.Context, identity InternalIdentity, profileID string) (NodeEnrollmentProfilePayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionNodesRead)) {
		return NodeEnrollmentProfilePayload{}, ErrForbidden
	}
	var result NodeEnrollmentProfilePayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		profile, err := repositories.NodeEnrollmentProfiles().FindNodeEnrollmentProfileByID(ctx, identity.OrganizationID, profileID)
		if err != nil {
			return err
		}
		if err := service.ensureCanUseNodeGroups(identity, decodeJSONStringList(profile.GroupIDsJSON)); err != nil {
			return err
		}
		result = service.toNodeEnrollmentProfilePayload(profile, "")
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) CreateNodeEnrollmentProfile(ctx context.Context, identity InternalIdentity, input NodeEnrollmentProfileMutationInput) (NodeEnrollmentProfilePayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionNodesManage)) {
		return NodeEnrollmentProfilePayload{}, ErrForbidden
	}
	if err := service.ensureCanIssueNodeEnrollmentToken(); err != nil {
		return NodeEnrollmentProfilePayload{}, err
	}
	input, err := normalizeNodeEnrollmentProfileInput(input)
	if err != nil {
		return NodeEnrollmentProfilePayload{}, err
	}
	if err := service.ensureCanManageNodeGroups(identity, input.GroupIDs); err != nil {
		return NodeEnrollmentProfilePayload{}, err
	}
	plaintext, err := randomRegistrationToken()
	if err != nil {
		return NodeEnrollmentProfilePayload{}, err
	}
	var result NodeEnrollmentProfilePayload
	err = service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if err := ensureNodeGroupsExist(ctx, repositories, identity.OrganizationID, input.GroupIDs); err != nil {
			return err
		}
		now := service.timestamp()
		profile, err := nodeEnrollmentProfileRecordFromInput(input, repo.NodeEnrollmentProfileRecord{
			ID:              service.newID(),
			OrganizationID:  identity.OrganizationID,
			TokenHash:       hmacTokenHash(service.agentTokenSigningSecret, plaintext),
			UsedCount:       0,
			CreatedByUserID: identity.UserID,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
		if err != nil {
			return err
		}
		if err := repositories.NodeEnrollmentProfiles().CreateNodeEnrollmentProfile(ctx, profile); err != nil {
			return err
		}
		result = service.toNodeEnrollmentProfilePayload(profile, plaintext)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "node_enrollment_profiles.create", "NODE_ENROLLMENT_PROFILE", profile.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) UpdateNodeEnrollmentProfile(ctx context.Context, identity InternalIdentity, profileID string, input NodeEnrollmentProfileMutationInput) (NodeEnrollmentProfilePayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionNodesManage)) {
		return NodeEnrollmentProfilePayload{}, ErrForbidden
	}
	var result NodeEnrollmentProfilePayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		existing, err := repositories.NodeEnrollmentProfiles().FindNodeEnrollmentProfileByID(ctx, identity.OrganizationID, profileID)
		if err != nil {
			return err
		}
		if err := service.ensureCanManageNodeGroups(identity, decodeJSONStringList(existing.GroupIDsJSON)); err != nil {
			return err
		}
		input = mergeNodeEnrollmentProfilePatchInput(input, existing)
		input, err = normalizeNodeEnrollmentProfileInput(input)
		if err != nil {
			return err
		}
		if err := service.ensureCanManageNodeGroups(identity, input.GroupIDs); err != nil {
			return err
		}
		if err := ensureNodeGroupsExist(ctx, repositories, identity.OrganizationID, input.GroupIDs); err != nil {
			return err
		}
		updated, err := nodeEnrollmentProfileRecordFromInput(input, existing)
		if err != nil {
			return err
		}
		updated.UpdatedAt = service.timestamp()
		if err := repositories.NodeEnrollmentProfiles().UpdateNodeEnrollmentProfile(ctx, updated, false); err != nil {
			return err
		}
		result = service.toNodeEnrollmentProfilePayload(updated, "")
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "node_enrollment_profiles.update", "NODE_ENROLLMENT_PROFILE", profileID, ""))
	})
	return result, mapServiceError(err)
}

func mergeNodeEnrollmentProfilePatchInput(input NodeEnrollmentProfileMutationInput, existing repo.NodeEnrollmentProfileRecord) NodeEnrollmentProfileMutationInput {
	merged := NodeEnrollmentProfileMutationInput{
		Name:                    existing.Name,
		Description:             existing.Description,
		Enabled:                 existing.Enabled,
		ExpiresAt:               existing.ExpiresAt,
		MaxUses:                 existing.MaxUses,
		NodeNameTemplate:        existing.NodeNameTemplate,
		GroupIDs:                decodeJSONStringList(existing.GroupIDsJSON),
		ListenIPs:               decodeNodeListenIPInputs(existing.ListenIPsJSON),
		SendIPs:                 decodeNodeSendIPInputs(existing.SendIPsJSON),
		PortRanges:              decodeNodePortRangeInputs(existing.PortRangesJSON),
		MaxRulePorts:            defaultMaxRulePorts(existing.MaxRulePorts),
		DNSPublishAddresses:     decodeNodeDNSPublishAddressInputs(existing.DNSPublishAddressesJSON),
		DataplaneMode:           existing.DataplaneMode,
		DataplaneConflictPolicy: existing.DataplaneConflictPolicy,
		AutoUpdateEnabled:       existing.AutoUpdateEnabled,
		AllowedCIDRs:            decodeJSONStringList(existing.AllowedCIDRsJSON),
	}
	if input.NameProvided {
		merged.Name = input.Name
	}
	if input.DescriptionProvided {
		merged.Description = input.Description
	}
	if input.EnabledProvided {
		merged.Enabled = input.Enabled
	}
	if input.ExpiresAtProvided {
		merged.ExpiresAt = input.ExpiresAt
	}
	if input.MaxUsesProvided {
		merged.MaxUses = input.MaxUses
	}
	if input.NodeNameTemplateProvided {
		merged.NodeNameTemplate = input.NodeNameTemplate
	}
	if input.GroupIDsProvided {
		merged.GroupIDs = input.GroupIDs
	}
	if input.ListenIPsProvided {
		merged.ListenIPs = input.ListenIPs
	}
	if input.SendIPsProvided {
		merged.SendIPs = input.SendIPs
	}
	if input.PortRangesProvided {
		merged.PortRanges = input.PortRanges
	}
	if input.MaxRulePortsProvided {
		merged.MaxRulePorts = input.MaxRulePorts
	}
	if input.DNSPublishAddressesProvided {
		merged.DNSPublishAddresses = input.DNSPublishAddresses
	}
	if input.DataplaneModeProvided {
		merged.DataplaneMode = input.DataplaneMode
	}
	if input.DataplaneConflictPolicyProvided {
		merged.DataplaneConflictPolicy = input.DataplaneConflictPolicy
	}
	if input.AutoUpdateEnabledProvided {
		merged.AutoUpdateEnabled = input.AutoUpdateEnabled
	}
	if input.AllowedCIDRsProvided {
		merged.AllowedCIDRs = input.AllowedCIDRs
	}
	return merged
}

func (service *ControlService) RotateNodeEnrollmentProfileToken(ctx context.Context, identity InternalIdentity, profileID string) (NodeEnrollmentProfilePayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionNodesManage)) {
		return NodeEnrollmentProfilePayload{}, ErrForbidden
	}
	if err := service.ensureCanIssueNodeEnrollmentToken(); err != nil {
		return NodeEnrollmentProfilePayload{}, err
	}
	plaintext, err := randomRegistrationToken()
	if err != nil {
		return NodeEnrollmentProfilePayload{}, err
	}
	var result NodeEnrollmentProfilePayload
	err = service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		profile, err := repositories.NodeEnrollmentProfiles().FindNodeEnrollmentProfileByID(ctx, identity.OrganizationID, profileID)
		if err != nil {
			return err
		}
		if err := service.ensureCanManageNodeGroups(identity, decodeJSONStringList(profile.GroupIDsJSON)); err != nil {
			return err
		}
		profile.TokenHash = hmacTokenHash(service.agentTokenSigningSecret, plaintext)
		profile.UsedCount = 0
		profile.RevokedAt = ""
		profile.UpdatedAt = service.timestamp()
		if err := repositories.NodeEnrollmentProfiles().UpdateNodeEnrollmentProfile(ctx, profile, true); err != nil {
			return err
		}
		result = service.toNodeEnrollmentProfilePayload(profile, plaintext)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "node_enrollment_profiles.rotate_token", "NODE_ENROLLMENT_PROFILE", profileID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) DeleteNodeEnrollmentProfile(ctx context.Context, identity InternalIdentity, profileID string) error {
	if !service.hasPermission(identity, string(domain.PermissionNodesManage)) {
		return ErrForbidden
	}
	var affectedDNSRecordIDs []string
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		profile, err := repositories.NodeEnrollmentProfiles().FindNodeEnrollmentProfileByIDForUpdate(ctx, identity.OrganizationID, profileID)
		if err != nil {
			return err
		}
		if err := service.ensureCanManageNodeGroups(identity, decodeJSONStringList(profile.GroupIDsJSON)); err != nil {
			return err
		}
		now := service.timestamp()
		releasedDNSRecordIDs, err := service.releasePendingNodeEnrollmentsForProfile(ctx, repositories, profile, now)
		if err != nil {
			return err
		}
		affectedDNSRecordIDs = releasedDNSRecordIDs
		if err := repositories.NodeEnrollmentProfiles().DeleteNodeEnrollmentProfile(ctx, identity.OrganizationID, profileID, now); err != nil {
			return err
		}
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "node_enrollment_profiles.delete", "NODE_ENROLLMENT_PROFILE", profileID, ""))
	})
	if err == nil && len(affectedDNSRecordIDs) > 0 {
		service.evaluateDNSManagedRecordsBestEffort(ctx, identity.OrganizationID, affectedDNSRecordIDs)
	}
	return mapServiceError(err)
}

func (service *ControlService) ListNodeEnrollmentEvents(ctx context.Context, identity InternalIdentity, profileID string) ([]NodeEnrollmentEventPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionNodesRead)) {
		return nil, ErrForbidden
	}
	var result []NodeEnrollmentEventPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		profile, err := repositories.NodeEnrollmentProfiles().FindNodeEnrollmentProfileByID(ctx, identity.OrganizationID, profileID)
		if err != nil {
			return err
		}
		if err := service.ensureCanUseNodeGroups(identity, decodeJSONStringList(profile.GroupIDsJSON)); err != nil {
			return err
		}
		events, err := repositories.NodeEnrollmentProfiles().ListNodeEnrollmentEvents(ctx, identity.OrganizationID, profileID, 50)
		if err != nil {
			return err
		}
		result = toNodeEnrollmentEventPayloads(events)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) authenticateNodeEnrollmentTokenInTx(ctx context.Context, repositories repo.Repositories, tokenHash string, exchangeRegistration bool, metadata AgentEnrollmentMetadata) (AgentAuthResult, bool, error) {
	profile, err := repositories.NodeEnrollmentProfiles().FindNodeEnrollmentProfileByTokenHashForUpdate(ctx, tokenHash)
	if err != nil {
		if err == repo.ErrNotFound {
			return AgentAuthResult{}, false, nil
		}
		return AgentAuthResult{}, false, err
	}
	_, staleAffectedDNSRecordIDs, err := service.validateNodeEnrollmentProfile(ctx, repositories, profile, metadata, exchangeRegistration)
	if err != nil {
		_ = service.recordNodeEnrollmentEvent(ctx, repositories, profile, "", "FAILED", enrollmentFailureCode(err), err.Error(), metadata)
		return AgentAuthResult{}, true, nodeEnrollmentAuthError{profile: profile, metadata: metadata, cause: err, commitFailure: true, affectedDNSRecordIDs: staleAffectedDNSRecordIDs}
	}
	if !exchangeRegistration {
		return AgentAuthResult{OrganizationID: profile.OrganizationID, AgentType: "NODE", RegisteredWithToken: true, EnrollmentProfileID: profile.ID, EnrollmentTokenHash: profile.TokenHash, AffectedDNSRecordIDs: staleAffectedDNSRecordIDs}, true, nil
	}
	authResult, err := service.createNodeFromEnrollmentProfile(ctx, repositories, profile, metadata)
	if err != nil {
		return AgentAuthResult{}, true, nodeEnrollmentAuthError{profile: profile, metadata: metadata, cause: err}
	}
	authResult.AffectedDNSRecordIDs = appendUniqueStrings(staleAffectedDNSRecordIDs, authResult.AffectedDNSRecordIDs...)
	return authResult, true, nil
}

func (service *ControlService) validateNodeEnrollmentProfile(ctx context.Context, repositories repo.Repositories, profile repo.NodeEnrollmentProfileRecord, metadata AgentEnrollmentMetadata, enforceCIDR bool) (int, []string, error) {
	released, affectedDNSRecordIDs, err := service.releaseStaleNodeEnrollmentReservations(ctx, repositories, profile)
	if err != nil {
		return 0, nil, err
	}
	profile.UsedCount -= released
	if profile.UsedCount < 0 {
		profile.UsedCount = 0
	}
	if !profile.Enabled || strings.TrimSpace(profile.RevokedAt) != "" {
		return released, affectedDNSRecordIDs, validationError("Enrollment profile is revoked or disabled.", map[string]any{"code": "ENROLLMENT_REVOKED"})
	}
	if strings.TrimSpace(profile.ExpiresAt) != "" {
		expiresAt, err := time.Parse(time.RFC3339Nano, profile.ExpiresAt)
		if err != nil || !expiresAt.After(service.now()) {
			return released, affectedDNSRecordIDs, validationError("Enrollment profile has expired.", map[string]any{"code": "ENROLLMENT_EXPIRED"})
		}
	}
	if profile.MaxUses > 0 && profile.UsedCount >= profile.MaxUses {
		return released, affectedDNSRecordIDs, validationError("Enrollment profile has reached its use limit.", map[string]any{"code": "ENROLLMENT_MAX_USES_EXCEEDED"})
	}
	if err := ensureNodeGroupsExist(ctx, repositories, profile.OrganizationID, decodeJSONStringList(profile.GroupIDsJSON)); err != nil {
		return released, affectedDNSRecordIDs, err
	}
	if enforceCIDR && !remoteIPAllowed(metadata.RemoteIP, decodeJSONStringList(profile.AllowedCIDRsJSON)) {
		return released, affectedDNSRecordIDs, validationError("Enrollment source IP is not allowed.", map[string]any{"code": "ENROLLMENT_CIDR_DENIED"})
	}
	return released, affectedDNSRecordIDs, nil
}

func (service *ControlService) createNodeFromEnrollmentProfile(ctx context.Context, repositories repo.Repositories, profile repo.NodeEnrollmentProfileRecord, metadata AgentEnrollmentMetadata) (AgentAuthResult, error) {
	now := service.timestamp()
	nodeID := service.newID()
	groupIDs := decodeJSONStringList(profile.GroupIDsJSON)
	renderedName := service.renderEnrollmentNodeName(profile.NodeNameTemplate, nodeID, metadata)
	renderedName = strings.TrimSpace(renderedName)
	if renderedName == "" || len(renderedName) > 120 {
		return AgentAuthResult{}, ErrInvalidInput
	}
	listenIPs := toNodeListenIPRecords(decodeNodeListenIPInputs(profile.ListenIPsJSON))
	sendIPs := toNodeSendIPRecords(decodeNodeSendIPInputs(profile.SendIPsJSON))
	portRanges := toNodePortRangeRecords(decodeNodePortRangeInputs(profile.PortRangesJSON))
	dnsAddresses := toNodeDNSPublishAddressRecords(decodeNodeDNSPublishAddressInputs(profile.DNSPublishAddressesJSON))
	node := repo.NodeRecord{
		ID:                      nodeID,
		OrganizationID:          profile.OrganizationID,
		Name:                    renderedName,
		Status:                  "PENDING",
		ConfigStatus:            "PENDING",
		AgentAutoUpdateEnabled:  profile.AutoUpdateEnabled,
		DataplaneMode:           profile.DataplaneMode,
		DataplaneConflictPolicy: profile.DataplaneConflictPolicy,
		EnrollmentProfileID:     profile.ID,
		CreatedAt:               now,
		UpdatedAt:               now,
		GroupIDs:                groupIDs,
		ListenIPs:               listenIPs,
		SendIPs:                 sendIPs,
		PortRanges:              portRanges,
		MaxRulePorts:            defaultMaxRulePorts(profile.MaxRulePorts),
		DNSPublishAddresses:     dnsAddresses,
	}
	nodes, err := repositories.Nodes().ListNodesByOrganization(ctx, profile.OrganizationID)
	if err != nil {
		return AgentAuthResult{}, err
	}
	node.Name = uniqueEnrollmentNodeName(node.Name, nodes, nodeID)
	if err := validateEnabledRulesForNodeSet(ctx, repositories, profile.OrganizationID, replaceNodeInSet(nodes, node)); err != nil {
		return AgentAuthResult{}, err
	}
	if err := repositories.Nodes().CreateNode(ctx, node, groupIDs, listenIPs, portRanges, now, service.newID); err != nil {
		return AgentAuthResult{}, err
	}
	if err := replaceManualNodeDNSPublishAddresses(ctx, repositories, profile.OrganizationID, node.ID, dnsAddresses, now, service.newID); err != nil {
		return AgentAuthResult{}, err
	}
	plaintext, err := randomRegistrationToken()
	if err != nil {
		return AgentAuthResult{}, err
	}
	credential := repo.AgentCredentialRecord{
		ID:                  service.newID(),
		OrganizationID:      profile.OrganizationID,
		AgentType:           "NODE",
		AgentID:             node.ID,
		CredentialHash:      hmacTokenHash(service.agentTokenSigningSecret, plaintext),
		EnrollmentProfileID: profile.ID,
		EnrollmentTokenHash: profile.TokenHash,
		CreatedAt:           now,
	}
	if err := repositories.AgentCredentials().CreateCredential(ctx, credential); err != nil {
		return AgentAuthResult{}, err
	}
	if err := repositories.NodeEnrollmentProfiles().IncrementNodeEnrollmentProfileUsedCount(ctx, profile.OrganizationID, profile.ID, now); err != nil {
		return AgentAuthResult{}, err
	}
	affectedDNSRecordIDs, err := service.markDNSRecordsDependingOnNodeGroupsPending(ctx, repositories, profile.OrganizationID, node.GroupIDs, now)
	if err != nil {
		return AgentAuthResult{}, err
	}
	return AgentAuthResult{
		OrganizationID:          profile.OrganizationID,
		AgentType:               "NODE",
		AgentID:                 node.ID,
		RegisteredWithToken:     true,
		EnrollmentProfileID:     profile.ID,
		EnrollmentTokenHash:     profile.TokenHash,
		EnrollmentMetadata:      metadata,
		AgentCredentialID:       credential.ID,
		AgentCredential:         plaintext,
		AgentCredentialFileHint: "agent-credential.json",
		AffectedDNSRecordIDs:    affectedDNSRecordIDs,
	}, nil
}

func (service *ControlService) releaseStaleNodeEnrollmentReservations(ctx context.Context, repositories repo.Repositories, profile repo.NodeEnrollmentProfileRecord) (int, []string, error) {
	credentials, err := repositories.AgentCredentials().ListPendingCredentialsByEnrollmentProfile(ctx, profile.OrganizationID, profile.ID)
	if err != nil {
		return 0, nil, err
	}
	now := service.timestamp()
	releasedUses := 0
	affectedDNSRecordIDs := make([]string, 0)
	for _, credential := range credentials {
		if !service.pendingCredentialIsStale(credential) {
			continue
		}
		authResult := AgentAuthResult{
			OrganizationID:      credential.OrganizationID,
			AgentType:           credential.AgentType,
			AgentID:             credential.AgentID,
			RegisteredWithToken: true,
			EnrollmentProfileID: credential.EnrollmentProfileID,
			EnrollmentTokenHash: credential.EnrollmentTokenHash,
			AgentCredentialID:   credential.ID,
		}
		releasedDNSRecordIDs, released, err := service.releaseAgentRegistrationCredentialInTx(ctx, repositories, authResult, now)
		if err != nil {
			return releasedUses, affectedDNSRecordIDs, err
		}
		affectedDNSRecordIDs = appendUniqueStrings(affectedDNSRecordIDs, releasedDNSRecordIDs...)
		if released && credential.EnrollmentTokenHash == profile.TokenHash {
			releasedUses++
		}
	}
	return releasedUses, affectedDNSRecordIDs, nil
}

func (service *ControlService) releasePendingNodeEnrollmentsForProfile(ctx context.Context, repositories repo.Repositories, profile repo.NodeEnrollmentProfileRecord, now string) ([]string, error) {
	credentials, err := repositories.AgentCredentials().ListPendingCredentialsByEnrollmentProfile(ctx, profile.OrganizationID, profile.ID)
	if err != nil {
		return nil, err
	}
	affectedDNSRecordIDs := make([]string, 0)
	for _, credential := range credentials {
		authResult := AgentAuthResult{
			OrganizationID:      credential.OrganizationID,
			AgentType:           credential.AgentType,
			AgentID:             credential.AgentID,
			RegisteredWithToken: true,
			EnrollmentProfileID: credential.EnrollmentProfileID,
			EnrollmentTokenHash: credential.EnrollmentTokenHash,
			AgentCredentialID:   credential.ID,
		}
		releasedDNSRecordIDs, _, err := service.releaseAgentRegistrationCredentialInTx(ctx, repositories, authResult, now)
		if err != nil {
			return affectedDNSRecordIDs, err
		}
		affectedDNSRecordIDs = appendUniqueStrings(affectedDNSRecordIDs, releasedDNSRecordIDs...)
	}
	return affectedDNSRecordIDs, nil
}

func (service *ControlService) releasePendingNodeEnrollmentForNode(ctx context.Context, repositories repo.Repositories, node repo.NodeRecord, now string) ([]string, bool, error) {
	if strings.TrimSpace(node.EnrollmentProfileID) == "" {
		return nil, false, nil
	}
	credentials, err := repositories.AgentCredentials().ListPendingCredentialsByEnrollmentProfile(ctx, node.OrganizationID, node.EnrollmentProfileID)
	if err != nil {
		return nil, false, err
	}
	for _, credential := range credentials {
		if credential.AgentID != node.ID {
			continue
		}
		authResult := AgentAuthResult{
			OrganizationID:      credential.OrganizationID,
			AgentType:           credential.AgentType,
			AgentID:             credential.AgentID,
			RegisteredWithToken: true,
			EnrollmentProfileID: credential.EnrollmentProfileID,
			EnrollmentTokenHash: credential.EnrollmentTokenHash,
			AgentCredentialID:   credential.ID,
		}
		releasedDNSRecordIDs, released, err := service.releaseAgentRegistrationCredentialInTx(ctx, repositories, authResult, now)
		return releasedDNSRecordIDs, released, err
	}
	return nil, false, nil
}

func (service *ControlService) ensureCanIssueNodeEnrollmentToken() error {
	if len(service.agentTokenSigningSecret) == 0 || service.appName == "" || service.controlPlaneURL == "" {
		return ErrInvalidInput
	}
	return nil
}

func (service *ControlService) recordNodeEnrollmentFailureBestEffort(ctx context.Context, profile repo.NodeEnrollmentProfileRecord, cause error, metadata AgentEnrollmentMetadata) {
	_ = service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		return service.recordNodeEnrollmentEvent(ctx, repositories, profile, "", "FAILED", enrollmentFailureCode(cause), cause.Error(), metadata)
	})
}

func enrollmentAuthError(err error) (nodeEnrollmentAuthError, bool) {
	var authErr nodeEnrollmentAuthError
	if errors.As(err, &authErr) {
		return authErr, true
	}
	return nodeEnrollmentAuthError{}, false
}

func appendUniqueStrings(values []string, next ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(next))
	for _, value := range values {
		if value == "" {
			continue
		}
		seen[value] = struct{}{}
	}
	for _, value := range next {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		values = append(values, value)
		seen[value] = struct{}{}
	}
	return values
}

func (service *ControlService) recordNodeEnrollmentEvent(ctx context.Context, repositories repo.Repositories, profile repo.NodeEnrollmentProfileRecord, nodeID string, status string, reasonCode string, message string, metadata AgentEnrollmentMetadata) error {
	return repositories.NodeEnrollmentProfiles().CreateNodeEnrollmentEvent(ctx, repo.NodeEnrollmentEventRecord{
		ID:                  service.newID(),
		OrganizationID:      profile.OrganizationID,
		EnrollmentProfileID: profile.ID,
		NodeID:              nodeID,
		Status:              status,
		ReasonCode:          reasonCode,
		Message:             message,
		RemoteIP:            metadata.RemoteIP,
		Hostname:            metadata.Hostname,
		MetadataJSON:        "{}",
		CreatedAt:           service.timestamp(),
	})
}

func normalizeNodeEnrollmentProfileInput(input NodeEnrollmentProfileMutationInput) (NodeEnrollmentProfileMutationInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.NodeNameTemplate = strings.TrimSpace(input.NodeNameTemplate)
	if input.Name == "" {
		return NodeEnrollmentProfileMutationInput{}, ErrInvalidInput
	}
	if input.MaxUses < 0 {
		return NodeEnrollmentProfileMutationInput{}, ErrInvalidInput
	}
	if strings.TrimSpace(input.ExpiresAt) != "" {
		if _, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(input.ExpiresAt)); err != nil {
			return NodeEnrollmentProfileMutationInput{}, ErrInvalidInput
		}
		input.ExpiresAt = strings.TrimSpace(input.ExpiresAt)
	}
	if input.NodeNameTemplate == "" {
		input.NodeNameTemplate = "{{hostname}}"
	}
	if !enrollmentNodeNameTemplateCanFit(input.NodeNameTemplate) {
		return NodeEnrollmentProfileMutationInput{}, ErrInvalidInput
	}
	if len(input.GroupIDs) == 0 {
		return NodeEnrollmentProfileMutationInput{}, ErrInvalidInput
	}
	if strings.TrimSpace(input.DataplaneMode) == "" {
		input.DataplaneMode = "AUTO"
	}
	if strings.TrimSpace(input.DataplaneConflictPolicy) == "" {
		input.DataplaneConflictPolicy = "FAIL_FAST"
	}
	dataplaneMode, err := normalizeNodeDataplaneModeForMutation(input.DataplaneMode)
	if err != nil {
		return NodeEnrollmentProfileMutationInput{}, err
	}
	input.DataplaneMode = dataplaneMode
	if err := validateNodeDataplaneConflictPolicy(input.DataplaneConflictPolicy); err != nil {
		return NodeEnrollmentProfileMutationInput{}, err
	}
	for _, cidr := range input.AllowedCIDRs {
		if strings.TrimSpace(cidr) == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(strings.TrimSpace(cidr)); err != nil {
			return NodeEnrollmentProfileMutationInput{}, ErrInvalidInput
		}
	}
	if len(input.ListenIPs) == 0 {
		input.ListenIPs = []NodeListenIPInput{{ListenIP: "0.0.0.0", DisplayName: "default"}}
	}
	if len(input.PortRanges) == 0 {
		input.PortRanges = []NodePortRangeInput{{Protocol: "TCP", StartPort: 10000, EndPort: 20000}}
	}
	input.MaxRulePorts = defaultMaxRulePorts(input.MaxRulePorts)
	return input, nil
}

func nodeEnrollmentProfileRecordFromInput(input NodeEnrollmentProfileMutationInput, profile repo.NodeEnrollmentProfileRecord) (repo.NodeEnrollmentProfileRecord, error) {
	groupIDsJSON, err := jsonStringList(input.GroupIDs)
	if err != nil {
		return repo.NodeEnrollmentProfileRecord{}, err
	}
	listenIPsJSON, err := jsonStringValue(input.ListenIPs)
	if err != nil {
		return repo.NodeEnrollmentProfileRecord{}, err
	}
	sendIPsJSON, err := jsonStringValue(input.SendIPs)
	if err != nil {
		return repo.NodeEnrollmentProfileRecord{}, err
	}
	portRangesJSON, err := jsonStringValue(input.PortRanges)
	if err != nil {
		return repo.NodeEnrollmentProfileRecord{}, err
	}
	dnsPublishAddressesJSON, err := jsonStringValue(input.DNSPublishAddresses)
	if err != nil {
		return repo.NodeEnrollmentProfileRecord{}, err
	}
	allowedCIDRsJSON, err := jsonStringList(input.AllowedCIDRs)
	if err != nil {
		return repo.NodeEnrollmentProfileRecord{}, err
	}
	profile.Name = input.Name
	profile.Description = input.Description
	profile.Enabled = input.Enabled
	profile.ExpiresAt = input.ExpiresAt
	profile.MaxUses = input.MaxUses
	profile.NodeNameTemplate = input.NodeNameTemplate
	profile.GroupIDsJSON = groupIDsJSON
	profile.ListenIPsJSON = listenIPsJSON
	profile.SendIPsJSON = sendIPsJSON
	profile.PortRangesJSON = portRangesJSON
	profile.MaxRulePorts = defaultMaxRulePorts(input.MaxRulePorts)
	profile.DNSPublishAddressesJSON = dnsPublishAddressesJSON
	profile.DataplaneMode = input.DataplaneMode
	profile.DataplaneConflictPolicy = input.DataplaneConflictPolicy
	profile.AutoUpdateEnabled = input.AutoUpdateEnabled
	profile.AllowedCIDRsJSON = allowedCIDRsJSON
	if strings.TrimSpace(profile.MetadataJSON) == "" {
		profile.MetadataJSON = "{}"
	}
	return profile, nil
}

func (service *ControlService) toNodeEnrollmentProfilePayload(profile repo.NodeEnrollmentProfileRecord, plaintextToken string) NodeEnrollmentProfilePayload {
	payload := NodeEnrollmentProfilePayload{
		ID:                      profile.ID,
		Name:                    profile.Name,
		Description:             profile.Description,
		Enabled:                 profile.Enabled,
		ExpiresAt:               profile.ExpiresAt,
		MaxUses:                 profile.MaxUses,
		UsedCount:               profile.UsedCount,
		NodeNameTemplate:        profile.NodeNameTemplate,
		GroupIDs:                decodeJSONStringList(profile.GroupIDsJSON),
		ListenIPs:               toNodeListenIPPayloads(toNodeListenIPRecords(decodeNodeListenIPInputs(profile.ListenIPsJSON))),
		SendIPs:                 toNodeSendIPPayloads(toNodeSendIPRecords(decodeNodeSendIPInputs(profile.SendIPsJSON))),
		PortRanges:              toNodePortRangePayloads(toNodePortRangeRecords(decodeNodePortRangeInputs(profile.PortRangesJSON))),
		MaxRulePorts:            defaultMaxRulePorts(profile.MaxRulePorts),
		DNSPublishAddresses:     toNodeDNSPublishAddressPayloads(toNodeDNSPublishAddressRecords(decodeNodeDNSPublishAddressInputs(profile.DNSPublishAddressesJSON))),
		DataplaneMode:           defaultNodeDataplaneMode(profile.DataplaneMode),
		DataplaneConflictPolicy: defaultNodeDataplaneConflictPolicy(profile.DataplaneConflictPolicy),
		AutoUpdateEnabled:       profile.AutoUpdateEnabled,
		AllowedCIDRs:            decodeJSONStringList(profile.AllowedCIDRsJSON),
		CreatedAt:               profile.CreatedAt,
		UpdatedAt:               profile.UpdatedAt,
		RevokedAt:               profile.RevokedAt,
		CreatedByUserID:         profile.CreatedByUserID,
	}
	if plaintextToken != "" {
		payload.Token = plaintextToken
		payload.InstallCommand = service.enrollmentInstallCommand(plaintextToken)
		payload.ShellScript = service.enrollmentShellScript(plaintextToken)
		payload.AWSCloudInit = "#cloud-config\nruncmd:\n  - |\n" + indentYAMLBlock(payload.ShellScript, 4)
		payload.TerraformUserData = "<<-EOF\n#!/bin/sh\nset -eu\n" + terraformEscapeShellScript(payload.ShellScript) + "\nEOF"
		payload.SystemdReadyScript = "#!/bin/sh\nset -eu\n" + payload.ShellScript + "\n"
	}
	return payload
}

func indentYAMLBlock(value string, spaces int) string {
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		lines[index] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func terraformEscapeShellScript(script string) string {
	return strings.ReplaceAll(script, "${", "$${")
}

func toNodeEnrollmentEventPayloads(events []repo.NodeEnrollmentEventRecord) []NodeEnrollmentEventPayload {
	payloads := make([]NodeEnrollmentEventPayload, 0, len(events))
	for _, event := range events {
		payloads = append(payloads, NodeEnrollmentEventPayload{
			ID:                  event.ID,
			EnrollmentProfileID: event.EnrollmentProfileID,
			NodeID:              event.NodeID,
			Status:              event.Status,
			ReasonCode:          event.ReasonCode,
			Message:             event.Message,
			RemoteIP:            event.RemoteIP,
			Hostname:            event.Hostname,
			CreatedAt:           event.CreatedAt,
		})
	}
	return payloads
}

func (service *ControlService) enrollmentInstallCommand(token string) string {
	releaseVersion := service.nodeAgentInstallReleaseVersion()
	return "(tmp=$(mktemp) && curl -fsSL " + shellQuote(nodeAgentInstallerURL(releaseVersion)) + " -o \"$tmp\" && sudo env APP_NAME=" + shellQuote(service.appName) + " sh \"$tmp\" --version " + shellQuote(releaseVersion) + " --control-url " + shellQuote(service.controlPlaneURL) + " --enrollment-token " + shellQuote(token) + " --credential-file agent-credential.json; status=$?; rm -f \"${tmp:-}\"; exit \"$status\")"
}

func (service *ControlService) enrollmentShellScript(token string) string {
	return service.enrollmentInstallCommand(token)
}

func decodeJSONStringList(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &values); err != nil {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func decodeNodeListenIPInputs(raw string) []NodeListenIPInput {
	var values []NodeListenIPInput
	_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &values)
	return values
}

func decodeNodeSendIPInputs(raw string) []NodeSendIPInput {
	var values []NodeSendIPInput
	_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &values)
	return values
}

func decodeNodePortRangeInputs(raw string) []NodePortRangeInput {
	var values []NodePortRangeInput
	_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &values)
	return values
}

func decodeNodeDNSPublishAddressInputs(raw string) []NodeDNSPublishAddressInput {
	var values []NodeDNSPublishAddressInput
	_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &values)
	return values
}

func jsonStringValue(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func remoteIPAllowed(rawRemoteIP string, allowedCIDRs []string) bool {
	if len(allowedCIDRs) == 0 {
		return true
	}
	remoteIP := net.ParseIP(strings.TrimSpace(rawRemoteIP))
	if remoteIP == nil {
		return false
	}
	for _, rawCIDR := range allowedCIDRs {
		_, cidr, err := net.ParseCIDR(strings.TrimSpace(rawCIDR))
		if err == nil && cidr.Contains(remoteIP) {
			return true
		}
	}
	return false
}

func (service *ControlService) renderEnrollmentNodeName(template string, nodeID string, metadata AgentEnrollmentMetadata) string {
	hostname := strings.TrimSpace(metadata.Hostname)
	if hostname == "" {
		hostname = "node-" + shortID(nodeID)
	}
	replacements := map[string]string{
		"{{hostname}}":    hostname,
		"{{instance_id}}": "",
		"{{private_ip}}":  "",
		"{{public_ip}}":   strings.TrimSpace(metadata.RemoteIP),
		"{{timestamp}}":   service.now().UTC().Format("20060102150405"),
	}
	result := strings.TrimSpace(template)
	for key, value := range replacements {
		result = strings.ReplaceAll(result, key, value)
	}
	result = strings.TrimSpace(result)
	if result == "" {
		return hostname
	}
	return result
}

func enrollmentNodeNameTemplateCanFit(template string) bool {
	fixed := strings.TrimSpace(template)
	for _, placeholder := range []string{"{{hostname}}", "{{instance_id}}", "{{private_ip}}", "{{public_ip}}", "{{timestamp}}"} {
		fixed = strings.ReplaceAll(fixed, placeholder, "")
	}
	return len(strings.TrimSpace(fixed)) <= 120
}

func uniqueEnrollmentNodeName(name string, nodes []repo.NodeRecord, nodeID string) string {
	for _, node := range nodes {
		if node.Name == name && node.DeletedAt == "" {
			suffix := "-" + shortID(nodeID)
			return trimStringBytes(name, 120-len(suffix)) + suffix
		}
	}
	return name
}

func trimStringBytes(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	var builder strings.Builder
	builder.Grow(maxBytes)
	for _, char := range value {
		if builder.Len()+len(string(char)) > maxBytes {
			break
		}
		builder.WriteRune(char)
	}
	return strings.TrimSpace(builder.String())
}

func shortID(id string) string {
	id = strings.ReplaceAll(id, "-", "")
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func enrollmentFailureCode(err error) string {
	if err != nil && strings.Contains(err.Error(), "expired") {
		return "ENROLLMENT_EXPIRED"
	}
	if err != nil && strings.Contains(err.Error(), "use limit") {
		return "ENROLLMENT_MAX_USES_EXCEEDED"
	}
	if err != nil && strings.Contains(err.Error(), "source IP") {
		return "ENROLLMENT_CIDR_DENIED"
	}
	if err != nil && strings.Contains(err.Error(), "revoked or disabled") {
		return "ENROLLMENT_REVOKED"
	}
	return "ENROLLMENT_FAILED"
}
