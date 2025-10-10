package smarterbase

import (
	"context"
	"crypto/rand"
	"testing"
)

func TestEncryptionBackend_ValidKey(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	key := make([]byte, 32)
	rand.Read(key)

	encBackend, err := NewEncryptionBackend(backend, key)
	if err != nil {
		t.Fatalf("Failed to create encryption backend: %v", err)
	}

	if encBackend == nil {
		t.Fatal("Expected non-nil encryption backend")
	}
}

func TestEncryptionBackend_InvalidKeyLength(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())

	// Test various invalid key lengths
	invalidLengths := []int{16, 24, 31, 33, 64}

	for _, length := range invalidLengths {
		key := make([]byte, length)
		_, err := NewEncryptionBackend(backend, key)

		if err == nil {
			t.Errorf("Expected error for key length %d, got nil", length)
		}
	}
}

func TestEncryptionBackend_PutAndGet(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())

	key := make([]byte, 32)
	rand.Read(key)

	encBackend, _ := NewEncryptionBackend(backend, key)

	// Test data
	original := []byte("sensitive data that needs encryption")

	// Put encrypted
	err := encBackend.Put(ctx, "test-key", original)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify data is actually encrypted in storage
	encrypted, _ := backend.Get(ctx, "test-key")
	if string(encrypted) == string(original) {
		t.Error("Data should be encrypted in storage")
	}

	// Get and decrypt
	retrieved, err := encBackend.Get(ctx, "test-key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(retrieved) != string(original) {
		t.Errorf("Expected %s, got %s", original, retrieved)
	}
}

func TestEncryptionBackend_PutIfMatch(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())

	key := make([]byte, 32)
	rand.Read(key)

	encBackend, _ := NewEncryptionBackend(backend, key)

	// Initial put
	data1 := []byte("version 1")
	etag1, err := encBackend.PutIfMatch(ctx, "versioned-key", data1, "")
	if err != nil {
		t.Fatalf("Initial PutIfMatch failed: %v", err)
	}

	// Update with correct ETag
	data2 := []byte("version 2")
	etag2, err := encBackend.PutIfMatch(ctx, "versioned-key", data2, etag1)
	if err != nil {
		t.Fatalf("PutIfMatch with correct ETag failed: %v", err)
	}

	if etag1 == etag2 {
		t.Error("ETag should change after update")
	}

	// Verify retrieval
	retrieved, err := encBackend.Get(ctx, "versioned-key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(retrieved) != string(data2) {
		t.Errorf("Expected %s, got %s", data2, retrieved)
	}
}

func TestEncryptionBackend_GetWithETag(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())

	key := make([]byte, 32)
	rand.Read(key)

	encBackend, _ := NewEncryptionBackend(backend, key)

	// Put data
	original := []byte("test data")
	encBackend.Put(ctx, "test-key", original)

	// Get with ETag
	retrieved, etag, err := encBackend.GetWithETag(ctx, "test-key")
	if err != nil {
		t.Fatalf("GetWithETag failed: %v", err)
	}

	if string(retrieved) != string(original) {
		t.Errorf("Expected %s, got %s", original, retrieved)
	}

	if etag == "" {
		t.Error("Expected non-empty ETag")
	}
}

func TestEncryptionBackend_MultipleOperations(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())

	key := make([]byte, 32)
	rand.Read(key)

	encBackend, _ := NewEncryptionBackend(backend, key)

	// Test multiple Put/Get cycles
	for i := 0; i < 10; i++ {
		data := []byte("iteration " + string(rune('0'+i)))
		key := "test-key-" + string(rune('0'+i))

		err := encBackend.Put(ctx, key, data)
		if err != nil {
			t.Fatalf("Put failed at iteration %d: %v", i, err)
		}

		retrieved, err := encBackend.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get failed at iteration %d: %v", i, err)
		}

		if string(retrieved) != string(data) {
			t.Errorf("Iteration %d: Expected %s, got %s", i, data, retrieved)
		}
	}
}

