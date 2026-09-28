package ratelimit

import (
	"sync"
	"time"
)

type bucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// Limiter holds buckets keyed by a composite key (projectID:routePrefix).
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

func NewLimiter() *Limiter {
	l := &Limiter{buckets: make(map[string]*bucket)}
	go l.cleanupLoop()
	return l
}

// Allow returns true if the request should be permitted.
// ratePerMinute is the max sustained rate; burst equals the rate.
func (l *Limiter) Allow(key string, ratePerMinute int) bool {
	if ratePerMinute <= 0 {
		return true // unlimited
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{
			tokens:     float64(ratePerMinute),
			maxTokens:  float64(ratePerMinute),
			refillRate: float64(ratePerMinute) / 60.0, // per second
			lastRefill: time.Now(),
		}
		l.buckets[key] = b
	}

	// Refill
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}
	b.lastRefill = now

	if b.tokens >= 1 {
		b.tokens -= 1
		return true
	}
	return false
}

func (l *Limiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		l.mu.Lock()
		now := time.Now()
		for k, b := range l.buckets {
			// Remove buckets that haven't been touched in 10 minutes
			if now.Sub(b.lastRefill) > 10*time.Minute {
				delete(l.buckets, k)
			}
		}
		l.mu.Unlock()
	}
}
