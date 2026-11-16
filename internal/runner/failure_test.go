package runner_test

import (
	"errors"
	"strings"
	"testing"

	"sdlc-coordinator/internal/failure"
	"sdlc-coordinator/internal/runner"
)

func TestFailureInfoOnlyRecognizesTypedProviderFailure(t *testing.T) {
	cause := errors.New("provider request failed")
	typed := &runner.ProviderFailure{
		Info: failure.Info{
			Version: failure.VersionV1,
			Stage:   failure.StageProvider,
			Source:  failure.SourceProvider,
			Code:    failure.CodeProviderRateLimited,
			Summary: "rate limited",
		},
		Cause: cause,
	}
	if !errors.Is(typed, cause) {
		t.Fatal("ProviderFailure must unwrap its cause")
	}
	info := runner.FailureInfo(typed)
	if info == nil || info.Code != failure.CodeProviderRateLimited {
		t.Fatalf("unexpected typed failure info: %#v", info)
	}
	if got := failure.Classify(info); got != failure.ClassificationTransient {
		t.Fatalf("Classify() = %q, want transient", got)
	}
	if runner.FailureInfo(errors.New("HTTP 429 Too Many Requests")) != nil {
		t.Fatal("ordinary errors must not be promoted to typed failure evidence")
	}
}

func TestProviderCodeInfoMapsProviderAliases(t *testing.T) {
	tests := map[string]string{
		"provider_unavailable":       failure.CodeProviderUnavailable,
		"service_unavailable":        failure.CodeProviderUnavailable,
		"provider_overloaded":        failure.CodeProviderOverloaded,
		"overloaded_error":           failure.CodeProviderOverloaded,
		"provider_rate_limited":      failure.CodeProviderRateLimited,
		"rate_limit_error":           failure.CodeProviderRateLimited,
		"rate_limited":               failure.CodeProviderRateLimited,
		"connection_failed":          failure.CodeConnectionFailed,
		"api_connection_error":       failure.CodeConnectionFailed,
		"connection_error":           failure.CodeConnectionFailed,
		"invalid_credentials":        failure.CodeInvalidCredentials,
		"authentication_error":       failure.CodeInvalidCredentials,
		"permission_denied":          failure.CodePermissionDenied,
		"permission_error":           failure.CodePermissionDenied,
		"insufficient_quota":         failure.CodeInsufficientQuota,
		"billing_hard_limit_reached": failure.CodeInsufficientQuota,
		"invalid_request":            failure.CodeInvalidRequest,
		"invalid_request_error":      failure.CodeInvalidRequest,
		"unsupported_schema":         failure.CodeUnsupportedSchema,
		"schema_rejected":            failure.CodeUnsupportedSchema,
	}
	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			info, ok := runner.ProviderCodeInfo(" " + strings.ToUpper(raw) + " ")
			if !ok {
				t.Fatalf("ProviderCodeInfo(%q) was not recognized", raw)
			}
			if info.Code != want || info.Source != failure.SourceProvider || info.Stage != failure.StageProvider || info.Version != failure.VersionV1 {
				t.Fatalf("ProviderCodeInfo(%q) = %#v, want code %q from the provider stage", raw, info, want)
			}
		})
	}
}

func TestProviderCodeInfoLeavesUnmappedCodesUnknown(t *testing.T) {
	for _, raw := range []string{
		"api_error", "server_error", "not_found_error", "rate_limit_exceeded",
		"invalid_api_key", "model_not_found", "context_length_exceeded", "request_too_large",
		"", "   ",
	} {
		if info, ok := runner.ProviderCodeInfo(raw); ok {
			t.Fatalf("ProviderCodeInfo(%q) = %#v, want no classification", raw, info)
		}
	}
}
