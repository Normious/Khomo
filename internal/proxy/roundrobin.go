package proxy

import (
	"sync"

	"github.com/Normious/Khomo/internal/circuit"
	"github.com/Normious/Khomo/internal/db"
)

// BackendPool holds one route's backends + per-backend circuit breakers.
type BackendPool struct {
	mu       sync.Mutex
	route    *db.Route
	backends []*db.Backend
	breakers map[int64]*circuit.Breaker
	counter  uint64
}

func NewBackendPool(route *db.Route, backends []*db.Backend, breakers map[int64]*circuit.Breaker) *BackendPool {
	return &BackendPool{
		route:    route,
		backends: backends,
		breakers: breakers,
	}
}

// Select returns the next healthy backend using round-robin + circuit state.
// Returns nil if no backend is available.
func (p *BackendPool) Select() *db.Backend {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.backends) == 0 {
		return nil
	}

	// Try each backend once, starting at counter
	n := len(p.backends)
	for i := 0; i < n; i++ {
		idx := int((p.counter + uint64(i)) % uint64(n))
		b := p.backends[idx]

		breaker, ok := p.breakers[b.ID]
		if !ok {
			continue
		}
		if breaker.Allow() {
			p.counter = uint64(idx) + 1
			return b
		}
	}

	return nil // All backends circuit-open
}

// RecordSuccess is called after a successful proxy attempt.
func (p *BackendPool) RecordSuccess(b *db.Backend) {
	if breaker, ok := p.breakers[b.ID]; ok {
		breaker.RecordSuccess()
	}
}

// RecordFailure is called after a failed proxy attempt.
func (p *BackendPool) RecordFailure(b *db.Backend) {
	if breaker, ok := p.breakers[b.ID]; ok {
		breaker.RecordFailure()
	}
}

// BreakerFor returns the breaker for a given backend (for observability).
func (p *BackendPool) BreakerFor(backendID int64) *circuit.Breaker {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.breakers[backendID]
}
