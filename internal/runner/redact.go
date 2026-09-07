package runner

import (
	"sort"
	"strings"
)

// ProviderSecrets collects every secret that flows into a provider process:
// the model API key, the per-run result channel bearer token and the custom
// header values. The token must never surface in logs, events or persisted
// error messages.
func ProviderSecrets(provider ProviderConfig, channel *ResultSubmissionChannel) []string {
	secrets := make([]string, 0, len(provider.Headers)+2)
	if value := strings.TrimSpace(provider.APIKey); value != "" {
		secrets = append(secrets, value)
	}
	if channel != nil {
		if value := strings.TrimSpace(channel.BearerToken); value != "" {
			secrets = append(secrets, value)
		}
	}
	for _, value := range provider.Headers {
		if value = strings.TrimSpace(value); value != "" {
			secrets = append(secrets, value)
		}
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	return secrets
}

// RedactSecrets replaces every secret occurrence with [REDACTED], longest
// first so overlapping values collapse onto the same placeholder.
func RedactSecrets(message string, secrets []string) string {
	for _, secret := range secrets {
		message = strings.ReplaceAll(message, secret, "[REDACTED]")
	}
	return strings.TrimSpace(message)
}
