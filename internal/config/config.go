// Package config loads firsttok JSON configs, applies CLI overrides, resolves
// API keys, and builds the request URL and headers. It is the single source of
// truth for the resolved probe configuration.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// API form values.
const (
	APIAuto               = "auto"
	APIOpenAIResponses    = "openai-responses"
	APIOpenAIChat         = "openai-chat-completions"
	APIAnthropicMessages  = "anthropic-messages"
	APIGoogleGenerativeAI = "google-generative-ai"
)

// Default request headers, applied first and overwritten by user headers.
var DefaultHeaders = map[string]string{
	"Accept":          "text/event-stream",
	"Accept-Encoding": "identity",
	"Cache-Control":   "no-cache",
	"Content-Type":    "application/json",
}

// Config is a fully resolved probe configuration ready for the HTTP probe.
type Config struct {
	Provider        string
	API             string // resolved api form (never "auto" after Resolve)
	BaseURL         string
	Path            string
	URL             string // explicit full url, highest priority
	APIKey          string
	Headers         map[string]string
	Request         map[string]interface{}
	RequestRaw      json.RawMessage
	FirstTokenPaths []string
	VerifySSL       bool
	Model           string
	ConfigPath      string
	Label           string
}

// CLIOverrides carries command-line flags that override config file values.
type CLIOverrides struct {
	Provider         string
	API              string
	BaseURL          string
	URL              string
	Path             string
	APIKey           string
	APIKeyEnv        string
	Headers          []string // repeated "Key: Value"
	RequestFile      string
	RequestJSON      string
	FirstTokenPaths  []string
	TimeoutSec       int
	Repeat           int
	Warmup           int
	NoValidateStream bool
	Insecure         bool
}

// ConfigError is a fatal configuration problem (exit code 1).
type ConfigError struct{ msg string }

func (e *ConfigError) Error() string { return e.msg }
func NewConfigError(format string, args ...interface{}) error {
	return &ConfigError{msg: fmt.Sprintf(format, args...)}
}

// IsConfigError reports whether err is a *ConfigError.
func IsConfigError(err error) bool {
	var ce *ConfigError
	return errors.As(err, &ce)
}

// rawConfig is the JSON config object with case-insensitive key lookup.
type rawConfig struct {
	keys map[string]interface{} // lowercased key -> value
}

func loadRawConfig(path string) (*rawConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, NewConfigError("read config %s: %v", path, err)
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, NewConfigError("parse config %s: %v", path, err)
	}
	rc := &rawConfig{keys: make(map[string]interface{}, len(obj))}
	for k, v := range obj {
		rc.keys[strings.ToLower(k)] = v
	}
	return rc, nil
}

// first returns the first present value among aliases (case-insensitive).
func (rc *rawConfig) first(aliases ...string) (interface{}, bool) {
	for _, a := range aliases {
		if v, ok := rc.keys[strings.ToLower(a)]; ok {
			return v, true
		}
	}
	return nil, false
}

