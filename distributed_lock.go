package smarterbase

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// DistributedLock provides Redis-based distributed locking for coordinating
// operations across multiple processes/servers.
//
// Use cases:
// - Filesystem backend with multiple application instances
// - Coordinating S3 PutIfMatch operations
// - Preventing concurrent modifications to the same resource
type DistributedLock struct {
	redis      *redis.Client
	keyPrefix  string
	defaultTTL time.Duration
	ownsClient bool // If true, Close() will close the Redis client
}

// NewDistributedLock creates a new distributed lock manager using Redis
func NewDistributedLock(redis *redis.Client, keyPrefix string) *DistributedLock {
	return &DistributedLock{
		redis:      redis,
		keyPrefix:  keyPrefix,
		defaultTTL: 30 * time.Second,
		ownsClient: false,
	}
}

// NewDistributedLockWithOwnedClient creates a lock manager that owns the Redis client
func NewDistributedLockWithOwnedClient(redis *redis.Client, keyPrefix string) *DistributedLock {
	return &DistributedLock{
		redis:      redis,
		keyPrefix:  keyPrefix,
		defaultTTL: 30 * time.Second,
		ownsClient: true,
	}
}

// Lock acquires a distributed lock for the given key.
// Returns a release function that MUST be called to release the lock.
//
// Example:
//
//	release, err := lock.Lock(ctx, "users/123", 5*time.Second)
//	if err != nil {
//	    return err
//	}
//	defer release()
//
//	// Critical section - only one process can execute this at a time
//	user := getUser()
//	user.Balance += 100
//	saveUser(user)
func (l *DistributedLock) Lock(ctx context.Context, key string, ttl time.Duration) (func(), error) {
	if ttl == 0 {
		ttl = l.defaultTTL
	}

	lockKey := fmt.Sprintf("%s:lock:%s", l.keyPrefix, key)
	lockValue := fmt.Sprintf("%d", time.Now().UnixNano())

	// Try to acquire lock with SET NX (only set if not exists)
	success, err := l.redis.SetNX(ctx, lockKey, lockValue, ttl).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}

	if !success {
		return nil, WithContext(ErrLockHeld, map[string]interface{}{
			"key": key,
			"ttl": ttl,
		})
	}

	// Return a release function
	release := func() {
		// Use a background context for cleanup (don't fail if parent context cancelled)
		cleanupCtx := context.Background()

		// Only delete if we still own the lock (check value matches)
		script := `
			if redis.call("get", KEYS[1]) == ARGV[1] then
				return redis.call("del", KEYS[1])
			else
				return 0
			end
		`
		l.redis.Eval(cleanupCtx, script, []string{lockKey}, lockValue).Result()
	}

	return release, nil
}

// TryLockWithRetry attempts to acquire a lock with exponential backoff retry.
// Useful for handling temporary contention.
func (l *DistributedLock) TryLockWithRetry(ctx context.Context, key string, ttl time.Duration, maxRetries int) (func(), error) {
	config := DefaultRetryConfig()
	config.MaxRetries = maxRetries

	var lastErr error
	for i := 0; i < config.MaxRetries; i++ {
		release, err := l.Lock(ctx, key, ttl)
		if err == nil {
			return release, nil
		}

		lastErr = err

		// Check if context cancelled
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Wait with exponential backoff
		if i < config.MaxRetries-1 {
			backoff := config.InitialBackoff * time.Duration(int64(1)<<uint(i))
			jitter := time.Duration(float64(backoff) * config.JitterPercent)
			time.Sleep(backoff + jitter)
		}
	}

	return nil, fmt.Errorf("failed to acquire lock after %d retries: %w", config.MaxRetries, lastErr)
}

// FilesystemBackendWithRedisLock wraps FilesystemBackend with Redis-based distributed locking
// for multi-instance deployments.
type FilesystemBackendWithRedisLock struct {
	*FilesystemBackend
	lock *DistributedLock
}

// NewFilesystemBackendWithRedisLock creates a filesystem backend with distributed locking
func NewFilesystemBackendWithRedisLock(basePath string, redisClient *redis.Client) *FilesystemBackendWithRedisLock {
	return &FilesystemBackendWithRedisLock{
		FilesystemBackend: NewFilesystemBackend(basePath),
		lock:              NewDistributedLock(redisClient, "smarterbase"),
	}
}

// PutIfMatch overrides the base implementation with distributed locking
func (b *FilesystemBackendWithRedisLock) PutIfMatch(ctx context.Context, key string, data []byte, expectedETag string) (string, error) {
	// Acquire distributed lock
	release, err := b.lock.TryLockWithRetry(ctx, key, 5*time.Second, 3)
	if err != nil {
		return "", fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer release()

	// Delegate to base implementation (now protected by distributed lock)
	return b.FilesystemBackend.PutIfMatch(ctx, key, data, expectedETag)
}

// Append overrides the base implementation with distributed locking
func (b *FilesystemBackendWithRedisLock) Append(ctx context.Context, key string, data []byte) error {
	// Acquire distributed lock
	release, err := b.lock.TryLockWithRetry(ctx, key, 5*time.Second, 3)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer release()

	// Delegate to base implementation (now protected by distributed lock)
	return b.FilesystemBackend.Append(ctx, key, data)
}

// Close releases resources held by the distributed lock
func (dl *DistributedLock) Close() error {
	if dl.ownsClient && dl.redis != nil {
		return dl.redis.Close()
	}
	return nil
}
