package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadBasicAndAliases(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "ttft.claude.dmxapi.json", `{
		"provider": "claude",
		"BASE_URL": "https://www.dmxapi.cn",
		"api_key_env": "DMX_API_KEY",
		"ENDPOINT": "/v1/messages",
		"request": {"model":"claude-sonnet-4-6","max_tokens":1000,"messages":[{"role":"user","content":"hi"}],"stream":true}
	}`)
	t.Setenv("DMX_API_KEY", "secret")
	c, err := Load(p, &CLIOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Provider != "claude" {
		t.Errorf("provider = %q", c.Provider)
	}
	if c.BaseURL != "https://www.dmxapi.cn" {
		t.Errorf("base_url = %q", c.BaseURL)
	}
	if c.Path != "/v1/messages" {
		t.Errorf("path = %q", c.Path)
	}
	if c.APIKey != "secret" {
		t.Errorf("api key not resolved from env, got %q", c.APIKey)
	}
	if c.API != APIAnthropicMessages {
		t.Errorf("api auto-infer = %q, want anthropic-messages", c.API)
	}
	if c.Headers["x-api-key"] != "secret" {
		t.Errorf("anthropic auth header not set, got %q", c.Headers["x-api-key"])
	}
}

func TestAPIKeyEnvPrefix(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "c.json", `{
		"provider":"openai","base_url":"https://api.openai.com","api_key":"env:MY_KEY",
		"path":"/v1/responses","request":{"model":"gpt","input":"hi","stream":true}
	}`)
	t.Setenv("MY_KEY", "k123")
	c, err := Load(p, &CLIOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if c.APIKey != "k123" {
		t.Errorf("env: prefix not resolved, got %q", c.APIKey)
	}
}

func TestAPIKeyResolutionOrder(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "c.json", `{
		"provider":"openai","base_url":"https://x","api_key_env":"CFG_ENV","api_key":"plainkey",
		"path":"/v1/responses","request":{"model":"gpt","input":"hi","stream":true}
	}`)
	t.Setenv("CFG_ENV", "fromenv")
	// CLI plaintext wins over everything.
	c, err := Load(p, &CLIOverrides{APIKey: "clikey"})
	if err != nil {
		t.Fatal(err)
	}
	if c.APIKey != "clikey" {
		t.Errorf("CLI plaintext should win, got %q", c.APIKey)
	}
	// CLI env name wins over config env name and config plaintext.
	t.Setenv("CLI_ENV", "clival")
	c2, err := Load(p, &CLIOverrides{APIKeyEnv: "CLI_ENV"})
	if err != nil {
		t.Fatal(err)
	}
	if c2.APIKey != "clival" {
		t.Errorf("CLI env should win, got %q", c2.APIKey)
	}
	// Without CLI, config env name wins over config plaintext.
	c3, err := Load(p, &CLIOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if c3.APIKey != "fromenv" {
		t.Errorf("config env should win over plaintext, got %q", c3.APIKey)
	}
}

func TestResolveAPIAutoInference(t *testing.T) {
	cases := []struct {
		url, path string
		want      string
	}{
		{"", "/v1/messages", APIAnthropicMessages},
		{"", "/v1/responses", APIOpenAIResponses},
		{"", "/v1/chat/completions", APIOpenAIChat},
		{"https://x/v1beta/models/gemini:streamGenerateContent?alt=sse", "", APIGoogleGenerativeAI},
	}
	for _, tc := range cases {
		got := ResolveAPI(APIAuto, tc.url, tc.path, nil)
		if got != tc.want {
			t.Errorf("ResolveAPI(%q,%q) = %q, want %q", tc.url, tc.path, got, tc.want)
		}
	}
}

