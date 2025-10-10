package smarterbase

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// TestQuery_Filter tests basic filtering
func TestQuery_Filter(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create test data
	for i := 1; i <= 5; i++ {
		key := fmt.Sprintf("query/item%d.json", i)
		store.PutJSON(ctx, key, map[string]int{"id": i, "value": i * 10})
	}

	// Filter for items with value > 30
	var results []map[string]int
	err := store.Query("query/").
		FilterJSON(func(obj map[string]interface{}) bool {
			val, ok := obj["value"].(float64) // JSON numbers are float64
			return ok && val > 30
		}).
		All(ctx, &results)

	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 2 { // items 4 and 5 (40, 50)
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

// TestQuery_Limit tests result limiting
func TestQuery_Limit(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create 10 items
	for i := 1; i <= 10; i++ {
		key := fmt.Sprintf("query/item%d.json", i)
		store.PutJSON(ctx, key, map[string]int{"id": i})
	}

	// Query with limit
	var results []map[string]int
	err := store.Query("query/").
		Limit(3).
		All(ctx, &results)

	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}
}

// TestQuery_Offset tests result offset
func TestQuery_Offset(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create 10 items
	for i := 1; i <= 10; i++ {
		key := fmt.Sprintf("query/item%02d.json", i) // Zero-padded for consistent ordering
		store.PutJSON(ctx, key, map[string]int{"id": i})
	}

	// Query with offset
	var results []map[string]int
	err := store.Query("query/").
		Offset(5).
		All(ctx, &results)

	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 5 {
		t.Errorf("Expected 5 results (10 total - 5 offset), got %d", len(results))
	}
}

// TestQuery_LimitAndOffset tests pagination
func TestQuery_LimitAndOffset(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create 20 items
	for i := 1; i <= 20; i++ {
		key := fmt.Sprintf("query/item%02d.json", i)
		store.PutJSON(ctx, key, map[string]int{"id": i})
	}

	// Get page 2 (items 6-10)
	var results []map[string]int
	err := store.Query("query/").
		Offset(5).
		Limit(5).
		All(ctx, &results)

	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 5 {
		t.Errorf("Expected 5 results (page 2), got %d", len(results))
	}
}

// TestQuery_SortByField_String tests string sorting
func TestQuery_SortByField_String(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create items in random order
	names := []string{"Charlie", "Alice", "Bob"}
	for i, name := range names {
		key := fmt.Sprintf("query/item%d.json", i)
		store.PutJSON(ctx, key, map[string]string{"name": name})
	}

	// Query with sort
	var results []map[string]string
	err := store.Query("query/").
		SortByField("name", true). // Ascending
		All(ctx, &results)

	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	// Verify sort order
	expected := []string{"Alice", "Bob", "Charlie"}
	for i, exp := range expected {
		if results[i]["name"] != exp {
			t.Errorf("Position %d: expected %s, got %s", i, exp, results[i]["name"])
		}
	}
}

// TestQuery_SortByField_Number tests numeric sorting
func TestQuery_SortByField_Number(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create items in random order
	values := []int{30, 10, 20}
	for i, val := range values {
		key := fmt.Sprintf("query/item%d.json", i)
		store.PutJSON(ctx, key, map[string]int{"value": val})
	}

	// Query with descending sort
	var results []map[string]interface{}
	err := store.Query("query/").
		SortByField("value", false). // Descending
		All(ctx, &results)

	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	// Verify descending order: 30, 20, 10
	if results[0]["value"].(float64) != 30 {
		t.Errorf("Expected first value to be 30, got %v", results[0]["value"])
	}
	if results[2]["value"].(float64) != 10 {
		t.Errorf("Expected last value to be 10, got %v", results[2]["value"])
	}
}

// TestQuery_All_Empty tests query with no results
func TestQuery_All_Empty(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Query empty prefix
	var results []map[string]int
	err := store.Query("nonexistent/").
		All(ctx, &results)

	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

// TestQuery_First tests finding first match
func TestQuery_First(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create test data
	for i := 1; i <= 5; i++ {
		key := fmt.Sprintf("query/item%d.json", i)
		store.PutJSON(ctx, key, map[string]int{"id": i, "active": i % 2})
	}

	// Find first active item (id=1 or id=3 or id=5)
	var result map[string]int
	err := store.Query("query/").
		FilterJSON(func(obj map[string]interface{}) bool {
			active, ok := obj["active"].(float64)
			return ok && active == 1
		}).
		First(ctx, &result)

	if err != nil {
		t.Fatalf("First failed: %v", err)
	}

	if result["active"] != 1 {
		t.Errorf("Expected active=1, got %d", result["active"])
	}
}

// TestQuery_First_NoMatch tests First with no results
func TestQuery_First_NoMatch(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create test data
	store.PutJSON(ctx, "query/item1.json", map[string]string{"status": "inactive"})

	// Try to find active item (doesn't exist)
	var result map[string]string
	err := store.Query("query/").
		FilterJSON(func(obj map[string]interface{}) bool {
			status, ok := obj["status"].(string)
			return ok && status == "active"
		}).
		First(ctx, &result)

	if err == nil {
		t.Error("Expected error when no match found")
	}
}

// TestQuery_Count tests counting results
func TestQuery_Count(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create 10 items, 5 active, 5 inactive
	for i := 1; i <= 10; i++ {
		key := fmt.Sprintf("query/item%d.json", i)
		store.PutJSON(ctx, key, map[string]bool{"active": i <= 5})
	}

	// Count active items
	count, err := store.Query("query/").
		FilterJSON(func(obj map[string]interface{}) bool {
			active, ok := obj["active"].(bool)
			return ok && active
		}).
		Count(ctx)

	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}

	if count != 5 {
		t.Errorf("Expected count=5, got %d", count)
	}
}

// TestQuery_Each tests iteration
func TestQuery_Each(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create test data
	for i := 1; i <= 5; i++ {
		key := fmt.Sprintf("query/item%d.json", i)
		store.PutJSON(ctx, key, map[string]int{"id": i})
	}

	// Iterate and collect IDs
	var ids []int
	err := store.Query("query/").Each(ctx, func(key string, data []byte) error {
		var obj map[string]interface{}
		if err := unmarshalJSON(data, &obj); err != nil {
			return err
		}
		ids = append(ids, int(obj["id"].(float64)))
		return nil
	})

	if err != nil {
		t.Fatalf("Each failed: %v", err)
	}

	if len(ids) != 5 {
		t.Errorf("Expected 5 IDs, got %d", len(ids))
	}
}

// Helper for JSON unmarshaling in tests
func unmarshalJSON(data []byte, v interface{}) error {
	return jsonUnmarshal(data, v)
}

var jsonUnmarshal = json.Unmarshal // For easier testing

// TestQuery_Each_EarlyExit tests stopping iteration early
func TestQuery_Each_EarlyExit(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create 10 items
	for i := 1; i <= 10; i++ {
		key := fmt.Sprintf("query/item%d.json", i)
		store.PutJSON(ctx, key, map[string]int{"id": i})
	}

	// Iterate but stop after 3
	count := 0
	err := store.Query("query/").Each(ctx, func(key string, data []byte) error {
		count++
		if count >= 3 {
			return fmt.Errorf("stop after 3")
		}
		return nil
	})

	// Error is expected (early exit)
	if err == nil {
		t.Error("Expected error from early exit")
	}

	if count != 3 {
		t.Errorf("Expected count=3, got %d", count)
	}
}

// TestQueryBuilder_CreatedAfter tests time-based filtering
func TestQueryBuilder_CreatedAfter(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	qb := NewQueryBuilder(store)

	now := time.Now()
	past := now.Add(-24 * time.Hour)
	future := now.Add(24 * time.Hour)

	// Create items with different timestamps
	store.PutJSON(ctx, "query/old.json", map[string]string{
		"id":         "old",
		"created_at": past.Format(time.RFC3339),
	})
	store.PutJSON(ctx, "query/new.json", map[string]string{
		"id":         "new",
		"created_at": future.Format(time.RFC3339),
	})

	// Query for items created after now
	var results []map[string]string
	err := qb.CreatedAfter("query/", now).All(ctx, &results)

	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	if len(results) > 0 && results[0]["id"] != "new" {
		t.Errorf("Expected 'new' item, got %s", results[0]["id"])
	}
}

// TestQueryBuilder_FieldEquals tests exact match filtering
func TestQueryBuilder_FieldEquals(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	qb := NewQueryBuilder(store)

	// Create test data
	store.PutJSON(ctx, "query/item1.json", map[string]string{"status": "active"})
	store.PutJSON(ctx, "query/item2.json", map[string]string{"status": "inactive"})
	store.PutJSON(ctx, "query/item3.json", map[string]string{"status": "active"})

	// Query for active items
	var results []map[string]string
	err := qb.FieldEquals("query/", "status", "active").All(ctx, &results)

	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 active items, got %d", len(results))
	}
}

