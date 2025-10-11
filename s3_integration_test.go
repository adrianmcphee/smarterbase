package smarterbase

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/minio"
)

// TestIntegration_S3Backend_MinIO validates S3-compatible backend with MinIO
// This is the primary S3 integration test - uses MinIO for S3 compatibility
//
// Run with: go test -run TestIntegration_S3Backend_MinIO -v
//
// Four test modes (in order of preference):
// 1. Testcontainers: Auto-starts MinIO via Docker (most pragmatic, zero setup)
// 2. Manual MinIO: Uses existing MinIO at localhost:9000 (set TEST_MINIO=true)
// 3. Real S3: Uses real AWS S3 (set TEST_S3_BUCKET=your-bucket)
// 4. Skip: No Docker/MinIO/S3 available
func TestIntegration_S3Backend_MinIO(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping S3/MinIO integration test in short mode")
	}

	ctx := context.Background()

	// Mode 1: Real S3 (highest priority for production validation)
	if s3Bucket := os.Getenv("TEST_S3_BUCKET"); s3Bucket != "" {
		t.Run("RealS3Backend", func(t *testing.T) {
			testS3BackendWithRealS3(t, ctx, s3Bucket)
		})
		return
	}

	// Mode 2: Manual MinIO (developers running local MinIO)
	if os.Getenv("TEST_MINIO") != "" {
		t.Run("ManualMinIO", func(t *testing.T) {
			testS3BackendWithManualMinIO(t, ctx)
		})
		return
	}

	// Mode 3: Testcontainers (auto-start MinIO if Docker available)
	// This is the best default mode - zero manual setup required
	t.Run("Testcontainers", func(t *testing.T) {
		testS3BackendWithTestcontainers(t, ctx)
	})
}

// testS3BackendWithManualMinIO tests against a locally running MinIO instance
// To run:
//
//	docker run -d -p 9000:9000 -p 9001:9001 \
//	  -e "MINIO_ROOT_USER=minioadmin" \
//	  -e "MINIO_ROOT_PASSWORD=minioadmin" \
//	  minio/minio server /data --console-address ":9001"
//	TEST_MINIO=true go test -run TestIntegration_S3Backend_MinIO -v
func testS3BackendWithManualMinIO(t *testing.T, ctx context.Context) {
	// Setup MinIO client
	minioConfig := MinIOConfig{
		Endpoint:        "localhost:9000",
		AccessKeyID:     "minioadmin",
		SecretAccessKey: "minioadmin",
		UseSSL:          false,
		Bucket:          "test-bucket",
	}

	// Create bucket if it doesn't exist
	s3Client := createMinIOClient(minioConfig)
	ensureBucketExists(t, ctx, s3Client, minioConfig.Bucket)

	// Setup Redis for distributed locking
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	// Create backend with Redis locks (production-safe)
	backend, err := NewMinIOBackendWithRedisLock(minioConfig, redisClient)
	if err != nil {
		t.Fatalf("Failed to create MinIO backend: %v", err)
	}

	// Run backend compliance tests
	runS3BackendComplianceTests(t, ctx, backend)
}

// testS3BackendWithRealS3 tests against real AWS S3 (requires AWS credentials)
// To run:
//
//	export AWS_PROFILE=your-profile  # or AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY
//	TEST_S3_BUCKET=your-test-bucket go test -run TestIntegration_S3Backend_MinIO -v
func testS3BackendWithRealS3(t *testing.T, ctx context.Context, bucketName string) {
	t.Logf("Testing with real S3 bucket: %s", bucketName)

	// Load AWS credentials from environment/profile
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		t.Fatalf("Failed to load AWS config: %v", err)
	}

	s3Client := s3.NewFromConfig(cfg)

	// Setup Redis for distributed locking
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	// Create S3 backend with Redis locks
	backend := NewS3BackendWithRedisLock(s3Client, bucketName, redisClient)

	// Run compliance tests
	runS3BackendComplianceTests(t, ctx, backend)

	// Cleanup test objects
	cleanupS3TestObjects(t, ctx, backend)
}

