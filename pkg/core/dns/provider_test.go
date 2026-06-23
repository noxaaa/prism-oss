package dns

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestCloudflareProviderDiscoversAccessibleZones(t *testing.T) {
	var listCalls int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("expected bearer auth header, got %q", request.Header.Get("Authorization"))
		}
		if request.Method != http.MethodGet || request.URL.Path != "/client/v4/zones" {
			t.Fatalf("unexpected zone request %s %s", request.Method, request.URL.Path)
		}
		listCalls++
		switch request.URL.Query().Get("page") {
		case "", "1":
			writeJSON(response, http.StatusOK, map[string]any{
				"success": true,
				"errors":  []any{},
				"result": []map[string]any{
					{"id": "zone_1", "name": "example.com", "status": "active"},
				},
				"result_info": map[string]any{"page": 1, "total_pages": 2},
			})
		case "2":
			writeJSON(response, http.StatusOK, map[string]any{
				"success": true,
				"errors":  []any{},
				"result": []map[string]any{
					{"id": "zone_2", "name": "example.net", "status": "pending"},
				},
				"result_info": map[string]any{"page": 2, "total_pages": 2},
			})
		case "3":
			writeJSON(response, http.StatusOK, map[string]any{
				"success":     true,
				"errors":      []any{},
				"result":      []map[string]any{},
				"result_info": map[string]any{"page": 3, "total_pages": 2},
			})
		default:
			t.Fatalf("unexpected page %q", request.URL.Query().Get("page"))
		}
	}))
	defer server.Close()

	zones, err := (CloudflareProvider{HTTPClient: server.Client(), BaseURL: server.URL + "/client/v4"}).ListZones(context.Background(), "token")
	if err != nil {
		t.Fatalf("list zones: %v", err)
	}
	if listCalls < 2 {
		t.Fatalf("expected both Cloudflare zone pages to be listed, got %d calls", listCalls)
	}
	if len(zones) != 2 || zones[0].ID != "zone_1" || zones[0].Name != "example.com" || zones[0].Status != "ACTIVE" || zones[1].ID != "zone_2" || zones[1].Name != "example.net" || zones[1].Status != "PENDING" {
		t.Fatalf("unexpected zones %#v", zones)
	}
}

func TestCloudflareProviderIgnoresSDKCloudflareEnv(t *testing.T) {
	t.Setenv("CLOUDFLARE_BASE_URL", "https://env-cloudflare.example.invalid/client/v4")
	t.Setenv("CLOUDFLARE_API_TOKEN", "env-token")
	t.Setenv("CLOUDFLARE_API_KEY", "env-key")
	t.Setenv("CLOUDFLARE_EMAIL", "env@example.com")
	t.Setenv("CLOUDFLARE_API_USER_SERVICE_KEY", "env-service-key")
	t.Setenv("CLOUDFLARE_CUSTOM_HEADERS", "Authorization: Bearer custom-env-token\nX-Auth-Key: custom-env-key\nX-Auth-Email: custom-env@example.com")

	var requests int
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.URL.Scheme != "https" || request.URL.Host != "api.cloudflare.com" {
			t.Fatalf("expected production Cloudflare API URL, got %s", request.URL.String())
		}
		if request.Header.Get("Authorization") != "Bearer provider-token" {
			t.Fatalf("expected provider token auth, got %q", request.Header.Get("Authorization"))
		}
		if request.Header.Get("X-Auth-Key") != "" || request.Header.Get("X-Auth-Email") != "" || request.Header.Get("X-Auth-User-Service-Key") != "" {
			t.Fatalf("expected SDK env auth headers to be stripped, got key=%q email=%q service=%q",
				request.Header.Get("X-Auth-Key"),
				request.Header.Get("X-Auth-Email"),
				request.Header.Get("X-Auth-User-Service-Key"),
			)
		}
		return jsonResponse(http.StatusOK, cloudflareListEnvelope(nil, 1, 1)), nil
	})}

	err := (CloudflareProvider{HTTPClient: client}).ApplyRecord(context.Background(), ApplyRecordInput{
		ProviderSecret: "provider-token",
		Zone:           "zone_1",
		RecordName:     "app.example.com",
		RecordType:     "A",
		Values:         nil,
	})
	if err != nil {
		t.Fatalf("apply record with isolated Cloudflare env: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected one list request, got %d", requests)
	}
}

