package smarterbase

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// setupTestRedis creates an in-memory Redis instance for testing
func setupTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	t.Cleanup(func() { mr.Close() })

	return redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
}

// TestTransaction_BasicCommit verifies successful transaction commit
func TestTransaction_BasicCommit(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	tx := store.BeginTx(ctx)

	// Add multiple operations
	tx.Put("tx/user1.json", map[string]string{"name": "Alice"})
	tx.Put("tx/user2.json", map[string]string{"name": "Bob"})
	tx.Put("tx/user3.json", map[string]string{"name": "Charlie"})

	err := tx.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Verify all items were written
	for i := 1; i <= 3; i++ {
		key := fmt.Sprintf("tx/user%d.json", i)
		exists, _ := backend.Exists(ctx, key)
		if !exists {
			t.Errorf("Expected %s to exist after commit", key)
		}
	}
}

// TestTransaction_BasicRollback verifies manual rollback
func TestTransaction_BasicRollback(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	tx := store.BeginTx(ctx)

	tx.Put("tx/temp1.json", map[string]string{"temp": "data"})
	tx.Put("tx/temp2.json", map[string]string{"temp": "data"})

	err := tx.Rollback(ctx)
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Verify no items were written
	exists1, _ := backend.Exists(ctx, "tx/temp1.json")
	exists2, _ := backend.Exists(ctx, "tx/temp2.json")

	if exists1 || exists2 {
		t.Error("Expected no items after rollback")
	}
}

// TestTransaction_OptimisticLockConflict tests ETag-based conflict detection
func TestTransaction_OptimisticLockConflict(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create initial object
	key := "tx/versioned.json"
	store.PutJSON(ctx, key, map[string]int{"version": 1})

	// Start transaction and read with ETag
	tx := store.BeginTx(ctx)
	var data map[string]int
	err := tx.Get(ctx, key, &data)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Simulate another writer modifying the object
	store.PutJSON(ctx, key, map[string]int{"version": 2})

	// Transaction should fail to commit (ETag changed)
	tx.Put(key, map[string]int{"version": 3})
	err = tx.Commit(ctx)
	if err == nil {
		t.Error("Expected commit to fail due to ETag mismatch")
	}

	if err != nil && err.Error() != "" {
		t.Logf("Got expected error: %v", err)
	}
}

// TestTransaction_MixedOperations tests Put and Delete in same transaction
func TestTransaction_MixedOperations(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create existing object
	store.PutJSON(ctx, "tx/old.json", map[string]string{"status": "old"})

	tx := store.BeginTx(ctx)
	tx.Put("tx/new.json", map[string]string{"status": "new"})
	tx.Delete("tx/old.json")

	err := tx.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Verify new exists, old doesn't
	newExists, _ := backend.Exists(ctx, "tx/new.json")
	oldExists, _ := backend.Exists(ctx, "tx/old.json")

	if !newExists {
		t.Error("Expected new.json to exist")
	}

	if oldExists {
		t.Error("Expected old.json to be deleted")
	}
}

// TestWithTransaction_Success tests automatic commit wrapper
func TestWithTransaction_Success(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	err := store.WithTransaction(ctx, func(tx *Transaction) error {
		tx.Put("tx/auto1.json", map[string]string{"id": "1"})
		tx.Put("tx/auto2.json", map[string]string{"id": "2"})
		return nil // Success - should auto-commit
	})

	if err != nil {
		t.Fatalf("WithTransaction failed: %v", err)
	}

	// Verify items exist
	exists1, _ := backend.Exists(ctx, "tx/auto1.json")
	exists2, _ := backend.Exists(ctx, "tx/auto2.json")

	if !exists1 || !exists2 {
		t.Error("Expected items to exist after successful WithTransaction")
	}
}

