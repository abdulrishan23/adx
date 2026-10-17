package registry

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ValidateJSON checks a value against a small JSON-Schema subset: type,
// required, properties, additionalProperties, enum, items, minimum,
// maximum, minLength, maxLength, minItems, maxItems. It returns every
// problem it finds, with a path, so a malformed output is named, not
// summarised.
func ValidateJSON(schema map[string]any, value any) []string {
	var problems []string
	validate(schema, value, "$", &problems)
	return problems
}

// ValidateRaw parses raw JSON and validates it.
func ValidateRaw(schema map[string]any, raw json.RawMessage) []string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return []string{"$: not valid JSON"}
	}
	return ValidateJSON(schema, v)
}

func validate(schema map[string]any, v any, path string, out *[]string) {
	if schema == nil {
		return
	}
	typ, _ := schema["type"].(string)
	if enum, ok := schema["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			if fmt.Sprint(e) == fmt.Sprint(v) {
				found = true
			}
		}
		if !found {
			*out = append(*out, fmt.Sprintf("%s: must be one of %s", path, joinAny(enum)))
			return
		}
	}
	switch typ {
	case "object":
		obj, ok := v.(map[string]any)
		if !ok {
			*out = append(*out, path+": must be an object")
			return
		}
		props, _ := schema["properties"].(map[string]any)
		for _, r := range stringList(schema["required"]) {
			if _, ok := obj[r]; !ok {
				*out = append(*out, fmt.Sprintf("%s: missing required field %q", path, r))
			}
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			ps, ok := props[k].(map[string]any)
			if !ok {
				if ap, has := schema["additionalProperties"]; has {
					switch a := ap.(type) {
					case bool:
						if !a {
							*out = append(*out, fmt.Sprintf("%s: unknown field %q", path, k))
						}
					case map[string]any:
						validate(a, obj[k], path+"."+k, out)
					}
				}
				continue
			}
			validate(ps, obj[k], path+"."+k, out)
		}
	case "array":
		arr, ok := v.([]any)
		if !ok {
			*out = append(*out, path+": must be an array")
			return
		}
		if n, ok := number(schema["minItems"]); ok && float64(len(arr)) < n {
			*out = append(*out, fmt.Sprintf("%s: needs at least %d items", path, int(n)))
		}
		if n, ok := number(schema["maxItems"]); ok && float64(len(arr)) > n {
			*out = append(*out, fmt.Sprintf("%s: allows at most %d items", path, int(n)))
		}
		if items, ok := schema["items"].(map[string]any); ok {
			for i, it := range arr {
				validate(items, it, fmt.Sprintf("%s[%d]", path, i), out)
			}
		}
	case "string":
		s, ok := v.(string)
		if !ok {
			*out = append(*out, path+": must be a string")
			return
		}
		if n, ok := number(schema["minLength"]); ok && float64(len(s)) < n {
			*out = append(*out, fmt.Sprintf("%s: needs at least %d characters", path, int(n)))
		}
		if n, ok := number(schema["maxLength"]); ok && float64(len(s)) > n {
			*out = append(*out, fmt.Sprintf("%s: allows at most %d characters", path, int(n)))
		}
	case "integer", "number":
		f, ok := v.(float64)
		if !ok {
			*out = append(*out, path+": must be a number")
			return
		}
		if typ == "integer" && f != float64(int64(f)) {
			*out = append(*out, path+": must be an integer")
		}
		if n, ok := number(schema["minimum"]); ok && f < n {
			*out = append(*out, fmt.Sprintf("%s: must be at least %v", path, n))
		}
		if n, ok := number(schema["maximum"]); ok && f > n {
			*out = append(*out, fmt.Sprintf("%s: must be at most %v", path, n))
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			*out = append(*out, path+": must be true or false")
		}
	case "null":
		if v != nil {
			*out = append(*out, path+": must be null")
		}
	case "":
	default:
		*out = append(*out, fmt.Sprintf("%s: schema uses unsupported type %q", path, typ))
	}
}

// ValidateSchema checks that a schema uses only the subset ValidateJSON
// understands, so a skill cannot declare an output nobody can check.
func ValidateSchema(schema map[string]any, path string) []string {
	var out []string
	if schema == nil {
		return []string{path + ": schema is required"}
	}
	typ, _ := schema["type"].(string)
	switch typ {
	case "object":
		props, _ := schema["properties"].(map[string]any)
		keys := make([]string, 0, len(props))
		for k := range props {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			ps, ok := props[k].(map[string]any)
			if !ok {
				out = append(out, fmt.Sprintf("%s.%s: property schema must be an object", path, k))
				continue
			}
			out = append(out, ValidateSchema(ps, path+"."+k)...)
		}
		for _, r := range stringList(schema["required"]) {
			if _, ok := props[r]; !ok {
				out = append(out, fmt.Sprintf("%s: required field %q is not a property", path, r))
			}
		}
	case "array":
		if items, ok := schema["items"].(map[string]any); ok {
			out = append(out, ValidateSchema(items, path+"[]")...)
		}
	case "string", "integer", "number", "boolean", "null":
	default:
		out = append(out, fmt.Sprintf("%s: type must be object, array, string, integer, number, boolean or null", path))
	}
	return out
}

func stringList(v any) []string {
	switch l := v.(type) {
	case []string:
		return l
	case []any:
		out := make([]string, 0, len(l))
		for _, x := range l {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func joinAny(l []any) string {
	parts := make([]string, 0, len(l))
	for _, x := range l {
		parts = append(parts, fmt.Sprint(x))
	}
	return strings.Join(parts, ", ")
}
