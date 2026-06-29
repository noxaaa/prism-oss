package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/auth"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/service"
	"github.com/noxaaa/prism-oss/pkg/edition"
)

type enrollmentProfileHandlerFixture struct {
	server http.Handler
	token  string
	group  ossNodeGroupPayload
}

func newEnrollmentProfileHandlerFixture(t *testing.T) enrollmentProfileHandlerFixture {
	t.Helper()

	db, store := openMigratedOSSControlTestStore(t)
	t.Cleanup(func() { closeTestDB(db) })

	seedBetterAuthUser(t, db, "user_owner", "owner@example.com", "Owner")
	webSigner := auth.HMACWebUserTokenSigner{Secret: []byte("test-secret")}
	internalSigner := auth.HMACInternalTokenSigner{Secret: []byte("test-secret")}
	server := NewControlServer(ControlServerOptions{
		TokenVerifier:           internalSigner,
		WebUserVerifier:         webSigner,
		RepositoryStore:         store,
		Edition:                 edition.OSSProvider(),
		InternalTokenTTL:        time.Minute,
		AppName:                 "OSS Control Console",
		ControlPlaneURL:         "http://127.0.0.1:8080",
		AgentReleaseVersion:     "v0.0.0-test",
		AgentTokenSigningSecret: []byte("agent-token-secret-32-byte-test-key"),
	})
	bootstrap := postBootstrap(t, server, webSigner, "user_owner", "owner@example.com")
	if bootstrap.Code != http.StatusCreated {
		t.Fatalf("expected bootstrap 201, got %d body=%s", bootstrap.Code, bootstrap.Body.String())
	}
	var bootstrapResponse controlResponse
	decodeJSON(t, bootstrap, &bootstrapResponse)
	token := signInternalToken(t, internalSigner, auth.InternalClaims{
		UserID:         "user_owner",
		OrganizationID: bootstrapResponse.Data.Organization.ID,
		MemberID:       "synthetic-member",
		SourceService:  auth.InternalSourceServiceWeb,
		Roles:          []string{"synthetic-owner"},
		Permissions:    []string{string(domain.PermissionNodesRead), string(domain.PermissionNodesManage)},
		ResourceScopes: []auth.ResourceScopeClaim{{ResourceType: string(domain.ResourceTypeNodeGroup), ResourceID: "*", AccessLevel: string(domain.AccessLevelManage)}},
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	group := createOSSNodeGroupViaAPI(t, server, token, "Enrollment Profile Group")
	return enrollmentProfileHandlerFixture{server: server, token: token, group: group}
}

func TestNodeEnrollmentProfileCreateCanNeverExpire(t *testing.T) {
	fixture := newEnrollmentProfileHandlerFixture(t)
	body := `{
		"name":"OSS Never Expire Enrollment",
		"enabled":true,
		"max_uses":0,
		"node_name_template":"{{hostname}}",
		"group_ids":["` + fixture.group.ID + `"],
		"listen_ips":[{"listen_ip":"0.0.0.0","display_name":"default"}],
		"send_ips":[{"send_ip":"192.0.2.10","display_name":"egress"}],
		"port_ranges":[{"protocol":"TCP","start_port":10000,"end_port":20000}],
		"dataplane_mode":"AUTO",
		"dataplane_conflict_policy":"FAIL_FAST",
		"auto_update_enabled":true,
		"allowed_cidrs":[]
	}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/node-enrollment-profiles", bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+fixture.token)
	request.Header.Set("Content-Type", "application/json")
	fixture.server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected never-expiring enrollment profile create 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data ossNodeEnrollmentProfilePayload `json:"data"`
	}
	decodeJSON(t, recorder, &response)
	if response.Data.ExpiresAt != "" {
		t.Fatalf("expected omitted expiry fields to create a never-expiring profile, got %q", response.Data.ExpiresAt)
	}
	if response.Data.Token == "" || response.Data.ShellScript == "" {
		t.Fatalf("expected one-time token and script for never-expiring profile: %#v", response.Data)
	}
}

func TestNodeEnrollmentProfileCreateReturnsFieldDetailsForInvalidTTL(t *testing.T) {
	for _, ttlHours := range []int{99999, 0, -1} {
		t.Run(strconv.Itoa(ttlHours), func(t *testing.T) {
			fixture := newEnrollmentProfileHandlerFixture(t)
			body := `{
				"name":"OSS Invalid TTL Enrollment",
				"enabled":true,
				"ttl_hours":` + strconv.Itoa(ttlHours) + `,
				"node_name_template":"{{hostname}}",
				"group_ids":["` + fixture.group.ID + `"],
				"listen_ips":[{"listen_ip":"0.0.0.0","display_name":"default"}],
				"port_ranges":[{"protocol":"TCP","start_port":10000,"end_port":20000}],
				"dataplane_mode":"AUTO",
				"dataplane_conflict_policy":"FAIL_FAST",
				"auto_update_enabled":true,
				"allowed_cidrs":[]
			}`
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/internal/v1/node-enrollment-profiles", bytes.NewBufferString(body))
			request.Header.Set("Authorization", "Bearer "+fixture.token)
			request.Header.Set("Content-Type", "application/json")
			fixture.server.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected invalid enrollment TTL to fail 400, got %d body=%s", recorder.Code, recorder.Body.String())
			}
			var response struct {
				Error struct {
					Code    string         `json:"code"`
					Details map[string]any `json:"details"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v body=%s", err, recorder.Body.String())
			}
			if response.Error.Code != "VALIDATION_FAILED" || response.Error.Details["field"] != "ttl_hours" {
				t.Fatalf("expected ttl_hours validation details, got %#v body=%s", response.Error, recorder.Body.String())
			}
		})
	}
}

