package smarterbase

import (
	"errors"
	"fmt"
)

// Sentinel errors for common conditions
var (
	// Data errors
	ErrNotFound      = errors.New("object not found")
	ErrAlreadyExists = errors.New("object already exists")
	ErrConflict      = errors.New("concurrent modification detected")
	ErrInvalidData   = errors.New("invalid data format")

	// Backend errors
	ErrBackendUnavailable = errors.New("backend unavailable")
	ErrUnauthorized       = errors.New("unauthorized access")
	ErrTimeout            = errors.New("operation timed out")
	ErrQuotaExceeded      = errors.New("storage quota exceeded")

	// Index errors
	ErrIndexCorrupted = errors.New("index corrupted, repair needed")
	ErrIndexRetries   = errors.New("index update retries exhausted")
	ErrIndexMismatch  = errors.New("index does not match data")

	// Lock errors
	ErrLockHeld       = errors.New("lock already held by another process")
	ErrLockTimeout    = errors.New("failed to acquire lock within timeout")
	ErrLockReleased   = errors.New("lock was already released")
	ErrLockNotFound   = errors.New("lock not found")
	ErrInvalidLockKey = errors.New("invalid lock key")

	// Transaction errors
	ErrTransactionFailed  = errors.New("transaction failed")
	ErrRollbackFailed     = errors.New("transaction rollback failed")
	ErrTransactionTimeout = errors.New("transaction timed out")

	// Configuration errors
	ErrInvalidConfig = errors.New("invalid configuration")
)

// ErrorWithContext adds additional context to errors for better debugging and logging
type ErrorWithContext struct {
	Err     error
	Context map[string]interface{}
}

func (e *ErrorWithContext) Error() string {
	if len(e.Context) == 0 {
		return e.Err.Error()
	}
	return fmt.Sprintf("%v (context: %+v)", e.Err, e.Context)
}

func (e *ErrorWithContext) Unwrap() error {
	return e.Err
}

// WithContext adds context to an error
func WithContext(err error, context map[string]interface{}) error {
	if err == nil {
		return nil
	}
	return &ErrorWithContext{
		Err:     err,
		Context: context,
	}
}

// Common error checking helpers

// IsNotFound checks if an error is a "not found" error
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsConflict checks if an error is a conflict/concurrent modification error
func IsConflict(err error) bool {
	return errors.Is(err, ErrConflict) || errors.Is(err, ErrIndexRetries)
}

// IsRetryable checks if an error is safe to retry
func IsRetryable(err error) bool {
	return errors.Is(err, ErrTimeout) ||
		errors.Is(err, ErrBackendUnavailable) ||
		errors.Is(err, ErrConflict) ||
		errors.Is(err, ErrLockHeld) ||
		errors.Is(err, ErrLockTimeout)
}

// IsPermanent checks if an error is permanent (not retryable)
func IsPermanent(err error) bool {
	return errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrUnauthorized) ||
		errors.Is(err, ErrInvalidData) ||
		errors.Is(err, ErrInvalidConfig)
}
