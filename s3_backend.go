package smarterbase

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Backend implements Backend using AWS S3 (or S3-compatible storage)
type S3Backend struct {
	client *s3.Client
	bucket string
}

// NewS3Backend creates a new S3 backend
func NewS3Backend(client *s3.Client, bucket string) Backend {
	return &S3Backend{
		client: client,
		bucket: bucket,
	}
}

// Get retrieves data for the given key from S3
func (b *S3Backend) Get(ctx context.Context, key string) ([]byte, error) {
	result, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if strings.Contains(err.Error(), "NoSuchKey") {
			return nil, ErrNotFound
		}
		if strings.Contains(err.Error(), "AccessDenied") {
			return nil, ErrUnauthorized
		}
		return nil, err
	}
	defer func() { _ = result.Body.Close() }() //nolint:errcheck // Deferred close

	return io.ReadAll(result.Body)
}

// Put stores data for the given key to S3
func (b *S3Backend) Put(ctx context.Context, key string, data []byte) error {
	_, err := b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	return err
}

// Delete removes the object at the given key from S3
func (b *S3Backend) Delete(ctx context.Context, key string) error {
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	return err
}

// Exists checks if an object exists at the given key in S3
func (b *S3Backend) Exists(ctx context.Context, key string) (bool, error) {
	_, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if strings.Contains(err.Error(), "NotFound") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetWithETag retrieves data and its ETag for optimistic locking from S3
func (b *S3Backend) GetWithETag(ctx context.Context, key string) ([]byte, string, error) {
	result, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = result.Body.Close() }() //nolint:errcheck // Deferred close

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, "", err
	}

	etag := strings.Trim(aws.ToString(result.ETag), "\"")
	return data, etag, nil
}

// PutIfMatch provides best-effort optimistic locking for S3.
//
// ⚠️ CRITICAL RACE CONDITION WARNING ⚠️
//
// This implementation has an unavoidable race window between HeadObject and PutObject
// that can lead to lost updates in concurrent scenarios.
//
// Race condition timeline:
//
//	T1: Thread A calls HeadObject, gets ETag "abc"  ✓
//	T2: Thread B calls PutObject, writes new data (ETag becomes "xyz")
//	T3: Thread A calls PutObject with expectedETag="abc"  ✓ SUCCEEDS (should fail!)
//	Result: Thread B's update is lost!
//
// Root cause: S3 PutObject doesn't support If-Match headers (only GetObject does)
//
// ❌ DO NOT USE for:
// - Financial data (balances, transactions, payments)
// - Counters or sequences that must be accurate
// - Any data where lost updates are unacceptable
// - High-concurrency scenarios (>1 update/sec per key)
//
// ✅ Safe to use for:
// - Low-traffic scenarios (<1 update/min per key)
// - Data where occasional inconsistency is acceptable
// - Non-critical metadata or cache invalidation
//
// ✅ Better alternatives:
// - DynamoDB with conditional writes (true atomic compare-and-swap)
// - Redis with WATCH/MULTI/EXEC (atomic transactions)
// - Application-level distributed locks (Redis, etcd, Consul)
// - Event sourcing with append-only logs (no overwrites)
//
// Example of proper usage (DynamoDB):
//
//	UpdateItemInput{
//	    ConditionExpression: "version = :expectedVersion",
//	    UpdateExpression: "SET #data = :data, version = version + 1",
//	}
//
// If you must use S3 for this, consider implementing application-level locking
// with Redis or adding a coordination layer with DynamoDB.
func (b *S3Backend) PutIfMatch(ctx context.Context, key string, data []byte, expectedETag string) (string, error) {
	// AWS SDK v2 doesn't support IfMatch on PutObject
	// Best-effort approach: check ETag before writing (small race window)
	if expectedETag != "" {
		headResult, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(b.bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			return "", err
		}

		currentETag := strings.Trim(aws.ToString(headResult.ETag), "\"")
		if currentETag != expectedETag {
			return "", WithContext(ErrConflict, map[string]interface{}{
				"expected": expectedETag,
				"actual":   currentETag,
			})
		}
	}

	// Perform the put
	putResult, err := b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return "", err
	}

	newETag := strings.Trim(aws.ToString(putResult.ETag), "\"")
	return newETag, nil
}

// List returns all keys with the given prefix from S3
func (b *S3Backend) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(b.bucket),
		Prefix: aws.String(prefix),
	}

	paginator := s3.NewListObjectsV2Paginator(b.client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, obj := range output.Contents {
			keys = append(keys, *obj.Key)
		}
	}

	return keys, nil
}

// ListPaginated streams keys with the given prefix in batches from S3
func (b *S3Backend) ListPaginated(ctx context.Context, prefix string, handler func(keys []string) error) error {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(b.bucket),
		Prefix: aws.String(prefix),
	}

	paginator := s3.NewListObjectsV2Paginator(b.client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}

		var keys []string
		for _, obj := range output.Contents {
			keys = append(keys, *obj.Key)
		}

		if err := handler(keys); err != nil {
			return err
		}
	}

	return nil
}

// GetStream returns a reader for streaming large objects from S3
func (b *S3Backend) GetStream(ctx context.Context, key string) (io.ReadCloser, error) {
	result, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}

	return result.Body, nil
}

// PutStream writes large objects from a stream to S3
func (b *S3Backend) PutStream(ctx context.Context, key string, reader io.Reader, size int64) error {
	_, err := b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
		Body:   reader,
	})
	return err
}

// Append appends data to an existing S3 object using read-modify-write.
//
// ⚠️ WARNING: This is NOT atomic. There's a race window between Get and Put.
// For high-concurrency append scenarios, consider:
// - Using DynamoDB for coordination
// - S3 Transfer Acceleration with versioning
// - Application-level locking (Redis)
//
// For append-only logs (JSONL), race conditions are acceptable if:
// - Events have unique IDs (deduplication downstream)
// - Lost appends can be replayed from source (Redis Streams)
func (b *S3Backend) Append(ctx context.Context, key string, data []byte) error {
	// Read existing content (if exists)
	existing, err := b.Get(ctx, key)
	if err != nil && !IsNotFound(err) {
		return fmt.Errorf("failed to read existing object: %w", err)
	}

	// Append new data
	combined := append(existing, data...)

	// Write back
	return b.Put(ctx, key, combined)
}

// Ping checks if the S3 backend is accessible and operational
func (b *S3Backend) Ping(ctx context.Context) error {
	_, err := b.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(b.bucket),
	})
	return err
}

// Close releases any resources held by the S3 backend
func (b *S3Backend) Close() error {
	// S3 client doesn't need explicit closing
	return nil
}
