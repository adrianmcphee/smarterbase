package smarterbase

import (
	"context"
	"testing"
)

func TestHelpers_PutJSON(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())

	type TestData struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	data := &TestData{Name: "test", Value: 42}

	err := PutJSON(backend, ctx, "test-key", data)
	if err != nil {
		t.Fatalf("PutJSON failed: %v", err)
	}

	// Verify data was stored
	exists, err := backend.Exists(ctx, "test-key")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Expected key to exist after PutJSON")
	}
}

func TestHelpers_GetJSON(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())

	type TestData struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	// Put data first
	original := &TestData{Name: "test", Value: 42}
	PutJSON(backend, ctx, "test-key", original)

	// Get using helper
	var retrieved TestData
	err := GetJSON(backend, ctx, "test-key", &retrieved)
	if err != nil {
		t.Fatalf("GetJSON failed: %v", err)
	}

	if retrieved.Name != original.Name {
		t.Errorf("Expected name %s, got %s", original.Name, retrieved.Name)
	}
	if retrieved.Value != original.Value {
		t.Errorf("Expected value %d, got %d", original.Value, retrieved.Value)
	}
}

func TestHelpers_Now(t *testing.T) {
	now := Now()
	if now.IsZero() {
		t.Error("Now() returned zero time")
	}
}

func TestFilesystemBackend_Close(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	err := backend.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}
}

func TestFilesystemBackend_WithStripes(t *testing.T) {
	backend := NewFilesystemBackendWithStripes(t.TempDir(), 64)
	if backend == nil {
		t.Fatal("NewFilesystemBackendWithStripes returned nil")
	}

	ctx := context.Background()
	err := backend.Put(ctx, "test-key", []byte("test data"))
	if err != nil {
		t.Fatalf("Put with striped backend failed: %v", err)
	}

	data, err := backend.Get(ctx, "test-key")
	if err != nil {
		t.Fatalf("Get with striped backend failed: %v", err)
	}

	if string(data) != "test data" {
		t.Errorf("Expected 'test data', got %s", data)
	}
}

// Test helpers for generic functions

type testUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	backend := NewFilesystemBackend(t.TempDir())
	return NewStore(backend)
}

// BatchGet Tests

