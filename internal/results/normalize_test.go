package results

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustNormalize(t *testing.T, schema map[string]any, raw string) NormalizationResult {
	t.Helper()
	result, err := NormalizeInput(schema, json.RawMessage(raw))
	if err != nil {
		t.Fatalf("normalize input: %v", err)
	}
	return result
}

func repairsOf(result NormalizationResult) map[string]Repair {
	byPath := make(map[string]Repair, len(result.Repairs))
	for _, repair := range result.Repairs {
		byPath[repair.Path] = repair
	}
	return byPath
}

func TestNormalizeInputKeepsValidPayloadByteIdentical(t *testing.T) {
	raw := `{"task_id":"task-1","run_id":"run-1","outcome":"fix_failed","summary":"blocked","blocker_reason":"no tests"}`
	result := mustNormalize(t, NewDeveloperAgentContract().InputSchema(), raw)
	if !result.Clean() {
		t.Fatalf("valid payload must produce no repairs: %#v", result.Repairs)
	}
	if string(result.Payload) != raw {
		t.Fatalf("clean payload must be byte identical, got %s", result.Payload)
	}
}

func TestNormalizeInputRenamesKnownKeysToSchemaForm(t *testing.T) {
	raw := `{"taskId":"task-1","runId":"run-1","outcome":"fix_failed","summary":"blocked","blockerReason":"why","mergeRequest":{"title":"mr"}}`
	result := mustNormalize(t, NewDeveloperAgentContract().InputSchema(), raw)
	repairs := repairsOf(result)
	for _, path := range []string{"taskId", "runId", "blockerReason", "mergeRequest"} {
		if _, ok := repairs[path]; !ok {
			t.Fatalf("missing rename repair for %s in %#v", path, result.Repairs)
		}
		if repairs[path].Op != "rename_key" {
			t.Fatalf("repair %s op = %s", path, repairs[path].Op)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		t.Fatalf("decode normalized payload: %v", err)
	}
	for _, key := range []string{"task_id", "run_id", "blocker_reason", "merge_request"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("normalized payload missing %s: %s", key, result.Payload)
		}
	}
	if _, ok := payload["taskId"]; ok {
		t.Fatalf("normalized payload still carries alias key: %s", result.Payload)
	}
}

func TestNormalizeInputCoercesStringScalarsToSchemaTypes(t *testing.T) {
	raw := `{"task_id":"task-1","run_id":"run-1","outcome":"change_ready","summary":"done","tests":[{"command":"go test ./...","passed":"true","summary":"all green"}]}`
	result := mustNormalize(t, NewDeveloperAgentContract().InputSchema(), raw)
	repairs := repairsOf(result)
	coercion, ok := repairs["tests[0].passed"]
	if !ok {
		t.Fatalf("missing scalar coercion repair in %#v", result.Repairs)
	}
	if coercion.Op != "coerce_scalar" || coercion.Before != `"true"` || coercion.After != "true" {
		t.Fatalf("unexpected coercion repair: %#v", coercion)
	}
	var payload struct {
		Tests []struct {
			Passed bool `json:"passed"`
		} `json:"tests"`
	}
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		t.Fatalf("decode normalized payload: %v", err)
	}
	if len(payload.Tests) != 1 || !payload.Tests[0].Passed {
		t.Fatalf("tests[0].passed was not coerced: %s", result.Payload)
	}
}

func TestNormalizeInputDropsNullValuesAsAbsent(t *testing.T) {
	raw := `{"task_id":"task-1","run_id":"run-1","outcome":"fix_failed","summary":"blocked","blocker_reason":null,"tests":null}`
	result := mustNormalize(t, NewDeveloperAgentContract().InputSchema(), raw)
	repairs := repairsOf(result)
	if repair, ok := repairs["blocker_reason"]; !ok || repair.Op != "drop_null" {
		t.Fatalf("missing drop_null repair for blocker_reason in %#v", result.Repairs)
	}
	if repair, ok := repairs["tests"]; !ok || repair.Op != "drop_null" {
		t.Fatalf("missing drop_null repair for tests in %#v", result.Repairs)
	}
	var payload map[string]any
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		t.Fatalf("decode normalized payload: %v", err)
	}
	if _, ok := payload["blocker_reason"]; ok {
		t.Fatalf("null blocker_reason must be dropped: %s", result.Payload)
	}
}

func TestNormalizeInputLeavesUnknownKeysUntouched(t *testing.T) {
	raw := `{"task_id":"task-1","run_id":"run-1","outcome":"fix_failed","summary":"blocked","blocker_reason":"x","totally_unknown":{"nestedAlias":"keep"}}`
	result := mustNormalize(t, NewDeveloperAgentContract().InputSchema(), raw)
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		t.Fatalf("decode normalized payload: %v", err)
	}
	if _, ok := payload["totally_unknown"]; !ok {
		t.Fatalf("unknown key must survive normalization for strict decode to reject: %s", result.Payload)
	}
	if string(payload["totally_unknown"]) != `{"nestedAlias":"keep"}` {
		t.Fatalf("unknown subtree must stay untouched, got %s", payload["totally_unknown"])
	}
}

