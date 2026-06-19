package service

import (
	"errors"
)

type controlServiceError struct {
	Code    string
	Message string
	Details map[string]any
	Cause   error
}

func (err *controlServiceError) Error() string {
	if err == nil {
		return ""
	}
	if err.Message == "" {
		return err.Code
	}
	return err.Code + ": " + err.Message
}

func (err *controlServiceError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func validationError(message string, details map[string]any) error {
	return &controlServiceError{
		Code:    "VALIDATION_FAILED",
		Message: message,
		Details: copyErrorDetails(details),
		Cause:   ErrInvalidInput,
	}
}

func validationFieldError(field string, message string, details map[string]any) error {
	if details == nil {
		details = map[string]any{}
	}
	details["field"] = field
	return validationError(message, details)
}

func copyErrorDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return nil
	}
	copied := make(map[string]any, len(details))
	for key, value := range details {
		copied[key] = value
	}
	return copied
}

func newRuleImportIssue(code string, scope string, index int, details map[string]any) RuleImportIssue {
	copiedDetails := copyErrorDetails(details)
	copiedIndex := index
	return RuleImportIssue{
		Code:    code,
		Scope:   scope,
		Index:   &copiedIndex,
		Details: copiedDetails,
	}
}

func importIssueFromError(scope string, index int, err error) RuleImportIssue {
	var issueErr *ruleImportIssueError
	if errors.As(err, &issueErr) {
		return newRuleImportIssue(issueErr.Code, scope, index, issueErr.Details)
	}
	payload := ErrorPayloadForError(err)
	details := detailsWithReason(nil, payload)
	return newRuleImportIssue(payload.Code, scope, index, details)
}

func importIssueWithReason(code string, scope string, index int, err error, details map[string]any) RuleImportIssue {
	payload := ErrorPayloadForError(err)
	return newRuleImportIssue(code, scope, index, detailsWithReason(details, payload))
}

func detailsWithReason(details map[string]any, payload ErrorPayload) map[string]any {
	next := copyErrorDetails(details)
	if next == nil {
		next = map[string]any{}
	}
	for key, value := range payload.Details {
		next[key] = value
	}
	if payload.Code != "" {
		next["reason_code"] = payload.Code
	}
	return next
}
