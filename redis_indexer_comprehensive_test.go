package smarterbase

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestRedisIndexer_BasicOperations tests basic indexing operations
func TestRedisIndexer_BasicOperations(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	indexer := NewRedisIndexer(redisClient)

	// Register a simple index
	indexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "users-by-email",
		EntityType:  "users",
		ExtractFunc: ExtractJSONField("email"),
	})

	ctx := context.Background()

	// Create user data
	user := map[string]interface{}{
		"id":    "user-123",
		"email": "alice@example.com",
		"name":  "Alice",
	}
	userData, _ := json.Marshal(user)

	// Update indexes
	err := indexer.UpdateIndexes(ctx, "users/user-123", userData)
	if err != nil {
		t.Fatalf("update indexes failed: %v", err)
	}

	// Query by email
	keys, err := indexer.Query(ctx, "users", "email", "alice@example.com")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	if keys[0] != "users/user-123" {
		t.Errorf("expected key 'users/user-123', got '%s'", keys[0])
	}
}

// TestRedisIndexer_MultiValueIndex tests multiple objects with same index value
func TestRedisIndexer_MultiValueIndex(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	indexer := NewRedisIndexer(redisClient)

	// Register index for user_id
	indexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "sessions-by-user",
		EntityType:  "sessions",
		ExtractFunc: ExtractJSONField("user_id"),
	})

	ctx := context.Background()

	// Create multiple sessions for same user
	for i := 1; i <= 3; i++ {
		session := map[string]interface{}{
			"id":      fmt.Sprintf("session-%d", i),
			"user_id": "user-123",
		}
		sessionData, _ := json.Marshal(session)
		err := indexer.UpdateIndexes(ctx, fmt.Sprintf("sessions/session-%d", i), sessionData)
		if err != nil {
			t.Fatalf("update indexes failed for session %d: %v", i, err)
		}
	}

	// Query sessions by user_id
	keys, err := indexer.Query(ctx, "sessions", "user_id", "user-123")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
}

// TestRedisIndexer_RemoveFromIndexes tests removing objects from indexes
func TestRedisIndexer_RemoveFromIndexes(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	indexer := NewRedisIndexer(redisClient)

	indexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "users-by-email",
		EntityType:  "users",
		ExtractFunc: ExtractJSONField("email"),
	})

	ctx := context.Background()

	// Add user
	user := map[string]interface{}{
		"id":    "user-123",
		"email": "alice@example.com",
	}
	userData, _ := json.Marshal(user)
	indexer.UpdateIndexes(ctx, "users/user-123", userData)

	// Verify user is indexed
	keys, _ := indexer.Query(ctx, "users", "email", "alice@example.com")
	if len(keys) != 1 {
		t.Fatal("user should be indexed")
	}

	// Remove from indexes
	err := indexer.RemoveFromIndexes(ctx, "users/user-123", userData)
	if err != nil {
		t.Fatalf("remove from indexes failed: %v", err)
	}

	// Verify user is no longer indexed
	keys, _ = indexer.Query(ctx, "users", "email", "alice@example.com")
	if len(keys) != 0 {
		t.Errorf("expected 0 keys after removal, got %d", len(keys))
	}
}

// TestRedisIndexer_ReplaceIndexes tests updating an object with different index values
func TestRedisIndexer_ReplaceIndexes(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	indexer := NewRedisIndexer(redisClient)

	indexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "users-by-email",
		EntityType:  "users",
		ExtractFunc: ExtractJSONField("email"),
	})

	ctx := context.Background()

	// Initial user
	oldUser := map[string]interface{}{
		"id":    "user-123",
		"email": "alice@old.com",
	}
	oldData, _ := json.Marshal(oldUser)
	indexer.UpdateIndexes(ctx, "users/user-123", oldData)

	// Updated user with new email
	newUser := map[string]interface{}{
		"id":    "user-123",
		"email": "alice@new.com",
	}
	newData, _ := json.Marshal(newUser)

	// Replace indexes
	err := indexer.ReplaceIndexes(ctx, "users/user-123", oldData, newData)
	if err != nil {
		t.Fatalf("replace indexes failed: %v", err)
	}

	// Old email should have no results
	keys, _ := indexer.Query(ctx, "users", "email", "alice@old.com")
	if len(keys) != 0 {
		t.Error("old email should not be indexed")
	}

	// New email should return the user
	keys, _ = indexer.Query(ctx, "users", "email", "alice@new.com")
	if len(keys) != 1 {
		t.Error("new email should be indexed")
	}
}

