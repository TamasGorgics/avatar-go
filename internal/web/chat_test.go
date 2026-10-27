package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"avatar-go/internal/twin"
)

const validChatBody = `{"history":[{"role":"user","content":"hi"}]}`

func doChat(t *testing.T, srv *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(body))
	req.RemoteAddr = "203.0.113.9:1234"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func TestChatSuccess(t *testing.T) {
	cfg := testConfig()
	lim := &fakeLimiter{allowed: true}
	stream := &fakeStream{deltas: []string{"", "Hel", "lo", ""}}
	chat := &fakeChatter{stream: stream}
	srv := New(cfg, lim, chat, discardLogger())

	rec := doChat(t, srv, validChatBody)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "Hello" {
		t.Fatalf("body = %q, want %q", got, "Hello")
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q", got)
	}
	if got := rec.Header().Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("X-Accel-Buffering = %q", got)
	}
	if !rec.Flushed {
		t.Error("response was not flushed")
	}

	if lim.calls != 1 {
		t.Errorf("limiter calls = %d, want 1", lim.calls)
	}
	if lim.ip != "203.0.113.9" {
		t.Errorf("limiter IP = %q, want %q", lim.ip, "203.0.113.9")
	}
	if lim.perHour != cfg.PerIPHourly || lim.daily != cfg.DailyCap {
		t.Errorf("limiter caps = (%d, %d), want (%d, %d)", lim.perHour, lim.daily, cfg.PerIPHourly, cfg.DailyCap)
	}

	if chat.calls != 1 {
		t.Fatalf("chatter calls = %d, want 1", chat.calls)
	}
	wantHistory := []twin.Message{{Role: "user", Content: "hi"}}
	if !reflect.DeepEqual(chat.history, wantHistory) {
		t.Errorf("history = %+v, want %+v", chat.history, wantHistory)
	}
	if !stream.closed {
		t.Error("stream was not closed")
	}
	if _, ok := chat.ctx.Deadline(); !ok {
		t.Error("chat context has no deadline")
	}
}

