package runner

import (
	"encoding/json"
	"strings"
)

// ProviderError is the provider-reported error carried by one CLI event.
type ProviderError struct {
	Code    string
	Message string
}

// ExtractProviderError reads a provider error using one field priority shared by
// every CLI: error.code, error.type, the top level error_code, then a JSON error
// envelope embedded in the message. Without a single order the same upstream
// error classifies differently per runner, which changes recovery eligibility.
func ExtractProviderError(event map[string]any) ProviderError {
	errorObject, _ := event["error"].(map[string]any)
	code := stringValue(errorObject, "code")
	if code == "" {
		code = stringValue(errorObject, "type")
	}
	if code == "" {
		code = stringValue(event, "error_code")
	}
	message := stringValue(errorObject, "message")
	if message == "" {
		message = stringValue(event, "message")
	}
	if envelope, ok := embeddedProviderEnvelope(message); ok {
		if code == "" {
			code = envelope.Code
		}
		message = envelope.Message
	}
	return ProviderError{Code: code, Message: message}
}

type providerEnvelope struct {
	Code    string
	Message string
}

// embeddedProviderEnvelope unwraps a provider error that arrived as JSON text
// inside the message field, which is how some gateways report a nested error.
func embeddedProviderEnvelope(message string) (providerEnvelope, bool) {
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(message)), &envelope) != nil || strings.TrimSpace(envelope.Error.Message) == "" {
		return providerEnvelope{}, false
	}
	code := strings.TrimSpace(envelope.Error.Code)
	if code == "" {
		code = strings.TrimSpace(envelope.Error.Type)
	}
	return providerEnvelope{Code: code, Message: strings.TrimSpace(envelope.Error.Message)}, true
}

func stringValue(source map[string]any, name string) string {
	value, _ := source[name].(string)
	return strings.TrimSpace(value)
}
