package goravel

import (
	"context"
	"fmt"

	contractshttp "github.com/goravel/framework/contracts/http"

	"github.com/vortechron/reverb-go/broadcast"
)

// Publisher sends one event to channels. *broadcast.Publisher is one; so are
// the log and null publishers the service provider can bind.
type Publisher interface {
	Trigger(ctx context.Context, channels []string, name string, data any, opts ...broadcast.Option) error
}

// Broadcaster publishes events the way Laravel's broadcast() helper does.
type Broadcaster struct {
	pub Publisher
}

// NewBroadcaster wraps a Publisher.
func NewBroadcaster(pub Publisher) *Broadcaster { return &Broadcaster{pub: pub} }

// BroadcastOption changes one Broadcast call.
type BroadcastOption func(ctx context.Context) []broadcast.Option

// ToOthers leaves out the browser tab that caused the event, like Laravel's
// toOthers(). It needs SocketIDMiddleware on the route, which keeps Echo's
// X-Socket-Id header in the context. Without a socket id everyone gets it.
func ToOthers() BroadcastOption {
	return func(ctx context.Context) []broadcast.Option {
		if id := SocketIDFrom(ctx); id != "" {
			return []broadcast.Option{broadcast.WithSocketID(id)}
		}

		return nil
	}
}

// ExceptSocket leaves out one named connection.
func ExceptSocket(id string) BroadcastOption {
	return func(context.Context) []broadcast.Option { return []broadcast.Option{broadcast.WithSocketID(id)} }
}

// Broadcast sends an event to channels. Use the wire names ("private-users.5")
// and the event name and payload Laravel would send (see the reverb-go README).
func (b *Broadcaster) Broadcast(ctx context.Context, channels []string, event string, payload any, opts ...BroadcastOption) error {
	var all []broadcast.Option
	for _, opt := range opts {
		all = append(all, opt(ctx)...)
	}
	if err := b.pub.Trigger(ctx, channels, event, payload, all...); err != nil {
		return fmt.Errorf("broadcast event %s: %w", event, err)
	}

	return nil
}

type socketKey struct{}

// WithSocketID returns a context that carries the sender's socket id.
func WithSocketID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, socketKey{}, id)
}

// SocketIDFrom returns the socket id stored by WithSocketID, or "".
func SocketIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(socketKey{}).(string)

	return id
}

type socketMiddleware struct{}

// SocketIDMiddleware stores the X-Socket-Id header (sent by Echo on every
// request) in the request context, so Broadcast(..., ToOthers()) can find it.
// A header that is not a valid socket id is ignored.
func SocketIDMiddleware() contractshttp.Middleware { return socketMiddleware{} }

func (socketMiddleware) Signature() string { return "reverb.socket_id" }

func (socketMiddleware) Handle(ctx contractshttp.Context) {
	if id := ctx.Request().Header("X-Socket-Id"); broadcast.ValidSocketID(id) {
		ctx.WithContext(WithSocketID(ctx.Context(), id))
	}
	ctx.Request().Next()
}
