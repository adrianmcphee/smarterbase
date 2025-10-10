package smarterbase

import (
	"context"
	"sync"
	"time"
)

// CircuitBreaker prevents cascading failures when dependencies are unavailable.
// Implements the circuit breaker pattern with three states: closed, open, half-open.
//
// States:
//   - Closed: Normal operation, requests pass through
//   - Open: Dependency failing, requests fail fast without calling dependency
//   - Half-Open: Testing if dependency recovered, limited requests allowed
//
// Use case: Wrap Redis operations to prevent cascading failures when Redis is down.
type CircuitBreaker struct {
	mu            sync.RWMutex
	maxFailures   int
	resetTimeout  time.Duration
	failures      int
	lastFailTime  time.Time
	state         string // "closed", "open", "half-open"
	onStateChange func(from, to string)
}

// NewCircuitBreaker creates a circuit breaker.
//
// Parameters:
//   - maxFailures: Number of consecutive failures before opening circuit
//   - resetTimeout: Duration before transitioning from open to half-open
//
// Example:
//
//	cb := NewCircuitBreaker(5, 30*time.Second)
//	err := cb.Execute(ctx, func() error {
//	    return redisClient.Get(ctx, key).Err()
//	})
func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        "closed",
	}
}

// WithStateChangeCallback adds a callback for state transitions.
// Useful for metrics and logging.
func (cb *CircuitBreaker) WithStateChangeCallback(fn func(from, to string)) *CircuitBreaker {
	cb.onStateChange = fn
	return cb
}

// Execute runs fn if circuit is closed or half-open.
// Returns ErrBackendUnavailable if circuit is open.
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	if !cb.allow() {
		return WithContext(ErrBackendUnavailable, map[string]interface{}{
			"reason": "circuit breaker is open",
			"state":  cb.State(),
		})
	}

	err := fn()
	cb.recordResult(err)
	return err
}

// allow checks if request should be allowed based on circuit state
func (cb *CircuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case "open":
		// Check if we should transition to half-open
		if time.Since(cb.lastFailTime) > cb.resetTimeout {
			cb.setState("half-open")
			return true
		}
		return false
	case "half-open":
		return true
	default: // closed
		return true
	}
}

// recordResult updates circuit breaker state based on operation result
func (cb *CircuitBreaker) recordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failures++
		cb.lastFailTime = time.Now()

		if cb.failures >= cb.maxFailures && cb.state != "open" {
			cb.setState("open")
		}
	} else {
		// Success - reset or close circuit
		if cb.state == "half-open" {
			cb.setState("closed")
			cb.failures = 0
		} else if cb.state == "closed" {
			cb.failures = 0
		}
	}
}

// setState transitions to a new state and triggers callback
func (cb *CircuitBreaker) setState(newState string) {
	oldState := cb.state
	cb.state = newState
	if cb.onStateChange != nil {
		cb.onStateChange(oldState, newState)
	}
}

// State returns current circuit breaker state (closed, open, or half-open)
func (cb *CircuitBreaker) State() string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.setState("closed")
}

// Failures returns the current failure count
func (cb *CircuitBreaker) Failures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures
}
