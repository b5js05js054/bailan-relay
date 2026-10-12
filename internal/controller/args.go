package controller

import "strconv"

func argAt(args []any, i int) any {
	if i < 0 || i >= len(args) {
		return nil
	}
	return args[i]
}

func argString(args []any, i int) string {
	if s, ok := argAt(args, i).(string); ok {
		return s
	}
	return ""
}

func argInt(args []any, i, def int) int {
	switch v := argAt(args, i).(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func argBool(args []any, i int, def bool) bool {
	if b, ok := argAt(args, i).(bool); ok {
		return b
	}
	return def
}

func argMap(args []any, i int) map[string]any {
	if m, ok := argAt(args, i).(map[string]any); ok {
		return m
	}
	return nil
}

func mapString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

func mapBool(m map[string]any, key string, def bool) bool {
	if m == nil {
		return def
	}
	if b, ok := m[key].(bool); ok {
		return b
	}
	return def
}

func mapStringSlice(m map[string]any, key string) []string {
	if m == nil {
		return nil
	}
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
