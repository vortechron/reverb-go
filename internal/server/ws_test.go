package server_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/vortechron/reverb-go/internal/apps"
)

func TestConnectionEstablished(t *testing.T) {
	h := newHarness(t)
	c := h.dial(t)
	f := c.read()
	if f.Event != "pusher:connection_established" {
		t.Fatalf("frame = %+v", f)
	}
	re := regexp.MustCompile(`^\{"socket_id":"\d+\.\d+","activity_timeout":30\}$`)
	if !re.MatchString(f.dataString()) {
		t.Fatalf("data = %s", f.dataString())
	}
}

func TestUnknownAppKeyGetsError4001(t *testing.T) {
	h := newHarness(t)
	c := h.dialWith(t, "nope", nil)
	c.expectError(4001)
}

func TestOriginNotAllowedGetsError4009(t *testing.T) {
	h := newHarness(t, func(a *apps.App) { a.AllowedOrigins = []string{"good.test"} })
	bad := h.dialWith(t, h.app.Key, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"https://evil.test"}}})
	bad.expectError(4009)
	good := h.dialWith(t, h.app.Key, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"https://good.test"}}})
	if f := good.read(); f.Event != "pusher:connection_established" {
		t.Fatalf("good origin got %+v", f)
	}
}

func TestConnectionLimitGetsError4004(t *testing.T) {
	h := newHarness(t, func(a *apps.App) { a.MaxConnections = 1 })
	h.connect(t)
	over := h.dial(t)
	over.expectError(4004)
}

func TestPingGetsPong(t *testing.T) {
	c := newHarness(t).connect(t)
	c.send(map[string]string{"event": "pusher:ping"})
	if f := c.read(); f.Event != "pusher:pong" {
		t.Fatalf("frame = %+v", f)
	}
}

func TestPublicSubscribeAndUnsubscribe(t *testing.T) {
	h := newHarness(t)
	c := h.connect(t)
	f := c.subscribeOK("news", "", "")
	if f.dataString() != "{}" {
		t.Fatalf("data = %s", f.dataString())
	}
	c.send(map[string]any{"event": "pusher:unsubscribe", "data": map[string]string{"channel": "news"}})
	c.send(map[string]string{"event": "pusher:ping"})
	if f := c.read(); f.Event != "pusher:pong" {
		t.Fatalf("frame = %+v", f)
	}
}

func TestPrivateSubscribeChecksSignature(t *testing.T) {
	h := newHarness(t)
	c := h.connect(t)
	c.subscribe("private-a", "key1:deadbeef", "")
	c.expectError(4009)
	c.subscribe("private-a", "", "")
	c.expectError(4009)
	c.subscribeOK("private-a", h.auth(t, c, "private-a"), "")
	other := h.connect(t)
	other.subscribe("private-a", h.auth(t, c, "private-a"), "")
	other.expectError(4009) // signature belongs to another socket id
}

func TestSubscribeRejectsMalformedChannelName(t *testing.T) {
	c := newHarness(t).connect(t)
	for _, name := range []string{"private-a:b", "has space", ""} {
		c.subscribe(name, "", "")
		c.expectError(4200)
	}
}

func TestMalformedMessagesGetError4200(t *testing.T) {
	c := newHarness(t).connect(t)
	for _, raw := range []string{
		`not json`, `{}`, `{"event":5}`, `{"event":"pusher:nope"}`,
		`{"event":"pusher:subscribe","data":{}}`,
		`{"event":"pusher:subscribe","data":{"channel":"x","channel_data":"{bad"}}`,
		`{"event":"pusher:subscribe","data":{"channel":"x","auth":5}}`,
	} {
		c.sendRaw(raw)
		c.expectError(4200)
	}
}

func TestPresenceJoinLeaveAndMemberEvents(t *testing.T) {
	h := newHarness(t)
	a, b := h.connect(t), h.connect(t)
	authA, dataA := h.presence(t, a, 1, map[string]string{"name": "Alice"})
	f := a.subscribeOK("presence-r", authA, dataA)
	want := `{"presence":{"count":1,"ids":[1],"hash":{"1":{"name":"Alice"}}}}`
	if f.dataString() != want {
		t.Fatalf("data = %s", f.dataString())
	}
	authB, dataB := h.presence(t, b, 2, map[string]string{"name": "Bob"})
	b.subscribeOK("presence-r", authB, dataB)
	added := a.read()
	if added.Event != "pusher_internal:member_added" || !strings.Contains(added.dataString(), `"user_id":2`) {
		t.Fatalf("a got %+v", added)
	}
	_ = b.conn.Close(websocket.StatusNormalClosure, "bye")
	removed := a.read()
	if removed.Event != "pusher_internal:member_removed" || removed.dataString() != `{"user_id":2}` {
		t.Fatalf("a got %+v", removed)
	}
}

