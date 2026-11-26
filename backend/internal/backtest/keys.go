package backtest

import (
	"strings"
)

// CamelCase converts snake_case text to camelCase.
func CamelCase(value string) string {
	parts := strings.Split(value, "_")
	if len(parts) == 1 {
		return value
	}
	var builder strings.Builder
	for i, part := range parts {
		if i == 0 {
			builder.WriteString(part)
			continue
		}
		if part == "" {
			builder.WriteByte('_')
			continue
		}
		builder.WriteString(strings.ToUpper(part[:1]))
		builder.WriteString(part[1:])
	}
	return builder.String()
}

// CamelCaseKeys recursively converts map keys while leaving values unchanged.
func CamelCaseKeys(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[CamelCase(key)] = CamelCaseKeys(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = CamelCaseKeys(item)
		}
		return out
	default:
		return value
	}
}
