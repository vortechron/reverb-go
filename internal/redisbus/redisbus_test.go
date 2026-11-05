package redisbus_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/vortechron/reverb-go/internal/redisbus"
	"github.com/vortechron/reverb-go/internal/server"
)

func opts(addr string) redisbus.Options {
	return redisbus.Options{
		Addr: addr, Channel: "test-chan", Secret: "bus-secret",
		RetryMin: 20 * time.Millisecond, RetryMax: 80 * time.Millisecond,
		PublishTimeout: 300 * time.Millisecond, QueueSize: 8,
	}
}

func newBus(t *testing.T, o redisbus.Options) *redisbus.Bus {
	t.Helper()
	b := redisbus.New(o)
	t.Cleanup(func() { _ = b.Close() })
	return b
}

func waitHealthy(t *testing.T, b *redisbus.Bus) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b.Healthy() == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("bus never became healthy: %v", b.Healthy())
}

func receive(t *testing.T, ch <-chan server.Message) server.Message {
	t.Helper()
	select {
	case m := <-ch:
		return m
	case <-time.After(3 * time.Second):
		t.Fatal("no message received")
		return server.Message{}
	}
}

func TestSignedMessagesTravelBetweenTwoBuses(t *testing.T) {
	mr := miniredis.RunT(t)
	a, b := newBus(t, opts(mr.Addr())), newBus(t, opts(mr.Addr()))
	got := make(chan server.Message, 4)
	if err := b.Subscribe(t.Context(), func(m server.Message) { got <- m }); err != nil {
		t.Fatal(err)
	}
	waitHealthy(t, b)
	want := server.Message{Node: "n1", Kind: "event", AppID: "app", Name: "news", Frame: []byte(`{"event":"e"}`), Except: "1.2"}
	if err := a.Publish(t.Context(), want); err != nil {
		t.Fatal(err)
	}
	m := receive(t, got)
	if m.Node != "n1" || m.Name != "news" || string(m.Frame) != `{"event":"e"}` || m.Except != "1.2" {
		t.Fatalf("got %+v", m)
	}
}

func TestUnsignedAndBadlySignedMessagesAreDropped(t *testing.T) {
	mr := miniredis.RunT(t)
	bus := newBus(t, opts(mr.Addr()))
	got := make(chan server.Message, 4)
	if err := bus.Subscribe(t.Context(), func(m server.Message) { got <- m }); err != nil {
		t.Fatal(err)
	}
	waitHealthy(t, bus)

	attacker := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = attacker.Close() })
	forged, _ := json.Marshal(server.Message{Node: "evil", Kind: "event", AppID: "app", Name: "news", Frame: []byte(`{}`)})
	for _, payload := range []string{
		string(forged), // unsigned message, the old format
		`{"sig":"00","body":` + string(forged) + `}`, // wrong signature
		`not json at all`,
	} {
		if err := attacker.Publish(t.Context(), "test-chan", payload).Err(); err != nil {
			t.Fatal(err)
		}
	}
	// A different secret is as bad as none.
	other := opts(mr.Addr())
	other.Secret = "another-secret"
	if err := newBus(t, other).Publish(t.Context(), server.Message{Node: "evil", Kind: "event"}); err != nil {
		t.Fatal(err)
	}
	// A valid message afterwards proves the earlier ones were read and dropped.
	if err := newBus(t, opts(mr.Addr())).Publish(t.Context(), server.Message{Node: "good", Kind: "event"}); err != nil {
		t.Fatal(err)
	}
	if m := receive(t, got); m.Node != "good" {
		t.Fatalf("first accepted message came from %q", m.Node)
	}
}

func TestPublishNeverBlocksWhenRedisIsDown(t *testing.T) {
	o := opts("127.0.0.1:1")
	o.QueueSize = 2
	bus := newBus(t, o)
	start := time.Now()
	var full int
	for range 200 {
		if err := bus.Publish(t.Context(), server.Message{Node: "n"}); errors.Is(err, redisbus.ErrQueueFull) {
			full++
		}
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("200 publishes took %v, they must not wait for Redis", elapsed)
	}
	if full == 0 {
		t.Fatal("the bounded queue never reported being full")
	}
}

func TestSubscribeRetriesAndReportsHealth(t *testing.T) {
	mr := miniredis.RunT(t)
	addr := mr.Addr()
	mr.Close() // Redis is down at start
	bus := newBus(t, opts(addr))
	got := make(chan server.Message, 4)
	if err := bus.Subscribe(t.Context(), func(m server.Message) { got <- m }); err != nil {
		t.Fatalf("Subscribe must not fail, it retries: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	if bus.Healthy() == nil {
		t.Fatal("bus must report unhealthy while it cannot subscribe")
	}
	if err := mr.Restart(); err != nil {
		t.Fatal(err)
	}
	waitHealthy(t, bus)
	if err := newBus(t, opts(addr)).Publish(t.Context(), server.Message{Node: "n2", Kind: "event"}); err != nil {
		t.Fatal(err)
	}
	if m := receive(t, got); m.Node != "n2" {
		t.Fatalf("got %+v", m)
	}
}

func TestBusRecoversAfterRedisRestarts(t *testing.T) {
	mr := miniredis.RunT(t)
	bus := newBus(t, opts(mr.Addr()))
	got := make(chan server.Message, 4)
	if err := bus.Subscribe(t.Context(), func(m server.Message) { got <- m }); err != nil {
		t.Fatal(err)
	}
	waitHealthy(t, bus)
	mr.Close()
	deadline := time.Now().Add(3 * time.Second)
	for bus.Healthy() == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if bus.Healthy() == nil {
		t.Fatal("lost connection was not reported")
	}
	if err := mr.Restart(); err != nil {
		t.Fatal(err)
	}
	waitHealthy(t, bus)
}

func TestPing(t *testing.T) {
	mr := miniredis.RunT(t)
	bus := newBus(t, opts(mr.Addr()))
	if err := bus.Ping(t.Context()); err != nil {
		t.Fatal(err)
	}
	mr.Close()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := bus.Ping(ctx); err == nil {
		t.Fatal("expected an error after Redis stopped")
	}
}
