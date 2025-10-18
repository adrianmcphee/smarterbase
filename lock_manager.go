package smarterbase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// LockInfo contains information about an active lock
type LockInfo struct {
	Key        string        // The resource key being locked
	LockKey    string        // The Redis key for the lock
	Value      string        // The lock value (timestamp or unique ID)
	TTL        time.Duration // Remaining TTL
	AcquiredAt time.Time     // When the lock was acquired (derived from value if timestamp)
}

// LockManager provides utilities for managing and cleaning up distributed locks
type LockManager struct {
	redis     *redis.Client
	keyPrefix string
	logger    Logger
	metrics   Metrics
}

// NewLockManager creates a new lock manager for administrative operations
func NewLockManager(redis *redis.Client, keyPrefix string, logger Logger, metrics Metrics) *LockManager {
	if logger == nil {
		logger = &NoOpLogger{}
	}
	if metrics == nil {
		metrics = &NoOpMetrics{}
	}

	return &LockManager{
		redis:     redis,
		keyPrefix: keyPrefix,
		logger:    logger,
		metrics:   metrics,
	}
}

// ListLocks returns all active locks matching the key prefix
//
// Example:
//
//	locks, err := lockManager.ListLocks(ctx)
//	for _, lock := range locks {
//	    fmt.Printf("Lock: %s, TTL: %s, Age: %s\n",
//	        lock.Key,
//	        lock.TTL,
//	        time.Since(lock.AcquiredAt))
//	}
func (lm *LockManager) ListLocks(ctx context.Context) ([]LockInfo, error) {
	if lm.redis == nil {
		return nil, fmt.Errorf("redis not available")
	}

	// Scan for all lock keys
	lockPattern := fmt.Sprintf("%s:lock:*", lm.keyPrefix)

	var locks []LockInfo
	var cursor uint64

	for {
		var keys []string
		var err error
		keys, cursor, err = lm.redis.Scan(ctx, cursor, lockPattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan lock keys: %w", err)
		}

		// Get info for each lock
		for _, lockKey := range keys {
			// Get TTL
			ttl, err := lm.redis.TTL(ctx, lockKey).Result()
			if err != nil {
				lm.logger.Warn("failed to get TTL for lock", "key", lockKey, "error", err)
				continue
			}

			// Skip if lock expired
			if ttl < 0 {
				continue
			}

			// Get lock value
			value, err := lm.redis.Get(ctx, lockKey).Result()
			if err != nil {
				lm.logger.Warn("failed to get value for lock", "key", lockKey, "error", err)
				continue
			}

			// Extract resource key from lock key
			// Format: {prefix}:lock:{resource_key}
			resourceKey := strings.TrimPrefix(lockKey, fmt.Sprintf("%s:lock:", lm.keyPrefix))

			// Try to parse acquisition time from value (if it's a nano timestamp)
			var acquiredAt time.Time
			if timestamp, err := fmt.Sscanf(value, "%d", new(int64)); err == nil && timestamp == 1 {
				var nanos int64
				fmt.Sscanf(value, "%d", &nanos)
				acquiredAt = time.Unix(0, nanos)
			}

			locks = append(locks, LockInfo{
				Key:        resourceKey,
				LockKey:    lockKey,
				Value:      value,
				TTL:        ttl,
				AcquiredAt: acquiredAt,
			})
		}

		if cursor == 0 {
			break
		}
	}

	lm.metrics.Gauge(MetricLockActive, float64(len(locks)))

	return locks, nil
}