func TestChatSanitizesHistory(t *testing.T) {
	long := strings.Repeat("x", maxContentBytes+100)
	in := struct {
		History []chatMessage `json:"history"`
	}{
		History: []chatMessage{
			{Role: "user", Content: "one"},
			{Role: "assistant", Content: "two"},
			{Role: "user", Content: "keep-1"},
			{Role: "system", Content: "ignore"},
			{Role: "assistant", Content: "keep-2"},
			{Role: "user", Content: "keep-3"},
			{Role: "tool", Content: "ignore"},
			{Role: "user", Content: long},
		},
	}
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	chat := &fakeChatter{stream: &fakeStream{}}
	srv := New(testConfig(), &fakeLimiter{allowed: true}, chat, discardLogger())
	rec := doChat(t, srv, string(body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	want := []twin.Message{
		{Role: "user", Content: "keep-1"},
		{Role: "assistant", Content: "keep-2"},
		{Role: "user", Content: "keep-3"},
		{Role: "user", Content: long[:maxContentBytes]},
	}
	if !reflect.DeepEqual(chat.history, want) {
		t.Fatalf("history = %+v, want %+v", chat.history, want)
	}
}

func TestChatRateLimited(t *testing.T) {
	lim := &fakeLimiter{allowed: false}
	chat := &fakeChatter{stream: &fakeStream{}}
	srv := New(testConfig(), lim, chat, discardLogger())

	// Invalid JSON proves the limiter runs before body parsing.
	rec := doChat(t, srv, "not json")

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if got := rec.Body.String(); got != "Rate limit reached, please try again later.\n" {
		t.Fatalf("body = %q", got)
	}
	if lim.calls != 1 {
		t.Errorf("limiter calls = %d, want 1", lim.calls)
	}
	if chat.calls != 0 {
		t.Errorf("chatter calls = %d, want 0", chat.calls)
	}
}

func TestChatBadRequest(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty", body: ""},
		{name: "invalid json", body: "not json"},
		{name: "no history", body: `{}`},
		{name: "empty history", body: `{"history":[]}`},
		{name: "null history", body: `{"history":null}`},
		{name: "wrong history type", body: `{"history":"nope"}`},
		{name: "too large", body: `{"history":[{"role":"user","content":"` + strings.Repeat("x", 20<<10) + `"}]}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chat := &fakeChatter{stream: &fakeStream{}}
			srv := New(testConfig(), &fakeLimiter{allowed: true}, chat, discardLogger())
			rec := doChat(t, srv, tc.body)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			if chat.calls != 0 {
				t.Errorf("chatter calls = %d, want 0", chat.calls)
			}
		})
	}
}

func TestChatUpstreamError(t *testing.T) {
	chat := &fakeChatter{err: errors.New("upstream down")}
	srv := New(testConfig(), &fakeLimiter{allowed: true}, chat, discardLogger())

	rec := doChat(t, srv, validChatBody)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if got := rec.Body.String(); got != unavailableMsg+"\n" {
		t.Fatalf("body = %q, want %q", got, unavailableMsg+"\n")
	}
	if chat.calls != 1 {
		t.Errorf("chatter calls = %d, want 1", chat.calls)
	}
}

func TestChatStreamErrorBeforeOutput(t *testing.T) {
	stream := &fakeStream{err: errors.New("stream down")}
	chat := &fakeChatter{stream: stream}
	srv := New(testConfig(), &fakeLimiter{allowed: true}, chat, discardLogger())

	rec := doChat(t, srv, validChatBody)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if got := rec.Body.String(); got != unavailableMsg+"\n" {
		t.Fatalf("body = %q, want %q", got, unavailableMsg+"\n")
	}
	if !stream.closed {
		t.Error("stream was not closed")
	}
}

func TestChatStreamErrorAfterOutput(t *testing.T) {
	stream := &fakeStream{deltas: []string{"partial"}, err: errors.New("stream broke")}
	chat := &fakeChatter{stream: stream}
	srv := New(testConfig(), &fakeLimiter{allowed: true}, chat, discardLogger())

	rec := doChat(t, srv, validChatBody)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "partial" {
		t.Fatalf("body = %q, want %q", got, "partial")
	}
	if strings.Contains(rec.Body.String(), unavailableMsg) {
		t.Error("error message was appended after partial output")
	}
	if !stream.closed {
		t.Error("stream was not closed")
	}
}

func TestChatUsesForwardedIPWhenTrusted(t *testing.T) {
	cfg := testConfig()
	cfg.TrustProxy = true
	lim := &fakeLimiter{allowed: true}
	srv := New(cfg, lim, &fakeChatter{stream: &fakeStream{}}, discardLogger())

	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(validChatBody))
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.1")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if lim.ip != "198.51.100.7" {
		t.Fatalf("limiter IP = %q, want %q", lim.ip, "198.51.100.7")
	}
}

type noFlushWriter struct {
	header http.Header
	body   strings.Builder
	status int
}

func (w *noFlushWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *noFlushWriter) Write(p []byte) (int, error) { return w.body.Write(p) }

func (w *noFlushWriter) WriteHeader(status int) { w.status = status }

func TestWriteStreamWithoutFlusher(t *testing.T) {
	w := &noFlushWriter{}
	stream := &fakeStream{deltas: []string{"", "hi"}}
	srv := New(testConfig(), &fakeLimiter{allowed: true}, &fakeChatter{}, discardLogger())

	srv.writeStream(w, stream)

	if got := w.body.String(); got != "hi" {
		t.Fatalf("body = %q, want %q", got, "hi")
	}
	if got := w.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		trustProxy bool
		want       string
	}{
		{name: "remote address", remoteAddr: "203.0.113.9:1234", want: "203.0.113.9"},
		{name: "xff ignored", remoteAddr: "203.0.113.9:1234", xff: "198.51.100.7", want: "203.0.113.9"},
		{name: "xff trusted", remoteAddr: "203.0.113.9:1234", xff: "198.51.100.7, 10.0.0.1", trustProxy: true, want: "198.51.100.7"},
		{name: "xff trimmed", remoteAddr: "203.0.113.9:1234", xff: " 198.51.100.7 ", trustProxy: true, want: "198.51.100.7"},
		{name: "trusted without xff", remoteAddr: "203.0.113.9:1234", trustProxy: true, want: "203.0.113.9"},
		{name: "ipv6", remoteAddr: "[2001:db8::1]:443", want: "2001:db8::1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := clientIP(req, tc.trustProxy); got != tc.want {
				t.Fatalf("clientIP() = %q, want %q", got, tc.want)
			}
		})
	}
}
