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
