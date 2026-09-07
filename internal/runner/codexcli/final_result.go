package codexcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sdlc-coordinator/internal/runner"
)

const diagnosticsDirectoryName = ".agent-runner-diagnostics"

func normalizeFinalResult(raw []byte) (json.RawMessage, bool) {
	candidate := bytes.TrimSpace(raw)
	if compact, ok := compactJSON(candidate); ok {
		return compact, true
	}
	if compact, ok := compactFencedJSON(candidate); ok {
		return compact, true
	}
	fenceStart := bytes.Index(candidate, []byte("\n```"))
	if fenceStart < 0 {
		return nil, false
	}
	prefix := candidate[:fenceStart]
	if bytes.Contains(prefix, []byte("```")) {
		return nil, false
	}
	fenced := candidate[fenceStart+1:]
	if bytes.Count(fenced, []byte("```")) != 2 {
		return nil, false
	}
	return compactFencedJSON(fenced)
}

func compactFencedJSON(candidate []byte) (json.RawMessage, bool) {
	if len(candidate) < 6 || !bytes.HasPrefix(candidate, []byte("```")) || !bytes.HasSuffix(candidate, []byte("```")) {
		return nil, false
	}
	firstNewline := bytes.IndexByte(candidate, '\n')
	if firstNewline < 0 {
		return nil, false
	}
	language := strings.TrimSpace(string(candidate[3:firstNewline]))
	if language != "" && !strings.EqualFold(language, "json") {
		return nil, false
	}
	body := bytes.TrimSpace(candidate[firstNewline+1 : len(candidate)-3])
	return compactJSON(body)
}

func compactJSON(raw []byte) (json.RawMessage, bool) {
	if !json.Valid(raw) {
		return nil, false
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, false
	}
	return append(json.RawMessage(nil), compact.Bytes()...), true
}

func persistInvalidFinalDiagnostics(workspaceRoot string, finalOutput, adaptedSchema []byte, provider runner.ProviderConfig) (string, error) {
	diagnosticsRoot := filepath.Join(workspaceRoot, diagnosticsDirectoryName)
	if err := os.MkdirAll(diagnosticsRoot, 0o700); err != nil {
		return diagnosticsRoot, fmt.Errorf("create Codex diagnostics directory: %w", err)
	}
	redactedOutput := []byte(redactProviderError(string(finalOutput), provider))
	if err := os.WriteFile(filepath.Join(diagnosticsRoot, "codex-final-output.txt"), redactedOutput, 0o600); err != nil {
		return diagnosticsRoot, fmt.Errorf("write invalid Codex final output: %w", err)
	}
	if err := os.WriteFile(filepath.Join(diagnosticsRoot, "codex-result-schema.json"), adaptedSchema, 0o600); err != nil {
		return diagnosticsRoot, fmt.Errorf("write adapted Codex result schema: %w", err)
	}
	return diagnosticsRoot, nil
}
