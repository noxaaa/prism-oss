package dns

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCloudflareProviderRejectsMultipleCNAMEValues(t *testing.T) {
	provider := CloudflareProvider{HTTPClient: &http.Client{}}

	err := provider.ApplyRecord(context.Background(), ApplyRecordInput{
		ProviderSecret: "token",
		Zone:           "zone_1",
		RecordName:     "alias.example.com",
		RecordType:     "CNAME",
		Values:         []string{"origin-a.example.com", "origin-b.example.com"},
	})
	if err == nil || !strings.Contains(err.Error(), "CNAME") {
		t.Fatalf("expected multiple CNAME values to be rejected, got %v", err)
	}
}

func TestCloudflareProviderDefaultClientHasTimeout(t *testing.T) {
	registry := DefaultProviderRegistry()
	provider, ok := registry.ProviderForKey("CLOUDFLARE")
	if !ok {
		t.Fatalf("expected default Cloudflare provider")
	}
	cloudflare, ok := provider.(CloudflareProvider)
	if !ok {
		t.Fatalf("expected CloudflareProvider, got %T", provider)
	}
	if cloudflare.HTTPClient == nil || cloudflare.HTTPClient.Timeout <= 0 || cloudflare.HTTPClient.Timeout > 30*time.Second {
		t.Fatalf("expected bounded Cloudflare HTTP client timeout, got %#v", cloudflare.HTTPClient)
	}
}

func TestCloudflareProviderPagesThroughRecords(t *testing.T) {
	var listCalls int
	var deleted []string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.Method {
		case http.MethodGet:
			listCalls++
			page := request.URL.Query().Get("page")
			if page == "" || page == "1" {
				return jsonResponse(200, map[string]any{
					"success": true,
					"result":  []map[string]string{{"id": "record_1", "content": "192.0.2.1"}},
					"result_info": map[string]int{
						"page":        1,
						"total_pages": 2,
					},
				}), nil
			}
			return jsonResponse(200, map[string]any{
				"success": true,
				"result":  []map[string]string{{"id": "record_2", "content": "192.0.2.2"}},
				"result_info": map[string]int{
					"page":        2,
					"total_pages": 2,
				},
			}), nil
		case http.MethodDelete:
			parts := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
			deleted = append(deleted, parts[len(parts)-1])
			return jsonResponse(200, map[string]any{"success": true}), nil
		default:
			t.Fatalf("unexpected method %s", request.Method)
			return nil, nil
		}
	})}

	err := (CloudflareProvider{HTTPClient: client, BaseURL: "https://cloudflare.test/client/v4"}).ApplyRecord(context.Background(), ApplyRecordInput{
		ProviderSecret: "token",
		Zone:           "zone_1",
		RecordName:     "app.example.com",
		RecordType:     "A",
		Values:         nil,
	})
	if err != nil {
		t.Fatalf("apply delete record: %v", err)
	}
	if listCalls != 2 {
		t.Fatalf("expected both Cloudflare pages to be listed, got %d calls", listCalls)
	}
	if len(deleted) != 2 || deleted[0] != "record_1" || deleted[1] != "record_2" {
		t.Fatalf("expected both paged records to be deleted, got %#v", deleted)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func jsonResponse(status int, payload any) *http.Response {
	data, _ := json.Marshal(payload)
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(data))),
	}
}
