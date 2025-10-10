package smarterbase

import "time"

// Configuration constants for Smarterbase operations
const (
	// Index update retry configuration
	DefaultMaxRetries      = 3
	DefaultInitialBackoff  = 100 * time.Millisecond
	DefaultBackoffMultiple = 2
	DefaultJitterPercent   = 0.5 // 50% jitter to avoid thundering herd

	// Batch operation configuration
	DefaultBatchSize         = 100
	DefaultListPaginatedSize = 100

	// File backend configuration
	DefaultFilePermissions = 0644
	DefaultDirPermissions  = 0755
)

// RetryConfig holds configuration for retry operations with exponential backoff
type RetryConfig struct {
	MaxRetries      int
	InitialBackoff  time.Duration
	BackoffMultiple int
	JitterPercent   float64
}

// DefaultRetryConfig returns the default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:      DefaultMaxRetries,
		InitialBackoff:  DefaultInitialBackoff,
		BackoffMultiple: DefaultBackoffMultiple,
		JitterPercent:   DefaultJitterPercent,
	}
}

// Validate checks if the RetryConfig is valid
func (c RetryConfig) Validate() error {
	if c.MaxRetries < 0 {
		return WithContext(ErrInvalidConfig, map[string]interface{}{
			"field":  "MaxRetries",
			"value":  c.MaxRetries,
			"reason": "must be non-negative",
		})
	}
	if c.InitialBackoff <= 0 {
		return WithContext(ErrInvalidConfig, map[string]interface{}{
			"field":  "InitialBackoff",
			"value":  c.InitialBackoff,
			"reason": "must be positive",
		})
	}
	if c.BackoffMultiple < 1 {
		return WithContext(ErrInvalidConfig, map[string]interface{}{
			"field":  "BackoffMultiple",
			"value":  c.BackoffMultiple,
			"reason": "must be >= 1",
		})
	}
	if c.JitterPercent < 0 || c.JitterPercent > 1 {
		return WithContext(ErrInvalidConfig, map[string]interface{}{
			"field":  "JitterPercent",
			"value":  c.JitterPercent,
			"reason": "must be between 0 and 1",
		})
	}
	return nil
}
