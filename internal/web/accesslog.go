package web

import (
	"log/slog"
	"net/http"
	"time"
)

// logRequests writes one structured access log per request. The status drives
// the level: 4xx is a warning and 5xx is an error.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)

		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		duration := time.Since(start)
		if r.URL.Path != "/metrics" {
			s.metrics.observeRequest(r.Method, r.URL.Path, status, duration)
		}
		level := slog.LevelInfo
		switch {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		}

		s.log.LogAttrs(r.Context(), level, "request",
			slog.String("request_id", requestID(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", status),
			slog.Int("bytes", sw.bytes),
			slog.Duration("duration", duration),
			slog.String("remote_ip", clientIP(r, s.cfg.TrustProxy)),
		)
	})
}

// statusWriter records the response status and body size while preserving
// http.Flusher, which the streaming chat handler depends on.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	if code >= 100 && code < 200 {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	if w.status != 0 {
		return
	}
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		// Record the implicit 200 without calling WriteHeader. Letting the
		// underlying writer see the first bytes preserves content sniffing.
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
