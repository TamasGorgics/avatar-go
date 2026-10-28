package web

import (
	"bytes"
	"log/slog"
	"net/http"
	"time"

	"avatar-go/internal/config"
	"avatar-go/internal/twin"
)

// Limiter separates early request-attempt checks from model-call admission.
type Limiter interface {
	CheckAttempt(ip string) (allowed bool, reason string, retryAfter time.Duration)
	ReserveModel(ip string, estimatedTokens int) (allowed bool, reason string, retryAfter time.Duration, finish func(actualTokens int, usageKnown bool))
}

type Server struct {
	cfg  config.Config
	lim  Limiter
	chat twin.Chatter
	log  *slog.Logger
}

func New(cfg config.Config, lim Limiter, chat twin.Chatter, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		cfg:  cfg,
		lim:  lim,
		chat: chat,
		log:  logger,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handlePage)
	mux.Handle("GET /assets/", assetHandler())
	mux.HandleFunc("POST /chat", s.handleChat)
	return s.logRequests(s.securityHeaders(mux))
}

const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline'; " +
	"style-src 'self'; " +
	"img-src 'self'; " +
	"connect-src 'self'; " +
	"font-src 'self'; " +
	"base-uri 'none'; " +
	"frame-ancestors 'none'; " +
	"form-action 'self'; " +
	"object-src 'none'"

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	if err := Page().Render(r.Context(), &buf); err != nil {
		s.log.Error("render page", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if _, err := buf.WriteTo(w); err != nil {
		s.log.Error("write page", "err", err)
	}
}
