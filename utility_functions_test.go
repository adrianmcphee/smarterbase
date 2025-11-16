package smarterbase

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestNewStdLogger tests the StdLogger constructor
func TestNewStdLogger(t *testing.T) {
	logger := NewStdLogger("test")
	if logger == nil {
		t.Fatal("expected logger, got nil")
	}

	// Test that it works
	logger.Info("test message")
	logger.Debug("debug message")
	logger.Warn("warn message")
	logger.Error("error message")
}

// TestStoreSetLogger tests setting logger on store
func TestStoreSetLogger(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()
	store := NewStore(backend)

	logger := &StdLogger{}
	store.SetLogger(logger)

	// Verify store continues to work
	ctx := context.Background()
	err := store.PutJSON(ctx, "test.json", map[string]interface{}{"key": "value"})
	if err != nil {
		t.Fatalf("put failed after setting logger: %v", err)
	}
}

// TestStoreSetMetrics tests setting metrics on store
func TestStoreSetMetrics(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()
	store := NewStore(backend)

	metrics := &NoOpMetrics{}
	store.SetMetrics(metrics)

	// Verify store continues to work
	ctx := context.Background()
	err := store.PutJSON(ctx, "test.json", map[string]interface{}{"key": "value"})
	if err != nil {
		t.Fatalf("put failed after setting metrics: %v", err)
	}
}

// TestIndexManagerWithRedisIndexer tests adding Redis indexer
func TestIndexManagerWithRedisIndexer(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()
	store := NewStore(backend)

	// Use miniredis for testing
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	redisIndexer := NewRedisIndexer(redisClient)

	// Register an index
	redisIndexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "users-by-email",
		EntityType:  "users",
		ExtractFunc: ExtractJSONField("email"),
	})

	// Add Redis indexer to index manager
	indexManager := NewIndexManager(store).WithRedisIndexer(redisIndexer)

	// Verify index manager works with Redis indexer
	key := "users/user-123.json"
	data := map[string]interface{}{
		"id":    "user-123",
		"email": "test@example.com",
	}

	err = indexManager.Create(ctx, key, data)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Verify Redis index was updated
	keys, err := redisIndexer.Query(ctx, "users", "email", "test@example.com")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(keys) != 1 || keys[0] != key {
		t.Errorf("expected key '%s' in Redis index, got %v", key, keys)
	}
}

// TestMultiIndexSpec tests creating a multi-index spec for Redis
func TestMultiIndexSpec(t *testing.T) {
	// Create a multi-index spec for Redis indexing
	spec := &MultiIndexSpec{
		Name:        "users-by-email",
		EntityType:  "users",
		ExtractFunc: ExtractJSONField("email"),
	}

	if spec.Name != "users-by-email" {
		t.Errorf("expected name 'users-by-email', got '%s'", spec.Name)
	}

	if spec.EntityType != "users" {
		t.Errorf("expected entity type 'users', got '%s'", spec.EntityType)
	}

	if spec.ExtractFunc == nil {
		t.Error("expected ExtractFunc to be set")
	}

	// Test ExtractFunc
	testData := []byte(`{"email": "test@example.com"}`)

	entries, err := spec.ExtractFunc("users/user-123.json", testData)
	if err != nil {
		t.Fatalf("ExtractFunc failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].IndexName != "email" {
		t.Errorf("expected index name 'email', got '%s'", entries[0].IndexName)
	}

	if entries[0].IndexValue != "test@example.com" {
		t.Errorf("expected index value 'test@example.com', got '%s'", entries[0].IndexValue)
	}
}
