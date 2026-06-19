package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrInvalidInternalToken = errors.New("invalid internal token")
var ErrExpiredInternalToken = errors.New("expired internal token")

const InternalSourceServiceWeb = "web"

type WebUserTokenPurpose string

const (
	WebUserTokenPurposeBootstrap WebUserTokenPurpose = "bootstrap"
	WebUserTokenPurposeSession   WebUserTokenPurpose = "session"
)

type InternalClaims struct {
	UserID         string               `json:"user_id"`
	OrganizationID string               `json:"organization_id"`
	MemberID       string               `json:"member_id"`
	SourceService  string               `json:"source_service"`
	Roles          []string             `json:"roles"`
	Permissions    []string             `json:"permissions"`
	ResourceScopes []ResourceScopeClaim `json:"resource_scopes,omitempty"`
	ExpiresAt      time.Time            `json:"expires_at"`
}

type ResourceScopeClaim struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	AccessLevel  string `json:"access_level"`
}

type WebUserClaims struct {
	UserID        string              `json:"user_id"`
	Email         string              `json:"email"`
	Name          string              `json:"name"`
	SourceService string              `json:"source_service"`
	Purpose       WebUserTokenPurpose `json:"purpose"`
	ExpiresAt     time.Time           `json:"expires_at"`
}

type InternalTokenVerifier interface {
	Verify(token string) (InternalClaims, error)
}

type WebUserTokenVerifier interface {
	Verify(token string, expectedPurpose WebUserTokenPurpose) (WebUserClaims, error)
}

type HMACInternalTokenSigner struct {
	Secret                []byte
	ExpectedSourceService string
}

type HMACWebUserTokenSigner struct {
	Secret                []byte
	ExpectedSourceService string
}

func (signer HMACInternalTokenSigner) Sign(claims InternalClaims) (string, error) {
	if claims.SourceService == "" {
		claims.SourceService = signer.expectedSourceService()
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signature := signer.sign(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (signer HMACWebUserTokenSigner) Sign(claims WebUserClaims) (string, error) {
	if claims.SourceService == "" {
		claims.SourceService = signer.expectedSourceService()
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signature := signer.sign(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (signer HMACInternalTokenSigner) Verify(token string) (InternalClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return InternalClaims{}, ErrInvalidInternalToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return InternalClaims{}, ErrInvalidInternalToken
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return InternalClaims{}, ErrInvalidInternalToken
	}
	if !hmac.Equal(signature, signer.sign(payload)) {
		return InternalClaims{}, ErrInvalidInternalToken
	}

	var claims InternalClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return InternalClaims{}, ErrInvalidInternalToken
	}
	if claims.SourceService != signer.expectedSourceService() {
		return InternalClaims{}, ErrInvalidInternalToken
	}
	if !claims.hasRequiredIdentity() {
		return InternalClaims{}, ErrInvalidInternalToken
	}
	if time.Now().After(claims.ExpiresAt) {
		return InternalClaims{}, ErrExpiredInternalToken
	}
	return claims, nil
}

func (signer HMACWebUserTokenSigner) Verify(token string, expectedPurpose WebUserTokenPurpose) (WebUserClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return WebUserClaims{}, ErrInvalidInternalToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return WebUserClaims{}, ErrInvalidInternalToken
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return WebUserClaims{}, ErrInvalidInternalToken
	}
	if !hmac.Equal(signature, signer.sign(payload)) {
		return WebUserClaims{}, ErrInvalidInternalToken
	}

	var claims WebUserClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return WebUserClaims{}, ErrInvalidInternalToken
	}
	if claims.SourceService != signer.expectedSourceService() || claims.Purpose != expectedPurpose {
		return WebUserClaims{}, ErrInvalidInternalToken
	}
	if !claims.hasRequiredIdentity() {
		return WebUserClaims{}, ErrInvalidInternalToken
	}
	if time.Now().After(claims.ExpiresAt) {
		return WebUserClaims{}, ErrExpiredInternalToken
	}
	return claims, nil
}

func (claims InternalClaims) hasRequiredIdentity() bool {
	if strings.TrimSpace(claims.UserID) == "" ||
		strings.TrimSpace(claims.OrganizationID) == "" ||
		strings.TrimSpace(claims.MemberID) == "" ||
		claims.ExpiresAt.IsZero() ||
		!hasNonBlankValues(claims.Roles) ||
		!hasNonBlankValues(claims.Permissions) {
		return false
	}
	return true
}

func (claims WebUserClaims) hasRequiredIdentity() bool {
	if strings.TrimSpace(claims.UserID) == "" ||
		strings.TrimSpace(claims.Email) == "" ||
		claims.Purpose == "" ||
		claims.ExpiresAt.IsZero() {
		return false
	}
	return true
}

func hasNonBlankValues(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return false
		}
	}
	return true
}

func (signer HMACInternalTokenSigner) expectedSourceService() string {
	if signer.ExpectedSourceService != "" {
		return signer.ExpectedSourceService
	}
	return InternalSourceServiceWeb
}

func (signer HMACWebUserTokenSigner) expectedSourceService() string {
	if signer.ExpectedSourceService != "" {
		return signer.ExpectedSourceService
	}
	return InternalSourceServiceWeb
}

func (signer HMACInternalTokenSigner) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, signer.Secret)
	mac.Write(payload)
	return mac.Sum(nil)
}

func (signer HMACWebUserTokenSigner) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, signer.Secret)
	mac.Write(payload)
	return mac.Sum(nil)
}
