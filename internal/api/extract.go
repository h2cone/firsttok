// Package api implements the API-form adapters: first-token extraction rules
// and event-name sets for OpenAI Responses, OpenAI Chat Completions, Anthropic
// Messages and Google Generative AI, plus the auto fallback.
package api

import "strings"

// OpenAI Responses API event names that carry text deltas.
var OpenAITextEvents = map[string]bool{
	"response.output_text.delta":            true,
	"response.reasoning_text.delta":         true,
	"response.reasoning_summary_text.delta": true,
	"response.refusal.delta":                true,
}

// Generic chat-completion token paths (also used as a Responses fallback).
var GenericChatTokenPaths = []string{
	"choices.*.delta.content",
	"choices.*.delta.reasoning_content",
	"choices.*.delta.refusal",
	"choices.*.text",
}

// Anthropic Messages token paths.
var ClaudeTokenPaths = []string{
	"delta.text",
	"content_block.text",
}

// Google Generative AI token paths. The `thought` path is boolean in Gemini and
// intentionally never matches because firstNonEmptyText only accepts non-empty
// strings.
var GeminiTokenPaths = []string{
	"candidates.*.content.parts.*.text",
	"candidates.*.content.parts.*.thought",
}

// TokenMatch is an extracted first token: its text and the source path/event.
type TokenMatch struct {
	Text   string
	Source string
}

// ExtractToken applies the api-form extraction rules to one parsed SSE event.
// eventType is the SSE event: line value (falls back to data["type"]).
// data is the parsed JSON payload (or nil for non-JSON events).
// raw is the raw stripped event data string (used by the auto fallback).
// customPaths are user-configured first_token_json_paths, tried first.
func ExtractToken(apiForm, eventType string, data interface{}, raw string, customPaths []string) (TokenMatch, bool) {
	// 1. Custom paths take precedence.
	if len(customPaths) > 0 && data != nil {
		if m, ok := extractByPaths(data, customPaths); ok {
			return m, true
		}
	}
	switch apiForm {
	case "openai-responses":
		return extractOpenAIResponses(eventType, data)
	case "openai-chat-completions":
		if data != nil {
			if m, ok := extractByPaths(data, GenericChatTokenPaths); ok {
				return m, true
			}
		}
		return TokenMatch{}, false
	case "anthropic-messages":
		if data != nil {
			if m, ok := extractByPaths(data, ClaudeTokenPaths); ok {
				return m, true
			}
		}
		return TokenMatch{}, false
	case "google-generative-ai":
		if data != nil {
			if m, ok := extractByPaths(data, GeminiTokenPaths); ok {
				return m, true
			}
		}
		return TokenMatch{}, false
	default:
		// auto fallback: the first non-empty event data string is the token.
		if strings.TrimSpace(raw) != "" && strings.TrimSpace(raw) != "[DONE]" {
			return TokenMatch{Text: raw, Source: "first-json-event"}, true
		}
		return TokenMatch{}, false
	}
}

// extractOpenAIResponses handles Responses API events: text-delta events carry
// the token in data["delta"]; other events fall back to generic chat paths.
func extractOpenAIResponses(eventType string, data interface{}) (TokenMatch, bool) {
	if dict, ok := data.(map[string]interface{}); ok {
		name := eventType
		if name == "" {
			if t, ok := dict["type"].(string); ok {
				name = t
			}
		}
		if OpenAITextEvents[name] {
			if delta, ok := dict["delta"].(string); ok && delta != "" {
				return TokenMatch{Text: delta, Source: name}, true
			}
		}
		if m, ok := extractByPaths(dict, GenericChatTokenPaths); ok {
			return m, true
		}
	}
	return TokenMatch{}, false
}
