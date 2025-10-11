package smarterbase

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// TestBatchPutJSON_Success verifies batch put with all successes
func TestBatchPutJSON_Success(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	items := map[string]interface{}{
		"batch/item1.json": map[string]string{"id": "1", "value": "first"},
		"batch/item2.json": map[string]string{"id": "2", "value": "second"},
		"batch/item3.json": map[string]string{"id": "3", "value": "third"},
	}

	results := store.BatchPutJSON(ctx, items)
	analysis := AnalyzeBatchResults(results)

	if analysis.Failed > 0 {
		t.Errorf("Expected 0 failures, got %d", analysis.Failed)
		for _, err := range analysis.Errors {
			t.Logf("  - %s: %v", err.Key, err.Error)
		}
	}

	if analysis.Successful != 3 {
		t.Errorf("Expected 3 successes, got %d", analysis.Successful)
	}

	// Verify all items were written
	for key := range items {
		exists, err := backend.Exists(ctx, key)
		if err != nil || !exists {
			t.Errorf("Item %s was not written", key)
		}
	}
}

// TestBatchPutJSON_MarshalError verifies handling of unmarshalable data
func TestBatchPutJSON_MarshalError(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	items := map[string]interface{}{
		"batch/good.json": map[string]string{"id": "1"},
		"batch/bad.json":  make(chan int), // Cannot marshal channels
	}

	results := store.BatchPutJSON(ctx, items)
	analysis := AnalyzeBatchResults(results)

	if analysis.Failed == 0 {
		t.Error("Expected at least one failure for unmarshalable data")
	}

	// Good item should still succeed
	if analysis.Successful == 0 {
		t.Error("Expected at least one success")
	}
}

// TestBatchGetJSON_MixedResults verifies batch get with some missing keys
func TestBatchGetJSON_MixedResults(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create only 2 of 3 items
	_ = store.PutJSON(ctx, "batch/exists1.json", map[string]string{"id": "1"})
	_ = store.PutJSON(ctx, "batch/exists2.json", map[string]string{"id": "2"})
	// batch/missing.json intentionally not created

	keys := []string{
		"batch/exists1.json",
		"batch/exists2.json",
		"batch/missing.json",
	}

	results, err := store.BatchGetJSON(ctx, keys, map[string]interface{}{})
	if err != nil {
		t.Fatalf("BatchGetJSON failed: %v", err)
	}

	// Should have 2 results (missing keys are skipped)
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	if _, exists := results["batch/exists1.json"]; !exists {
		t.Error("Expected exists1.json in results")
	}

	if _, exists := results["batch/exists2.json"]; !exists {
		t.Error("Expected exists2.json in results")
	}

	if _, exists := results["batch/missing.json"]; exists {
		t.Error("Did not expect missing.json in results")
	}
}

// TestBatchDelete_AllSucceed verifies batch delete
func TestBatchDelete_AllSucceed(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create test data
	keys := []string{
		"batch/delete1.json",
		"batch/delete2.json",
		"batch/delete3.json",
	}

	for _, key := range keys {
		_ = store.PutJSON(ctx, key, map[string]string{"test": "data"})
	}

	// Delete all
	results := store.BatchDelete(ctx, keys)
	analysis := AnalyzeBatchResults(results)

	if analysis.Failed > 0 {
		t.Errorf("Expected 0 failures, got %d", analysis.Failed)
	}

	// Verify all deleted
	for _, key := range keys {
		exists, _ := backend.Exists(ctx, key)
		if exists {
			t.Errorf("Item %s still exists after delete", key)
		}
	}
}

// TestBatchExists_AccurateResults verifies batch exists check
func TestBatchExists_AccurateResults(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create only some items
	store.PutJSON(ctx, "batch/exists.json", map[string]string{"id": "1"})
	// batch/missing.json not created

	keys := []string{
		"batch/exists.json",
		"batch/missing.json",
	}

	results := store.BatchExists(ctx, keys)

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	if !results["batch/exists.json"] {
		t.Error("Expected exists.json to exist")
	}

	if results["batch/missing.json"] {
		t.Error("Expected missing.json to not exist")
	}
}

