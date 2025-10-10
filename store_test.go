package smarterbase

import (
	"context"
	"fmt"
	"testing"
)

func TestStore_BasicOperations(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	t.Run("GetJSON_PutJSON", func(t *testing.T) {
		type TestData struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		}

		key := "test/data.json"
		original := TestData{Name: "test", Value: 42}

		// Put
		err := store.PutJSON(ctx, key, original)
		if err != nil {
			t.Fatalf("PutJSON failed: %v", err)
		}

		// Get
		var retrieved TestData
		err = store.GetJSON(ctx, key, &retrieved)
		if err != nil {
			t.Fatalf("GetJSON failed: %v", err)
		}

		if retrieved.Name != original.Name || retrieved.Value != original.Value {
			t.Errorf("Data mismatch: got %+v, want %+v", retrieved, original)
		}
	})

	t.Run("GetJSONWithETag", func(t *testing.T) {
		key := "test/etag.json"
		data := map[string]string{"version": "1"}

		err := store.PutJSON(ctx, key, data)
		if err != nil {
			t.Fatalf("PutJSON failed: %v", err)
		}

		var retrieved map[string]string
		etag, err := store.GetJSONWithETag(ctx, key, &retrieved)
		if err != nil {
			t.Fatalf("GetJSONWithETag failed: %v", err)
		}

		if etag == "" {
			t.Error("Expected non-empty ETag")
		}

		if retrieved["version"] != "1" {
			t.Errorf("Data mismatch: got %v", retrieved)
		}
	})

	t.Run("PutJSONWithETag", func(t *testing.T) {
		key := "test/etag-update.json"
		data1 := map[string]int{"count": 1}
		data2 := map[string]int{"count": 2}

		// Initial write
		etag1, err := store.PutJSONWithETag(ctx, key, data1, "")
		if err != nil {
			t.Fatalf("Initial PutJSONWithETag failed: %v", err)
		}

		// Update with correct ETag
		etag2, err := store.PutJSONWithETag(ctx, key, data2, etag1)
		if err != nil {
			t.Fatalf("Update with correct ETag failed: %v", err)
		}

		if etag1 == etag2 {
			t.Error("Expected ETag to change after update")
		}

		// Update with wrong ETag should fail
		_, err = store.PutJSONWithETag(ctx, key, data1, "wrong-etag")
		if err == nil {
			t.Error("Expected error with wrong ETag")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		key := "test/delete.json"
		data := map[string]bool{"exists": true}

		err := store.PutJSON(ctx, key, data)
		if err != nil {
			t.Fatalf("PutJSON failed: %v", err)
		}

		err = store.Delete(ctx, key)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		exists, err := store.Exists(ctx, key)
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}

		if exists {
			t.Error("Expected key to not exist after delete")
		}
	})

	t.Run("List", func(t *testing.T) {
		// Create test data
		for i := 1; i <= 3; i++ {
			key := fmt.Sprintf("list-test/item%d.json", i)
			data := map[string]int{"id": i}
			if err := store.PutJSON(ctx, key, data); err != nil {
				t.Fatalf("Failed to create test data: %v", err)
			}
		}

		keys, err := store.List(ctx, "list-test/")
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}

		if len(keys) != 3 {
			t.Errorf("Expected 3 keys, got %d", len(keys))
		}
	})
}

func TestStore_IndexOperations(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	t.Run("GetIndex_PutIndex", func(t *testing.T) {
		indexKey := "indexes/test-index.json"

		// Create index
		idx := &Index{
			Key: indexKey,
			Entries: map[string]string{
				"item1": "parent1",
				"item2": "parent1",
				"item3": "parent2",
			},
		}

		err := store.PutIndex(ctx, idx)
		if err != nil {
			t.Fatalf("PutIndex failed: %v", err)
		}

		// Retrieve index
		retrieved, err := store.GetIndex(ctx, indexKey)
		if err != nil {
			t.Fatalf("GetIndex failed: %v", err)
		}

		if len(retrieved.Entries) != 3 {
			t.Errorf("Expected 3 entries, got %d", len(retrieved.Entries))
		}

		if retrieved.Entries["item1"] != "parent1" {
			t.Error("Index entry mismatch")
		}
	})

	t.Run("UpdateIndex_Atomic", func(t *testing.T) {
		indexKey := "indexes/atomic-test.json"

		// Create initial index
		err := store.UpdateIndex(ctx, indexKey, "item1", "parent1")
		if err != nil {
			t.Fatalf("Initial UpdateIndex failed: %v", err)
		}

		// Update existing index
		err = store.UpdateIndex(ctx, indexKey, "item2", "parent2")
		if err != nil {
			t.Fatalf("Second UpdateIndex failed: %v", err)
		}

		// Verify both entries exist
		idx, err := store.GetIndex(ctx, indexKey)
		if err != nil {
			t.Fatalf("GetIndex failed: %v", err)
		}

		if len(idx.Entries) != 2 {
			t.Errorf("Expected 2 entries, got %d", len(idx.Entries))
		}

		if idx.Entries["item1"] != "parent1" || idx.Entries["item2"] != "parent2" {
			t.Error("Index entries mismatch")
		}
	})

	t.Run("RemoveFromIndex", func(t *testing.T) {
		indexKey := "indexes/remove-test.json"

		// Create index with 2 entries
		idx := &Index{
			Key: indexKey,
			Entries: map[string]string{
				"item1": "parent1",
				"item2": "parent2",
			},
		}
		store.PutIndex(ctx, idx)

		// Remove one entry
		err := store.RemoveFromIndex(ctx, indexKey, "item1")
		if err != nil {
			t.Fatalf("RemoveFromIndex failed: %v", err)
		}

		// Verify only item2 remains
		updated, err := store.GetIndex(ctx, indexKey)
		if err != nil {
			t.Fatalf("GetIndex failed: %v", err)
		}

		if len(updated.Entries) != 1 {
			t.Errorf("Expected 1 entry, got %d", len(updated.Entries))
		}

		if _, exists := updated.Entries["item1"]; exists {
			t.Error("item1 should be removed")
		}

		if updated.Entries["item2"] != "parent2" {
			t.Error("item2 should still exist")
		}
	})

	// Concurrent updates are handled by retry logic in UpdateIndex
	// Basic concurrency is tested in TestFilesystemBackend_Specific/ConcurrentAccess
}

func TestStore_Ping(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	err := store.Ping(ctx)
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

// Benchmark tests
func BenchmarkStore_PutJSON(b *testing.B) {
	ctx := context.Background()
	backend := NewFilesystemBackend(b.TempDir())
	store := NewStore(backend)

	data := map[string]string{"test": "value"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench/item%d.json", i)
		store.PutJSON(ctx, key, data)
	}
}

func BenchmarkStore_GetJSON(b *testing.B) {
	ctx := context.Background()
	backend := NewFilesystemBackend(b.TempDir())
	store := NewStore(backend)

	// Setup
	key := "bench/item.json"
	data := map[string]string{"test": "value"}
	store.PutJSON(ctx, key, data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result map[string]string
		store.GetJSON(ctx, key, &result)
	}
}

func BenchmarkStore_UpdateIndex(b *testing.B) {
	ctx := context.Background()
	backend := NewFilesystemBackend(b.TempDir())
	store := NewStore(backend)

	indexKey := "bench/index.json"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		itemID := fmt.Sprintf("item%d", i)
		store.UpdateIndex(ctx, indexKey, itemID, "parent")
	}
}
