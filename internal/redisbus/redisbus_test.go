package redisbus_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/vortechron/reverb-go/internal/redisbus"
	"github.com/vortechron/reverb-go/internal/server"
)

func TestMessagesTravelBetweenTwoBuses(t *testing.T) {
	mr := miniredis.RunT(t)
	opts := redisbus.Options{Addr: mr.Addr(), Channel: "test-chan"}
	a, b := redisbus.New(opts), redisbus.New(opts)
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })

	got := make(chan server.Message, 1)
	if err := b.Subscribe(t.Context(), func(m server.Message) { got <- m }); err != nil {
		t.Fatal(err)
	}
	want := server.Message{Node: "n1", Kind: "event", AppID: "app", Name: "news", Frame: []byte(`{"event":"e"}`), Except: "1.2"}
	if err := a.Publish(t.Context(), want); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if m.Node != "n1" || m.Name != "news" || string(m.Frame) != `{"event":"e"}` || m.Except != "1.2" {
			t.Fatalf("got %+v", m)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no message received")
	}
}

func TestSubscribeFailsWhenRedisIsDown(t *testing.T) {
	bus := redisbus.New(redisbus.Options{Addr: "127.0.0.1:1", Channel: "x"})
	t.Cleanup(func() { _ = bus.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := bus.Subscribe(ctx, func(server.Message) {}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestPublishFailsWhenRedisIsDown(t *testing.T) {
	bus := redisbus.New(redisbus.Options{Addr: "127.0.0.1:1", Channel: "x"})
	t.Cleanup(func() { _ = bus.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := bus.Publish(ctx, server.Message{}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestPing(t *testing.T) {
	mr := miniredis.RunT(t)
	bus := redisbus.New(redisbus.Options{Addr: mr.Addr(), Channel: "x"})
	t.Cleanup(func() { _ = bus.Close() })
	if err := bus.Ping(t.Context()); err != nil {
		t.Fatal(err)
	}
	mr.Close()
	if err := bus.Ping(t.Context()); err == nil {
		t.Fatal("expected an error after Redis stopped")
	}
}
