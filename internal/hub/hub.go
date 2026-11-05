// Package hub keeps channel state for one app: who is subscribed, who is
// present, and the last event of cache channels.
//
// The hub never sends to a client while holding its lock, except the single
// frame that must reach a joining client before anything else. Fan-out runs on
// an immutable snapshot of the subscribers.
package hub

import (
	"errors"
	"sync"

	"github.com/vortechron/reverb-go/internal/wire"
)

// Errors returned by Subscribe. The server maps them to pusher:error codes.
var (
	ErrTooManyChannels = errors.New("hub: connection is subscribed to too many channels")
	ErrChannelFull     = errors.New("hub: presence channel is full")
	ErrMemberRequired  = errors.New("hub: presence channels need channel_data with a user_id")
)

// Limits cap what one subscribe may do. Zero means no limit.
type Limits struct {
	MaxChannels int // channels per connection
	MaxMembers  int // distinct users per presence channel
}

// Conn is a connected client. Send must not block; a slow client is dropped by its owner.
type Conn interface {
	ID() string
	Send(frame []byte)
	Close(code int, reason string)
}

type entry struct {
	conn     Conn
	channels map[string]struct{}
}

// Hub holds the state for one app.
type Hub struct {
	mu       sync.Mutex
	conns    map[string]*entry
	channels map[string]*channel
}

// New creates an empty hub.
func New() *Hub {
	return &Hub{conns: map[string]*entry{}, channels: map[string]*channel{}}
}

// fanout is a frame and the connections that must receive it, sent after unlock.
type fanout struct {
	targets []Conn
	frame   []byte
	except  string
}

func (f fanout) send() {
	for _, c := range f.targets {
		if c.ID() != f.except {
			c.Send(f.frame)
		}
	}
}

func sendAll(list []fanout) {
	for _, f := range list {
		f.send()
	}
}

// TryAdd registers a connection unless the hub already holds limit connections
// (0 means no limit). The check and the insert happen under one lock, so
// parallel connects cannot pass the limit together.
func (h *Hub) TryAdd(c Conn, limit int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if limit > 0 && len(h.conns) >= limit {
		return false
	}
	h.conns[c.ID()] = &entry{conn: c, channels: map[string]struct{}{}}
	return true
}

// Remove forgets a connection and leaves every channel it joined.
func (h *Hub) Remove(c Conn) {
	h.mu.Lock()
	e, ok := h.conns[c.ID()]
	if !ok {
		h.mu.Unlock()
		return
	}
	var out []fanout
	for name := range e.channels {
		out = append(out, h.leaveLocked(c, name)...)
	}
	delete(h.conns, c.ID())
	h.mu.Unlock()
	sendAll(out)
}

// Subscribe joins a channel. Credentials must be checked by the caller.
// It sends subscription_succeeded (and the cache frame) to the joiner first.
func (h *Hub) Subscribe(c Conn, name, channelData string, lim Limits) error {
	member, err := ParseMember(channelData)
	if err != nil {
		return err
	}
	kind := KindOf(name)
	if kind.IsPresence() && !member.HasUser() {
		return ErrMemberRequired
	}
	h.mu.Lock()
	e, ok := h.conns[c.ID()]
	if !ok {
		h.mu.Unlock()
		return nil
	}
	ch, created := h.channelLocked(name)
	if err := h.admitLocked(e, ch, member, lim); err != nil {
		if created {
			delete(h.channels, name)
		}
		h.mu.Unlock()
		return err
	}
	out := ch.join(c, member)
	e.channels[name] = struct{}{}
	c.Send(wire.Internal("subscription_succeeded", name, ch.successData()))
	if kind.IsCache() {
		ch.replay(c)
	}
	h.mu.Unlock()
	sendAll(out)
	return nil
}

func (h *Hub) admitLocked(e *entry, ch *channel, m Member, lim Limits) error {
	_, joined := ch.subs[e.conn.ID()]
	if !joined && lim.MaxChannels > 0 && len(e.channels) >= lim.MaxChannels {
		return ErrTooManyChannels
	}
	if ch.kind.IsPresence() && lim.MaxMembers > 0 && ch.users[m.UserID] == nil && len(ch.users) >= lim.MaxMembers {
		return ErrChannelFull
	}
	return nil
}

func (h *Hub) channelLocked(name string) (ch *channel, created bool) {
	ch, ok := h.channels[name]
	if !ok {
		ch = newChannel(name)
		h.channels[name] = ch
	}
	return ch, !ok
}

// Unsubscribe leaves one channel.
func (h *Hub) Unsubscribe(c Conn, name string) {
	h.mu.Lock()
	out := h.leaveLocked(c, name)
	h.mu.Unlock()
	sendAll(out)
}

func (h *Hub) leaveLocked(c Conn, name string) []fanout {
	ch, ok := h.channels[name]
	if !ok {
		return nil
	}
	out := ch.leave(c)
	if e, ok := h.conns[c.ID()]; ok {
		delete(e.channels, name)
	}
	if len(ch.subs) == 0 {
		delete(h.channels, name)
	}
	return out
}

// Publish sends a ready-made frame to a channel, skipping one socket id.
// Cache channels remember the frame. Unknown channels are ignored.
func (h *Hub) Publish(name string, frame []byte, exceptID string) {
	h.publish(name, frame, exceptID, true)
}

// PublishNoCache sends a frame without touching the cache of cache channels.
// Client events (whispers) use it, so a subscriber cannot overwrite the event
// that later subscribers receive.
func (h *Hub) PublishNoCache(name string, frame []byte, exceptID string) {
	h.publish(name, frame, exceptID, false)
}

func (h *Hub) publish(name string, frame []byte, exceptID string, remember bool) {
	h.mu.Lock()
	ch, ok := h.channels[name]
	if !ok {
		h.mu.Unlock()
		return
	}
	if remember && ch.kind.IsCache() {
		ch.cached = frame
	}
	targets := ch.conns()
	h.mu.Unlock()
	fanout{targets: targets, frame: frame, except: exceptID}.send()
}

// Member returns the subscription data of c on a channel.
func (h *Hub) Member(c Conn, name string) (Member, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.channels[name]; ok {
		if s, ok := ch.subs[c.ID()]; ok {
			return s.member, true
		}
	}
	return Member{}, false
}

// Terminate closes every connection that joined a channel as userID.
func (h *Hub) Terminate(userID string) {
	if userID == "" {
		return
	}
	h.mu.Lock()
	victims := map[string]Conn{}
	for _, ch := range h.channels {
		for id, s := range ch.subs {
			if s.member.UserID == userID {
				victims[id] = s.conn
			}
		}
	}
	h.mu.Unlock()
	for _, c := range victims {
		c.Close(1000, "terminated")
	}
}

// ConnectionCount is the number of registered connections.
func (h *Hub) ConnectionCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.conns)
}
