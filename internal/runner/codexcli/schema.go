package codexcli

import (
	"encoding/json"
	"errors"
	"sort"
)

func adaptResponsesSchema(raw []byte) ([]byte, error) {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, errors.New("codex result schema is invalid JSON")
	}
	adaptResponsesSchemaNode(schema)
	adapted, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, errors.New("encode codex result schema")
	}
	return append(adapted, '\n'), nil
}

func adaptResponsesSchemaNode(schema map[string]any) {
	properties, hasProperties := schema["properties"].(map[string]any)
	if hasProperties {
		originallyRequired := stringSet(schema["required"])
		names := make([]string, 0, len(properties))
		for name, rawProperty := range properties {
			property, ok := rawProperty.(map[string]any)
			if !ok {
				continue
			}
			adaptResponsesSchemaNode(property)
			if !originallyRequired[name] {
				makeSchemaNullable(property)
			}
			properties[name] = property
			names = append(names, name)
		}
		sort.Strings(names)
		schema["required"] = names
		schema["additionalProperties"] = false
	}

	if items, ok := schema["items"].(map[string]any); ok {
		adaptResponsesSchemaNode(items)
	}
	if definitions, ok := schema["$defs"].(map[string]any); ok {
		for _, rawDefinition := range definitions {
			if definition, ok := rawDefinition.(map[string]any); ok {
				adaptResponsesSchemaNode(definition)
			}
		}
	}
	for _, keyword := range []string{"allOf", "anyOf", "oneOf"} {
		variants, _ := schema[keyword].([]any)
		for _, rawVariant := range variants {
			if variant, ok := rawVariant.(map[string]any); ok {
				adaptResponsesSchemaNode(variant)
			}
		}
	}
}

func stringSet(raw any) map[string]bool {
	result := map[string]bool{}
	values, _ := raw.([]any)
	for _, value := range values {
		if text, ok := value.(string); ok {
			result[text] = true
		}
	}
	return result
}

func makeSchemaNullable(schema map[string]any) {
	if schemaAllowsNull(schema) {
		return
	}
	nonNull := make(map[string]any, len(schema))
	for key, value := range schema {
		nonNull[key] = value
		delete(schema, key)
	}
	schema["anyOf"] = []any{nonNull, map[string]any{"type": "null"}}
}

func schemaAllowsNull(schema map[string]any) bool {
	if schema["type"] == "null" {
		return true
	}
	if values, ok := schema["type"].([]any); ok {
		for _, value := range values {
			if value == "null" {
				return true
			}
		}
	}
	if variants, ok := schema["anyOf"].([]any); ok {
		for _, rawVariant := range variants {
			variant, _ := rawVariant.(map[string]any)
			if variant["type"] == "null" {
				return true
			}
		}
	}
	return false
}
