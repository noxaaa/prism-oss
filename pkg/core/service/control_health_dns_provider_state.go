package service

import (
	"context"
	"strings"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

func clampFutureObservedAt(observedAt string, serverNow string) string {
	observedAt = strings.TrimSpace(observedAt)
	serverNow = strings.TrimSpace(serverNow)
	if observedAt == "" {
		return serverNow
	}
	observedTime, err := time.Parse(time.RFC3339Nano, observedAt)
	if err != nil {
		return serverNow
	}
	serverTime, err := time.Parse(time.RFC3339Nano, serverNow)
	if err != nil {
		return observedAt
	}
	if observedTime.After(serverTime) {
		return serverNow
	}
	return observedAt
}

func (service *ControlService) buildDNSRecordPendingRetireAction(ctx context.Context, repositories repo.Repositories, organizationID string, record repo.DNSRecordRecord) (dnsEventAction, bool, error) {
	if !dnsRecordHasPendingRetire(record) {
		return dnsEventAction{}, false, nil
	}
	credential, err := repositories.DNSCredentials().FindDNSCredentialByID(ctx, organizationID, record.PendingRetireDNSCredentialID)
	if err != nil {
		return dnsEventAction{}, false, err
	}
	return dnsEventAction{
		OrganizationID:    organizationID,
		DNSRecordID:       record.ID,
		Provider:          credential.Provider,
		EncryptedSecret:   credential.EncryptedSecret,
		Zone:              record.PendingRetireZone,
		RecordName:        record.PendingRetireRecordName,
		RecordType:        record.PendingRetireRecordType,
		Values:            nil,
		LastAppliedValues: "[]",
	}, true, nil
}

func setDNSRecordPendingRetire(record *repo.DNSRecordRecord, previous repo.DNSRecordRecord) {
	record.PendingRetireDNSCredentialID = previous.DNSCredentialID
	record.PendingRetireZone = previous.Zone
	record.PendingRetireRecordName = previous.RecordName
	record.PendingRetireRecordType = previous.RecordType
	record.PendingRetireValuesJSON = stringListJSON(parseStringListJSON(previous.LastAppliedValuesJSON))
	record.PendingRetireAt = record.UpdatedAt
}

func dnsRecordHasPendingRetire(record repo.DNSRecordRecord) bool {
	return strings.TrimSpace(record.PendingRetireDNSCredentialID) != "" &&
		strings.TrimSpace(record.PendingRetireZone) != "" &&
		strings.TrimSpace(record.PendingRetireRecordName) != "" &&
		strings.TrimSpace(record.PendingRetireRecordType) != ""
}
