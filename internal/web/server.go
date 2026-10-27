package web

import (
	"html/template"
	"log"
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
}

func New(cfg config.Config, lim Limiter, chat twin.Chatter) *Server {
	return &Server{
		cfg:  cfg,
		lim:  lim,
		chat: chat,
		page: template.Must(template.New("p").Parse(pageHTML)),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handlePage)
	mux.HandleFunc("POST /chat", s.handleChat)
	return mux
}

func (s *Server) handlePage(w http.ResponseWriter, _ *http.Request) {
	if err := s.page.Execute(w, nil); err != nil {
		log.Printf("render page: %v", err)
	}
}
