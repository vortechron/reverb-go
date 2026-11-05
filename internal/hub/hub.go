// Package hub keeps channel state for one app: who is subscribed, who is
// present, and the last event of cache channels. It never blocks on a client.
package hub

import (
	"sort"
	"sync"

	"github.com/vortechron/reverb-go/internal/wire"
)

// Conn is a connected client. Send must not block; a slow client is dropped by its owner.
type Conn interface {
	ID() string
	Send(frame []byte)
	Close(code int, reason string)
}

type sub struct {
	conn   Conn
	member Member
	seq    uint64
}

type channel struct {
	name   string
	kind   Kind
	subs   map[string]*sub
	cached []byte
}

type entry struct {
	conn     Conn
	channels map[string]struct{}
}

// Hub holds the state for one app.
type Hub struct {
	mu       sync.Mutex
	seq      uint64
	conns    map[string]*entry
	channels map[string]*channel
}

// New creates an empty hub.
func New() *Hub {
	return &Hub{conns: map[string]*entry{}, channels: map[string]*channel{}}
}

// Add registers a connection.
func (h *Hub) Add(c Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[c.ID()] = &entry{conn: c, channels: map[string]struct{}{}}
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
	defer h.mu.Unlock()
	e, ok := h.conns[c.ID()]
	if !ok {
		return
	}
	for name := range e.channels {
		h.leaveLocked(c, name)
	}
	delete(h.conns, c.ID())
}

// Subscribe joins a channel. Credentials must be checked by the caller.
// It sends subscription_succeeded (and the cache frame) to the joiner.
func (h *Hub) Subscribe(c Conn, name, channelData string) error {
	member, err := ParseMember(channelData)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.conns[c.ID()]
	if !ok {
		return nil
	}
	ch := h.channelLocked(name)
	_, rejoin := ch.subs[c.ID()]
	newUser := !rejoin && (!member.HasUser() || !ch.hasUser(member.UserID))
	h.seq++
	ch.subs[c.ID()] = &sub{conn: c, member: member, seq: h.seq}
	e.channels[name] = struct{}{}
	if ch.kind.IsPresence() && newUser {
		ch.broadcast(wire.Internal("member_added", name, member.Raw), c.ID())
	}
	c.Send(wire.Internal("subscription_succeeded", name, ch.successData()))
	if ch.kind.IsCache() {
		ch.replay(c)
	}
	return nil
}

func (h *Hub) channelLocked(name string) *channel {
	ch, ok := h.channels[name]
	if !ok {
		ch = &channel{name: name, kind: KindOf(name), subs: map[string]*sub{}}
		h.channels[name] = ch
	}
	return ch
}

// Unsubscribe leaves one channel.
func (h *Hub) Unsubscribe(c Conn, name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.leaveLocked(c, name)
}

func (h *Hub) leaveLocked(c Conn, name string) {
	ch, ok := h.channels[name]
	if !ok {
		return
	}
	s, ok := ch.subs[c.ID()]
	if !ok {
		return
	}
	delete(ch.subs, c.ID())
	if e, ok := h.conns[c.ID()]; ok {
		delete(e.channels, name)
	}
	if len(ch.subs) == 0 {
		delete(h.channels, name)
		return
	}
	if ch.kind.IsPresence() && s.member.HasUser() && !ch.hasUser(s.member.UserID) {
		data := `{"user_id":` + string(s.member.RawUserID) + `}`
		ch.broadcast(wire.Internal("member_removed", name, data), "")
	}
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
	defer h.mu.Unlock()
	ch, ok := h.channels[name]
	if !ok {
		return
	}
	if remember && ch.kind.IsCache() {
		ch.cached = frame
	}
	ch.broadcast(frame, exceptID)
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

// sortedNames returns channel names in a stable order.
func (h *Hub) sortedNames() []string {
	names := make([]string, 0, len(h.channels))
	for n := range h.channels {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
