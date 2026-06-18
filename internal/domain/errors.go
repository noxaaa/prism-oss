package domain

type ErrorCode string

const (
	ErrValidationFailed ErrorCode = "VALIDATION_FAILED"
	ErrRulePortConflict ErrorCode = "RULE_PORT_CONFLICT"
	ErrRuleDuplicateSNI ErrorCode = "RULE_DUPLICATE_SNI"
	ErrUnauthenticated  ErrorCode = "UNAUTHENTICATED"
	ErrForbidden        ErrorCode = "FORBIDDEN"
	ErrNotFound         ErrorCode = "NOT_FOUND"
	ErrConflict         ErrorCode = "CONFLICT"
)

type DomainError struct {
	Code    ErrorCode
	Message string
}

func (err *DomainError) Error() string {
	if err == nil {
		return ""
	}
	return string(err.Code) + ": " + err.Message
}

func newDomainError(code ErrorCode, message string) *DomainError {
	return &DomainError{Code: code, Message: message}
}