func TestEncryptionBackend_NonceUniqueness(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())

	key := make([]byte, 32)
	rand.Read(key)

	encBackend, _ := NewEncryptionBackend(backend, key)

	// Encrypt same plaintext multiple times
	plaintext := []byte("same data")

	var ciphertexts [][]byte
	for i := 0; i < 5; i++ {
		keyName := "test-" + string(rune('0'+i))
		encBackend.Put(ctx, keyName, plaintext)

		// Get encrypted data directly from backend
		encrypted, _ := backend.Get(ctx, keyName)
		ciphertexts = append(ciphertexts, encrypted)
	}

	// Verify all ciphertexts are different (nonces are random)
	for i := 0; i < len(ciphertexts); i++ {
		for j := i + 1; j < len(ciphertexts); j++ {
			if string(ciphertexts[i]) == string(ciphertexts[j]) {
				t.Error("Ciphertexts should be different due to random nonces")
			}
		}
	}
}

func TestEncryptionBackend_Delete(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())

	key := make([]byte, 32)
	rand.Read(key)

	encBackend, _ := NewEncryptionBackend(backend, key)

	// Put and delete
	encBackend.Put(ctx, "test-key", []byte("data"))
	err := encBackend.Delete(ctx, "test-key")

	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	_, err = encBackend.Get(ctx, "test-key")
	if err == nil {
		t.Error("Expected error when getting deleted key")
	}
}

func TestEncryptionBackend_CorruptedData(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())

	key := make([]byte, 32)
	rand.Read(key)

	encBackend, _ := NewEncryptionBackend(backend, key)

	// Put encrypted data
	encBackend.Put(ctx, "test-key", []byte("original data"))

	// Corrupt the encrypted data
	encrypted, _ := backend.Get(ctx, "test-key")
	corrupted := append(encrypted[:len(encrypted)-5], []byte("xxxxx")...)
	backend.Put(ctx, "test-key", corrupted)

	// Attempt to decrypt corrupted data
	_, err := encBackend.Get(ctx, "test-key")

	if err == nil {
		t.Error("Expected error when decrypting corrupted data")
	}
}

func TestEncryptionBackend_GetStream(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())

	key := make([]byte, 32)
	rand.Read(key)

	encBackend, _ := NewEncryptionBackend(backend, key)

	// Put data first
	testData := []byte("streaming test data with encryption")
	err := encBackend.Put(ctx, "stream-key", testData)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get stream
	reader, err := encBackend.GetStream(ctx, "stream-key")
	if err != nil {
		t.Fatalf("GetStream failed: %v", err)
	}
	defer reader.Close()

	// Read all data from stream
	retrieved := make([]byte, len(testData))
	n, err := reader.Read(retrieved)
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("Read from stream failed: %v", err)
	}

	if n != len(testData) {
		t.Errorf("Expected to read %d bytes, got %d", len(testData), n)
	}

	if string(retrieved) != string(testData) {
		t.Errorf("Expected %s, got %s", testData, retrieved)
	}
}

func TestEncryptionBackend_PutStream(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())

	key := make([]byte, 32)
	rand.Read(key)

	encBackend, _ := NewEncryptionBackend(backend, key)

	// Create test data
	testData := []byte("large streaming data for put stream test with encryption")
	reader := newBytesReader(testData)

	// Put via stream
	err := encBackend.PutStream(ctx, "stream-put-key", reader, int64(len(testData)))
	if err != nil {
		t.Fatalf("PutStream failed: %v", err)
	}

	// Retrieve and verify
	retrieved, err := encBackend.Get(ctx, "stream-put-key")
	if err != nil {
		t.Fatalf("Get after PutStream failed: %v", err)
	}

	if string(retrieved) != string(testData) {
		t.Errorf("Expected %s, got %s", testData, retrieved)
	}
}

func TestEncryptionBackend_Append(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())

	key := make([]byte, 32)
	rand.Read(key)

	encBackend, _ := NewEncryptionBackend(backend, key)

	// First append (creates file)
	line1 := []byte("first line\n")
	err := encBackend.Append(ctx, "append-key", line1)
	if err != nil {
		t.Fatalf("First Append failed: %v", err)
	}

	// Second append (appends to existing)
	line2 := []byte("second line\n")
	err = encBackend.Append(ctx, "append-key", line2)
	if err != nil {
		t.Fatalf("Second Append failed: %v", err)
	}

	// Retrieve and verify both lines
	retrieved, err := encBackend.Get(ctx, "append-key")
	if err != nil {
		t.Fatalf("Get after Append failed: %v", err)
	}

	expected := string(line1) + string(line2)
	if string(retrieved) != expected {
		t.Errorf("Expected %s, got %s", expected, retrieved)
	}
}
