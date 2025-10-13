package smarterbase

import (
	"os"
	"testing"
)

func TestRedisOptions_Defaults(t *testing.T) {
	// Clear env vars to test defaults
	os.Unsetenv("REDIS_ADDR")
	os.Unsetenv("REDIS_PASSWORD")
	os.Unsetenv("REDIS_DB")

	opts := RedisOptions()

	if opts.Addr != "localhost:6379" {
		t.Errorf("expected default addr localhost:6379, got %s", opts.Addr)
	}

	if opts.Password != "" {
		t.Errorf("expected default password empty, got %s", opts.Password)
	}

	if opts.DB != 0 {
		t.Errorf("expected default db 0, got %d", opts.DB)
	}
}

func TestRedisOptions_FromEnvironment(t *testing.T) {
	// Set env vars
	os.Setenv("REDIS_ADDR", "redis.example.com:6380")
	os.Setenv("REDIS_PASSWORD", "secret123")
	os.Setenv("REDIS_DB", "5")
	defer func() {
		os.Unsetenv("REDIS_ADDR")
		os.Unsetenv("REDIS_PASSWORD")
		os.Unsetenv("REDIS_DB")
	}()

	opts := RedisOptions()

	if opts.Addr != "redis.example.com:6380" {
		t.Errorf("expected addr redis.example.com:6380, got %s", opts.Addr)
	}

	if opts.Password != "secret123" {
		t.Errorf("expected password secret123, got %s", opts.Password)
	}

	if opts.DB != 5 {
		t.Errorf("expected db 5, got %d", opts.DB)
	}
}

func TestRedisOptions_PartialEnvironment(t *testing.T) {
	// Only set some env vars
	os.Setenv("REDIS_ADDR", "redis.prod.example.com:6379")
	defer os.Unsetenv("REDIS_ADDR")
	os.Unsetenv("REDIS_PASSWORD")
	os.Unsetenv("REDIS_DB")

	opts := RedisOptions()

	if opts.Addr != "redis.prod.example.com:6379" {
		t.Errorf("expected addr redis.prod.example.com:6379, got %s", opts.Addr)
	}

	if opts.Password != "" {
		t.Errorf("expected password empty (default), got %s", opts.Password)
	}

	if opts.DB != 0 {
		t.Errorf("expected db 0 (default), got %d", opts.DB)
	}
}

func TestRedisOptions_InvalidDB(t *testing.T) {
	// Set invalid DB value
	os.Setenv("REDIS_DB", "invalid")
	defer os.Unsetenv("REDIS_DB")

	opts := RedisOptions()

	// Should fall back to default
	if opts.DB != 0 {
		t.Errorf("expected db 0 (default for invalid value), got %d", opts.DB)
	}
}

func TestGetEnvAsInt(t *testing.T) {
	tests := []struct {
		name       string
		envValue   string
		defaultVal int
		expected   int
	}{
		{
			name:       "valid integer",
			envValue:   "42",
			defaultVal: 0,
			expected:   42,
		},
		{
			name:       "empty string uses default",
			envValue:   "",
			defaultVal: 99,
			expected:   99,
		},
		{
			name:       "invalid integer uses default",
			envValue:   "not-a-number",
			defaultVal: 10,
			expected:   10,
		},
		{
			name:       "negative integer",
			envValue:   "-5",
			defaultVal: 0,
			expected:   -5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_INT_VAR"
			if tt.envValue != "" {
				os.Setenv(key, tt.envValue)
				defer os.Unsetenv(key)
			} else {
				os.Unsetenv(key)
			}

			result := getEnvAsInt(key, tt.defaultVal)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}
