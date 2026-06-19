package service

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/noxaaa/prism-oss/pkg/core/domain"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
	"github.com/noxaaa/prism-oss/pkg/edition"
)

var ErrForbidden = errors.New("forbidden")
var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrInvalidInput = errors.New("invalid input")
var ErrQuotaExceeded = errors.New("quota exceeded")

const (
	MemberStatusActive   = "ACTIVE"
	MemberStatusDisabled = "DISABLED"

	agentPendingCredentialTTL = 5 * time.Minute
)

type ControlService struct {
	store                   repo.UnitOfWork
	now                     func() time.Time
	newID                   func() string
	appName                 string
	controlPlaneURL         string
	agentReleaseVersion     string
	agentTokenSigningSecret []byte
	edition                 edition.Provider
	authorizer              Authorizer
	sessionBackend          SessionBackend
	rbacBackend             RBACBackend
}

func NewControlService(store repo.UnitOfWork) *ControlService {
	return NewControlServiceWithOptions(store, ControlServiceOptions{})
}

type ControlServiceOptions struct {
	AppName                 string
	ControlPlaneURL         string
	AgentReleaseVersion     string
	AgentTokenSigningSecret []byte
	Edition                 edition.Provider
	Authorizer              Authorizer
	SessionBackend          SessionBackend
	RBACBackend             RBACBackend
}

func NewControlServiceWithOptions(store repo.UnitOfWork, options ControlServiceOptions) *ControlService {
	provider := options.Edition
	if provider == nil {
		provider = defaultControlEdition()
	}
	service := &ControlService{
		store:                   store,
		now:                     func() time.Time { return time.Now().UTC() },
		newID:                   func() string { return uuid.NewString() },
		appName:                 options.AppName,
		controlPlaneURL:         options.ControlPlaneURL,
		agentReleaseVersion:     options.AgentReleaseVersion,
		agentTokenSigningSecret: append([]byte(nil), options.AgentTokenSigningSecret...),
		edition:                 provider,
		authorizer:              options.Authorizer,
		sessionBackend:          options.SessionBackend,
		rbacBackend:             options.RBACBackend,
	}
	if service.authorizer == nil {
		service.authorizer = defaultControlAuthorizer()
	}
	return service
}

func (service *ControlService) timestamp() string {
	return service.now().UTC().Format(time.RFC3339Nano)
}

func mapServiceError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, repo.ErrNotFound) {
		return ErrNotFound
	}
	if errors.Is(err, repo.ErrConflict) {
		return ErrConflict
	}
	return err
}

func ErrorPayloadForError(err error) ErrorPayload {
	var controlErr *controlServiceError
	if errors.As(err, &controlErr) {
		return ErrorPayload{Code: controlErr.Code, Message: controlErr.Message, Details: copyErrorDetails(controlErr.Details)}
	}
	err = mapServiceError(err)
	var domainErr *domain.DomainError
	if errors.As(err, &domainErr) {
		return ErrorPayload{Code: string(domainErr.Code), Message: domainErr.Message}
	}
	switch {
	case errors.Is(err, ErrForbidden):
		return ErrorPayload{Code: "FORBIDDEN", Message: "You do not have permission to perform this action."}
	case errors.Is(err, ErrNotFound):
		return ErrorPayload{Code: "NOT_FOUND", Message: "The requested resource was not found."}
	case errors.Is(err, ErrConflict):
		return ErrorPayload{Code: "CONFLICT", Message: "The request conflicts with the current environment state."}
	case errors.Is(err, ErrQuotaExceeded):
		return ErrorPayload{Code: "QUOTA_EXCEEDED", Message: "The request exceeds the configured quota."}
	case errors.Is(err, ErrInvalidInput):
		return ErrorPayload{Code: "VALIDATION_FAILED", Message: "The request payload is invalid."}
	default:
		return ErrorPayload{Code: "INTERNAL_ERROR", Message: "An internal error occurred."}
	}
}
