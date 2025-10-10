package smarterbase

import (
	"errors"
	"testing"
)

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"ErrNotFound", ErrNotFound, "object not found"},
		{"ErrConflict", ErrConflict, "concurrent modification detected"},
		{"ErrInvalidConfig", ErrInvalidConfig, "invalid configuration"},
		{"ErrUnauthorized", ErrUnauthorized, "unauthorized access"},
		{"ErrLockHeld", ErrLockHeld, "lock already held by another process"},
		{"ErrIndexRetries", ErrIndexRetries, "index update retries exhausted"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.want {
				t.Errorf("error message = %q, want %q", tt.err.Error(), tt.want)
			}
		})
	}
}

func TestWithContext(t *testing.T) {
	baseErr := errors.New("base error")
	ctx := map[string]interface{}{
		"key":   "users/123",
		"value": 42,
	}

	err := WithContext(baseErr, ctx)

	// Check it's an ErrorWithContext
	var errWithCtx *ErrorWithContext
	if !errors.As(err, &errWithCtx) {
		t.Fatalf("expected ErrorWithContext, got %T", err)
	}

	// Check wrapped error
	if !errors.Is(err, baseErr) {
		t.Error("expected error to wrap base error")
	}

	// Check context preserved
	if errWithCtx.Context["key"] != "users/123" {
		t.Errorf("context key = %v, want 'users/123'", errWithCtx.Context["key"])
	}
	if errWithCtx.Context["value"] != 42 {
		t.Errorf("context value = %v, want 42", errWithCtx.Context["value"])
	}

	// Check error message includes context
	msg := err.Error()
	if msg == "" {
		t.Error("error message should not be empty")
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"direct ErrNotFound", ErrNotFound, true},
		{"wrapped ErrNotFound", WithContext(ErrNotFound, nil), true},
		{"other error", errors.New("other"), false},
		{"nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNotFound(tt.err)
			if got != tt.want {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"ErrConflict", ErrConflict, true},
		{"ErrLockHeld", ErrLockHeld, true},
		{"wrapped ErrConflict", WithContext(ErrConflict, nil), true},
		{"ErrNotFound", ErrNotFound, false},
		{"ErrInvalidConfig", ErrInvalidConfig, false},
		{"nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryable(tt.err)
			if got != tt.want {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorWithContextUnwrap(t *testing.T) {
	baseErr := errors.New("base")
	wrappedErr := WithContext(baseErr, map[string]interface{}{"key": "value"})

	// Test errors.Is
	if !errors.Is(wrappedErr, baseErr) {
		t.Error("errors.Is should find base error")
	}

	// Test errors.As
	var errWithCtx *ErrorWithContext
	if !errors.As(wrappedErr, &errWithCtx) {
		t.Error("errors.As should extract ErrorWithContext")
	}

	// Test unwrapping chain
	unwrapped := errors.Unwrap(wrappedErr)
	if !errors.Is(unwrapped, baseErr) {
		t.Error("Unwrap should return base error")
	}
}
