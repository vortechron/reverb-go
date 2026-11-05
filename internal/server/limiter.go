package server

import (
	"time"

	"github.com/vortechron/reverb-go/internal/apps"
)

// limiter is a fixed window counter, like Reverb's RateLimiter. Only the
// connection's read goroutine uses it, so it needs no lock.
type limiter struct {
	cfg    apps.RateLimit
	count  int
	window time.Time
}

func newLimiter(cfg apps.RateLimit) *limiter {
	if !cfg.Enabled || cfg.MaxAttempts <= 0 {
		return nil
	}
	return &limiter{cfg: cfg}
}

// allow counts one message and reports whether it is within the limit.
func (l *limiter) allow(now time.Time) bool {
	if l == nil {
		return true
	}
	if l.window.IsZero() || now.Sub(l.window) >= l.cfg.Decay {
		l.window, l.count = now, 0
	}
	l.count++
	return l.count <= l.cfg.MaxAttempts
}
