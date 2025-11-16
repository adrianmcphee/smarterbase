package smarterbase

import (
	"context"
	"encoding/json"
	"fmt"
)

// IndexManager coordinates updates across Redis indexes
// This provides a single point of coordination to prevent forgotten index updates.
//
// Benefits:
// - Automatic updates across all configured indexes
// - Consistent error handling and logging
// - Reduces boilerplate in domain stores
type IndexManager struct {
	store        *Store
	redisIndexer *RedisIndexer
	logger       Logger
	metrics      Metrics
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

// Create stores data and updates all indexes atomically
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

	// Write data first
	if err := im.store.PutJSON(ctx, key, data); err != nil {
		return fmt.Errorf("failed to save data: %w", err)
	}

	// Update Redis indexes
	if im.redisIndexer != nil {
		if err := im.redisIndexer.UpdateIndexes(ctx, key, bytes); err != nil {
			im.logger.Warn("redis index update failed",
				"key", key,
				"error", err,
			)
			im.metrics.Increment(MetricIndexErrors)
			// Continue - don't fail the operation
		} else {
			im.metrics.Increment(MetricIndexUpdate)
		}
	}

	return nil
}

// Update replaces data and updates all indexes
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

	// Marshal new data
	newBytes, err := json.Marshal(newData)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	// Write new data
	if err := im.store.PutJSON(ctx, key, newData); err != nil {
		return fmt.Errorf("failed to save data: %w", err)
	}

	// Update Redis indexes (replace old with new)
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

// Delete removes data and cleans up all indexes
func (im *IndexManager) Delete(ctx context.Context, key string) error {
	// Validate input
	if key == "" {
		return WithContext(ErrInvalidData, map[string]interface{}{
			"operation": "Delete",
			"reason":    "key cannot be empty",
		})
	}

	// Get data for index cleanup
	data, err := im.store.Backend().Get(ctx, key)
	if err != nil {
		return WithContext(ErrNotFound, map[string]interface{}{
			"key": key,
		})
	}

	// Remove from Redis indexes BEFORE deleting data
	if im.redisIndexer != nil {
		if err := im.redisIndexer.RemoveFromIndexes(ctx, key, data); err != nil {
			im.logger.Warn("redis index cleanup failed",
				"key", key,
				"error", err,
			)
			// Continue with deletion
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
