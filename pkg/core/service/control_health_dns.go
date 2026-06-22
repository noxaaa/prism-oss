package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func (service *ControlService) ListHealthChecks(ctx context.Context, identity InternalIdentity) ([]HealthCheckPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionHealthChecksRead)) && !service.hasPermission(identity, string(domain.PermissionHealthChecksManage)) {
		return nil, ErrForbidden
	}
	var result []HealthCheckPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		checks, err := repositories.HealthChecks().ListHealthChecksByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		result = toHealthCheckPayloads(checks)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) CreateHealthCheck(ctx context.Context, identity InternalIdentity, input HealthCheckMutationInput) (HealthCheckPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionHealthChecksManage)) {
		return HealthCheckPayload{}, ErrForbidden
	}
	var result HealthCheckPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		targets, monitorScopes, err := service.buildHealthBindings(ctx, repositories, identity.OrganizationID, input)
		if err != nil {
			return err
		}
		now := service.timestamp()
		check := repo.HealthCheckRecord{
			ID:              service.newID(),
			OrganizationID:  identity.OrganizationID,
			Name:            input.Name,
			ProbeType:       input.ProbeType,
			IntervalSeconds: input.IntervalSeconds,
			TimeoutSeconds:  input.TimeoutSeconds,
			ConfigJSON:      normalizedConfigJSON(input.ConfigJSON),
			Enabled:         input.Enabled,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := repositories.HealthChecks().CreateHealthCheck(ctx, check, targets, monitorScopes, now, service.newID); err != nil {
			return err
		}
		check, err = repositories.HealthChecks().FindHealthCheckByID(ctx, identity.OrganizationID, check.ID)
		if err != nil {
			return err
		}
		result = toHealthCheckPayload(check)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "health_checks.create", "HEALTH_CHECK", check.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) UpdateHealthCheck(ctx context.Context, identity InternalIdentity, healthCheckID string, input HealthCheckMutationInput) (HealthCheckPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionHealthChecksManage)) {
		return HealthCheckPayload{}, ErrForbidden
	}
	var result HealthCheckPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		check, err := repositories.HealthChecks().FindHealthCheckByID(ctx, identity.OrganizationID, healthCheckID)
		if err != nil {
			return err
		}
		targets, monitorScopes, err := service.buildHealthBindings(ctx, repositories, identity.OrganizationID, input)
		if err != nil {
			return err
		}
		check.Name = input.Name
		check.ProbeType = input.ProbeType
		check.IntervalSeconds = input.IntervalSeconds
		check.TimeoutSeconds = input.TimeoutSeconds
		check.ConfigJSON = normalizedConfigJSON(input.ConfigJSON)
		check.Enabled = input.Enabled
		check.UpdatedAt = service.timestamp()
		if err := repositories.HealthChecks().UpdateHealthCheck(ctx, check, targets, monitorScopes, check.UpdatedAt, service.newID); err != nil {
			return err
		}
		check, err = repositories.HealthChecks().FindHealthCheckByID(ctx, identity.OrganizationID, check.ID)
		if err != nil {
			return err
		}
		result = toHealthCheckPayload(check)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "health_checks.update", "HEALTH_CHECK", check.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) DeleteHealthCheck(ctx context.Context, identity InternalIdentity, healthCheckID string) error {
	if !service.hasPermission(identity, string(domain.PermissionHealthChecksManage)) {
		return ErrForbidden
	}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if err := ensureHealthCheckHasNoActions(ctx, repositories, identity.OrganizationID, healthCheckID); err != nil {
			return err
		}
		deletedAt := service.timestamp()
		if err := repositories.HealthChecks().DeleteHealthCheck(ctx, identity.OrganizationID, healthCheckID, deletedAt); err != nil {
			return err
		}
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "health_checks.delete", "HEALTH_CHECK", healthCheckID, ""))
	})
	return mapServiceError(err)
}

func ensureHealthCheckHasNoActions(ctx context.Context, repositories repo.Repositories, organizationID string, healthCheckID string) error {
	rules, err := repositories.HealthChecks().ListHealthEvaluationRulesByCheck(ctx, organizationID, healthCheckID)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		for _, event := range rule.Events {
			if event.Enabled {
				return ErrConflict
			}
		}
	}
	return nil
}

