package smarterbase

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/redis/go-redis/v9"
)

// Test entities for ADR-0006 helper functions
type TestUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
	Age   int    `json:"age"`
}

type TestProperty struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Address   string `json:"address"`
	IsPrimary bool   `json:"is_primary"`
}

func TestQueryWithFallback_WithRedis(t *testing.T) {
	// Setup
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Setup Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	ctx := context.Background()

	// Ping Redis to check if available
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping test")
	}
	defer redisClient.Close()

	redisIndexer := NewRedisIndexer(redisClient)

	// Register index
	redisIndexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "users-by-role",
		EntityType:  "users",
		ExtractFunc: ExtractJSONField("role"),
	})

	// Create test users
	admin1 := &TestUser{ID: "user-1", Email: "admin1@test.com", Role: "admin", Age: 30}
	admin2 := &TestUser{ID: "user-2", Email: "admin2@test.com", Role: "admin", Age: 35}
	regular := &TestUser{ID: "user-3", Email: "user@test.com", Role: "user", Age: 25}

	// Save users
	if err := store.PutJSON(ctx, "users/user-1.json", admin1); err != nil {
		t.Fatalf("Failed to save admin1: %v", err)
	}
	if err := store.PutJSON(ctx, "users/user-2.json", admin2); err != nil {
		t.Fatalf("Failed to save admin2: %v", err)
	}
	if err := store.PutJSON(ctx, "users/user-3.json", regular); err != nil {
		t.Fatalf("Failed to save regular user: %v", err)
	}

	// Update indexes
	admin1Bytes, _ := marshalJSON(admin1)
	admin2Bytes, _ := marshalJSON(admin2)
	regularBytes, _ := marshalJSON(regular)

	redisIndexer.UpdateIndexes(ctx, "users/user-1.json", admin1Bytes)
	redisIndexer.UpdateIndexes(ctx, "users/user-2.json", admin2Bytes)
	redisIndexer.UpdateIndexes(ctx, "users/user-3.json", regularBytes)

	// Test: Query for admins using Redis index
	admins, err := QueryWithFallback[TestUser](
		ctx, store, redisIndexer,
		"users", "role", "admin",
		"users/",
		func(u *TestUser) bool { return u.Role == "admin" },
	)

	if err != nil {
		t.Fatalf("QueryWithFallback failed: %v", err)
	}

	if len(admins) != 2 {
		t.Errorf("Expected 2 admins, got %d", len(admins))
	}

	// Verify results
	foundAdmin1 := false
	foundAdmin2 := false
	for _, admin := range admins {
		if admin.ID == "user-1" {
			foundAdmin1 = true
		}
		if admin.ID == "user-2" {
			foundAdmin2 = true
		}
	}

	if !foundAdmin1 || !foundAdmin2 {
		t.Error("Did not find all expected admins")
	}

	// Cleanup
	redisClient.FlushAll(ctx)
}

func TestQueryWithFallback_Fallback(t *testing.T) {
	// Setup without Redis - should fall back to scan
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	ctx := context.Background()

	// Create test users
	admin1 := &TestUser{ID: "user-1", Email: "admin1@test.com", Role: "admin", Age: 30}
	admin2 := &TestUser{ID: "user-2", Email: "admin2@test.com", Role: "admin", Age: 35}
	regular := &TestUser{ID: "user-3", Email: "user@test.com", Role: "user", Age: 25}

	// Save users
	if err := store.PutJSON(ctx, "users/user-1.json", admin1); err != nil {
		t.Fatalf("Failed to save admin1: %v", err)
	}
	if err := store.PutJSON(ctx, "users/user-2.json", admin2); err != nil {
		t.Fatalf("Failed to save admin2: %v", err)
	}
	if err := store.PutJSON(ctx, "users/user-3.json", regular); err != nil {
		t.Fatalf("Failed to save regular user: %v", err)
	}

	// Test: Query without Redis should fallback to scan
	admins, err := QueryWithFallback[TestUser](
		ctx, store, nil, // No Redis indexer
		"users", "role", "admin",
		"users/",
		func(u *TestUser) bool { return u.Role == "admin" },
	)

	if err != nil {
		t.Fatalf("QueryWithFallback failed: %v", err)
	}

	if len(admins) != 2 {
		t.Errorf("Expected 2 admins, got %d", len(admins))
	}
}

