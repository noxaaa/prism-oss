package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type ossNodeGroupPayload struct {
	ID string `json:"id"`
}

type ossNodeEnrollmentProfilePayload struct {
	ID                string `json:"id"`
	Description       string `json:"description"`
	Enabled           bool   `json:"enabled"`
	ExpiresAt         string `json:"expires_at"`
	Token             string `json:"token"`
	InstallCommand    string `json:"install_command"`
	ShellScript       string `json:"shell_script"`
	AWSCloudInit      string `json:"aws_cloud_init"`
	TerraformUserData string `json:"terraform_user_data"`
}

type ossNodeEnrollmentEventPayload struct {
	ReasonCode string `json:"reason_code"`
	RemoteIP   string `json:"remote_ip"`
	Hostname   string `json:"hostname"`
	Status     string `json:"status"`
}

func createOSSNodeGroupViaAPI(t *testing.T, server http.Handler, token string, name string) ossNodeGroupPayload {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/node-groups", bytes.NewBufferString(`{"name":"`+name+`","description":"OSS core group"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected OSS node group create 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data ossNodeGroupPayload `json:"data"`
	}
	decodeJSON(t, recorder, &response)
	return response.Data
}

func createOSSNodeEnrollmentProfileViaAPI(t *testing.T, server http.Handler, token string, groupID string) ossNodeEnrollmentProfilePayload {
	t.Helper()
	return createOSSNodeEnrollmentProfileWithOptionsViaAPI(t, server, token, groupID, 0, true)
}

func createOSSNodeEnrollmentProfileWithMaxUsesViaAPI(t *testing.T, server http.Handler, token string, groupID string, maxUses int) ossNodeEnrollmentProfilePayload {
	t.Helper()
	return createOSSNodeEnrollmentProfileWithOptionsViaAPI(t, server, token, groupID, maxUses, true)
}

func createOSSNodeEnrollmentProfileWithEnabledViaAPI(t *testing.T, server http.Handler, token string, groupID string, enabled bool) ossNodeEnrollmentProfilePayload {
	t.Helper()
	return createOSSNodeEnrollmentProfileWithOptionsViaAPI(t, server, token, groupID, 0, enabled)
}

func createOSSNodeEnrollmentProfileWithAllowedCIDRsViaAPI(t *testing.T, server http.Handler, token string, groupID string, allowedCIDRs []string) ossNodeEnrollmentProfilePayload {
	t.Helper()
	return createOSSNodeEnrollmentProfileWithOptionsAndAllowedCIDRsViaAPI(t, server, token, groupID, 0, true, allowedCIDRs)
}

func createOSSNodeEnrollmentProfileWithOptionsViaAPI(t *testing.T, server http.Handler, token string, groupID string, maxUses int, enabled bool) ossNodeEnrollmentProfilePayload {
	t.Helper()
	return createOSSNodeEnrollmentProfileWithOptionsAndAllowedCIDRsViaAPI(t, server, token, groupID, maxUses, enabled, nil)
}

func createOSSNodeEnrollmentProfileWithOptionsAndAllowedCIDRsViaAPI(t *testing.T, server http.Handler, token string, groupID string, maxUses int, enabled bool, allowedCIDRs []string) ossNodeEnrollmentProfilePayload {
	t.Helper()

	if allowedCIDRs == nil {
		allowedCIDRs = []string{}
	}
	body, err := json.Marshal(map[string]any{
		"name":                      "OSS API Enrollment",
		"description":               "autoscale",
		"enabled":                   enabled,
		"ttl_hours":                 720,
		"max_uses":                  maxUses,
		"node_name_template":        "{{hostname}}",
		"group_ids":                 []string{groupID},
		"listen_ips":                []map[string]string{{"listen_ip": "0.0.0.0", "display_name": "default"}},
		"port_ranges":               []map[string]any{{"protocol": "TCP", "start_port": 10000, "end_port": 20000}},
		"dataplane_mode":            "AUTO",
		"dataplane_conflict_policy": "FAIL_FAST",
		"auto_update_enabled":       true,
		"allowed_cidrs":             allowedCIDRs,
	})
	if err != nil {
		t.Fatalf("encode enrollment profile create body: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/node-enrollment-profiles", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected OSS node enrollment profile create 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data ossNodeEnrollmentProfilePayload `json:"data"`
	}
	decodeJSON(t, recorder, &response)
	if response.Data.Token == "" || response.Data.ShellScript == "" {
		t.Fatalf("expected enrollment create to return one-time token and shell script: %#v", response.Data)
	}
	if !strings.Contains(response.Data.TerraformUserData, "$${tmp:-}") {
		t.Fatalf("expected Terraform user data to escape shell interpolation, got %s", response.Data.TerraformUserData)
	}
	if !strings.Contains(response.Data.AWSCloudInit, "runcmd:\n  - |\n    ") {
		t.Fatalf("expected AWS cloud-init to use a YAML block scalar, got %s", response.Data.AWSCloudInit)
	}
	return response.Data
}

func createOSSNodeRegistrationTokenViaAPI(t *testing.T, server http.Handler, token string, nodeID string) ossRegistrationTokenPayload {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/nodes/"+nodeID+"/registration-token", bytes.NewBufferString(`{"ttl_hours":1}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected OSS node registration token create 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data ossRegistrationTokenPayload `json:"data"`
	}
	decodeJSON(t, recorder, &response)
	return response.Data
}

