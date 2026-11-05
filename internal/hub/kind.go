package hub

import "strings"

// Kind describes what a channel name means.
type Kind uint8

// Kind flags. A name can have several, for example presence-cache-x.
const (
	kindPrivate Kind = 1 << iota
	kindPresence
	kindCache
)

// KindOf classifies a channel by its prefix. It follows Reverb's ChannelBroker,
// which tests the prefix without the dash: "privatefoo" needs auth like "private-foo".
func KindOf(name string) Kind {
	switch {
	case strings.HasPrefix(name, "private-cache-"):
		return kindPrivate | kindCache
	case strings.HasPrefix(name, "presence-cache-"):
		return kindPrivate | kindPresence | kindCache
	case strings.HasPrefix(name, "cache"):
		return kindCache
	case strings.HasPrefix(name, "private"):
		return kindPrivate
	case strings.HasPrefix(name, "presence"):
		return kindPrivate | kindPresence
	}
	return 0
}

// NeedsAuth is true for private and presence channels.
func (k Kind) NeedsAuth() bool { return k&kindPrivate != 0 }

// IsPresence is true for presence channels.
func (k Kind) IsPresence() bool { return k&kindPresence != 0 }

// IsCache is true for channels that replay their last event.
func (k Kind) IsCache() bool { return k&kindCache != 0 }
