// Package probe performs the cable-level TTFT measurement: it issues the HTTP
// request, reads the raw response body in chunks, incrementally parses SSE or
// NDJSON, and records headers_ms / ttfb_ms / first_event_ms / ttft_ms.
package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/firsttok/firsttok/internal/api"
	"github.com/firsttok/firsttok/internal/result"
)

// SSEEvent is one parsed server-sent event (or NDJSON line).
type SSEEvent struct {
	EventType   string
	Data        string
	CompletedAt time.Time
}

// SSEParser incrementally parses an SSE/NDJSON byte stream.
type SSEParser struct {
	line      []byte
	eventType string
	dataLines []string
}

// FeedByte processes one byte, returning any events that completed at now.
func (p *SSEParser) FeedByte(b byte, now time.Time) []SSEEvent {
	if b == '\n' {
		return p.processLine(now)
	}
	p.line = append(p.line, b)
	return nil
}

func (p *SSEParser) processLine(now time.Time) []SSEEvent {
	raw := p.line
	p.line = p.line[:0]
	if len(raw) > 0 && raw[len(raw)-1] == '\r' {
		raw = raw[:len(raw)-1]
	}
	line := string(raw)
	if line == "" {
		if ev := p.dispatch(now); ev != nil {
			return []SSEEvent{*ev}
		}
		return nil
	}
	stripped := strings.TrimSpace(line)
	if stripped == "[DONE]" || strings.HasPrefix(stripped, "{") || strings.HasPrefix(stripped, "[") {
		return []SSEEvent{{EventType: "", Data: stripped, CompletedAt: now}}
	}
	if strings.HasPrefix(line, ":") {
		return nil
	}
	idx := strings.Index(line, ":")
	if idx < 0 {
		return nil
	}
	field := line[:idx]
	value := line[idx+1:]
	if strings.HasPrefix(value, " ") {
		value = value[1:]
	}
	switch field {
	case "event":
		p.eventType = value
	case "data":
		p.dataLines = append(p.dataLines, value)
	}
	return nil
}

func (p *SSEParser) dispatch(now time.Time) *SSEEvent {
	if len(p.dataLines) == 0 {
		p.eventType = ""
		return nil
	}
	ev := &SSEEvent{
		EventType:   p.eventType,
		Data:        strings.Join(p.dataLines, "\n"),
		CompletedAt: now,
	}
	p.eventType = ""
	p.dataLines = p.dataLines[:0]
	return ev
}

// Options configures one probe attempt.
type Options struct {
	URL         string
	Headers     map[string]string
	Body        []byte
	API         string
	CustomPaths []string
	VerifySSL   bool
	Timeout     time.Duration
}

// Run executes a single probe and returns a populated result.Single.
// runIndex/warmup identify the attempt; provider/api are recorded for reporting.
func Run(opts Options, runIndex int, warmup bool, provider, apiForm string) result.Single {
	startedAt := result.StartedAtNow()
	res := result.Single{
		Run:       runIndex,
		Warmup:    warmup,
		Provider:  provider,
		API:       apiForm,
		URL:       result.RedactURL(opts.URL),
		StartedAt: startedAt,
	}

	ctx := context.Background()
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, opts.URL, bytes.NewReader(opts.Body))
	if err != nil {
		res.Error = fmt.Sprintf("build request: %v", err)
		return res
	}
	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{
		Transport: newTransport(opts.VerifySSL),
		Timeout:   opts.Timeout,
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		res.Error = fmt.Sprintf("request: %v", err)
		return res
	}
	headersAt := time.Now()
	res.Status = resp.StatusCode
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Non-2xx: read a short error preview.
		body := make([]byte, 2048)
		n, _ := resp.Body.Read(body)
		preview := string(body[:n])
		res.Error = result.PreviewText(preview, 2048)
		return res
	}

	parser := &SSEParser{}
	var firstByteAt, firstEventAt, ttftAt time.Time
	var eventsRead int
	var bytesRead int64
	var firstToken *result.FirstToken
	done := false

	buf := make([]byte, 65536)
	for !done {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			now := time.Now()
			if firstByteAt.IsZero() {
				firstByteAt = now
			}
			bytesRead += int64(n)
			chunk := buf[:n]
			for i := 0; i < len(chunk); i++ {
				events := parser.FeedByte(chunk[i], now)
				for _, ev := range events {
					eventsRead++
					if firstEventAt.IsZero() {
						firstEventAt = ev.CompletedAt
					}
					if firstToken == nil {
						data := parseJSON(ev.Data)
						if match, ok := api.ExtractToken(apiForm, ev.EventType, data, ev.Data, opts.CustomPaths); ok {
							firstToken = &result.FirstToken{
								Source:  match.Source,
								Preview: result.PreviewText(match.Text, 80),
							}
							ttftAt = ev.CompletedAt
							done = true
						}
					}
				}
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			if firstToken == nil {
				res.Error = fmt.Sprintf("read body: %v", rerr)
				return res
			}
			break
		}
	}

	if firstToken == nil {
		res.Error = "no first token"
		res.EventsRead = eventsRead
		res.BytesRead = bytesRead
		return res
	}

	res.OK = true
	res.HeadersMS = ms(start, headersAt)
	res.TTFBMS = ms(start, firstByteAt)
	res.FirstEventMS = ms(start, firstEventAt)
	res.TTFTMS = ms(start, ttftAt)
	res.EventsRead = eventsRead
	res.BytesRead = bytesRead
	res.FirstToken = firstToken
	return res
}

// parseJSON best-effort parses an SSE data payload as JSON; returns nil if not
// a JSON object/array.
func parseJSON(data string) interface{} {
	s := strings.TrimSpace(data)
	if s == "" || s == "[DONE]" {
		return nil
	}
	if !(strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[")) {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil
	}
	return v
}

func ms(start, end time.Time) *float64 {
	if end.IsZero() {
		return nil
	}
	v := float64(end.Sub(start).Nanoseconds()) / 1e6
	r := math.Round(v*1000) / 1000
	if r == 0 {
		r = 0.0
	}
	return &r
}
