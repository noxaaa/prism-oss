package dns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	cloudflare "github.com/cloudflare/cloudflare-go/v7"
	cfdns "github.com/cloudflare/cloudflare-go/v7/dns"
	"github.com/cloudflare/cloudflare-go/v7/option"
	cfzones "github.com/cloudflare/cloudflare-go/v7/zones"
)

type ApplyRecordInput struct {
	ProviderSecret string
	Zone           string
	RecordName     string
	RecordType     string
	Values         []string
	TTL            int
	Proxied        bool
}

type Provider interface {
	ApplyRecord(ctx context.Context, input ApplyRecordInput) error
}

type ZoneInfo struct {
	ID     string
	Name   string
	Status string
}

type ZoneLister interface {
	ListZones(ctx context.Context, providerSecret string) ([]ZoneInfo, error)
}

type ProviderRegistry interface {
	ProviderForKey(provider string) (Provider, bool)
}

type StaticProviderRegistry map[string]Provider

func (registry StaticProviderRegistry) ProviderForKey(provider string) (Provider, bool) {
	provider = strings.ToUpper(strings.TrimSpace(provider))
	value, ok := registry[provider]
	return value, ok
}

func DefaultProviderRegistry() ProviderRegistry {
	return StaticProviderRegistry{
		"CLOUDFLARE": CloudflareProvider{HTTPClient: defaultCloudflareHTTPClient()},
	}
}

type CloudflareProvider struct {
	HTTPClient *http.Client
	BaseURL    string
}

const cloudflareProductionBaseURL = "https://api.cloudflare.com/client/v4"

func defaultCloudflareHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

func (provider CloudflareProvider) ApplyRecord(ctx context.Context, input ApplyRecordInput) error {
	secret := strings.TrimSpace(input.ProviderSecret)
	zone := strings.TrimSpace(input.Zone)
	recordName := strings.TrimSpace(input.RecordName)
	recordType := strings.ToUpper(strings.TrimSpace(input.RecordType))
	if secret == "" || zone == "" || recordName == "" || recordType == "" {
		return errors.New("invalid dns record input")
	}
	ttl := input.TTL
	if ttl <= 0 {
		ttl = 60
	}
	if input.Proxied {
		ttl = 1
	}
	desired := normalizeStringSet(input.Values)
	if recordType == "CNAME" && len(desired) > 1 {
		return errors.New("CNAME records support exactly one value")
	}
	client := provider.newCloudflareClient(secret)
	records, err := provider.listCloudflareRecords(ctx, client, zone, recordName, recordType)
	if err != nil {
		return err
	}
	if len(desired) == 0 {
		for _, record := range records {
			if err := provider.deleteCloudflareRecord(ctx, client, zone, record.ID); err != nil {
				return err
			}
		}
		return nil
	}
	remaining := make(map[string]bool, len(desired))
	for _, value := range desired {
		remaining[value] = true
	}
	for _, record := range records {
		if remaining[record.Content] {
			delete(remaining, record.Content)
			if !cloudflareRecordSettingsMatch(record, ttl, input.Proxied) {
				if err := provider.updateCloudflareRecord(ctx, client, zone, record.ID, recordName, recordType, record.Content, ttl, input.Proxied); err != nil {
					return err
				}
			}
			continue
		}
		value := firstValue(remaining)
		if value == "" {
			if err := provider.deleteCloudflareRecord(ctx, client, zone, record.ID); err != nil {
				return err
			}
			continue
		}
		if err := provider.updateCloudflareRecord(ctx, client, zone, record.ID, recordName, recordType, value, ttl, input.Proxied); err != nil {
			return err
		}
		delete(remaining, value)
	}
	for value := range remaining {
		if err := provider.createCloudflareRecord(ctx, client, zone, recordName, recordType, value, ttl, input.Proxied); err != nil {
			return err
		}
	}
	return nil
}

func cloudflareRecordSettingsMatch(record cloudflareRecord, ttl int, proxied bool) bool {
	recordTTL := record.TTL
	if recordTTL == 0 {
		recordTTL = ttl
	}
	return recordTTL == ttl && record.Proxied == proxied
}

func (provider CloudflareProvider) ListZones(ctx context.Context, providerSecret string) ([]ZoneInfo, error) {
	secret := strings.TrimSpace(providerSecret)
	if secret == "" {
		return nil, errors.New("invalid dns credential input")
	}
	client := provider.newCloudflareClient(secret)
	pager := client.Zones.ListAutoPaging(ctx, cfzones.ZoneListParams{})
	zones := make([]ZoneInfo, 0)
	for pager.Next() {
		zone := pager.Current()
		zoneID := strings.TrimSpace(zone.ID)
		zoneName := strings.TrimSpace(zone.Name)
		if zoneID == "" || zoneName == "" {
			continue
		}
		zones = append(zones, ZoneInfo{
			ID:     zoneID,
			Name:   strings.ToLower(zoneName),
			Status: strings.ToUpper(strings.TrimSpace(string(zone.Status))),
		})
	}
	if err := pager.Err(); err != nil {
		return nil, fmt.Errorf("cloudflare dns list zones: %w", err)
	}
	sort.Slice(zones, func(left, right int) bool {
		if zones[left].Name == zones[right].Name {
			return zones[left].ID < zones[right].ID
		}
		return zones[left].Name < zones[right].Name
	})
	return zones, nil
}

