package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewUsesDefaultLogger(t *testing.T) {
	srv := New(testConfig(), &fakeLimiter{allowed: true}, &fakeChatter{}, nil)
	if srv.log == nil {
		t.Fatal("logger is nil")
	}
}

func TestRoutes(t *testing.T) {
	srv := New(testConfig(), &fakeLimiter{allowed: true}, &fakeChatter{stream: &fakeStream{}}, discardLogger())

	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{name: "page", method: http.MethodGet, path: "/", want: http.StatusOK},
		{name: "liveness", method: http.MethodGet, path: "/healthz", want: http.StatusOK},
		{name: "readiness", method: http.MethodGet, path: "/readyz", want: http.StatusOK},
		{name: "metrics", method: http.MethodGet, path: "/metrics", want: http.StatusOK},
		{name: "chat method", method: http.MethodGet, path: "/chat", want: http.StatusMethodNotAllowed},
		{name: "page method", method: http.MethodPost, path: "/", want: http.StatusMethodNotAllowed},
		{name: "asset method", method: http.MethodPost, path: "/assets/app.css", want: http.StatusMethodNotAllowed},
		{name: "unknown", method: http.MethodGet, path: "/missing", want: http.StatusNotFound},
		{name: "unknown asset", method: http.MethodGet, path: "/assets/missing.css", want: http.StatusNotFound},
		{name: "asset directory", method: http.MethodGet, path: "/assets/", want: http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if rec.Header().Get(requestIDHeader) == "" {
				t.Error("X-Request-ID header is missing")
			}
		})
	}
}

func TestHealthEndpointsAndReadiness(t *testing.T) {
	srv := New(testConfig(), &fakeLimiter{allowed: true}, &fakeChatter{}, discardLogger())

	for _, tc := range []struct {
		path string
		code int
		body string
	}{
		{path: "/healthz", code: http.StatusOK, body: "ok\n"},
		{path: "/readyz", code: http.StatusOK, body: "ready\n"},
	} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != tc.code || rec.Body.String() != tc.body {
			t.Errorf("GET %s = (%d, %q), want (%d, %q)", tc.path, rec.Code, rec.Body.String(), tc.code, tc.body)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("GET %s Cache-Control = %q, want no-store", tc.path, got)
		}
	}

	srv.SetReady(false)
	health := httptest.NewRecorder()
	srv.Handler().ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Errorf("liveness while draining = %d, want %d", health.Code, http.StatusOK)
	}
	ready := httptest.NewRecorder()
	srv.Handler().ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusServiceUnavailable {
		t.Errorf("readiness while draining = %d, want %d", ready.Code, http.StatusServiceUnavailable)
	}
}