func revokeNodeRegistrationTokenViaAPI(t *testing.T, server http.Handler, token string, nodeID string, tokenID string) {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/internal/v1/nodes/"+nodeID+"/registration-tokens/"+tokenID, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected OSS node registration token revoke 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func deleteOSSNodeViaAPI(t *testing.T, server http.Handler, token string, nodeID string) {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/internal/v1/nodes/"+nodeID, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected OSS node delete 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func deleteOSSNodeGroupViaAPI(t *testing.T, server http.Handler, token string, groupID string) {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/internal/v1/node-groups/"+groupID, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected OSS node group delete 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func assertOSSNodeGroupDeleteConflictViaAPI(t *testing.T, server http.Handler, token string, groupID string) {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/internal/v1/node-groups/"+groupID, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected OSS node group delete conflict 409, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func rotateOSSNodeEnrollmentProfileTokenViaAPI(t *testing.T, server http.Handler, token string, profileID string) ossNodeEnrollmentProfilePayload {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/node-enrollment-profiles/"+profileID+"/rotate-token", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected OSS node enrollment token rotate 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data ossNodeEnrollmentProfilePayload `json:"data"`
	}
	decodeJSON(t, recorder, &response)
	if response.Data.Token == "" || response.Data.ShellScript == "" {
		t.Fatalf("expected enrollment rotate to return one-time token and shell script: %#v", response.Data)
	}
	if !strings.Contains(response.Data.TerraformUserData, "$${tmp:-}") {
		t.Fatalf("expected enrollment rotate Terraform user data to escape shell interpolation, got %s", response.Data.TerraformUserData)
	}
	return response.Data
}

func assertOSSNodeEnrollmentEventsIncludeFailureViaAPI(t *testing.T, server http.Handler, token string, profileID string, reasonCode string) {
	t.Helper()

	events := listOSSNodeEnrollmentEventsViaAPI(t, server, token, profileID)
	for _, event := range events {
		if event.Status == "FAILED" && event.ReasonCode == reasonCode {
			return
		}
	}
	t.Fatalf("expected enrollment events to include failed reason %q, got %#v", reasonCode, events)
}

func assertOSSNodeEnrollmentEventsIncludeFailureWithMetadataViaAPI(t *testing.T, server http.Handler, token string, profileID string, reasonCode string, remoteIP string, hostname string) {
	t.Helper()

	events := listOSSNodeEnrollmentEventsViaAPI(t, server, token, profileID)
	for _, event := range events {
		if event.Status == "FAILED" && event.ReasonCode == reasonCode && event.RemoteIP == remoteIP && event.Hostname == hostname {
			return
		}
	}
	t.Fatalf("expected enrollment events to include failed reason %q with remote IP %q and hostname %q, got %#v", reasonCode, remoteIP, hostname, events)
}

func assertOSSNodeEnrollmentEventsIncludeSuccessViaAPI(t *testing.T, server http.Handler, token string, profileID string) {
	t.Helper()

	events := listOSSNodeEnrollmentEventsViaAPI(t, server, token, profileID)
	for _, event := range events {
		if event.Status == "SUCCEEDED" {
			return
		}
	}
	t.Fatalf("expected enrollment events to include success, got %#v", events)
}

func assertOSSNodeEnrollmentEventsDoNotIncludeSuccessViaAPI(t *testing.T, server http.Handler, token string, profileID string) {
	t.Helper()

	events := listOSSNodeEnrollmentEventsViaAPI(t, server, token, profileID)
	for _, event := range events {
		if event.Status == "SUCCEEDED" {
			t.Fatalf("expected enrollment events not to include success, got %#v", events)
		}
	}
}

func listOSSNodeEnrollmentEventsViaAPI(t *testing.T, server http.Handler, token string, profileID string) []ossNodeEnrollmentEventPayload {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/internal/v1/node-enrollment-profiles/"+profileID+"/events", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected OSS node enrollment events 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data []ossNodeEnrollmentEventPayload `json:"data"`
	}
	decodeJSON(t, recorder, &response)
	return response.Data
}

func deleteOSSNodeEnrollmentProfileViaAPI(t *testing.T, server http.Handler, token string, profileID string) {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/internal/v1/node-enrollment-profiles/"+profileID, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected OSS node enrollment profile delete 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func createOSSMonitorRegistrationTokenViaAPI(t *testing.T, server http.Handler, token string, monitorID string) ossRegistrationTokenPayload {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/monitors/"+monitorID+"/registration-token", bytes.NewBufferString(`{"ttl_hours":1}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected OSS monitor registration token create 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data ossRegistrationTokenPayload `json:"data"`
	}
	decodeJSON(t, recorder, &response)
	return response.Data
}

func revokeMonitorRegistrationTokenViaAPI(t *testing.T, server http.Handler, token string, monitorID string, tokenID string) {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/internal/v1/monitors/"+monitorID+"/registration-tokens/"+tokenID, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected OSS monitor registration token revoke 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func deleteOSSDNSCredentialViaAPI(t *testing.T, server http.Handler, token string, credentialID string) {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/internal/v1/dns/credentials/"+credentialID, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected OSS DNS credential delete 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func assertOSSErrorCode(t *testing.T, recorder *httptest.ResponseRecorder, expected string) {
	t.Helper()

	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v body=%s", err, recorder.Body.String())
	}
	if response.Error.Code != expected {
		t.Fatalf("expected error code %s, got %s body=%s", expected, response.Error.Code, recorder.Body.String())
	}
}