func TestBatchGet_Success(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)

	// Create test data
	users := []*testUser{
		{ID: "user1", Email: "alice@example.com", Name: "Alice", Role: "admin"},
		{ID: "user2", Email: "bob@example.com", Name: "Bob", Role: "user"},
		{ID: "user3", Email: "charlie@example.com", Name: "Charlie", Role: "user"},
	}

	keys := []string{}
	for _, user := range users {
		key := "users/" + user.ID + ".json"
		keys = append(keys, key)
		if err := store.PutJSON(ctx, key, user); err != nil {
			t.Fatal(err)
		}
	}

	// Test BatchGet
	results, err := BatchGet[testUser](ctx, store, keys)
	if err != nil {
		t.Fatalf("BatchGet failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	// Verify contents
	for i, result := range results {
		if result.ID != users[i].ID {
			t.Errorf("Result %d: expected ID %s, got %s", i, users[i].ID, result.ID)
		}
		if result.Email != users[i].Email {
			t.Errorf("Result %d: expected email %s, got %s", i, users[i].Email, result.Email)
		}
	}
}

func TestBatchGet_EmptyKeys(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)

	results, err := BatchGet[testUser](ctx, store, []string{})
	if err != nil {
		t.Fatalf("BatchGet with empty keys failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

func TestBatchGet_MissingKeys(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)

	// Create only one user
	user := &testUser{ID: "user1", Email: "alice@example.com", Name: "Alice"}
	if err := store.PutJSON(ctx, "users/user1.json", user); err != nil {
		t.Fatal(err)
	}

	// Request three keys, only one exists
	keys := []string{
		"users/user1.json",
		"users/missing1.json",
		"users/missing2.json",
	}

	results, err := BatchGet[testUser](ctx, store, keys)
	if err != nil {
		t.Fatalf("BatchGet failed: %v", err)
	}

	// Should skip missing items
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	if results[0].ID != "user1" {
		t.Errorf("Expected user1, got %s", results[0].ID)
	}
}

func TestBatchGetWithErrors_Success(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)

	// Create test data
	user := &testUser{ID: "user1", Email: "alice@example.com", Name: "Alice"}
	if err := store.PutJSON(ctx, "users/user1.json", user); err != nil {
		t.Fatal(err)
	}

	keys := []string{
		"users/user1.json",
		"users/missing.json",
	}

	results, errors := BatchGetWithErrors[testUser](ctx, store, keys)

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	// Missing items should not generate errors (404 is expected)
	if errors != nil {
		t.Errorf("Expected no errors for missing items, got %v", errors)
	}
}

// KeyBuilder Tests

func TestKeyBuilder_BasicKey(t *testing.T) {
	kb := KeyBuilder{Prefix: "users", Suffix: ".json"}

	key := kb.Key("user123")
	expected := "users/user123.json"

	if key != expected {
		t.Errorf("Expected %s, got %s", expected, key)
	}
}

func TestKeyBuilder_NoSuffix(t *testing.T) {
	kb := KeyBuilder{Prefix: "sessions"}

	key := kb.Key("sess_abc")
	expected := "sessions/sess_abc"

	if key != expected {
		t.Errorf("Expected %s, got %s", expected, key)
	}
}

func TestKeyBuilder_Keys(t *testing.T) {
	kb := KeyBuilder{Prefix: "users", Suffix: ".json"}

	ids := []string{"user1", "user2", "user3"}
	keys := kb.Keys(ids)

	expected := []string{
		"users/user1.json",
		"users/user2.json",
		"users/user3.json",
	}

	if len(keys) != len(expected) {
		t.Fatalf("Expected %d keys, got %d", len(expected), len(keys))
	}

	for i, key := range keys {
		if key != expected[i] {
			t.Errorf("Key %d: expected %s, got %s", i, expected[i], key)
		}
	}
}

func TestKeyBuilder_EmptyIDs(t *testing.T) {
	kb := KeyBuilder{Prefix: "users", Suffix: ".json"}

	keys := kb.Keys([]string{})
	if len(keys) != 0 {
		t.Errorf("Expected empty slice, got %d keys", len(keys))
	}
}

// UnmarshalBatchResults Tests

func TestUnmarshalBatchResults_Success(t *testing.T) {
	// Simulate BatchGetJSON result
	results := map[string]interface{}{
		"users/user1.json": map[string]interface{}{
			"id":    "user1",
			"email": "alice@example.com",
			"name":  "Alice",
			"role":  "admin",
		},
		"users/user2.json": map[string]interface{}{
			"id":    "user2",
			"email": "bob@example.com",
			"name":  "Bob",
			"role":  "user",
		},
	}

	users, err := UnmarshalBatchResults[testUser](results)
	if err != nil {
		t.Fatalf("UnmarshalBatchResults failed: %v", err)
	}

	if len(users) != 2 {
		t.Errorf("Expected 2 users, got %d", len(users))
	}

	// Verify at least one user
	found := false
	for _, user := range users {
		if user.ID == "user1" && user.Email == "alice@example.com" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected to find user1 with correct email")
	}
}

func TestUnmarshalBatchResults_Empty(t *testing.T) {
	results := map[string]interface{}{}

	users, err := UnmarshalBatchResults[testUser](results)
	if err != nil {
		t.Fatalf("UnmarshalBatchResults failed: %v", err)
	}

	if len(users) != 0 {
		t.Errorf("Expected 0 users, got %d", len(users))
	}
}

// QueryIndexTyped and GetByIndex require Redis setup

func TestQueryIndexTyped_NoRedis(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	im := NewIndexManager(store)

	// Without Redis, should return error
	_, err := QueryIndexTyped[testUser](ctx, im, "users", "email", "test@example.com")
	if err == nil {
		t.Error("Expected error when Redis not configured")
	}

	if err.Error() != "redis indexer not configured" {
		t.Errorf("Expected 'redis indexer not configured' error, got: %v", err)
	}
}

func TestGetByIndex_NoRedis(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	im := NewIndexManager(store)

	// Without Redis, should return error
	_, err := GetByIndex[testUser](ctx, im, "users", "email", "test@example.com")
	if err == nil {
		t.Error("Expected error when Redis not configured")
	}

	if err.Error() != "redis indexer not configured" {
		t.Errorf("Expected 'redis indexer not configured' error, got: %v", err)
	}
}

// Benchmark tests

func BenchmarkBatchGet(b *testing.B) {
	ctx := context.Background()
	backend := NewFilesystemBackend(b.TempDir())
	store := NewStore(backend)

	// Setup test data
	keys := make([]string, 100)
	for i := 0; i < 100; i++ {
		user := &testUser{
			ID:    string(rune('a' + i)),
			Email: "user" + string(rune('a'+i)) + "@example.com",
			Name:  "User " + string(rune('a'+i)),
		}
		key := "users/" + user.ID + ".json"
		keys[i] = key
		store.PutJSON(ctx, key, user)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BatchGet[testUser](ctx, store, keys)
	}
}

func BenchmarkKeyBuilder(b *testing.B) {
	kb := KeyBuilder{Prefix: "users", Suffix: ".json"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		kb.Key("user123")
	}
}

func BenchmarkKeyBuilder_Keys(b *testing.B) {
	kb := KeyBuilder{Prefix: "users", Suffix: ".json"}
	ids := make([]string, 100)
	for i := 0; i < 100; i++ {
		ids[i] = "user" + string(rune('a'+i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		kb.Keys(ids)
	}
}
