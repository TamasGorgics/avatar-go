package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestChatMetricsCaptureReportedUsageAndTiming(t *testing.T) {
	stream := &fakeStream{
		deltas:           []string{"", "hello"},
		promptTokens:     20,
		completionTokens: 5,
		tokens:           25,
		usageKnown:       true,
	}
	srv := New(testConfig(), &fakeLimiter{allowed: true}, &fakeChatter{
		stream:        stream,
		tokenEstimate: 100,
	}, discardLogger())

	rec := doChat(t, srv, validChatBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	assertCounter(t, srv.metrics.requests.WithLabelValues("POST", "/chat", "200"), 1)
	assertCounter(t, srv.metrics.promptTokens.WithLabelValues("reported"), 20)
	assertCounter(t, srv.metrics.completionTokens.WithLabelValues("reported"), 5)
	assertCounter(t, srv.metrics.totalTokens.WithLabelValues("reported"), 25)
	assertHistogramCount(t, srv.metrics.registry, "avatar_upstream_start_duration_seconds", 1)
	assertHistogramCount(t, srv.metrics.registry, "avatar_chat_stream_duration_seconds", 1)
	assertHistogramCount(t, srv.metrics.registry, "avatar_chat_time_to_first_token_seconds", 1)

	metrics := httptest.NewRecorder()
	srv.Handler().ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	for _, name := range []string{
		"avatar_prompt_tokens_total",
		"avatar_completion_tokens_total",
		"avatar_tokens_total",
	} {
		if !strings.Contains(metrics.Body.String(), name) {
			t.Errorf("metrics output is missing %q", name)
		}
	}
}

func TestChatMetricsUseEstimateWhenUsageIsMissing(t *testing.T) {
	srv := New(testConfig(), &fakeLimiter{allowed: true}, &fakeChatter{
		stream:        &fakeStream{deltas: []string{"answer"}},
		tokenEstimate: 123,
	}, discardLogger())

	rec := doChat(t, srv, validChatBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	assertCounter(t, srv.metrics.promptTokens.WithLabelValues("estimated"), 123)
	assertCounter(t, srv.metrics.completionTokens.WithLabelValues("estimated"), 0)
	assertCounter(t, srv.metrics.totalTokens.WithLabelValues("estimated"), 123)
}

func TestMetricsRateLimitReasonAndScrape(t *testing.T) {
	srv := New(testConfig(), &fakeLimiter{allowed: false, reason: "per_ip_minute"}, &fakeChatter{}, discardLogger())
	rec := doChat(t, srv, "not json")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	assertCounter(t, srv.metrics.rateLimitDenials.WithLabelValues("per_ip_minute"), 1)
	assertCounter(t, srv.metrics.requests.WithLabelValues("POST", "/chat", "429"), 1)

	metrics := httptest.NewRecorder()
	srv.Handler().ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metrics.Code != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want %d", metrics.Code, http.StatusOK)
	}
	for _, name := range []string{
		"avatar_http_requests_total",
		"avatar_rate_limit_denials_total",
		"avatar_upstream_start_duration_seconds",
		"avatar_chat_time_to_first_token_seconds",
	} {
		if !strings.Contains(metrics.Body.String(), name) {
			t.Errorf("metrics output is missing %q:\n%s", name, metrics.Body.String())
		}
	}
	for _, sensitive := range []string{"203.0.113.9", "request_id", "not json"} {
		if strings.Contains(metrics.Body.String(), sensitive) {
			t.Errorf("metrics output contains sensitive value %q", sensitive)
		}
	}
	// Scraping metrics does not inflate the request counter.
	assertCounter(t, srv.metrics.requests.WithLabelValues("POST", "/chat", "429"), 1)
}

func TestMetricsCountUpstreamErrorsByPhase(t *testing.T) {
	t.Run("start", func(t *testing.T) {
		srv := New(testConfig(), &fakeLimiter{allowed: true}, &fakeChatter{err: errTestUpstream}, discardLogger())
		rec := doChat(t, srv, validChatBody)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
		}
		assertCounter(t, srv.metrics.upstreamErrors.WithLabelValues("start"), 1)
	})

	t.Run("stream", func(t *testing.T) {
		srv := New(testConfig(), &fakeLimiter{allowed: true}, &fakeChatter{
			stream: &fakeStream{err: errTestUpstream},
		}, discardLogger())
		rec := doChat(t, srv, validChatBody)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
		}
		assertCounter(t, srv.metrics.upstreamErrors.WithLabelValues("stream"), 1)
	})
}

var errTestUpstream = upstreamTestError{}

type upstreamTestError struct{}

func (upstreamTestError) Error() string { return "upstream test error" }

func assertCounter(t *testing.T, counter prometheus.Counter, want float64) {
	t.Helper()
	if got := testutil.ToFloat64(counter); got != want {
		t.Errorf("counter = %v, want %v", got, want)
	}
}

func assertHistogramCount(t *testing.T, registry *prometheus.Registry, name string, want uint64) {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		if len(family.Metric) == 0 || family.Metric[0].GetHistogram().GetSampleCount() != want {
			t.Fatalf("%s sample count = %v, want %d", name, family.Metric, want)
		}
		return
	}
	t.Fatalf("metric family %q not found", name)
}