// TestRedisIndexer_QueryMultiple tests OR queries
func TestRedisIndexer_QueryMultiple(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	indexer := NewRedisIndexer(redisClient)

	indexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "orders-by-status",
		EntityType:  "orders",
		ExtractFunc: ExtractJSONField("status"),
	})

	ctx := context.Background()

	// Create orders with different statuses
	statuses := []string{"pending", "processing", "completed"}
	for i, status := range statuses {
		order := map[string]interface{}{
			"id":     fmt.Sprintf("order-%d", i),
			"status": status,
		}
		orderData, _ := json.Marshal(order)
		indexer.UpdateIndexes(ctx, fmt.Sprintf("orders/order-%d", i), orderData)
	}

	// Query for pending OR processing orders
	keys, err := indexer.QueryMultiple(ctx, "orders", "status", []string{"pending", "processing"})
	if err != nil {
		t.Fatalf("query multiple failed: %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
}

// TestRedisIndexer_Count tests counting objects in an index
func TestRedisIndexer_Count(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	indexer := NewRedisIndexer(redisClient)

	indexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "orders-by-status",
		EntityType:  "orders",
		ExtractFunc: ExtractJSONField("status"),
	})

	ctx := context.Background()

	// Create 5 pending orders
	for i := 0; i < 5; i++ {
		order := map[string]interface{}{
			"id":     fmt.Sprintf("order-%d", i),
			"status": "pending",
		}
		orderData, _ := json.Marshal(order)
		indexer.UpdateIndexes(ctx, fmt.Sprintf("orders/order-%d", i), orderData)
	}

	// Count pending orders
	count, err := indexer.Count(ctx, "orders", "status", "pending")
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}

	if count != 5 {
		t.Errorf("expected count 5, got %d", count)
	}
}

// TestRedisIndexer_GetIndexStats tests getting statistics for multiple index values
func TestRedisIndexer_GetIndexStats(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	indexer := NewRedisIndexer(redisClient)

	indexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "orders-by-status",
		EntityType:  "orders",
		ExtractFunc: ExtractJSONField("status"),
	})

	ctx := context.Background()

	// Create orders with different statuses
	statusCounts := map[string]int{
		"pending":    3,
		"processing": 2,
		"completed":  5,
	}

	orderNum := 0
	for status, count := range statusCounts {
		for i := 0; i < count; i++ {
			order := map[string]interface{}{
				"id":     fmt.Sprintf("order-%d", orderNum),
				"status": status,
			}
			orderData, _ := json.Marshal(order)
			indexer.UpdateIndexes(ctx, fmt.Sprintf("orders/order-%d", orderNum), orderData)
			orderNum++
		}
	}

	// Get stats
	stats, err := indexer.GetIndexStats(ctx, "orders", "status", []string{"pending", "processing", "completed"})
	if err != nil {
		t.Fatalf("get index stats failed: %v", err)
	}

	for status, expectedCount := range statusCounts {
		actualCount := stats[status]
		if actualCount != int64(expectedCount) {
			t.Errorf("status %s: expected count %d, got %d", status, expectedCount, actualCount)
		}
	}
}

// TestRedisIndexer_ExtractNestedJSONField tests extracting nested fields
func TestRedisIndexer_ExtractNestedJSONField(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	indexer := NewRedisIndexer(redisClient)

	indexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "photos-by-postcode",
		EntityType:  "photos",
		ExtractFunc: ExtractNestedJSONField("gallery", "postcode"),
	})

	ctx := context.Background()

	// Create photo with nested structure
	photo := map[string]interface{}{
		"id": "photo-123",
		"gallery": map[string]interface{}{
			"postcode": "1234AB",
			"name":     "Summer Gallery",
		},
	}
	photoData, _ := json.Marshal(photo)

	err := indexer.UpdateIndexes(ctx, "photos/photo-123", photoData)
	if err != nil {
		t.Fatalf("update indexes failed: %v", err)
	}

	// Query by nested field
	keys, err := indexer.Query(ctx, "photos", "postcode", "1234AB")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}
}

