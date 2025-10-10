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

// BatchPutJSON stores multiple JSON objects in a single operation
// Returns a slice of errors (one per key), or nil if all succeeded
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

// BatchGetJSON retrieves multiple JSON objects in a single operation
// Returns a map of key -> value for successful retrievals
// Failures are silently skipped (check return map for missing keys)
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

// BatchDelete deletes multiple objects in a single operation
// Returns a slice of errors (one per key), or nil if all succeeded
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

// BatchExists checks if multiple keys exist
// Returns a map of key -> exists status
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

// Analyze analyzes batch operation results
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

// BatchWriter provides a convenient interface for batching writes
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

// Add adds an item to the batch
// Automatically flushes when batch size is reached
func (bw *BatchWriter) Add(ctx context.Context, key string, value interface{}) error {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	bw.items[key] = value

	if len(bw.items) >= bw.batchSize {
		return bw.flushLocked(ctx)
	}

	return nil
}

// Flush writes all pending items
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
