package runner_test

import (
	"errors"
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
