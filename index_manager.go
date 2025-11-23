package smarterbase

import (
	"context"
	"encoding/json"
	"fmt"
)

// IndexManager coordinates updates across Redis indexes and uniqueness constraints
// This provides a single point of coordination to prevent forgotten index updates.
//
// Benefits:
// - Automatic updates across all configured indexes
// - Atomic uniqueness constraints (prevents duplicates)
// - Consistent error handling and logging
// - Reduces boilerplate in domain stores
type IndexManager struct {
	store             *Store
	redisIndexer      *RedisIndexer
	constraintManager *ConstraintManager
	logger            Logger
	metrics           Metrics
}

// NewIndexManager creates a new index manager
func NewIndexManager(store *Store) *IndexManager {
	return &IndexManager{
		store:   store,
		logger:  store.logger,
		metrics: store.metrics,
	}
}

// WithRedisIndexer adds Redis-based indexing
func (im *IndexManager) WithRedisIndexer(indexer *RedisIndexer) *IndexManager {
	im.redisIndexer = indexer
	return im
}

// WithConstraintManager adds uniqueness constraint enforcement
func (im *IndexManager) WithConstraintManager(manager *ConstraintManager) *IndexManager {
	im.constraintManager = manager
	return im
}

// Create stores data and updates all indexes atomically
//
// CRITICAL: Enforces uniqueness constraints BEFORE writing to storage.
// If any unique field (email, platform_user_id, etc.) already exists,
// this will fail with ConstraintViolationError - preventing duplicates.
func (im *IndexManager) Create(ctx context.Context, key string, data interface{}) error {
	// Validate input
	if key == "" {
		return WithContext(ErrInvalidData, map[string]interface{}{
			"operation": "Create",
			"reason":    "key cannot be empty",
		})
	}
	if data == nil {
		return WithContext(ErrInvalidData, map[string]interface{}{
			"operation": "Create",
			"key":       key,
			"reason":    "data cannot be nil",
		})
	}

	// Marshal once
	bytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	if len(bytes) == 0 {
		return WithContext(ErrInvalidData, map[string]interface{}{
			"operation": "Create",
			"key":       key,
			"reason":    "marshaled data is empty",
		})
	}

	// STEP 1: Claim uniqueness constraints BEFORE writing (prevents race conditions)
	var claimedKeys []string
	if im.constraintManager != nil {
		// Extract entity type from key (e.g., "users/123.json" → "users")
		entityType := extractEntityType(key)

		claimedKeys, err = im.constraintManager.ClaimUniqueKeys(ctx, entityType, key, data)
		if err != nil {
			// Uniqueness constraint violated - fail immediately
			return err
		}
	}

	// STEP 2: Write data to storage
	if err := im.store.PutJSON(ctx, key, data); err != nil {
		// Storage write failed - rollback claimed constraints
		if len(claimedKeys) > 0 {
			_ = im.constraintManager.ReleaseUniqueKeys(ctx, claimedKeys)
		}
		return fmt.Errorf("failed to save data: %w", err)
	}

	// STEP 3: Update Redis indexes (best effort)
	if im.redisIndexer != nil {
		if err := im.redisIndexer.UpdateIndexes(ctx, key, bytes); err != nil {
			im.logger.Warn("redis index update failed",
				"key", key,
				"error", err,
			)
			im.metrics.Increment(MetricIndexErrors)
			// Don't rollback - indexes can be rebuilt
		} else {
			im.metrics.Increment(MetricIndexUpdate)
		}
	}

	return nil
}

// extractEntityType extracts entity type from object key
// Example: "users/123/profile.json" → "users"
// Example: "admin_users/456/profile.json" → "admin_users"
func extractEntityType(key string) string {
	// Simple extraction: everything before first "/"
	for i, c := range key {
		if c == '/' {
			return key[:i]
		}
	}
	return key // No slash found - use whole key as type
}

