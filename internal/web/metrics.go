package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var observationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 90}

type appMetrics struct {
	registry         *prometheus.Registry
	requests         *prometheus.CounterVec
	requestDuration  *prometheus.HistogramVec
	rateLimitDenials *prometheus.CounterVec
	upstreamStart    prometheus.Histogram
	upstreamErrors   *prometheus.CounterVec
	streamDuration   prometheus.Histogram
	timeToFirstToken prometheus.Histogram
	promptTokens     *prometheus.CounterVec
	completionTokens *prometheus.CounterVec
	totalTokens      *prometheus.CounterVec
}

func newAppMetrics() *appMetrics {
	m := &appMetrics{
		registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "avatar",
			Name:      "http_requests_total",
			Help:      "HTTP requests served by route, method, and status.",
		}, []string{"method", "route", "status"}),
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "avatar",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds.",
			Buckets:   observationBuckets,
		}, []string{"method", "route"}),
		rateLimitDenials: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "avatar",
			Name:      "rate_limit_denials_total",
			Help:      "Requests denied by the in-memory limiter, by reason.",
		}, []string{"reason"}),
		upstreamStart: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "avatar",
			Name:      "upstream_start_duration_seconds",
			Help:      "Time for a model stream request to return its stream handle.",
			Buckets:   observationBuckets,
		}),
		upstreamErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "avatar",
			Name:      "upstream_errors_total",
			Help:      "Model upstream errors, by request or stream phase.",
		}, []string{"phase"}),
		streamDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "avatar",
			Name:      "chat_stream_duration_seconds",
			Help:      "Duration from starting a model request to exhausting its stream.",
			Buckets:   observationBuckets,
		}),
		timeToFirstToken: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "avatar",
			Name:      "chat_time_to_first_token_seconds",
			Help:      "Time from starting a model request to its first non-empty text delta.",
			Buckets:   observationBuckets,
		}),
		promptTokens: newTokenCounter("prompt_tokens_total", "Prompt tokens reported or estimated for model calls."),
		completionTokens: newTokenCounter(
			"completion_tokens_total",
			"Completion tokens reported or reserved for model calls.",
		),
		totalTokens: newTokenCounter("tokens_total", "Total tokens reported or estimated for model calls."),
	}

	m.registry.MustRegister(
		m.requests,
		m.requestDuration,
		m.rateLimitDenials,
		m.upstreamStart,
		m.upstreamErrors,
		m.streamDuration,
		m.timeToFirstToken,
		m.promptTokens,
		m.completionTokens,
		m.totalTokens,
	)
	return m
}

func newTokenCounter(name, help string) *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "avatar",
		Name:      name,
		Help:      help,
	}, []string{"source"})
}

func (m *appMetrics) handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *appMetrics) observeRequest(method, path string, status int, duration time.Duration) {
	method = metricMethod(method)
	route := metricRoute(path)
	m.requests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
	m.requestDuration.WithLabelValues(method, route).Observe(duration.Seconds())
}

func (m *appMetrics) observeRateLimit(reason string) {
	m.rateLimitDenials.WithLabelValues(metricLimitReason(reason)).Inc()
}

func (m *appMetrics) observeTokens(source string, prompt, completion, total int) {
	if prompt < 0 {
		prompt = 0
	}
	if completion < 0 {
		completion = 0
	}
	if total < 0 {
		total = 0
	}
	m.promptTokens.WithLabelValues(source).Add(float64(prompt))
	m.completionTokens.WithLabelValues(source).Add(float64(completion))
	m.totalTokens.WithLabelValues(source).Add(float64(total))
}

func metricMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace:
		return method
	default:
		return "OTHER"
	}
}

func metricRoute(path string) string {
	switch {
	case path == "/":
		return "/"
	case path == "/chat":
		return "/chat"
	case path == "/healthz":
		return "/healthz"
	case path == "/readyz":
		return "/readyz"
	case path == "/metrics":
		return "/metrics"
	case strings.HasPrefix(path, "/assets/"):
		return "/assets/"
	default:
		return "unmatched"
	}
}

func metricLimitReason(reason string) string {
	switch reason {
	case "per_ip_minute", "per_ip_hour", "per_ip_daily_tokens", "global_minute", "global_daily", "per_ip_concurrent", "global_concurrent":
		return reason
	default:
		return "unknown"
	}
}
