package service

import (
	"context"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func (service *ControlService) RecordNodeTrafficReport(ctx context.Context, organizationID string, nodeID string, input AgentTrafficReportInput) (bool, error) {
	if input.ReportID == "" || len(input.Deltas) == 0 {
		return false, nil
	}
	var recorded bool
	err := service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		if _, err := repositories.Nodes().FindNodeByID(ctx, organizationID, nodeID); err != nil {
			return err
		}
		deltas := make([]repo.RuleTrafficDeltaRecord, 0, len(input.Deltas))
		for _, delta := range input.Deltas {
			deltas = append(deltas, repo.RuleTrafficDeltaRecord{
				RuleID:         delta.RuleID,
				UploadBytes:    delta.UploadBytes,
				DownloadBytes:  delta.DownloadBytes,
				TCPConnections: delta.TCPConnections,
				UDPPackets:     delta.UDPPackets,
			})
		}
		ok, err := repositories.Rules().RecordRuleTrafficReport(ctx, organizationID, nodeID, repo.RuleTrafficReportRecord{
			OrganizationID: organizationID,
			AgentID:        nodeID,
			ReportID:       input.ReportID,
		}, deltas, service.timestamp(), service.newID)
		if err != nil {
			return err
		}
		recorded = ok
		return nil
	})
	return recorded, mapServiceError(err)
}