func (service *ControlService) ListHealthResults(ctx context.Context, identity InternalIdentity, healthCheckID string) ([]HealthResultPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionHealthChecksRead)) && !service.hasPermission(identity, string(domain.PermissionHealthChecksManage)) {
		return nil, ErrForbidden
	}
	var result []HealthResultPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		results, err := repositories.HealthChecks().ListHealthResults(ctx, identity.OrganizationID, healthCheckID, 100)
		if err != nil {
			return err
		}
		result = toHealthResultPayloads(results)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) RecordMonitorHealthResults(ctx context.Context, organizationID string, monitorID string, results []HealthResultInput) error {
	var actions []pendingHealthAction
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		now := service.timestamp()
		records := make([]repo.HealthResultRecord, 0, len(results))
		if err := service.authorizeMonitorHealthResults(ctx, repositories, organizationID, monitorID, results); err != nil {
			return err
		}
		for _, result := range results {
			observedAt := clampFutureObservedAt(result.ObservedAt, now)
			records = append(records, repo.HealthResultRecord{
				ID:                  service.newID(),
				OrganizationID:      organizationID,
				HealthCheckID:       result.HealthCheckID,
				HealthCheckTargetID: result.HealthCheckTargetID,
				MonitorID:           monitorID,
				TargetID:            result.TargetID,
				Status:              result.Status,
				LatencyMS:           result.LatencyMS,
				ErrorMessage:        result.ErrorMessage,
				ObservedAt:          observedAt,
				CreatedAt:           now,
			})
		}
		if err := repositories.HealthChecks().RecordHealthResults(ctx, organizationID, records); err != nil {
			return err
		}
		var err error
		actions, err = service.buildHealthActions(ctx, repositories, organizationID, records)
		return err
	})
	if err != nil {
		return mapServiceError(err)
	}
	for _, action := range actions {
		if err := action.executor.Execute(ctx, action.payload); err != nil {
			return mapServiceError(err)
		}
	}
	return nil
}

func (service *ControlService) authorizeMonitorHealthResults(ctx context.Context, repositories repo.Repositories, organizationID string, monitorID string, results []HealthResultInput) error {
	if len(results) == 0 {
		return nil
	}
	monitor, err := repositories.Monitors().FindMonitorByID(ctx, organizationID, monitorID)
	if err != nil {
		return err
	}
	activeMonitorGroupIDs, err := activeMonitorGroupIDSet(ctx, repositories, organizationID)
	if err != nil {
		return err
	}
	checks := make(map[string]repo.HealthCheckRecord)
	for _, result := range results {
		check, ok := checks[result.HealthCheckID]
		if !ok {
			check, err = repositories.HealthChecks().FindHealthCheckByID(ctx, organizationID, result.HealthCheckID)
			if err != nil {
				return err
			}
			check, err = service.syncHealthCheckTargetGroupBindings(ctx, repositories, organizationID, check)
			if err != nil {
				return err
			}
			checks[result.HealthCheckID] = check
		}
		if !check.Enabled || !healthCheckTargetsMonitor(check, monitor, activeMonitorGroupIDs) || !healthCheckIncludesResultTarget(check, result) {
			return ErrForbidden
		}
	}
	return nil
}

func healthCheckIncludesResultTarget(check repo.HealthCheckRecord, result HealthResultInput) bool {
	for _, target := range check.Targets {
		if target.ID == result.HealthCheckTargetID && target.TargetID == result.TargetID {
			return true
		}
	}
	return false
}

