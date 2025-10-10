package smarterbase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisIndexer provides fast multi-value secondary indexes using Redis Sets.
//
// Purpose: Enables O(1) lookups for non-unique indexes like:
// - user_id → [session1, session2, ...]
// - postcode → [vision_card1, vision_card2, ...]
// - area_id → [photo1, photo2, ...]
//
// Performance: Prevents expensive O(N) scans of all objects in S3/filesystem.
//
// Architecture:
// - File-based Indexer: Unique 1:1 mappings (email → user)
// - RedisIndexer: Multi-value 1:N mappings (user_id → sessions)
type RedisIndexer struct {
	redis      *redis.Client
	specs      map[string]*MultiIndexSpec
	ownsClient bool // If true, Close() will close the Redis client
}

// MultiIndexSpec defines a multi-value secondary index
type MultiIndexSpec struct {
	Name        string                                      // e.g., "sessions-by-user-id"
	EntityType  string                                      // e.g., "sessions" (for key namespacing)
	ExtractFunc func(objectKey string, data []byte) ([]IndexEntry, error) // Extract index values from object
	TTL         time.Duration                               // Optional TTL for index keys (0 = no expiry)
}

// IndexEntry represents a single index value for an object
type IndexEntry struct {
	IndexName  string // e.g., "user_id", "area_id", "postcode"
	IndexValue string // e.g., "user-123", "area-456", "1234AB"
}

// NewRedisIndexer creates a new Redis-backed indexer
// If ownConnection is true, the indexer will close the Redis client on Close()
func NewRedisIndexer(redis *redis.Client) *RedisIndexer {
	return &RedisIndexer{
		redis: redis,
		specs: make(map[string]*MultiIndexSpec),
	}
}

// NewRedisIndexerWithOwnedClient creates a new Redis indexer that owns the client
// The client will be closed when Close() is called
func NewRedisIndexerWithOwnedClient(redis *redis.Client) *RedisIndexer {
	return &RedisIndexer{
		redis:      redis,
		specs:      make(map[string]*MultiIndexSpec),
		ownsClient: true,
	}
}

// RegisterMultiIndex registers a multi-value index specification
func (r *RedisIndexer) RegisterMultiIndex(spec *MultiIndexSpec) {
	r.specs[spec.Name] = spec
}

// UpdateIndexes updates all registered multi-value indexes for an object
//
// Call this after Put() operations:
//   store.PutJSON(ctx, key, session)
//   redisIndexer.UpdateIndexes(ctx, key, data)
func (r *RedisIndexer) UpdateIndexes(ctx context.Context, objectKey string, data []byte) error {
	if r.redis == nil {
		return nil // Graceful degradation if Redis unavailable
	}

	for _, spec := range r.specs {
		if err := r.updateIndex(ctx, spec, objectKey, data); err != nil {
			return fmt.Errorf("failed to update index %s: %w", spec.Name, err)
		}
	}
	return nil
}

// updateIndex updates a single multi-value index
func (r *RedisIndexer) updateIndex(ctx context.Context, spec *MultiIndexSpec, objectKey string, data []byte) error {
	// Extract index entries from object
	entries, err := spec.ExtractFunc(objectKey, data)
	if err != nil {
		return nil // Skip if extraction fails (object might not have this field)
	}

	if len(entries) == 0 {
		return nil // No index values to store
	}

	// Add object to each index value's set
	for _, entry := range entries {
		setKey := r.getSetKey(spec.EntityType, entry.IndexName, entry.IndexValue)

		// Add to Redis Set (SADD is idempotent)
		if err := r.redis.SAdd(ctx, setKey, objectKey).Err(); err != nil {
			return fmt.Errorf("failed to add to Redis set %s: %w", setKey, err)
		}

		// Set TTL if configured
		if spec.TTL > 0 {
			r.redis.Expire(ctx, setKey, spec.TTL)
		}
	}

	return nil
}