func (rc *rawConfig) string(aliases ...string) (string, bool) {
	v, ok := rc.first(aliases...)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func (rc *rawConfig) boolValue(aliases ...string) (bool, bool) {
	v, ok := rc.first(aliases...)
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// Load reads a config file and applies CLI overrides, producing a resolved Config.
func Load(path string, o *CLIOverrides) (*Config, error) {
	rc, err := loadRawConfig(path)
	if err != nil {
		return nil, err
	}
	c := &Config{ConfigPath: path, VerifySSL: true}

	// Provider (credential namespace only).
	if o.Provider != "" {
		c.Provider = o.Provider
	} else if v, ok := rc.string("provider", "PROVIDER"); ok {
		c.Provider = v
	}

	// API form.
	api := APIAuto
	if o.API != "" {
		api = o.API
	} else if v, ok := rc.string("api", "API"); ok {
		api = v
	}

	// base_url / url / path.
	if o.BaseURL != "" {
		c.BaseURL = o.BaseURL
	} else if v, ok := rc.string("base_url", "BASE_URL"); ok {
		c.BaseURL = v
	}
	if o.URL != "" {
		c.URL = o.URL
	} else if v, ok := rc.string("url", "URL"); ok {
		c.URL = v
	}
	if o.Path != "" {
		c.Path = o.Path
	} else if v, ok := rc.string("path", "PATH", "endpoint", "ENDPOINT"); ok {
		c.Path = v
	}

	// Model (top-level), used by gemini url construction.
	if v, ok := rc.string("model", "MODEL"); ok {
		c.Model = v
	}

	// Request body.
	req, reqRaw, err := resolveRequest(rc, o)
	if err != nil {
		return nil, err
	}
	c.Request = req
	c.RequestRaw = reqRaw

	// Resolve api=auto using url/path/body before url/header construction.
	c.API = ResolveAPI(api, c.URL, c.Path, c.Request)

	// Fill default path from api form if none provided.
	if c.Path == "" && c.URL == "" {
		c.Path = DefaultPath(c.API, c.Model)
	}

	// First token paths.
	paths := append([]string{}, o.FirstTokenPaths...)
	if configured, ok := rc.first("first_token_json_paths", "FIRST_TOKEN_JSON_PATHS", "token_paths"); ok {
		list, err := toStringListOfStrings(configured)
		if err != nil {
			return nil, NewConfigError("first_token_json_paths must be a list of JSON path strings")
		}
		paths = append(paths, list...)
	}
	c.FirstTokenPaths = paths

	// verify_ssl.
	if v, ok := rc.boolValue("verify_ssl"); ok {
		c.VerifySSL = v
	}
	if o.Insecure {
		c.VerifySSL = false
	}

	// Headers.
	hdrs, err := resolveHeaders(rc, o)
	if err != nil {
		return nil, err
	}
	c.Headers = hdrs

	// API key resolution: CLI plaintext, CLI env name, config env name, config plaintext.
	key, err := resolveAPIKey(rc, o)
	if err != nil {
		return nil, err
	}
	c.APIKey = key

	// Add default auth header by api form if user did not provide an equivalent.
	applyAuthHeader(c.Headers, c.API, c.APIKey)

	// Label defaults to provider; compare/runset may override from filename.
	if c.Label == "" {
		c.Label = c.Provider
	}

	return c, nil
}

func resolveRequest(rc *rawConfig, o *CLIOverrides) (map[string]interface{}, json.RawMessage, error) {
	// CLI overrides take precedence.
	if o.RequestFile != "" {
		data, err := os.ReadFile(o.RequestFile)
		if err != nil {
			return nil, nil, NewConfigError("read request file %s: %v", o.RequestFile, err)
		}
		return parseRequest(data)
	}
	if o.RequestJSON != "" {
		return parseRequest([]byte(o.RequestJSON))
	}
	if v, ok := rc.first("request", "REQUEST", "request_json", "REQUEST_JSON", "body", "BODY"); ok {
		switch t := v.(type) {
		case map[string]interface{}:
			raw, _ := canonicalJSON(t)
			return t, raw, nil
		case string:
			// Could be a file path or an inline JSON string.
			if fi, err := os.Stat(t); err == nil && !fi.IsDir() {
				data, err := os.ReadFile(t)
				if err != nil {
					return nil, nil, NewConfigError("read request file %s: %v", t, err)
				}
				return parseRequest(data)
			}
			return parseRequest([]byte(t))
		default:
			return nil, nil, NewConfigError("request must be a JSON object or string")
		}
	}
	if v, ok := rc.string("request_file", "REQUEST_FILE"); ok {
		data, err := os.ReadFile(v)
		if err != nil {
			return nil, nil, NewConfigError("read request file %s: %v", v, err)
		}
		return parseRequest(data)
	}
	return nil, nil, NewConfigError("no request body configured")
}

func parseRequest(data []byte) (map[string]interface{}, json.RawMessage, error) {
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, nil, NewConfigError("parse request JSON: %v", err)
	}
	raw, _ := canonicalJSON(obj)
	return obj, raw, nil
}

// canonicalJSON serializes v with sorted object keys for stable hashing.
func canonicalJSON(v interface{}) (json.RawMessage, error) {
	return json.Marshal(v)
}

func toStringListOfStrings(v interface{}) ([]string, error) {
	list, ok := v.([]interface{})
	if !ok {
		return nil, fmt.Errorf("not a list")
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("non-string item")
		}
		out = append(out, s)
	}
	return out, nil
}