// TestWithTransaction_AutoRollback tests automatic rollback on error
func TestWithTransaction_AutoRollback(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	expectedErr := errors.New("simulated error")

	err := store.WithTransaction(ctx, func(tx *Transaction) error {
		tx.Put("tx/rollback1.json", map[string]string{"id": "1"})
		tx.Put("tx/rollback2.json", map[string]string{"id": "2"})
		return expectedErr // Error - should auto-rollback
	})

	if err != expectedErr {
		t.Fatalf("Expected error %v, got %v", expectedErr, err)
	}

	// Verify items were rolled back
	exists1, _ := backend.Exists(ctx, "tx/rollback1.json")
	exists2, _ := backend.Exists(ctx, "tx/rollback2.json")

	if exists1 || exists2 {
		t.Error("Expected items to be rolled back after error")
	}
}

// TestTransaction_ContextCancellation tests context cancellation handling
func TestTransaction_ContextCancellation(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	ctx, cancel := context.WithCancel(context.Background())

	tx := store.BeginTx(ctx)
	tx.Put("tx/cancel1.json", map[string]string{"id": "1"})

	// Cancel context before commit
	cancel()

	err := tx.Commit(ctx)
	// Behavior depends on implementation - may succeed or fail
	// At minimum, should not panic
	t.Logf("Commit with cancelled context: %v", err)
}

// TestTransaction_ConcurrentConflicts tests multiple transactions on same key
func TestTransaction_ConcurrentConflicts(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	key := "tx/contested.json"
	store.PutJSON(ctx, key, map[string]int{"counter": 0})

	workers := 10
	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			tx := store.BeginTx(ctx)

			// Read current value with ETag
			var data map[string]int
			readErr := tx.Get(ctx, key, &data)
			if readErr != nil || data == nil {
				// Failed to read, skip this transaction
				return
			}

			// Increment counter
			data["counter"]++
			tx.Put(key, data)

			// Try to commit - some will fail due to conflicts
			commitErr := tx.Commit(ctx)
			if commitErr == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(w)
	}

	wg.Wait()

	t.Logf("Concurrent transactions: %d succeeded out of %d", successCount, workers)

	// At least some should succeed (not all conflict)
	if successCount == 0 {
		t.Error("Expected at least one transaction to succeed")
	}

	// Not all should succeed (conflicts expected)
	if successCount == workers {
		t.Error("Expected some conflicts, but all transactions succeeded")
	}
}

// TestTransaction_RollbackUpdate tests rollback of updated object
func TestTransaction_RollbackUpdate(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create initial object
	key := "tx/update.json"
	originalData := map[string]string{"status": "original"}
	store.PutJSON(ctx, key, originalData)

	// Start transaction, update, then rollback
	tx := store.BeginTx(ctx)
	tx.Put(key, map[string]string{"status": "modified"})

	err := tx.Rollback(ctx)
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Verify object is restored to original
	var retrieved map[string]string
	store.GetJSON(ctx, key, &retrieved)

	if retrieved["status"] != "original" {
		t.Errorf("Expected status=original after rollback, got %s", retrieved["status"])
	}
}

// TestTransaction_DeleteRollback tests rollback of delete operation
func TestTransaction_DeleteRollback(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create object
	key := "tx/delete-rollback.json"
	originalData := map[string]string{"keep": "me"}
	store.PutJSON(ctx, key, originalData)

	// Start transaction, delete, then rollback
	tx := store.BeginTx(ctx)
	tx.Delete(key)

	err := tx.Rollback(ctx)
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Verify object still exists
	var retrieved map[string]string
	err = store.GetJSON(ctx, key, &retrieved)
	if err != nil {
		t.Error("Expected object to be restored after delete rollback")
	}

	if retrieved["keep"] != "me" {
		t.Error("Object data not restored correctly")
	}
}

// Benchmark transaction performance
func BenchmarkTransaction_5Writes(b *testing.B) {
	ctx := context.Background()
	backend := NewFilesystemBackend(b.TempDir())
	store := NewStore(backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx := store.BeginTx(ctx)
		for j := 0; j < 5; j++ {
			key := fmt.Sprintf("bench/tx%d/item%d.json", i, j)
			tx.Put(key, map[string]int{"id": j})
		}
		tx.Commit(ctx)
	}
}

