package redisbus

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/vortechron/reverb-go/internal/server"
)

// Subscribe starts a goroutine that calls handle for every valid message until
// ctx or the bus ends. It returns at once. If Redis is not reachable it keeps
// retrying with backoff, and Healthy reports the problem meanwhile.
func (b *Bus) Subscribe(ctx context.Context, handle func(server.Message)) error {
	b.mu.Lock()
	b.subscribed = true
	b.subErr = errors.New("redisbus: not subscribed yet")
	b.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(b.ctx, cancel) //nolint:contextcheck // the bus owns its goroutines and ends them in Close
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer stop()
		defer cancel()
		b.subscribeLoop(ctx, handle)
	}()
	return nil
}

func (b *Bus) subscribeLoop(ctx context.Context, handle func(server.Message)) {
	delay := b.opts.RetryMin
	for ctx.Err() == nil {
		healthy, err := b.session(ctx, handle)
		if ctx.Err() != nil {
			return
		}
		b.setErr(err)
		b.log.Warn("redis subscription lost, retrying", "error", err, "in", delay)
		if healthy {
			delay = b.opts.RetryMin
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, b.opts.RetryMax)
	}
}

// session runs one subscription until it breaks. healthy tells whether it ever worked.
func (b *Bus) session(ctx context.Context, handle func(server.Message)) (healthy bool, err error) {
	sub := b.client.Subscribe(ctx, b.opts.Channel)
	defer func() { _ = sub.Close() }()
	// A blocked read does not watch the context, so close the subscription instead.
	defer context.AfterFunc(ctx, func() { _ = sub.Close() })()
	confirmCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	_, err = sub.Receive(confirmCtx)
	cancel()
	if err != nil {
		return false, fmt.Errorf("redisbus: subscribe: %w", err)
	}
	b.setErr(nil)
	for {
		msg, err := sub.Receive(ctx)
		if err != nil {
			return true, fmt.Errorf("redisbus: receive: %w", err)
		}
		if m, ok := msg.(*redis.Message); ok {
			b.deliver(m.Payload, handle)
		}
	}
}

// deliver verifies the signature and passes valid messages on. Everything else is dropped.
func (b *Bus) deliver(payload string, handle func(server.Message)) {
	var env envelope
	if err := json.Unmarshal([]byte(payload), &env); err != nil || len(env.Body) == 0 {
		b.log.Warn("dropping malformed bus message")
		return
	}
	if !hmac.Equal([]byte(b.sign(env.Body)), []byte(env.Sig)) {
		b.log.Warn("dropping bus message with a bad signature")
		return
	}
	var m server.Message
	if err := json.Unmarshal(env.Body, &m); err != nil {
		b.log.Warn("dropping undecodable bus message")
		return
	}
	handle(m)
}
