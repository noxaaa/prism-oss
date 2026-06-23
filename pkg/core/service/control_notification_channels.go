package service

import (
	"context"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func (service *ControlService) CreateNotificationChannel(ctx context.Context, identity InternalIdentity, input NotificationChannelMutationInput) (NotificationChannelPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return NotificationChannelPayload{}, ErrForbidden
	}
	var result NotificationChannelPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		channel, replaceSecret, err := service.buildNotificationChannel(repo.NotificationChannelRecord{
			ID:             service.newID(),
			OrganizationID: identity.OrganizationID,
		}, input, true)
		if err != nil {
			return err
		}
		now := service.timestamp()
		channel.CreatedAt = now
		channel.UpdatedAt = now
		if err := repositories.DNSRecords().CreateNotificationChannel(ctx, channel); err != nil {
			return err
		}
		_ = replaceSecret
		result = toNotificationChannelPayload(channel)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "notification_channels.create", "NOTIFICATION_CHANNEL", channel.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) UpdateNotificationChannel(ctx context.Context, identity InternalIdentity, channelID string, input NotificationChannelMutationInput) (NotificationChannelPayload, error) {
	if !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return NotificationChannelPayload{}, ErrForbidden
	}
	var result NotificationChannelPayload
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		channel, err := repositories.DNSRecords().FindNotificationChannelByID(ctx, identity.OrganizationID, channelID)
		if err != nil {
			return err
		}
		channel, replaceSecret, err := service.buildNotificationChannel(channel, input, false)
		if err != nil {
			return err
		}
		channel.UpdatedAt = service.timestamp()
		if err := repositories.DNSRecords().UpdateNotificationChannel(ctx, channel, replaceSecret); err != nil {
			return err
		}
		result = toNotificationChannelPayload(channel)
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "notification_channels.update", "NOTIFICATION_CHANNEL", channel.ID, ""))
	})
	return result, mapServiceError(err)
}

func (service *ControlService) DeleteNotificationChannel(ctx context.Context, identity InternalIdentity, channelID string) error {
	if !service.hasPermission(identity, string(domain.PermissionDNSManage)) {
		return ErrForbidden
	}
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		channel, err := repositories.DNSRecords().FindNotificationChannelByID(ctx, identity.OrganizationID, channelID)
		if err != nil {
			return err
		}
		now := service.timestamp()
		if err := repositories.DNSRecords().DeleteNotificationChannel(ctx, identity.OrganizationID, channel.ID, now); err != nil {
			return err
		}
		return service.writeAudit(ctx, repositories, service.auditForIdentity(identity, "notification_channels.delete", "NOTIFICATION_CHANNEL", channel.ID, ""))
	})
	return mapServiceError(err)
}

func (service *ControlService) buildNotificationChannel(channel repo.NotificationChannelRecord, input NotificationChannelMutationInput, secretRequired bool) (repo.NotificationChannelRecord, bool, error) {
	name := strings.TrimSpace(input.Name)
	channelType := strings.ToUpper(strings.TrimSpace(input.ChannelType))
	if name == "" || len(name) > 120 || (channelType != "WEBHOOK" && channelType != "EMAIL") {
		return repo.NotificationChannelRecord{}, false, ErrInvalidInput
	}
	configJSON, err := jsonObjectString(input.Config)
	if err != nil {
		return repo.NotificationChannelRecord{}, false, err
	}
	replaceSecret := strings.TrimSpace(input.Secret) != ""
	typeChanged := channel.ID != "" && channel.ChannelType != "" && channel.ChannelType != channelType
	if secretRequired && channelType == "EMAIL" && !replaceSecret {
		return repo.NotificationChannelRecord{}, false, ErrInvalidInput
	}
	if typeChanged && channelType == "EMAIL" && !replaceSecret {
		return repo.NotificationChannelRecord{}, false, ErrInvalidInput
	}
	if replaceSecret {
		encrypted, err := service.encryptDNSSecret(input.Secret)
		if err != nil {
			return repo.NotificationChannelRecord{}, false, err
		}
		channel.EncryptedSecret = encrypted
	} else if typeChanged {
		channel.EncryptedSecret = ""
		replaceSecret = true
	}
	channel.Name = name
	channel.ChannelType = channelType
	channel.ConfigJSON = configJSON
	channel.Enabled = input.Enabled
	return channel, replaceSecret, nil
}