// Query returns all object keys matching an index value
//
// Example: Query(ctx, "user_id", "user-123") → ["sessions/abc.json", "sessions/def.json"]
func (r *RedisIndexer) Query(ctx context.Context, entityType, indexName, indexValue string) ([]string, error) {
	if r.redis == nil {
		return nil, fmt.Errorf("redis not available")
	}

	setKey := r.getSetKey(entityType, indexName, indexValue)

	// Get all members from Redis Set
	members, err := r.redis.SMembers(ctx, setKey).Result()
	if err == redis.Nil {
		return []string{}, nil // Empty set
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query Redis set %s: %w", setKey, err)
	}

	return members, nil
}

// QueryMultiple returns object keys matching ANY of the provided values (OR query)
//
// Example: QueryMultiple(ctx, "properties", "user_id", []string{"user-1", "user-2"})
func (r *RedisIndexer) QueryMultiple(ctx context.Context, entityType, indexName string, indexValues []string) ([]string, error) {
	if r.redis == nil {
		return nil, fmt.Errorf("redis not available")
	}

	if len(indexValues) == 0 {
		return []string{}, nil
	}

	// Build set keys for SUNION
	setKeys := make([]string, len(indexValues))
	for i, value := range indexValues {
		setKeys[i] = r.getSetKey(entityType, indexName, value)
	}

	// Use SUNION for efficient multi-value query
	members, err := r.redis.SUnion(ctx, setKeys...).Result()
	if err == redis.Nil {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query multiple Redis sets: %w", err)
	}

	return members, nil
}

// Count returns the number of objects matching an index value
func (r *RedisIndexer) Count(ctx context.Context, entityType, indexName, indexValue string) (int64, error) {
	if r.redis == nil {
		return 0, fmt.Errorf("redis not available")
	}

	setKey := r.getSetKey(entityType, indexName, indexValue)
	return r.redis.SCard(ctx, setKey).Result()
}

// RemoveFromIndexes removes an object from all indexes
//
// Call this before Delete() operations:
//   redisIndexer.RemoveFromIndexes(ctx, key, oldData)
//   store.Delete(ctx, key)
func (r *RedisIndexer) RemoveFromIndexes(ctx context.Context, objectKey string, data []byte) error {
	if r.redis == nil {
		return nil // Graceful degradation
	}

	for _, spec := range r.specs {
		if err := r.removeFromIndex(ctx, spec, objectKey, data); err != nil {
			return fmt.Errorf("failed to remove from index %s: %w", spec.Name, err)
		}
	}
	return nil
}

// removeFromIndex removes an object from a single index
func (r *RedisIndexer) removeFromIndex(ctx context.Context, spec *MultiIndexSpec, objectKey string, data []byte) error {
	// Extract index entries from object
	entries, err := spec.ExtractFunc(objectKey, data)
	if err != nil {
		return nil // Skip if extraction fails
	}

	// Remove object from each index value's set
	for _, entry := range entries {
		setKey := r.getSetKey(spec.EntityType, entry.IndexName, entry.IndexValue)
		if err := r.redis.SRem(ctx, setKey, objectKey).Err(); err != nil {
			return fmt.Errorf("failed to remove from Redis set %s: %w", setKey, err)
		}
	}

	return nil
}

// ReplaceIndexes atomically updates indexes when an object is modified
//
// This removes the object from old index values and adds it to new ones.
// Call this for Update() operations:
//   oldData, _ := store.Backend().Get(ctx, key)
//   store.PutJSON(ctx, key, newObject)
//   redisIndexer.ReplaceIndexes(ctx, key, oldData, newData)
//
// If oldData is nil/empty, behaves like UpdateIndexes (create case)
func (r *RedisIndexer) ReplaceIndexes(ctx context.Context, objectKey string, oldData, newData []byte) error {
	if r.redis == nil {
		return nil // Graceful degradation
	}

	// Remove from old indexes first (if old data exists)
	if len(oldData) > 0 {
		if err := r.RemoveFromIndexes(ctx, objectKey, oldData); err != nil {
			return fmt.Errorf("failed to remove from old indexes: %w", err)
		}
	}

	// Add to new indexes
	if len(newData) > 0 {
		if err := r.UpdateIndexes(ctx, objectKey, newData); err != nil {
			return fmt.Errorf("failed to add to new indexes: %w", err)
		}
	}

	return nil
}

