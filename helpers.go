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
