package ai

import "encoding/json"

// FunctionParametersSchema converts a tool param map into a JSON Schema document
// with type "object". OpenAI-compatible providers (including OpenCode) reject
// null, a bare type name, or a flat {name: "string"} map.
func FunctionParametersSchema(params map[string]interface{}) json.RawMessage {
	b, err := json.Marshal(normalizeToolJSONSchema(params))
	if err != nil || len(b) == 0 || string(b) == "null" {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return json.RawMessage(b)
}

func normalizeToolJSONSchema(params map[string]interface{}) map[string]interface{} {
	if params == nil {
		return emptyObjectSchema()
	}

	if _, hasType := params["type"]; hasType || hasProperties(params) {
		out := cloneMap(params)
		if out["type"] == nil || out["type"] == "" {
			out["type"] = "object"
		}
		if _, ok := out["properties"]; !ok {
			out["properties"] = map[string]interface{}{}
		}
		return out
	}

	properties := make(map[string]interface{}, len(params))
	required := make([]string, 0, len(params))
	for name, spec := range params {
		properties[name] = propertySchema(spec)
		required = append(required, name)
	}
	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func emptyObjectSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func hasProperties(m map[string]interface{}) bool {
	_, ok := m["properties"]
	return ok
}

func cloneMap(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func propertySchema(v interface{}) map[string]interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		if t["type"] == nil || t["type"] == "" {
			cp := cloneMap(t)
			cp["type"] = "string"
			return cp
		}
		return t
	case string:
		typ := t
		if typ == "" {
			typ = "string"
		}
		return map[string]interface{}{"type": typ}
	default:
		return map[string]interface{}{"type": "string"}
	}
}
