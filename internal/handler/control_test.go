package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/noxaaa/prism-oss/internal/auth"
	"github.com/noxaaa/prism-oss/internal/domain"
)

func TestControlServerHealthz(t *testing.T) {
	server := NewControlServer(ControlServerOptions{
		TokenVerifier: auth.HMACInternalTokenSigner{Secret: []byte("test-secret")},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
}

func TestControlServerRejectsMissingInternalToken(t *testing.T) {
	server := NewControlServer(ControlServerOptions{
		TokenVerifier: auth.HMACInternalTokenSigner{Secret: []byte("test-secret")},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/internal/v1/organizations/current", nil)

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
}

func TestControlServerAcceptsValidInternalToken(t *testing.T) {
	signer := auth.HMACInternalTokenSigner{Secret: []byte("test-secret")}
	token, err := signer.Sign(auth.InternalClaims{
		UserID:         "user_1",
		OrganizationID: "org_1",
		MemberID:       "member_1",
		Roles:          []string{"owner"},
		Permissions:    []string{string(domain.PermissionOrganizationRead)},
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	server := NewControlServer(ControlServerOptions{TokenVerifier: signer})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/internal/v1/organizations/current", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}
