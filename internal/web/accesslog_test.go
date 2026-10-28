package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestLog(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))
	srv := New(testConfig(), &fakeLimiter{allowed: true}, &fakeChatter{
		stream: &fakeStream{deltas: []string{"ok"}},
	}, logger)

	rec := doChat(t, srv, validChatBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	requestID := rec.Header().Get(requestIDHeader)
	if requestID == "" {
		t.Fatal("X-Request-ID header is missing")
	}

	entry := findLog(t, logs.Bytes(), "request")
	if entry["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", entry["level"])
	}
	if entry["method"] != "POST" {
		t.Errorf("method = %v, want POST", entry["method"])
	}
	if entry["path"] != "/chat" {
		t.Errorf("path = %v, want /chat", entry["path"])
	}
	if entry["request_id"] != requestID {
		t.Errorf("request_id = %v, want %q", entry["request_id"], requestID)
	}
	if entry["status"] != float64(http.StatusOK) {
		t.Errorf("status = %v, want %d", entry["status"], http.StatusOK)
	}
	if entry["bytes"] != float64(2) {
		t.Errorf("bytes = %v, want 2", entry["bytes"])
	}
	if entry["remote_ip"] != "203.0.113.9" {
		t.Errorf("remote_ip = %v, want 203.0.113.9", entry["remote_ip"])
	}
	if _, ok := entry["duration"]; !ok {
		t.Error("duration is missing")
	}
	if _, ok := entry["source"]; !ok {
		t.Error("source is missing")
	}
}

func TestRequestLogLevels(t *testing.T) {
	tests := []struct {
		name       string
		limiter    *fakeLimiter
		chatter    *fakeChatter
		wantStatus int
		wantLevel  string
	}{
		{
			name:       "client error",
			limiter:    &fakeLimiter{allowed: false},
			chatter:    &fakeChatter{},
			wantStatus: http.StatusTooManyRequests,
			wantLevel:  "WARN",
		},
		{
			name:       "server error",
			limiter:    &fakeLimiter{allowed: true},
			chatter:    &fakeChatter{err: errors.New("upstream down")},
			wantStatus: http.StatusBadGateway,
			wantLevel:  "ERROR",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			srv := New(testConfig(), tc.limiter, tc.chatter, logger)

			rec := doChat(t, srv, validChatBody)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}

			entry := findLog(t, logs.Bytes(), "request")
			if entry["level"] != tc.wantLevel {
				t.Errorf("level = %v, want %s", entry["level"], tc.wantLevel)
			}
			if entry["status"] != float64(tc.wantStatus) {
				t.Errorf("status = %v, want %d", entry["status"], tc.wantStatus)
			}
		})
	}
}

func TestStatusWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec}

	if _, err := sw.Write([]byte("hi")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if sw.status != http.StatusOK || sw.bytes != 2 {
		t.Fatalf("status, bytes = %d, %d; want %d, 2", sw.status, sw.bytes, http.StatusOK)
	}

	sw.WriteHeader(http.StatusTeapot)
	if sw.status != http.StatusOK {
		t.Fatalf("status changed after first write: %d", sw.status)
	}

	sw.Flush()
	if !rec.Flushed {
		t.Error("Flush was not forwarded")
	}
	if sw.Unwrap() != rec {
		t.Error("Unwrap did not return the wrapped writer")
	}
}

func TestStatusWriterDefaultStatus(t *testing.T) {
	sw := &statusWriter{ResponseWriter: httptest.NewRecorder()}
	sw.WriteHeader(http.StatusTeapot)

	if sw.status != http.StatusTeapot {
		t.Fatalf("status = %d, want %d", sw.status, http.StatusTeapot)
	}
}

func TestStatusWriterIgnoresInformationalStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec}
	sw.WriteHeader(http.StatusEarlyHints)
	sw.WriteHeader(http.StatusOK)

	if sw.status != http.StatusOK {
		t.Fatalf("status = %d, want %d", sw.status, http.StatusOK)
	}
}

func findLog(t *testing.T, data []byte, msg string) map[string]any {
	t.Helper()
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("unmarshal log line: %v\n%s", err, line)
		}
		if entry["msg"] == msg {
			return entry
		}
	}
	t.Fatalf("no %q log entry in:\n%s", msg, data)
	return nil
}
