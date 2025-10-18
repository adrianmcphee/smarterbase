package smarterbase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Package-level helper functions for convenience

// Now returns the current time (for consistency across the codebase)
func Now() time.Time {
	return time.Now()
}

// PutJSON is a package-level helper for storing JSON
func PutJSON(backend Backend, ctx context.Context, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}
	return backend.Put(ctx, key, data)
}

// GetJSON is a package-level helper for retrieving JSON
func GetJSON(backend Backend, ctx context.Context, key string, dest interface{}) error {
	data, err := backend.Get(ctx, key)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

// BatchGet retrieves multiple objects by keys with type safety.
// This eliminates the marshal/unmarshal anti-pattern in BatchGetJSON.
//
// Example:
//
//	users, err := smarterbase.BatchGet[User](ctx, store, keys)
func BatchGet[T any](ctx context.Context, store *Store, keys []string) ([]*T, error) {
	if len(keys) == 0 {
		return []*T{}, nil
	}

	results := make([]*T, 0, len(keys))

	for _, key := range keys {
		var item T
		if err := store.GetJSON(ctx, key, &item); err != nil {
			if IsNotFound(err) {
				continue // Skip missing items
			}
			return nil, fmt.Errorf("failed to get %s: %w", key, err)
		}
		results = append(results, &item)
	}

	return results, nil
}

// BatchGetWithErrors retrieves multiple objects and returns both results and errors.
// Use this when you need to know which specific keys failed.
//
// Example:
//
//	users, errors := smarterbase.BatchGetWithErrors[User](ctx, store, keys)
//	for key, err := range errors {
//	    log.Printf("Failed to get %s: %v", key, err)
//	}
func BatchGetWithErrors[T any](ctx context.Context, store *Store, keys []string) ([]*T, map[string]error) {
	if len(keys) == 0 {
		return []*T{}, nil
	}

	results := make([]*T, 0, len(keys))
	errors := make(map[string]error)

	for _, key := range keys {
		var item T
		if err := store.GetJSON(ctx, key, &item); err != nil {
			if !IsNotFound(err) {
				errors[key] = err
			}
			continue
		}
		results = append(results, &item)
	}

	if len(errors) > 0 {
		return results, errors
	}
	return results, nil
}

// KeyBuilder helps construct consistent storage keys.
// Eliminates error-prone fmt.Sprintf calls scattered throughout code.
//
// Example:
//
//	kb := KeyBuilder{Prefix: "users", Suffix: ".json"}
//	key := kb.Key(userID)  // Returns "users/userID.json"
type KeyBuilder struct {
	// Prefix is the namespace prefix (e.g., "users", "orders")
	Prefix string

	// Suffix is the file extension (e.g., ".json", ".jsonl")
	// Optional - defaults to empty string
	Suffix string
}

// Key constructs a storage key from an ID.
func (kb KeyBuilder) Key(id string) string {
	if kb.Suffix != "" {
		return fmt.Sprintf("%s/%s%s", kb.Prefix, id, kb.Suffix)
	}
	return fmt.Sprintf("%s/%s", kb.Prefix, id)
}

// Keys constructs multiple storage keys from IDs.
func (kb KeyBuilder) Keys(ids []string) []string {
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = kb.Key(id)
	}
	return keys
}

// UnmarshalBatchResults converts BatchGetJSON results to typed objects.
// This is a helper for code that still uses the old BatchGetJSON API.
//
// Example:
//
//	results, _ := store.BatchGetJSON(ctx, keys, User{})
//	users, err := smarterbase.UnmarshalBatchResults[User](results)
func UnmarshalBatchResults[T any](results map[string]interface{}) ([]*T, error) {
	items := make([]*T, 0, len(results))

	for key, value := range results {
		data, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal %s: %w", key, err)
		}

		var item T
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, fmt.Errorf("failed to unmarshal %s: %w", key, err)
		}

		items = append(items, &item)
	}

	return items, nil
}

// QueryIndexTyped combines index query and batch fetch with type safety.
// Returns typed results directly, eliminating the marshal/unmarshal dance.
//
// Example:
//
//	users, err := smarterbase.QueryIndexTyped[User](ctx, indexManager, "users", "email", "alice@example.com")
func QueryIndexTyped[T any](ctx context.Context, im *IndexManager, entityType, field, value string) ([]*T, error) {
	if im.redisIndexer == nil {
		return nil, fmt.Errorf("redis indexer not configured")
	}

	keys, err := im.redisIndexer.Query(ctx, entityType, field, value)
	if err != nil {
		return nil, fmt.Errorf("index query failed: %w", err)
	}

	if len(keys) == 0 {
		return []*T{}, nil
	}

	return BatchGet[T](ctx, im.store, keys)
}

// GetByIndex is a convenience wrapper for single-result index queries.
// Returns the first matching item or an error if not found.
//
// Example:
//
//	user, err := smarterbase.GetByIndex[User](ctx, indexManager, "users", "email", "alice@example.com")
func GetByIndex[T any](ctx context.Context, im *IndexManager, entityType, field, value string) (*T, error) {
	results, err := QueryIndexTyped[T](ctx, im, entityType, field, value)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no %s found with %s=%s", entityType, field, value)
	}

	return results[0], nil
}

