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
	os.Unsetenv("REDIS_TLS_ENABLED")

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

	if opts.TLSConfig != nil {
		t.Error("expected default TLS config nil")
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

func TestRedisOptions_WithTLS(t *testing.T) {
	os.Setenv("REDIS_ADDR", "redis.tls.example.com:6380")
	os.Setenv("REDIS_TLS_ENABLED", "true")
	defer func() {
		os.Unsetenv("REDIS_ADDR")
		os.Unsetenv("REDIS_TLS_ENABLED")
	}()

	opts := RedisOptions()

	if opts.TLSConfig == nil {
		t.Fatal("expected TLS config to be set")
	}

	if opts.TLSConfig.ServerName != "redis.tls.example.com" {
		t.Errorf("expected ServerName redis.tls.example.com, got %s", opts.TLSConfig.ServerName)
	}
}

func TestRedisOptionsWithOverrides_WithTLS(t *testing.T) {
	os.Setenv("REDIS_ADDR", "default.example.com:6379")
	os.Setenv("REDIS_TLS_ENABLED", "true")
	defer func() {
		os.Unsetenv("REDIS_ADDR")
		os.Unsetenv("REDIS_TLS_ENABLED")
	}()

	// Case 1: Override address
	opts := RedisOptionsWithOverrides("custom.example.com:6380", "", 0, 0)

	if opts.TLSConfig == nil {
		t.Fatal("expected TLS config to be set")
	}

	if opts.TLSConfig.ServerName != "custom.example.com" {
		t.Errorf("expected ServerName custom.example.com, got %s", opts.TLSConfig.ServerName)
	}

	// Case 2: No override address (should use env var)
	opts2 := RedisOptionsWithOverrides("", "", 0, 0)

	if opts2.TLSConfig == nil {
		t.Fatal("expected TLS config to be set")
	}

	if opts2.TLSConfig.ServerName != "default.example.com" {
		t.Errorf("expected ServerName default.example.com, got %s", opts2.TLSConfig.ServerName)
	}
}

func TestRedisOptions_TLSDisabled(t *testing.T) {
	os.Setenv("REDIS_ADDR", "redis.example.com:6379")
	os.Setenv("REDIS_TLS_ENABLED", "false")
	defer func() {
		os.Unsetenv("REDIS_ADDR")
		os.Unsetenv("REDIS_TLS_ENABLED")
	}()

	opts := RedisOptions()

	if opts.TLSConfig != nil {
		t.Error("expected TLS config to be nil when disabled")
	}
}

func TestRedisOptions_AutoTLSEnabled(t *testing.T) {
	os.Setenv("REDIS_ADDR", "redis.digitalocean.com:25061")
	os.Unsetenv("REDIS_TLS_ENABLED") // Ensure not explicitly enabled
	defer func() {
		os.Unsetenv("REDIS_ADDR")
	}()

	opts := RedisOptions()

	if opts.TLSConfig == nil {
		t.Fatal("expected TLS config to be set for port 25061")
	}

	if opts.TLSConfig.ServerName != "redis.digitalocean.com" {
		t.Errorf("expected ServerName redis.digitalocean.com, got %s", opts.TLSConfig.ServerName)
	}
}

func TestRedisOptionsWithOverrides_AutoTLSEnabled(t *testing.T) {
	os.Setenv("REDIS_ADDR", "default.example.com:6379")
	os.Unsetenv("REDIS_TLS_ENABLED")
	defer func() {
		os.Unsetenv("REDIS_ADDR")
	}()

	// Override with managed Redis address
	opts := RedisOptionsWithOverrides("managed.digitalocean.com:25061", "", 0, 0)

	if opts.TLSConfig == nil {
		t.Fatal("expected TLS config to be set for port 25061")
	}

	if opts.TLSConfig.ServerName != "managed.digitalocean.com" {
		t.Errorf("expected ServerName managed.digitalocean.com, got %s", opts.TLSConfig.ServerName)
	}
}
