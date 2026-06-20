package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/noxaaa/prism-oss/pkg/core/auth"
)

type controlResponse struct {
	Data struct {
		Organization   organizationPayload    `json:"organization"`
		Organizations  []organizationPayload  `json:"organizations"`
		Member         memberPayload          `json:"member"`
		Roles          []rolePayload          `json:"roles"`
		Permissions    []string               `json:"permissions"`
		ResourceScopes []resourceScopePayload `json:"resource_scopes"`
	} `json:"data"`
}

type organizationPayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type memberPayload struct {
	ID      string   `json:"id"`
	UserID  string   `json:"user_id"`
	Status  string   `json:"status"`
	RoleIDs []string `json:"role_ids"`
}

type rolePayload struct {
	ID             string                 `json:"id"`
	Key            string                 `json:"key"`
	Permissions    []string               `json:"permissions"`
	ResourceScopes []resourceScopePayload `json:"resource_scopes"`
}

type resourceScopePayload struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	AccessLevel  string `json:"access_level"`
}

func closeTestDB(db *sql.DB) {
	_ = db.Close()
}

func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func seedBetterAuthUser(t *testing.T, db *sql.DB, id string, email string, name string) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO "user" ("id", "name", "email", "emailVerified", "createdAt", "updatedAt")
		VALUES ($1, $2, $3, false, $4, $5)
	`, id, name, email, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func signWebUserToken(t *testing.T, signer auth.HMACWebUserTokenSigner, purpose auth.WebUserTokenPurpose, userID string, email string, name string) string {
	t.Helper()

	token, err := signer.Sign(auth.WebUserClaims{
		UserID:        userID,
		Email:         email,
		Name:          name,
		SourceService: auth.InternalSourceServiceWeb,
		Purpose:       purpose,
		ExpiresAt:     time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("sign web user token: %v", err)
	}
	return token
}

func postBootstrap(t *testing.T, server http.Handler, signer auth.HMACWebUserTokenSigner, userID string, email string) *httptest.ResponseRecorder {
	t.Helper()

	token := signWebUserToken(t, signer, auth.WebUserTokenPurposeBootstrap, userID, email, "")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/bootstrap", bytes.NewBufferString(`{
		"organization_name": "Acme Inc",
		"organization_slug": "acme"
	}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)
	return recorder
}

func decodeJSON(t *testing.T, recorder *httptest.ResponseRecorder, value any) {
	t.Helper()

	if err := json.Unmarshal(recorder.Body.Bytes(), value); err != nil {
		t.Fatalf("decode response: %v body=%s", err, recorder.Body.String())
	}
}

func signInternalToken(t *testing.T, signer auth.HMACInternalTokenSigner, claims auth.InternalClaims) string {
	t.Helper()

	token, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("sign internal token: %v", err)
	}
	return token
}

func assertPermission(t *testing.T, permissions []string, expected string) {
	t.Helper()

	for _, permission := range permissions {
		if permission == expected {
			return
		}
	}
	t.Fatalf("permission %q not found in %#v", expected, permissions)
}

func assertScope(t *testing.T, scopes []resourceScopePayload, resourceType string, resourceID string, accessLevel string) {
	t.Helper()

	for _, scope := range scopes {
		if scope.ResourceType == resourceType && scope.ResourceID == resourceID && scope.AccessLevel == accessLevel {
			return
		}
	}
	t.Fatalf("scope %s/%s/%s not found in %#v", resourceType, resourceID, accessLevel, scopes)
}
