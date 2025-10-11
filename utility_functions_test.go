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
		Name:       "users-by-email",
		EntityType: "users",
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

// TestSimpleIndexSpec tests creating a simple index spec
func TestSimpleIndexSpec(t *testing.T) {
	spec := SimpleIndexSpec("users-by-email", "idx/users/email/", func(data interface{}) string {
		if m, ok := data.(map[string]interface{}); ok {
			if email, ok := m["email"].(string); ok {
				return email
			}
		}
		return ""
	})

	if spec.Name != "users-by-email" {
		t.Errorf("expected name 'users-by-email', got '%s'", spec.Name)
	}

	if spec.KeyFunc == nil {
		t.Error("expected KeyFunc to be set")
	}

	if spec.ExtractFunc == nil {
		t.Error("expected ExtractFunc to be set")
	}

	if spec.IndexKey == nil {
		t.Error("expected IndexKey to be set")
	}

	// Test KeyFunc
	testData := map[string]interface{}{
		"email": "test@example.com",
	}

	key, err := spec.KeyFunc(testData)
	if err != nil {
		t.Fatalf("KeyFunc failed: %v", err)
	}

	if key != "test@example.com" {
		t.Errorf("expected key 'test@example.com', got '%s'", key)
	}
}

