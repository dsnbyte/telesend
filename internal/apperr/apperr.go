// Package apperr defines the application error type and secret redaction.
package apperr

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// Code classifies an application error.
type Code string

const (
	Configuration    Code = "configuration"
	Conflict         Code = "conflict"
	FilesystemPolicy Code = "filesystem_policy"
	Internal         Code = "internal"
	NotFound         Code = "not_found"
	Telegram         Code = "telegram"
	Unauthorized     Code = "unauthorized"
	Validation       Code = "validation"
)

// Error is an error with a stable, safe-to-expose code.
type Error struct {
	Code    Code
	Message string
	Details map[string]any
}

func (e *Error) Error() string { return e.Message }

// New creates an Error.
func New(code Code, message string) *Error { return &Error{Code: code, Message: message} }

// Newf creates an Error with a formatted message.
func Newf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// HTTPStatus maps the code to a REST status.
func (e *Error) HTTPStatus() int {
	switch e.Code {
	case Unauthorized:
		return http.StatusUnauthorized
	case Validation:
		return http.StatusBadRequest
	case NotFound:
		return http.StatusNotFound
	case Conflict:
		return http.StatusConflict
	case FilesystemPolicy:
		return http.StatusForbidden
	case Telegram:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

var (
	telegramToken    = regexp.MustCompile(`\b\d{6,}:[A-Za-z0-9_-]{20,}\b`)
	telegramTokenURL = regexp.MustCompile(`/bot[^/\s]+/`)
)

// Redact removes the given secrets and any Telegram bot tokens from text.
func Redact(text string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	text = telegramTokenURL.ReplaceAllString(text, "/bot[REDACTED]/")
	return telegramToken.ReplaceAllString(text, "[REDACTED]")
}

// From converts any error into a redacted *Error. Non-application errors
// become Internal errors.
func From(err error, secrets ...string) *Error {
	var appErr *Error
	if errors.As(err, &appErr) {
		return &Error{Code: appErr.Code, Message: Redact(appErr.Message, secrets...), Details: appErr.Details}
	}
	return &Error{Code: Internal, Message: Redact(err.Error(), secrets...)}
}
