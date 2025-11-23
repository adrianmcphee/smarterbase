package smarterbase

import (
	"crypto/tls"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

// RedisOptions returns redis.Options populated from standard environment variables.
//
// Environment variables read (with defaults):
//   - REDIS_ADDR (default: "localhost:6379")
//   - REDIS_PASSWORD (default: "")
//   - REDIS_DB (default: 0)
//   - REDIS_TLS_ENABLED (default: false, auto-enabled for port 25061)
//
// This is a convenience function for production deployments following 12-factor app
// principles. It provides sensible defaults for local development while allowing
// production configuration via environment variables.
//
// Users can still construct redis.Options manually for advanced scenarios (Redis
// Cluster, Sentinel, custom TLS, connection pools, etc.).
//
// Example usage:
//
//	// Simple case - works locally and in production
//	redisClient := redis.NewClient(smarterbase.RedisOptions())
//	defer redisClient.Close()
//
//	// Production deployment:
//	// export REDIS_ADDR=redis.prod.example.com:6379
//	// export REDIS_PASSWORD=secret
//	// export REDIS_DB=0
//	// export REDIS_TLS_ENABLED=true
//
// For more complex setups, use redis.Options directly:
//
//	redisClient := redis.NewClient(&redis.Options{
//	    Addr:     "redis.example.com:6379",
//	    Password: "secret",
//	    TLSConfig: &tls.Config{...},
//	    PoolSize: 100,
//	})
func RedisOptions() *redis.Options {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	password := os.Getenv("REDIS_PASSWORD")

	db := getEnvAsInt("REDIS_DB", 0)

	opts := &redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	}

	// Enable TLS if explicitly requested or for managed Redis ports
	tlsEnabled := os.Getenv("REDIS_TLS_ENABLED") == "true" || strings.HasSuffix(addr, ":25061")
	if tlsEnabled {
		host := extractHostname(addr)
		opts.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: host,
		}
	}

	return opts
}

// RedisOptionsWithOverrides returns redis.Options with explicit overrides for common parameters.
//
// This helper is designed for applications that have explicit configuration but want
// environment variable fallback. Pass empty strings to use environment variables.
//
// Parameters:
//   - addr: Redis server address (empty = use REDIS_ADDR env var or "localhost:6379")
//   - password: Redis password (empty = use REDIS_PASSWORD env var)
//   - poolSize: Connection pool size (0 = use Redis default of 10)
//   - minIdleConns: Minimum idle connections (0 = use Redis default of 0)
//
// Example - Application config with environment fallback:
//
//	opts := smarterbase.RedisOptionsWithOverrides(
//	    cfg.RedisHost,     // Use config if present, else env var
//	    cfg.RedisPassword, // Use config if present, else env var
//	    10,                // App-specific pool size
//	    5,                 // App-specific min idle
//	)
//	redisClient := redis.NewClient(opts)
//
// Example - Pure environment config:
//
//	opts := smarterbase.RedisOptionsWithOverrides("", "", 10, 5)
//	// Reads REDIS_ADDR and REDIS_PASSWORD from environment
//	redisClient := redis.NewClient(opts)
func RedisOptionsWithOverrides(addr, password string, poolSize, minIdleConns int) *redis.Options {
	// Start with environment-based config
	opts := RedisOptions()

	// Override with explicit values if provided
	if addr != "" {
		opts.Addr = addr
		// If TLS is enabled, we must update ServerName to match the new address
		if opts.TLSConfig != nil {
			opts.TLSConfig.ServerName = extractHostname(addr)
		}
		// Enable TLS for managed Redis ports if not already enabled
		if opts.TLSConfig == nil && strings.HasSuffix(addr, ":25061") {
			host := extractHostname(addr)
			opts.TLSConfig = &tls.Config{
				MinVersion: tls.VersionTLS12,
				ServerName: host,
			}
		}
	}
	if password != "" {
		opts.Password = password
	}
	if poolSize > 0 {
		opts.PoolSize = poolSize
	}
	if minIdleConns > 0 {
		opts.MinIdleConns = minIdleConns
	}

	// Ensure ServerName is set if TLS is enabled
	if opts.TLSConfig != nil && opts.TLSConfig.ServerName == "" {
		opts.TLSConfig.ServerName = extractHostname(opts.Addr)
	}

	return opts
}

// getEnvAsInt reads an integer environment variable with a default fallback.
func getEnvAsInt(key string, defaultVal int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultVal
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultVal
	}

	return value
}

// extractHostname returns the hostname from a host:port string.
// If parsing fails (e.g. no port), returns the original string.
func extractHostname(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
