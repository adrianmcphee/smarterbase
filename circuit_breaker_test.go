package smarterbase

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond)

	// Initially closed
	if cb.State() != "closed" {
		t.Errorf("Expected initial state 'closed', got %s", cb.State())
	}

	// Record 3 failures to open circuit
	testErr := errors.New("test error")
	for i := 0; i < 3; i++ {
		cb.Execute(context.Background(), func() error {
			return testErr
		})
	}

	// Circuit should be open
	if cb.State() != "open" {
		t.Errorf("Expected state 'open' after 3 failures, got %s", cb.State())
	}

	// Requests should fail fast when open
	err := cb.Execute(context.Background(), func() error {
		t.Error("Should not execute when circuit is open")
		return nil
	})

	if err == nil {
		t.Error("Expected error when circuit is open")
	}
	if !errors.Is(err, ErrBackendUnavailable) {
		t.Errorf("Expected ErrBackendUnavailable, got %v", err)
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// Circuit should transition to half-open
	cb.Execute(context.Background(), func() error {
		return nil // Success
	})

	if cb.State() != "closed" {
		t.Errorf("Expected state 'closed' after successful half-open request, got %s", cb.State())
	}
}

func TestCircuitBreaker_FailureCount(t *testing.T) {
	cb := NewCircuitBreaker(5, 1*time.Second)

	// Record failures
	testErr := errors.New("test error")
	for i := 0; i < 3; i++ {
		cb.Execute(context.Background(), func() error {
			return testErr
		})
	}

	if cb.Failures() != 3 {
		t.Errorf("Expected 3 failures, got %d", cb.Failures())
	}

	// Success should reset counter in closed state
	cb.Execute(context.Background(), func() error {
		return nil
	})

	if cb.Failures() != 0 {
		t.Errorf("Expected failures reset to 0 after success, got %d", cb.Failures())
	}
}

func TestCircuitBreaker_StateChangeCallback(t *testing.T) {
	var transitions []string

	cb := NewCircuitBreaker(2, 50*time.Millisecond).
		WithStateChangeCallback(func(from, to string) {
			transitions = append(transitions, from+"→"+to)
		})

	// Trigger state transitions
	testErr := errors.New("test error")

	// closed → open
	cb.Execute(context.Background(), func() error { return testErr })
	cb.Execute(context.Background(), func() error { return testErr })

	if len(transitions) == 0 {
		t.Error("Expected state change callback to be called")
	}

	if transitions[0] != "closed→open" {
		t.Errorf("Expected 'closed→open' transition, got %s", transitions[0])
	}

	// Wait for half-open
	time.Sleep(100 * time.Millisecond)

	// open → half-open → closed
	cb.Execute(context.Background(), func() error { return nil })

	if len(transitions) < 2 {
		t.Errorf("Expected at least 2 transitions, got %d", len(transitions))
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(2, 1*time.Second)

	// Trigger open state
	testErr := errors.New("test error")
	cb.Execute(context.Background(), func() error { return testErr })
	cb.Execute(context.Background(), func() error { return testErr })

	if cb.State() != "open" {
		t.Error("Circuit should be open")
	}

	// Manual reset
	cb.Reset()

	if cb.State() != "closed" {
		t.Errorf("Expected state 'closed' after reset, got %s", cb.State())
	}

	if cb.Failures() != 0 {
		t.Errorf("Expected 0 failures after reset, got %d", cb.Failures())
	}
}

func TestCircuitBreaker_HalfOpenRecovery(t *testing.T) {
	cb := NewCircuitBreaker(2, 50*time.Millisecond)

	// Open circuit
	testErr := errors.New("test error")
	cb.Execute(context.Background(), func() error { return testErr })
	cb.Execute(context.Background(), func() error { return testErr })

	// Wait for half-open
	time.Sleep(100 * time.Millisecond)

	// First request in half-open - if it fails, should go back to open
	cb.Execute(context.Background(), func() error { return testErr })

	if cb.State() != "open" {
		t.Errorf("Expected state 'open' after failed half-open request, got %s", cb.State())
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(10, 100*time.Millisecond)

	// Concurrent execution
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			cb.Execute(context.Background(), func() error {
				time.Sleep(10 * time.Millisecond)
				return nil
			})
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should remain closed with successful requests
	if cb.State() != "closed" {
		t.Errorf("Expected state 'closed' after concurrent successful requests, got %s", cb.State())
	}
}