func resolveHeaders(rc *rawConfig, o *CLIOverrides) (map[string]string, error) {
	hdrs := make(map[string]string)
	for k, v := range DefaultHeaders {
		hdrs[k] = v
	}
	if configured, ok := rc.first("headers", "HEADERS"); ok {
		m, ok := configured.(map[string]interface{})
		if !ok {
			return nil, NewConfigError("headers must be a JSON object")
		}
		for k, v := range m {
			hdrs[k] = fmt.Sprintf("%v", v)
		}
	}
	for _, h := range o.Headers {
		name, value, err := splitHeader(h)
		if err != nil {
			return nil, err
		}
		hdrs[name] = value
	}
	return hdrs, nil
}

func splitHeader(h string) (string, string, error) {
	idx := strings.Index(h, ":")
	if idx < 0 {
		return "", "", NewConfigError("invalid --header %q (expected \"Name: Value\")", h)
	}
	name := strings.TrimSpace(h[:idx])
	value := strings.TrimSpace(h[idx+1:])
	if name == "" {
		return "", "", NewConfigError("invalid --header %q (empty name)", h)
	}
	return name, value, nil
}

// resolveAPIKey implements the order:
// 1. CLI plaintext --api-key
// 2. CLI --api-key-env or config api_key_env
// 3. config api_key (with env: prefix support)
func resolveAPIKey(rc *rawConfig, o *CLIOverrides) (string, error) {
	if o.APIKey != "" {
		return o.APIKey, nil
	}
	envName := o.APIKeyEnv
	if envName == "" {
		if v, ok := rc.string("api_key_env", "API_KEY_ENV", "key_env"); ok {
			envName = v
		}
	}
	if envName != "" {
		val := os.Getenv(envName)
		if val == "" {
			return "", NewConfigError("environment variable %s is empty or not set", envName)
		}
		return val, nil
	}
	if v, ok := rc.string("api_key", "API_KEY", "token", "TOKEN"); ok {
		if strings.HasPrefix(v, "env:") {
			envKey := v[4:]
			val := os.Getenv(envKey)
			if val == "" {
				return "", NewConfigError("environment variable %s is empty or not set", envKey)
			}
			return val, nil
		}
		return v, nil
	}
	return "", nil // no key configured; allowed (e.g. public endpoints)
}

// applyAuthHeader adds the default auth header for the api form unless the user
// already provided an equivalent authentication header.
func applyAuthHeader(hdrs map[string]string, api, key string) {
	if key == "" {
		return
	}
	lower := make(map[string]bool, len(hdrs))
	for k := range hdrs {
		lower[strings.ToLower(k)] = true
	}
	switch api {
	case APIAnthropicMessages:
		if !lower["x-api-key"] && !lower["authorization"] {
			hdrs["x-api-key"] = key
		}
	case APIGoogleGenerativeAI:
		if !lower["x-goog-api-key"] && !lower["authorization"] {
			hdrs["x-goog-api-key"] = key
		}
	default: // openai-responses, openai-chat-completions, auto fallback
		if !lower["authorization"] {
			hdrs["Authorization"] = "Bearer " + key
		}
	}
}

// RequestSHA256 returns a stable hex digest of the request body.
func (c *Config) RequestSHA256() string {
	if len(c.RequestRaw) == 0 {
		return ""
	}
	sum := sha256.Sum256(c.RequestRaw)
	return hex.EncodeToString(sum[:])
}

// AbsConfigPath returns the absolute config file path.
func (c *Config) AbsConfigPath() string {
	abs, err := filepath.Abs(c.ConfigPath)
	if err != nil {
		return c.ConfigPath
	}
	return abs
}

// BodyBytes returns the serialized request body sent over the wire.
func (c *Config) BodyBytes() []byte {
	return []byte(c.RequestRaw)
}
