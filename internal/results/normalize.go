package results

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Repair records one whitelist normalization applied to an agent result
// payload before strict decoding. Ops: rename_key, coerce_scalar, drop_null.
type Repair struct {
	Op     string `json:"op"`
	Path   string `json:"path"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// NormalizationResult carries the (possibly rewritten) payload plus the repair
// trail. A clean result returns the original bytes untouched.
type NormalizationResult struct {
	Payload json.RawMessage
	Repairs []Repair
}

func (r NormalizationResult) Clean() bool { return len(r.Repairs) == 0 }

// NormalizeInput applies the shared-entry thin normalization: known-key alias
// renaming (taskId→task_id), string scalar coercion toward schema types
// ("true"→true, "9"→9), and null treated as absent. Values outside the schema
// key domain are never rewritten, invented or dropped — strict decoding stays
// the only authority for rejecting unknown fields.
func NormalizeInput(schema map[string]any, raw json.RawMessage) (NormalizationResult, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return NormalizationResult{}, fmt.Errorf("payload is not valid JSON: %w", err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return NormalizationResult{}, errors.New("payload must be a JSON object")
	}
	normalized, repairs, err := normalizeNode(schema, object, "")
	if err != nil {
		return NormalizationResult{}, err
	}
	if len(repairs) == 0 {
		return NormalizationResult{Payload: json.RawMessage(raw)}, nil
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return NormalizationResult{}, fmt.Errorf("encode normalized payload: %w", err)
	}
	return NormalizationResult{Payload: encoded, Repairs: repairs}, nil
}

func normalizeNode(schema map[string]any, value any, path string) (any, []Repair, error) {
	switch typed := value.(type) {
	case map[string]any:
		return normalizeObject(schema, typed, path)
	case []any:
		itemsSchema, _ := schema["items"].(map[string]any)
		normalized := make([]any, 0, len(typed))
		var repairs []Repair
		for index, item := range typed {
			itemPath := fmt.Sprintf("%s[%d]", path, index)
			converted, itemRepairs, err := normalizeNode(itemsSchema, item, itemPath)
			if err != nil {
				return nil, nil, err
			}
			normalized = append(normalized, converted)
			repairs = append(repairs, itemRepairs...)
		}
		return normalized, repairs, nil
	default:
		return value, nil, nil
	}
}

func normalizeObject(schema map[string]any, object map[string]any, path string) (any, []Repair, error) {
	properties, _ := schema["properties"].(map[string]any)
	aliases := buildAliasIndex(properties)
	normalized := make(map[string]any, len(object))
	var repairs []Repair
	for key, item := range object {
		keyPath := key
		if path != "" {
			keyPath = path + "." + key
		}
		schemaNode, _ := properties[key].(map[string]any)
		known := schemaNode != nil
		if !known {
			if matches := aliases[aliasKey(key)]; len(matches) == 1 {
				target := matches[0]
				repairs = append(repairs, Repair{Op: "rename_key", Path: keyPath, Before: key, After: target})
				key, known = target, true
				schemaNode, _ = properties[target].(map[string]any)
			}
		}
		if !known {
			// 未知键整体原样保留，交给严格解码拒绝。
			normalized[key] = item
			continue
		}
		if item == nil {
			repairs = append(repairs, Repair{Op: "drop_null", Path: keyPath, Before: "null"})
			continue
		}
		converted, itemRepairs, err := normalizeNode(schemaNode, item, keyPath)
		if err != nil {
			return nil, nil, err
		}
		if coerced, repair := coerceScalar(schemaNode, item, keyPath); repair.Op != "" {
			converted, itemRepairs = coerced, append(itemRepairs, repair)
		}
		normalized[key] = converted
		repairs = append(repairs, itemRepairs...)
	}
	return normalized, repairs, nil
}

// coerceScalar rewrites string leaves toward the schema-declared type. Only
// lossless conversions apply; anything else passes through for strict decoding.
func coerceScalar(schema map[string]any, value any, path string) (any, Repair) {
	text, ok := value.(string)
	if !ok {
		return value, Repair{}
	}
	for _, schemaType := range schemaTypes(schema) {
		switch schemaType {
		case "boolean":
			if text == "true" || text == "false" {
				return text == "true", Repair{Op: "coerce_scalar", Path: path, Before: strconvQuote(text), After: text}
			}
		case "integer":
			if validIntegerLiteral(text) {
				return json.Number(text), Repair{Op: "coerce_scalar", Path: path, Before: strconvQuote(text), After: text}
			}
		case "number":
			if validNumberLiteral(text) {
				return json.Number(text), Repair{Op: "coerce_scalar", Path: path, Before: strconvQuote(text), After: text}
			}
		}
	}
	return value, Repair{}
}

func schemaTypes(schema map[string]any) []string {
	switch typed := schema["type"].(type) {
	case string:
		return []string{typed}
	case []string:
		return typed
	case []any:
		types := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				types = append(types, text)
			}
		}
		return types
	default:
		return nil
	}
}

func buildAliasIndex(properties map[string]any) map[string][]string {
	aliases := make(map[string][]string, len(properties))
	for name := range properties {
		key := aliasKey(name)
		aliases[key] = append(aliases[key], name)
	}
	return aliases
}

func aliasKey(name string) string {
	replaced := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(name))
	return replaced
}

func validIntegerLiteral(text string) bool {
	if text == "" {
		return false
	}
	if strings.ContainsAny(text, ".eE") {
		return false
	}
	var number json.Number = json.Number(text)
	_, err := number.Float64()
	return err == nil
}

func validNumberLiteral(text string) bool {
	if text == "" {
		return false
	}
	var number json.Number = json.Number(text)
	_, err := number.Float64()
	return err == nil
}

func strconvQuote(text string) string {
	encoded, _ := json.Marshal(text)
	return string(encoded)
}
