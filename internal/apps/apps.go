// Package apps holds the tenant definitions: id, key, secret and limits.
package apps

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ClientEvents says who may send client-* events.
type ClientEvents string

// Client event modes, named as in Reverb's accept_client_events_from.
const (
	ClientEventsMembers ClientEvents = "members"
	ClientEventsAll     ClientEvents = "all"
	ClientEventsOff     ClientEvents = "none"
)

// App is one tenant of the server.
type App struct {
	ID              string
	Key             string
	Secret          string
	AllowedOrigins  []string
	PingInterval    time.Duration
	ActivityTimeout time.Duration
	MaxConnections  int // 0 means no limit
	MaxMessageSize  int64
	ClientEvents    ClientEvents

	// Abuse limits. Zero means no limit.
	MaxChannels        int // channels one connection may join
	MaxPresenceMembers int // distinct users in one presence channel
	MaxChannelDataSize int // bytes of channel_data in a subscribe
	RateLimit          RateLimit
}

// RateLimit follows Reverb's rate_limiting option: at most MaxAttempts client
// messages per Decay window, error 4301 above that, and optionally a disconnect.
type RateLimit struct {
	Enabled     bool
	MaxAttempts int
	Decay       time.Duration
	Terminate   bool
}

// Registry looks apps up by key (WebSocket) or id (HTTP API). It is read-only after creation.
type Registry struct {
	list  []App
	byKey map[string]App
	byID  map[string]App
}

// NewRegistry validates the apps and indexes them.
func NewRegistry(list []App) (*Registry, error) {
	if len(list) == 0 {
		return nil, errors.New("apps: at least one app is required")
	}
	reg := &Registry{list: append([]App(nil), list...), byKey: map[string]App{}, byID: map[string]App{}}
	for _, a := range list {
		if a.ID == "" || a.Key == "" || a.Secret == "" {
			return nil, fmt.Errorf("apps: app %q needs id, key and secret", a.ID)
		}
		if _, dup := reg.byKey[a.Key]; dup {
			return nil, fmt.Errorf("apps: duplicate key for app %q", a.ID)
		}
		if _, dup := reg.byID[a.ID]; dup {
			return nil, fmt.Errorf("apps: duplicate id %q", a.ID)
		}
		reg.byKey[a.Key] = a
		reg.byID[a.ID] = a
	}
	return reg, nil
}

// ByKey finds an app by its public key.
func (r *Registry) ByKey(key string) (App, bool) { a, ok := r.byKey[key]; return a, ok }

// ByID finds an app by its id.
func (r *Registry) ByID(id string) (App, bool) { a, ok := r.byID[id]; return a, ok }

// All returns a copy of every app.
func (r *Registry) All() []App { return append([]App(nil), r.list...) }

// OriginAllowed checks an Origin header against AllowedOrigins, as Reverb does:
// only the host is compared, and "*" inside a pattern matches any text.
func (a App) OriginAllowed(origin string) bool {
	if len(a.AllowedOrigins) == 0 {
		return true
	}
	for _, p := range a.AllowedOrigins {
		if p == "*" {
			return true
		}
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return false
	}
	for _, p := range a.AllowedOrigins {
		if globMatch(p, u.Hostname()) {
			return true
		}
	}
	return false
}

func globMatch(pattern, value string) bool {
	quoted := regexp.QuoteMeta(pattern)
	expr := "^" + strings.ReplaceAll(quoted, `\*`, ".*") + "$"
	ok, err := regexp.MatchString(expr, value)
	return err == nil && ok
}