func TestCloudflareProviderPagesThroughRecords(t *testing.T) {
	var listCalls int
	var deleted []string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("expected bearer auth header, got %q", request.Header.Get("Authorization"))
		}
		switch request.Method {
		case http.MethodGet:
			listCalls++
			if request.URL.Query().Get("type") != "A" || request.URL.Query().Get("name.exact") != "app.example.com" {
				t.Fatalf("unexpected list query %s", request.URL.RawQuery)
			}
			page := request.URL.Query().Get("page")
			if page == "" || page == "1" {
				writeJSON(response, http.StatusOK, cloudflareListEnvelope([]map[string]any{
					cloudflareRecordPayload("record_1", "app.example.com", "A", "192.0.2.1"),
				}, 1, 2))
				return
			}
			if page == "2" {
				writeJSON(response, http.StatusOK, cloudflareListEnvelope([]map[string]any{
					cloudflareRecordPayload("record_2", "app.example.com", "A", "192.0.2.2"),
				}, 2, 2))
				return
			}
			writeJSON(response, http.StatusOK, cloudflareListEnvelope(nil, 3, 2))
		case http.MethodDelete:
			parts := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
			deleted = append(deleted, parts[len(parts)-1])
			writeJSON(response, http.StatusOK, map[string]any{
				"success":  true,
				"errors":   []any{},
				"messages": []any{},
				"result":   map[string]string{"id": parts[len(parts)-1]},
			})
		default:
			t.Fatalf("unexpected method %s", request.Method)
		}
	}))
	defer server.Close()

	err := (CloudflareProvider{HTTPClient: server.Client(), BaseURL: server.URL + "/client/v4"}).ApplyRecord(context.Background(), ApplyRecordInput{
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

func TestCloudflareProviderLeavesMatchingRecordUnchanged(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.Method+" "+request.URL.Path)
		if request.Method != http.MethodGet {
			t.Fatalf("matching DNS record should not be modified, got %s", request.Method)
		}
		if request.URL.Query().Get("page") == "2" {
			writeJSON(response, http.StatusOK, cloudflareListEnvelope(nil, 2, 1))
			return
		}
		writeJSON(response, http.StatusOK, cloudflareListEnvelope([]map[string]any{
			cloudflareRecordPayload("record_1", "app.example.com", "A", "192.0.2.1"),
		}, 1, 1))
	}))
	defer server.Close()

	err := (CloudflareProvider{HTTPClient: server.Client(), BaseURL: server.URL + "/client/v4"}).ApplyRecord(context.Background(), ApplyRecordInput{
		ProviderSecret: "token",
		Zone:           "zone_1",
		RecordName:     "app.example.com",
		RecordType:     "A",
		Values:         []string{"192.0.2.1"},
	})
	if err != nil {
		t.Fatalf("apply unchanged record: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected only list request, got %#v", requests)
	}
}

func TestCloudflareProviderUpdatesMatchingRecordSettings(t *testing.T) {
	var updated map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			record := cloudflareRecordPayload("record_1", "app.example.com", "A", "192.0.2.1")
			record["ttl"] = 60
			record["proxied"] = false
			writeJSON(response, http.StatusOK, cloudflareListEnvelope([]map[string]any{record}, 1, 1))
		case http.MethodPut:
			updated = decodeJSONBody(t, request)
			writeJSON(response, http.StatusOK, map[string]any{
				"success":  true,
				"errors":   []any{},
				"messages": []any{},
				"result":   cloudflareRecordPayload("record_1", "app.example.com", "A", "192.0.2.1"),
			})
		default:
			t.Fatalf("unexpected method %s", request.Method)
		}
	}))
	defer server.Close()

	err := (CloudflareProvider{HTTPClient: server.Client(), BaseURL: server.URL + "/client/v4"}).ApplyRecord(context.Background(), ApplyRecordInput{
		ProviderSecret: "token",
		Zone:           "zone_1",
		RecordName:     "app.example.com",
		RecordType:     "A",
		Values:         []string{"192.0.2.1"},
		TTL:            120,
		Proxied:        true,
	})
	if err != nil {
		t.Fatalf("apply settings update: %v", err)
	}
	if updated["content"] != "192.0.2.1" || updated["ttl"] != float64(1) || updated["proxied"] != true {
		t.Fatalf("unexpected update body %#v", updated)
	}
}