func TestNormalizeInputSkipsAmbiguousAliases(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"foo_bar": map[string]any{"type": "string"},
			"foobar":  map[string]any{"type": "string"},
			"FooBar":  map[string]any{"type": "string"},
		},
	}
	raw := `{"fooBar":"value"}`
	result := mustNormalize(t, schema, raw)
	if !result.Clean() {
		t.Fatalf("ambiguous alias must not be rewritten: %#v", result.Repairs)
	}
	if !strings.Contains(string(result.Payload), `"fooBar":"value"`) {
		t.Fatalf("ambiguous key must survive untouched: %s", result.Payload)
	}
}

func TestNormalizeInputCoercesNumbersForIntegerSchema(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"attempt_count": map[string]any{"type": "integer"},
		},
	}
	raw := `{"attempt_count":"9"}`
	result := mustNormalize(t, schema, raw)
	repairs := repairsOf(result)
	if repair, ok := repairs["attempt_count"]; !ok || repair.Op != "coerce_scalar" || repair.After != "9" {
		t.Fatalf("unexpected repairs: %#v", result.Repairs)
	}
	if string(result.Payload) != `{"attempt_count":9}` {
		t.Fatalf("number not coerced: %s", result.Payload)
	}
}

func TestNormalizeInputRejectsNonObjectPayloads(t *testing.T) {
	schema := NewDeveloperAgentContract().InputSchema()
	if _, err := NormalizeInput(schema, json.RawMessage(`not json`)); err == nil {
		t.Fatal("malformed json must error")
	}
	if _, err := NormalizeInput(schema, json.RawMessage(`["array"]`)); err == nil {
		t.Fatal("non-object payload must error")
	}
}

func TestNormalizeInputRepairsAreSerializable(t *testing.T) {
	raw := `{"taskId":"task-1","runId":"run-1","outcome":"fix_failed","summary":"blocked","blocker_reason":null}`
	result := mustNormalize(t, NewDeveloperAgentContract().InputSchema(), raw)
	encoded, err := json.Marshal(result.Repairs)
	if err != nil {
		t.Fatalf("marshal repairs: %v", err)
	}
	if !strings.Contains(string(encoded), `"path":"taskId"`) {
		t.Fatalf("repairs not serializable with path field: %s", encoded)
	}
}

func TestNormalizeInputRejectsNonJSONNumberForms(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"attempt_count": map[string]any{"type": "integer"},
		},
	}
	// ParseFloat 会接受这些形式，但它们不是合法 JSON 数字——必须原样
	// 穿透交由严格解码给出清晰的类型错误（回归锁定）。
	for _, form := range []string{`"NaN"`, `"Inf"`, `"+5"`, `"0x1p4"`} {
		raw := `{"attempt_count":` + form + `}`
		result, err := NormalizeInput(schema, json.RawMessage(raw))
		if err != nil {
			t.Fatalf("form %s must pass through without error, got %v", form, err)
		}
		if !result.Clean() {
			t.Fatalf("form %s must not be coerced: %#v", form, result.Repairs)
		}
	}
}

func TestNormalizeInputKeepsAliasWhenCanonicalKeyAlsoPresent(t *testing.T) {
	raw := `{"task_id":"task-canonical","taskId":"task-alias","run_id":"run-1","outcome":"fix_failed","summary":"x","blocker_reason":"y"}`
	result, err := NormalizeInput(NewDeveloperAgentContract().InputSchema(), json.RawMessage(raw))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Clean() {
		t.Fatalf("double-keyed payload must not be rewritten: %#v", result.Repairs)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if _, ok := payload["taskId"]; !ok {
		t.Fatalf("alias key must survive for strict decode to reject: %s", result.Payload)
	}
}

func TestNormalizeInputRejectsTrailingJSON(t *testing.T) {
	schema := NewDeveloperAgentContract().InputSchema()
	if _, err := NormalizeInput(schema, json.RawMessage(`{"task_id":"t"} {"run_id":"r"}`)); err == nil {
		t.Fatal("trailing JSON value must error like strict decode")
	}
}

func TestNormalizeInputDropsLiteralNullStringsAsAbsent(t *testing.T) {
	raw := `{"task_id":"task-1","run_id":"run-1","outcome":"change_ready","summary":"done","blocker_reason":" null "}`
	result := mustNormalize(t, NewDeveloperAgentContract().InputSchema(), raw)
	repairs := repairsOf(result)
	if repair, ok := repairs["blocker_reason"]; !ok || repair.Op != "drop_empty" {
		t.Fatalf("missing drop_empty repair for literal null string in %#v", result.Repairs)
	}
	var payload map[string]any
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		t.Fatalf("decode normalized payload: %v", err)
	}
	if _, ok := payload["blocker_reason"]; ok {
		t.Fatalf(`literal "null" blocker_reason must be dropped: %s`, result.Payload)
	}
}

