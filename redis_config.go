package smarterbase

import (
	"os"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// RedisOptions returns redis.Options populated from standard environment variables.
//
// Environment variables read (with defaults):
//   - REDIS_ADDR (default: "localhost:6379")
//   - REDIS_PASSWORD (default: "")
//   - REDIS_DB (default: 0)
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

	return &redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	}
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