func TestCloudflareProviderUpdatesAndCreatesMissingAddressRecords(t *testing.T) {
	var updated []string
	var created []string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			if request.URL.Query().Get("page") == "2" {
				writeJSON(response, http.StatusOK, cloudflareListEnvelope(nil, 2, 1))
				return
			}
			writeJSON(response, http.StatusOK, cloudflareListEnvelope([]map[string]any{
				cloudflareRecordPayload("record_keep", "app.example.com", "AAAA", "2001:db8::1"),
				cloudflareRecordPayload("record_update", "app.example.com", "AAAA", "2001:db8::2"),
			}, 1, 1))
		case http.MethodPut:
			body := decodeJSONBody(t, request)
			updated = append(updated, body["content"].(string))
			if body["type"] != "AAAA" || body["name"] != "app.example.com" || body["proxied"] != false || body["ttl"] != float64(60) {
				t.Fatalf("unexpected update body %#v", body)
			}
			writeJSON(response, http.StatusOK, map[string]any{
				"success":  true,
				"errors":   []any{},
				"messages": []any{},
				"result":   cloudflareRecordPayload("record_update", "app.example.com", "AAAA", body["content"].(string)),
			})
		case http.MethodPost:
			body := decodeJSONBody(t, request)
			created = append(created, body["content"].(string))
			if body["type"] != "AAAA" || body["name"] != "app.example.com" || body["proxied"] != false || body["ttl"] != float64(60) {
				t.Fatalf("unexpected create body %#v", body)
			}
			writeJSON(response, http.StatusOK, map[string]any{
				"success":  true,
				"errors":   []any{},
				"messages": []any{},
				"result":   cloudflareRecordPayload("record_create", "app.example.com", "AAAA", body["content"].(string)),
			})
		default:
			t.Fatalf("unexpected method %s", request.Method)
		}
	}))
	defer server.Close()

	err := (CloudflareProvider{HTTPClient: server.Client(), BaseURL: server.URL + "/client/v4"}).ApplyRecord(context.Background(), ApplyRecordInput{
		ProviderSecret: "token",
		Zone:           "zone_1",
		RecordName:     "app.example.com",
		RecordType:     "AAAA",
		Values:         []string{"2001:db8::1", "2001:db8::3", "2001:db8::4"},
	})
	if err != nil {
		t.Fatalf("apply address records: %v", err)
	}
	changed := append(updated, created...)
	if len(updated) != 1 || len(created) != 1 || !sameStringSet(changed, []string{"2001:db8::3", "2001:db8::4"}) {
		t.Fatalf("expected one update and one create for missing values, updated=%#v created=%#v", updated, created)
	}
}

func TestCloudflareProviderStopsPaginationWhenContextIsCanceled(t *testing.T) {
	var cancel context.CancelFunc
	var listCalls int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Fatalf("unexpected method after canceled list context: %s", request.Method)
		}
		listCalls++
		if listCalls > 1 {
			t.Fatalf("expected pagination to stop after context cancellation, got call %d", listCalls)
		}
		cancel()
		writeJSON(response, http.StatusOK, cloudflareListEnvelope([]map[string]any{
			cloudflareRecordPayload("record_1", "app.example.com", "A", "192.0.2.1"),
		}, 1, 2))
	}))
	defer server.Close()

	ctx, cancelContext := context.WithCancel(context.Background())
	cancel = cancelContext
	defer cancelContext()
	err := (CloudflareProvider{HTTPClient: server.Client(), BaseURL: server.URL + "/client/v4"}).ApplyRecord(ctx, ApplyRecordInput{
		ProviderSecret: "token",
		Zone:           "zone_1",
		RecordName:     "app.example.com",
		RecordType:     "A",
		Values:         nil,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
	if listCalls != 1 {
		t.Fatalf("expected one list call before cancellation, got %d", listCalls)
	}
}

func TestCloudflareProviderErrorDoesNotExposeToken(t *testing.T) {
	const token = "super-secret-token"
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		writeJSON(response, http.StatusForbidden, map[string]any{
			"success":  false,
			"errors":   []map[string]string{{"message": "invalid token"}},
			"messages": []any{},
			"result":   nil,
		})
	}))
	defer server.Close()

	err := (CloudflareProvider{HTTPClient: server.Client(), BaseURL: server.URL + "/client/v4"}).ApplyRecord(context.Background(), ApplyRecordInput{
		ProviderSecret: token,
		Zone:           "zone_1",
		RecordName:     "app.example.com",
		RecordType:     "A",
		Values:         []string{"192.0.2.1"},
	})
	if err == nil {
		t.Fatal("expected Cloudflare error")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("provider error exposed token: %v", err)
	}
	if !strings.Contains(err.Error(), "cloudflare dns list records") {
		t.Fatalf("expected operation context in error, got %v", err)
	}
}

func cloudflareListEnvelope(records []map[string]any, page int, totalPages int) map[string]any {
	return map[string]any{
		"success":  true,
		"errors":   []any{},
		"messages": []any{},
		"result":   records,
		"result_info": map[string]int{
			"page":        page,
			"total_pages": totalPages,
		},
	}
}

func cloudflareRecordPayload(id string, name string, recordType string, content string) map[string]any {
	return map[string]any{
		"id":      id,
		"name":    name,
		"type":    recordType,
		"content": content,
		"ttl":     60,
		"proxied": false,
	}
}

func writeJSON(response http.ResponseWriter, status int, payload any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(payload)
}

func jsonResponse(status int, payload any) *http.Response {
	data, _ := json.Marshal(payload)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(data))),
	}
}

func decodeJSONBody(t *testing.T, request *http.Request) map[string]any {
	t.Helper()
	data, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("decode request body %s: %v", string(data), err)
	}
	return body
}

func sameStringSet(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]int, len(left))
	for _, value := range left {
		seen[value]++
	}
	for _, value := range right {
		if seen[value] == 0 {
			return false
		}
		seen[value]--
	}
	return true
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
