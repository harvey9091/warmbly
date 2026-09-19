package campaign

import (
	"sync"
	"time"
)

// readCache holds a derived read for a few seconds. Both the send plan and
// the workspace capacity walk every mailbox (and the plan every lead) through
// the scheduler's gates, and both are polled by every open dashboard tab, so
// two viewers of one campaign share one computation instead of doubling it.
// The clock is the only input a tick would change, and it does not change
// inside the window.
type readCache[T any] struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[string]cacheEntry[T]
}

type cacheEntry[T any] struct {
	v   T
	exp time.Time
}

func newReadCache[T any](ttl time.Duration) *readCache[T] {
	return &readCache[T]{ttl: ttl, m: map[string]cacheEntry[T]{}}
}

// get returns the cached value while it is fresh.
func (c *readCache[T]) get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || time.Now().After(e.exp) {
		var zero T
		return zero, false
	}
	return e.v, true
}

func (c *readCache[T]) put(key string, v T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	// Sweep expired entries so a long-lived process does not keep every
	// campaign it ever planned.
	for k, e := range c.m {
		if now.After(e.exp) {
			delete(c.m, k)
		}
	}
	c.m[key] = cacheEntry[T]{v: v, exp: now.Add(c.ttl)}
}
