package config

import (
	"net/url"
	"strings"
)

// ResolveAPI resolves api=auto using the url/path/body, otherwise validates
// and returns the explicit api form.
func ResolveAPI(api, explicitURL, path string, body map[string]interface{}) string {
	if api != APIAuto {
		return normalizeAPI(api)
	}
	return inferAPI(explicitURL, path, body)
}

func normalizeAPI(api string) string {
	switch strings.ToLower(api) {
	case APIOpenAIResponses:
		return APIOpenAIResponses
	case APIOpenAIChat:
		return APIOpenAIChat
	case APIAnthropicMessages:
		return APIAnthropicMessages
	case APIGoogleGenerativeAI:
		return APIGoogleGenerativeAI
	case APIAuto:
		return APIAuto
	default:
		return api
	}
}

// inferAPI decides the api form from url, path, and request body.
func inferAPI(explicitURL, path string, body map[string]interface{}) string {
	combined := strings.ToLower(explicitURL + " " + path)
	switch {
	case strings.Contains(combined, "/v1/messages"):
		return APIAnthropicMessages
	case strings.Contains(combined, "/v1/responses"):
		return APIOpenAIResponses
	case strings.Contains(combined, "/v1/chat/completions"):
		return APIOpenAIChat
	case strings.Contains(combined, "streamgeneratecontent"),
		strings.Contains(combined, "generativelanguage"),
		strings.Contains(combined, ":generatecontent"):
		return APIGoogleGenerativeAI
	}
	// Body heuristics.
	if body != nil {
		if _, hasContents := body["contents"]; hasContents {
			return APIGoogleGenerativeAI
		}
	}
	// Fall back to first-json-event semantics; signal that with the auto marker
	// so token extraction knows there are no api-specific rules.
	return APIAuto
}

// DefaultPath returns the default request path for an api form.
func DefaultPath(api, model string) string {
	switch api {
	case APIOpenAIResponses:
		return "/v1/responses"
	case APIOpenAIChat:
		return "/v1/chat/completions"
	case APIAnthropicMessages:
		return "/v1/messages"
	case APIGoogleGenerativeAI:
		m := model
		if m == "" {
			m = "{model}"
		}
		return "/v1beta/models/" + m + ":streamGenerateContent?alt=sse"
	}
	return ""
}

// RequiresStreamTrue reports whether the api form requires stream=true in the
// request body (Gemini streaming is controlled by the URL, not the body).
func RequiresStreamTrue(api string) bool {
	switch api {
	case APIOpenAIResponses, APIOpenAIChat, APIAnthropicMessages:
		return true
	}
	return false
}

// ValidateStream checks that stream=true is set when required. Use
// noValidate to bypass for non-standard proxies.
func ValidateStream(api string, body map[string]interface{}, noValidate bool) error {
	if noValidate || !RequiresStreamTrue(api) {
		return nil
	}
	if body == nil {
		return NewConfigError("%s request JSON must set stream=true for TTFT measurement (use --no-validate-stream to bypass)", api)
	}
	v, ok := body["stream"]
	if !ok || v != true {
		return NewConfigError("%s request JSON must set stream=true for TTFT measurement (use --no-validate-stream to bypass)", api)
	}
	return nil
}

// BuildURL constructs the final request URL from explicit url, base_url and path.
// Priority: explicit url (optionally joined with base_url if it has no scheme),
// then base_url + path, then error.
func BuildURL(c *Config) (string, error) {
	if c.URL != "" {
		if c.BaseURL != "" && !hasScheme(c.URL) {
			return joinBaseAndPath(c.BaseURL, c.URL), nil
		}
		return c.URL, nil
	}
	if c.BaseURL != "" && c.Path != "" {
		return joinBaseAndPath(c.BaseURL, c.Path), nil
	}
	if c.BaseURL != "" && c.Path == "" {
		// Default path already filled by Load; if still empty, error.
		return joinBaseAndPath(c.BaseURL, DefaultPath(c.API, c.Model)), nil
	}
	return "", NewConfigError("cannot build request url: set url, or base_url + path")
}

func hasScheme(s string) bool {
	if u, err := url.Parse(s); err == nil && u.Scheme != "" {
		return true
	}
	return strings.Contains(s, "://")
}

// joinBaseAndPath joins a base url and a path, normalizing the boundary slash.
func joinBaseAndPath(base, path string) string {
	base = strings.TrimRight(base, "/") + "/"
	p := strings.TrimLeft(path, "/")
	return base + p
}
