package handler

import (
	"bytes"
	"context"
	"database/sql"
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
	"github.com/noxaaa/prism-oss/pkg/core/dns"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
	"github.com/noxaaa/prism-oss/pkg/core/service"
	"github.com/noxaaa/prism-oss/pkg/edition"
)

type ossRouteTestDNSProvider struct{}

func (ossRouteTestDNSProvider) ApplyRecord(context.Context, dns.ApplyRecordInput) error {
	return nil
}

func (ossRouteTestDNSProvider) ListZones(context.Context, string) ([]dns.ZoneInfo, error) {
	return []dns.ZoneInfo{{ID: "zone_1", Name: "example.com", Status: "ACTIVE"}}, nil
}

type ossRouteFailingDNSProvider struct{}

func (ossRouteFailingDNSProvider) ApplyRecord(context.Context, dns.ApplyRecordInput) error {
	return nil
}

func (ossRouteFailingDNSProvider) ListZones(context.Context, string) ([]dns.ZoneInfo, error) {
	return nil, context.Canceled
}

func TestOSSControlServerDoesNotRegisterRBACRoutes(t *testing.T) {
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
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("expected RBAC route %s to be unregistered in OSS, got %d body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestOSSProviderEnablesMonitorHealthAndDNSCapabilities(t *testing.T) {
	provider := edition.OSSProvider()
	for _, capability := range []edition.Capability{
		edition.CapabilityMonitors,
		edition.CapabilityHealthChecks,
		edition.CapabilityDNS,
	} {
		if !provider.Has(capability) {
			t.Fatalf("expected OSS provider to enable %s", capability)
		}
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

func TestOSSControlServerDNSCredentialDiscoveryFailureIsValidationError(t *testing.T) {
	signer := auth.HMACInternalTokenSigner{Secret: []byte("test-secret")}
	server := NewControlServer(ControlServerOptions{
		TokenVerifier: signer,
		Edition:       edition.OSSProvider(),
		ControlService: service.NewControlServiceWithOptions(nil, service.ControlServiceOptions{
			Edition:                edition.OSSProvider(),
			DNSSecretEncryptionKey: "test-dns-secret-key",
			DNSProviders:           dns.StaticProviderRegistry{"CLOUDFLARE": ossRouteFailingDNSProvider{}},
		}),
		AgentTokenSigningSecret: []byte("agent-token-secret-32-byte-test-key"),
	})
	token := signInternalToken(t, signer, auth.InternalClaims{
		UserID:         "user_owner",
		OrganizationID: "org_oss",
		MemberID:       "member_oss",
		Roles:          []string{"owner"},
		Permissions:    []string{string(domain.PermissionDNSManage)},
		ExpiresAt:      time.Now().Add(time.Minute),
	})

	request := httptest.NewRequest(http.MethodPost, "/internal/v1/dns/credentials", bytes.NewBufferString(`{"name":"test","provider":"CLOUDFLARE","secret":"test-cloudflare-token"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected DNS credential discovery failure to return 400, got %d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "test-cloudflare-token") {
		t.Fatalf("DNS credential discovery failure must not expose token: %s", response.Body.String())
	}
	assertOSSErrorCode(t, response, "VALIDATION_FAILED")
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
	assertPermission(t, bootstrapResponse.Data.Permissions, string(domain.PermissionMonitorsRead))
	assertPermission(t, bootstrapResponse.Data.Permissions, string(domain.PermissionMonitorsManage))
	assertPermission(t, bootstrapResponse.Data.Permissions, string(domain.PermissionDNSRead))
	assertPermission(t, bootstrapResponse.Data.Permissions, string(domain.PermissionDNSManage))
	assertPermission(t, bootstrapResponse.Data.Permissions, string(domain.PermissionHealthChecksRead))
	assertPermission(t, bootstrapResponse.Data.Permissions, string(domain.PermissionHealthChecksManage))
	assertMissingPermission(t, bootstrapResponse.Data.Permissions, "members.manage")
	assertMissingPermission(t, bootstrapResponse.Data.Permissions, "roles.manage")
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
		DNSSecretEncryptionKey:  "test-dns-secret-key",
		DNSProviders:            dns.StaticProviderRegistry{"CLOUDFLARE": ossRouteTestDNSProvider{}},
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
			string(domain.PermissionMonitorsRead),
			string(domain.PermissionMonitorsManage),
			string(domain.PermissionTargetsRead),
			string(domain.PermissionTargetsManage),
			string(domain.PermissionRulesReadAll),
			string(domain.PermissionRulesManageAll),
			string(domain.PermissionTrafficReadAll),
			string(domain.PermissionHealthChecksRead),
			string(domain.PermissionHealthChecksManage),
			string(domain.PermissionDNSRead),
			string(domain.PermissionDNSManage),
		},
		ResourceScopes: []auth.ResourceScopeClaim{{ResourceType: string(domain.ResourceTypeNodeGroup), ResourceID: "*", AccessLevel: string(domain.AccessLevelManage)}},
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	group := createOSSNodeGroupViaAPI(t, server, token, "OSS API Group")
	enrollmentProfile := createOSSNodeEnrollmentProfileViaAPI(t, server, token, group.ID)
	limitedEnrollmentProfile := createOSSNodeEnrollmentProfileWithMaxUsesViaAPI(t, server, token, group.ID, 1)
	rotatedLimitedEnrollmentProfile := createOSSNodeEnrollmentProfileWithMaxUsesViaAPI(t, server, token, group.ID, 1)
	disabledEnrollmentProfile := createOSSNodeEnrollmentProfileWithEnabledViaAPI(t, server, token, group.ID, false)
	rotatedDisabledEnrollmentProfile := rotateOSSNodeEnrollmentProfileTokenViaAPI(t, server, token, disabledEnrollmentProfile.ID)
	if rotatedDisabledEnrollmentProfile.Enabled {
		t.Fatalf("expected rotating disabled enrollment profile to preserve disabled state")
	}
	enrollmentOnlyGroup := createOSSNodeGroupViaAPI(t, server, token, "OSS API Enrollment Only Group")
	enrollmentOnlyProfile := createOSSNodeEnrollmentProfileViaAPI(t, server, token, enrollmentOnlyGroup.ID)
	assertOSSNodeGroupDeleteConflictViaAPI(t, server, token, enrollmentOnlyGroup.ID)
	node := createOSSNodeViaAPI(t, server, token, group.ID, "OSS API Node")
	monitorGroup := createOSSMonitorGroupViaAPI(t, server, token, "OSS API Monitor Group")
	monitor := createOSSMonitorViaAPI(t, server, token, monitorGroup.ID, "OSS API Monitor")
	target := createOSSTargetViaAPI(t, server, token, "OSS API Target")
	targetGroup := createOSSTargetGroupViaAPI(t, server, token, target.ID, "OSS API Target Group")
	healthCheck := createOSSHealthCheckViaAPI(t, server, token, target.ID, monitor.ID, "OSS API Health")
	dnsCredential := createOSSDNSCredentialViaAPI(t, server, token, "OSS API Cloudflare")
	dnsManagedRecord := createOSSDNSManagedRecordViaAPI(t, server, token, dnsCredential, "smart", "A")
	dnsInstance := createOSSDNSInstanceViaAPI(t, server, token, dnsManagedRecord.ID, "OSS API DNS Instance")
	assertOSSDNSManagedRecordTypeConflictViaAPI(t, server, token, dnsCredential, "smart", "CNAME")
	serviceNow := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	controlService := service.NewControlServiceWithOptions(store, service.ControlServiceOptions{
		Edition:                 edition.OSSProvider(),
		AppName:                 "OSS Control Console",
		ControlPlaneURL:         "http://127.0.0.1:8080",
		AgentReleaseVersion:     "v0.0.0-test",
		AgentTokenSigningSecret: []byte("agent-token-secret-32-byte-test-key"),
		Now: func() time.Time {
			return serviceNow
		},
	})
	enrollmentAuth, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", limitedEnrollmentProfile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-one",
		RemoteIP: "203.0.113.10",
	})
	if err != nil {
		t.Fatalf("expected limited enrollment token to create node, got %v", err)
	}
	if _, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", limitedEnrollmentProfile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-two",
		RemoteIP: "203.0.113.11",
	}); err == nil {
		t.Fatalf("expected limited enrollment token to reject second concurrent use")
	}
	assertOSSNodeEnrollmentEventsIncludeFailureViaAPI(t, server, token, limitedEnrollmentProfile.ID, "ENROLLMENT_MAX_USES_EXCEEDED")
	if err := controlService.ReleaseAgentRegistrationCredential(context.Background(), enrollmentAuth); err != nil {
		t.Fatalf("release unfinalized enrollment: %v", err)
	}
	reusedEnrollmentAuth, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", limitedEnrollmentProfile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-three",
		RemoteIP: "203.0.113.12",
	})
	if err != nil {
		t.Fatalf("expected released enrollment reservation to become reusable, got %v", err)
	}
	if err := controlService.ReleaseAgentRegistrationCredential(context.Background(), reusedEnrollmentAuth); err != nil {
		t.Fatalf("release reused enrollment: %v", err)
	}
	staleLimitedEnrollmentProfile := createOSSNodeEnrollmentProfileWithMaxUsesViaAPI(t, server, token, group.ID, 1)
	staleEnrollmentAuth, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", staleLimitedEnrollmentProfile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-stale-one",
		RemoteIP: "203.0.113.13",
	})
	if err != nil {
		t.Fatalf("expected stale enrollment setup to create node, got %v", err)
	}
	serviceNow = serviceNow.Add(10 * time.Minute)
	if _, err := controlService.ValidateAgentToken(context.Background(), "NODE", staleLimitedEnrollmentProfile.Token); err != nil {
		t.Fatalf("expected stale enrollment preflight to reclaim abandoned reservation, got %v", err)
	}
	reclaimedEnrollmentAuth, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", staleLimitedEnrollmentProfile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-stale-two",
		RemoteIP: "203.0.113.14",
	})
	if err != nil {
		t.Fatalf("expected stale enrollment reservation to be reclaimed, got %v", err)
	}
	if err := controlService.ReleaseAgentRegistrationCredential(context.Background(), staleEnrollmentAuth); err != nil {
		t.Fatalf("release stale enrollment reservation after cleanup: %v", err)
	}
	if err := controlService.ReleaseAgentRegistrationCredential(context.Background(), reclaimedEnrollmentAuth); err != nil {
		t.Fatalf("release reclaimed enrollment reservation: %v", err)
	}
	pendingReconnectProfile := createOSSNodeEnrollmentProfileWithMaxUsesViaAPI(t, server, token, group.ID, 1)
	pendingReconnectAuth, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", pendingReconnectProfile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-pending-reconnect",
		RemoteIP: "203.0.113.15",
	})
	if err != nil {
		t.Fatalf("expected pending reconnect setup to create node, got %v", err)
	}
	reconnectedAuth, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", pendingReconnectAuth.AgentCredential, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-pending-reconnect",
		RemoteIP: "203.0.113.15",
	})
	if err != nil {
		t.Fatalf("expected pending enrollment credential reconnect to authenticate, got %v", err)
	}
	if reconnectedAuth.AgentID != pendingReconnectAuth.AgentID {
		t.Fatalf("expected pending enrollment credential reconnect to reuse node %q, got %q", pendingReconnectAuth.AgentID, reconnectedAuth.AgentID)
	}
	if _, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", pendingReconnectProfile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-pending-duplicate",
		RemoteIP: "203.0.113.16",
	}); err == nil {
		t.Fatalf("expected pending credential reconnect to keep enrollment max uses consumed")
	}
	duplicateReleaseProfile := createOSSNodeEnrollmentProfileWithMaxUsesViaAPI(t, server, token, group.ID, 2)
	duplicateReleaseA, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", duplicateReleaseProfile.Token, service.AgentEnrollmentMetadata{Hostname: "autoscale-duplicate-a", RemoteIP: "203.0.113.17"})
	if err != nil {
		t.Fatalf("expected duplicate release setup A to create node, got %v", err)
	}
	duplicateReleaseB, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", duplicateReleaseProfile.Token, service.AgentEnrollmentMetadata{Hostname: "autoscale-duplicate-b", RemoteIP: "203.0.113.18"})
	if err != nil {
		t.Fatalf("expected duplicate release setup B to create node, got %v", err)
	}
	if err := controlService.ReleaseAgentRegistrationCredential(context.Background(), duplicateReleaseA); err != nil {
		t.Fatalf("release duplicate A: %v", err)
	}
	if err := controlService.ReleaseAgentRegistrationCredential(context.Background(), duplicateReleaseA); err != nil {
		t.Fatalf("duplicate release duplicate A: %v", err)
	}
	duplicateReleaseC, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", duplicateReleaseProfile.Token, service.AgentEnrollmentMetadata{Hostname: "autoscale-duplicate-c", RemoteIP: "203.0.113.19"})
	if err != nil {
		t.Fatalf("expected one released enrollment use to be reusable, got %v", err)
	}
	if _, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", duplicateReleaseProfile.Token, service.AgentEnrollmentMetadata{Hostname: "autoscale-duplicate-d", RemoteIP: "203.0.113.23"}); err == nil {
		t.Fatalf("expected duplicate release not to undercount live enrollment uses")
	}
	if err := controlService.ReleaseAgentRegistrationCredential(context.Background(), duplicateReleaseB); err != nil {
		t.Fatalf("release duplicate B: %v", err)
	}
	if err := controlService.ReleaseAgentRegistrationCredential(context.Background(), duplicateReleaseC); err != nil {
		t.Fatalf("release duplicate C: %v", err)
	}
	rotatedOldAuth, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", rotatedLimitedEnrollmentProfile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-before-rotate",
		RemoteIP: "203.0.113.20",
	})
	if err != nil {
		t.Fatalf("expected old limited enrollment token to create node before rotation, got %v", err)
	}
	rotatedLimitedEnrollmentProfile = rotateOSSNodeEnrollmentProfileTokenViaAPI(t, server, token, rotatedLimitedEnrollmentProfile.ID)
	rotatedNewAuth, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", rotatedLimitedEnrollmentProfile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-after-rotate",
		RemoteIP: "203.0.113.21",
	})
	if err != nil {
		t.Fatalf("expected rotated enrollment token to create node, got %v", err)
	}
	if err := controlService.ReleaseAgentRegistrationCredential(context.Background(), rotatedOldAuth); err != nil {
		t.Fatalf("release old rotated enrollment reservation: %v", err)
	}
	if _, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", rotatedLimitedEnrollmentProfile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-after-old-release",
		RemoteIP: "203.0.113.22",
	}); err == nil {
		t.Fatalf("expected rotated token max uses to remain consumed after releasing old token reservation")
	}
	if err := controlService.ReleaseAgentRegistrationCredential(context.Background(), rotatedNewAuth); err != nil {
		t.Fatalf("release new rotated enrollment reservation: %v", err)
	}
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
	monitorRegistrationToken := createOSSMonitorRegistrationTokenViaAPI(t, server, token, monitor.ID)
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
		"/internal/v1/node-enrollment-profiles",
		"/internal/v1/node-enrollment-profiles/" + enrollmentProfile.ID,
		"/internal/v1/node-enrollment-profiles/" + enrollmentProfile.ID + "/events",
		"/internal/v1/nodes",
		"/internal/v1/nodes/" + node.ID,
		"/internal/v1/nodes/" + node.ID + "/registration-tokens",
		"/internal/v1/monitor-groups",
		"/internal/v1/monitors",
		"/internal/v1/monitors/" + monitor.ID,
		"/internal/v1/monitors/" + monitor.ID + "/registration-tokens",
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
		"/internal/v1/health-checks",
		"/internal/v1/health-checks/" + healthCheck.ID + "/results",
		"/internal/v1/dns/credentials",
		"/internal/v1/dns/managed-records",
		"/internal/v1/dns/instances",
		"/internal/v1/notification-channels",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("expected %s to return 200, got %d body=%s", path, response.Code, response.Body.String())
		}
	}
	assertOSSRouteNotFound(t, server, token, "/internal/v1/dns/"+"records")
	rotateOSSNodeEnrollmentProfileTokenViaAPI(t, server, token, enrollmentProfile.ID)
	revokeNodeRegistrationTokenViaAPI(t, server, token, node.ID, registrationToken.TokenID)
	revokeMonitorRegistrationTokenViaAPI(t, server, token, monitor.ID, monitorRegistrationToken.TokenID)
	deleteOSSDNSInstanceViaAPI(t, server, token, dnsInstance.ID)
	deleteOSSDNSManagedRecordViaAPI(t, server, token, dnsManagedRecord.ID)
	deleteOSSDNSCredentialViaAPI(t, server, token, dnsCredential.ID)
	deleteOSSNodeEnrollmentProfileViaAPI(t, server, token, enrollmentProfile.ID)
	deleteOSSNodeEnrollmentProfileViaAPI(t, server, token, staleLimitedEnrollmentProfile.ID)
	deleteOSSNodeEnrollmentProfileViaAPI(t, server, token, pendingReconnectProfile.ID)
	deleteOSSNodeEnrollmentProfileViaAPI(t, server, token, duplicateReleaseProfile.ID)
	deleteOSSNodeEnrollmentProfileViaAPI(t, server, token, disabledEnrollmentProfile.ID)
	deleteOSSNodeEnrollmentProfileViaAPI(t, server, token, enrollmentOnlyProfile.ID)
	deleteOSSNodeGroupViaAPI(t, server, token, enrollmentOnlyGroup.ID)
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

type ossNodePayload struct {
	ID string `json:"id"`
}

type ossMonitorGroupPayload struct {
	ID string `json:"id"`
}

type ossMonitorPayload struct {
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

type ossHealthCheckPayload struct {
	ID string `json:"id"`
}

type ossDNSCredentialPayload struct {
	ID    string                        `json:"id"`
	Zones []ossDNSCredentialZonePayload `json:"zones"`
}

type ossDNSCredentialZonePayload struct {
	ID       string `json:"id"`
	ZoneID   string `json:"zone_id"`
	ZoneName string `json:"zone_name"`
}

type ossDNSManagedRecordPayload struct {
	ID string `json:"id"`
}

type ossDNSInstancePayload struct {
	ID string `json:"id"`
}

type ossRegistrationTokenPayload struct {
	TokenID string `json:"token_id"`
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

func createOSSMonitorGroupViaAPI(t *testing.T, server http.Handler, token string, name string) ossMonitorGroupPayload {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/monitor-groups", bytes.NewBufferString(`{"name":"`+name+`","description":"OSS monitor group"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected OSS monitor group create 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data ossMonitorGroupPayload `json:"data"`
	}
	decodeJSON(t, recorder, &response)
	return response.Data
}

func createOSSMonitorViaAPI(t *testing.T, server http.Handler, token string, groupID string, name string) ossMonitorPayload {
	t.Helper()

	body := `{"name":"` + name + `","group_ids":["` + groupID + `"]}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/monitors", bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected OSS monitor create 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data ossMonitorPayload `json:"data"`
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

func createOSSHealthCheckViaAPI(t *testing.T, server http.Handler, token string, targetID string, monitorID string, name string) ossHealthCheckPayload {
	t.Helper()

	body := `{
		"name":"` + name + `",
		"probe_type":"TCP_PORT",
		"interval_seconds":30,
		"timeout_seconds":5,
		"enabled":true,
		"target_scope":{"type":"TARGETS","target_ids":["` + targetID + `"]},
		"monitor_scope":{"type":"MONITOR","monitor_id":"` + monitorID + `"},
		"config":{}
	}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/health-checks", bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected OSS health check create 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data ossHealthCheckPayload `json:"data"`
	}
	decodeJSON(t, recorder, &response)
	return response.Data
}

func createOSSDNSCredentialViaAPI(t *testing.T, server http.Handler, token string, name string) ossDNSCredentialPayload {
	t.Helper()

	body := `{"name":"` + name + `","provider":"CLOUDFLARE","secret":"test-cloudflare-token"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/dns/credentials", bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected OSS DNS credential create 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "test-cloudflare-token") {
		t.Fatalf("DNS credential response must not expose the secret: %s", recorder.Body.String())
	}
	var response struct {
		Data ossDNSCredentialPayload `json:"data"`
	}
	decodeJSON(t, recorder, &response)
	if len(response.Data.Zones) == 0 {
		t.Fatalf("expected OSS DNS credential create to discover zones, got body=%s", recorder.Body.String())
	}
	return response.Data
}

func createOSSDNSManagedRecordViaAPI(t *testing.T, server http.Handler, token string, credential ossDNSCredentialPayload, host string, recordType string) ossDNSManagedRecordPayload {
	t.Helper()
	if len(credential.Zones) == 0 {
		t.Fatalf("expected DNS credential to include at least one zone")
	}

	body := `{
		"dns_credential_id":"` + credential.ID + `",
		"credential_zone_id":"` + credential.Zones[0].ID + `",
		"record_host":"` + host + `",
		"record_type":"` + recordType + `",
		"ttl":60,
		"proxied":false
	}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/dns/managed-records", bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected OSS DNS managed record create 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data ossDNSManagedRecordPayload `json:"data"`
	}
	decodeJSON(t, recorder, &response)
	return response.Data
}

func assertOSSRouteNotFound(t *testing.T, server http.Handler, token string, path string) {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected %s to return 404, got %d body=%s", path, recorder.Code, recorder.Body.String())
	}
}

func assertOSSDNSManagedRecordTypeConflictViaAPI(t *testing.T, server http.Handler, token string, credential ossDNSCredentialPayload, host string, recordType string) {
	t.Helper()
	if len(credential.Zones) == 0 {
		t.Fatalf("expected DNS credential to include at least one zone")
	}

	body := `{
		"dns_credential_id":"` + credential.ID + `",
		"credential_zone_id":"` + credential.Zones[0].ID + `",
		"record_host":"` + host + `",
		"record_type":"` + recordType + `",
		"ttl":60,
		"proxied":false
	}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/dns/managed-records", bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected OSS DNS managed record type conflict 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func createOSSDNSInstanceViaAPI(t *testing.T, server http.Handler, token string, managedRecordID string, name string) ossDNSInstancePayload {
	t.Helper()

	body := `{
		"managed_record_id":"` + managedRecordID + `",
		"name":"` + name + `",
		"priority":10,
		"enabled":true,
		"node_group_ids":[],
		"answer_count":-1,
		"condition":{},
		"action":{"type":"SET_STATIC_ADDRESSES","values":["192.0.2.10"]},
		"notification_channel_ids":[]
	}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/dns/instances", bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected OSS DNS instance create 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data ossDNSInstancePayload `json:"data"`
	}
	decodeJSON(t, recorder, &response)
	return response.Data
}

func deleteOSSDNSInstanceViaAPI(t *testing.T, server http.Handler, token string, instanceID string) {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/internal/v1/dns/instances/"+instanceID, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected OSS DNS instance delete 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func deleteOSSDNSManagedRecordViaAPI(t *testing.T, server http.Handler, token string, recordID string) {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/internal/v1/dns/managed-records/"+recordID, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected OSS DNS managed record delete 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}
