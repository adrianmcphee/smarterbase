package smarterbase

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// BatchOperation represents a batch operation result
type BatchOperation struct {
	Key   string
	Error error
}

// BatchPutJSON stores multiple JSON objects in parallel for improved performance.
//
// This method executes all writes concurrently, making it significantly faster than
// sequential PutJSON calls when writing multiple objects.
//
// Basic usage:
//
//	items := map[string]interface{}{
//	    "users/1": &User{ID: "1", Email: "alice@example.com"},
//	    "users/2": &User{ID: "2", Email: "bob@example.com"},
//	    "users/3": &User{ID: "3", Email: "carol@example.com"},
//	}
//	results := store.BatchPutJSON(ctx, items)
//
// Error handling:
//
//	results := store.BatchPutJSON(ctx, items)
//	for _, result := range results {
//	    if result.Error != nil {
//	        log.Printf("Failed to write %s: %v", result.Key, result.Error)
//	    }
//	}
//
// Analyzing results:
//
//	results := store.BatchPutJSON(ctx, items)
//	analysis := smarterbase.AnalyzeBatchResults(results)
//	fmt.Printf("Success: %d/%d, Failed: %d\n",
//	    analysis.Successful, analysis.Total, analysis.Failed)
//
// Note: This is the older batch API. Consider using smarterbase.BatchGet[T]() from helpers.go
// for type-safe batch reads with automatic unmarshaling.
//
// Returns a slice of BatchOperation results (one per key), containing any errors that occurred.
func (s *Store) BatchPutJSON(ctx context.Context, items map[string]interface{}) []BatchOperation {
	results := make([]BatchOperation, 0, len(items))
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Process in parallel for better performance
	for key, value := range items {
		// Check for context cancellation before starting goroutine
		select {
		case <-ctx.Done():
			return []BatchOperation{{Key: key, Error: ctx.Err()}}
		default:
		}

		wg.Add(1)
		go func(k string, v interface{}) {
			defer wg.Done()

			// Check context in goroutine
			select {
			case <-ctx.Done():
				mu.Lock()
				results = append(results, BatchOperation{Key: k, Error: ctx.Err()})
				mu.Unlock()
				return
			default:
			}

			data, err := json.Marshal(v)
			if err != nil {
				mu.Lock()
				results = append(results, BatchOperation{Key: k, Error: fmt.Errorf("marshal error: %w", err)})
				mu.Unlock()
				return
			}

			err = s.backend.Put(ctx, k, data)
			mu.Lock()
			results = append(results, BatchOperation{Key: k, Error: err})
			mu.Unlock()
		}(key, value)
	}

	wg.Wait()
	return results
}

// BatchGetJSON retrieves multiple JSON objects in parallel.
//
// Important: This is the older batch API. Prefer using BatchGet[T]() from helpers.go
// for type-safe batch reads with automatic unmarshaling and better error handling:
//
//	users, err := smarterbase.BatchGet[User](ctx, store, keys)
//
// This method returns a map of successfully fetched objects. Failed retrievals are silently
// skipped - check the return map for missing keys to detect failures.
//
// Basic usage:
//
//	keys := []string{"users/1", "users/2", "users/3"}
//	results, err := store.BatchGetJSON(ctx, keys, nil)
//	for key, value := range results {
//	    // Process value
//	}
//
// Detecting missing keys:
//
//	results, err := store.BatchGetJSON(ctx, keys, nil)
//	for _, key := range keys {
//	    if _, found := results[key]; !found {
//	        log.Printf("Key %s was not found or failed to fetch", key)
//	    }
//	}
//
// Returns a map of key -> value for successful retrievals. Failed retrievals are omitted.
func (s *Store) BatchGetJSON(ctx context.Context, keys []string, destType interface{}) (map[string]interface{}, error) {
	results := make(map[string]interface{})
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, key := range keys {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		wg.Add(1)
		go func(k string) {
			defer wg.Done()

			// Check context in goroutine
			select {
			case <-ctx.Done():
				return
			default:
			}

			data, err := s.backend.Get(ctx, k)
			if err != nil {
				return // Skip failures
			}

			// Unmarshal into a new instance of the destination type
			var value interface{}
			if err := json.Unmarshal(data, &value); err != nil {
				return
			}

			mu.Lock()
			results[k] = value
			mu.Unlock()
		}(key)
	}

	wg.Wait()
	return results, nil
}

// BatchDelete deletes multiple objects in parallel for improved performance.
//
// This method executes all deletions concurrently, making it significantly faster than
// sequential Delete calls when removing multiple objects.
//
// Basic usage:
//
//	keys := []string{"users/1", "users/2", "users/3"}
//	results := store.BatchDelete(ctx, keys)
//
// Error handling:
//
//	results := store.BatchDelete(ctx, keys)
//	for _, result := range results {
//	    if result.Error != nil {
//	        log.Printf("Failed to delete %s: %v", result.Key, result.Error)
//	    }
//	}
//
// Analyzing results:
//
//	results := store.BatchDelete(ctx, keys)
//	analysis := smarterbase.AnalyzeBatchResults(results)
//	if analysis.Failed > 0 {
//	    log.Printf("Deletion failed for %d/%d keys", analysis.Failed, analysis.Total)
//	}
//
// Returns a slice of BatchOperation results (one per key), containing any errors that occurred.
func (s *Store) BatchDelete(ctx context.Context, keys []string) []BatchOperation {
	results := make([]BatchOperation, 0, len(keys))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, key := range keys {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return []BatchOperation{{Key: key, Error: ctx.Err()}}
		default:
		}

		wg.Add(1)
		go func(k string) {
			defer wg.Done()

			// Check context in goroutine
			select {
			case <-ctx.Done():
				mu.Lock()
				results = append(results, BatchOperation{Key: k, Error: ctx.Err()})
				mu.Unlock()
				return
			default:
			}

			err := s.backend.Delete(ctx, k)
			mu.Lock()
			results = append(results, BatchOperation{Key: k, Error: err})
			mu.Unlock()
		}(key)
	}

	wg.Wait()
	return results
}

