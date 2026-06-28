package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/auth"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/service"
	"github.com/noxaaa/prism-oss/pkg/edition"
)

func TestEnrollmentPendingCredentialRejectsRotatedProfileBeforeActivation(t *testing.T) {
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
	group := createOSSNodeGroupViaAPI(t, server, token, "Enrollment Activation Group")
	profile := createOSSNodeEnrollmentProfileWithMaxUsesViaAPI(t, server, token, group.ID, 1)
	controlService := service.NewControlServiceWithOptions(store, service.ControlServiceOptions{
		Edition:                 edition.OSSProvider(),
		AppName:                 "OSS Control Console",
		ControlPlaneURL:         "http://127.0.0.1:8080",
		AgentReleaseVersion:     "v0.0.0-test",
		AgentTokenSigningSecret: []byte("agent-token-secret-32-byte-test-key"),
	})
	pendingAuth, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", profile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-before-activation-rotate",
		RemoteIP: "203.0.113.44",
	})
	if err != nil {
		t.Fatalf("expected enrollment token to create pending node, got %v", err)
	}
	rotateOSSNodeEnrollmentProfileTokenViaAPI(t, server, token, profile.ID)
	if err := controlService.FinalizeAgentRegistrationDelivery(context.Background(), pendingAuth); err == nil {
		t.Fatalf("expected pending credential finalization to fail after profile token rotation")
	}
	if _, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", pendingAuth.AgentCredential, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-before-activation-rotate",
		RemoteIP: "203.0.113.44",
	}); err == nil {
		t.Fatalf("expected pending credential activation to fail after profile token rotation")
	}
	if err := controlService.ReleaseAgentRegistrationCredential(context.Background(), pendingAuth); err != nil {
		t.Fatalf("release rotated pending enrollment: %v", err)
	}
}

func TestEnrollmentSuccessEventIsRecordedAfterCredentialFinalization(t *testing.T) {
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
	group := createOSSNodeGroupViaAPI(t, server, token, "Enrollment Success Event Group")
	profile := createOSSNodeEnrollmentProfileWithMaxUsesViaAPI(t, server, token, group.ID, 1)
	controlService := service.NewControlServiceWithOptions(store, service.ControlServiceOptions{
		Edition:                 edition.OSSProvider(),
		AppName:                 "OSS Control Console",
		ControlPlaneURL:         "http://127.0.0.1:8080",
		AgentReleaseVersion:     "v0.0.0-test",
		AgentTokenSigningSecret: []byte("agent-token-secret-32-byte-test-key"),
	})
	pendingAuth, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", profile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-success-after-ack",
		RemoteIP: "203.0.113.45",
	})
	if err != nil {
		t.Fatalf("expected enrollment token to create pending node, got %v", err)
	}
	assertOSSNodeEnrollmentEventsDoNotIncludeSuccessViaAPI(t, server, token, profile.ID)
	if err := controlService.FinalizeAgentRegistrationDelivery(context.Background(), pendingAuth); err != nil {
		t.Fatalf("expected pending credential finalization to record success, got %v", err)
	}
	assertOSSNodeEnrollmentEventsIncludeSuccessViaAPI(t, server, token, profile.ID)
}

func TestEnrollmentReleaseDoesNotDeleteNodeAfterPendingCredentialReconnect(t *testing.T) {
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
	group := createOSSNodeGroupViaAPI(t, server, token, "Enrollment Reconnect Group")
	profile := createOSSNodeEnrollmentProfileWithMaxUsesViaAPI(t, server, token, group.ID, 1)
	controlService := service.NewControlServiceWithOptions(store, service.ControlServiceOptions{
		Edition:                 edition.OSSProvider(),
		AppName:                 "OSS Control Console",
		ControlPlaneURL:         "http://127.0.0.1:8080",
		AgentReleaseVersion:     "v0.0.0-test",
		AgentTokenSigningSecret: []byte("agent-token-secret-32-byte-test-key"),
	})
	pendingAuth, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", profile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-pending-reconnect",
		RemoteIP: "203.0.113.46",
	})
	if err != nil {
		t.Fatalf("expected enrollment token to create pending node, got %v", err)
	}
	reconnectedAuth, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", pendingAuth.AgentCredential, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-pending-reconnect",
		RemoteIP: "203.0.113.46",
	})
	if err != nil {
		t.Fatalf("expected pending enrollment credential reconnect to authenticate, got %v", err)
	}
	if reconnectedAuth.AgentID != pendingAuth.AgentID {
		t.Fatalf("expected pending enrollment credential reconnect to reuse node %q, got %q", pendingAuth.AgentID, reconnectedAuth.AgentID)
	}
	if err := controlService.ReleaseAgentRegistrationCredential(context.Background(), pendingAuth); err != nil {
		t.Fatalf("release original pending enrollment after credential reconnect: %v", err)
	}
	if _, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", pendingAuth.AgentCredential, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-pending-reconnect",
		RemoteIP: "203.0.113.46",
	}); err != nil {
		t.Fatalf("expected old websocket release not to revoke activated enrollment credential, got %v", err)
	}
	if _, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", profile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-pending-after-release",
		RemoteIP: "203.0.113.47",
	}); err == nil {
		t.Fatalf("expected old websocket release not to decrement active enrollment use")
	}
}

