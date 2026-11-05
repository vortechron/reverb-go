package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"

	"github.com/coder/websocket"

	"github.com/vortechron/reverb-go/internal/apps"
	"github.com/vortechron/reverb-go/internal/wire"
)

const maxSocketPart = 1_000_000_000

func newNodeID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// newSocketID makes ids like "123456789.987654321", the shape Pusher clients expect.
func newSocketID() string {
	part := func() int64 {
		n, err := rand.Int(rand.Reader, big.NewInt(maxSocketPart-1))
		if err != nil {
			return 1
		}
		return n.Int64() + 1
	}
	return fmt.Sprintf("%d.%d", part(), part())
}

func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	// Origin is checked against the app list below, not by the library.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	s.wg.Add(1)
	defer s.wg.Done()
	app, ok := s.opts.Apps.ByKey(r.PathValue("key"))
	if !ok {
		refuse(r.Context(), conn, wire.CodeAppNotFound, "Application does not exist")
		return
	}
	if !app.OriginAllowed(r.Header.Get("Origin")) {
		refuse(r.Context(), conn, wire.CodeUnauthorized, "Origin not allowed")
		return
	}
	s.run(r.Context(), conn, app)
}

// refuse sends one pusher:error frame and closes with the same code.
func refuse(ctx context.Context, conn *websocket.Conn, code int, message string) {
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	_ = conn.Write(wctx, websocket.MessageText, wire.Error(code, message))
	_ = conn.Close(websocket.StatusCode(code), message)
}

func (s *Server) run(parent context.Context, conn *websocket.Conn, app apps.App) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	go func() {
		select {
		case <-s.ctx.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	sess := newSession(s, app, newSocketID())
	sess.conn = conn
	conn.SetReadLimit(app.MaxMessageSize)
	if !sess.hub.TryAdd(sess, app.MaxConnections) {
		refuse(parent, conn, wire.CodeOverQuota, "Application is over connection quota")
		return
	}
	sess.Send(wire.Established(sess.id, int(app.ActivityTimeout.Seconds())))

	writerDone := make(chan struct{})
	go func() { defer close(writerDone); sess.writeLoop(ctx) }()

	sess.readLoop(ctx)

	sess.hub.Remove(sess)
	sess.Close(int(websocket.StatusNormalClosure), "")
	<-writerDone
}

func (s *session) readLoop(ctx context.Context) {
	for {
		_, data, err := s.conn.Read(ctx)
		if err != nil {
			return
		}
		s.handle(data)
	}
}
