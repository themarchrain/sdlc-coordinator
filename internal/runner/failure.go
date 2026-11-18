package runner

import (
	"errors"
	"strings"

	"sdlc-coordinator/internal/failure"
)

// ProviderFailure carries structured, trusted failure evidence while keeping
// the original execution error available for diagnostics.
type ProviderFailure struct {
	Info  failure.Info
	Cause error
}

func (e *ProviderFailure) Error() string {
	if e == nil {
		return ""
	}
	if summary := strings.TrimSpace(e.Info.Summary); summary != "" {
		return summary
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return "provider failure"
}

func (e *ProviderFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// FailureInfo returns structured evidence only for an explicitly typed
// ProviderFailure. Plain errors remain unknown to the coordinator.
func FailureInfo(err error) *failure.Info {
	var providerFailure *ProviderFailure
	if !errors.As(err, &providerFailure) || providerFailure == nil {
		return nil
	}
	info := providerFailure.Info
	return &info
}

// ProviderCodeInfo maps explicit provider error codes to the shared trusted
// vocabulary. Unlisted codes are intentionally left out.
func ProviderCodeInfo(rawCode string) (failure.Info, bool) {
	var code string
	switch strings.ToLower(strings.TrimSpace(rawCode)) {
	case "provider_unavailable", "service_unavailable":
		code = failure.CodeProviderUnavailable
	case "provider_overloaded", "overloaded_error":
		code = failure.CodeProviderOverloaded
	case "provider_rate_limited", "rate_limit_error", "rate_limited":
		code = failure.CodeProviderRateLimited
	case "connection_failed", "api_connection_error", "connection_error":
		code = failure.CodeConnectionFailed
	case "invalid_credentials", "authentication_error":
		code = failure.CodeInvalidCredentials
	case "permission_denied", "permission_error":
		code = failure.CodePermissionDenied
	case "insufficient_quota", "billing_hard_limit_reached":
		code = failure.CodeInsufficientQuota
	case "invalid_request", "invalid_request_error":
		code = failure.CodeInvalidRequest
	case "unsupported_schema", "schema_rejected":
		code = failure.CodeUnsupportedSchema
	default:
		return failure.Info{}, false
	}
	return failure.Info{
		Version: failure.VersionV1,
		Stage:   failure.StageProvider,
		Source:  failure.SourceProvider,
		Code:    code,
	}, true
}
