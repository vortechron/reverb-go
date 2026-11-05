package hub

import (
	"encoding/json"
	"strings"

	"github.com/vortechron/reverb-go/internal/wire"
)

// Info describes one occupied channel.
type Info struct {
	Name              string
	Kind              Kind
	SubscriptionCount int
	UserCount         int
	Cache             []byte
}

func cacheMiss(name string) []byte { return wire.CacheMiss(name) }

func (ch *channel) info() Info {
	in := Info{Name: ch.name, Kind: ch.kind, SubscriptionCount: len(ch.subs), Cache: ch.cached}
	if ch.kind.IsPresence() {
		seen := map[string]bool{}
		for _, s := range ch.subs {
			seen[s.member.UserID] = true
		}
		in.UserCount = len(seen)
	}
	return in
}

// Channel returns info for one occupied channel.
func (h *Hub) Channel(name string) (Info, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch, ok := h.channels[name]
	if !ok {
		return Info{}, false
	}
	return ch.info(), true
}

// Channels lists occupied channels whose name starts with prefix.
func (h *Hub) Channels(prefix string) []Info {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []Info
	for _, name := range h.sortedNames() {
		if strings.HasPrefix(name, prefix) {
			out = append(out, h.channels[name].info())
		}
	}
	return out
}

// Users lists the user ids of a presence channel. found is false for unknown channels.
func (h *Hub) Users(name string) (ids []json.RawMessage, kind Kind, found bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch, ok := h.channels[name]
	if !ok {
		return nil, 0, false
	}
	users := ch.uniqueUsers()
	for _, u := range users {
		ids = append(ids, u.member.RawUserID)
	}
	return ids, ch.kind, true
}