func BenchmarkTransaction_WithOptimisticLocking(b *testing.B) {
	ctx := context.Background()
	backend := NewFilesystemBackend(b.TempDir())
	store := NewStore(backend)

	// Setup initial object
	key := "bench/locked.json"
	store.PutJSON(ctx, key, map[string]int{"counter": 0})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx := store.BeginTx(ctx)
		var data map[string]int
		tx.Get(ctx, key, &data)
		data["counter"]++
		tx.Put(key, data)

		// Retry on conflict
		for {
			err := tx.Commit(ctx)
			if err == nil {
				break
			}
			// Retry
			tx = store.BeginTx(ctx)
			tx.Get(ctx, key, &data)
			data["counter"]++
			tx.Put(key, data)
			time.Sleep(time.Millisecond)
		}
	}
}

// TestWithAtomicUpdate_BasicSuccess verifies atomic update succeeds
func TestWithAtomicUpdate_BasicSuccess(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	redisClient := setupTestRedis(t)
	lock := NewDistributedLock(redisClient, "test")

	// Setup initial account
	key := "accounts/123"
	store.PutJSON(ctx, key, map[string]int{"balance": 100})

	// Perform atomic update
	err := WithAtomicUpdate(ctx, store, lock, key, 5*time.Second,
		func(ctx context.Context) error {
			var account map[string]int
			store.GetJSON(ctx, key, &account)
			account["balance"] += 50
			store.PutJSON(ctx, key, &account)
			return nil
		})

	if err != nil {
		t.Fatalf("WithAtomicUpdate failed: %v", err)
	}

	// Verify balance was updated
	var result map[string]int
	store.GetJSON(ctx, key, &result)
	if result["balance"] != 150 {
		t.Errorf("Expected balance=150, got %d", result["balance"])
	}
}

// TestWithAtomicUpdate_PreventsRaceConditions demonstrates that WithAtomicUpdate
// prevents concurrent modifications that would cause lost updates
func TestWithAtomicUpdate_PreventsRaceConditions(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	redisClient := setupTestRedis(t)
	lock := NewDistributedLock(redisClient, "test")

	key := "accounts/contested"
	store.PutJSON(ctx, key, map[string]int{"balance": 0})

	workers := 10
	incrementsPerWorker := 10
	var wg sync.WaitGroup

	// All workers increment the counter using WithAtomicUpdate
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < incrementsPerWorker; i++ {
				// Use longer TTL and custom retry logic for high-contention test
				maxRetries := 10
				for retry := 0; retry < maxRetries; retry++ {
					err := WithAtomicUpdate(ctx, store, lock, key, 10*time.Second,
						func(ctx context.Context) error {
							var account map[string]int
							store.GetJSON(ctx, key, &account)
							account["balance"]++
							store.PutJSON(ctx, key, &account)
							return nil
						})
					if err == nil {
						break // Success
					}
					if retry == maxRetries-1 {
						t.Errorf("WithAtomicUpdate failed after %d retries: %v", maxRetries, err)
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
		}()
	}

	wg.Wait()

	// Verify final balance is exactly what we expect (no lost updates)
	var final map[string]int
	store.GetJSON(ctx, key, &final)

	expected := workers * incrementsPerWorker
	if final["balance"] != expected {
		t.Errorf("Expected balance=%d (no lost updates), got %d", expected, final["balance"])
	} else {
		t.Logf("✅ SUCCESS: All %d updates were applied atomically with no race conditions", expected)
	}
}

// TestWithAtomicUpdate_VsOptimisticTransaction compares the behavior of
// atomic updates vs optimistic transactions under concurrent load
func TestWithAtomicUpdate_VsOptimisticTransaction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping comparison test in short mode")
	}

	ctx := context.Background()
	redisClient := setupTestRedis(t)

	// Test 1: Optimistic transactions (will have conflicts)
	t.Run("OptimisticTransaction_HasLostUpdates", func(t *testing.T) {
		backend := NewFilesystemBackend(t.TempDir())
		store := NewStore(backend)
		key := "accounts/optimistic"
		store.PutJSON(ctx, key, map[string]int{"balance": 0})

		workers := 5
		incrementsPerWorker := 20
		var wg sync.WaitGroup
		successCount := 0
		var mu sync.Mutex

		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < incrementsPerWorker; i++ {
					err := store.WithTransaction(ctx, func(tx *OptimisticTransaction) error {
						var account map[string]int
						tx.Get(ctx, key, &account)
						// Simulate some processing time
						time.Sleep(time.Millisecond)
						account["balance"]++
						tx.Put(key, account)
						return nil
					})
					if err == nil {
						mu.Lock()
						successCount++
						mu.Unlock()
					}
				}
			}()
		}

		wg.Wait()

		var final map[string]int
		store.GetJSON(ctx, key, &final)

		expected := workers * incrementsPerWorker
		t.Logf("⚠️  Optimistic Transactions: Expected %d updates, got %d (%d conflicts)",
			expected, final["balance"], expected-final["balance"])

		// Expect some lost updates due to conflicts
		if final["balance"] == expected {
			t.Log("Note: No conflicts occurred (may happen with low contention)")
		}
	})

	// Test 2: Atomic updates (no lost updates)
	t.Run("WithAtomicUpdate_NoLostUpdates", func(t *testing.T) {
		backend := NewFilesystemBackend(t.TempDir())
		store := NewStore(backend)
		lock := NewDistributedLock(redisClient, "test")
		key := "accounts/atomic"
		store.PutJSON(ctx, key, map[string]int{"balance": 0})

		workers := 5
		incrementsPerWorker := 20
		var wg sync.WaitGroup

		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < incrementsPerWorker; i++ {
					// Retry with backoff for high contention
					maxRetries := 10
					for retry := 0; retry < maxRetries; retry++ {
						err := WithAtomicUpdate(ctx, store, lock, key, 10*time.Second,
							func(ctx context.Context) error {
								var account map[string]int
								store.GetJSON(ctx, key, &account)
								// Same processing time as optimistic version
								time.Sleep(time.Millisecond)
								account["balance"]++
								store.PutJSON(ctx, key, &account)
								return nil
							})
						if err == nil {
							break // Success
						}
						if retry == maxRetries-1 {
							t.Errorf("WithAtomicUpdate failed after %d retries: %v", maxRetries, err)
						}
						time.Sleep(10 * time.Millisecond)
					}
				}
			}()
		}

		wg.Wait()

		var final map[string]int
		store.GetJSON(ctx, key, &final)

		expected := workers * incrementsPerWorker
		if final["balance"] != expected {
			t.Errorf("✗ Expected %d updates, got %d (lost %d updates)",
				expected, final["balance"], expected-final["balance"])
		} else {
			t.Logf("✅ WithAtomicUpdate: All %d updates applied successfully (no conflicts)", expected)
		}
	})
}

