package smarterbase

import (
	"context"
	"encoding/json"
	"fmt"
)

// OptimisticTransaction provides best-effort transactional semantics using optimistic locking.
//
// ⚠️ IMPORTANT LIMITATIONS:
// - This is NOT true ACID transactions
// - Uses optimistic locking with best-effort rollback
// - Rollback may fail, leaving partial updates
// - Race conditions possible on non-tracked keys
//
// When to use:
// - Low-contention scenarios where conflicts are rare
// - Non-critical data where eventual consistency is acceptable
// - Coordinating updates across multiple objects
//
// When NOT to use:
// - High-contention scenarios (use Redis locks or DynamoDB transactions)
// - Financial transactions or critical data requiring strict consistency
// - Operations that must be atomic across distributed systems
//
// For true ACID transactions, consider:
// - DynamoDB Transactions (TransactWriteItems)
// - Redis-based distributed locks
// - Application-level saga pattern with compensation
type OptimisticTransaction struct {
	store   *Store
	writes  []writeOp
	deletes []string
	etags   map[string]string // Track ETags for optimistic locking
}

// Transaction is deprecated. Use OptimisticTransaction instead.
// Kept for backward compatibility.
type Transaction = OptimisticTransaction

type writeOp struct {
	key   string
	value interface{}
}

// BeginTx creates a new optimistic transaction
func (s *Store) BeginTx(ctx context.Context) *OptimisticTransaction {
	return &OptimisticTransaction{
		store:  s,
		writes: make([]writeOp, 0),
		etags:  make(map[string]string),
	}
}

// Put queues a write operation
func (tx *OptimisticTransaction) Put(key string, value interface{}) {
	tx.writes = append(tx.writes, writeOp{key: key, value: value})
}

// Delete queues a delete operation
func (tx *OptimisticTransaction) Delete(key string) {
	tx.deletes = append(tx.deletes, key)
}

// Get retrieves a value and tracks its ETag for optimistic locking
func (tx *OptimisticTransaction) Get(ctx context.Context, key string, dest interface{}) error {
	etag, err := tx.store.GetJSONWithETag(ctx, key, dest)
	if err != nil {
		return err
	}
	tx.etags[key] = etag
	return nil
}

// Commit attempts to commit all operations using optimistic locking.
// If any operation fails, attempts to rollback (best effort).
//
// Returns an error if:
// - Any ETag check fails (concurrent modification detected)
// - Any write/delete operation fails
// - Rollback fails (data may be in inconsistent state)
func (tx *OptimisticTransaction) Commit(ctx context.Context) error {
	// Track what we've written for potential rollback
	written := make([]string, 0)
	originalValues := make(map[string][]byte)

	// Step 1: Backup existing values for potential rollback
	for _, op := range tx.writes {
		if data, err := tx.store.backend.Get(ctx, op.key); err == nil {
			originalValues[op.key] = data
		}
	}

	// Step 2: Execute all writes with optimistic locking
	for _, op := range tx.writes {
		data, err := json.Marshal(op.value)
		if err != nil {
			tx.rollback(ctx, written, originalValues)
			return fmt.Errorf("marshal error for %s: %w", op.key, err)
		}

		// Use PutIfMatch for keys we've read (optimistic locking)
		if expectedETag, tracked := tx.etags[op.key]; tracked {
			_, err = tx.store.backend.PutIfMatch(ctx, op.key, data, expectedETag)
			if err != nil {
				tx.rollback(ctx, written, originalValues)
				return fmt.Errorf("optimistic lock failed for %s: %w", op.key, err)
			}
		} else {
			// Regular put for keys we didn't read
			if err := tx.store.backend.Put(ctx, op.key, data); err != nil {
				tx.rollback(ctx, written, originalValues)
				return fmt.Errorf("write error for %s: %w", op.key, err)
			}
		}

		written = append(written, op.key)
	}

	// Step 4: Execute all deletes
	for _, key := range tx.deletes {
		if data, err := tx.store.backend.Get(ctx, key); err == nil {
			originalValues[key] = data
		}

		if err := tx.store.backend.Delete(ctx, key); err != nil {
			tx.rollback(ctx, written, originalValues)
			return fmt.Errorf("delete error for %s: %w", key, err)
		}
	}

	return nil
}

// Rollback attempts to restore original values (best effort)
func (tx *OptimisticTransaction) Rollback(ctx context.Context) error {
	return tx.rollback(ctx, nil, nil)
}

func (tx *OptimisticTransaction) rollback(ctx context.Context, written []string, originalValues map[string][]byte) error {
	var rollbackErrors []error

	// Restore written keys to original values
	for _, key := range written {
		if originalData, exists := originalValues[key]; exists {
			// Restore original value
			if err := tx.store.backend.Put(ctx, key, originalData); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("failed to restore %s: %w", key, err))
			}
		} else {
			// Key didn't exist before, delete it
			if err := tx.store.backend.Delete(ctx, key); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("failed to delete %s: %w", key, err))
			}
		}
	}

	// Restore deleted keys
	for _, key := range tx.deletes {
		if originalData, exists := originalValues[key]; exists {
			if err := tx.store.backend.Put(ctx, key, originalData); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("failed to restore deleted %s: %w", key, err))
			}
		}
	}

	if len(rollbackErrors) > 0 {
		return fmt.Errorf("rollback incomplete (%d errors): %v", len(rollbackErrors), rollbackErrors)
	}

	return nil
}

// WithTransaction executes a function within an optimistic transaction.
// Automatically commits on success, rolls back on error.
//
// ⚠️ WARNING: This does NOT provide isolation guarantees!
// Another process can modify data between your Get() and Put() calls.
//
// ❌ DO NOT USE for critical updates like:
// - Financial transactions (account balances, payments)
// - Inventory updates
// - Counter increments
// - Any operation where race conditions would cause data corruption
//
// ✅ USE distributed locks instead for critical updates:
//
//	lock := smarterbase.NewDistributedLock(redisClient, "smarterbase")
//	err := smarterbase.WithAtomicUpdate(ctx, store, lock, "accounts/123", 10*time.Second,
//	    func(ctx context.Context) error {
//	        var account Account
//	        store.GetJSON(ctx, "accounts/123", &account)
//	        account.Balance += 100 // Safe: protected by distributed lock
//	        store.PutJSON(ctx, "accounts/123", &account)
//	        return nil
//	    })
//
// Example (optimistic transaction - use only for low-contention scenarios):
//
//	err := store.WithTransaction(ctx, func(tx *OptimisticTransaction) error {
//	    // Read with optimistic lock
//	    var user User
//	    if err := tx.Get(ctx, "users/123", &user); err != nil {
//	        return err
//	    }
//
//	    // ⚠️ CAUTION: Another process could modify user here!
//	    user.LastSeen = time.Now()
//
//	    // Queue write (will check ETag on commit)
//	    tx.Put("users/123", user)
//	    return nil
//	})
func (s *Store) WithTransaction(ctx context.Context, fn func(tx *OptimisticTransaction) error) error {
	tx := s.BeginTx(ctx)

	if err := fn(tx); err != nil {
		tx.Rollback(ctx)
		return err
	}

	return tx.Commit(ctx)
}
