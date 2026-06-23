package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/dns"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

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
		zones, err := repositories.DNSCredentials().ListDNSCredentialZonesByOrganization(ctx, identity.OrganizationID)
		if err != nil {
			return err
		}
		result = toDNSCredentialPayloads(credentials, zones)
		return nil
	})
	return result, mapServiceError(err)
}

func (service *ControlService) CreateDNSCredential(ctx context.Context, identity InternalIdentity, input DNSCredentialMutationInput) (DNSCredentialPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return DNSCredentialPayload{}, ErrForbidden
	}
	zones, err := service.discoverDNSCredentialZones(ctx, input.Provider, input.Secret)
	if err != nil {
		return DNSCredentialPayload{}, err
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
		if err := repositories.DNSCredentials().ReplaceDNSCredentialZones(ctx, identity.OrganizationID, credential.ID, zones, now, service.newID); err != nil {
			return err
		}
		storedZones, err := repositories.DNSCredentials().ListDNSCredentialZonesByCredential(ctx, identity.OrganizationID, credential.ID)
		if err != nil {
			return err
		}
		result = toDNSCredentialPayload(credential, storedZones)
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
	var discoveredZones []repo.DNSCredentialZoneRecord
	if strings.TrimSpace(input.Secret) != "" {
		zones, err := service.discoverDNSCredentialZones(ctx, input.Provider, input.Secret)
		if err != nil {
			return DNSCredentialPayload{}, err
		}
		discoveredZones = zones
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
		if replaceSecret {
			if err := repositories.DNSCredentials().ReplaceDNSCredentialZones(ctx, identity.OrganizationID, credential.ID, discoveredZones, credential.UpdatedAt, service.newID); err != nil {
				return err
			}
		}
		zones, err := repositories.DNSCredentials().ListDNSCredentialZonesByCredential(ctx, identity.OrganizationID, credential.ID)
		if err != nil {
			return err
		}
		result = toDNSCredentialPayload(credential, zones)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "dns_credentials.update", "DNS_CREDENTIAL", credential.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) RefreshDNSCredentialZones(ctx context.Context, identity InternalIdentity, credentialID string) (DNSCredentialPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return DNSCredentialPayload{}, ErrForbidden
	}
	var credential repo.DNSCredentialRecord
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		var err error
		credential, err = repositories.DNSCredentials().FindDNSCredentialByID(ctx, identity.OrganizationID, credentialID)
		return err
	})
	if err != nil {
		return DNSCredentialPayload{}, mapServiceError(err)
	}
	secret, err := service.decryptDNSSecret(credential.EncryptedSecret)
	if err != nil {
		return DNSCredentialPayload{}, err
	}
	zones, err := service.discoverDNSCredentialZones(ctx, credential.Provider, secret)
	if err != nil {
		return DNSCredentialPayload{}, err
	}
	var result DNSCredentialPayload
	err = service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		current, err := repositories.DNSCredentials().FindDNSCredentialByID(ctx, identity.OrganizationID, credentialID)
		if err != nil {
			return err
		}
		if current.Provider != credential.Provider || current.EncryptedSecret != credential.EncryptedSecret {
			return ErrConflict
		}
		now := service.timestamp()
		if err := repositories.DNSCredentials().ReplaceDNSCredentialZones(ctx, identity.OrganizationID, current.ID, zones, now, service.newID); err != nil {
			return err
		}
		storedZones, err := repositories.DNSCredentials().ListDNSCredentialZonesByCredential(ctx, identity.OrganizationID, current.ID)
		if err != nil {
			return err
		}
		result = toDNSCredentialPayload(current, storedZones)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "dns_credentials.refresh_zones", "DNS_CREDENTIAL", current.ID, ""))
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
	records, err := repositories.DNSRecords().ListDNSManagedRecordsByOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.DNSCredentialID == credentialID {
			return ErrConflict
		}
	}
	return nil
}

func (service *ControlService) latestHealthResultsByCheck(ctx context.Context, repositories repo.Repositories, organizationID string, checks []repo.HealthCheckRecord) (map[string][]repo.HealthResultRecord, error) {
	checkIDs := make([]string, 0, len(checks))
	for _, check := range checks {
		checkIDs = append(checkIDs, check.ID)
	}
	return repositories.HealthChecks().ListLatestHealthResultsByChecks(ctx, organizationID, checkIDs)
}

func (service *ControlService) discoverDNSCredentialZones(ctx context.Context, providerKey string, secret string) ([]repo.DNSCredentialZoneRecord, error) {
	providerKey = strings.ToUpper(strings.TrimSpace(providerKey))
	secret = strings.TrimSpace(secret)
	if providerKey == "" || secret == "" {
		return nil, ErrInvalidInput
	}
	provider, ok := service.dnsProviders.ProviderForKey(providerKey)
	if !ok {
		return nil, ErrInvalidInput
	}
	lister, ok := provider.(dns.ZoneLister)
	if !ok {
		return nil, ErrInvalidInput
	}
	zones, err := lister.ListZones(ctx, secret)
	if err != nil {
		return nil, validationFieldError("secret", "Could not discover DNS zones for this credential. Confirm the Cloudflare token has Zone Read and DNS Edit permissions.", map[string]any{
			"provider": providerKey,
		})
	}
	if len(zones) == 0 {
		return nil, validationFieldError("secret", "No DNS zones were discovered for this credential. Confirm the Cloudflare token can read at least one zone.", map[string]any{
			"provider": providerKey,
		})
	}
	records := make([]repo.DNSCredentialZoneRecord, 0, len(zones))
	for _, zone := range zones {
		zoneID := strings.TrimSpace(zone.ID)
		zoneName := strings.ToLower(strings.Trim(strings.TrimSpace(zone.Name), "."))
		if zoneID == "" || zoneName == "" {
			continue
		}
		records = append(records, repo.DNSCredentialZoneRecord{
			ZoneID:   zoneID,
			ZoneName: zoneName,
			Status:   normalizeDNSZoneStatus(zone.Status),
		})
	}
	if len(records) == 0 {
		return nil, validationFieldError("secret", "No usable DNS zones were discovered for this credential. Confirm the Cloudflare zones have valid names and IDs.", map[string]any{
			"provider": providerKey,
		})
	}
	return records, nil
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

func normalizeDNSZoneStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "ACTIVE", "PENDING", "MOVED", "DELETED", "DEACTIVATED", "READ_ONLY", "UNAVAILABLE":
		return strings.ToUpper(strings.TrimSpace(status))
	default:
		return "UNKNOWN"
	}
}
