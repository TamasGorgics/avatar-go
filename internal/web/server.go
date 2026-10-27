package web

import (
	"html/template"
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
	page *template.Template
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
		page: template.Must(template.New("p").Parse(pageHTML)),
		log:  logger,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handlePage)
	mux.HandleFunc("POST /chat", s.handleChat)
	return s.logRequests(mux)
}

func (s *Server) handlePage(w http.ResponseWriter, _ *http.Request) {
	if err := s.page.Execute(w, nil); err != nil {
		s.log.Error("render page", "err", err)
	}
}
