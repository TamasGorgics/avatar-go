package main

import (
	"context"
	_ "embed"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"avatar-go/internal/config"
	"avatar-go/internal/ratelimit"
	"avatar-go/internal/twin"
	"avatar-go/internal/web"
)

const shutdownTimeout = 10 * time.Second

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
	handler := web.New(cfg, ratelimit.New(), chat).Handler()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	log.Printf("listening on %s (model %s)", cfg.Addr, cfg.Model)
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	case <-ctx.Done():
		// Restore default signal handling so a second signal kills immediately.
		stop()
		log.Printf("shutting down")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown: %v", err)
			_ = srv.Close()
		}
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server: %v", err)
		}
	}
}
