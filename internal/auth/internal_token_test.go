package auth

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestInternalTokenRoundTrip(t *testing.T) {
	signer := HMACInternalTokenSigner{Secret: []byte("test-secret")}
	claims := InternalClaims{
		UserID:         "user_1",
		OrganizationID: "org_1",
		MemberID:       "member_1",
		Roles:          []string{"owner"},
		Permissions:    []string{"nodes.manage"},
		ExpiresAt:      time.Now().Add(time.Minute),
	}

	token, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	parsed, err := signer.Verify(token)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}

	if parsed.UserID != claims.UserID || parsed.OrganizationID != claims.OrganizationID || parsed.MemberID != claims.MemberID {
		t.Fatalf("parsed claims mismatch: %#v", parsed)
	}
	if parsed.SourceService != InternalSourceServiceWeb {
		t.Fatalf("expected source service %q, got %q", InternalSourceServiceWeb, parsed.SourceService)
	}
	if len(parsed.Permissions) != 1 || parsed.Permissions[0] != "nodes.manage" {
		t.Fatalf("parsed permissions mismatch: %#v", parsed.Permissions)
	}
}

func TestInternalTokenRejectsExpiredClaims(t *testing.T) {
	signer := HMACInternalTokenSigner{Secret: []byte("test-secret")}
	token, err := signer.Sign(InternalClaims{
		UserID:         "user_1",
		OrganizationID: "org_1",
		MemberID:       "member_1",
		Roles:          []string{"owner"},
		Permissions:    []string{"nodes.manage"},
		ExpiresAt:      time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	if _, err := signer.Verify(token); err == nil {
		t.Fatalf("expected expired token rejection")
	}
}

func TestInternalTokenRejectsIncompleteIdentityClaims(t *testing.T) {
	signer := HMACInternalTokenSigner{Secret: []byte("test-secret")}
	validClaims := InternalClaims{
		UserID:         "user_1",
		OrganizationID: "org_1",
		MemberID:       "member_1",
		Roles:          []string{"owner"},
		Permissions:    []string{"nodes.manage"},
		ExpiresAt:      time.Now().Add(time.Minute),
	}

	tests := []struct {
		name   string
		mutate func(*InternalClaims)
	}{
		{
			name: "missing user",
			mutate: func(claims *InternalClaims) {
				claims.UserID = ""
			},
		},
		{
			name: "blank organization",
			mutate: func(claims *InternalClaims) {
				claims.OrganizationID = " "
			},
		},
		{
			name: "missing member",
			mutate: func(claims *InternalClaims) {
				claims.MemberID = ""
			},
		},
		{
			name: "missing roles",
			mutate: func(claims *InternalClaims) {
				claims.Roles = nil
			},
		},
		{
			name: "blank role",
			mutate: func(claims *InternalClaims) {
				claims.Roles = []string{"owner", ""}
			},
		},
		{
			name: "missing permissions",
			mutate: func(claims *InternalClaims) {
				claims.Permissions = nil
			},
		},
		{
			name: "blank permission",
			mutate: func(claims *InternalClaims) {
				claims.Permissions = []string{"nodes.manage", " "}
			},
		},
		{
			name: "missing expiry",
			mutate: func(claims *InternalClaims) {
				claims.ExpiresAt = time.Time{}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims := validClaims
			claims.Roles = append([]string(nil), validClaims.Roles...)
			claims.Permissions = append([]string(nil), validClaims.Permissions...)
			test.mutate(&claims)

			token, err := signer.Sign(claims)
			if err != nil {
				t.Fatalf("sign token: %v", err)
			}
			if _, err := signer.Verify(token); err == nil {
				t.Fatalf("expected incomplete identity claims to be rejected")
			}
		})
	}
}

func TestInternalTokenRejectsMissingSourceService(t *testing.T) {
	signer := HMACInternalTokenSigner{Secret: []byte("test-secret")}
	payload, err := json.Marshal(InternalClaims{
		UserID:         "user_1",
		OrganizationID: "org_1",
		MemberID:       "member_1",
		Roles:          []string{"owner"},
		Permissions:    []string{"nodes.manage"},
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	token := base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signer.sign(payload))

	if _, err := signer.Verify(token); err == nil {
		t.Fatalf("expected token without source service to be rejected")
	}
}

func TestInternalTokenRejectsUnexpectedSourceService(t *testing.T) {
	signer := HMACInternalTokenSigner{Secret: []byte("test-secret")}
	token, err := signer.Sign(InternalClaims{
		UserID:         "user_1",
		OrganizationID: "org_1",
		MemberID:       "member_1",
		SourceService:  "node-agent",
		Roles:          []string{"owner"},
		Permissions:    []string{"nodes.manage"},
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	if _, err := signer.Verify(token); err == nil {
		t.Fatalf("expected token from unexpected source service to be rejected")
	}
}