// TestQueryBuilder_FieldContains tests substring matching
func TestQueryBuilder_FieldContains(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	qb := NewQueryBuilder(store)

	// Create test data
	store.PutJSON(ctx, "query/item1.json", map[string]string{"description": "Hello world"})
	store.PutJSON(ctx, "query/item2.json", map[string]string{"description": "Goodbye moon"})
	store.PutJSON(ctx, "query/item3.json", map[string]string{"description": "Hello universe"})

	// Query for items containing "Hello"
	var results []map[string]string
	err := qb.FieldContains("query/", "description", "Hello").All(ctx, &results)

	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 items containing 'Hello', got %d", len(results))
	}
}

// TestQuery_LargeDataset tests query performance with many objects
func TestQuery_LargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large dataset test in short mode")
	}

	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create 1000 objects
	for i := 1; i <= 1000; i++ {
		key := fmt.Sprintf("large/item%04d.json", i)
		store.PutJSON(ctx, key, map[string]int{"id": i, "active": i % 2})
	}

	start := time.Now()

	// Query for active items (500 results)
	var results []map[string]interface{}
	err := store.Query("large/").
		FilterJSON(func(obj map[string]interface{}) bool {
			active, ok := obj["active"].(float64)
			return ok && active == 1
		}).
		All(ctx, &results)

	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 500 {
		t.Errorf("Expected 500 results, got %d", len(results))
	}

	t.Logf("Query 1000 objects (filter 500) took %v", elapsed)

	// Should complete in reasonable time (< 5 seconds)
	if elapsed > 5*time.Second {
		t.Errorf("Query took too long: %v (expected < 5s)", elapsed)
	}
}

// Benchmark query operations
func BenchmarkQuery_Filter_1000Objects(b *testing.B) {
	ctx := context.Background()
	backend := NewFilesystemBackend(b.TempDir())
	store := NewStore(backend)

	// Setup: create 1000 objects
	for i := 1; i <= 1000; i++ {
		key := fmt.Sprintf("bench/item%04d.json", i)
		store.PutJSON(ctx, key, map[string]int{"id": i, "active": i % 2})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var results []map[string]interface{}
		store.Query("bench/").
			FilterJSON(func(obj map[string]interface{}) bool {
				active, ok := obj["active"].(float64)
				return ok && active == 1
			}).
			All(ctx, &results)
	}
}

func BenchmarkQuery_SortByField_1000Objects(b *testing.B) {
	ctx := context.Background()
	backend := NewFilesystemBackend(b.TempDir())
	store := NewStore(backend)

	// Setup
	for i := 1; i <= 1000; i++ {
		key := fmt.Sprintf("bench/item%04d.json", i)
		store.PutJSON(ctx, key, map[string]int{"id": i, "value": 1000 - i})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var results []map[string]interface{}
		store.Query("bench/").
			SortByField("value", true).
			All(ctx, &results)
	}
}