func (service *ControlService) buildHealthBindings(ctx context.Context, repositories repo.Repositories, organizationID string, input HealthCheckMutationInput) ([]repo.HealthCheckTargetRecord, []repo.HealthCheckMonitorScopeRecord, error) {
	targets := make([]repo.HealthCheckTargetRecord, 0)
	switch input.TargetScope.Type {
	case "TARGETS":
		for _, targetID := range input.TargetScope.TargetIDs {
			target, err := repositories.Targets().FindTargetByID(ctx, organizationID, targetID)
			if err != nil {
				return nil, nil, err
			}
			if !target.Enabled {
				return nil, nil, ErrInvalidInput
			}
			targets = append(targets, repo.HealthCheckTargetRecord{ScopeType: "TARGET", TargetID: targetID})
		}
	case "TARGET_GROUP":
		targetGroup, err := repositories.TargetGroups().FindTargetGroupByID(ctx, organizationID, input.TargetScope.TargetGroupID)
		if err != nil {
			return nil, nil, err
		}
		targets, err = service.appendTargetGroupHealthBindings(ctx, repositories, organizationID, targets, targetGroup)
		if err != nil {
			return nil, nil, err
		}
	default:
		return nil, nil, ErrInvalidInput
	}
	if len(targets) == 0 {
		return nil, nil, ErrInvalidInput
	}

	var scopes []repo.HealthCheckMonitorScopeRecord
	switch input.MonitorScope.Type {
	case "MONITOR":
		if _, err := repositories.Monitors().FindMonitorByID(ctx, organizationID, input.MonitorScope.MonitorID); err != nil {
			return nil, nil, err
		}
		scopes = []repo.HealthCheckMonitorScopeRecord{{ScopeType: "MONITOR", MonitorID: input.MonitorScope.MonitorID}}
	case "MONITOR_GROUP":
		if _, err := repositories.MonitorGroups().FindMonitorGroupByID(ctx, organizationID, input.MonitorScope.MonitorGroupID); err != nil {
			return nil, nil, err
		}
		scopes = []repo.HealthCheckMonitorScopeRecord{{ScopeType: "MONITOR_GROUP", MonitorGroupID: input.MonitorScope.MonitorGroupID}}
	default:
		return nil, nil, ErrInvalidInput
	}
	return targets, scopes, nil
}

func (service *ControlService) appendTargetGroupHealthBindings(ctx context.Context, repositories repo.Repositories, organizationID string, targets []repo.HealthCheckTargetRecord, targetGroup repo.TargetGroupRecord) ([]repo.HealthCheckTargetRecord, error) {
	targets = append(targets, repo.HealthCheckTargetRecord{
		ScopeType:     "TARGET_GROUP",
		TargetGroupID: targetGroup.ID,
	})
	for _, member := range targetGroup.Members {
		if !member.Enabled {
			continue
		}
		target, err := repositories.Targets().FindTargetByID(ctx, organizationID, member.TargetID)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				continue
			}
			return nil, err
		}
		if !target.Enabled {
			continue
		}
		targets = append(targets, repo.HealthCheckTargetRecord{
			ScopeType:     "TARGET_GROUP",
			TargetID:      target.ID,
			TargetGroupID: targetGroup.ID,
		})
	}
	return targets, nil
}

func (service *ControlService) CompileMonitorAgentConfig(ctx context.Context, organizationID string, monitorID string) (agent.MonitorConfigSnapshot, error) {
	var snapshot agent.MonitorConfigSnapshot
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		monitor, err := repositories.Monitors().FindMonitorByID(ctx, organizationID, monitorID)
		if err != nil {
			return err
		}
		checks, err := repositories.HealthChecks().ListHealthChecksByOrganization(ctx, organizationID)
		if err != nil {
			return err
		}
		snapshot = agent.MonitorConfigSnapshot{
			MonitorID:      monitor.ID,
			ConfigVersion:  monitor.DesiredConfigVersion,
			HealthChecks:   make([]agent.MonitorHealthCheck, 0),
			GeneratedAtUTC: service.timestamp(),
		}
		activeMonitorGroupIDs, err := activeMonitorGroupIDSet(ctx, repositories, organizationID)
		if err != nil {
			return err
		}
		for _, check := range checks {
			if !check.Enabled || !healthCheckTargetsMonitor(check, monitor, activeMonitorGroupIDs) {
				continue
			}
			check, err = service.syncHealthCheckTargetGroupBindings(ctx, repositories, organizationID, check)
			if err != nil {
				return err
			}
			compiled := agent.MonitorHealthCheck{
				ID:              check.ID,
				ProbeType:       check.ProbeType,
				IntervalSeconds: check.IntervalSeconds,
				TimeoutSeconds:  check.TimeoutSeconds,
				ConfigJSON:      check.ConfigJSON,
				Targets:         make([]agent.MonitorHealthTarget, 0, len(check.Targets)),
			}
			for _, target := range check.Targets {
				if target.TargetID == "" {
					continue
				}
				compiled.Targets = append(compiled.Targets, agent.MonitorHealthTarget{
					HealthCheckTargetID: target.ID,
					TargetID:            target.TargetID,
					Name:                target.TargetName,
					Host:                target.TargetHost,
					Port:                target.TargetPort,
				})
			}
			snapshot.HealthChecks = append(snapshot.HealthChecks, compiled)
		}
		return nil
	})
	return snapshot, mapServiceError(err)
}

