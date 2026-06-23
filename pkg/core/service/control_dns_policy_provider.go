package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/noxaaa/prism-oss/pkg/core/dns"
)

type dnsProviderApplyAction struct {
	Provider        string
	EncryptedSecret string
	Zone            string
	RecordName      string
	RecordType      string
	Values          []string
	TTL             int
	Proxied         bool
}

func (service *ControlService) executeDNSProviderAction(ctx context.Context, action dnsProviderApplyAction) error {
	secret, err := service.decryptDNSSecret(action.EncryptedSecret)
	if err != nil {
		return err
	}
	provider, ok := service.dnsProviders.ProviderForKey(action.Provider)
	if !ok {
		return ErrInvalidInput
	}
	return provider.ApplyRecord(ctx, dns.ApplyRecordInput{
		ProviderSecret: secret,
		Zone:           action.Zone,
		RecordName:     action.RecordName,
		RecordType:     action.RecordType,
		Values:         action.Values,
		TTL:            action.TTL,
		Proxied:        action.Proxied,
	})
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
