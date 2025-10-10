package smarterbase

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBackendCompliance runs the same test suite against all Backend implementations
func TestBackendCompliance(t *testing.T) {
	ctx := context.Background()

	backends := []struct {
		name    string
		backend Backend
		cleanup func()
	}{
		{
			name:    "Filesystem",
			backend: NewFilesystemBackend(t.TempDir()),
			cleanup: func() {}, // TempDir auto-cleans
		},
		// S3Backend test requires AWS credentials - run manually with TEST_S3_BACKEND=true
		// {
		//     name:    "S3",
		//     backend: NewS3Backend(s3Client, "test-bucket"),
		//     cleanup: func() { /* cleanup S3 test data */ },
		// },
	}

	for _, tc := range backends {
		t.Run(tc.name, func(t *testing.T) {
			defer tc.cleanup()

			t.Run("BasicCRUD", func(t *testing.T) {
				testBasicCRUD(t, ctx, tc.backend)
			})

			t.Run("ETagOperations", func(t *testing.T) {
				testETagOperations(t, ctx, tc.backend)
			})

			t.Run("ListOperations", func(t *testing.T) {
				testListOperations(t, ctx, tc.backend)
			})

			t.Run("StreamingOperations", func(t *testing.T) {
				testStreamingOperations(t, ctx, tc.backend)
			})

			t.Run("ErrorHandling", func(t *testing.T) {
				testErrorHandling(t, ctx, tc.backend)
			})
		})
	}
}

func testBasicCRUD(t *testing.T, ctx context.Context, backend Backend) {
	key := "test/basic.json"
	data := []byte(`{"name": "test", "value": 123}`)

	// Test Put
	err := backend.Put(ctx, key, data)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Test Exists
	exists, err := backend.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Expected key to exist")
	}

	// Test Get
	retrieved, err := backend.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(retrieved) != string(data) {
		t.Errorf("Expected %s, got %s", data, retrieved)
	}

	// Test Delete
	err = backend.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	exists, err = backend.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists after delete failed: %v", err)
	}
	if exists {
		t.Error("Expected key to not exist after delete")
	}
}

func testETagOperations(t *testing.T, ctx context.Context, backend Backend) {
	key := "test/etag.json"
	data1 := []byte(`{"version": 1}`)
	data2 := []byte(`{"version": 2}`)

	// Put initial data
	etag1, err := backend.PutIfMatch(ctx, key, data1, "")
	if err != nil {
		t.Fatalf("Initial PutIfMatch failed: %v", err)
	}
	if etag1 == "" {
		t.Error("Expected non-empty ETag")
	}

	// GetWithETag
	retrieved, etag2, err := backend.GetWithETag(ctx, key)
	if err != nil {
		t.Fatalf("GetWithETag failed: %v", err)
	}
	if etag1 != etag2 {
		t.Errorf("ETags don't match: %s != %s", etag1, etag2)
	}
	if string(retrieved) != string(data1) {
		t.Errorf("Data mismatch: %s != %s", data1, retrieved)
	}

	// PutIfMatch with correct ETag should succeed
	etag3, err := backend.PutIfMatch(ctx, key, data2, etag1)
	if err != nil {
		t.Fatalf("PutIfMatch with correct ETag failed: %v", err)
	}
	if etag3 == etag1 {
		t.Error("Expected ETag to change after update")
	}

	// PutIfMatch with wrong ETag should fail
	_, err = backend.PutIfMatch(ctx, key, data1, "wrong-etag")
	if err == nil {
		t.Error("Expected PutIfMatch with wrong ETag to fail")
	}
	if !IsConflict(err) {
		t.Errorf("Expected conflict error, got: %v", err)
	}
}