// Update replaces data and updates all indexes
//
// Handles uniqueness constraints atomically:
// 1. Claims new unique values (if changed)
// 2. Writes to storage
// 3. Releases old unique values
func (im *IndexManager) Update(ctx context.Context, key string, newData interface{}) error {
	// Validate input
	if key == "" {
		return WithContext(ErrInvalidData, map[string]interface{}{
			"operation": "Update",
			"reason":    "key cannot be empty",
		})
	}
	if newData == nil {
		return WithContext(ErrInvalidData, map[string]interface{}{
			"operation": "Update",
			"key":       key,
			"reason":    "data cannot be nil",
		})
	}

	// Get old data for index cleanup
	oldBytes, err := im.store.Backend().Get(ctx, key)
	if err != nil && !IsNotFound(err) {
		return err
	}

	// Unmarshal old data for constraint updates
	var oldData interface{}
	if len(oldBytes) > 0 {
		if err := json.Unmarshal(oldBytes, &oldData); err != nil {
			oldData = nil // Can't unmarshal - skip constraint cleanup
		}
	}

	// Marshal new data
	newBytes, err := json.Marshal(newData)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	// STEP 1: Update uniqueness constraints (claim new, release old)
	var claimedKeys []string
	if im.constraintManager != nil {
		entityType := extractEntityType(key)

		claimedKeys, err = im.constraintManager.UpdateUniqueKeys(ctx, entityType, key, oldData, newData)
		if err != nil {
			// Constraint violation - unique field already taken by another entity
			return err
		}
	}

	// STEP 2: Write new data
	if err := im.store.PutJSON(ctx, key, newData); err != nil {
		// Storage write failed - rollback new constraints, restore old
		if len(claimedKeys) > 0 {
			_ = im.constraintManager.ReleaseUniqueKeys(ctx, claimedKeys)
			// TODO: Re-claim old keys (complex rollback)
		}
		return fmt.Errorf("failed to save data: %w", err)
	}

	// STEP 3: Update Redis indexes (replace old with new)
	if im.redisIndexer != nil {
		if err := im.redisIndexer.ReplaceIndexes(ctx, key, oldBytes, newBytes); err != nil {
			im.logger.Warn("redis index replace failed",
				"key", key,
				"error", err,
			)
			im.metrics.Increment(MetricIndexErrors)
		}
	}

	return nil
}

// Delete removes data and cleans up all indexes and constraints
func (im *IndexManager) Delete(ctx context.Context, key string) error {
	// Validate input
	if key == "" {
		return WithContext(ErrInvalidData, map[string]interface{}{
			"operation": "Delete",
			"reason":    "key cannot be empty",
		})
	}

	// Get data for index cleanup
	dataBytes, err := im.store.Backend().Get(ctx, key)
	if err != nil {
		return WithContext(ErrNotFound, map[string]interface{}{
			"key": key,
		})
	}

	// Unmarshal data for constraint cleanup
	var data interface{}
	if len(dataBytes) > 0 {
		_ = json.Unmarshal(dataBytes, &data) // Best effort
	}

	// Remove from Redis indexes BEFORE deleting data
	if im.redisIndexer != nil {
		if err := im.redisIndexer.RemoveFromIndexes(ctx, key, dataBytes); err != nil {
			im.logger.Warn("redis index cleanup failed",
				"key", key,
				"error", err,
			)
			// Continue with deletion
		}
	}

	// Remove uniqueness constraints
	if im.constraintManager != nil && data != nil {
		entityType := extractEntityType(key)
		constraintKeys := im.constraintManager.extractConstraintKeys(ctx, entityType, key, data)
		if len(constraintKeys) > 0 {
			_ = im.constraintManager.ReleaseUniqueKeys(ctx, constraintKeys)
		}
	}

	// Delete the data
	if err := im.store.Delete(ctx, key); err != nil {
		return err
	}

	return nil
}

// Get retrieves data without index updates
func (im *IndexManager) Get(ctx context.Context, key string, dest interface{}) error {
	return im.store.GetJSON(ctx, key, dest)
}

// Exists checks if data exists
func (im *IndexManager) Exists(ctx context.Context, key string) (bool, error) {
	return im.store.Exists(ctx, key)
}

// Example usage:
//
//	// In store initialization:
//	indexManager := smarterbase.NewIndexManager(store).
//	    WithRedisIndexer(redisIndexer)
//
//	// In CRUD operations:
//	func (s *Store) CreateUser(ctx context.Context, user *User) error {
//	    key := fmt.Sprintf("users/%s.json", user.ID)
//	    return s.indexManager.Create(ctx, key, user)
//	    // All indexes updated automatically!
//	}
//
//	// Query with type safety (package-level functions):
//	users, err := smarterbase.QueryIndexTyped[User](ctx, indexManager, "users", "role", "admin")
//	user, err := smarterbase.GetByIndex[User](ctx, indexManager, "users", "email", "alice@example.com")
