package smarterbase

import (
	"testing"
	"time"
)

func TestRetryConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  RetryConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: RetryConfig{
				MaxRetries:      3,
				InitialBackoff:  10 * time.Millisecond,
				BackoffMultiple: 2,
				JitterPercent:   0.1,
			},
			wantErr: false,
		},
		{
			name: "default config",
			config: RetryConfig{
				MaxRetries:      5,
				InitialBackoff:  50 * time.Millisecond,
				BackoffMultiple: 2,
				JitterPercent:   0.2,
			},
			wantErr: false,
		},
		{
			name: "zero retries valid",
			config: RetryConfig{
				MaxRetries:      0,
				InitialBackoff:  10 * time.Millisecond,
				BackoffMultiple: 2,
				JitterPercent:   0.1,
			},
			wantErr: false,
		},
		{
			name: "negative retries invalid",
			config: RetryConfig{
				MaxRetries:      -1,
				InitialBackoff:  10 * time.Millisecond,
				BackoffMultiple: 2,
				JitterPercent:   0.1,
			},
			wantErr: true,
		},
		{
			name: "zero backoff invalid",
			config: RetryConfig{
				MaxRetries:      3,
				InitialBackoff:  0,
				BackoffMultiple: 2,
				JitterPercent:   0.1,
			},
			wantErr: true,
		},
		{
			name: "negative backoff invalid",
			config: RetryConfig{
				MaxRetries:      3,
				InitialBackoff:  -1 * time.Millisecond,
				BackoffMultiple: 2,
				JitterPercent:   0.1,
			},
			wantErr: true,
		},
		{
			name: "negative jitter invalid",
			config: RetryConfig{
				MaxRetries:      3,
				InitialBackoff:  10 * time.Millisecond,
				BackoffMultiple: 2,
				JitterPercent:   -0.1,
			},
			wantErr: true,
		},
		{
			name: "jitter > 1 invalid",
			config: RetryConfig{
				MaxRetries:      3,
				InitialBackoff:  10 * time.Millisecond,
				BackoffMultiple: 2,
				JitterPercent:   1.5,
			},
			wantErr: true,
		},
		{
			name: "jitter exactly 1 valid",
			config: RetryConfig{
				MaxRetries:      3,
				InitialBackoff:  10 * time.Millisecond,
				BackoffMultiple: 2,
				JitterPercent:   1.0,
			},
			wantErr: false,
		},
		{
			name: "zero jitter valid",
			config: RetryConfig{
				MaxRetries:      3,
				InitialBackoff:  10 * time.Millisecond,
				BackoffMultiple: 2,
				JitterPercent:   0.0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			// If error expected, verify it's ErrInvalidConfig
			if tt.wantErr && err != nil {
				if !IsInvalidConfig(err) {
					t.Errorf("expected ErrInvalidConfig, got %v", err)
				}
			}
		})
	}
}

func TestBackendConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  BackendConfig
		wantErr bool
	}{
		{
			name: "valid S3 config",
			config: BackendConfig{
				Type:   "s3",
				Bucket: "my-bucket",
				Region: "us-west-2",
			},
			wantErr: false,
		},
		{
			name: "valid filesystem config",
			config: BackendConfig{
				Type:   "filesystem",
				Bucket: "/tmp/data",
			},
			wantErr: false,
		},
		{
			name: "empty type invalid",
			config: BackendConfig{
				Type:   "",
				Bucket: "my-bucket",
			},
			wantErr: true,
		},
		{
			name: "S3 without bucket invalid",
			config: BackendConfig{
				Type:   "s3",
				Region: "us-west-2",
			},
			wantErr: true,
		},
		{
			name: "S3 without region invalid",
			config: BackendConfig{
				Type:   "s3",
				Bucket: "my-bucket",
			},
			wantErr: true,
		},
		{
			name: "filesystem without bucket invalid",
			config: BackendConfig{
				Type: "filesystem",
			},
			wantErr: true,
		},
		{
			name: "unknown type invalid",
			config: BackendConfig{
				Type:   "unknown",
				Bucket: "/tmp",
			},
			wantErr: true,
		},
		{
			name: "S3 with endpoint valid",
			config: BackendConfig{
				Type:     "s3",
				Bucket:   "my-bucket",
				Region:   "us-west-2",
				Endpoint: "https://s3.custom.com",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			// If error expected, verify it's ErrInvalidConfig
			if tt.wantErr && err != nil {
				if !IsInvalidConfig(err) {
					t.Errorf("expected ErrInvalidConfig, got %v", err)
				}
			}
		})
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	// Verify it's valid
	if err := config.Validate(); err != nil {
		t.Errorf("DefaultRetryConfig should be valid: %v", err)
	}

	// Verify reasonable defaults
	if config.MaxRetries <= 0 {
		t.Errorf("MaxRetries = %d, want > 0", config.MaxRetries)
	}
	if config.InitialBackoff <= 0 {
		t.Errorf("InitialBackoff = %v, want > 0", config.InitialBackoff)
	}
	if config.JitterPercent < 0 || config.JitterPercent > 1 {
		t.Errorf("JitterPercent = %f, want [0, 1]", config.JitterPercent)
	}
}

// Helper function to check if error is ErrInvalidConfig
func IsInvalidConfig(err error) bool {
	if err == nil {
		return false
	}
	// Check if it's directly ErrInvalidConfig or wrapped
	var errWithCtx *ErrorWithContext
	if IsError(err, ErrInvalidConfig) {
		return true
	}
	if AsError(err, &errWithCtx) {
		return IsError(errWithCtx.Err, ErrInvalidConfig)
	}
	return false
}

// Helper functions that mirror errors.Is and errors.As for our error types
func IsError(err, target error) bool {
	return err == target
}

func AsError(err error, target interface{}) bool {
	if errWithCtx, ok := err.(*ErrorWithContext); ok {
		if ptr, ok := target.(**ErrorWithContext); ok {
			*ptr = errWithCtx
			return true
		}
	}
	return false
}

func TestRetryConfigDefaults(t *testing.T) {
	config := DefaultRetryConfig()

	// Document the expected defaults
	expectedRetries := 3
	expectedBackoff := 100 * time.Millisecond
	expectedMultiple := 2
	expectedJitter := 0.5

	if config.MaxRetries != expectedRetries {
		t.Errorf("MaxRetries = %d, want %d", config.MaxRetries, expectedRetries)
	}
	if config.InitialBackoff != expectedBackoff {
		t.Errorf("InitialBackoff = %v, want %v", config.InitialBackoff, expectedBackoff)
	}
	if config.BackoffMultiple != expectedMultiple {
		t.Errorf("BackoffMultiple = %d, want %d", config.BackoffMultiple, expectedMultiple)
	}
	if config.JitterPercent != expectedJitter {
		t.Errorf("JitterPercent = %f, want %f", config.JitterPercent, expectedJitter)
	}
}