func TestBuildURL(t *testing.T) {
	c := &Config{BaseURL: "https://api.x.com", Path: "/v1/responses", API: APIOpenAIResponses}
	u, err := BuildURL(c)
	if err != nil {
		t.Fatal(err)
	}
	if u != "https://api.x.com/v1/responses" {
		t.Errorf("url = %q", u)
	}
	// Explicit url with scheme wins.
	c2 := &Config{BaseURL: "https://api.x.com", URL: "https://other.com/v1/messages", API: APIAnthropicMessages}
	u2, _ := BuildURL(c2)
	if u2 != "https://other.com/v1/messages" {
		t.Errorf("url = %q", u2)
	}
	// base_url + url without scheme joins.
	c3 := &Config{BaseURL: "https://api.x.com", URL: "v1/messages", API: APIAnthropicMessages}
	u3, _ := BuildURL(c3)
	if u3 != "https://api.x.com/v1/messages" {
		t.Errorf("url = %q", u3)
	}
}

func TestValidateStream(t *testing.T) {
	body := map[string]interface{}{"stream": true}
	if err := ValidateStream(APIOpenAIResponses, body, false); err != nil {
		t.Errorf("expected pass for stream=true, got %v", err)
	}
	body2 := map[string]interface{}{"stream": false}
	if err := ValidateStream(APIOpenAIChat, body2, false); err == nil {
		t.Error("expected error for stream=false")
	}
	// --no-validate-stream bypasses.
	if err := ValidateStream(APIOpenAIChat, body2, true); err != nil {
		t.Errorf("no-validate should bypass, got %v", err)
	}
	// Gemini does not require stream in body.
	if err := ValidateStream(APIGoogleGenerativeAI, map[string]interface{}{}, false); err != nil {
		t.Errorf("gemini should not require stream, got %v", err)
	}
}

func TestEndpointKeyFromName(t *testing.T) {
	cases := map[string]string{
		"ttft.claude.dmxapi.json":              "dmxapi",
		"ttft.dmxapi.json":                     "dmxapi",
		"ttft.claude.dcs-no-plugin-proxy.json": "dcs-no-plugin-proxy",
		"ttft.claude.opus.dmxapi.json":         "opus.dmxapi",
		"ttft.openai.azure.json":               "azure",
		"random.json":                          "random",
	}
	for in, want := range cases {
		got := EndpointKeyFromName(in)
		if got != want {
			t.Errorf("EndpointKeyFromName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUniqueKey(t *testing.T) {
	used := map[string]bool{}
	if k := UniqueKey("dcs", used); k != "dcs" {
		t.Errorf("first = %q", k)
	}
	if k := UniqueKey("dcs", used); k != "dcs_2" {
		t.Errorf("dup = %q", k)
	}
	if k := UniqueKey("dcs", used); k != "dcs_3" {
		t.Errorf("dup2 = %q", k)
	}
}

func TestAuthHeaderByAPI(t *testing.T) {
	// OpenAI -> Authorization Bearer.
	h := map[string]string{}
	applyAuthHeader(h, APIOpenAIResponses, "k")
	if h["Authorization"] != "Bearer k" {
		t.Errorf("openai auth = %q", h["Authorization"])
	}
	// User-provided Authorization suppresses auto-add.
	h2 := map[string]string{"Authorization": "Token existing"}
	applyAuthHeader(h2, APIOpenAIChat, "k")
	if h2["Authorization"] != "Token existing" {
		t.Errorf("user auth overwritten: %q", h2["Authorization"])
	}
	// Anthropic -> x-api-key unless authorization present.
	h3 := map[string]string{}
	applyAuthHeader(h3, APIAnthropicMessages, "k")
	if h3["x-api-key"] != "k" {
		t.Errorf("anthropic auth = %q", h3["x-api-key"])
	}
	// Google -> x-goog-api-key.
	h4 := map[string]string{}
	applyAuthHeader(h4, APIGoogleGenerativeAI, "k")
	if h4["x-goog-api-key"] != "k" {
		t.Errorf("google auth = %q", h4["x-goog-api-key"])
	}
}
