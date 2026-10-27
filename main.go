package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"avatar-go/internal/config"
	"avatar-go/internal/logging"
	"avatar-go/internal/ratelimit"
	"avatar-go/internal/twin"
	"avatar-go/internal/web"
)

const shutdownTimeout = 10 * time.Second

//go:embed about.md
var about string

func main() {
	cfg := config.Load()
	logger := logging.New(logging.Options{
		Format: cfg.LogFormat,
		Level:  cfg.LogLevel,
	})
	slog.SetDefault(logger)

	if err := run(cfg, logger); err != nil {
		logger.Error("server failed", "err", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, logger *slog.Logger) error {
	chat := twin.New(twin.Options{
		Profile: about,
		APIKey:  cfg.APIKey,
		BaseURL: cfg.LLMBaseURL,
		Model:   cfg.Model,
	})
	handler := web.New(cfg, ratelimit.New(), chat, logger).Handler()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Once the first signal cancels ctx, restore default handling so a second signal
	// terminates the process immediately.
	restore := context.AfterFunc(ctx, stop)
	defer restore()

	logger.Info("listening", "addr", ln.Addr().String(), "model", cfg.Model)
	if err := serve(ctx, logger, srv, ln, shutdownTimeout); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

// serve runs srv on ln until ctx is canceled, then drains in-flight requests.
func serve(ctx context.Context, logger *slog.Logger, srv *http.Server, ln net.Listener, timeout time.Duration) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		logger.Info("shutting down")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		shutdownErr := srv.Shutdown(shutdownCtx)
		if shutdownErr != nil {
			logger.Error("graceful shutdown incomplete", "err", shutdownErr)
			_ = srv.Close()
		}
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return shutdownErr
	}
}
