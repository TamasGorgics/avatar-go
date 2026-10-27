package web

import (
	"bytes"
	"log/slog"
	"net/http"

	"avatar-go/internal/config"
	"avatar-go/internal/twin"
)

// Limiter is the abuse-protection dependency of the HTTP layer.
type Limiter interface {
	Allow(ip string, perHour, daily int) bool
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
	return s.logRequests(mux)
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
