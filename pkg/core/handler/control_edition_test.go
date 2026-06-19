package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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

type testControlRouteExtension struct {
	register func(ControlRouteRegistry)
}

func (extension testControlRouteExtension) RegisterControlRoutes(registry ControlRouteRegistry) {
	extension.register(registry)
}

func openMigratedOSSControlTestStore(t *testing.T) (*sql.DB, *repo.SQLiteStore) {
	t.Helper()

	root := repoRoot(t)
	databasePath := filepath.Join(t.TempDir(), "control-oss.db")
	cmd := exec.Command("go", "run", "./cmd/migrate", "-database", databasePath, "-dir", "migrations/core", "up")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run OSS migrate command: %v output=%s", err, output)
	}

	db, err := repo.OpenSQLite(databasePath)
	if err != nil {
		t.Fatalf("open OSS sqlite: %v", err)
	}
	return db, repo.NewSQLiteStore(db)
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