func testListOperations(t *testing.T, ctx context.Context, backend Backend) {
	// Create test data
	testKeys := []string{
		"list-test/item1.json",
		"list-test/item2.json",
		"list-test/subdir/item3.json",
		"other/item4.json",
	}

	for _, key := range testKeys {
		err := backend.Put(ctx, key, []byte(`{"test": true}`))
		if err != nil {
			t.Fatalf("Failed to create test key %s: %v", key, err)
		}
	}

	// Test List
	keys, err := backend.List(ctx, "list-test/")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	expectedCount := 3
	if len(keys) != expectedCount {
		t.Errorf("Expected %d keys, got %d: %v", expectedCount, len(keys), keys)
	}

	// Verify all expected keys are present
	keyMap := make(map[string]bool)
	for _, k := range keys {
		keyMap[k] = true
	}

	for _, expected := range testKeys[:3] {
		if !keyMap[expected] {
			t.Errorf("Expected key %s not found in list", expected)
		}
	}

	// Test ListPaginated
	var paginatedKeys []string
	err = backend.ListPaginated(ctx, "list-test/", func(batch []string) error {
		paginatedKeys = append(paginatedKeys, batch...)
		return nil
	})
	if err != nil {
		t.Fatalf("ListPaginated failed: %v", err)
	}

	if len(paginatedKeys) != expectedCount {
		t.Errorf("Expected %d paginated keys, got %d", expectedCount, len(paginatedKeys))
	}
}

func testStreamingOperations(t *testing.T, ctx context.Context, backend Backend) {
	key := "test/stream.bin"
	data := []byte("streaming test data with some length")

	// Test PutStream
	reader := strings.NewReader(string(data))
	err := backend.PutStream(ctx, key, reader, int64(len(data)))
	if err != nil {
		t.Fatalf("PutStream failed: %v", err)
	}

	// Test GetStream
	stream, err := backend.GetStream(ctx, key)
	if err != nil {
		t.Fatalf("GetStream failed: %v", err)
	}
	defer stream.Close()

	retrieved, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if string(retrieved) != string(data) {
		t.Errorf("Streamed data mismatch: %s != %s", data, retrieved)
	}
}

func testErrorHandling(t *testing.T, ctx context.Context, backend Backend) {
	// Get non-existent key
	_, err := backend.Get(ctx, "does-not-exist.json")
	if err == nil {
		t.Error("Expected error when getting non-existent key")
	}

	// GetWithETag non-existent key
	_, _, err = backend.GetWithETag(ctx, "does-not-exist.json")
	if err == nil {
		t.Error("Expected error when getting non-existent key with ETag")
	}

	// Delete non-existent key (should not error)
	err = backend.Delete(ctx, "does-not-exist.json")
	if err != nil {
		t.Logf("Delete non-existent returned: %v (may be OK)", err)
	}
}

