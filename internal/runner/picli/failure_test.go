package picli

import (
	"errors"
	"strings"
	"testing"

	"sdlc-coordinator/internal/failure"
	"sdlc-coordinator/internal/runner"
)

func TestClassifyProviderErrorStripsHTTPStatusPrefix(t *testing.T) {
	code, message := classifyProviderError(`429: {"error":{"code":"rate_limit_exceeded","message":"slow down"}}`)
	if code != failure.CodeProviderRateLimited || message != "slow down" {
		t.Fatalf("classifyProviderError() = (%q, %q)", code, message)
	}
}

func TestClassifyProviderErrorReadsFlatEnvelope(t *testing.T) {
	code, message := classifyProviderError(`429: {"code":"rate_limit_exceeded","message":"slow down"}`)
	if code != failure.CodeProviderRateLimited || message != "slow down" {
		t.Fatalf("classifyProviderError() = (%q, %q)", code, message)
	}
}

func TestClassifyProviderErrorMapsKnownCodes(t *testing.T) {
	tests := map[string]string{
		"rate_limit_exceeded":     failure.CodeProviderRateLimited,
		"insufficient_quota":      failure.CodeInsufficientQuota,
		"invalid_api_key":         failure.CodeInvalidCredentials,
		"overloaded_error":        failure.CodeProviderOverloaded,
		"context_length_exceeded": failure.CodeInvalidRequest,
		"invented_code":           failure.CodeProviderFailed,
	}
	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			code, _ := classifyProviderError(`{"error":{"code":"` + raw + `","message":"failed"}}`)
			if code != want {
				t.Fatalf("classifyProviderError(%q) code = %q, want %q", raw, code, want)
			}
		})
	}
}

func TestClassifyProviderErrorKeepsPlainMessage(t *testing.T) {
	code, message := classifyProviderError("connection reset by peer")
	if code != failure.CodeProviderFailed || message != "connection reset by peer" {
		t.Fatalf("classifyProviderError() = (%q, %q)", code, message)
	}
}

func TestExecutionFailureIsTyped(t *testing.T) {
	err := executionFailure(failure.CodeProviderRateLimited, "slow down", errors.New("exit status 1"))
	info := runner.FailureInfo(err)
	if info == nil {
		t.Fatalf("execution failure was not typed: %v", err)
	}
	if info.Code != failure.CodeProviderRateLimited || info.Stage != failure.StageProvider || info.Source != failure.SourceProvider {
		t.Fatalf("unexpected failure info: %#v", info)
	}
	if got := failure.Classify(info); got != failure.ClassificationTransient {
		t.Fatalf("Classify() = %q, want transient", got)
	}
	if !strings.Contains(info.Summary, "slow down") {
		t.Fatalf("summary lost the provider detail: %#v", info)
	}
}

func TestSetupFailureIsRunnerSide(t *testing.T) {
	err := setupFailure(failure.CodeMissingConfiguration, errors.New("missing"))
	info := runner.FailureInfo(err)
	if info == nil {
		t.Fatalf("setup failure was not typed: %v", err)
	}
	if info.Stage != failure.StageInitialization || info.Source != failure.SourceRunner || info.Code != failure.CodeMissingConfiguration {
		t.Fatalf("unexpected failure info: %#v", info)
	}
}

func TestClassifyProviderErrorMapsStatusOnlyEvidence(t *testing.T) {
	tests := map[string]string{
		"429: rate limited by the gateway": failure.CodeProviderRateLimited,
		"503: upstream unavailable":        failure.CodeProviderUnavailable,
		"401: bad key":                     failure.CodeInvalidCredentials,
		"403: region blocked":              failure.CodePermissionDenied,
		"422: schema rejected":             failure.CodeInvalidRequest,
		"504: gateway timeout":             failure.CodeConnectionFailed,
	}
	for raw, want := range tests {
		code, _ := classifyProviderError(raw)
		if code != want {
			t.Errorf("classifyProviderError(%q) code = %q, want %q", raw, code, want)
		}
	}
}

func TestClassifyProviderErrorMapsPIConnectionPrefix(t *testing.T) {
	code, message := classifyProviderError("Connection error.: connect ECONNREFUSED 127.0.0.1:8080")
	if code != failure.CodeConnectionFailed {
		t.Fatalf("classifyProviderError() code = %q, want %q", code, failure.CodeConnectionFailed)
	}
	if message != "Connection error.: connect ECONNREFUSED 127.0.0.1:8080" {
		t.Fatalf("classifyProviderError() message = %q", message)
	}
}
