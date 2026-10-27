package main

import (
	_ "embed"
	"log"
	"net/http"

	"avatar-go/internal/config"
	"avatar-go/internal/ratelimit"
	"avatar-go/internal/twin"
	"avatar-go/internal/web"
)

//go:embed about.md
var about string

func main() {
	cfg := config.Load()
	chat := twin.New(twin.Options{
		Profile: about,
		APIKey:  cfg.APIKey,
		BaseURL: cfg.LLMBaseURL,
		Model:   cfg.Model,
	})
	srv := web.New(cfg, ratelimit.New(), chat)

	log.Printf("listening on %s (model %s)", cfg.Addr, cfg.Model)
	log.Fatal(http.ListenAndServe(cfg.Addr, srv.Handler()))
}
