package server_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/vortechron/reverb-go/broadcast"
	"github.com/vortechron/reverb-go/internal/apps"
	"github.com/vortechron/reverb-go/internal/server"
)

var testCreds = broadcast.Credentials{AppID: "app1", Key: "key1", Secret: "secret1"}

func testApp() apps.App {
	return apps.App{
		ID: testCreds.AppID, Key: testCreds.Key, Secret: testCreds.Secret,
		PingInterval: time.Minute, ActivityTimeout: 30 * time.Second,
		MaxMessageSize: 10000, ClientEvents: apps.ClientEventsMembers,
	}
}

type harness struct {
	srv  *server.Server
	http *httptest.Server
	app  apps.App
}

func newHarness(t *testing.T, mutate ...func(*apps.App)) *harness {
	t.Helper()
	return newHarnessOpts(t, server.Options{}, mutate...)
}

func newHarnessOpts(t *testing.T, opts server.Options, mutate ...func(*apps.App)) *harness {
	t.Helper()
	app := testApp()
	for _, m := range mutate {
		m(&app)
	}
	reg, err := apps.NewRegistry([]apps.App{app})
	if err != nil {
		t.Fatal(err)
	}
	opts.Apps = reg
	srv := server.New(opts)
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { hs.Close(); srv.Close() })
	return &harness{srv: srv, http: hs, app: app}
}

func (h *harness) wsURL(key string) string {
	return "ws" + strings.TrimPrefix(h.http.URL, "http") + "/app/" + key + "?protocol=7&client=js&version=8.4.0"
}

// client is a raw Pusher client used to look at exact frames.
type client struct {
	t        *testing.T
	conn     *websocket.Conn
	SocketID string
}

type frame struct {
	Event   string          `json:"event"`
	Channel string          `json:"channel"`
	Data    json.RawMessage `json:"data"`
	UserID  json.RawMessage `json:"user_id"`
}

// dataString returns data when it is a JSON string (the usual Pusher shape).
func (f frame) dataString() string {
	var s string
	if err := json.Unmarshal(f.Data, &s); err != nil {
		return string(f.Data)
	}
	return s
}

func (h *harness) dial(t *testing.T) *client {
	t.Helper()
	return h.dialWith(t, h.app.Key, nil)
}

func (h *harness) dialWith(t *testing.T, key string, opts *websocket.DialOptions) *client {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, h.wsURL(key), opts)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return &client{t: t, conn: conn}
}

// connect dials and checks the connection_established frame.
func (h *harness) connect(t *testing.T) *client {
	t.Helper()
	c := h.dial(t)
	f := c.read()
	if f.Event != "pusher:connection_established" {
		t.Fatalf("first frame = %+v", f)
	}
	var d struct {
		SocketID string `json:"socket_id"`
	}
	if err := json.Unmarshal([]byte(f.dataString()), &d); err != nil {
		t.Fatal(err)
	}
	c.SocketID = d.SocketID
	return c
}

func (c *client) read() frame {
	c.t.Helper()
	f, err := c.tryRead(3 * time.Second)
	if err != nil {
		c.t.Fatalf("read: %v", err)
	}
	return f
}

func (c *client) tryRead(d time.Duration) (frame, error) {
	ctx, cancel := context.WithTimeout(c.t.Context(), d)
	defer cancel()
	_, data, err := c.conn.Read(ctx)
	if err != nil {
		return frame{}, err
	}
	var f frame
	if err := json.Unmarshal(data, &f); err != nil {
		c.t.Fatalf("bad frame %q: %v", data, err)
	}
	return f, nil
}

func (c *client) expectSilence() {
	c.t.Helper()
	if f, err := c.tryRead(150 * time.Millisecond); err == nil {
		c.t.Fatalf("unexpected frame %+v", f)
	}
}

func (c *client) send(v any) {
	c.t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		c.t.Fatal(err)
	}
	c.sendRaw(string(raw))
}

func (c *client) sendRaw(s string) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(c.t.Context(), 3*time.Second)
	defer cancel()
	if err := c.conn.Write(ctx, websocket.MessageText, []byte(s)); err != nil {
		c.t.Fatal(err)
	}
}

func (c *client) subscribe(channel, auth, channelData string) {
	c.t.Helper()
	data := map[string]string{"channel": channel}
	if auth != "" {
		data["auth"] = auth
	}
	if channelData != "" {
		data["channel_data"] = channelData
	}
	c.send(map[string]any{"event": "pusher:subscribe", "data": data})
}

func (c *client) subscribeOK(channel, auth, channelData string) frame {
	c.t.Helper()
	c.subscribe(channel, auth, channelData)
	f := c.read()
	if f.Event != "pusher_internal:subscription_succeeded" || f.Channel != channel {
		c.t.Fatalf("subscribe %s: got %+v", channel, f)
	}
	return f
}

func (c *client) expectError(code int) {
	c.t.Helper()
	f := c.read()
	if f.Event != "pusher:error" {
		c.t.Fatalf("want pusher:error, got %+v", f)
	}
	var d struct{ Code int }
	_ = json.Unmarshal([]byte(f.dataString()), &d)
	if d.Code != code {
		c.t.Fatalf("error code = %d, want %d (%s)", d.Code, code, f.dataString())
	}
}

func (h *harness) auth(t *testing.T, c *client, channel string) string {
	t.Helper()
	a, err := testCreds.AuthorizePrivate(c.SocketID, channel)
	if err != nil {
		t.Fatal(err)
	}
	return a.Auth
}

// presence signs channel_data for the channel "presence-r" and returns auth and data.
func (h *harness) presence(t *testing.T, c *client, userID, info any) (auth, data string) {
	t.Helper()
	a, err := testCreds.AuthorizePresence(c.SocketID, "presence-r", broadcast.PresenceMember{UserID: userID, UserInfo: info})
	if err != nil {
		t.Fatal(err)
	}
	return a.Auth, a.ChannelData
}
