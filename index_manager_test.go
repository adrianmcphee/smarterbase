package smarterbase

import (
	"context"
	"encoding/json"
	"testing"
)

type testEntity struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func TestIndexManagerCreate(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	im := NewIndexManager(store)

	ctx := context.Background()
	entity := &testEntity{
		ID:     "123",
		Name:   "Test",
		Status: "active",
	}

	err := im.Create(ctx, "entities/123.json", entity)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify data was stored
	var retrieved testEntity
	err = im.Get(ctx, "entities/123.json", &retrieved)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.ID != "123" {
		t.Errorf("ID = %q, want '123'", retrieved.ID)
	}
	if retrieved.Name != "Test" {
		t.Errorf("Name = %q, want 'Test'", retrieved.Name)
	}
}

func TestIndexManagerUpdate(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	im := NewIndexManager(store)

	ctx := context.Background()
	entity := &testEntity{ID: "123", Name: "Original", Status: "active"}

	// Create initial version
	err := im.Create(ctx, "entities/123.json", entity)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update
	updated := &testEntity{ID: "123", Name: "Updated", Status: "inactive"}
	err = im.Update(ctx, "entities/123.json", updated)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify update
	var retrieved testEntity
	err = im.Get(ctx, "entities/123.json", &retrieved)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.Name != "Updated" {
		t.Errorf("Name = %q, want 'Updated'", retrieved.Name)
	}
	if retrieved.Status != "inactive" {
		t.Errorf("Status = %q, want 'inactive'", retrieved.Status)
	}
}

func TestIndexManagerDelete(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	im := NewIndexManager(store)

	ctx := context.Background()
	entity := &testEntity{ID: "123", Name: "Test", Status: "active"}

	// Create
	err := im.Create(ctx, "entities/123.json", entity)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Delete
	err = im.Delete(ctx, "entities/123.json")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deletion
	exists, err := im.Exists(ctx, "entities/123.json")
	if err != nil {
		t.Fatalf("Exists check failed: %v", err)
	}
	if exists {
		t.Error("entity should not exist after deletion")
	}

	// Get should return error
	var retrieved testEntity
	err = im.Get(ctx, "entities/123.json", &retrieved)
	if !IsNotFound(err) {
		t.Errorf("Get should return ErrNotFound, got %v", err)
	}
}

func TestIndexManagerDeleteNonexistent(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	im := NewIndexManager(store)

	ctx := context.Background()

	// Delete nonexistent entity
	err := im.Delete(ctx, "entities/999.json")
	if !IsNotFound(err) {
		t.Errorf("Delete should return ErrNotFound for nonexistent entity, got %v", err)
	}
}

func TestIndexManagerWithFileIndexer(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create file indexer with a simple index
	indexer := NewIndexer(store)
	indexer.RegisterIndex(&IndexSpec{
		Name: "status",
		KeyFunc: func(data interface{}) (string, error) {
			if entity, ok := data.(*testEntity); ok {
				return entity.Status, nil
			}
			return "", nil
		},
		ExtractFunc: func(data []byte) (interface{}, error) {
			var entity testEntity
			err := json.Unmarshal(data, &entity)
			return &entity, err
		},
		IndexKey: func(key string) string {
			return "indexes/status/" + key + ".json"
		},
	})

	// Create index manager with file indexer
	im := NewIndexManager(store).WithFileIndexer(indexer)

	ctx := context.Background()
	entity := &testEntity{ID: "123", Name: "Test", Status: "active"}

	err := im.Create(ctx, "entities/123.json", entity)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Note: We can't easily verify the index was updated without more complex setup,
	// but we can verify no errors occurred and the data was stored
	var retrieved testEntity
	err = im.Get(ctx, "entities/123.json", &retrieved)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
}

func TestIndexManagerWithLogger(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	logger := &MockLogger{}
	store := NewStoreWithLogger(backend, logger)
	im := NewIndexManager(store)

	ctx := context.Background()
	entity := &testEntity{ID: "123", Name: "Test", Status: "active"}

	err := im.Create(ctx, "entities/123.json", entity)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Logger should be available to index manager
	if im.logger == nil {
		t.Error("index manager should have logger")
	}
}

func TestIndexManagerWithMetrics(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	metrics := NewInMemoryMetrics()
	store := NewStoreWithObservability(backend, &NoOpLogger{}, metrics)
	im := NewIndexManager(store)

	ctx := context.Background()
	entity := &testEntity{ID: "123", Name: "Test", Status: "active"}

	err := im.Create(ctx, "entities/123.json", entity)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify metrics were recorded (at least for the Put operation)
	if metrics.Counters[MetricPutSuccess] < 1 {
		t.Error("expected at least one put success metric")
	}
}

func TestIndexManagerExists(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	im := NewIndexManager(store)

	ctx := context.Background()
	entity := &testEntity{ID: "123", Name: "Test", Status: "active"}

	// Should not exist initially
	exists, err := im.Exists(ctx, "entities/123.json")
	if err != nil {
		t.Fatalf("Exists check failed: %v", err)
	}
	if exists {
		t.Error("entity should not exist initially")
	}

	// Create entity
	err = im.Create(ctx, "entities/123.json", entity)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Should exist now
	exists, err = im.Exists(ctx, "entities/123.json")
	if err != nil {
		t.Fatalf("Exists check failed: %v", err)
	}
	if !exists {
		t.Error("entity should exist after creation")
	}
}

func TestIndexManagerMarshalError(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	im := NewIndexManager(store)

	ctx := context.Background()

	// Try to create with unmarshallable data
	invalidData := make(chan int) // channels can't be marshaled

	err := im.Create(ctx, "entities/invalid.json", invalidData)
	if err == nil {
		t.Error("Create should fail with unmarshallable data")
	}
}

// MockLogger from logger_test.go
type MockLogger struct {
	DebugCalls []string
	InfoCalls  []string
	WarnCalls  []string
	ErrorCalls []string
}

func (m *MockLogger) Debug(msg string, fields ...interface{}) {
	m.DebugCalls = append(m.DebugCalls, msg)
}

func (m *MockLogger) Info(msg string, fields ...interface{}) {
	m.InfoCalls = append(m.InfoCalls, msg)
}

func (m *MockLogger) Warn(msg string, fields ...interface{}) {
	m.WarnCalls = append(m.WarnCalls, msg)
}

func (m *MockLogger) Error(msg string, fields ...interface{}) {
	m.ErrorCalls = append(m.ErrorCalls, msg)
}