// BatchExists checks if multiple keys exist in parallel.
//
// This method executes all existence checks concurrently, making it significantly faster
// than sequential Exists calls when checking many keys.
//
// Basic usage:
//
//	keys := []string{"users/1", "users/2", "users/3"}
//	results := store.BatchExists(ctx, keys)
//	for key, exists := range results {
//	    if exists {
//	        fmt.Printf("%s exists\n", key)
//	    }
//	}
//
// Filtering existing keys:
//
//	results := store.BatchExists(ctx, keys)
//	existingKeys := make([]string, 0)
//	for _, key := range keys {
//	    if results[key] {
//	        existingKeys = append(existingKeys, key)
//	    }
//	}
//
// Returns a map of key -> boolean indicating whether each key exists.
// If an error occurs checking a key, it's treated as not existing (false).
func (s *Store) BatchExists(ctx context.Context, keys []string) map[string]bool {
	results := make(map[string]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, key := range keys {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()

			exists, err := s.backend.Exists(ctx, k)
			if err != nil {
				exists = false
			}

			mu.Lock()
			results[k] = exists
			mu.Unlock()
		}(key)
	}

	wg.Wait()
	return results
}

// BatchOperationResult summarizes the results of a batch operation
type BatchOperationResult struct {
	Total      int
	Successful int
	Failed     int
	Errors     []BatchOperation
}

// AnalyzeBatchResults analyzes batch operation results
func AnalyzeBatchResults(operations []BatchOperation) *BatchOperationResult {
	result := &BatchOperationResult{
		Total:  len(operations),
		Errors: make([]BatchOperation, 0),
	}

	for _, op := range operations {
		if op.Error == nil {
			result.Successful++
		} else {
			result.Failed++
			result.Errors = append(result.Errors, op)
		}
	}

	return result
}

// BatchWriter provides a convenient interface for batching writes with automatic flushing.
//
// Use BatchWriter when you have a stream of writes and want to automatically batch them
// for better performance. The writer automatically flushes when the batch size is reached.
//
// Basic usage:
//
//	writer := store.NewBatchWriter(100)  // Flush every 100 items
//
//	for _, user := range users {
//	    key := fmt.Sprintf("users/%s", user.ID)
//	    if err := writer.Add(ctx, key, user); err != nil {
//	        log.Printf("Batch write failed: %v", err)
//	        break
//	    }
//	}
//
//	// Flush remaining items
//	if err := writer.Flush(ctx); err != nil {
//	    log.Printf("Final flush failed: %v", err)
//	}
//
// With progress tracking:
//
//	writer := store.NewBatchWriter(100)
//	for i, user := range users {
//	    key := fmt.Sprintf("users/%s", user.ID)
//	    if err := writer.Add(ctx, key, user); err != nil {
//	        return fmt.Errorf("failed at user %d: %w", i, err)
//	    }
//	    if (i+1) % 1000 == 0 {
//	        log.Printf("Processed %d/%d users", i+1, len(users))
//	    }
//	}
//	return writer.Flush(ctx)
type BatchWriter struct {
	store     *Store
	items     map[string]interface{}
	batchSize int
	mu        sync.Mutex
}

// NewBatchWriter creates a new batch writer
func (s *Store) NewBatchWriter(batchSize int) *BatchWriter {
	return &BatchWriter{
		store:     s,
		items:     make(map[string]interface{}),
		batchSize: batchSize,
	}
}

// Add adds an item to the batch and automatically flushes when the batch size is reached.
//
// Returns an error if the automatic flush fails. On error, the batch is cleared and
// you should handle the error appropriately (retry, log, abort, etc.).
//
// Example:
//
//	for _, item := range items {
//	    if err := writer.Add(ctx, "items/"+item.ID, item); err != nil {
//	        return fmt.Errorf("batch write failed: %w", err)
//	    }
//	}
func (bw *BatchWriter) Add(ctx context.Context, key string, value interface{}) error {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	bw.items[key] = value

	if len(bw.items) >= bw.batchSize {
		return bw.flushLocked(ctx)
	}

	return nil
}

// Flush writes all pending items in the batch.
//
// You must call Flush at the end of your batch writing to ensure all items are written.
// Returns an error if any writes fail.
//
// Example:
//
//	defer func() {
//	    if err := writer.Flush(ctx); err != nil {
//	        log.Printf("Final flush failed: %v", err)
//	    }
//	}()
func (bw *BatchWriter) Flush(ctx context.Context) error {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	return bw.flushLocked(ctx)
}

func (bw *BatchWriter) flushLocked(ctx context.Context) error {
	if len(bw.items) == 0 {
		return nil
	}

	results := bw.store.BatchPutJSON(ctx, bw.items)
	analysis := AnalyzeBatchResults(results)

	// Clear the batch
	bw.items = make(map[string]interface{})

	if analysis.Failed > 0 {
		return fmt.Errorf("batch write failed: %d/%d operations failed", analysis.Failed, analysis.Total)
	}

	return nil
}
