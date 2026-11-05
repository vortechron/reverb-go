package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/vortechron/reverb-go/internal/config"
	"github.com/vortechron/reverb-go/internal/redisbus"
	"github.com/vortechron/reverb-go/internal/server"
)

const shutdownGrace = 10 * time.Second

func buildServer(ctx context.Context, cfg config.Config, log *slog.Logger) (*server.Server, func(), error) {
	opts := server.Options{Apps: cfg.Apps, Path: cfg.Path, MaxRequestSize: cfg.MaxRequestSize, Logger: log}
	cleanup := func() {}
	if cfg.Scaling.Enabled {
		bus := redisbus.New(redisbus.Options{ //nolint:contextcheck // the bus owns its goroutines and ends them in Close
			Addr: cfg.Scaling.Addr, Password: cfg.Scaling.Password, DB: cfg.Scaling.DB, Channel: cfg.Scaling.Channel, Secret: cfg.Scaling.Secret, Logger: log,
		})
		if err := bus.Ping(ctx); err != nil {
			_ = bus.Close()
			return nil, nil, fmt.Errorf("scaling is enabled but Redis is not reachable: %w", err)
		}
		opts.Bus = bus
		cleanup = func() { _ = bus.Close() }
	}
	return server.New(opts), cleanup, nil
}

// serve runs until ctx ends, then shuts down gracefully.
func serve(ctx context.Context, cfg config.Config, ln net.Listener, log *slog.Logger) error {
	srv, cleanup, err := buildServer(ctx, cfg, log)
	if err != nil {
		_ = ln.Close()
		return err
	}
	defer cleanup()
	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- httpSrv.Serve(ln) }()
	log.Info("reverb-go listening", "addr", ln.Addr().String())

	select {
	case err := <-errc:
		srv.Close()
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
	defer cancel()
	srv.Close() // closes WebSocket sessions first, they would block Shutdown
	if err := httpSrv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}