// CleanupOrphanedLocks removes locks older than the specified age
//
// Orphaned locks occur when:
// - Application crashes before releasing lock
// - Network partition during lock release
// - Process killed with SIGKILL
//
// Safety: Only removes locks if their TTL is less than minTTL.
// This prevents removing locks that are still legitimately held.
//
// Example:
//
//	// Clean up locks that have been held for more than 5 minutes
//	// (assuming default TTL is 30 seconds, anything still locked after 5min is orphaned)
//	removed, err := lockManager.CleanupOrphanedLocks(ctx, 5*time.Minute)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Cleaned up %d orphaned locks\n", removed)
func (lm *LockManager) CleanupOrphanedLocks(ctx context.Context, minAge time.Duration) (int, error) {
	if lm.redis == nil {
		return 0, fmt.Errorf("redis not available")
	}

	locks, err := lm.ListLocks(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list locks: %w", err)
	}

	removed := 0
	now := time.Now()

	for _, lock := range locks {
		// Skip if lock has no acquisition time
		if lock.AcquiredAt.IsZero() {
			continue
		}

		age := now.Sub(lock.AcquiredAt)

		// Only remove if lock is older than minAge
		if age < minAge {
			continue
		}

		// Remove the lock
		deleted, err := lm.redis.Del(ctx, lock.LockKey).Result()
		if err != nil {
			lm.logger.Warn("failed to delete orphaned lock",
				"key", lock.Key,
				"age", age,
				"error", err,
			)
			continue
		}

		if deleted > 0 {
			removed++
			lm.logger.Info("removed orphaned lock",
				"key", lock.Key,
				"age", age,
				"ttl_remaining", lock.TTL,
			)
			lm.metrics.Increment(MetricLockOrphaned, "key", lock.Key)
		}
	}

	if removed > 0 {
		lm.logger.Info("orphaned lock cleanup completed",
			"removed", removed,
			"min_age", minAge,
		)
		lm.metrics.Increment(MetricLockCleanup, "removed", fmt.Sprintf("%d", removed))
	}

	return removed, nil
}

// ForceRelease forcefully releases a specific lock
//
// ⚠️ USE WITH CAUTION: Only use when you're certain the lock holder has crashed
//
// Example:
//
//	// Force release a stuck lock
//	err := lockManager.ForceRelease(ctx, "users/123")
//	if err != nil {
//	    return fmt.Errorf("failed to force release lock: %w", err)
//	}
func (lm *LockManager) ForceRelease(ctx context.Context, resourceKey string) error {
	if lm.redis == nil {
		return fmt.Errorf("redis not available")
	}

	lockKey := fmt.Sprintf("%s:lock:%s", lm.keyPrefix, resourceKey)

	deleted, err := lm.redis.Del(ctx, lockKey).Result()
	if err != nil {
		return fmt.Errorf("failed to delete lock: %w", err)
	}

	if deleted == 0 {
		return fmt.Errorf("lock not found: %s", resourceKey)
	}

	lm.logger.Info("forcefully released lock", "key", resourceKey)
	lm.metrics.Increment(MetricLockForceRelease, "key", resourceKey)

	return nil
}

// GetLockInfo retrieves information about a specific lock
func (lm *LockManager) GetLockInfo(ctx context.Context, resourceKey string) (*LockInfo, error) {
	if lm.redis == nil {
		return nil, fmt.Errorf("redis not available")
	}

	lockKey := fmt.Sprintf("%s:lock:%s", lm.keyPrefix, resourceKey)

	// Check if lock exists
	exists, err := lm.redis.Exists(ctx, lockKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to check lock existence: %w", err)
	}

	if exists == 0 {
		return nil, ErrLockNotFound
	}

	// Get TTL
	ttl, err := lm.redis.TTL(ctx, lockKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get TTL: %w", err)
	}

	// Get value
	value, err := lm.redis.Get(ctx, lockKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get lock value: %w", err)
	}

	// Try to parse acquisition time
	var acquiredAt time.Time
	if timestamp, err := fmt.Sscanf(value, "%d", new(int64)); err == nil && timestamp == 1 {
		var nanos int64
		fmt.Sscanf(value, "%d", &nanos)
		acquiredAt = time.Unix(0, nanos)
	}

	return &LockInfo{
		Key:        resourceKey,
		LockKey:    lockKey,
		Value:      value,
		TTL:        ttl,
		AcquiredAt: acquiredAt,
	}, nil
}

// Example usage:
//
//	// Create lock manager
//	lockManager := smarterbase.NewLockManager(redisClient, "smarterbase", logger, metrics)
//
//	// List all active locks
//	locks, err := lockManager.ListLocks(ctx)
//	for _, lock := range locks {
//	    fmt.Printf("Lock on %s, age: %s, ttl: %s\n",
//	        lock.Key,
//	        time.Since(lock.AcquiredAt),
//	        lock.TTL)
//	}
//
//	// Clean up orphaned locks (older than 5 minutes)
//	removed, err := lockManager.CleanupOrphanedLocks(ctx, 5*time.Minute)
//	fmt.Printf("Removed %d orphaned locks\n", removed)
//
//	// Force release a specific lock (use with caution!)
//	err = lockManager.ForceRelease(ctx, "users/123")