func TestNormalizeInputDropsBlankStringsAsAbsent(t *testing.T) {
	raw := `{"task_id":"task-1","run_id":"run-1","outcome":"change_ready","summary":"done","blocker_reason":"   "}`
	result := mustNormalize(t, NewDeveloperAgentContract().InputSchema(), raw)
	if repair, ok := repairsOf(result)["blocker_reason"]; !ok || repair.Op != "drop_empty" {
		t.Fatalf("blank blocker_reason must be dropped: %#v", result.Repairs)
	}
}

func TestNormalizeInputKeepsRealContentStringsUntouched(t *testing.T) {
	raw := `{"task_id":"task-1","run_id":"run-1","outcome":"fix_failed","summary":"blocked by flaky ci","blocker_reason":"nullish coverage gate"}`
	result := mustNormalize(t, NewDeveloperAgentContract().InputSchema(), raw)
	if !result.Clean() {
		t.Fatalf("real content must not be rewritten: %#v", result.Repairs)
	}
}

func TestNormalizeInputDropsEmptyObjectsAsAbsent(t *testing.T) {
	raw := `{"task_id":"task-1","run_id":"run-1","outcome":"change_ready","summary":"done","defect":{}}`
	result := mustNormalize(t, NewDeveloperAgentContract().InputSchema(), raw)
	if repair, ok := repairsOf(result)["defect"]; !ok || repair.Op != "drop_empty" {
		t.Fatalf("empty defect object must be dropped: %#v", result.Repairs)
	}
	var payload map[string]any
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		t.Fatalf("decode normalized payload: %v", err)
	}
	if _, ok := payload["defect"]; ok {
		t.Fatalf("empty defect object must be dropped: %s", result.Payload)
	}
}

func TestNormalizeInputKeepsEmptyArraysUntouched(t *testing.T) {
	// 空数组解码为 nil 切片，len() 类契约校验视同缺键——删它没有收益，
	// 只会把合法的显式空清单（如 reviewer approved 的 findings:[]）
	// 污染成归一修复（回归锁定）。
	raw := `{"task_id":"task-1","run_id":"run-1","outcome":"change_ready","summary":"done","tests":[]}`
	result := mustNormalize(t, NewDeveloperAgentContract().InputSchema(), raw)
	if !result.Clean() {
		t.Fatalf("empty arrays must pass through untouched: %#v", result.Repairs)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		t.Fatalf("decode normalized payload: %v", err)
	}
	if string(payload["tests"]) != "[]" {
		t.Fatalf("empty array must survive byte-identical, got %s", payload["tests"])
	}
}

func TestNormalizeInputCollapsesAllEmptyFieldsToObjectDrop(t *testing.T) {
	raw := `{"task_id":"task-1","run_id":"run-1","outcome":"change_ready","summary":"done","defect":{"repository":"null","title":"","description":"NULL"}}`
	result := mustNormalize(t, NewDeveloperAgentContract().InputSchema(), raw)
	repairs := repairsOf(result)
	if _, ok := repairs["defect"]; !ok || repairs["defect"].Op != "drop_empty" {
		t.Fatalf("all-empty defect must collapse to a key drop: %#v", result.Repairs)
	}
	var payload map[string]any
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		t.Fatalf("decode normalized payload: %v", err)
	}
	if _, ok := payload["defect"]; ok {
		t.Fatalf("all-empty defect must be dropped: %s", result.Payload)
	}
}

func TestNormalizeInputDropsEmptyRequiredStringForAccurateRejection(t *testing.T) {
	raw := `{"task_id":"task-1","run_id":"run-1","outcome":"fix_failed","summary":"null"}`
	result := mustNormalize(t, NewDeveloperAgentContract().InputSchema(), raw)
	if repair, ok := repairsOf(result)["summary"]; !ok || repair.Op != "drop_empty" {
		t.Fatalf(`"null" summary must be dropped for the required check to reject: %#v`, result.Repairs)
	}
	var payload map[string]any
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		t.Fatalf("decode normalized payload: %v", err)
	}
	if _, ok := payload["summary"]; ok {
		t.Fatalf(`"null" summary must not survive into strict decoding: %s`, result.Payload)
	}
}

func TestNormalizeInputKeepsPartlyEmptyObjectsIntact(t *testing.T) {
	raw := `{"task_id":"task-1","run_id":"run-1","outcome":"change_ready","summary":"done","merge_request":{"title":"real title","description":""}}`
	result := mustNormalize(t, NewDeveloperAgentContract().InputSchema(), raw)
	var payload struct {
		MergeRequest struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"merge_request"`
	}
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		t.Fatalf("decode normalized payload: %v", err)
	}
	if payload.MergeRequest.Title != "real title" {
		t.Fatalf("real title must survive: %s", result.Payload)
	}
	if payload.MergeRequest.Description != "" {
		t.Fatalf("blank description should decode as empty, got %q: %s", payload.MergeRequest.Description, result.Payload)
	}
}
