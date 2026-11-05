// Package server serves the Pusher WebSocket protocol and HTTP API.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/vortechron/reverb-go/internal/apps"
	"github.com/vortechron/reverb-go/internal/hub"
)

// defaultSendQueue is how many frames may wait for one slow client.
const defaultSendQueue = 256

// Options configure a Server.
type Options struct {
	Apps *apps.Registry
	// Path is an optional URL prefix, like REVERB_SERVER_PATH.
	Path string
	// MaxRequestSize limits HTTP API bodies in bytes. Default 10000.
	MaxRequestSize int64
	// Bus is optional and shares events between nodes.
	Bus Bus
	// Logger is optional.
	Logger *slog.Logger
	// SendQueue is the per-connection frame queue. Default 256.
	SendQueue int
}

// Server is the whole realtime server. Use Handler to mount it.
type Server struct {
	opts   Options
	log    *slog.Logger
	hubs   map[string]*hub.Hub
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	nodeID string
}

// New builds a Server and starts listening to the bus when one is set.
func New(opts Options) *Server {
	if opts.MaxRequestSize <= 0 {
		opts.MaxRequestSize = 10000
	}
	if opts.SendQueue <= 0 {
		opts.SendQueue = defaultSendQueue
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	opts.Path = strings.TrimRight(opts.Path, "/")
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{opts: opts, log: opts.Logger, hubs: map[string]*hub.Hub{}, ctx: ctx, cancel: cancel, nodeID: newNodeID()}
	for _, a := range opts.Apps.All() {
		s.hubs[a.ID] = hub.New()
	}
	s.startBus()
	return s
}

// Close disconnects everyone and waits for connection goroutines to finish.
func (s *Server) Close() {
	s.cancel()
	s.wg.Wait()
}

// Handler returns the HTTP handler with WebSocket and API routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	p := s.opts.Path
	mux.HandleFunc("GET "+p+"/app/{key}", s.serveWS)
	s.routeAPI(mux, p)
	return mux
}