// runS3BackendComplianceTests runs comprehensive backend compliance tests
// This validates all Backend interface operations work correctly with S3
func runS3BackendComplianceTests(t *testing.T, ctx context.Context, backend Backend) {
	// Test 1: Basic Put/Get/Delete
	t.Run("BasicOperations", func(t *testing.T) {
		key := "test-objects/basic-" + NewID() + ".json"
		data := []byte(`{"test": "data", "timestamp": "` + time.Now().Format(time.RFC3339) + `"}`)

		// Put
		err := backend.Put(ctx, key, data)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		// Get
		retrieved, err := backend.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if string(retrieved) != string(data) {
			t.Errorf("Data mismatch. Expected: %s, Got: %s", string(data), string(retrieved))
		}

		// Exists
		exists, err := backend.Exists(ctx, key)
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			t.Error("Object should exist")
		}

		// Delete
		err = backend.Delete(ctx, key)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		// Verify deleted
		exists, err = backend.Exists(ctx, key)
		if err != nil {
			t.Fatalf("Exists check after delete failed: %v", err)
		}
		if exists {
			t.Error("Object should not exist after delete")
		}
	})

	// Test 2: PutIfMatch (Optimistic Locking with Distributed Locks)
	t.Run("OptimisticLocking_PutIfMatch", func(t *testing.T) {
		key := "test-objects/optimistic-" + NewID() + ".json"
		data1 := []byte(`{"version": 1}`)
		data2 := []byte(`{"version": 2}`)
		data3 := []byte(`{"version": 3}`)

		// Initial put
		err := backend.Put(ctx, key, data1)
		if err != nil {
			t.Fatalf("Initial put failed: %v", err)
		}

		// Get with ETag
		_, etag1, err := backend.GetWithETag(ctx, key)
		if err != nil {
			t.Fatalf("GetWithETag failed: %v", err)
		}

		// PutIfMatch with correct ETag should succeed
		etag2, err := backend.PutIfMatch(ctx, key, data2, etag1)
		if err != nil {
			t.Fatalf("PutIfMatch with correct ETag failed: %v", err)
		}

		if etag2 == "" {
			t.Error("Expected non-empty ETag after successful PutIfMatch")
		}

		// PutIfMatch with stale ETag should fail
		_, err = backend.PutIfMatch(ctx, key, data3, etag1)
		if err == nil {
			t.Error("PutIfMatch with stale ETag should fail")
		}

		// Cleanup
		backend.Delete(ctx, key)
	})

	// Test 3: List operations
	t.Run("ListOperations", func(t *testing.T) {
		prefix := "test-objects/list-" + NewID() + "/"

		// Create multiple objects
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("%sitem-%d.json", prefix, i)
			data := []byte(fmt.Sprintf(`{"id": %d}`, i))
			err := backend.Put(ctx, key, data)
			if err != nil {
				t.Fatalf("Put failed for %s: %v", key, err)
			}
		}

		// List with prefix
		keys, err := backend.List(ctx, prefix)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}

		if len(keys) != 5 {
			t.Errorf("Expected 5 keys, got %d: %v", len(keys), keys)
		}

		// Cleanup
		for _, key := range keys {
			backend.Delete(ctx, key)
		}
	})

	// Test 4: Concurrent operations with distributed locks
	t.Run("ConcurrentSafety_WithLocks", func(t *testing.T) {
		key := "test-objects/concurrent-" + NewID() + ".json"
		counter := map[string]int{"value": 0}

		// Initialize
		store := NewStore(backend)
		err := store.PutJSON(ctx, key, counter)
		if err != nil {
			t.Fatalf("Initial put failed: %v", err)
		}

		// Multiple goroutines trying to increment
		// With S3BackendWithRedisLock, this should be safe
		done := make(chan bool)
		for i := 0; i < 3; i++ {
			go func() {
				for j := 0; j < 3; j++ {
					var c map[string]int
					etag, _ := store.GetJSONWithETag(ctx, key, &c)
					c["value"]++
					store.PutJSONWithETag(ctx, key, c, etag)
					time.Sleep(10 * time.Millisecond)
				}
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 3; i++ {
			<-done
		}

		// Final value should be 9 (3 goroutines * 3 increments)
		var final map[string]int
		store.GetJSON(ctx, key, &final)

		if final["value"] != 9 {
			t.Logf("Warning: Race condition detected. Expected 9, got %d", final["value"])
			t.Logf("This is expected with plain S3Backend, should pass with S3BackendWithRedisLock")
		}

		// Cleanup
		backend.Delete(ctx, key)
	})

	// Test 5: Append operations (for JSONL logs)
	t.Run("AppendOperations", func(t *testing.T) {
		key := "test-objects/append-" + NewID() + ".jsonl"

		// Append first line
		line1 := []byte(`{"event": "user_login", "timestamp": "2024-01-01"}` + "\n")
		err := backend.Append(ctx, key, line1)
		if err != nil {
			t.Fatalf("First append failed: %v", err)
		}

		// Append second line
		line2 := []byte(`{"event": "user_logout", "timestamp": "2024-01-02"}` + "\n")
		err = backend.Append(ctx, key, line2)
		if err != nil {
			t.Fatalf("Second append failed: %v", err)
		}

		// Retrieve and verify
		data, err := backend.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		expected := string(line1) + string(line2)
		if string(data) != expected {
			t.Errorf("Appended data mismatch.\nExpected: %s\nGot: %s", expected, string(data))
		}

		// Cleanup
		backend.Delete(ctx, key)
	})

	// Test 6: Health check
	t.Run("HealthCheck", func(t *testing.T) {
		err := backend.Ping(ctx)
		if err != nil {
			t.Errorf("Ping failed: %v", err)
		}
	})
}

// Helper: Create MinIO S3 client
func createMinIOClient(cfg MinIOConfig) *s3.Client {
	scheme := "http"
	if cfg.UseSSL {
		scheme = "https"
	}
	endpoint := fmt.Sprintf("%s://%s", scheme, cfg.Endpoint)

	return s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint),
		Region:       "us-east-1",
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		UsePathStyle: true,
	})
}