func (service *ControlService) syncHealthCheckTargetGroupBindings(ctx context.Context, repositories repo.Repositories, organizationID string, check repo.HealthCheckRecord) (repo.HealthCheckRecord, error) {
	targetGroupIDs := healthCheckTargetGroupIDs(check)
	if len(targetGroupIDs) == 0 {
		return check, nil
	}
	targets := make([]repo.HealthCheckTargetRecord, 0)
	for _, targetGroupID := range targetGroupIDs {
		group, err := repositories.TargetGroups().FindTargetGroupByID(ctx, organizationID, targetGroupID)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				targets = append(targets, repo.HealthCheckTargetRecord{
					ScopeType:     "TARGET_GROUP",
					TargetGroupID: targetGroupID,
				})
				continue
			}
			return repo.HealthCheckRecord{}, err
		}
		targets, err = service.appendTargetGroupHealthBindings(ctx, repositories, organizationID, targets, group)
		if err != nil {
			return repo.HealthCheckRecord{}, err
		}
	}
	now := service.timestamp()
	if err := repositories.HealthChecks().SyncHealthCheckTargets(ctx, organizationID, check.ID, targets, now, service.newID); err != nil {
		return repo.HealthCheckRecord{}, err
	}
	return repositories.HealthChecks().FindHealthCheckByID(ctx, organizationID, check.ID)
}

func healthCheckTargetGroupIDs(check repo.HealthCheckRecord) []string {
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, target := range check.Targets {
		if target.ScopeType != "TARGET_GROUP" || target.TargetGroupID == "" || seen[target.TargetGroupID] {
			continue
		}
		seen[target.TargetGroupID] = true
		result = append(result, target.TargetGroupID)
	}
	return result
}

func (service *ControlService) AcknowledgeMonitorAgentConfig(ctx context.Context, organizationID string, monitorID string, configVersion int) error {
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		return repositories.Monitors().RecordMonitorConfigAck(ctx, organizationID, monitorID, configVersion, service.timestamp())
	})
	return mapServiceError(err)
}

func (service *ControlService) ListDNSCredentials(ctx context.Context, identity InternalIdentity) ([]DNSCredentialPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionDNSRead)) && !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return nil, ErrForbidden
	}
	var result []DNSCredentialPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		credentials, err := repositories.DNSCredentials().ListDNSCredentialsByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		result = toDNSCredentialPayloads(credentials)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) CreateDNSCredential(ctx context.Context, identity InternalIdentity, input DNSCredentialMutationInput) (DNSCredentialPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return DNSCredentialPayload{}, ErrForbidden
	}
	encryptedSecret, err := service.encryptDNSSecret(input.Secret)
	if err != nil {
		return DNSCredentialPayload{}, err
	}
	var result DNSCredentialPayload
	err = service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		now := service.timestamp()
		credential := repo.DNSCredentialRecord{
			ID:              service.newID(),
			OrganizationID:  identity.OrganizationID,
			Name:            input.Name,
			Provider:        input.Provider,
			EncryptedSecret: encryptedSecret,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := repositories.DNSCredentials().CreateDNSCredential(ctx, credential); err != nil {
			return err
		}
		result = toDNSCredentialPayload(credential)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "dns_credentials.create", "DNS_CREDENTIAL", credential.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) UpdateDNSCredential(ctx context.Context, identity InternalIdentity, credentialID string, input DNSCredentialMutationInput) (DNSCredentialPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return DNSCredentialPayload{}, ErrForbidden
	}
	var encryptedSecret string
	var replaceSecret bool
	if strings.TrimSpace(input.Secret) != "" {
		value, err := service.encryptDNSSecret(input.Secret)
		if err != nil {
			return DNSCredentialPayload{}, err
		}
		encryptedSecret = value
		replaceSecret = true
	}
	var result DNSCredentialPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		credential, err := repositories.DNSCredentials().FindDNSCredentialByID(ctx, identity.OrganizationID, credentialID)
		if err != nil {
			return err
		}
		credential.Name = input.Name
		credential.Provider = input.Provider
		if replaceSecret {
			credential.EncryptedSecret = encryptedSecret
		}
		credential.UpdatedAt = service.timestamp()
		if err := repositories.DNSCredentials().UpdateDNSCredential(ctx, credential, replaceSecret); err != nil {
			return err
		}
		result = toDNSCredentialPayload(credential)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "dns_credentials.update", "DNS_CREDENTIAL", credential.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) DeleteDNSCredential(ctx context.Context, identity InternalIdentity, credentialID string) error {
	if !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return ErrForbidden
	}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if err := service.ensureDNSCredentialNotReferenced(ctx, repositories, identity.OrganizationID, credentialID); err != nil {
			return err
		}
		if err := repositories.DNSCredentials().DeleteDNSCredential(ctx, identity.OrganizationID, credentialID, service.timestamp()); err != nil {
			return err
		}
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "dns_credentials.delete", "DNS_CREDENTIAL", credentialID, ""))
	})
	return mapServiceError(err)
}

