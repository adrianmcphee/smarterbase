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
		// Use a background context for cleanup (don't fail if parent context canceled)
		cleanupCtx := context.Background()

		// Only delete if we still own the lock (check value matches)
		script := `
			if redis.call("get", KEYS[1]) == ARGV[1] then
				return redis.call("del", KEYS[1])
			else
				return 0
			end
		`
		_, _ = l.redis.Eval(cleanupCtx, script, []string{lockKey}, lockValue).Result() //nolint:errcheck // Cleanup operation, safe to ignore
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

		// Check if context canceled
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

// WithAtomicUpdate executes a function with distributed lock protection.
// This ensures that read-modify-write operations are truly atomic across all processes.
//
// ✅ USE THIS for critical updates that require isolation:
// - Financial transactions (account balance updates)
// - Inventory modifications
// - Counter increments
// - Any read-modify-write that must be atomic
//
// Example:
//
//	lock := smarterbase.NewDistributedLock(redisClient, "smarterbase")
//	err := smarterbase.WithAtomicUpdate(ctx, store, lock, "accounts/123", 10*time.Second,
//	    func(ctx context.Context) error {
//	        var account Account
//	        store.GetJSON(ctx, "accounts/123", &account)
//
//	        // Safe: No other process can modify this account during this function
//	        account.Balance += 100
//
//	        store.PutJSON(ctx, "accounts/123", &account)
//	        return nil
//	    })
//
// Performance: Adds 2-5ms latency for lock acquisition (no contention).
// Under contention: +10-50ms per retry (exponential backoff).
// Retries: Automatically retries 3 times with exponential backoff if lock is held.
// Metrics: Tracks lock contention, wait time, and timeouts via store.metrics.
func WithAtomicUpdate(ctx context.Context, store *Store, lock *DistributedLock, key string, ttl time.Duration, fn func(ctx context.Context) error) error {
	if lock == nil {
		return fmt.Errorf("distributed lock is required for atomic updates")
	}
	if store == nil {
		return fmt.Errorf("store is required for atomic updates")
	}
	if ttl == 0 {
		ttl = 10 * time.Second // Sensible default
	}

	// Track lock acquisition time and contention
	lockStart := time.Now()

	// Acquire distributed lock with retry
	release, err := lock.TryLockWithRetry(ctx, key, ttl, 3)

	lockWaitTime := time.Since(lockStart)
	store.metrics.Timing(MetricLockWaitTime, lockWaitTime, "key", key)

	if err != nil {
		store.metrics.Increment(MetricLockFailed, "key", key)
		store.metrics.Increment(MetricLockTimeout, "key", key)
		return fmt.Errorf("failed to acquire lock for atomic update on %s: %w", key, err)
	}

	store.metrics.Increment(MetricLockAcquired, "key", key)

	// Track contention if lock took significant time
	if lockWaitTime > 5*time.Millisecond {
		store.metrics.Increment(MetricLockContention, "key", key)
		store.metrics.Histogram(MetricLockContention, lockWaitTime.Seconds(), "key", key)
	}

	defer release()

	// Execute the function within the lock
	executionStart := time.Now()
	fnErr := fn(ctx)
	store.metrics.Timing(MetricLockDuration, time.Since(executionStart), "key", key)

	return fnErr
}

// Close releases resources held by the distributed lock
func (dl *DistributedLock) Close() error {
	if dl.ownsClient && dl.redis != nil {
		return dl.redis.Close()
	}
	return nil
}
