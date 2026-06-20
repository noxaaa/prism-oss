package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/noxaaa/prism-oss/pkg/core/auth"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
	"github.com/noxaaa/prism-oss/pkg/core/service"
	"github.com/noxaaa/prism-oss/pkg/edition"
)

func TestOSSControlServerDoesNotRegisterRBACOrMonitorRoutes(t *testing.T) {
	signer := auth.HMACInternalTokenSigner{Secret: []byte("test-secret")}
	server := NewControlServer(ControlServerOptions{
		TokenVerifier: signer,
		Edition:       edition.OSSProvider(),
	})
	token := signInternalToken(t, signer, auth.InternalClaims{
		UserID:         "user_owner",
		OrganizationID: "org_oss",
		MemberID:       "member_oss",
		Roles:          []string{"owner"},
		Permissions:    []string{"roles.read", "members.read"},
		ExpiresAt:      time.Now().Add(time.Minute),
	})

	for _, path := range []string{
		"/internal/v1/organizations/current/members",
		"/internal/v1/organizations/current/roles",
		"/internal/v1/monitor-groups",
		"/internal/v1/monitors",
		"/internal/v1/monitors/monitor_1/registration-token",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("expected OSS route %s to be unregistered, got %d body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestOSSControlServerRejectsMonitorAgentConnect(t *testing.T) {
	server := NewControlServer(ControlServerOptions{
		ControlService: service.NewControlServiceWithOptions(nil, service.ControlServiceOptions{
			AgentTokenSigningSecret: []byte("agent-secret"),
			Edition:                 edition.OSSProvider(),
		}),
		Edition: edition.OSSProvider(),
	})

	request := httptest.NewRequest(http.MethodGet, "/agent/v1/connect", nil)
	request.Header.Set("Authorization", "Bearer monitor-token")
	request.Header.Set("X-Agent-Type", "MONITOR")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected OSS monitor agent connect to be rejected, got %d body=%s", response.Code, response.Body.String())
	}
}

func TestControlServerRouteExtensionReceivesInternalIdentity(t *testing.T) {
	signer := auth.HMACInternalTokenSigner{Secret: []byte("test-secret")}
	server := NewControlServer(ControlServerOptions{
		TokenVerifier: signer,
		Edition:       edition.OSSProvider(),
		RouteExtensions: []ControlRouteExtension{
			testControlRouteExtension{register: func(registry ControlRouteRegistry) {
				registry.HandleInternal("GET /internal/v1/extension-test", func(response http.ResponseWriter, request *http.Request, identity service.InternalIdentity) {
					WriteServiceResponse(response, http.StatusOK, map[string]any{
						"user_id":      identity.UserID,
						"has_service":  registry.ControlService() != nil,
						"edition_key":  string(registry.Edition().Key()),
						"resource_len": len(identity.ResourceScopes),
					}, nil)
				})
			}},
		},
	})
	token := signInternalToken(t, signer, auth.InternalClaims{
		UserID:         "user_extension",
		OrganizationID: "org_extension",
		MemberID:       "member_extension",
		Roles:          []string{"owner"},
		Permissions:    []string{string(domain.PermissionOrganizationRead)},
		ResourceScopes: []auth.ResourceScopeClaim{{ResourceType: "NODE_GROUP", ResourceID: "*", AccessLevel: "MANAGE"}},
		ExpiresAt:      time.Now().Add(time.Minute),
	})

	request := httptest.NewRequest(http.MethodGet, "/internal/v1/extension-test", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected extension route 200, got %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data struct {
			UserID      string `json:"user_id"`
			HasService  bool   `json:"has_service"`
			EditionKey  string `json:"edition_key"`
			ResourceLen int    `json:"resource_len"`
		} `json:"data"`
	}
	decodeJSON(t, response, &payload)
	if payload.Data.UserID != "user_extension" || payload.Data.EditionKey != string(edition.KeyOSS) || payload.Data.ResourceLen != 1 {
		t.Fatalf("unexpected extension payload: %#v", payload.Data)
	}
}

func TestOSSControlServerBootstrapsWithCoreOnlySchema(t *testing.T) {
	db, store := openMigratedOSSControlTestStore(t)
	defer closeTestDB(db)

	seedBetterAuthUser(t, db, "user_owner", "owner@example.com", "Owner")
	seedBetterAuthUser(t, db, "user_other", "other@example.com", "Other")
	webSigner := auth.HMACWebUserTokenSigner{Secret: []byte("test-secret")}
	internalSigner := auth.HMACInternalTokenSigner{Secret: []byte("test-secret")}
	server := NewControlServer(ControlServerOptions{
		TokenVerifier:    internalSigner,
		WebUserVerifier:  webSigner,
		RepositoryStore:  store,
		Edition:          edition.OSSProvider(),
		InternalTokenTTL: time.Minute,
	})

	bootstrap := postBootstrap(t, server, webSigner, "user_owner", "owner@example.com")
	if bootstrap.Code != http.StatusCreated {
		t.Fatalf("expected OSS bootstrap 201 with core schema, got %d body=%s", bootstrap.Code, bootstrap.Body.String())
	}
	var bootstrapResponse controlResponse
	decodeJSON(t, bootstrap, &bootstrapResponse)
	if bootstrapResponse.Data.Member.ID == "" {
		t.Fatalf("expected synthetic OSS member in bootstrap response: %#v", bootstrapResponse.Data.Member)
	}
	assertPermission(t, bootstrapResponse.Data.Permissions, string(domain.PermissionOrganizationUpdate))
	assertPermission(t, bootstrapResponse.Data.Permissions, string(domain.PermissionNodesManage))
	assertMissingPermission(t, bootstrapResponse.Data.Permissions, "members.manage")
	assertMissingPermission(t, bootstrapResponse.Data.Permissions, "roles.manage")
	assertMissingPermission(t, bootstrapResponse.Data.Permissions, "monitors.read")
	assertMissingPermission(t, bootstrapResponse.Data.Permissions, "monitors.manage")
	assertMissingPermission(t, bootstrapResponse.Data.Permissions, "dns.manage")
	assertMissingPermission(t, bootstrapResponse.Data.Permissions, "health_checks.manage")
	assertScope(t, bootstrapResponse.Data.ResourceScopes, "NODE_GROUP", "*", "MANAGE")

	sessionToken := signWebUserToken(t, webSigner, auth.WebUserTokenPurposeSession, "user_owner", "owner@example.com", "Owner")
	sessionRecorder := httptest.NewRecorder()
	sessionRequest := httptest.NewRequest(http.MethodGet, "/internal/v1/session", nil)
	sessionRequest.Header.Set("Authorization", "Bearer "+sessionToken)
	server.ServeHTTP(sessionRecorder, sessionRequest)
	if sessionRecorder.Code != http.StatusOK {
		t.Fatalf("expected OSS session 200 with core schema, got %d body=%s", sessionRecorder.Code, sessionRecorder.Body.String())
	}
	var sessionResponse controlResponse
	decodeJSON(t, sessionRecorder, &sessionResponse)
	if sessionResponse.Data.Organization.ID != bootstrapResponse.Data.Organization.ID {
		t.Fatalf("expected OSS session organization %q, got %#v", bootstrapResponse.Data.Organization.ID, sessionResponse.Data.Organization)
	}

	ownerInternalToken := signInternalToken(t, internalSigner, auth.InternalClaims{
		UserID:         "user_owner",
		OrganizationID: bootstrapResponse.Data.Organization.ID,
		MemberID:       "synthetic-member",
		SourceService:  auth.InternalSourceServiceWeb,
		Roles:          []string{"synthetic-owner"},
		Permissions:    []string{string(domain.PermissionOrganizationRead)},
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	group := createOSSNodeGroupViaAPI(t, server, ownerInternalToken, "OSS Core Group")
	if group.ID == "" {
		t.Fatalf("expected OSS single-user authorizer to allow core resource management with synthetic claims")
	}

	otherSessionToken := signWebUserToken(t, webSigner, auth.WebUserTokenPurposeSession, "user_other", "other@example.com", "Other")
	otherSessionRecorder := httptest.NewRecorder()
	otherSessionRequest := httptest.NewRequest(http.MethodGet, "/internal/v1/session", nil)
	otherSessionRequest.Header.Set("Authorization", "Bearer "+otherSessionToken)
	server.ServeHTTP(otherSessionRecorder, otherSessionRequest)
	if otherSessionRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected second OSS user session to be forbidden, got %d body=%s", otherSessionRecorder.Code, otherSessionRecorder.Body.String())
	}
	assertOSSErrorCode(t, otherSessionRecorder, "OSS_OWNER_REQUIRED")

	otherBootstrap := postBootstrap(t, server, webSigner, "user_other", "other@example.com")
	if otherBootstrap.Code != http.StatusForbidden {
		t.Fatalf("expected second OSS user bootstrap to be forbidden, got %d body=%s", otherBootstrap.Code, otherBootstrap.Body.String())
	}
	assertOSSErrorCode(t, otherBootstrap, "OSS_OWNER_REQUIRED")
}

func TestOSSControlServerPostgresCoreListAPIs(t *testing.T) {
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
		Permissions: []string{
			string(domain.PermissionOrganizationRead),
			string(domain.PermissionNodesRead),
			string(domain.PermissionNodesManage),
			string(domain.PermissionTargetsRead),
			string(domain.PermissionTargetsManage),
			string(domain.PermissionRulesReadAll),
			string(domain.PermissionRulesManageAll),
			string(domain.PermissionTrafficReadAll),
		},
		ResourceScopes: []auth.ResourceScopeClaim{{ResourceType: string(domain.ResourceTypeNodeGroup), ResourceID: "*", AccessLevel: string(domain.AccessLevelManage)}},
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	group := createOSSNodeGroupViaAPI(t, server, token, "OSS API Group")
	node := createOSSNodeViaAPI(t, server, token, group.ID, "OSS API Node")
	target := createOSSTargetViaAPI(t, server, token, "OSS API Target")
	targetGroup := createOSSTargetGroupViaAPI(t, server, token, target.ID, "OSS API Target Group")
	controlService := service.NewControlServiceWithOptions(store, service.ControlServiceOptions{
		Edition:                 edition.OSSProvider(),
		AppName:                 "OSS Control Console",
		ControlPlaneURL:         "http://127.0.0.1:8080",
		AgentReleaseVersion:     "v0.0.0-test",
		AgentTokenSigningSecret: []byte("agent-token-secret-32-byte-test-key"),
	})
	if _, err := controlService.CreateRule(context.Background(), service.InternalIdentity{
		UserID:         "user_owner",
		OrganizationID: bootstrapResponse.Data.Organization.ID,
		MemberID:       "synthetic-member",
		Roles:          []string{"synthetic-owner"},
		Permissions:    []string{string(domain.PermissionRulesManageAll)},
		ResourceScopes: []service.ResourceScopePayload{{ResourceType: string(domain.ResourceTypeNodeGroup), ResourceID: "*", AccessLevel: string(domain.AccessLevelManage)}},
	}, service.RuleMutationInput{
		Name:           "Service smoke rule",
		Tags:           []string{"smoke"},
		NodeGroupID:    group.ID,
		ListenIP:       "0.0.0.0",
		ForwardingType: "DIRECT",
		Protocol:       "TCP",
		Port:           10001,
		Match:          service.RuleMatchInput{Type: "ANY_INBOUND"},
		ProxyProtocol:  service.RuleProxyProtocolInput{},
		Upstream:       service.RuleUpstreamInput{Type: "TARGET_GROUP", TargetGroupID: targetGroup.ID},
		Enabled:        false,
		EnabledSet:     true,
	}); err != nil {
		t.Fatalf("expected direct Postgres rule create to succeed: %v", err)
	}
	rule := createOSSRuleViaAPI(t, server, token, group.ID, targetGroup.ID, "OSS API Rule")
	registrationToken := createOSSNodeRegistrationTokenViaAPI(t, server, token, node.ID)
	if _, err := store.Nodes().ListNodesByOrganization(context.Background(), bootstrapResponse.Data.Organization.ID); err != nil {
		t.Fatalf("expected direct Postgres node listing to succeed: %v", err)
	}
	if _, err := controlService.ListNodes(context.Background(), service.InternalIdentity{
		UserID:         "user_owner",
		OrganizationID: bootstrapResponse.Data.Organization.ID,
		MemberID:       "synthetic-member",
		Roles:          []string{"synthetic-owner"},
		Permissions:    []string{string(domain.PermissionNodesRead)},
		ResourceScopes: []service.ResourceScopePayload{{ResourceType: string(domain.ResourceTypeNodeGroup), ResourceID: "*", AccessLevel: string(domain.AccessLevelManage)}},
	}); err != nil {
		t.Fatalf("expected service Postgres node listing to succeed: %v", err)
	}

	for _, path := range []string{
		"/internal/v1/node-groups",
		"/internal/v1/nodes",
		"/internal/v1/nodes/" + node.ID,
		"/internal/v1/nodes/" + node.ID + "/registration-tokens",
		"/internal/v1/resource-options/node-groups",
		"/internal/v1/resource-options/node-group-listen-ips?node_group_id=" + group.ID,
		"/internal/v1/resource-options/node-group-listen-ips?node_group_id=" + group.ID + "&protocol=TCP&port=10000",
		"/internal/v1/resource-options/targets",
		"/internal/v1/resource-options/target-groups",
		"/internal/v1/targets",
		"/internal/v1/target-groups",
		"/internal/v1/rules",
		"/internal/v1/rules/" + rule.ID,
		"/internal/v1/rules/" + rule.ID + "/traffic",
		"/internal/v1/rules/" + rule.ID + "/diagnostics",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("expected %s to return 200, got %d body=%s", path, response.Code, response.Body.String())
		}
	}
	revokeNodeRegistrationTokenViaAPI(t, server, token, node.ID, registrationToken.TokenID)
}

type testControlRouteExtension struct {
	register func(ControlRouteRegistry)
}

func (extension testControlRouteExtension) RegisterControlRoutes(registry ControlRouteRegistry) {
	extension.register(registry)
}

func openMigratedOSSControlTestStore(t *testing.T) (*sql.DB, *repo.PostgresStore) {
	t.Helper()

	root := repoRoot(t)
	baseURL := os.Getenv("TEST_DATABASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("DATABASE_URL")
	}
	if baseURL == "" {
		t.Skip("TEST_DATABASE_URL or DATABASE_URL is required for PostgreSQL handler integration tests")
	}
	databaseName := "prism_handler_test_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	adminDB, err := sql.Open("pgx", baseURL)
	if err != nil {
		t.Fatalf("open postgres admin database: %v", err)
	}
	if _, err := adminDB.Exec(`CREATE DATABASE ` + quoteIdentifier(databaseName)); err != nil {
		_ = adminDB.Close()
		t.Fatalf("create handler test database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := adminDB.Exec(`DROP DATABASE IF EXISTS ` + quoteIdentifier(databaseName) + ` WITH (FORCE)`); err != nil {
			t.Fatalf("drop handler test database: %v", err)
		}
		_ = adminDB.Close()
	})
	migrationURL := databaseURLWithName(t, baseURL, databaseName, "")
	databaseURL := databaseURLWithName(t, baseURL, databaseName, "app,auth,public")
	cmd := exec.Command("go", "run", "./cmd/migrate", "-database", migrationURL, "-dir", "migrations/auth,migrations/core", "up")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run OSS migrate command: %v output=%s", err, output)
	}

	db, err := repo.OpenPostgres(databaseURL)
	if err != nil {
		t.Fatalf("open OSS postgres: %v", err)
	}
	return db, repo.NewPostgresStore(db)
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func databaseURLWithName(t *testing.T, rawURL string, databaseName string, searchPath string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse database URL: %v", err)
	}
	parsed.Path = "/" + databaseName
	query := parsed.Query()
	if searchPath != "" {
		query.Set("options", "-c search_path="+searchPath)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func assertMissingPermission(t *testing.T, permissions []string, expected string) {
	t.Helper()
	for _, permission := range permissions {
		if permission == expected {
			t.Fatalf("permission %q should not be present in %#v", expected, permissions)
		}
	}
}

type ossNodeGroupPayload struct {
	ID string `json:"id"`
}

type ossNodePayload struct {
	ID string `json:"id"`
}

type ossTargetPayload struct {
	ID string `json:"id"`
}

type ossTargetGroupPayload struct {
	ID string `json:"id"`
}

type ossRulePayload struct {
	ID string `json:"id"`
}

type ossRegistrationTokenPayload struct {
	TokenID string `json:"token_id"`
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

func createOSSNodeViaAPI(t *testing.T, server http.Handler, token string, groupID string, name string) ossNodePayload {
	t.Helper()

	body := `{
		"name":"` + name + `",
		"group_ids":["` + groupID + `"],
		"listen_ips":[{"listen_ip":"0.0.0.0","display_name":"default"}],
		"port_ranges":[{"protocol":"TCP","start_port":10000,"end_port":20000}],
		"public_description":""
	}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/nodes", bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected OSS node create 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data ossNodePayload `json:"data"`
	}
	decodeJSON(t, recorder, &response)
	return response.Data
}

func createOSSTargetViaAPI(t *testing.T, server http.Handler, token string, name string) ossTargetPayload {
	t.Helper()

	body := `{"name":"` + name + `","host":"1.1.1.1","port":443,"enabled":true}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/targets", bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected OSS target create 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data ossTargetPayload `json:"data"`
	}
	decodeJSON(t, recorder, &response)
	return response.Data
}

func createOSSTargetGroupViaAPI(t *testing.T, server http.Handler, token string, targetID string, name string) ossTargetGroupPayload {
	t.Helper()

	body := `{
		"name":"` + name + `",
		"description":"OSS API target group",
		"scheduler":"PRIORITY_IPHASH",
		"members":[{"target_id":"` + targetID + `","priority":10,"enabled":true}]
	}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/target-groups", bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected OSS target group create 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data ossTargetGroupPayload `json:"data"`
	}
	decodeJSON(t, recorder, &response)
	return response.Data
}

func createOSSRuleViaAPI(t *testing.T, server http.Handler, token string, nodeGroupID string, targetGroupID string, name string) ossRulePayload {
	t.Helper()

	body := `{
		"name":"` + name + `",
		"tags":["smoke"],
		"node_group_id":"` + nodeGroupID + `",
		"listen_ip":"0.0.0.0",
		"forwarding_type":"DIRECT",
		"protocol":"TCP",
		"port":10000,
		"match":{"type":"ANY_INBOUND"},
		"proxy_protocol":{"in":"NONE","out":"NONE"},
		"upstream":{"type":"TARGET_GROUP","target_group_id":"` + targetGroupID + `"},
		"enabled":false
	}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/rules", bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected OSS rule create 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data ossRulePayload `json:"data"`
	}
	decodeJSON(t, recorder, &response)
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
