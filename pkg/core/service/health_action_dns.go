package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/dns"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
)

type dnsHealthActionConfig struct {
	DNSRecordID    string   `json:"dns_record_id"`
	FailoverValues []string `json:"failover_values,omitempty"`
}

type dnsEventAction struct {
	OrganizationID    string
	DNSRecordID       string
	Provider          string
	EncryptedSecret   string
	Zone              string
	RecordName        string
	RecordType        string
	Values            []string
	LastAppliedValues string
}

type dnsHealthActionExecutor struct {
	service *ControlService
}

func (executor dnsHealthActionExecutor) Supports(eventType string) bool {
	switch strings.ToUpper(strings.TrimSpace(eventType)) {
	case "DNS_FAILOVER", "DNS_DELETE_OFFLINE", "DNS_DELETE_ALL", "DNS_RESTORE":
		return true
	default:
		return false
	}
}

func (executor dnsHealthActionExecutor) BuildAction(ctx context.Context, repositories repo.Repositories, input HealthActionExecutionInput) (any, bool, error) {
	var config dnsHealthActionConfig
	if err := json.Unmarshal([]byte(input.Event.ConfigJSON), &config); err != nil {
		return dnsEventAction{}, false, err
	}
	if strings.TrimSpace(config.DNSRecordID) == "" {
		return dnsEventAction{}, false, ErrInvalidInput
	}
	record, err := repositories.DNSRecords().FindDNSRecordByID(ctx, input.OrganizationID, config.DNSRecordID)
	if err != nil {
		return dnsEventAction{}, false, err
	}
	desiredValues := parseStringListJSON(record.DesiredValuesJSON)
	values := desiredValues
	status := strings.ToUpper(strings.TrimSpace(input.Result.Status))
	switch strings.ToUpper(strings.TrimSpace(input.Event.EventType)) {
	case "DNS_FAILOVER":
		if status == "OFFLINE" {
			values = config.FailoverValues
		}
	case "DNS_DELETE_OFFLINE":
		if status == "OFFLINE" {
			values = nil
		}
	case "DNS_DELETE_ALL":
		values = nil
	case "DNS_RESTORE":
		if status != "ONLINE" {
			return dnsEventAction{}, false, nil
		}
	default:
		return dnsEventAction{}, false, ErrInvalidInput
	}
	lastApplied := stringListJSON(parseStringListJSON(record.LastAppliedValuesJSON))
	nextApplied := stringListJSON(values)
	if lastApplied == nextApplied {
		return dnsEventAction{}, false, nil
	}
	credential, err := repositories.DNSCredentials().FindDNSCredentialByID(ctx, input.OrganizationID, record.DNSCredentialID)
	if err != nil {
		return dnsEventAction{}, false, err
	}
	return dnsEventAction{
		OrganizationID:    input.OrganizationID,
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

func (executor dnsHealthActionExecutor) Execute(ctx context.Context, rawAction any) error {
	action, ok := rawAction.(dnsEventAction)
	if !ok {
		return ErrInvalidInput
	}
	secret, err := executor.service.decryptDNSSecret(action.EncryptedSecret)
	if err != nil {
		return err
	}
	provider, ok := executor.service.dnsProviders.ProviderForKey(action.Provider)
	if !ok {
		return ErrInvalidInput
	}
	if err := provider.ApplyRecord(ctx, dns.ApplyRecordInput{
		ProviderSecret: secret,
		Zone:           action.Zone,
		RecordName:     action.RecordName,
		RecordType:     action.RecordType,
		Values:         action.Values,
	}); err != nil {
		return err
	}
	err = executor.service.store.WithinTx(ctx, func(ctx context.Context, repositories repo.Repositories) error {
		return repositories.DNSRecords().UpdateDNSRecordLastApplied(ctx, action.OrganizationID, action.DNSRecordID, action.LastAppliedValues, executor.service.timestamp())
	})
	return mapServiceError(err)
}

func stringListJSON(values []string) string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	sort.Strings(normalized)
	data, err := json.Marshal(normalized)
	if err != nil {
		return "[]"
	}
	return string(data)
}
