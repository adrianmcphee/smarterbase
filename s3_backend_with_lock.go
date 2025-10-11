package smarterbase

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/redis/go-redis/v9"
)

// S3BackendWithRedisLock wraps S3Backend with distributed locking to eliminate
// the race condition in PutIfMatch operations.
//
// Race condition eliminated:
//
//	T1: Thread A acquires lock for key
//	T2: Thread A: HeadObject (get ETag)
//	T3: Thread A: PutObject (write)
//	T4: Thread A releases lock
//	✓ No other thread can modify the object while A holds the lock
//
// Use this for:
//   - Critical data requiring strong consistency (financial, counters)
//   - High-concurrency scenarios
//   - Multi-instance deployments
//
// Note: Requires Redis for distributed locking
type S3BackendWithRedisLock struct {
	*S3Backend
	lock           *DistributedLock
	defaultLockTTL time.Duration
	maxRetries     int
}

// NewS3BackendWithRedisLock creates an S3 backend with distributed locking
func NewS3BackendWithRedisLock(client *s3.Client, bucket string, redisClient *redis.Client) *S3BackendWithRedisLock {
	return &S3BackendWithRedisLock{
		S3Backend:      NewS3Backend(client, bucket).(*S3Backend),
		lock:           NewDistributedLock(redisClient, "smarterbase"),
		defaultLockTTL: 10 * time.Second,
		maxRetries:     3,
	}
}

// NewS3BackendWithRedisLockCustom creates an S3 backend with custom lock settings
func NewS3BackendWithRedisLockCustom(
	client *s3.Client,
	bucket string,
	redisClient *redis.Client,
	lockTTL time.Duration,
	maxRetries int,
) *S3BackendWithRedisLock {
	return &S3BackendWithRedisLock{
		S3Backend:      NewS3Backend(client, bucket).(*S3Backend),
		lock:           NewDistributedLock(redisClient, "smarterbase"),
		defaultLockTTL: lockTTL,
		maxRetries:     maxRetries,
	}
}

// PutIfMatch overrides the base implementation with distributed locking
// This eliminates the race condition present in the base S3Backend implementation.
func (b *S3BackendWithRedisLock) PutIfMatch(ctx context.Context, key string, data []byte, expectedETag string) (string, error) {
	// Acquire distributed lock BEFORE reading current ETag
	// This ensures no other process can modify the object while we're checking
	release, err := b.lock.TryLockWithRetry(ctx, key, b.defaultLockTTL, b.maxRetries)
	if err != nil {
		return "", WithContext(ErrLockTimeout, map[string]interface{}{
			"key":     key,
			"retries": b.maxRetries,
			"error":   err.Error(),
		})
	}
	defer release()

	// Now delegate to base implementation
	// The lock guarantees no concurrent modifications during HeadObject → PutObject
	return b.S3Backend.PutIfMatch(ctx, key, data, expectedETag)
}

// Append overrides the base implementation with distributed locking
// This ensures atomic append operations across multiple processes.
func (b *S3BackendWithRedisLock) Append(ctx context.Context, key string, data []byte) error {
	// Acquire distributed lock for atomic read-modify-write
	release, err := b.lock.TryLockWithRetry(ctx, key, b.defaultLockTTL, b.maxRetries)
	if err != nil {
		return WithContext(ErrLockTimeout, map[string]interface{}{
			"key":     key,
			"retries": b.maxRetries,
			"error":   err.Error(),
		})
	}
	defer release()

	// Delegate to base implementation (now protected by distributed lock)
	return b.S3Backend.Append(ctx, key, data)
}

// Close releases resources held by the backend
func (b *S3BackendWithRedisLock) Close() error {
	// S3 client doesn't need explicit closing
	// DistributedLock owns its Redis client only if created with NewDistributedLockWithOwnedClient
	return b.lock.Close()
}

// Example usage:
//
//	// Initialize with Redis
//	redisClient := redis.NewClient(&redis.Options{
//	    Addr: "localhost:6379",
//	})
//
//	// Create S3 backend with distributed locking
//	backend := NewS3BackendWithRedisLock(s3Client, "my-bucket", redisClient)
//	store := NewStore(backend)
//
//	// Now PutIfMatch is safe for concurrent use across multiple processes
//	etag, err := backend.PutIfMatch(ctx, key, data, expectedETag)
