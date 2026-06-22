package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ApplyRecordInput struct {
	ProviderSecret string
	Zone           string
	RecordName     string
	RecordType     string
	Values         []string
}

type Provider interface {
	ApplyRecord(ctx context.Context, input ApplyRecordInput) error
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
	desired := normalizeStringSet(input.Values)
	if recordType == "CNAME" && len(desired) > 1 {
		return errors.New("CNAME records support exactly one value")
	}
	client := provider.HTTPClient
	if client == nil {
		client = defaultCloudflareHTTPClient()
	}
	baseURL := strings.TrimRight(provider.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.cloudflare.com/client/v4"
	}
	records, err := provider.listCloudflareRecords(ctx, client, baseURL, secret, zone, recordName, recordType)
	if err != nil {
		return err
	}
	if len(desired) == 0 {
		for _, record := range records {
			if err := provider.deleteCloudflareRecord(ctx, client, baseURL, secret, zone, record.ID); err != nil {
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
			continue
		}
		value := firstValue(remaining)
		if value == "" {
			if err := provider.deleteCloudflareRecord(ctx, client, baseURL, secret, zone, record.ID); err != nil {
				return err
			}
			continue
		}
		if err := provider.updateCloudflareRecord(ctx, client, baseURL, secret, zone, record.ID, recordName, recordType, value); err != nil {
			return err
		}
		delete(remaining, value)
	}
	for value := range remaining {
		if err := provider.createCloudflareRecord(ctx, client, baseURL, secret, zone, recordName, recordType, value); err != nil {
			return err
		}
	}
	return nil
}

type cloudflareRecord struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

type cloudflareResponse struct {
	Success    bool                 `json:"success"`
	Errors     []cloudflareError    `json:"errors"`
	Result     []cloudflareRecord   `json:"result"`
	ResultInfo cloudflareResultInfo `json:"result_info"`
}

type cloudflareResultInfo struct {
	Page       int `json:"page"`
	TotalPages int `json:"total_pages"`
}

type cloudflareWriteResponse struct {
	Success bool              `json:"success"`
	Errors  []cloudflareError `json:"errors"`
}

type cloudflareError struct {
	Message string `json:"message"`
}

func (provider CloudflareProvider) listCloudflareRecords(ctx context.Context, client *http.Client, baseURL string, secret string, zone string, recordName string, recordType string) ([]cloudflareRecord, error) {
	records := make([]cloudflareRecord, 0)
	for page := 1; ; page++ {
		query := url.Values{}
		query.Set("type", recordType)
		query.Set("name", recordName)
		query.Set("page", fmt.Sprintf("%d", page))
		query.Set("per_page", "100")
		endpoint := fmt.Sprintf("%s/zones/%s/dns_records?%s", baseURL, url.PathEscape(zone), query.Encode())
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Authorization", "Bearer "+secret)
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		body, err := decodeCloudflareResponse(response)
		if err != nil {
			return nil, err
		}
		records = append(records, body.Result...)
		totalPages := body.ResultInfo.TotalPages
		if totalPages <= 0 {
			totalPages = 1
		}
		if page >= totalPages {
			return records, nil
		}
	}
}

func decodeCloudflareResponse(response *http.Response) (cloudflareResponse, error) {
	defer func() { _ = response.Body.Close() }()
	var body cloudflareResponse
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return body, err
	}
	if len(data) != 0 {
		if err := json.Unmarshal(data, &body); err != nil {
			return body, err
		}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !body.Success {
		return body, errors.New(cloudflareErrorMessage(body.Errors, response.StatusCode))
	}
	return body, nil
}

func (provider CloudflareProvider) createCloudflareRecord(ctx context.Context, client *http.Client, baseURL string, secret string, zone string, recordName string, recordType string, value string) error {
	return provider.writeCloudflareRecord(ctx, client, secret, http.MethodPost, fmt.Sprintf("%s/zones/%s/dns_records", baseURL, url.PathEscape(zone)), recordName, recordType, value)
}

func (provider CloudflareProvider) updateCloudflareRecord(ctx context.Context, client *http.Client, baseURL string, secret string, zone string, recordID string, recordName string, recordType string, value string) error {
	if recordID == "" || value == "" {
		return nil
	}
	return provider.writeCloudflareRecord(ctx, client, secret, http.MethodPut, fmt.Sprintf("%s/zones/%s/dns_records/%s", baseURL, url.PathEscape(zone), url.PathEscape(recordID)), recordName, recordType, value)
}

func (provider CloudflareProvider) writeCloudflareRecord(ctx context.Context, client *http.Client, secret string, method string, endpoint string, recordName string, recordType string, value string) error {
	payload := map[string]any{"name": recordName, "type": recordType, "content": value, "ttl": 60, "proxied": false}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+secret)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	var responseBody cloudflareWriteResponse
	_ = json.NewDecoder(response.Body).Decode(&responseBody)
	if response.StatusCode < 200 || response.StatusCode >= 300 || !responseBody.Success {
		return errors.New(cloudflareErrorMessage(responseBody.Errors, response.StatusCode))
	}
	return nil
}

func (provider CloudflareProvider) deleteCloudflareRecord(ctx context.Context, client *http.Client, baseURL string, secret string, zone string, recordID string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("%s/zones/%s/dns_records/%s", baseURL, url.PathEscape(zone), url.PathEscape(recordID)), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+secret)
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("cloudflare dns delete failed with status %d", response.StatusCode)
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

func cloudflareErrorMessage(errors []cloudflareError, status int) string {
	if len(errors) == 0 || strings.TrimSpace(errors[0].Message) == "" {
		return fmt.Sprintf("cloudflare dns request failed with status %d", status)
	}
	return errors[0].Message
}
