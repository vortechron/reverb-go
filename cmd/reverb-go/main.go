// Command reverb-go runs a Pusher-protocol WebSocket server that Laravel Echo
// and pusher-js can use in place of Laravel Reverb.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/vortechron/reverb-go/internal/config"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := start(ctx, os.Getenv, log); err != nil {
		log.Error("reverb-go stopped", "error", err)
		stop()
		os.Exit(1)
	}
}

func start(ctx context.Context, getenv func(string) string, log *slog.Logger) error {
	cfg, err := config.Load(getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", cfg.Addr())
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Addr(), err)
	}
	return serve(ctx, cfg, ln, log)
}