func TestNodeEnrollmentProfilePatchPreservesOmittedDefaults(t *testing.T) {
	db, store := openMigratedOSSControlTestStore(t)
	defer closeTestDB(db)

	seedBetterAuthUser(t, db, "user_owner", "owner@example.com", "Owner")
	webSigner := auth.HMACWebUserTokenSigner{Secret: []byte("test-secret")}
	internalSigner := auth.HMACInternalTokenSigner{Secret: []byte("test-secret")}
	server := NewControlServer(ControlServerOptions{
		TokenVerifier:           internalSigner,
		WebUserVerifier:         webSigner,
		RepositoryStore:         store,
		Edition:                 edition.OSSProvider(),
		InternalTokenTTL:        time.Minute,
		AppName:                 "OSS Control Console",
		ControlPlaneURL:         "http://127.0.0.1:8080",
		AgentReleaseVersion:     "v0.0.0-test",
		AgentTokenSigningSecret: []byte("agent-token-secret-32-byte-test-key"),
	})
	bootstrap := postBootstrap(t, server, webSigner, "user_owner", "owner@example.com")
	if bootstrap.Code != http.StatusCreated {
		t.Fatalf("expected bootstrap 201, got %d body=%s", bootstrap.Code, bootstrap.Body.String())
	}
	var bootstrapResponse controlResponse
	decodeJSON(t, bootstrap, &bootstrapResponse)
	token := signInternalToken(t, internalSigner, auth.InternalClaims{
		UserID:         "user_owner",
		OrganizationID: bootstrapResponse.Data.Organization.ID,
		MemberID:       "synthetic-member",
		SourceService:  auth.InternalSourceServiceWeb,
		Roles:          []string{"synthetic-owner"},
		Permissions:    []string{string(domain.PermissionNodesRead), string(domain.PermissionNodesManage)},
		ResourceScopes: []auth.ResourceScopeClaim{{ResourceType: string(domain.ResourceTypeNodeGroup), ResourceID: "*", AccessLevel: string(domain.AccessLevelManage)}},
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	group := createOSSNodeGroupViaAPI(t, server, token, "Enrollment Patch Group")
	profile := createOSSNodeEnrollmentProfileWithEnabledViaAPI(t, server, token, group.ID, false)
	if profile.Enabled {
		t.Fatalf("expected test profile to start disabled")
	}
	if profile.ExpiresAt == "" {
		t.Fatalf("expected test profile to have a default expiry")
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/internal/v1/node-enrollment-profiles/"+profile.ID, bytes.NewBufferString(`{"description":"updated only"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enrollment profile patch 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data ossNodeEnrollmentProfilePayload `json:"data"`
	}
	decodeJSON(t, recorder, &response)
	if response.Data.Description != "updated only" {
		t.Fatalf("expected patch to update description, got %#v", response.Data)
	}
	if response.Data.Enabled {
		t.Fatalf("expected patch omitting enabled to preserve disabled state")
	}
	expectedExpiresAt := postgresTimestampString(t, profile.ExpiresAt)
	if response.Data.ExpiresAt != expectedExpiresAt {
		t.Fatalf("expected patch omitting expiry fields to preserve expires_at %q, got %q", expectedExpiresAt, response.Data.ExpiresAt)
	}
}

func TestNodeEnrollmentProfileRejectsOverlongFixedNameTemplate(t *testing.T) {
	db, store := openMigratedOSSControlTestStore(t)
	defer closeTestDB(db)

	seedBetterAuthUser(t, db, "user_owner", "owner@example.com", "Owner")
	webSigner := auth.HMACWebUserTokenSigner{Secret: []byte("test-secret")}
	internalSigner := auth.HMACInternalTokenSigner{Secret: []byte("test-secret")}
	server := NewControlServer(ControlServerOptions{
		TokenVerifier:           internalSigner,
		WebUserVerifier:         webSigner,
		RepositoryStore:         store,
		Edition:                 edition.OSSProvider(),
		InternalTokenTTL:        time.Minute,
		AppName:                 "OSS Control Console",
		ControlPlaneURL:         "http://127.0.0.1:8080",
		AgentReleaseVersion:     "v0.0.0-test",
		AgentTokenSigningSecret: []byte("agent-token-secret-32-byte-test-key"),
	})
	bootstrap := postBootstrap(t, server, webSigner, "user_owner", "owner@example.com")
	if bootstrap.Code != http.StatusCreated {
		t.Fatalf("expected bootstrap 201, got %d body=%s", bootstrap.Code, bootstrap.Body.String())
	}
	var bootstrapResponse controlResponse
	decodeJSON(t, bootstrap, &bootstrapResponse)
	token := signInternalToken(t, internalSigner, auth.InternalClaims{
		UserID:         "user_owner",
		OrganizationID: bootstrapResponse.Data.Organization.ID,
		MemberID:       "synthetic-member",
		SourceService:  auth.InternalSourceServiceWeb,
		Roles:          []string{"synthetic-owner"},
		Permissions:    []string{string(domain.PermissionNodesRead), string(domain.PermissionNodesManage)},
		ResourceScopes: []auth.ResourceScopeClaim{{ResourceType: string(domain.ResourceTypeNodeGroup), ResourceID: "*", AccessLevel: string(domain.AccessLevelManage)}},
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	group := createOSSNodeGroupViaAPI(t, server, token, "Enrollment Template Group")
	body := `{
		"name":"OSS API Enrollment",
		"enabled":true,
		"ttl_hours":720,
		"node_name_template":"` + strings.Repeat("x", 121) + `{{hostname}}",
		"group_ids":["` + group.ID + `"],
		"listen_ips":[{"listen_ip":"0.0.0.0","display_name":"default"}],
		"port_ranges":[{"protocol":"TCP","start_port":10000,"end_port":20000}],
		"dataplane_mode":"AUTO",
		"dataplane_conflict_policy":"FAIL_FAST",
		"auto_update_enabled":true,
		"allowed_cidrs":[]
	}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/node-enrollment-profiles", bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected overlong fixed enrollment template to fail 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestNodeEnrollmentProfileRejectsExplicitExpiryBeyondCap(t *testing.T) {
	db, store := openMigratedOSSControlTestStore(t)
	defer closeTestDB(db)

	seedBetterAuthUser(t, db, "user_owner", "owner@example.com", "Owner")
	webSigner := auth.HMACWebUserTokenSigner{Secret: []byte("test-secret")}
	internalSigner := auth.HMACInternalTokenSigner{Secret: []byte("test-secret")}
	server := NewControlServer(ControlServerOptions{
		TokenVerifier:           internalSigner,
		WebUserVerifier:         webSigner,
		RepositoryStore:         store,
		Edition:                 edition.OSSProvider(),
		InternalTokenTTL:        time.Minute,
		AppName:                 "OSS Control Console",
		ControlPlaneURL:         "http://127.0.0.1:8080",
		AgentReleaseVersion:     "v0.0.0-test",
		AgentTokenSigningSecret: []byte("agent-token-secret-32-byte-test-key"),
	})
	bootstrap := postBootstrap(t, server, webSigner, "user_owner", "owner@example.com")
	if bootstrap.Code != http.StatusCreated {
		t.Fatalf("expected bootstrap 201, got %d body=%s", bootstrap.Code, bootstrap.Body.String())
	}
	var bootstrapResponse controlResponse
	decodeJSON(t, bootstrap, &bootstrapResponse)
	token := signInternalToken(t, internalSigner, auth.InternalClaims{
		UserID:         "user_owner",
		OrganizationID: bootstrapResponse.Data.Organization.ID,
		MemberID:       "synthetic-member",
		SourceService:  auth.InternalSourceServiceWeb,
		Roles:          []string{"synthetic-owner"},
		Permissions:    []string{string(domain.PermissionNodesRead), string(domain.PermissionNodesManage)},
		ResourceScopes: []auth.ResourceScopeClaim{{ResourceType: string(domain.ResourceTypeNodeGroup), ResourceID: "*", AccessLevel: string(domain.AccessLevelManage)}},
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	group := createOSSNodeGroupViaAPI(t, server, token, "Enrollment Expiry Cap Group")
	expiresAt := time.Now().UTC().Add(time.Hour * 24 * 367).Format(time.RFC3339Nano)
	body := `{
		"name":"OSS Expiry Cap Enrollment",
		"enabled":true,
		"expires_at":"` + expiresAt + `",
		"node_name_template":"{{hostname}}",
		"group_ids":["` + group.ID + `"],
		"listen_ips":[{"listen_ip":"0.0.0.0","display_name":"default"}],
		"port_ranges":[{"protocol":"TCP","start_port":10000,"end_port":20000}],
		"dataplane_mode":"AUTO",
		"dataplane_conflict_policy":"FAIL_FAST",
		"auto_update_enabled":true,
		"allowed_cidrs":[]
	}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/node-enrollment-profiles", bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected explicit expires_at beyond cap to fail 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestNodeEnrollmentProfileRejectsPastExplicitExpiry(t *testing.T) {
	db, store := openMigratedOSSControlTestStore(t)
	defer closeTestDB(db)

	seedBetterAuthUser(t, db, "user_owner", "owner@example.com", "Owner")
	webSigner := auth.HMACWebUserTokenSigner{Secret: []byte("test-secret")}
	internalSigner := auth.HMACInternalTokenSigner{Secret: []byte("test-secret")}
	server := NewControlServer(ControlServerOptions{
		TokenVerifier:           internalSigner,
		WebUserVerifier:         webSigner,
		RepositoryStore:         store,
		Edition:                 edition.OSSProvider(),
		InternalTokenTTL:        time.Minute,
		AppName:                 "OSS Control Console",
		ControlPlaneURL:         "http://127.0.0.1:8080",
		AgentReleaseVersion:     "v0.0.0-test",
		AgentTokenSigningSecret: []byte("agent-token-secret-32-byte-test-key"),
	})
	bootstrap := postBootstrap(t, server, webSigner, "user_owner", "owner@example.com")
	if bootstrap.Code != http.StatusCreated {
		t.Fatalf("expected bootstrap 201, got %d body=%s", bootstrap.Code, bootstrap.Body.String())
	}
	var bootstrapResponse controlResponse
	decodeJSON(t, bootstrap, &bootstrapResponse)
	token := signInternalToken(t, internalSigner, auth.InternalClaims{
		UserID:         "user_owner",
		OrganizationID: bootstrapResponse.Data.Organization.ID,
		MemberID:       "synthetic-member",
		SourceService:  auth.InternalSourceServiceWeb,
		Roles:          []string{"synthetic-owner"},
		Permissions:    []string{string(domain.PermissionNodesRead), string(domain.PermissionNodesManage)},
		ResourceScopes: []auth.ResourceScopeClaim{{ResourceType: string(domain.ResourceTypeNodeGroup), ResourceID: "*", AccessLevel: string(domain.AccessLevelManage)}},
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	group := createOSSNodeGroupViaAPI(t, server, token, "Enrollment Past Expiry Group")
	expiresAt := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	body := `{
		"name":"OSS Past Expiry Enrollment",
		"enabled":true,
		"expires_at":"` + expiresAt + `",
		"node_name_template":"{{hostname}}",
		"group_ids":["` + group.ID + `"],
		"listen_ips":[{"listen_ip":"0.0.0.0","display_name":"default"}],
		"port_ranges":[{"protocol":"TCP","start_port":10000,"end_port":20000}],
		"dataplane_mode":"AUTO",
		"dataplane_conflict_policy":"FAIL_FAST",
		"auto_update_enabled":true,
		"allowed_cidrs":[]
	}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/node-enrollment-profiles", bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected past explicit expires_at to fail 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPendingEnrollmentReleasePreservesEnabledRuleCoverage(t *testing.T) {
	db, store := openMigratedOSSControlTestStore(t)
	defer closeTestDB(db)

	seedBetterAuthUser(t, db, "user_owner", "owner@example.com", "Owner")
	webSigner := auth.HMACWebUserTokenSigner{Secret: []byte("test-secret")}
	internalSigner := auth.HMACInternalTokenSigner{Secret: []byte("test-secret")}
	controlService := service.NewControlServiceWithOptions(store, service.ControlServiceOptions{
		Edition:                 edition.OSSProvider(),
		AppName:                 "OSS Control Console",
		ControlPlaneURL:         "http://127.0.0.1:8080",
		AgentReleaseVersion:     "v0.0.0-test",
		AgentTokenSigningSecret: []byte("agent-token-secret-32-byte-test-key"),
	})
	server := NewControlServer(ControlServerOptions{
		TokenVerifier:           internalSigner,
		WebUserVerifier:         webSigner,
		RepositoryStore:         store,
		Edition:                 edition.OSSProvider(),
		InternalTokenTTL:        time.Minute,
		AppName:                 "OSS Control Console",
		ControlPlaneURL:         "http://127.0.0.1:8080",
		AgentReleaseVersion:     "v0.0.0-test",
		AgentTokenSigningSecret: []byte("agent-token-secret-32-byte-test-key"),
	})
	bootstrap := postBootstrap(t, server, webSigner, "user_owner", "owner@example.com")
	if bootstrap.Code != http.StatusCreated {
		t.Fatalf("expected bootstrap 201, got %d body=%s", bootstrap.Code, bootstrap.Body.String())
	}
	var bootstrapResponse controlResponse
	decodeJSON(t, bootstrap, &bootstrapResponse)
	token := signInternalToken(t, internalSigner, auth.InternalClaims{
		UserID:         "user_owner",
		OrganizationID: bootstrapResponse.Data.Organization.ID,
		MemberID:       "synthetic-member",
		SourceService:  auth.InternalSourceServiceWeb,
		Roles:          []string{"synthetic-owner"},
		Permissions: []string{
			string(domain.PermissionNodesRead),
			string(domain.PermissionNodesManage),
			string(domain.PermissionTargetsManage),
			string(domain.PermissionRulesManageAll),
		},
		ResourceScopes: []auth.ResourceScopeClaim{{ResourceType: string(domain.ResourceTypeNodeGroup), ResourceID: "*", AccessLevel: string(domain.AccessLevelManage)}},
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	group := createOSSNodeGroupViaAPI(t, server, token, "Enrollment Rule Coverage Group")
	profile := createOSSNodeEnrollmentProfileWithMaxUsesViaAPI(t, server, token, group.ID, 1)
	authResult, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", profile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-rule-owner",
		RemoteIP: "203.0.113.45",
	})
	if err != nil {
		t.Fatalf("expected enrollment token to create a pending node, got %v", err)
	}
	target := createOSSTargetViaAPI(t, server, token, "coverage target")
	targetGroup := createOSSTargetGroupViaAPI(t, server, token, target.ID, "coverage target group")
	if _, err := controlService.CreateRule(context.Background(), service.InternalIdentity{
		UserID:         "user_owner",
		OrganizationID: bootstrapResponse.Data.Organization.ID,
		MemberID:       "synthetic-member",
		Roles:          []string{"synthetic-owner"},
		Permissions:    []string{string(domain.PermissionRulesManageAll)},
		ResourceScopes: []service.ResourceScopePayload{{ResourceType: string(domain.ResourceTypeNodeGroup), ResourceID: "*", AccessLevel: string(domain.AccessLevelManage)}},
	}, service.RuleMutationInput{
		Name:           "Pending enrollment coverage rule",
		NodeGroupID:    group.ID,
		ListenIP:       "0.0.0.0",
		ForwardingType: "DIRECT",
		Protocol:       "TCP",
		Port:           10000,
		Match:          service.RuleMatchInput{Type: "ANY_INBOUND"},
		ProxyProtocol:  service.RuleProxyProtocolInput{In: "NONE", Out: "NONE"},
		Upstream:       service.RuleUpstreamInput{Type: "TARGET_GROUP", TargetGroupID: targetGroup.ID},
		Enabled:        true,
		EnabledSet:     true,
	}); err != nil {
		t.Fatalf("expected enabled rule create to succeed before release: %v", err)
	}
	err = controlService.ReleaseAgentRegistrationCredential(context.Background(), authResult)
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("expected release to preserve enabled rule coverage with ErrConflict, got %v", err)
	}
	if _, err := store.Nodes().FindNodeByID(context.Background(), bootstrapResponse.Data.Organization.ID, authResult.AgentID); err != nil {
		t.Fatalf("expected pending enrollment node to remain after rejected release: %v", err)
	}
}

func postgresTimestampString(t *testing.T, value string) string {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatalf("parse timestamp %q: %v", value, err)
	}
	return parsed.UTC().Round(time.Microsecond).Format("2006-01-02T15:04:05.000000Z")
}
