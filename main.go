package main

import (
	"context"
	_ "embed"
	"errors"
	"log"
	"net"
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

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Once the first signal cancels ctx, restore default handling so a second signal
	// terminates the process immediately.
	restore := context.AfterFunc(ctx, stop)
	defer restore()

	log.Printf("listening on %s (model %s)", ln.Addr(), cfg.Model)
	if err := serve(ctx, srv, ln, shutdownTimeout); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// serve runs srv on ln until ctx is canceled, then drains in-flight requests.
func serve(ctx context.Context, srv *http.Server, ln net.Listener, timeout time.Duration) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		log.Printf("shutting down")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		shutdownErr := srv.Shutdown(shutdownCtx)
		if shutdownErr != nil {
			_ = srv.Close()
		}
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return shutdownErr
	}
}