func (service *ControlService) ensureDNSCredentialNotReferenced(ctx context.Context, repositories repo.Repositories, organizationID string, credentialID string) error {
	records, err := repositories.DNSRecords().ListDNSRecordsByOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.DNSCredentialID == credentialID || record.PendingRetireDNSCredentialID == credentialID {
			return ErrConflict
		}
	}
	return nil
}

func (service *ControlService) encryptDNSSecret(secret string) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" || strings.TrimSpace(service.dnsSecretEncryptionKey) == "" {
		return "", ErrInvalidInput
	}
	key := sha256.Sum256([]byte(service.dnsSecretEncryptionKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(secret), nil)
	return base64.StdEncoding.EncodeToString(append(nonce, ciphertext...)), nil
}

func (service *ControlService) decryptDNSSecret(encryptedSecret string) (string, error) {
	encryptedSecret = strings.TrimSpace(encryptedSecret)
	if encryptedSecret == "" || strings.TrimSpace(service.dnsSecretEncryptionKey) == "" {
		return "", ErrInvalidInput
	}
	payload, err := base64.StdEncoding.DecodeString(encryptedSecret)
	if err != nil {
		return "", err
	}
	key := sha256.Sum256([]byte(service.dnsSecretEncryptionKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(payload) < gcm.NonceSize() {
		return "", ErrInvalidInput
	}
	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (service *ControlService) ListDNSRecords(ctx context.Context, identity InternalIdentity) ([]DNSRecordPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionDNSRead)) && !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return nil, ErrForbidden
	}
	var result []DNSRecordPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		records, err := repositories.DNSRecords().ListDNSRecordsByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		result = toDNSRecordPayloads(records)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) CreateDNSRecord(ctx context.Context, identity InternalIdentity, input DNSRecordMutationInput) (DNSRecordPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return DNSRecordPayload{}, ErrForbidden
	}
	if err := service.ensureDNSRecordMutationAllowed(identity, input); err != nil {
		return DNSRecordPayload{}, err
	}
	var result DNSRecordPayload
	var action dnsEventAction
	var hasAction bool
	var recordID string
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if _, err := repositories.DNSCredentials().FindDNSCredentialByID(ctx, identity.OrganizationID, input.DNSCredentialID); err != nil {
			return err
		}
		now := service.timestamp()
		record := repo.DNSRecordRecord{
			ID:                    service.newID(),
			OrganizationID:        identity.OrganizationID,
			DNSCredentialID:       input.DNSCredentialID,
			Zone:                  input.Zone,
			RecordName:            input.RecordName,
			RecordType:            input.RecordType,
			ManagedMode:           "CUSTOMER_CREDENTIAL",
			DesiredValuesJSON:     stringListJSON(input.DesiredValues),
			LastAppliedValuesJSON: "[]",
			CreatedAt:             now,
			UpdatedAt:             now,
		}
		providerAction, ok, err := service.buildDNSRecordProviderAction(ctx, repositories, identity.OrganizationID, record, input.DesiredValues, true)
		if err != nil {
			return err
		}
		if ok {
			action = providerAction
			hasAction = true
		}
		if err := repositories.DNSRecords().CreateDNSRecord(ctx, record); err != nil {
			return err
		}
		if input.HealthCheckID != "" {
			if err := service.createDNSHealthEvent(ctx, repositories, identity.OrganizationID, input, record, now); err != nil {
				return err
			}
		}
		result = toDNSRecordPayload(record)
		recordID = record.ID
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "dns_records.create", "DNS_RECORD", record.ID, ""))
	})
	if err != nil {
		return DNSRecordPayload{}, mapServiceError(err)
	}
	if hasAction {
		if err := service.executeDNSProviderAction(ctx, action); err != nil {
			if cleanupErr := service.deleteCreatedDNSRecordLocalState(ctx, identity.OrganizationID, recordID); cleanupErr != nil {
				return DNSRecordPayload{}, mapServiceError(cleanupErr)
			}
			return DNSRecordPayload{}, err
		}
		appliedAt := service.timestamp()
		err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
			return repositories.DNSRecords().UpdateDNSRecordLastApplied(ctx, identity.OrganizationID, recordID, action.LastAppliedValues, appliedAt)
		})
		if err != nil {
			return DNSRecordPayload{}, mapServiceError(err)
		}
		result.LastAppliedValues = parseStringListJSON(action.LastAppliedValues)
		result.LastAppliedAt = appliedAt
	}
	return result, nil
}