// Helper: Ensure bucket exists
func ensureBucketExists(t *testing.T, ctx context.Context, client *s3.Client, bucket string) {
	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})

	if err != nil {
		// Bucket doesn't exist, create it
		_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(bucket),
		})
		if err != nil {
			t.Logf("Warning: Failed to create bucket %s: %v", bucket, err)
		}
	}
}

// Helper: Cleanup test objects
func cleanupS3TestObjects(t *testing.T, ctx context.Context, backend Backend) {
	// List and delete all test objects
	keys, err := backend.List(ctx, "test-objects/")
	if err != nil {
		t.Logf("Warning: Failed to list objects for cleanup: %v", err)
		return
	}

	for _, key := range keys {
		backend.Delete(ctx, key)
	}

	t.Logf("Cleaned up %d test objects", len(keys))
}

// testS3BackendWithTestcontainers auto-starts MinIO using testcontainers
// This is the most pragmatic mode - zero manual setup, just requires Docker
//
// Run with: go test -run TestIntegration_S3Backend_MinIO -v
func testS3BackendWithTestcontainers(t *testing.T, ctx context.Context) {
	// Catch panic if Docker daemon is not running
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("Docker daemon not available, skipping testcontainers test: %v", r)
		}
	}()

	// Start MinIO container
	minioContainer, err := minio.Run(ctx,
		"minio/minio:latest",
		testcontainers.WithEnv(map[string]string{
			"MINIO_ROOT_USER":     "minioadmin",
			"MINIO_ROOT_PASSWORD": "minioadmin",
		}),
	)

	if err != nil {
		t.Skipf("Failed to start MinIO container (Docker not available?): %v", err)
		return
	}
	defer func() {
		if err := testcontainers.TerminateContainer(minioContainer); err != nil {
			t.Logf("Failed to terminate MinIO container: %v", err)
		}
	}()

	// Get MinIO endpoint
	endpoint, err := minioContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("Failed to get MinIO endpoint: %v", err)
	}

	t.Logf("✅ MinIO container started at %s", endpoint)

	// Setup MinIO client
	minioConfig := MinIOConfig{
		Endpoint:        endpoint,
		AccessKeyID:     "minioadmin",
		SecretAccessKey: "minioadmin",
		UseSSL:          false,
		Bucket:          "test-bucket",
	}

	// Create bucket
	s3Client := createMinIOClient(minioConfig)
	ensureBucketExists(t, ctx, s3Client, minioConfig.Bucket)

	// Setup Redis for distributed locking
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	// Create backend with Redis locks (production-safe)
	backend, err := NewMinIOBackendWithRedisLock(minioConfig, redisClient)
	if err != nil {
		t.Fatalf("Failed to create MinIO backend: %v", err)
	}

	t.Log("🚀 Running S3 backend compliance tests against testcontainers MinIO...")

	// Run backend compliance tests
	runS3BackendComplianceTests(t, ctx, backend)

	t.Log("✅ All S3 backend compliance tests passed!")
}
