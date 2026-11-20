package picli

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"sdlc-coordinator/internal/failure"
	"sdlc-coordinator/internal/runner"
)

// httpStatusPrefix 是 pi 透传网关错误时加在前面的三位状态码前缀。
var httpStatusPrefix = regexp.MustCompile(`^\d{3}:\s*`)

// classifyProviderError 把 pi 报告的失败文本归一成共享失败词表的 code 与
// 可读消息:先剥状态码前缀,再按 error 对象或顶层信封解包,最后映射 code。
func classifyProviderError(raw string) (string, string) {
	trimmed := httpStatusPrefix.ReplaceAllString(strings.TrimSpace(raw), "")
	extracted := runner.ExtractProviderError(map[string]any{"message": trimmed})
	rawCode := strings.TrimSpace(extracted.Code)
	message := strings.TrimSpace(extracted.Message)
	if rawCode == "" {
		// ExtractProviderError 只解包 {"error":{...}};网关也常把 code 与
		// message 放在顶层,这里补一层。
		var flat struct {
			Code    string `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
		}
		if json.Unmarshal([]byte(trimmed), &flat) == nil {
			candidate := strings.TrimSpace(flat.Code)
			if candidate == "" {
				candidate = strings.TrimSpace(flat.Type)
			}
			if candidate != "" {
				rawCode = candidate
				message = strings.TrimSpace(flat.Message)
			}
		}
	}
	if message == "" {
		message = trimmed
	}
	return providerFailureCode(rawCode), message
}

// providerFailureCode 把网关与上游厂商的 code 别名映射到共享失败词表。
// 未识别的 code 归入 provider_failed,保持未知分类。
func providerFailureCode(rawCode string) string {
	switch strings.ToLower(strings.TrimSpace(rawCode)) {
	case "provider_unavailable", "service_unavailable":
		return failure.CodeProviderUnavailable
	case "provider_overloaded", "overloaded_error", "overloaded":
		return failure.CodeProviderOverloaded
	case "provider_rate_limited", "rate_limit_error", "rate_limited", "rate_limit_exceeded", "too_many_requests":
		return failure.CodeProviderRateLimited
	case "connection_failed", "api_connection_error", "connection_error":
		return failure.CodeConnectionFailed
	case "invalid_credentials", "authentication_error", "invalid_api_key", "unauthorized":
		return failure.CodeInvalidCredentials
	case "permission_denied", "permission_error":
		return failure.CodePermissionDenied
	case "insufficient_quota", "quota_exceeded", "billing_hard_limit_reached":
		return failure.CodeInsufficientQuota
	case "invalid_request", "invalid_request_error", "context_length_exceeded":
		return failure.CodeInvalidRequest
	case "unsupported_schema", "schema_rejected":
		return failure.CodeUnsupportedSchema
	default:
		return failure.CodeProviderFailed
	}
}

func executionFailure(code, detail string, cause error) error {
	info, known := runner.ProviderCodeInfo(code)
	if !known {
		info = failure.Info{
			Version: failure.VersionV1,
			Stage:   failure.StageProvider,
			Source:  failure.SourceProvider,
			Code:    strings.TrimSpace(code),
		}
	}
	info.Summary = strings.TrimSpace(detail)
	if code := strings.TrimSpace(code); code != "" && !strings.Contains(info.Summary, code) {
		info.Summary = code + ": " + info.Summary
	}
	if cause == nil {
		cause = errors.New(info.Summary)
	}
	return &runner.ProviderFailure{Info: info, Cause: cause}
}

func setupFailure(code string, cause error) error {
	return &runner.ProviderFailure{
		Info: failure.Info{
			Version: failure.VersionV1,
			Stage:   failure.StageInitialization,
			Source:  failure.SourceRunner,
			Code:    code,
		},
		Cause: cause,
	}
}

func redactProviderError(message string, provider runner.ProviderConfig, channel *runner.ResultSubmissionChannel) string {
	return runner.RedactSecrets(message, runner.ProviderSecrets(provider, channel))
}

// wrapSetupFailure 把写配置、建临时目录一类的初始化故障统一标注为 runner
// 侧缺配置,调用点只需给出动作描述。
func wrapSetupFailure(action string, err error) error {
	return setupFailure(failure.CodeMissingConfiguration, fmt.Errorf("%s: %w", action, err))
}
