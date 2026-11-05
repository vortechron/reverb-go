package server_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/vortechron/reverb-go/internal/apps"
	"github.com/vortechron/reverb-go/internal/server"
)

// memBus fans every message out to all subscribers, itself included, like Redis pub/sub.
type memBus struct {
	mu   sync.Mutex
	subs []func(server.Message)
}

func (b *memBus) Publish(_ context.Context, m server.Message) error {
	b.mu.Lock()
	subs := append([]func(server.Message){}, b.subs...)
	b.mu.Unlock()
	for _, s := range subs {
		s(m)
	}
	return nil
}

func (b *memBus) Subscribe(_ context.Context, h func(server.Message)) error {
	b.mu.Lock()
	b.subs = append(b.subs, h)
	b.mu.Unlock()
	return nil
}

func twoNodes(t *testing.T) (a, b *harness, bus *memBus) {
	t.Helper()
	bus = &memBus{}
	mk := func() *harness {
		reg, err := apps.NewRegistry([]apps.App{testApp()})
		if err != nil {
			t.Fatal(err)
		}
		srv := server.New(server.Options{Apps: reg, Bus: bus})
		hs := httptest.NewServer(srv.Handler())
		t.Cleanup(func() { hs.Close(); srv.Close() })
		return &harness{srv: srv, http: hs, app: testApp()}
	}
	return mk(), mk(), bus
}

func TestEventPublishedOnOneNodeReachesClientsOnAnother(t *testing.T) {
	a, b, _ := twoNodes(t)
	onB := b.connect(t)
	onA := a.connect(t)
	onB.subscribeOK("news", "", "")
	onA.subscribeOK("news", "", "")
	if code, _ := a.post(t, "events", `{"name":"e","channel":"news","data":"d"}`); code != 200 {
		t.Fatalf("status %d", code)
	}
	if f := onB.read(); f.Event != "e" {
		t.Fatalf("node B client got %+v", f)
	}
	if f := onA.read(); f.Event != "e" {
		t.Fatalf("node A client got %+v", f)
	}
	onA.expectSilence() // own node must not deliver twice
}

func TestWhisperCrossesNodes(t *testing.T) {
	a, b, _ := twoNodes(t)
	onA, onB := a.connect(t), b.connect(t)
	onA.subscribeOK("private-x", a.auth(t, onA, "private-x"), "")
	onB.subscribeOK("private-x", b.auth(t, onB, "private-x"), "")
	onA.send(map[string]any{"event": "client-hi", "channel": "private-x", "data": map[string]int{"n": 1}})
	if f := onB.read(); f.Event != "client-hi" {
		t.Fatalf("got %+v", f)
	}
	onA.expectSilence()
}

func TestTerminateCrossesNodes(t *testing.T) {
	a, b, _ := twoNodes(t)
	onB := b.connect(t)
	auth, data := b.presence(t, onB, 3, nil)
	onB.subscribeOK("presence-r", auth, data)
	if code, _ := a.call(t, apiCall{method: "POST", path: "/apps/app1/users/3/terminate_connections"}); code != 200 {
		t.Fatalf("status %d", code)
	}
	if _, err := onB.tryRead(2e9); err == nil {
		t.Fatal("client on node B should be disconnected")
	}
}

type sickBus struct {
	memBus
	err error
}

func (b *sickBus) Healthy() error { return b.err }

func TestUpReportsAnUnhealthyBus(t *testing.T) {
	bus := &sickBus{err: errors.New("redis is gone")}
	reg, err := apps.NewRegistry([]apps.App{testApp()})
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(server.Options{Apps: reg, Bus: bus})
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { hs.Close(); srv.Close() })

	get := func() (int, string) {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, hs.URL+"/up", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, body := get(); code != 503 || !strings.Contains(body, "redis is gone") {
		t.Fatalf("unhealthy bus: %d %s", code, body)
	}
	bus.err = nil
	if code, _ := get(); code != 200 {
		t.Fatalf("healthy bus: %d", code)
	}
}

func TestBusMessagesWithBadContentAreIgnored(t *testing.T) {
	a, b, bus := twoNodes(t)
	onB := b.connect(t)
	onB.subscribeOK("news", "", "")
	forged := []server.Message{
		{Node: "evil", Kind: "event", AppID: "no-such-app", Name: "news", Frame: []byte(`{"event":"x"}`)},
		{Node: "evil", Kind: "event", AppID: "app1", Name: "bad channel:name", Frame: []byte(`{"event":"x"}`)},
		{Node: "evil", Kind: "mystery", AppID: "app1", Name: "news", Frame: []byte(`{"event":"x"}`)},
		{Node: "evil", Kind: "event", AppID: "app1", Name: "news", Frame: []byte(`{"event":"x"}`), Except: "not a socket id"},
		{Node: "evil", Kind: "terminate", AppID: "app1", UserID: ""},
	}
	for _, m := range forged {
		_ = bus.Publish(t.Context(), m)
	}
	// If any forged message got through, it would arrive before "ok".
	if code, _ := a.post(t, "events", `{"name":"ok","channel":"news","data":"d"}`); code != 200 {
		t.Fatalf("status %d", code)
	}
	if f := onB.read(); f.Event != "ok" {
		t.Fatalf("got %+v", f)
	}
}
