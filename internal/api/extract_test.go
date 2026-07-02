package api

import "testing"

func TestExtractOpenAIResponses(t *testing.T) {
	data := map[string]interface{}{"type": "response.output_text.delta", "delta": "Hello"}
	m, ok := ExtractToken("openai-responses", "response.output_text.delta", data, "", nil)
	if !ok || m.Text != "Hello" {
		t.Errorf("expected Hello, got %+v ok=%v", m, ok)
	}
	if m.Source != "response.output_text.delta" {
		t.Errorf("source = %q", m.Source)
	}
	// event: line absent -> fall back to data["type"].
	m2, ok := ExtractToken("openai-responses", "", data, "", nil)
	if !ok || m2.Text != "Hello" {
		t.Errorf("fallback to type field failed: %+v", m2)
	}
}

func TestExtractOpenAIChat(t *testing.T) {
	data := map[string]interface{}{"choices": []interface{}{
		map[string]interface{}{"delta": map[string]interface{}{"content": "Hi"}},
	}}
	m, ok := ExtractToken("openai-chat-completions", "", data, "", nil)
	if !ok || m.Text != "Hi" {
		t.Errorf("expected Hi, got %+v", m)
	}
	if m.Source != "choices.*.delta.content" {
		t.Errorf("source = %q", m.Source)
	}
}

func TestExtractAnthropic(t *testing.T) {
	data := map[string]interface{}{"delta": map[string]interface{}{"text": "Bonjour"}}
	m, ok := ExtractToken("anthropic-messages", "content_block_delta", data, "", nil)
	if !ok || m.Text != "Bonjour" {
		t.Errorf("expected Bonjour, got %+v", m)
	}
	if m.Source != "delta.text" {
		t.Errorf("source = %q", m.Source)
	}
}

func TestExtractGeminiThoughtNeverMatches(t *testing.T) {
	// thought is boolean -> must NOT be coerced to a token.
	data := map[string]interface{}{"candidates": []interface{}{
		map[string]interface{}{"content": map[string]interface{}{"parts": []interface{}{
			map[string]interface{}{"thought": true},
		}}},
	}}
	if _, ok := ExtractToken("google-generative-ai", "", data, "", nil); ok {
		t.Error("boolean thought must never match as a token")
	}
	// text part matches.
	data2 := map[string]interface{}{"candidates": []interface{}{
		map[string]interface{}{"content": map[string]interface{}{"parts": []interface{}{
			map[string]interface{}{"text": "Hola"},
		}}},
	}}
	m, ok := ExtractToken("google-generative-ai", "", data2, "", nil)
	if !ok || m.Text != "Hola" {
		t.Errorf("expected Hola, got %+v", m)
	}
}

func TestCustomPathsPrecedence(t *testing.T) {
	data := map[string]interface{}{"foo": map[string]interface{}{"bar": "custom"}}
	m, ok := ExtractToken("openai-chat-completions", "", data, "", []string{"foo.bar"})
	if !ok || m.Text != "custom" {
		t.Errorf("custom path should win, got %+v", m)
	}
}

func TestAutoFallback(t *testing.T) {
	m, ok := ExtractToken("auto", "", nil, `{"some":"json"}`, nil)
	if !ok || m.Text != `{"some":"json"}` {
		t.Errorf("auto fallback should return raw data, got %+v", m)
	}
	if m.Source != "first-json-event" {
		t.Errorf("source = %q", m.Source)
	}
	// [DONE] is skipped.
	if _, ok := ExtractToken("auto", "", nil, "[DONE]", nil); ok {
		t.Error("[DONE] should not match")
	}
}

func TestParsePathTokens(t *testing.T) {
	tokens := parsePathTokens("choices.*.delta.content")
	want := []interface{}{"choices", "*", "delta", "content"}
	if len(tokens) != len(want) {
		t.Fatalf("got %v", tokens)
	}
	for i := range want {
		if tokens[i] != want[i] {
			t.Errorf("token %d = %v, want %v", i, tokens[i], want[i])
		}
	}
	// $. prefix and [N] index.
	t2 := parsePathTokens("$.choices[0].text")
	if t2[0] != "choices" || t2[1] != 0 || t2[2] != "text" {
		t.Errorf("path with $.[N] = %v", t2)
	}
}

func TestEmptyDeltaSkipped(t *testing.T) {
	// OpenAI responses delta empty string should not match the text event.
	data := map[string]interface{}{"type": "response.output_text.delta", "delta": ""}
	if _, ok := ExtractToken("openai-responses", "response.output_text.delta", data, "", nil); ok {
		t.Error("empty delta should not match")
	}
}
