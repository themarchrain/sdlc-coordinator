package failure

import "strings"

var classByCode = map[string]Classification{
	CodeProviderUnavailable: ClassificationTransient,
	CodeProviderOverloaded:  ClassificationTransient,
	CodeProviderRateLimited: ClassificationTransient,
	CodeConnectionFailed:    ClassificationTransient,

	CodeInvalidCredentials:   ClassificationPrerequisite,
	CodeMissingConfiguration: ClassificationPrerequisite,
	CodeCLIMissing:           ClassificationPrerequisite,
	CodeInsufficientQuota:    ClassificationPrerequisite,
	CodePermissionDenied:     ClassificationPrerequisite,

	CodeInvalidRequest:    ClassificationFatal,
	CodeUnsupportedSchema: ClassificationFatal,
}

var trustedSources = map[string]struct{}{
	SourceProvider:    {},
	SourceRunner:      {},
	SourceCoordinator: {},
}

// runnerSetupCodes only mean the runner observed a missing local prerequisite.
// The same word echoed by a provider or an old coordinator record must stay
// unknown, otherwise a provider error can unlock the prerequisite gate.
var runnerSetupCodes = map[string]struct{}{
	CodeCLIMissing:           {},
	CodeMissingConfiguration: {},
}

// Classify only trusts a supported version, source and explicit code. Free-form
// summaries are never used as classification evidence.
func Classify(info *Info) Classification {
	if info == nil || info.Version != VersionV1 {
		return ClassificationUnknown
	}
	if _, ok := trustedSources[info.Source]; !ok {
		return ClassificationUnknown
	}
	code := strings.TrimSpace(info.Code)
	if _, runnerOnly := runnerSetupCodes[code]; runnerOnly && info.Source != SourceRunner {
		return ClassificationUnknown
	}
	class, ok := classByCode[code]
	if !ok {
		return ClassificationUnknown
	}
	return class
}

// FromLegacy projects old execution error code/message fields conservatively.
// Only an exact supported code can classify; messages are retained verbatim.
func FromLegacy(code, message string) *Info {
	return &Info{
		Version: VersionV1,
		Stage:   StageResult,
		Code:    strings.TrimSpace(code),
		Source:  SourceCoordinator,
		Summary: strings.TrimSpace(message),
	}
}