func TestReadinessRequiresAPIKey(t *testing.T) {
	cfg := testConfig()
	cfg.APIKey = ""
	srv := New(cfg, &fakeLimiter{allowed: true}, &fakeChatter{}, discardLogger())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestRequestIDsAreGeneratedForErrorsAndIgnoreIncomingHeader(t *testing.T) {
	srv := New(testConfig(), &fakeLimiter{allowed: true}, &fakeChatter{}, discardLogger())
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.Header.Set(requestIDHeader, "visitor-controlled-id")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	id := rec.Header().Get(requestIDHeader)
	if id == "" || id == "visitor-controlled-id" {
		t.Fatalf("X-Request-ID = %q, want a server-generated value", id)
	}
}

func TestPage(t *testing.T) {
	srv := New(testConfig(), &fakeLimiter{allowed: true}, &fakeChatter{stream: &fakeStream{}}, discardLogger())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/html; charset=utf-8", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", got)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"<!doctype html>",
		"<title>Tamas’ Digital Twin</title>",
		`<link rel="icon" type="image/svg+xml" href="/assets/favicon.svg">`,
		`<link rel="stylesheet" href="/assets/app.css">`,
		`<script src="/assets/app.js" defer></script>`,
		`src="/assets/portrait.jpg"`,
		`alt="Portrait of Tamas Gorgics"`,
		`id="theme-toggle"`,
		`role="switch"`,
		`aria-checked="false"`,
		`aria-label="Dark mode"`,
		`id="chat-log"`,
		`id="chat-form"`,
		`id="chat-input"`,
		`id="chat-clear"`,
		`I’m Tamas’ digital twin`,
		`Ask me about my work`,
		`Ask about my work`,
		`Projects, experience, and how I work.`,
		`data-role="actions"`,
		`data-role="note"`,
		`data-action="stop"`,
		`data-action="retry"`,
		`data-action="edit"`,
		`data-action="copy"`,
		`data-action="regenerate"`,
		`data-action="edit-last"`,
		`data-prompt="What have you built with Go?"`,
		`href="https://www.linkedin.com/in/tamasgorgics/"`,
		`target="_blank"`,
		`rel="noopener noreferrer"`,
		`aria-label="Tamas Gorgics on LinkedIn"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page body missing %q", want)
		}
	}
	if strings.Contains(body, "Built with Go, Templ, and Tailwind CSS.") {
		t.Error("page body still contains the old footer credit")
	}
	for _, stale := range []string{
		"Digital twin</p><h1",
		"Ask my digital twin",
		"Grounded in my notes",
		"A conversation, not a search box",
		"Answers come only from my CV and notes.",
	} {
		if strings.Contains(body, stale) {
			t.Errorf("page body still contains old copy %q", stale)
		}
	}
	if strings.Contains(body, "<style") {
		t.Error("page body contains an inline style block")
	}
	if got := strings.Count(body, "<script>"); got != 1 {
		t.Errorf("inline script count = %d, want 1 (theme bootstrap)", got)
	}
	bootstrap := strings.Index(body, "prefers-color-scheme")
	stylesheet := strings.Index(body, `<link rel="stylesheet" href="/assets/app.css">`)
	if bootstrap == -1 || stylesheet == -1 || bootstrap > stylesheet {
		t.Error("theme bootstrap must run before the stylesheet")
	}
}

func TestPageRenderFailure(t *testing.T) {
	srv := New(testConfig(), &fakeLimiter{allowed: true}, &fakeChatter{stream: &fakeStream{}}, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestSecurityHeaders(t *testing.T) {
	srv := New(testConfig(), &fakeLimiter{allowed: true}, &fakeChatter{stream: &fakeStream{}}, discardLogger())

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "page", method: http.MethodGet, path: "/"},
		{name: "asset", method: http.MethodGet, path: "/assets/app.css"},
		{name: "liveness", method: http.MethodGet, path: "/healthz"},
		{name: "readiness", method: http.MethodGet, path: "/readyz"},
		{name: "metrics", method: http.MethodGet, path: "/metrics"},
		{name: "chat", method: http.MethodPost, path: "/chat", body: validChatBody},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var body *strings.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			} else {
				body = strings.NewReader("")
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			if tc.path == "/chat" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q", got)
			}
			if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
				t.Errorf("X-Frame-Options = %q", got)
			}
			if got := rec.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
				t.Errorf("Referrer-Policy = %q", got)
			}
			if got := rec.Header().Get("Permissions-Policy"); got != "camera=(), microphone=(), geolocation=()" {
				t.Errorf("Permissions-Policy = %q", got)
			}
			csp := rec.Header().Get("Content-Security-Policy")
			for _, want := range []string{"default-src 'self'", "frame-ancestors 'none'", "script-src 'self' 'unsafe-inline'"} {
				if !strings.Contains(csp, want) {
					t.Errorf("Content-Security-Policy missing %q: %q", want, csp)
				}
			}
		})
	}
}

func TestAssets(t *testing.T) {
	srv := New(testConfig(), &fakeLimiter{allowed: true}, &fakeChatter{stream: &fakeStream{}}, discardLogger())

	tests := []struct {
		name        string
		path        string
		contentType string
		contains    string
	}{
		{name: "css", path: "/assets/app.css", contentType: "text/css", contains: "--color-canvas"},
		{name: "javascript", path: "/assets/app.js", contentType: "javascript", contains: "chat-log"},
		{name: "favicon", path: "/assets/favicon.svg", contentType: "svg", contains: "TG"},
		{name: "portrait", path: "/assets/portrait.jpg", contentType: "image/jpeg", contains: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Header().Get("Content-Type"); !strings.Contains(got, tc.contentType) {
				t.Fatalf("Content-Type = %q, want it to contain %q", got, tc.contentType)
			}
			if got := rec.Header().Get("Cache-Control"); got != "public, max-age=300" {
				t.Fatalf("Cache-Control = %q, want public, max-age=300", got)
			}
			if body := rec.Body.String(); tc.contains != "" && !strings.Contains(body, tc.contains) {
				t.Fatalf("asset body missing %q", tc.contains)
			}
		})
	}
}
