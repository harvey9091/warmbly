package formserver

import (
	"sync"
	"time"
)

// ipLimiter is a fixed-window in-memory submission limiter, the same shape
// the tracking service uses for pixels. Budgets are per forms-service
// instance; replicas each carry their own window, which only ever errs
// toward letting a real visitor through.
type ipLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	seen   map[string]*ipWindow
}

type ipWindow struct {
	n     int
	reset time.Time
}

// ipLimiterMax bounds the map against source-address spray; sweeping first,
// then dropping the table, costs a reset budget, not unbounded memory.
const ipLimiterMax = 100_000

func newIPLimiter(limit int, window time.Duration) *ipLimiter {
	return &ipLimiter{limit: limit, window: window, seen: map[string]*ipWindow{}}
}

func (l *ipLimiter) Allow(ip string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	w, ok := l.seen[ip]
	if !ok || now.After(w.reset) {
		if len(l.seen) >= ipLimiterMax {
			for k, e := range l.seen {
				if now.After(e.reset) {
					delete(l.seen, k)
				}
			}
			if len(l.seen) >= ipLimiterMax {
				l.seen = map[string]*ipWindow{}
			}
		}
		l.seen[ip] = &ipWindow{n: 1, reset: now.Add(l.window)}
		return true
	}
	w.n++
	return w.n <= l.limit
}

// ttlSet remembers keys for a fixed duration; Add reports whether the key was
// new. It backs the per-visitor view dedupe.
type ttlSet struct {
	mu   sync.Mutex
	ttl  time.Duration
	seen map[string]time.Time
}

const ttlSetMax = 200_000

func newTTLSet(ttl time.Duration) *ttlSet {
	return &ttlSet{ttl: ttl, seen: map[string]time.Time{}}
}

func (s *ttlSet) Add(key string) bool {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if exp, ok := s.seen[key]; ok && now.Before(exp) {
		return false
	}
	if len(s.seen) >= ttlSetMax {
		for k, exp := range s.seen {
			if now.After(exp) {
				delete(s.seen, k)
			}
		}
		if len(s.seen) >= ttlSetMax {
			s.seen = map[string]time.Time{}
		}
	}
	s.seen[key] = now.Add(s.ttl)
	return true
}
