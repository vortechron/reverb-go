package hub

import (
	"encoding/json"
	"sort"
	"strings"
)

// Info describes one occupied channel.
type Info struct {
	Name              string
	Kind              Kind
	SubscriptionCount int
	UserCount         int
	Cache             []byte
}

func (ch *channel) info() Info {
	return Info{
		Name: ch.name, Kind: ch.kind, SubscriptionCount: len(ch.subs),
		UserCount: len(ch.users), Cache: ch.cached,
	}
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

// Channels lists occupied channels whose name starts with prefix, sorted by name.
func (h *Hub) Channels(prefix string) []Info {
	h.mu.Lock()
	var out []Info
	for name, ch := range h.channels {
		if strings.HasPrefix(name, prefix) {
			out = append(out, ch.info())
		}
	}
	h.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
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
	for _, u := range ch.users {
		ids = append(ids, u.rawID)
	}
	return ids, ch.kind, true
}