// TestRedisIndexer_RebuildIndex tests rebuilding an index from scratch
func TestRedisIndexer_RebuildIndex(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	indexer := NewRedisIndexer(redisClient)

	spec := &MultiIndexSpec{
		Name:        "users-by-email",
		EntityType:  "users",
		ExtractFunc: ExtractJSONField("email"),
	}
	indexer.RegisterMultiIndex(spec)

	ctx := context.Background()

	// Prepare objects for rebuild
	objects := make(map[string][]byte)
	for i := 1; i <= 3; i++ {
		user := map[string]interface{}{
			"id":    fmt.Sprintf("user-%d", i),
			"email": fmt.Sprintf("user%d@example.com", i),
		}
		userData, _ := json.Marshal(user)
		objects[fmt.Sprintf("users/user-%d", i)] = userData
	}

	// Rebuild index
	err := indexer.RebuildIndex(ctx, spec, objects)
	if err != nil {
		t.Fatalf("rebuild index failed: %v", err)
	}

	// Verify all users are indexed
	for i := 1; i <= 3; i++ {
		keys, err := indexer.Query(ctx, "users", "email", fmt.Sprintf("user%d@example.com", i))
		if err != nil {
			t.Errorf("query for user %d failed: %v", i, err)
		}
		if len(keys) != 1 {
			t.Errorf("user %d should be indexed", i)
		}
	}
}

// TestRedisIndexer_GracefulDegradation tests behavior when Redis is unavailable
func TestRedisIndexer_GracefulDegradation(t *testing.T) {
	// Create indexer with nil Redis client
	indexer := NewRedisIndexer(nil)

	indexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "users-by-email",
		EntityType:  "users",
		ExtractFunc: ExtractJSONField("email"),
	})

	ctx := context.Background()

	user := map[string]interface{}{
		"id":    "user-123",
		"email": "alice@example.com",
	}
	userData, _ := json.Marshal(user)

	// UpdateIndexes should not fail
	err := indexer.UpdateIndexes(ctx, "users/user-123", userData)
	if err != nil {
		t.Errorf("update should gracefully degrade when Redis is nil, got error: %v", err)
	}

	// RemoveFromIndexes should not fail
	err = indexer.RemoveFromIndexes(ctx, "users/user-123", userData)
	if err != nil {
		t.Errorf("remove should gracefully degrade when Redis is nil, got error: %v", err)
	}

	// Query should return error
	_, err = indexer.Query(ctx, "users", "email", "alice@example.com")
	if err == nil {
		t.Error("query should return error when Redis is nil")
	}
}

// TestRedisIndexer_EmptyIndexValue tests handling of empty index values
func TestRedisIndexer_EmptyIndexValue(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	indexer := NewRedisIndexer(redisClient)

	indexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "users-by-email",
		EntityType:  "users",
		ExtractFunc: ExtractJSONField("email"),
	})

	ctx := context.Background()

	// User with empty email
	user := map[string]interface{}{
		"id":    "user-123",
		"email": "",
	}
	userData, _ := json.Marshal(user)

	// Should not create index entry for empty value
	err := indexer.UpdateIndexes(ctx, "users/user-123", userData)
	// ExtractJSONField returns error for empty strings, so this should not fail
	// but also should not create an index entry
	if err != nil {
		t.Logf("Update returned error (expected for empty value): %v", err)
	}

	// Query should return no results
	keys, _ := indexer.Query(ctx, "users", "email", "")
	if len(keys) != 0 {
		t.Error("empty email should not be indexed")
	}
}

// TestRedisIndexer_MissingField tests handling of missing fields
func TestRedisIndexer_MissingField(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	indexer := NewRedisIndexer(redisClient)

	indexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "users-by-email",
		EntityType:  "users",
		ExtractFunc: ExtractJSONField("email"),
	})

	ctx := context.Background()

	// User without email field
	user := map[string]interface{}{
		"id":   "user-123",
		"name": "Alice",
	}
	userData, _ := json.Marshal(user)

	// Should gracefully handle missing field
	err := indexer.UpdateIndexes(ctx, "users/user-123", userData)
	if err != nil {
		t.Logf("Update handled missing field: %v", err)
	}
}

// TestRedisIndexer_WithOwnedClient tests Close() with owned client
func TestRedisIndexer_WithOwnedClient(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	indexer := NewRedisIndexerWithOwnedClient(redisClient)

	// Close should close the Redis client
	err := indexer.Close()
	if err != nil {
		t.Errorf("close failed: %v", err)
	}

	// Redis client should be closed
	ctx := context.Background()
	err = redisClient.Ping(ctx).Err()
	if err == nil {
		t.Error("redis client should be closed")
	}
}

// TestRedisIndexer_NonOwnedClient tests Close() without owned client
func TestRedisIndexer_NonOwnedClient(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	indexer := NewRedisIndexer(redisClient)

	// Close should not close the Redis client
	err := indexer.Close()
	if err != nil {
		t.Errorf("close failed: %v", err)
	}

	// Redis client should still be usable
	ctx := context.Background()
	err = redisClient.Ping(ctx).Err()
	if err != nil {
		t.Error("redis client should still be usable")
	}
}
