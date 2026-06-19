package auth

import (
	"testing"
	"time"
)

func TestWebUserTokenRoundTrip(t *testing.T) {
	signer := HMACWebUserTokenSigner{Secret: []byte("test-secret")}
	claims := WebUserClaims{
		UserID:        "user_1",
		Email:         "owner@example.com",
		Name:          "Owner",
		SourceService: InternalSourceServiceWeb,
		Purpose:       WebUserTokenPurposeBootstrap,
		ExpiresAt:     time.Now().Add(time.Minute),
	}

	token, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("sign web user token: %v", err)
	}
	parsed, err := signer.Verify(token, WebUserTokenPurposeBootstrap)
	if err != nil {
		t.Fatalf("verify web user token: %v", err)
	}

	if parsed.UserID != claims.UserID || parsed.Email != claims.Email || parsed.Name != claims.Name {
		t.Fatalf("parsed claims mismatch: %#v", parsed)
	}
	if parsed.SourceService != InternalSourceServiceWeb {
		t.Fatalf("expected source service %q, got %q", InternalSourceServiceWeb, parsed.SourceService)
	}
}

func TestWebUserTokenRejectsWrongPurpose(t *testing.T) {
	signer := HMACWebUserTokenSigner{Secret: []byte("test-secret")}
	token, err := signer.Sign(WebUserClaims{
		UserID:        "user_1",
		Email:         "owner@example.com",
		SourceService: InternalSourceServiceWeb,
		Purpose:       WebUserTokenPurposeBootstrap,
		ExpiresAt:     time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("sign web user token: %v", err)
	}

	if _, err := signer.Verify(token, WebUserTokenPurposeSession); err == nil {
		t.Fatalf("expected wrong-purpose web user token to be rejected")
	}
}

func TestWebUserTokenCannotSatisfyOrganizationIdentity(t *testing.T) {
	webSigner := HMACWebUserTokenSigner{Secret: []byte("test-secret")}
	identityVerifier := HMACInternalTokenSigner{Secret: []byte("test-secret")}
	token, err := webSigner.Sign(WebUserClaims{
		UserID:        "user_1",
		Email:         "owner@example.com",
		SourceService: InternalSourceServiceWeb,
		Purpose:       WebUserTokenPurposeSession,
		ExpiresAt:     time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("sign web user token: %v", err)
	}

	if _, err := identityVerifier.Verify(token); err == nil {
		t.Fatalf("expected web user token to be rejected as org-scoped identity")
	}
}
