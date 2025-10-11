package smarterbase

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestFilesystemBackendGetError tests error handling in Get
func TestFilesystemBackendGetError(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()

	// Try to get non-existent file
	_, err := backend.Get(ctx, "nonexistent.json")
	if err == nil {
		t.Error("expected error for non-existent file")
	}

	if !IsNotFound(err) {
		t.Errorf("expected NotFound error, got: %v", err)
	}
}

// TestFilesystemBackendDeleteError tests error handling in Delete
func TestFilesystemBackendDeleteError(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()

	// Try to delete non-existent file (should not error on filesystem backend)
	err := backend.Delete(ctx, "nonexistent.json")
	if err != nil {
		t.Logf("delete non-existent returned: %v", err)
	}
}

// TestFilesystemBackendReadOnlyError tests error with read-only directory
func TestFilesystemBackendReadOnlyError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping read-only test when running as root")
	}

	ctx := context.Background()
	dir := t.TempDir()
	backend := NewFilesystemBackend(dir)
	defer backend.Close()

	// Write a file first
	key := "test.json"
	err := backend.Put(ctx, key, []byte(`{"test": "data"}`))
	if err != nil {
		t.Fatalf("initial put failed: %v", err)
	}

	// Make directory read-only
	if err := os.Chmod(dir, 0444); err != nil {
		t.Fatalf("chmod failed: %v", err)
	}
	defer os.Chmod(dir, 0755) // Restore for cleanup

	// Try to write - should fail
	err = backend.Put(ctx, "readonly.json", []byte(`{"test": "fail"}`))
	if err == nil {
		t.Error("expected error when writing to read-only directory")
	}

	// Try to delete - should fail
	err = backend.Delete(ctx, key)
	if err == nil {
		t.Error("expected error when deleting from read-only directory")
	}
}

// TestPutJSONMarshalError tests JSON marshal error handling
func TestPutJSONMarshalError(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()

	// Create an object that cannot be marshaled (circular reference)
	type Circular struct {
		Self *Circular
	}
	obj := &Circular{}
	obj.Self = obj // Circular reference

	err := PutJSON(backend, ctx, "circular.json", obj)
	if err == nil {
		t.Error("expected marshal error for circular reference")
	}
}

// TestGetJSONUnmarshalError tests JSON unmarshal error handling
func TestGetJSONUnmarshalError(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()

	// Write invalid JSON
	key := "invalid.json"
	err := backend.Put(ctx, key, []byte(`{invalid json`))
	if err != nil {
		t.Fatalf("put failed: %v", err)
	}

	// Try to read as JSON
	var result map[string]interface{}
	err = GetJSON(backend, ctx, key, &result)
	if err == nil {
		t.Error("expected unmarshal error for invalid JSON")
	}
}

// TestGetJSONNotFound tests GetJSON with non-existent file
func TestGetJSONNotFound(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()

	var result map[string]interface{}
	err := GetJSON(backend, ctx, "nonexistent.json", &result)
	if err == nil {
		t.Error("expected error for non-existent file")
	}

	if !IsNotFound(err) {
		t.Errorf("expected NotFound error, got: %v", err)
	}
}

// TestFilesystemBackendCorruptedData tests handling of corrupted files
func TestFilesystemBackendCorruptedData(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := NewFilesystemBackend(dir)
	defer backend.Close()

	key := "corrupted.json"
	filePath := filepath.Join(dir, key)

	// Write valid data first
	err := backend.Put(ctx, key, []byte(`{"valid": "data"}`))
	if err != nil {
		t.Fatalf("initial put failed: %v", err)
	}

	// Corrupt the file by truncating it
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	f.Write([]byte(`{truncated`))
	f.Close()

	// Try to read - should succeed but return corrupted data
	data, err := backend.Get(ctx, key)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}

	// Verify we got the corrupted data
	if string(data) != "{truncated" {
		t.Errorf("expected corrupted data, got: %s", string(data))
	}
}

// TestFilesystemBackendConcurrentDelete tests concurrent deletes
func TestFilesystemBackendConcurrentDelete(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()

	key := "concurrent.json"
	err := backend.Put(ctx, key, []byte(`{"test": "data"}`))
	if err != nil {
		t.Fatalf("put failed: %v", err)
	}

	// Delete concurrently multiple times (should be safe)
	done := make(chan error, 3)
	for i := 0; i < 3; i++ {
		go func() {
			done <- backend.Delete(ctx, key)
		}()
	}

	// Collect results
	for i := 0; i < 3; i++ {
		err := <-done
		if err != nil {
			t.Logf("concurrent delete returned: %v", err)
		}
	}
}

// TestPutJSONNilBackend tests error handling with operations on nil values
func TestPutJSONWithInvalidInput(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()

	// Try to put nil value (should marshal to "null")
	err := PutJSON(backend, ctx, "nil.json", nil)
	if err != nil {
		t.Fatalf("put nil failed: %v", err)
	}

	// Read it back
	var result interface{}
	err = GetJSON(backend, ctx, "nil.json", &result)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}

	if result != nil {
		t.Errorf("expected nil result, got: %v", result)
	}
}
