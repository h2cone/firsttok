package probe

import (
	"testing"
	"time"
)

func newParser() *SSEParser { return &SSEParser{} }

// feedString feeds bytes of s and returns all completed events.
func feedString(p *SSEParser, s string) []SSEEvent {
	var events []SSEEvent
	now := time.Now()
	for i := 0; i < len(s); i++ {
		events = append(events, p.FeedByte(s[i], now)...)
	}
	return events
}

func collectEvents(s string) []SSEEvent {
	return feedString(newParser(), s)
}

func TestSSEMultilineData(t *testing.T) {
	events := collectEvents("event: foo\ndata: line1\ndata: line2\n\n")
	if len(events) != 1 {
		t.Fatalf("got %d events", len(events))
	}
	if events[0].EventType != "foo" {
		t.Errorf("event type = %q", events[0].EventType)
	}
	if events[0].Data != "line1\nline2" {
		t.Errorf("data = %q", events[0].Data)
	}
}

func TestSSECommentAndDone(t *testing.T) {
	events := collectEvents(": this is a comment\nevent: x\ndata: {\"a\":1}\n\ndata: [DONE]\n\n")
	// comment skipped; first event {a:1}; [DONE] is a standalone event.
	if len(events) != 2 {
		t.Fatalf("got %d events: %+v", len(events), events)
	}
	if events[0].Data != `{"a":1}` {
		t.Errorf("first data = %q", events[0].Data)
	}
	if events[1].Data != "[DONE]" {
		t.Errorf("done data = %q", events[1].Data)
	}
}

func TestSSECRLF(t *testing.T) {
	events := collectEvents("data: hello\r\n\r\n")
	if len(events) != 1 || events[0].Data != "hello" {
		t.Fatalf("CRLF handling failed: %+v", events)
	}
}

func TestNDJSON(t *testing.T) {
	events := collectBytes([]byte(`{"a":1}` + "\n" + `{"a":2}` + "\n"))
	if len(events) != 2 {
		t.Fatalf("got %d events", len(events))
	}
	if events[0].Data != `{"a":1}` || events[1].Data != `{"a":2}` {
		t.Errorf("ndjson = %+v", events)
	}
}

func collectBytes(b []byte) []SSEEvent {
	p := newParser()
	var events []SSEEvent
	now := time.Now()
	for _, x := range b {
		events = append(events, p.FeedByte(x, now)...)
	}
	return events
}

func TestParseJSONHelper(t *testing.T) {
	if parseJSON("[DONE]") != nil {
		t.Error("[DONE] should parse nil")
	}
	if parseJSON("not json") != nil {
		t.Error("non-json should parse nil")
	}
	v := parseJSON(`{"a":1}`)
	if v == nil {
		t.Error("json object should parse")
	}
}