func (service *ControlService) UpdateDNSRecord(ctx context.Context, identity InternalIdentity, recordID string, input DNSRecordMutationInput) (DNSRecordPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return DNSRecordPayload{}, ErrForbidden
	}
	if err := service.ensureDNSRecordMutationAllowed(identity, input); err != nil {
		return DNSRecordPayload{}, err
	}
	var result DNSRecordPayload
	var retireAction dnsEventAction
	var hasRetireAction bool
	var applyAction dnsEventAction
	var hasApplyAction bool
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		record, err := repositories.DNSRecords().FindDNSRecordByID(ctx, identity.OrganizationID, recordID)
		if err != nil {
			return err
		}
		previousRecord := record
		if strings.TrimSpace(input.HealthCheckID) == "" {
			if err := service.ensureCanRemoveDNSHealthBinding(ctx, repositories, identity, record.ID); err != nil {
				return err
			}
		}
		if _, err := repositories.DNSCredentials().FindDNSCredentialByID(ctx, identity.OrganizationID, input.DNSCredentialID); err != nil {
			return err
		}
		record.DNSCredentialID = input.DNSCredentialID
		record.Zone = input.Zone
		record.RecordName = input.RecordName
		record.RecordType = input.RecordType
		record.ManagedMode = "CUSTOMER_CREDENTIAL"
		record.DesiredValuesJSON = stringListJSON(input.DesiredValues)
		record.UpdatedAt = service.timestamp()
		identityChanged := dnsRecordProviderIdentityChanged(previousRecord, record)
		if identityChanged {
			if dnsRecordHasPendingRetire(previousRecord) {
				return ErrConflict
			}
			record.LastAppliedValuesJSON = "[]"
			record.LastAppliedAt = ""
			setDNSRecordPendingRetire(&record, previousRecord)
		}
		action, ok, err := service.buildDNSRecordPendingRetireAction(ctx, repositories, identity.OrganizationID, record)
		if err != nil {
			return err
		}
		if ok {
			retireAction = action
			hasRetireAction = true
		}
		action, ok, err = service.buildDNSRecordProviderAction(ctx, repositories, identity.OrganizationID, record, input.DesiredValues, identityChanged)
		if err != nil {
			return err
		}
		if ok {
			applyAction = action
			hasApplyAction = true
		}
		if err := repositories.DNSRecords().UpdateDNSRecord(ctx, record); err != nil {
			return err
		}
		if err := repositories.HealthChecks().DeleteHealthEvaluationRulesForDNSRecord(ctx, identity.OrganizationID, record.ID, record.UpdatedAt); err != nil {
			return err
		}
		if input.HealthCheckID != "" {
			if err := service.createDNSHealthEvent(ctx, repositories, identity.OrganizationID, input, record, record.UpdatedAt); err != nil {
				return err
			}
		}
		result = toDNSRecordPayload(record)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "dns_records.update", "DNS_RECORD", record.ID, ""))
	})
	if err != nil {
		return DNSRecordPayload{}, mapServiceError(err)
	}
	if hasRetireAction {
		if err := service.executeDNSProviderAction(ctx, retireAction); err != nil {
			return DNSRecordPayload{}, err
		}
		clearedAt := service.timestamp()
		err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
			return repositories.DNSRecords().ClearDNSRecordPendingRetire(ctx, identity.OrganizationID, recordID, clearedAt)
		})
		if err != nil {
			return DNSRecordPayload{}, mapServiceError(err)
		}
	}
	if hasApplyAction {
		if err := service.executeDNSProviderAction(ctx, applyAction); err != nil {
			return DNSRecordPayload{}, err
		}
		appliedAt := service.timestamp()
		err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
			return repositories.DNSRecords().UpdateDNSRecordLastApplied(ctx, identity.OrganizationID, recordID, applyAction.LastAppliedValues, appliedAt)
		})
		if err != nil {
			return DNSRecordPayload{}, mapServiceError(err)
		}
		result.LastAppliedValues = parseStringListJSON(applyAction.LastAppliedValues)
		result.LastAppliedAt = appliedAt
	}
	return result, nil
}

