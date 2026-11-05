// Package redisbus shares events between reverb-go nodes through Redis pub/sub.
//
// The message format is reverb-go's own JSON. Laravel Reverb publishes
// PHP-serialized objects, so a PHP Reverb node and a reverb-go node cannot
// join the same cluster. Use a different channel name for each to be safe.
//
// Every message is signed with HMAC-SHA256 using a shared secret. A node drops
// anything that is not signed with that secret, so someone who can only write
// to Redis cannot inject events. Redis itself must still require a password.
//
// Publishing never blocks the caller: messages go to a bounded queue and one
// goroutine sends them with a timeout. Subscribing retries with backoff and
// Healthy reports the state, so /up can show a broken bus.
package redisbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/vortechron/reverb-go/broadcast"
	"github.com/vortechron/reverb-go/internal/server"
)

// ErrQueueFull is returned by Publish when Redis cannot keep up. The message is dropped.
var ErrQueueFull = errors.New("redisbus: publish queue is full")

// Options configure the Redis connection.
type Options struct {
	Addr     string
	Password string
	DB       int
	Channel  string
	// Secret signs and verifies messages. Required.
	Secret string
	// QueueSize bounds the publish queue. Default 1024.
	QueueSize int
	// PublishTimeout limits one publish. Default 2 seconds.
	PublishTimeout time.Duration
	// RetryMin and RetryMax bound the subscribe backoff. Default 500 ms and 30 s.
	RetryMin, RetryMax time.Duration
	Logger             *slog.Logger
}

// envelope is what travels through Redis.
type envelope struct {
	Sig  string          `json:"sig"`
	Body json.RawMessage `json:"body"`
}

// Bus implements server.Bus.
type Bus struct {
	client *redis.Client
	opts   Options
	log    *slog.Logger
	queue  chan []byte
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu         sync.Mutex
	subscribed bool  // Subscribe was called
	subErr     error // last subscribe problem, nil when healthy
}

// New creates a Bus and starts its publisher goroutine. Redis is contacted lazily.
func New(o Options) *Bus {
	if o.QueueSize <= 0 {
		o.QueueSize = 1024
	}
	if o.PublishTimeout <= 0 {
		o.PublishTimeout = 2 * time.Second
	}
	if o.RetryMin <= 0 {
		o.RetryMin = 500 * time.Millisecond
	}
	if o.RetryMax < o.RetryMin {
		o.RetryMax = 30 * time.Second
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	ctx, cancel := context.WithCancel(context.Background())
	b := &Bus{
		client: redis.NewClient(&redis.Options{Addr: o.Addr, Password: o.Password, DB: o.DB, DialTimeout: time.Second}),
		opts:   o, log: o.Logger, queue: make(chan []byte, o.QueueSize), ctx: ctx, cancel: cancel,
	}
	b.wg.Add(1)
	go b.publishLoop()
	return b
}

// Close stops the goroutines and releases the Redis connection.
func (b *Bus) Close() error {
	b.cancel()
	b.wg.Wait()
	return b.client.Close()
}

// Ping checks that Redis answers, so the process can fail at start-up.
func (b *Bus) Ping(ctx context.Context) error {
	if err := b.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redisbus: ping: %w", err)
	}
	return nil
}

// Healthy returns nil when the bus works. After Subscribe it reports the last subscribe error.
func (b *Bus) Healthy() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.subscribed {
		return nil
	}
	return b.subErr
}

func (b *Bus) setErr(err error) {
	b.mu.Lock()
	b.subErr = err
	b.mu.Unlock()
}

func (b *Bus) sign(body []byte) string { return broadcast.HMACHex(b.opts.Secret, string(body)) }

// Publish queues a signed message for every node. It never waits for Redis.
func (b *Bus) Publish(_ context.Context, m server.Message) error {
	body, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("redisbus: encode message: %w", err)
	}
	payload, err := json.Marshal(envelope{Sig: b.sign(body), Body: body})
	if err != nil {
		return fmt.Errorf("redisbus: encode envelope: %w", err)
	}
	select {
	case b.queue <- payload:
		return nil
	default:
		return ErrQueueFull
	}
}

func (b *Bus) publishLoop() {
	defer b.wg.Done()
	for {
		select {
		case <-b.ctx.Done():
			return
		case payload := <-b.queue:
			ctx, cancel := context.WithTimeout(b.ctx, b.opts.PublishTimeout)
			if err := b.client.Publish(ctx, b.opts.Channel, payload).Err(); err != nil {
				b.log.Warn("redis publish failed", "error", err)
			}
			cancel()
		}
	}
}