// TestWithAtomicUpdate_ErrorHandling verifies error handling
func TestWithAtomicUpdate_ErrorHandling(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	redisClient := setupTestRedis(t)
	lock := NewDistributedLock(redisClient, "test")

	key := "accounts/error-test"
	store.PutJSON(ctx, key, map[string]int{"balance": 100})

	expectedErr := errors.New("simulated error")

	// Error in function should be returned
	err := WithAtomicUpdate(ctx, store, lock, key, 5*time.Second,
		func(ctx context.Context) error {
			var account map[string]int
			store.GetJSON(ctx, key, &account)
			account["balance"] += 50
			// Don't save - return error instead
			return expectedErr
		})

	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}

	// Verify balance was NOT changed (error prevents update)
	var result map[string]int
	store.GetJSON(ctx, key, &result)
	if result["balance"] != 100 {
		t.Errorf("Expected balance unchanged at 100, got %d", result["balance"])
	}
}

// TestWithAtomicUpdate_ValidationRequired verifies parameter validation
func TestWithAtomicUpdate_ValidationRequired(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	redisClient := setupTestRedis(t)
	lock := NewDistributedLock(redisClient, "test")

	// Test nil lock
	err := WithAtomicUpdate(ctx, store, nil, "key", 5*time.Second, func(ctx context.Context) error {
		return nil
	})
	if err == nil {
		t.Error("Expected error when lock is nil")
	}

	// Test nil store
	err = WithAtomicUpdate(ctx, nil, lock, "key", 5*time.Second, func(ctx context.Context) error {
		return nil
	})
	if err == nil {
		t.Error("Expected error when store is nil")
	}
}