func TestPresenceRejectsTamperedChannelData(t *testing.T) {
	h := newHarness(t)
	c := h.connect(t)
	auth, _ := h.presence(t, c, 1, nil)
	c.subscribe("presence-r", auth, `{"user_id":99}`)
	c.expectError(4009)
}

func TestClientEventsReachOthersNotSender(t *testing.T) {
	h := newHarness(t)
	a, b := h.connect(t), h.connect(t)
	a.subscribeOK("private-calls.1", h.auth(t, a, "private-calls.1"), "")
	b.subscribeOK("private-calls.1", h.auth(t, b, "private-calls.1"), "")
	a.send(map[string]any{"event": "client-EndCall", "channel": "private-calls.1", "data": map[string]int{"x": 1}})
	f := b.read()
	if f.Event != "client-EndCall" || f.Channel != "private-calls.1" || string(f.Data) != `{"x":1}` {
		t.Fatalf("b got %+v", f)
	}
	a.expectSilence()
}

func TestClientEventOnPresenceCarriesUserID(t *testing.T) {
	h := newHarness(t)
	a, b := h.connect(t), h.connect(t)
	aa, ad := h.presence(t, a, 5, nil)
	a.subscribeOK("presence-r", aa, ad)
	ba, bd := h.presence(t, b, 6, nil)
	b.subscribeOK("presence-r", ba, bd)
	a.read() // member_added
	a.send(map[string]any{"event": "client-typing", "channel": "presence-r", "data": map[string]bool{"t": true}})
	f := b.read()
	if string(f.UserID) != "5" {
		t.Fatalf("user_id = %s", f.UserID)
	}
}

func TestClientEventRules(t *testing.T) {
	h := newHarness(t)
	c := h.connect(t)
	send := func(ch string, data any) {
		c.send(map[string]any{"event": "client-x", "channel": ch, "data": data})
	}
	send("private-nope", map[string]int{}) // not subscribed
	c.expectError(4009)
	c.subscribeOK("news", "", "")
	send("news", map[string]int{}) // public channel
	c.expectError(4009)
	c.subscribeOK("private-ok", h.auth(t, c, "private-ok"), "")
	send("private-ok", "just a string") // data must be an object
	c.expectError(4200)
	c.send(map[string]any{"event": "client-x", "data": map[string]int{}}) // channel missing
	c.expectError(4200)
	c.send(map[string]any{"event": "other-event", "channel": "private-ok", "data": map[string]int{}})
	c.expectSilence() // not a client event, ignored like Reverb
}

func TestClientEventsDisabledGetError4301(t *testing.T) {
	h := newHarness(t, func(a *apps.App) { a.ClientEvents = apps.ClientEventsOff })
	c := h.connect(t)
	c.subscribeOK("private-a", h.auth(t, c, "private-a"), "")
	c.send(map[string]any{"event": "client-x", "channel": "private-a", "data": map[string]int{}})
	c.expectError(4301)
}

func TestClientEventsFromAllSkipsMembershipCheck(t *testing.T) {
	h := newHarness(t, func(a *apps.App) { a.ClientEvents = apps.ClientEventsAll })
	a, b := h.connect(t), h.connect(t)
	b.subscribeOK("private-a", h.auth(t, b, "private-a"), "")
	a.send(map[string]any{"event": "client-x", "channel": "private-a", "data": map[string]int{}})
	if f := b.read(); f.Event != "client-x" {
		t.Fatalf("b got %+v", f)
	}
}

func TestIdleConnectionIsPingedThenPruned(t *testing.T) {
	h := newHarness(t, func(a *apps.App) { a.PingInterval = 80 * time.Millisecond })
	c := h.connect(t)
	if f := c.read(); f.Event != "pusher:ping" {
		t.Fatalf("want ping, got %+v", f)
	}
	c.expectError(4201)
	if _, err := c.tryRead(2 * time.Second); err == nil {
		t.Fatal("connection should be closed")
	}
}

func TestAnsweringPingKeepsConnection(t *testing.T) {
	h := newHarness(t, func(a *apps.App) { a.PingInterval = 80 * time.Millisecond })
	c := h.connect(t)
	for range 4 {
		f := c.read()
		if f.Event != "pusher:ping" {
			t.Fatalf("frame = %+v", f)
		}
		c.send(map[string]string{"event": "pusher:pong"})
	}
}

func TestOversizedMessageClosesConnection(t *testing.T) {
	h := newHarness(t, func(a *apps.App) { a.MaxMessageSize = 200 })
	c := h.connect(t)
	c.sendRaw(`{"event":"client-x","data":"` + strings.Repeat("a", 500) + `"}`)
	if _, err := c.tryRead(2 * time.Second); err == nil {
		t.Fatal("connection should be closed")
	}
}
