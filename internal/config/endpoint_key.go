package config

import (
	"regexp"
	"strings"
)

// providerSegmentKeys lists values that may appear as the second dotted segment
// of a `ttft.<provider>.<name>.json` filename and should be skipped when
// deriving the endpoint key.
var providerSegmentKeys = map[string]bool{
	"claude":                 true,
	"gpt":                    true,
	"gemini":                 true,
	"openai":                 true,
	"anthropic":              true,
	"google":                 true,
	"deepseek":               true,
	"openrouter":             true,
	"azure-openai-responses": true,
	"vercel-ai-gateway":      true,
	"cloudflare-ai-gateway":  true,
	"cloudflare-workers-ai":  true,
}

var unsafeKeyChar = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

// EndpointKeyFromName derives an endpoint key from a config filename.
//
//	ttft.<provider>.<name>.json  -> <name>          (provider segment skipped)
//	ttft.<name>.json             -> <name>
//	<other>.json                 -> safe stem of <other>
//
// Unsafe characters become "_"; an all-empty result becomes "target".
func EndpointKeyFromName(filename string) string {
	parts := strings.Split(filename, ".")
	var key string
	if len(parts) >= 3 && strings.ToLower(parts[0]) == "ttft" && strings.ToLower(parts[len(parts)-1]) == "json" {
		if len(parts) >= 4 && providerSegmentKeys[strings.ToLower(parts[1])] {
			key = strings.Join(parts[2:len(parts)-1], ".")
		} else {
			key = strings.Join(parts[1:len(parts)-1], ".")
		}
	} else {
		// Strip the final extension only.
		if idx := strings.LastIndex(filename, "."); idx >= 0 {
			key = filename[:idx]
		} else {
			key = filename
		}
	}
	key = unsafeKeyChar.ReplaceAllString(key, "_")
	if strings.Trim(key, "._-") == "" {
		return "target"
	}
	return key
}

// UniqueKey returns key, appending _2, _3, ... if it already exists in used.
// The chosen key is added to used before returning.
func UniqueKey(base string, used map[string]bool) string {
	if !used[base] {
		used[base] = true
		return base
	}
	for counter := 2; ; counter++ {
		candidate := base + "_" + itoa(counter)
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
