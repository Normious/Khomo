package circuit

import (
	"sync"
	"time"
)

type State string

const (
	StateClosed   State = "closed"
	StateOpen     State = "open"
	StateHalfOpen State = "half_open"
)

// Breaker is a per-backend circuit breaker.
type Breaker struct {
	mu                   sync.Mutex
	backendID            int64
	state                State
	consecutiveFailures  int
	consecutiveSuccesses int
	lastFailureAt        time.Time
	openedAt             time.Time

	failureThreshold int
	successThreshold int
	timeout          time.Duration

	// Half-open concurrency limiter
	halfOpenInFlight int
	halfOpenMax      int
}

func NewBreaker(backendID int64, failureThreshold, successThreshold int, timeout time.Duration, halfOpenMax int) *Breaker {
	if failureThreshold <= 0 {
		failureThreshold = 5
	}
	if successThreshold <= 0 {
		successThreshold = 2
	}
	if halfOpenMax <= 0 {
		halfOpenMax = 1
	}
	return &Breaker{
		backendID:        backendID,
		state:            StateClosed,
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		timeout:          timeout,
		halfOpenMax:      halfOpenMax,
	}
}

// Allow returns true if the request should be attempted.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		return true

	case StateOpen:
		// Time to try half-open?
		if time.Since(b.openedAt) >= b.timeout {
			b.state = StateHalfOpen
			b.halfOpenInFlight = 0
			b.consecutiveSuccesses = 0
		} else {
			return false
		}
		fallthrough

	case StateHalfOpen:
		if b.halfOpenInFlight >= b.halfOpenMax {
			return false
		}
		b.halfOpenInFlight++
		return true
	}

	return false
}

// RecordSuccess records a successful request.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.consecutiveFailures = 0
	b.consecutiveSuccesses++

	if b.state == StateHalfOpen {
		if b.halfOpenInFlight > 0 {
			b.halfOpenInFlight--
		}
		if b.consecutiveSuccesses >= b.successThreshold {
			b.state = StateClosed
			b.consecutiveSuccesses = 0
		}
	} else if b.state == StateClosed {
		b.consecutiveFailures = 0
	}
}

// RecordFailure records a failed request.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.lastFailureAt = time.Now()
	b.consecutiveSuccesses = 0
	b.consecutiveFailures++

	if b.state == StateHalfOpen {
		if b.halfOpenInFlight > 0 {
			b.halfOpenInFlight--
		}
		b.state = StateOpen
		b.openedAt = time.Now()
		return
	}

	if b.state == StateClosed && b.consecutiveFailures >= b.failureThreshold {
		b.state = StateOpen
		b.openedAt = time.Now()
	}
}

// State returns the current state (thread-safe).
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// Stats returns a snapshot for observability.
func (b *Breaker) Stats() (state State, failures, successes int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state, b.consecutiveFailures, b.consecutiveSuccesses
}