type cloudflareRecord struct {
	ID      string
	Content string
	TTL     int
	Proxied bool
}

func (provider CloudflareProvider) newCloudflareClient(secret string) *cloudflare.Client {
	client := provider.HTTPClient
	if client == nil {
		client = defaultCloudflareHTTPClient()
	}
	baseURL := strings.TrimSpace(provider.BaseURL)
	if baseURL == "" {
		baseURL = cloudflareProductionBaseURL
	}
	options := []option.RequestOption{
		option.WithBaseURL(baseURL),
		option.WithHeaderDel("X-Auth-Key"),
		option.WithHeaderDel("X-Auth-Email"),
		option.WithHeaderDel("X-Auth-User-Service-Key"),
		option.WithAPIToken(secret),
		option.WithHTTPClient(client),
		option.WithMaxRetries(0),
	}
	return cloudflare.NewClient(options...)
}

func (provider CloudflareProvider) listCloudflareRecords(ctx context.Context, client *cloudflare.Client, zone string, recordName string, recordType string) ([]cloudflareRecord, error) {
	records := make([]cloudflareRecord, 0)
	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("cloudflare dns list records: %w", err)
		}
		result, err := client.DNS.Records.List(ctx, cfdns.RecordListParams{
			ZoneID:  cloudflare.F(zone),
			Match:   cloudflare.F(cfdns.RecordListParamsMatchAll),
			Name:    cloudflare.F(cfdns.RecordListParamsName{Exact: cloudflare.F(recordName)}),
			Type:    cloudflare.F(cfdns.RecordListParamsType(recordType)),
			Page:    cloudflare.F(float64(page)),
			PerPage: cloudflare.F(float64(100)),
		})
		if err != nil {
			return nil, fmt.Errorf("cloudflare dns list records: %w", err)
		}
		totalPages := cloudflareTotalPages(result.JSON.RawJSON())
		if len(result.Result) == 0 {
			return records, nil
		}
		for _, record := range result.Result {
			records = append(records, cloudflareRecord{ID: record.ID, Content: record.Content, TTL: int(record.TTL), Proxied: record.Proxied})
		}
		if totalPages > 0 && page >= totalPages {
			return records, nil
		}
	}
}

func cloudflareTotalPages(raw string) int {
	if raw == "" {
		return 0
	}
	var envelope struct {
		ResultInfo struct {
			TotalPages float64 `json:"total_pages"`
		} `json:"result_info"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil || envelope.ResultInfo.TotalPages <= 0 {
		return 0
	}
	return int(envelope.ResultInfo.TotalPages)
}

func (provider CloudflareProvider) createCloudflareRecord(ctx context.Context, client *cloudflare.Client, zone string, recordName string, recordType string, value string, ttl int, proxied bool) error {
	_, err := client.DNS.Records.New(ctx, cfdns.RecordNewParams{
		ZoneID: cloudflare.F(zone),
		Body:   cloudflareRecordNewBody(recordName, recordType, value, ttl, proxied),
	})
	if err != nil {
		return fmt.Errorf("cloudflare dns create record: %w", err)
	}
	return nil
}

func (provider CloudflareProvider) updateCloudflareRecord(ctx context.Context, client *cloudflare.Client, zone string, recordID string, recordName string, recordType string, value string, ttl int, proxied bool) error {
	if recordID == "" || value == "" {
		return nil
	}
	_, err := client.DNS.Records.Update(ctx, recordID, cfdns.RecordUpdateParams{
		ZoneID: cloudflare.F(zone),
		Body:   cloudflareRecordUpdateBody(recordName, recordType, value, ttl, proxied),
	})
	if err != nil {
		return fmt.Errorf("cloudflare dns update record: %w", err)
	}
	return nil
}

func cloudflareRecordNewBody(recordName string, recordType string, value string, ttl int, proxied bool) cfdns.RecordNewParamsBody {
	return cfdns.RecordNewParamsBody{
		Name:    cloudflare.F(recordName),
		TTL:     cloudflare.F(cfdns.TTL(ttl)),
		Type:    cloudflare.F(cfdns.RecordNewParamsBodyType(recordType)),
		Content: cloudflare.F(value),
		Proxied: cloudflare.F(proxied),
	}
}

func cloudflareRecordUpdateBody(recordName string, recordType string, value string, ttl int, proxied bool) cfdns.RecordUpdateParamsBody {
	return cfdns.RecordUpdateParamsBody{
		Name:    cloudflare.F(recordName),
		TTL:     cloudflare.F(cfdns.TTL(ttl)),
		Type:    cloudflare.F(cfdns.RecordUpdateParamsBodyType(recordType)),
		Content: cloudflare.F(value),
		Proxied: cloudflare.F(proxied),
	}
}

func (provider CloudflareProvider) deleteCloudflareRecord(ctx context.Context, client *cloudflare.Client, zone string, recordID string) error {
	_, err := client.DNS.Records.Delete(ctx, recordID, cfdns.RecordDeleteParams{ZoneID: cloudflare.F(zone)})
	if err != nil {
		return fmt.Errorf("cloudflare dns delete record: %w", err)
	}
	return nil
}

func normalizeStringSet(values []string) []string {
	seen := make(map[string]bool, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	return normalized
}

func firstValue(values map[string]bool) string {
	for value := range values {
		return value
	}
	return ""
}
