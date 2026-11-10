package failure

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		info *Info
		want Classification
	}{
		{
			name: "provider unavailable is transient",
			info: &Info{Version: VersionV1, Stage: StageProvider, Source: SourceProvider, Code: CodeProviderUnavailable},
			want: ClassificationTransient,
		},
		{
			name: "provider overloaded is transient",
			info: &Info{Version: VersionV1, Stage: StageProvider, Source: SourceProvider, Code: CodeProviderOverloaded},
			want: ClassificationTransient,
		},
		{
			name: "provider rate limited is transient",
			info: &Info{Version: VersionV1, Stage: StageProvider, Source: SourceProvider, Code: CodeProviderRateLimited},
			want: ClassificationTransient,
		},
		{
			name: "connection failed is transient",
			info: &Info{Version: VersionV1, Stage: StageProvider, Source: SourceProvider, Code: CodeConnectionFailed},
			want: ClassificationTransient,
		},
		{
			name: "invalid credentials is prerequisite",
			info: &Info{Version: VersionV1, Stage: StageProvider, Source: SourceProvider, Code: CodeInvalidCredentials},
			want: ClassificationPrerequisite,
		},
		{
			name: "missing configuration is prerequisite",
			info: &Info{Version: VersionV1, Stage: StageInitialization, Source: SourceRunner, Code: CodeMissingConfiguration},
			want: ClassificationPrerequisite,
		},
		{
			name: "cli missing is prerequisite",
			info: &Info{Version: VersionV1, Stage: StageInitialization, Source: SourceRunner, Code: CodeCLIMissing},
			want: ClassificationPrerequisite,
		},
		{
			name: "insufficient quota is not transient",
			info: &Info{Version: VersionV1, Stage: StageProvider, Source: SourceProvider, Code: CodeInsufficientQuota},
			want: ClassificationPrerequisite,
		},
		{
			name: "invalid request is fatal",
			info: &Info{Version: VersionV1, Stage: StageProvider, Source: SourceProvider, Code: CodeInvalidRequest},
			want: ClassificationFatal,
		},
		{
			name: "unsupported schema is fatal",
			info: &Info{Version: VersionV1, Stage: StageProvider, Source: SourceProvider, Code: CodeUnsupportedSchema},
			want: ClassificationFatal,
		},
		{
			name: "bare provider failed is unknown",
			info: &Info{Version: VersionV1, Stage: StageResult, Source: SourceRunner, Code: CodeProviderFailed},
			want: ClassificationUnknown,
		},
		{
			name: "bare timeout is unknown",
			info: &Info{Version: VersionV1, Stage: StageResult, Source: SourceRunner, Code: "timeout"},
			want: ClassificationUnknown,
		},
		{
			name: "unknown version is unknown",
			info: &Info{Version: 99, Stage: StageProvider, Source: SourceProvider, Code: CodeProviderUnavailable},
			want: ClassificationUnknown,
		},
		{
			name: "unknown source is unknown",
			info: &Info{Version: VersionV1, Stage: StageProvider, Source: "agent", Code: CodeProviderUnavailable},
			want: ClassificationUnknown,
		},
		{
			name: "unknown code is unknown",
			info: &Info{Version: VersionV1, Stage: StageProvider, Source: SourceProvider, Code: "something_new"},
			want: ClassificationUnknown,
		},
		{
			name: "agent text is not classification evidence",
			info: &Info{Version: VersionV1, Stage: StageProvider, Source: SourceProvider, Summary: "temporary outage, retry later"},
			want: ClassificationUnknown,
		},
		{
			name: "nil is unknown",
			info: nil,
			want: ClassificationUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.info); got != tt.want {
				t.Fatalf("Classify() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLegacyFailureIsNotTransientBySubstring(t *testing.T) {
	messages := []string{
		"HTTP 429 Too Many Requests",
		"503 Service Unavailable",
		"temporary failure, please retry",
		"provider overloaded, retry later",
		"connection failed",
	}
	for _, message := range messages {
		info := FromLegacy(CodeProviderFailed, message)
		if info == nil {
			t.Fatalf("FromLegacy(%q) returned nil", message)
		}
		if got := Classify(info); got != ClassificationUnknown {
			t.Fatalf("Classify(FromLegacy(%q)) = %q, want unknown", message, got)
		}
	}
}

func TestLegacyStructuredCodeIsPreserved(t *testing.T) {
	info := FromLegacy(CodeInvalidCredentials, "credential expired")
	if info == nil {
		t.Fatal("FromLegacy returned nil")
	}
	if info.Code != CodeInvalidCredentials || info.Stage != StageResult || info.Source != SourceCoordinator {
		t.Fatalf("unexpected legacy projection: %+v", info)
	}
	if got := Classify(info); got != ClassificationPrerequisite {
		t.Fatalf("Classify() = %q, want prerequisite", got)
	}
}
