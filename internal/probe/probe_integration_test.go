package probe

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// streamingHandler writes an SSE stream with a small delay so timing edges are
// observable, recording the request headers for assertions.
func streamingHandler(t *testing.T, chunks []string, captured *http.Header) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if captured != nil {
			*captured = r.Header.Clone()
		}
		// Drain request body.
		io.Copy(io.Discard, r.Body)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not support flushing")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
		for _, c := range chunks {
			time.Sleep(20 * time.Millisecond)
			io.WriteString(w, c)
			flusher.Flush()
		}
	}
}

func TestRunProbeOpenAIChat(t *testing.T) {
	var hdr http.Header
	srv := httptest.NewServer(streamingHandler(t, []string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n",
		"data: [DONE]\n\n",
	}, &hdr))
	defer srv.Close()

	opts := Options{
		URL: srv.URL + "/v1/chat/completions",
		Headers: map[string]string{
			"Accept":          "text/event-stream",
			"Accept-Encoding": "identity",
			"Cache-Control":   "no-cache",
			"Content-Type":    "application/json",
		},
		Body:    []byte(`{"model":"gpt","stream":true}`),
		API:     "openai-chat-completions",
		Timeout: 5 * time.Second,
	}
	res := Run(opts, 1, false, "openai", "openai-chat-completions")
	if !res.OK {
		t.Fatalf("probe failed: %s", res.Error)
	}
	if res.TTFTMS == nil {
		t.Fatal("ttft not measured")
	}
	if res.FirstToken == nil || res.FirstToken.Source != "choices.*.delta.content" {
		t.Errorf("first token source = %+v", res.FirstToken)
	}
	if res.FirstToken != nil && !strings.Contains(res.FirstToken.Preview, "Hello") {
		t.Errorf("preview = %q", res.FirstToken.Preview)
	}
	// Timing ordering: headers <= ttfb <= first_event <= ttft.
	if res.HeadersMS == nil || res.TTFBMS == nil || res.FirstEventMS == nil {
		t.Fatalf("timing missing: %+v", res)
	}
	if *res.HeadersMS > *res.TTFBMS {
		t.Errorf("headers_ms (%v) > ttfb_ms (%v)", *res.HeadersMS, *res.TTFBMS)
	}
	if *res.TTFBMS > *res.FirstEventMS {
		t.Errorf("ttfb_ms (%v) > first_event_ms (%v)", *res.TTFBMS, *res.FirstEventMS)
	}
	if *res.FirstEventMS > *res.TTFTMS {
		t.Errorf("first_event_ms (%v) > ttft_ms (%v)", *res.FirstEventMS, *res.TTFTMS)
	}
	// Compression disabled: client must send Accept-Encoding: identity.
	if got := hdr.Get("Accept-Encoding"); got != "identity" {
		t.Errorf("Accept-Encoding = %q, want identity", got)
	}
}

func TestRunProbeNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":"bad key"}`)
	}))
	defer srv.Close()
	opts := Options{
		URL:     srv.URL,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    []byte(`{}`),
		API:     "openai-chat-completions",
		Timeout: 5 * time.Second,
	}
	res := Run(opts, 1, false, "openai", "openai-chat-completions")
	if res.OK {
		t.Error("expected failure on 401")
	}
	if res.Status != 401 {
		t.Errorf("status = %d", res.Status)
	}
	if !strings.Contains(res.Error, "bad key") {
		t.Errorf("error preview = %q", res.Error)
	}
}

func TestRunProbeNoToken(t *testing.T) {
	srv := httptest.NewServer(streamingHandler(t, []string{
		"data: {\"choices\":[]}\n\n",
	}, nil))
	defer srv.Close()
	opts := Options{
		URL:     srv.URL,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    []byte(`{}`),
		API:     "openai-chat-completions",
		Timeout: 5 * time.Second,
	}
	res := Run(opts, 1, false, "openai", "openai-chat-completions")
	if res.OK {
		t.Error("expected no-token failure")
	}
	if res.Error != "no first token" {
		t.Errorf("error = %q", res.Error)
	}
}