// getSetKey generates the Redis key for a secondary index
// Format: idx:{entity}:{index_name}:{index_value}
// Example: idx:sessions:user_id:user-123
func (r *RedisIndexer) getSetKey(entityType, indexName, indexValue string) string {
	return fmt.Sprintf("idx:%s:%s:%s", entityType, indexName, indexValue)
}

// RebuildIndex rebuilds a secondary index from scratch
//
// Useful for:
// - Initial data migration
// - Index repair after corruption
// - Adding new indexes to existing data
func (r *RedisIndexer) RebuildIndex(ctx context.Context, spec *MultiIndexSpec, objects map[string][]byte) error {
	if r.redis == nil {
		return fmt.Errorf("redis not available")
	}

	// Clear existing indexes for this spec (optional - may want to keep for rollback)
	// For now, just overwrite

	// Add all objects to indexes
	for objectKey, data := range objects {
		if err := r.updateIndex(ctx, spec, objectKey, data); err != nil {
			return fmt.Errorf("failed to rebuild index for %s: %w", objectKey, err)
		}
	}

	return nil
}

// GetIndexStats returns statistics about an index
func (r *RedisIndexer) GetIndexStats(ctx context.Context, entityType, indexName string, indexValues []string) (map[string]int64, error) {
	if r.redis == nil {
		return nil, fmt.Errorf("redis not available")
	}

	stats := make(map[string]int64)

	for _, value := range indexValues {
		count, err := r.Count(ctx, entityType, indexName, value)
		if err != nil {
			continue
		}
		stats[value] = count
	}

	return stats, nil
}

// Helper function to extract a simple field from JSON
func ExtractJSONField(fieldName string) func(objectKey string, data []byte) ([]IndexEntry, error) {
	return func(objectKey string, data []byte) ([]IndexEntry, error) {
		var obj map[string]interface{}
		if err := json.Unmarshal(data, &obj); err != nil {
			return nil, err
		}

		value, ok := obj[fieldName]
		if !ok {
			return nil, fmt.Errorf("field %s not found", fieldName)
		}

		valueStr, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("field %s is not a string", fieldName)
		}

		if valueStr == "" {
			return nil, fmt.Errorf("field %s is empty", fieldName)
		}

		return []IndexEntry{{
			IndexName:  fieldName,
			IndexValue: valueStr,
		}}, nil
	}
}

// Helper function to extract nested JSON field (e.g., "gallery.postcode")
func ExtractNestedJSONField(fieldPath ...string) func(objectKey string, data []byte) ([]IndexEntry, error) {
	return func(objectKey string, data []byte) ([]IndexEntry, error) {
		var obj map[string]interface{}
		if err := json.Unmarshal(data, &obj); err != nil {
			return nil, err
		}

		// Navigate nested path
		current := obj
		for i, field := range fieldPath[:len(fieldPath)-1] {
			next, ok := current[field]
			if !ok {
				return nil, fmt.Errorf("field path %v not found at level %d", fieldPath, i)
			}
			current, ok = next.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("field path %v is not an object at level %d", fieldPath, i)
			}
		}

		// Get final value
		finalField := fieldPath[len(fieldPath)-1]
		value, ok := current[finalField]
		if !ok {
			return nil, fmt.Errorf("field %s not found", finalField)
		}

		valueStr, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("field %s is not a string", finalField)
		}

		if valueStr == "" {
			return nil, fmt.Errorf("field %s is empty", finalField)
		}

		return []IndexEntry{{
			IndexName:  finalField,
			IndexValue: valueStr,
		}}, nil
	}
}

// Close releases resources held by the indexer
// If the indexer owns the Redis client, it will be closed
func (r *RedisIndexer) Close() error {
	if r.ownsClient && r.redis != nil {
		return r.redis.Close()
	}
	return nil
}
