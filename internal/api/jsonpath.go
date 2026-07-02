package api

import (
	"strconv"
	"strings"
)

// parsePathTokens converts a JSON path like "choices.*.delta.content" or
// "$.choices[0].delta.content" into a sequence of string / int / "*" tokens.
func parsePathTokens(path string) []interface{} {
	s := strings.TrimSpace(path)
	switch {
	case strings.HasPrefix(s, "$."):
		s = s[2:]
	case strings.HasPrefix(s, "$"):
		s = s[1:]
	}
	s = strings.ReplaceAll(s, "[*]", ".*")
	// Replace [N] with .N for arbitrary N.
	s = replaceIndexed(s)

	var tokens []interface{}
	for _, part := range strings.Split(s, ".") {
		if part == "" {
			continue
		}
		if part == "*" {
			tokens = append(tokens, "*")
			continue
		}
		if n, err := strconv.Atoi(part); err == nil {
			tokens = append(tokens, n)
			continue
		}
		tokens = append(tokens, part)
	}
	return tokens
}

var digits = "0123456789"

// replaceIndexed replaces [N] with .N for arbitrary N.
func replaceIndexed(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '[' {
			j := i + 1
			for j < len(s) && strings.ContainsRune(digits, rune(s[j])) {
				j++
			}
			if j > i+1 && j < len(s) && s[j] == ']' {
				b.WriteByte('.')
				b.WriteString(s[i+1 : j])
				i = j + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// iterPathValues yields every value reachable from value along tokens.
func iterPathValues(value interface{}, tokens []interface{}) []interface{} {
	if len(tokens) == 0 {
		return []interface{}{value}
	}
	head := tokens[0]
	tail := tokens[1:]
	switch h := head.(type) {
	case string:
		if h == "*" {
			switch v := value.(type) {
			case map[string]interface{}:
				out := []interface{}{}
				for _, item := range v {
					out = append(out, iterPathValues(item, tail)...)
				}
				return out
			case []interface{}:
				out := []interface{}{}
				for _, item := range v {
					out = append(out, iterPathValues(item, tail)...)
				}
				return out
			}
			return nil
		}
		if dict, ok := value.(map[string]interface{}); ok {
			if item, present := dict[h]; present {
				return iterPathValues(item, tail)
			}
		}
		return nil
	case int:
		if list, ok := value.([]interface{}); ok {
			if h >= 0 && h < len(list) {
				return iterPathValues(list[h], tail)
			}
		}
		return nil
	}
	return nil
}

// extractByPaths tries each JSON path in order, returning the first non-empty
// string value found. The source is the matching path.
func extractByPaths(data interface{}, paths []string) (TokenMatch, bool) {
	for _, p := range paths {
		tokens := parsePathTokens(p)
		for _, v := range iterPathValues(data, tokens) {
			if s, ok := v.(string); ok && s != "" {
				return TokenMatch{Text: s, Source: p}, true
			}
		}
	}
	return TokenMatch{}, false
}
