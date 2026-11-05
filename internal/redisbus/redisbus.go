// Package redisbus shares events between reverb-go nodes through Redis pub/sub.
//
// The message format is reverb-go's own JSON. Laravel Reverb publishes
// PHP-serialized objects, so a PHP Reverb node and a reverb-go node cannot
// join the same cluster. Use a different channel name for each to be safe.
package redisbus

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/vortechron/reverb-go/internal/server"
)

// Options configure the Redis connection.
type Options struct {
	Addr     string
	Password string
	DB       int
	Channel  string
}

// Bus implements server.Bus.
type Bus struct {
	client  *redis.Client
	channel string
}

// New creates a Bus. It connects lazily.
func New(o Options) *Bus {
	return &Bus{
		client:  redis.NewClient(&redis.Options{Addr: o.Addr, Password: o.Password, DB: o.DB}),
		channel: o.Channel,
	}
}

// Close releases the Redis connection.
func (b *Bus) Close() error { return b.client.Close() }

// Publish sends one message to every node.
func (b *Bus) Publish(ctx context.Context, m server.Message) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("redisbus: encode message: %w", err)
	}
	if err := b.client.Publish(ctx, b.channel, raw).Err(); err != nil {
		return fmt.Errorf("redisbus: publish: %w", err)
	}
	return nil
}

// Subscribe starts a goroutine that calls handle for each message until ctx ends.
// It returns after the subscription is confirmed, or with the error.
func (b *Bus) Subscribe(ctx context.Context, handle func(server.Message)) error {
	sub := b.client.Subscribe(ctx, b.channel)
	if _, err := sub.Receive(ctx); err != nil {
		_ = sub.Close()
		return fmt.Errorf("redisbus: subscribe: %w", err)
	}
	go func() {
		defer func() { _ = sub.Close() }()
		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				var m server.Message
				if json.Unmarshal([]byte(msg.Payload), &m) == nil {
					handle(m)
				}
			}
		}
	}()
	return nil
}

// Ping checks that Redis answers, so the process can fail at start-up.
func (b *Bus) Ping(ctx context.Context) error {
	if err := b.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redisbus: ping: %w", err)
	}
	return nil
}