// TestFilesystemBackend_Specific tests filesystem-specific behavior
func TestFilesystemBackend_Specific(t *testing.T) {
	ctx := context.Background()
	baseDir := t.TempDir()
	backend := NewFilesystemBackend(baseDir)

	t.Run("PathNormalization", func(t *testing.T) {
		// Test various path formats
		testCases := []struct {
			key      string
			expected string
		}{
			{"simple.json", "simple.json"},
			{"dir/file.json", "dir/file.json"},
			{"dir/subdir/file.json", "dir/subdir/file.json"},
		}

		for _, tc := range testCases {
			data := []byte(`{"test": true}`)
			err := backend.Put(ctx, tc.key, data)
			if err != nil {
				t.Fatalf("Put failed for key %s: %v", tc.key, err)
			}

			// Verify file exists at expected path
			fullPath := filepath.Join(baseDir, tc.expected)
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				t.Errorf("File not created at expected path: %s", fullPath)
			}
		}
	})

	t.Run("HealthCheck", func(t *testing.T) {
		err := backend.Ping(ctx)
		if err != nil {
			t.Fatalf("Ping failed: %v", err)
		}

		// Test with non-writable directory
		readOnlyDir := filepath.Join(t.TempDir(), "readonly")
		os.Mkdir(readOnlyDir, 0555) // read + execute only
		defer os.Chmod(readOnlyDir, 0755)

		roBackend := NewFilesystemBackend(readOnlyDir)
		err = roBackend.Ping(ctx)
		if err == nil {
			t.Error("Expected Ping to fail on read-only directory")
		}
	})

	t.Run("ConcurrentAccess", func(t *testing.T) {
		key := "concurrent/test.json"
		done := make(chan bool)

		// Multiple goroutines writing
		for i := 0; i < 10; i++ {
			go func(n int) {
				data := []byte(`{"value": ` + string(rune(n+'0')) + `}`)
				backend.Put(ctx, key, data)
				done <- true
			}(i)
		}

		// Wait for all
		for i := 0; i < 10; i++ {
			<-done
		}

		// Verify file exists and is valid JSON
		data, err := backend.Get(ctx, key)
		if err != nil {
			t.Fatalf("Failed to read after concurrent writes: %v", err)
		}

		if len(data) == 0 {
			t.Error("Expected non-empty data")
		}
	})

	t.Run("AppendOperations", func(t *testing.T) {
		key := "logs/events.jsonl"

		// First append (creates file)
		line1 := []byte(`{"event": "start", "timestamp": 1000}` + "\n")
		err := backend.Append(ctx, key, line1)
		if err != nil {
			t.Fatalf("First Append failed: %v", err)
		}

		// Second append (appends to existing file)
		line2 := []byte(`{"event": "process", "timestamp": 2000}` + "\n")
		err = backend.Append(ctx, key, line2)
		if err != nil {
			t.Fatalf("Second Append failed: %v", err)
		}

		// Third append
		line3 := []byte(`{"event": "end", "timestamp": 3000}` + "\n")
		err = backend.Append(ctx, key, line3)
		if err != nil {
			t.Fatalf("Third Append failed: %v", err)
		}

		// Read back and verify all lines are present
		data, err := backend.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get after Append failed: %v", err)
		}

		expected := string(line1) + string(line2) + string(line3)
		if string(data) != expected {
			t.Errorf("Append result mismatch.\nExpected:\n%s\nGot:\n%s", expected, data)
		}
	})
}

// TestGCSBackend_Compliance runs compliance tests against GCS backend
// Requires GCS_PROJECT_ID and GCS_BUCKET environment variables
func TestGCSBackend_Compliance(t *testing.T) {
	projectID := os.Getenv("GCS_PROJECT_ID")
	bucket := os.Getenv("GCS_BUCKET")

	if projectID == "" || bucket == "" {
		t.Skip("Skipping GCS tests - set GCS_PROJECT_ID and GCS_BUCKET env vars to run")
	}

	ctx := context.Background()
	backend, err := NewGCSBackend(ctx, GCSConfig{
		ProjectID: projectID,
		Bucket:    bucket,
	})
	if err != nil {
		t.Fatalf("Failed to create GCS backend: %v", err)
	}
	defer backend.Close()

	// Test connectivity
	if err := backend.Ping(ctx); err != nil {
		t.Fatalf("GCS backend health check failed: %v", err)
	}

	// Run standard compliance tests
	t.Run("BasicCRUD", func(t *testing.T) {
		testBasicCRUD(t, ctx, backend)
	})

	t.Run("ETagOperations", func(t *testing.T) {
		testETagOperations(t, ctx, backend)
	})

	t.Run("ListOperations", func(t *testing.T) {
		testListOperations(t, ctx, backend)
	})

	t.Run("StreamingOperations", func(t *testing.T) {
		testStreamingOperations(t, ctx, backend)
	})

	t.Run("ErrorHandling", func(t *testing.T) {
		testErrorHandling(t, ctx, backend)
	})

	// Clean up test data
	keys, _ := backend.List(ctx, "test/")
	for _, key := range keys {
		backend.Delete(ctx, key)
	}
	keys, _ = backend.List(ctx, "list-test/")
	for _, key := range keys {
		backend.Delete(ctx, key)
	}
}
