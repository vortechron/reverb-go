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

// KindOf classifies a channel by its prefix.
func KindOf(name string) Kind {
	var k Kind
	switch {
	case strings.HasPrefix(name, "presence-"):
		k = kindPrivate | kindPresence
	case strings.HasPrefix(name, "private-"):
		k = kindPrivate
	}
	if strings.HasPrefix(name, "cache-") || strings.HasPrefix(name, "private-cache-") ||
		strings.HasPrefix(name, "presence-cache-") {
		k |= kindCache
	}
	return k
}

// NeedsAuth is true for private and presence channels.
func (k Kind) NeedsAuth() bool { return k&kindPrivate != 0 }

// IsPresence is true for presence channels.
func (k Kind) IsPresence() bool { return k&kindPresence != 0 }

// IsCache is true for channels that replay their last event.
func (k Kind) IsCache() bool { return k&kindCache != 0 }
