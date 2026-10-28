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

const (
	shutdownTimeout   = 10 * time.Second
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 75 * time.Second
	idleTimeout       = 120 * time.Second
	maxHeaderBytes    = 32 << 10
)

//go:embed about.md
var about string

func main() {
	envErr := config.LoadDotEnv(".env")
	cfg := config.Load()
	logger := logging.New(logging.Options{
		Format: cfg.LogFormat,
		Level:  cfg.LogLevel,
	})
	slog.SetDefault(logger)

	if envErr != nil {
		logger.Error("load .env", "path", ".env", "err", envErr)
		os.Exit(1)
	}

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
	limiter := ratelimit.New(ratelimit.Limits{
		PerIPMinute:      cfg.PerIPMinute,
		PerIPHourly:      cfg.PerIPHourly,
		PerIPDailyTokens: cfg.PerIPDailyTokens,
		GlobalMinute:     cfg.GlobalMinute,
		GlobalDaily:      cfg.DailyCap,
		PerIPConcurrent:  cfg.PerIPConcurrent,
		GlobalConcurrent: cfg.GlobalConcurrent,
	})
	webServer := web.New(cfg, limiter, chat, logger)
	handler := webServer.Handler()

	srv := newHTTPServer(cfg, handler, logger)
	srv.RegisterOnShutdown(func() { webServer.SetReady(false) })

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

func newHTTPServer(cfg config.Config, handler http.Handler, logger *slog.Logger) *http.Server {
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
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
