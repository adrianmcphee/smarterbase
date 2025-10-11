package smarterbase

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestIntegration_ConcurrentWrites validates distributed locking prevents race conditions
func TestIntegration_ConcurrentWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent write test in short mode")
	}

	ctx := context.Background()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	lock := NewDistributedLock(redisClient, "smarterbase")

	key := "counter.json"
	counter := map[string]int{"value": 0}
	store.PutJSON(ctx, key, counter)

	// Simulate 5 concurrent processes incrementing counter
	var wg sync.WaitGroup
	concurrency := 5
	incrementsPerWorker := 20

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerWorker; j++ {
				// Retry loop for lock contention with more aggressive retries
				maxRetries := 20
				succeeded := false
				var lastErr error
				for retry := 0; retry < maxRetries; retry++ {
					// Use WithAtomicUpdate for proper isolation
					err := WithAtomicUpdate(ctx, store, lock, key, 1*time.Second, func(ctx context.Context) error {
						var c map[string]int
						if err := store.GetJSON(ctx, key, &c); err != nil {
							c = map[string]int{"value": 0}
						}
						c["value"]++
						return store.PutJSON(ctx, key, c)
					})
					if err == nil {
						succeeded = true
						break // Success
					}
					lastErr = err
					// Shorter, consistent backoff
					time.Sleep(50 * time.Millisecond)
				}
				if !succeeded {
					t.Errorf("Increment failed after %d retries: %v", maxRetries, lastErr)
				}
			}
		}()
	}

	wg.Wait()

	// Verify final count
	var final map[string]int
	store.GetJSON(ctx, key, &final)

	expected := concurrency * incrementsPerWorker
	if final["value"] != expected {
		t.Errorf("Race condition detected! Expected %d, got %d", expected, final["value"])
	}
}

// TestIntegration_RedisFailover validates graceful degradation when Redis fails
func TestIntegration_RedisFailover(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping failover test in short mode")
	}

	ctx := context.Background()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	redisIndexer := NewRedisIndexer(redisClient)

	redisIndexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:       "users-by-email",
		EntityType: "users",
		ExtractFunc: func(objectKey string, data []byte) ([]IndexEntry, error) {
			return ExtractJSONField("email")(objectKey, data)
		},
	})

	indexManager := NewIndexManager(store).WithRedisIndexer(redisIndexer)

	// Create user with Redis working
	user1 := map[string]string{"id": "1", "email": "user1@test.com"}
	err = indexManager.Create(ctx, "users/1.json", user1)
	if err != nil {
		t.Fatalf("Create with Redis failed: %v", err)
	}

	// Verify index exists
	keys, _ := redisIndexer.Query(ctx, "users", "email", "user1@test.com")
	if len(keys) != 1 {
		t.Error("Index should exist before Redis failure")
	}

	// Simulate Redis failure
	mr.Close()
	redisClient.Close()

	// Create user with Redis down - should still succeed (graceful degradation)
	user2 := map[string]string{"id": "2", "email": "user2@test.com"}
	err = indexManager.Create(ctx, "users/2.json", user2)

	if err != nil {
		t.Errorf("Create should succeed even with Redis down, got: %v", err)
	}

	// Verify data was still saved to filesystem
	var retrieved map[string]string
	err = store.GetJSON(ctx, "users/2.json", &retrieved)
	if err != nil {
		t.Error("Data should be saved even if indexing fails")
	}

	if retrieved["email"] != "user2@test.com" {
		t.Errorf("Expected email user2@test.com, got %s", retrieved["email"])
	}
}

// TestIntegration_CircuitBreakerProtection validates circuit breaker prevents cascading failures
func TestIntegration_CircuitBreakerProtection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping circuit breaker test in short mode")
	}

	ctx := context.Background()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	redisIndexer := NewRedisIndexer(redisClient)

	redisIndexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:       "test-entities",
		EntityType: "entities",
		ExtractFunc: func(objectKey string, data []byte) ([]IndexEntry, error) {
			return ExtractJSONField("category")(objectKey, data)
		},
	})

	// Query should work
	_, err = redisIndexer.Query(ctx, "entities", "category", "test")
	if err != nil {
		t.Errorf("Query should work with Redis up: %v", err)
	}

	// Close Redis to simulate failure
	mr.Close()

	// Trigger 5 failures to open circuit breaker
	for i := 0; i < 5; i++ {
		redisIndexer.Query(ctx, "entities", "category", "test")
	}

	// Circuit breaker should be open now
	if redisIndexer.circuitBreaker.State() != "open" {
		t.Errorf("Circuit breaker should be open after 5 failures, got state: %s", redisIndexer.circuitBreaker.State())
	}

	// Next query should fail fast without hitting Redis
	start := time.Now()
	_, err = redisIndexer.Query(ctx, "entities", "category", "test")
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Query should fail when circuit breaker is open")
	}

	// Should fail fast (< 10ms)
	if elapsed > 10*time.Millisecond {
		t.Errorf("Circuit breaker should fail fast, took %v", elapsed)
	}
}