func TestBatchGetWithFilter(t *testing.T) {
	// Setup
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	ctx := context.Background()

	// Create test properties
	props := []*TestProperty{
		{ID: "prop-1", UserID: "user-1", Address: "123 Main St", IsPrimary: true},
		{ID: "prop-2", UserID: "user-1", Address: "456 Oak Ave", IsPrimary: false},
		{ID: "prop-3", UserID: "user-1", Address: "789 Elm St", IsPrimary: false},
	}

	// Save properties
	keys := []string{}
	for _, prop := range props {
		key := fmt.Sprintf("properties/%s.json", prop.ID)
		if err := store.PutJSON(ctx, key, prop); err != nil {
			t.Fatalf("Failed to save property: %v", err)
		}
		keys = append(keys, key)
	}

	// Test: Get all properties (no filter)
	allProps, err := BatchGetWithFilter[TestProperty](ctx, store, keys, nil)
	if err != nil {
		t.Fatalf("BatchGetWithFilter failed: %v", err)
	}

	if len(allProps) != 3 {
		t.Errorf("Expected 3 properties, got %d", len(allProps))
	}

	// Test: Get only primary properties
	primaryProps, err := BatchGetWithFilter[TestProperty](
		ctx, store, keys,
		func(p *TestProperty) bool { return p.IsPrimary },
	)

	if err != nil {
		t.Fatalf("BatchGetWithFilter with filter failed: %v", err)
	}

	if len(primaryProps) != 1 {
		t.Errorf("Expected 1 primary property, got %d", len(primaryProps))
	}

	if primaryProps[0].ID != "prop-1" {
		t.Errorf("Expected prop-1, got %s", primaryProps[0].ID)
	}
}

func TestBatchGetWithFilter_MissingKeys(t *testing.T) {
	// Setup
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	ctx := context.Background()

	// Save only one property
	prop := &TestProperty{ID: "prop-1", UserID: "user-1", Address: "123 Main St", IsPrimary: true}
	if err := store.PutJSON(ctx, "properties/prop-1.json", prop); err != nil {
		t.Fatalf("Failed to save property: %v", err)
	}

	// Try to get multiple keys including non-existent ones
	keys := []string{
		"properties/prop-1.json",
		"properties/prop-2.json", // Doesn't exist
		"properties/prop-3.json", // Doesn't exist
	}

	results, err := BatchGetWithFilter[TestProperty](ctx, store, keys, nil)
	if err != nil {
		t.Fatalf("BatchGetWithFilter failed: %v", err)
	}

	// Should only return the one that exists
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	if results[0].ID != "prop-1" {
		t.Errorf("Expected prop-1, got %s", results[0].ID)
	}
}

