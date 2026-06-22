package service

import (
	"context"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func (service *ControlService) deleteCreatedDNSRecordLocalState(ctx context.Context, organizationID string, recordID string) error {
	deletedAt := service.timestamp()
	return service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if err := repositories.HealthChecks().DeleteHealthEvaluationRulesForDNSRecord(ctx, organizationID, recordID, deletedAt); err != nil {
			return err
		}
		return repositories.DNSRecords().DeleteDNSRecord(ctx, organizationID, recordID, deletedAt)
	})
}
