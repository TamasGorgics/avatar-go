package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutes(t *testing.T) {
	srv := New(testConfig(), &fakeLimiter{allowed: true}, &fakeChatter{stream: &fakeStream{}})

	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{name: "page", method: http.MethodGet, path: "/", want: http.StatusOK},
		{name: "chat method", method: http.MethodGet, path: "/chat", want: http.StatusMethodNotAllowed},
		{name: "page method", method: http.MethodPost, path: "/", want: http.StatusMethodNotAllowed},
		{name: "unknown", method: http.MethodGet, path: "/missing", want: http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestPage(t *testing.T) {
	srv := New(testConfig(), &fakeLimiter{allowed: true}, &fakeChatter{stream: &fakeStream{}})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); !strings.Contains(body, "<title>Ask my digital twin</title>") {
		t.Fatalf("page body missing title:\n%s", body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", ct)
	}
}
