package failure

// Info is the bounded, trusted description of an execution failure.
type Info struct {
	Version int    `json:"version"`
	Stage   string `json:"stage,omitempty"`
	Code    string `json:"code,omitempty"`
	Source  string `json:"source,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type Classification string

const (
	ClassificationTransient    Classification = "transient"
	ClassificationPrerequisite Classification = "prerequisite"
	ClassificationFatal        Classification = "fatal"
	ClassificationUnknown      Classification = "unknown"
)

const VersionV1 = 1

const (
	SourceProvider    = "provider"
	SourceRunner      = "runner"
	SourceCoordinator = "coordinator"
)

const (
	StageProvider       = "provider"
	StageInitialization = "initialization"
	StageResult         = "result"
	StageCoordinator    = "coordinator"
)

const (
	CodeProviderUnavailable  = "provider_unavailable"
	CodeProviderOverloaded   = "provider_overloaded"
	CodeProviderRateLimited  = "provider_rate_limited"
	CodeConnectionFailed     = "connection_failed"
	CodeInvalidCredentials   = "invalid_credentials"
	CodeMissingConfiguration = "missing_configuration"
	CodeCLIMissing           = "cli_missing"
	CodeInsufficientQuota    = "insufficient_quota"
	CodePermissionDenied     = "permission_denied"
	CodeInvalidRequest       = "invalid_request"
	CodeUnsupportedSchema    = "unsupported_schema"
	CodeProviderFailed       = "provider_failed"
)