func TestDeleteEnrollmentProfileReleasesPendingEnrollmentNodes(t *testing.T) {
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
	group := createOSSNodeGroupViaAPI(t, server, token, "Enrollment Delete Release Group")
	profile := createOSSNodeEnrollmentProfileWithMaxUsesViaAPI(t, server, token, group.ID, 1)
	controlService := service.NewControlServiceWithOptions(store, service.ControlServiceOptions{
		Edition:                 edition.OSSProvider(),
		AppName:                 "OSS Control Console",
		ControlPlaneURL:         "http://127.0.0.1:8080",
		AgentReleaseVersion:     "v0.0.0-test",
		AgentTokenSigningSecret: []byte("agent-token-secret-32-byte-test-key"),
	})
	pendingAuth, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", profile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-delete-profile",
		RemoteIP: "203.0.113.48",
	})
	if err != nil {
		t.Fatalf("expected enrollment token to create pending node, got %v", err)
	}
	deleteOSSNodeEnrollmentProfileViaAPI(t, server, token, profile.ID)
	nodes, err := store.Nodes().ListNodesByOrganization(context.Background(), bootstrapResponse.Data.Organization.ID)
	if err != nil {
		t.Fatalf("list nodes after deleting enrollment profile: %v", err)
	}
	for _, node := range nodes {
		if node.ID == pendingAuth.AgentID {
			t.Fatalf("expected deleting profile to delete pending enrollment node %q", pendingAuth.AgentID)
		}
	}
	if _, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", pendingAuth.AgentCredential, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-delete-profile",
		RemoteIP: "203.0.113.48",
	}); err == nil {
		t.Fatalf("expected pending credential to be revoked when enrollment profile is deleted")
	}
}

func TestDeletePendingEnrollmentNodeReleasesProfileUse(t *testing.T) {
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
	group := createOSSNodeGroupViaAPI(t, server, token, "Enrollment Pending Delete Group")
	profile := createOSSNodeEnrollmentProfileWithMaxUsesViaAPI(t, server, token, group.ID, 1)
	controlService := service.NewControlServiceWithOptions(store, service.ControlServiceOptions{
		Edition:                 edition.OSSProvider(),
		AppName:                 "OSS Control Console",
		ControlPlaneURL:         "http://127.0.0.1:8080",
		AgentReleaseVersion:     "v0.0.0-test",
		AgentTokenSigningSecret: []byte("agent-token-secret-32-byte-test-key"),
	})
	pendingAuth, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", profile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-delete-pending-node",
		RemoteIP: "203.0.113.49",
	})
	if err != nil {
		t.Fatalf("expected enrollment token to create pending node, got %v", err)
	}
	deleteOSSNodeViaAPI(t, server, token, pendingAuth.AgentID)
	if _, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", profile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-reuse-after-node-delete",
		RemoteIP: "203.0.113.50",
	}); err != nil {
		t.Fatalf("expected deleting pending enrollment node to release max_uses slot, got %v", err)
	}
	if _, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", pendingAuth.AgentCredential, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-delete-pending-node",
		RemoteIP: "203.0.113.49",
	}); err == nil {
		t.Fatalf("expected deleting pending enrollment node to revoke pending credential")
	}
}

func TestEnrollmentPreflightFailureRecordsMetadata(t *testing.T) {
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
		AppName:                 "OSS: Control # Console",
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
	group := createOSSNodeGroupViaAPI(t, server, token, "Enrollment Preflight Metadata Group")
	profile := createOSSNodeEnrollmentProfileWithEnabledViaAPI(t, server, token, group.ID, false)
	controlService := service.NewControlServiceWithOptions(store, service.ControlServiceOptions{
		Edition:                 edition.OSSProvider(),
		AppName:                 "OSS: Control # Console",
		ControlPlaneURL:         "http://127.0.0.1:8080",
		AgentReleaseVersion:     "v0.0.0-test",
		AgentTokenSigningSecret: []byte("agent-token-secret-32-byte-test-key"),
	})
	if _, err := controlService.ValidateAgentTokenWithMetadata(context.Background(), "NODE", profile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-preflight-denied",
		RemoteIP: "203.0.113.61",
	}); err == nil {
		t.Fatalf("expected disabled enrollment token preflight to fail")
	}
	assertOSSNodeEnrollmentEventsIncludeFailureWithMetadataViaAPI(t, server, token, profile.ID, "ENROLLMENT_REVOKED", "203.0.113.61", "autoscale-preflight-denied")
}

func TestPendingEnrollmentCredentialActivationEnforcesCIDR(t *testing.T) {
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
	group := createOSSNodeGroupViaAPI(t, server, token, "Enrollment Pending CIDR Group")
	profile := createOSSNodeEnrollmentProfileWithAllowedCIDRsViaAPI(t, server, token, group.ID, []string{"203.0.113.0/24"})
	controlService := service.NewControlServiceWithOptions(store, service.ControlServiceOptions{
		Edition:                 edition.OSSProvider(),
		AppName:                 "OSS Control Console",
		ControlPlaneURL:         "http://127.0.0.1:8080",
		AgentReleaseVersion:     "v0.0.0-test",
		AgentTokenSigningSecret: []byte("agent-token-secret-32-byte-test-key"),
	})
	pendingAuth, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", profile.Token, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-cidr-pending",
		RemoteIP: "203.0.113.62",
	})
	if err != nil {
		t.Fatalf("expected enrollment token to create pending node from allowed CIDR, got %v", err)
	}
	if _, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", pendingAuth.AgentCredential, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-cidr-pending",
		RemoteIP: "198.51.100.62",
	}); err == nil {
		t.Fatalf("expected pending credential activation from disallowed CIDR to fail")
	}
	if _, err := controlService.AuthenticateAgentTokenWithMetadata(context.Background(), "NODE", pendingAuth.AgentCredential, service.AgentEnrollmentMetadata{
		Hostname: "autoscale-cidr-pending",
		RemoteIP: "203.0.113.63",
	}); err != nil {
		t.Fatalf("expected pending credential activation from allowed CIDR to succeed, got %v", err)
	}
}
