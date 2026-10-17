package llm

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
)

// DecodeJSON unmarshals a model's JSON reply into v (a non-nil pointer).
//
// Models don't always return a clean object, so this tolerates prose around the
// object, Python-style single quotes, and replies that only landed in the
// reasoning stream. Candidates are tried from most to least likely; the first
// one that decodes wins. v is reset before each attempt so a failed partial
// decode never leaks fields into the result.
func DecodeJSON(content, reasoning string, v any) error {
	target := reflect.ValueOf(v)
	if target.Kind() != reflect.Pointer || target.IsNil() {
		return errors.New("llm: DecodeJSON requires a non-nil pointer")
	}
	zero := reflect.Zero(target.Elem().Type())

	var candidates []string
	if c := strings.TrimSpace(content); c != "" {
		candidates = append(candidates, c)
	}
	candidates = append(candidates, lastFirst(topLevelObjects(content))...)
	candidates = append(candidates, lastFirst(topLevelObjects(reasoning))...)
	if len(candidates) == 0 {
		return errors.New("llm: no JSON object found in response")
	}

	var firstErr error
	for _, c := range candidates {
		for _, attempt := range []string{c, normalizeQuotes(c)} {
			target.Elem().Set(zero)
			err := json.Unmarshal([]byte(attempt), v)
			if err == nil {
				return nil
			}
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	target.Elem().Set(zero)
	return firstErr
}

// topLevelObjects returns every balanced top-level {...} span in s, in order.
// Braces inside double-quoted strings are ignored.
func topLevelObjects(s string) []string {
	var objs []string
	depth, start := 0, -1
	inString, escaped := false, false

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			if depth > 0 {
				inString = true
			}
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					objs = append(objs, s[start:i+1])
					start = -1
				}
			}
		}
	}
	return objs
}

func lastFirst(items []string) []string {
	out := make([]string, 0, len(items))
	for i := len(items) - 1; i >= 0; i-- {
		out = append(out, items[i])
	}
	return out
}

// normalizeQuotes converts Python-style single-quoted objects to double quotes,
// keeping apostrophes inside words (don't, Vance's) intact.
func normalizeQuotes(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") || !strings.Contains(s, "'") {
		return s
	}

	isLetter := func(b byte) bool {
		return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
	}

	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && i+1 < len(s) && s[i+1] == '\'':
			sb.WriteByte('\'')
			i++
		case s[i] == '\'' && i > 0 && i+1 < len(s) && isLetter(s[i-1]) && isLetter(s[i+1]):
			sb.WriteByte('\'')
		case s[i] == '\'':
			sb.WriteByte('"')
		default:
			sb.WriteByte(s[i])
		}
	}
	return sb.String()
}