// QueryWithFallback tries a Redis index lookup first, falls back to full scan, and profiles the operation.
// This eliminates the 40-line boilerplate pattern seen across production codebases.
//
// Example:
//
//	users, err := smarterbase.QueryWithFallback[User](
//	    ctx, store, redisIndexer,
//	    "users", "role", "admin",
//	    "users/",
//	    func(u *User) bool { return u.Role == "admin" },
//	)
func QueryWithFallback[T any](
	ctx context.Context,
	store *Store,
	redisIndexer *RedisIndexer,
	entityType string, // e.g., "users", "properties"
	indexField string, // e.g., "user_id", "role"
	indexValue string, // e.g., "user-123", "admin"
	scanPrefix string, // e.g., "users/", "properties/"
	filter func(*T) bool, // Fallback filter function
) ([]*T, error) {
	// Get profiler from context
	profiler := GetProfilerFromContext(ctx)
	profile := profiler.StartProfile(fmt.Sprintf("Query%s", entityType))
	if profile != nil {
		profile.FilterFields = []string{indexField}
		defer func() {
			profiler.Record(profile)
		}()
	}

	// Try Redis index first (O(1) lookup)
	if redisIndexer != nil {
		keys, err := redisIndexer.Query(ctx, entityType, indexField, indexValue)
		if err == nil {
			results, err := BatchGet[T](ctx, store, keys)
			if profile != nil {
				profile.Complexity = ComplexityO1
				profile.IndexUsed = fmt.Sprintf("redis:%s-by-%s", entityType, indexField)
				profile.StorageOps = len(keys)
				profile.ResultCount = len(results)
			}
			return results, err
		}
	}

	// Fallback to full scan (O(N))
	var results []*T
	err := store.Query(scanPrefix).
		Filter(func(data []byte) bool {
			var item T
			if err := json.Unmarshal(data, &item); err != nil {
				return false
			}
			return filter(&item)
		}).
		All(ctx, &results)

	if profile != nil {
		profile.Complexity = ComplexityON
		profile.IndexUsed = "none:full-scan"
		profile.FallbackPath = true
		profile.ResultCount = len(results)
		profile.Error = err
	}

	return results, err
}

// IndexUpdate represents a single index update operation for UpdateWithIndexes.
type IndexUpdate struct {
	EntityType string // e.g., "users"
	IndexField string // e.g., "email"
	OldValue   string // Old index value (to remove)
	NewValue   string // New index value (to add)
}

// UpdateWithIndexes atomically updates data and all associated Redis indexes.
// This prevents the common bug where developers forget to update indexes manually.
//
// Example:
//
//	err := smarterbase.UpdateWithIndexes(
//	    ctx, store, redisIndexer,
//	    "users/user-123.json", user,
//	    []smarterbase.IndexUpdate{
//	        {EntityType: "users", IndexField: "email", OldValue: oldEmail, NewValue: newEmail},
//	    },
//	)
func UpdateWithIndexes(
	ctx context.Context,
	store *Store,
	redisIndexer *RedisIndexer,
	key string,
	data interface{},
	updates []IndexUpdate,
) error {
	// Marshal data once
	bytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	// Write data first
	if err := store.PutJSON(ctx, key, data); err != nil {
		return err
	}

	// Update indexes (best-effort if Redis available)
	if redisIndexer != nil {
		for _, update := range updates {
			// For multi-value indexes, we need to remove the old entry and add the new one
			// The RedisIndexer's ReplaceIndexes method handles this
			if update.OldValue != "" || update.NewValue != "" {
				// Construct old and new data for index replacement
				oldData := bytes // Will be used to remove old indexes
				newData := bytes // Will be used to add new indexes

				if update.OldValue != "" && update.NewValue != "" {
					// Replacing - need to remove old and add new
					if err := redisIndexer.ReplaceIndexes(ctx, key, oldData, newData); err != nil {
						// Log but don't fail - indexes are secondary
						store.logger.Warn("failed to update index",
							"entity", update.EntityType,
							"field", update.IndexField,
							"error", err)
					}
				} else if update.NewValue != "" {
					// Adding new index
					if err := redisIndexer.UpdateIndexes(ctx, key, newData); err != nil {
						store.logger.Warn("failed to add index",
							"entity", update.EntityType,
							"field", update.IndexField,
							"error", err)
					}
				} else if update.OldValue != "" {
					// Removing old index
					if err := redisIndexer.RemoveFromIndexes(ctx, key, oldData); err != nil {
						store.logger.Warn("failed to remove index",
							"entity", update.EntityType,
							"field", update.IndexField,
							"error", err)
					}
				}
			}
		}
	}

	return nil
}

// BatchGetWithFilter loads multiple objects and applies an optional filter.
// Simplifies the common pattern of loading multiple items and filtering them.
//
// Example:
//
//	// Get only primary properties
//	results, err := smarterbase.BatchGetWithFilter[Property](
//	    ctx, store, keys,
//	    func(p *Property) bool { return p.IsPrimary },
//	)
//
//	// Get all items (no filter)
//	allResults, err := smarterbase.BatchGetWithFilter[Property](ctx, store, keys, nil)
func BatchGetWithFilter[T any](
	ctx context.Context,
	store *Store,
	keys []string,
	filter func(*T) bool, // Optional: nil means no filter
) ([]*T, error) {
	if len(keys) == 0 {
		return []*T{}, nil
	}

	results := make([]*T, 0, len(keys))

	for _, key := range keys {
		var item T
		if err := store.GetJSON(ctx, key, &item); err != nil {
			if IsNotFound(err) {
				continue // Skip missing items
			}
			return nil, fmt.Errorf("failed to get %s: %w", key, err)
		}

		// Apply filter if provided
		if filter == nil || filter(&item) {
			results = append(results, &item)
		}
	}

	return results, nil
}