func (service *ControlService) DeleteDNSRecord(ctx context.Context, identity InternalIdentity, recordID string) error {
	if !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return ErrForbidden
	}
	var action dnsEventAction
	var hasAction bool
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		record, err := repositories.DNSRecords().FindDNSRecordByID(ctx, identity.OrganizationID, recordID)
		if err != nil {
			return err
		}
		if err := service.ensureCanRemoveDNSHealthBinding(ctx, repositories, identity, recordID); err != nil {
			return err
		}
		providerAction, ok, err := service.buildDNSRecordProviderAction(ctx, repositories, identity.OrganizationID, record, nil, true)
		if err != nil {
			return err
		}
		if ok {
			action = providerAction
			hasAction = true
		}
		if hasAction {
			return repositories.DNSRecords().MarkDNSRecordProviderDeletePending(ctx, identity.OrganizationID, recordID, service.timestamp())
		}
		deletedAt := service.timestamp()
		if err := repositories.HealthChecks().DeleteHealthEvaluationRulesForDNSRecord(ctx, identity.OrganizationID, recordID, deletedAt); err != nil {
			return err
		}
		if err := repositories.DNSRecords().DeleteDNSRecord(ctx, identity.OrganizationID, recordID, deletedAt); err != nil {
			return err
		}
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "dns_records.delete", "DNS_RECORD", recordID, ""))
	})
	if err != nil {
		return mapServiceError(err)
	}
	if hasAction {
		if err := service.executeDNSProviderAction(ctx, action); err != nil {
			return err
		}
		err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
			deletedAt := service.timestamp()
			if err := repositories.HealthChecks().DeleteHealthEvaluationRulesForDNSRecord(ctx, identity.OrganizationID, recordID, deletedAt); err != nil {
				return err
			}
			if err := repositories.DNSRecords().DeleteDNSRecord(ctx, identity.OrganizationID, recordID, deletedAt); err != nil {
				return err
			}
			return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "dns_records.delete", "DNS_RECORD", recordID, ""))
		})
		return mapServiceError(err)
	}
	return nil
}

func (service *ControlService) ensureDNSRecordMutationAllowed(identity InternalIdentity, input DNSRecordMutationInput) error {
	if strings.EqualFold(strings.TrimSpace(input.RecordType), "CNAME") && (hasMultipleDistinctValues(input.DesiredValues) || hasMultipleDistinctValues(input.FailoverValues)) {
		return ErrInvalidInput
	}
	if strings.TrimSpace(input.HealthCheckID) == "" {
		return nil
	}
	if service.hasPermission(identity, string(domain.PermissionHealthChecksRead)) || service.hasPermission(identity, string(domain.PermissionHealthChecksManage)) {
		return nil
	}
	return ErrForbidden
}

func (service *ControlService) ensureCanRemoveDNSHealthBinding(ctx context.Context, repositories repo.Repositories, identity InternalIdentity, dnsRecordID string) error {
	if service.hasPermission(identity, string(domain.PermissionHealthChecksRead)) || service.hasPermission(identity, string(domain.PermissionHealthChecksManage)) {
		return nil
	}
	hasRules, err := dnsRecordHasHealthActions(ctx, repositories, identity.OrganizationID, dnsRecordID)
	if err != nil {
		return err
	}
	if hasRules {
		return ErrForbidden
	}
	return nil
}