func TestUpdateWithIndexes(t *testing.T) {
	// Setup
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Setup Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	ctx := context.Background()

	// Ping Redis to check if available
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping test")
	}
	defer redisClient.Close()

	redisIndexer := NewRedisIndexer(redisClient)

	// Register indexes
	redisIndexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "users-by-email",
		EntityType:  "users",
		ExtractFunc: ExtractJSONField("email"),
	})

	redisIndexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "users-by-role",
		EntityType:  "users",
		ExtractFunc: ExtractJSONField("role"),
	})

	// Create initial user
	user := &TestUser{ID: "user-1", Email: "old@test.com", Role: "user", Age: 30}

	// Initial save with indexes
	userBytes, _ := marshalJSON(user)
	if err := store.PutJSON(ctx, "users/user-1.json", user); err != nil {
		t.Fatalf("Failed to save user: %v", err)
	}
	redisIndexer.UpdateIndexes(ctx, "users/user-1.json", userBytes)

	// Update user email
	oldEmail := user.Email
	user.Email = "new@test.com"

	err := UpdateWithIndexes(
		ctx, store, redisIndexer,
		"users/user-1.json", user,
		[]IndexUpdate{
			{EntityType: "users", IndexField: "email", OldValue: oldEmail, NewValue: user.Email},
		},
	)

	if err != nil {
		t.Fatalf("UpdateWithIndexes failed: %v", err)
	}

	// Verify user was updated
	var updatedUser TestUser
	if err := store.GetJSON(ctx, "users/user-1.json", &updatedUser); err != nil {
		t.Fatalf("Failed to get updated user: %v", err)
	}

	if updatedUser.Email != "new@test.com" {
		t.Errorf("Expected email 'new@test.com', got '%s'", updatedUser.Email)
	}

	// Verify new index exists
	newKeys, err := redisIndexer.Query(ctx, "users", "email", "new@test.com")
	if err != nil {
		t.Fatalf("Failed to query new index: %v", err)
	}

	if len(newKeys) != 1 {
		t.Errorf("Expected 1 key in new index, got %d", len(newKeys))
	}

	// Cleanup
	redisClient.FlushAll(ctx)
}

func TestUpdateWithIndexes_WithoutRedis(t *testing.T) {
	// Setup without Redis
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	ctx := context.Background()

	// Create and update user
	user := &TestUser{ID: "user-1", Email: "old@test.com", Role: "user", Age: 30}

	oldEmail := user.Email
	user.Email = "new@test.com"

	// Should work even without Redis
	err := UpdateWithIndexes(
		ctx, store, nil, // No Redis
		"users/user-1.json", user,
		[]IndexUpdate{
			{EntityType: "users", IndexField: "email", OldValue: oldEmail, NewValue: user.Email},
		},
	)

	if err != nil {
		t.Fatalf("UpdateWithIndexes failed without Redis: %v", err)
	}

	// Verify user was updated
	var updatedUser TestUser
	if err := store.GetJSON(ctx, "users/user-1.json", &updatedUser); err != nil {
		t.Fatalf("Failed to get updated user: %v", err)
	}

	if updatedUser.Email != "new@test.com" {
		t.Errorf("Expected email 'new@test.com', got '%s'", updatedUser.Email)
	}
}

func TestQueryWithFallback_Profiling(t *testing.T) {
	// Setup
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	ctx := context.Background()

	// Setup profiler
	profiler := NewQueryProfiler()
	ctx = WithProfiler(ctx, profiler)

	// Create test data
	admin := &TestUser{ID: "user-1", Email: "admin@test.com", Role: "admin", Age: 30}
	if err := store.PutJSON(ctx, "users/user-1.json", admin); err != nil {
		t.Fatalf("Failed to save user: %v", err)
	}

	// Query (will use fallback since no Redis)
	_, err := QueryWithFallback[TestUser](
		ctx, store, nil,
		"users", "role", "admin",
		"users/",
		func(u *TestUser) bool { return u.Role == "admin" },
	)

	if err != nil {
		t.Fatalf("QueryWithFallback failed: %v", err)
	}

	// Check profiling was recorded
	profiles := profiler.GetProfiles()
	if len(profiles) == 0 {
		t.Error("Expected profiling data, got none")
	}

	// Verify profile details
	profile := profiles[0]
	if profile.Complexity != ComplexityON {
		t.Errorf("Expected ComplexityON, got %s", profile.Complexity)
	}

	if !profile.FallbackPath {
		t.Error("Expected FallbackPath to be true")
	}

	if profile.IndexUsed != "none:full-scan" {
		t.Errorf("Expected 'none:full-scan', got '%s'", profile.IndexUsed)
	}
}

// Helper function to marshal JSON (for tests)
func marshalJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
