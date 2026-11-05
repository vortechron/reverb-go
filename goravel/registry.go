package goravel

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/vortechron/reverb-go/broadcast"
)

// ErrNoPresenceMember is returned when a presence rule allows a user but
// returns no member. It is a bug in the rule, not a denial.
var ErrNoPresenceMember = errors.New("reverb: presence rule allowed the user but returned no member")

// Callback decides whether user may join a channel. params holds the values
// matched by the pattern, for example {"id": "5"} for "users.{id}".
//
// A private rule returns (allowed, nil, nil). A presence rule returns the
// member that joins. A returned error is a server fault (500), not a denial.
type Callback func(ctx context.Context, user any, params map[string]string) (allowed bool, presence *broadcast.PresenceMember, err error)

// Decision is the result of Authorize.
type Decision struct {
	Allowed bool
	// Member is set only for an allowed presence channel.
	Member *broadcast.PresenceMember
}

type rule struct {
	pattern string
	re      *regexp.Regexp
	names   []string
	call    Callback
}

// Registry maps channel patterns to rules, like Laravel's routes/channels.php.
// It is safe for concurrent use.
type Registry struct {
	mu    sync.RWMutex
	rules []rule
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{} }

var paramPattern = regexp.MustCompile(`\{([^{}]*)\}`)

// Channel adds a rule. The pattern has no "private-" or "presence-" prefix,
// as in Laravel: "users.{id}" guards "private-users.5" and "presence-users.5".
// A {name} matches one dot-free segment. Rules are tried in order.
func (r *Registry) Channel(pattern string, call Callback) error {
	if pattern == "" || call == nil {
		return errors.New("reverb: a channel rule needs a pattern and a callback")
	}
	rl, err := compile(pattern, call)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.rules {
		if existing.pattern == pattern {
			return fmt.Errorf("reverb: channel pattern %q is already registered", pattern)
		}
	}
	r.rules = append(r.rules, rl)

	return nil
}

func compile(pattern string, call Callback) (rule, error) {
	var names []string
	seen := map[string]bool{}
	var b strings.Builder
	last := 0
	for _, loc := range paramPattern.FindAllStringSubmatchIndex(pattern, -1) {
		name := pattern[loc[2]:loc[3]]
		if name == "" || seen[name] {
			return rule{}, fmt.Errorf("reverb: channel pattern %q has an empty or repeated parameter", pattern)
		}
		seen[name] = true
		names = append(names, name)
		b.WriteString(regexp.QuoteMeta(pattern[last:loc[0]]) + `([^.]+)`)
		last = loc[1]
	}
	b.WriteString(regexp.QuoteMeta(pattern[last:]))
	re, err := regexp.Compile("^" + b.String() + "$")
	if err != nil {
		return rule{}, fmt.Errorf("reverb: compile channel pattern %q: %w", pattern, err)
	}

	return rule{pattern: pattern, re: re, names: names, call: call}, nil
}

// Authorize finds the first rule for a wire channel name ("private-users.5")
// and runs it. A name with no rule, or a public name, is denied.
func (r *Registry) Authorize(ctx context.Context, user any, channel string) (Decision, error) {
	name, presence, ok := stripPrefix(channel)
	if !ok {
		return Decision{}, nil
	}
	r.mu.RLock()
	rules := r.rules
	r.mu.RUnlock()

	for _, rl := range rules {
		match := rl.re.FindStringSubmatch(name)
		if match == nil {
			continue
		}
		params := make(map[string]string, len(rl.names))
		for i, n := range rl.names {
			params[n] = match[i+1]
		}

		return run(ctx, rl, user, params, presence)
	}

	return Decision{}, nil
}

func run(ctx context.Context, rl rule, user any, params map[string]string, presence bool) (Decision, error) {
	allowed, member, err := rl.call(ctx, user, params)
	if err != nil {
		return Decision{}, fmt.Errorf("reverb: channel rule %q: %w", rl.pattern, err)
	}
	if !allowed {
		return Decision{}, nil
	}
	if !presence {
		return Decision{Allowed: true}, nil
	}
	if member == nil {
		return Decision{}, fmt.Errorf("%w (%s)", ErrNoPresenceMember, rl.pattern)
	}

	return Decision{Allowed: true, Member: member}, nil
}

// stripPrefix removes the type prefix. ok is false for a public channel.
func stripPrefix(channel string) (name string, presence, ok bool) {
	if n, found := strings.CutPrefix(channel, "presence-"); found {
		return n, true, true
	}
	if n, found := strings.CutPrefix(channel, "private-"); found {
		return n, false, true
	}

	return "", false, false
}