func dnsRecordHasHealthActions(ctx context.Context, repositories repo.Repositories, organizationID string, dnsRecordID string) (bool, error) {
	checks, err := repositories.HealthChecks().ListHealthChecksByOrganization(ctx, organizationID)
	if err != nil {
		return false, err
	}
	for _, check := range checks {
		rules, err := repositories.HealthChecks().ListHealthEvaluationRulesByCheck(ctx, organizationID, check.ID)
		if err != nil {
			return false, err
		}
		for _, rule := range rules {
			for _, event := range rule.Events {
				if healthEventReferencesDNSRecord(event, dnsRecordID) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func healthEventReferencesDNSRecord(event repo.HealthEventRecord, dnsRecordID string) bool {
	var config dnsHealthActionConfig
	if err := json.Unmarshal([]byte(event.ConfigJSON), &config); err != nil {
		return false
	}
	return strings.TrimSpace(config.DNSRecordID) == dnsRecordID
}

func hasMultipleDistinctValues(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		if len(seen) > 1 {
			return true
		}
	}
	return false
}

func (service *ControlService) createDNSHealthEvent(ctx context.Context, repositories repo.Repositories, organizationID string, input DNSRecordMutationInput, record repo.DNSRecordRecord, now string) error {
	if _, err := repositories.HealthChecks().FindHealthCheckByID(ctx, organizationID, input.HealthCheckID); err != nil {
		return err
	}
	configJSON, err := json.Marshal(dnsHealthActionConfig{DNSRecordID: record.ID, FailoverValues: input.FailoverValues})
	if err != nil {
		return err
	}
	eventType := input.EventType
	if eventType == "" {
		eventType = "DNS_FAILOVER"
	}
	rule := repo.HealthEvaluationRuleRecord{
		ID:             service.newID(),
		OrganizationID: organizationID,
		HealthCheckID:  input.HealthCheckID,
		Name:           record.RecordName + " DNS failover",
		Enabled:        true,
		ExpressionJSON: `{"mode":"latest_result"}`,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	event := repo.HealthEventRecord{
		ID:                     service.newID(),
		OrganizationID:         organizationID,
		HealthEvaluationRuleID: rule.ID,
		EventType:              eventType,
		ConfigJSON:             string(configJSON),
		Enabled:                true,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	return repositories.HealthChecks().CreateHealthEvaluationRule(ctx, rule, []repo.HealthEventRecord{event})
}

func (service *ControlService) buildDNSRecordProviderAction(ctx context.Context, repositories repo.Repositories, organizationID string, record repo.DNSRecordRecord, values []string, force bool) (dnsEventAction, bool, error) {
	nextApplied := stringListJSON(values)
	lastApplied := stringListJSON(parseStringListJSON(record.LastAppliedValuesJSON))
	if !force && nextApplied == lastApplied {
		return dnsEventAction{}, false, nil
	}
	credential, err := repositories.DNSCredentials().FindDNSCredentialByID(ctx, organizationID, record.DNSCredentialID)
	if err != nil {
		return dnsEventAction{}, false, err
	}
	return dnsEventAction{
		OrganizationID:    organizationID,
		DNSRecordID:       record.ID,
		Provider:          credential.Provider,
		EncryptedSecret:   credential.EncryptedSecret,
		Zone:              record.Zone,
		RecordName:        record.RecordName,
		RecordType:        record.RecordType,
		Values:            values,
		LastAppliedValues: nextApplied,
	}, true, nil
}

func dnsRecordProviderIdentityChanged(previous repo.DNSRecordRecord, next repo.DNSRecordRecord) bool {
	return strings.TrimSpace(previous.DNSCredentialID) != strings.TrimSpace(next.DNSCredentialID) ||
		strings.TrimSpace(previous.Zone) != strings.TrimSpace(next.Zone) ||
		strings.TrimSpace(previous.RecordName) != strings.TrimSpace(next.RecordName) ||
		!strings.EqualFold(strings.TrimSpace(previous.RecordType), strings.TrimSpace(next.RecordType))
}

func normalizedConfigJSON(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "{}"
	}
	return value
}