// TestIntegration_IndexDriftDetectionAndRepair validates health monitoring
func TestIntegration_IndexDriftDetectionAndRepair(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping drift detection test in short mode")
	}

	ctx := context.Background()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	redisIndexer := NewRedisIndexer(redisClient)

	redisIndexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:       "products",
		EntityType: "products",
		ExtractFunc: func(objectKey string, data []byte) ([]IndexEntry, error) {
			return ExtractJSONField("category")(objectKey, data)
		},
	})

	indexManager := NewIndexManager(store).WithRedisIndexer(redisIndexer)

	// Create test data
	for i := 0; i < 10; i++ {
		product := map[string]interface{}{
			"id":       i,
			"category": "electronics",
		}
		key := fmt.Sprintf("products/item-%d.json", i)
		indexManager.Create(ctx, key, product)
	}

	// Setup health monitor
	monitor := NewIndexHealthMonitor(store, redisIndexer).
		WithInterval(100 * time.Millisecond).
		WithSampleSize(20).
		WithDriftThreshold(5.0)

	// Initial health check should be clean
	report, err := monitor.Check(ctx, "products")
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if report.DriftPercentage > 0 {
		t.Errorf("Expected no drift initially, got %.2f%%", report.DriftPercentage)
	}

	// Simulate drift by manually removing index entries
	redisClient.Del(ctx, "idx:products:category:electronics")

	// Check again - should detect drift
	report, err = monitor.Check(ctx, "products")
	if err != nil {
		t.Fatalf("Check after drift failed: %v", err)
	}
	if report.DriftPercentage == 0 {
		t.Error("Should detect drift after index deletion")
	}

	if report.MissingInRedis != 10 {
		t.Errorf("Expected 10 missing entries, got %d", report.MissingInRedis)
	}

	// Repair drift
	err = monitor.RepairDrift(ctx, report)
	if err != nil {
		t.Fatalf("RepairDrift failed: %v", err)
	}

	// Verify repair worked
	count, err := redisIndexer.Count(ctx, "products", "category", "electronics")
	if err != nil {
		t.Fatalf("Count after repair failed: %v", err)
	}

	if count != 10 {
		t.Errorf("Expected 10 entries after repair, got %d", count)
	}

	// Final health check should be clean
	report, err = monitor.Check(ctx, "products")
	if err != nil {
		t.Fatalf("Final check failed: %v", err)
	}

	if report.DriftPercentage > 0 {
		t.Errorf("Expected no drift after repair, got %.2f%%", report.DriftPercentage)
	}
}

// TestIntegration_HighConcurrencyIndexing validates index updates under load
func TestIntegration_HighConcurrencyIndexing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high concurrency test in short mode")
	}

	ctx := context.Background()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	redisIndexer := NewRedisIndexer(redisClient)

	redisIndexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:       "sessions",
		EntityType: "sessions",
		ExtractFunc: func(objectKey string, data []byte) ([]IndexEntry, error) {
			return ExtractJSONField("user_id")(objectKey, data)
		},
	})

	indexManager := NewIndexManager(store).WithRedisIndexer(redisIndexer)

	// Create 100 sessions concurrently across 10 users
	var wg sync.WaitGroup
	concurrency := 10

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		userID := "user-" + string(rune('0'+i))
		go func(uid string) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				session := map[string]interface{}{
					"id":      uid + "-session-" + string(rune('0'+j)),
					"user_id": uid,
				}
				key := "sessions/" + uid + "-" + string(rune('0'+j)) + ".json"
				if err := indexManager.Create(ctx, key, session); err != nil {
					t.Errorf("Failed to create session: %v", err)
				}
			}
		}(userID)
	}

	wg.Wait()

	// Verify all indexes are correct
	for i := 0; i < concurrency; i++ {
		userID := "user-" + string(rune('0'+i))
		count, err := redisIndexer.Count(ctx, "sessions", "user_id", userID)
		if err != nil {
			t.Errorf("Failed to count sessions for %s: %v", userID, err)
		}
		if count != 10 {
			t.Errorf("Expected 10 sessions for %s, got %d", userID, count)
		}
	}
}
