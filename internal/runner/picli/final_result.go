package picli

import (
	"bytes"
	"encoding/json"
	"strings"
)

// normalizeFinalResult 从最终助手消息里取出可用的结果 JSON:裸对象、整段
// 围栏代码块,或散文后紧跟的唯一围栏块。行为与 codex Provider 保持一致,
// 各 Provider 包各自持有一份实现。
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
