package server

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/vortechron/reverb-go/internal/apps"
	"github.com/vortechron/reverb-go/internal/hub"
	"github.com/vortechron/reverb-go/internal/wire"
)

const writeTimeout = 10 * time.Second

// session is one WebSocket connection. It owns a writer goroutine; the HTTP
// handler goroutine is the reader. Everything else talks to it through Send.
type session struct {
	id   string
	app  apps.App
	srv  *Server
	hub  *hub.Hub
	conn *websocket.Conn

	out  chan []byte
	done chan struct{}

	closeOnce sync.Once
	code      int
	reason    string

	lastSeen atomic.Int64 // unix nanoseconds
	pinged   atomic.Bool
}

func newSession(srv *Server, app apps.App, id string) *session {
	s := &session{
		id: id, app: app, srv: srv, hub: srv.hubs[app.ID],
		out: make(chan []byte, srv.opts.SendQueue), done: make(chan struct{}),
	}
	s.touch()
	return s
}

// ID implements hub.Conn.
func (s *session) ID() string { return s.id }

// Send queues a frame. It never blocks: a client whose queue is full is dropped.
func (s *session) Send(frame []byte) {
	select {
	case s.out <- frame:
	default:
		s.srv.log.Warn("dropping slow consumer", "socket_id", s.id)
		s.Close(int(websocket.StatusPolicyViolation), "slow consumer")
		// The writer may be stuck in a blocked write, so cut the socket now.
		_ = s.conn.CloseNow()
	}
}

// Close asks the writer to flush, then close the socket with a code. Only the first call counts.
func (s *session) Close(code int, reason string) {
	s.closeOnce.Do(func() {
		s.code, s.reason = code, reason
		close(s.done)
	})
}

func (s *session) touch() {
	s.lastSeen.Store(time.Now().UnixNano())
	s.pinged.Store(false)
}

func (s *session) idle() bool {
	return time.Since(time.Unix(0, s.lastSeen.Load())) >= s.app.PingInterval
}

// checkIdle mirrors Reverb: ping a silent client once, drop it if it stays silent.
func (s *session) checkIdle() {
	if !s.idle() {
		return
	}
	if s.pinged.Load() {
		s.Send(wire.Error(wire.CodePongTimeout, "Pong reply not received in time"))
		s.Close(4201, "pong reply not received in time")
		return
	}
	s.pinged.Store(true)
	s.Send(wire.Ping())
}

func (s *session) writeLoop(ctx context.Context) {
	ticker := time.NewTicker(s.app.PingInterval)
	defer ticker.Stop()
	for {
		select {
		case frame := <-s.out:
			if !s.write(ctx, frame) {
				s.Close(int(websocket.StatusInternalError), "write failed")
			}
		case <-ticker.C:
			s.checkIdle()
		case <-s.done:
			s.flush(ctx)
			_ = s.conn.Close(websocket.StatusCode(s.code), s.reason)
			return
		case <-ctx.Done():
			_ = s.conn.CloseNow()
			return
		}
	}
}

func (s *session) write(ctx context.Context, frame []byte) bool {
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return s.conn.Write(wctx, websocket.MessageText, frame) == nil
}

// flush writes what is already queued, so an error frame is not lost before a close.
func (s *session) flush(ctx context.Context) {
	for {
		select {
		case frame := <-s.out:
			if !s.write(ctx, frame) {
				return
			}
		default:
			return
		}
	}
}
