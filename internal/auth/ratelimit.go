package auth

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Limiter throttles attempts per key (e.g. client IP or email).
type Limiter struct {
	mu      sync.Mutex
	entries map[string]*limiterEntry
	every   time.Duration
	burst   int
}

type limiterEntry struct {
	lim  *rate.Limiter
	seen time.Time
}

// NewLimiter allows burst attempts, refilling one every `every`.
func NewLimiter(every time.Duration, burst int) *Limiter {
	return &Limiter{entries: map[string]*limiterEntry{}, every: every, burst: burst}
}

// Allow reports whether an attempt for key is permitted now.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.entries) > 10_000 { // bound memory: drop idle entries
		for k, e := range l.entries {
			if now.Sub(e.seen) > 10*l.every*time.Duration(l.burst) {
				delete(l.entries, k)
			}
		}
	}
	e, ok := l.entries[key]
	if !ok {
		e = &limiterEntry{lim: rate.NewLimiter(rate.Every(l.every), l.burst)}
		l.entries[key] = e
	}
	e.seen = now
	return e.lim.Allow()
}
