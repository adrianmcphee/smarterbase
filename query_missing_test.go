package smarterbase

import (
	"context"
	"encoding/json"
	"testing"
)

// TestQuery_FilterRawBytes tests the raw byte filter that was missing coverage
func TestQuery_FilterRawBytes(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()
	store := NewStore(backend)

	// Create test data
	for i := 0; i < 5; i++ {
		item := map[string]interface{}{
			"id":   i,
			"name": "item-" + string(rune(i+'0')),
			"size": i * 100,
		}
		store.PutJSON(ctx, "items/"+string(rune(i+'0'))+".json", item)
	}

	// Filter with raw bytes - check if JSON contains "size":200
	var results []map[string]interface{}
	err := store.Query("items/").
		Filter(func(data []byte) bool {
			// Simple check: does the JSON contain the string "size":200
			dataStr := string(data)
			return len(dataStr) > 0 && contains(dataStr, "\"size\":200")
		}).
		All(ctx, &results)

	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	// Should find exactly 1 item with size 200 (id=2)
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	if len(results) > 0 {
		size, _ := results[0]["size"].(float64)
		if size != 200 {
			t.Errorf("expected size 200, got %v", size)
		}
	}
}

// TestQuery_CustomSortFunction tests the custom sort that was missing coverage
func TestQuery_CustomSortFunction(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()
	store := NewStore(backend)

	// Create test data in random order
	items := []map[string]interface{}{
		{"id": 3, "priority": 1},
		{"id": 1, "priority": 3},
		{"id": 4, "priority": 2},
		{"id": 2, "priority": 4},
		{"id": 0, "priority": 5},
	}

	for i, item := range items {
		store.PutJSON(ctx, "items/"+string(rune(i+'0'))+".json", item)
	}

	// Sort by priority (ascending) using custom sort function
	var results []map[string]interface{}
	err := store.Query("items/").
		Sort(func(a, b []byte) bool {
			var objA, objB map[string]interface{}
			json.Unmarshal(a, &objA)
			json.Unmarshal(b, &objB)
			prioA, _ := objA["priority"].(float64)
			prioB, _ := objB["priority"].(float64)
			return prioA < prioB
		}).
		All(ctx, &results)

	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	// Verify sorted by priority: should be 1, 2, 3, 4, 5
	for i := 0; i < len(results)-1; i++ {
		prioThis, _ := results[i]["priority"].(float64)
		prioNext, _ := results[i+1]["priority"].(float64)
		if prioThis > prioNext {
			t.Errorf("results not sorted: priority %v > %v", prioThis, prioNext)
		}
	}

	// First item should have priority 1
	firstPrio, _ := results[0]["priority"].(float64)
	if firstPrio != 1 {
		t.Errorf("first item should have priority 1, got %v", firstPrio)
	}
}

// Helper
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if i+len(substr) <= len(s) && s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