// TestBatchWriter_AutoFlush verifies auto-flush behavior
func TestBatchWriter_AutoFlush(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	batchSize := 5
	writer := store.NewBatchWriter(batchSize)

	// Add exactly batchSize items - should auto-flush
	for i := 0; i < batchSize; i++ {
		key := fmt.Sprintf("batch/item%d.json", i)
		err := writer.Add(ctx, key, map[string]int{"id": i})
		if err != nil {
			t.Fatalf("Add failed: %v", err)
		}
	}

	// Verify all items were written
	for i := 0; i < batchSize; i++ {
		key := fmt.Sprintf("batch/item%d.json", i)
		exists, _ := backend.Exists(ctx, key)
		if !exists {
			t.Errorf("Item %s was not written after auto-flush", key)
		}
	}
}

// TestBatchWriter_ManualFlush verifies manual flush
func TestBatchWriter_ManualFlush(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	writer := store.NewBatchWriter(100) // Large batch size

	// Add 3 items (less than batch size)
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("batch/item%d.json", i)
		_ = writer.Add(ctx, key, map[string]int{"id": i})
	}

	// Items should NOT be written yet
	exists, _ := backend.Exists(ctx, "batch/item0.json")
	if exists {
		t.Error("Items were written before manual flush")
	}

	// Manual flush
	err := writer.Flush(ctx)
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Now items should exist
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("batch/item%d.json", i)
		exists, _ := backend.Exists(ctx, key)
		if !exists {
			t.Errorf("Item %s was not written after flush", key)
		}
	}
}

// TestBatchWriter_ErrorHandling verifies error propagation
func TestBatchWriter_ErrorHandling(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	writer := store.NewBatchWriter(2)

	// Add unmarshalable item
	err := writer.Add(ctx, "batch/bad.json", make(chan int))
	if err != nil {
		// Add may fail immediately or on flush
		return
	}

	// Add normal item to trigger flush
	err = writer.Add(ctx, "batch/good.json", map[string]string{"id": "1"})
	if err == nil {
		t.Error("Expected error from batch with unmarshalable data")
	}
}

// TestBatchOperations_Concurrent verifies thread safety
func TestBatchOperations_Concurrent(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	workers := 10
	itemsPerWorker := 100
	var wg sync.WaitGroup

	// Multiple goroutines writing batches
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			items := make(map[string]interface{})
			for i := 0; i < itemsPerWorker; i++ {
				key := fmt.Sprintf("concurrent/worker%d/item%d.json", workerID, i)
				items[key] = map[string]int{"worker": workerID, "item": i}
			}

			results := store.BatchPutJSON(ctx, items)
			analysis := AnalyzeBatchResults(results)
			if analysis.Failed > 0 {
				t.Errorf("Worker %d had %d failures", workerID, analysis.Failed)
			}
		}(w)
	}

	wg.Wait()

	// Verify all items exist
	totalExpected := workers * itemsPerWorker
	keys, err := backend.List(ctx, "concurrent/")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(keys) != totalExpected {
		t.Errorf("Expected %d items, found %d", totalExpected, len(keys))
	}
}

// Benchmark batch operations
func BenchmarkBatchPutJSON_100Items(b *testing.B) {
	ctx := context.Background()
	backend := NewFilesystemBackend(b.TempDir())
	store := NewStore(backend)

	items := make(map[string]interface{})
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("bench/item%d.json", i)
		items[key] = map[string]int{"id": i}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.BatchPutJSON(ctx, items)
	}
}

func BenchmarkBatchPutJSON_1000Items(b *testing.B) {
	ctx := context.Background()
	backend := NewFilesystemBackend(b.TempDir())
	store := NewStore(backend)

	items := make(map[string]interface{})
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("bench/item%d.json", i)
		items[key] = map[string]int{"id": i}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.BatchPutJSON(ctx, items)
	}
}
