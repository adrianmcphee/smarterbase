package smarterbase

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// EncryptionBackend wraps any backend with AES-256-GCM encryption at rest.
//
// All data is encrypted before storage and decrypted after retrieval.
// Uses AES-256-GCM for authenticated encryption with random nonces.
//
// Example:
//
//	key := make([]byte, 32) // Generate or load from secrets manager
//	rand.Read(key)
//	encryptedBackend := smarterbase.NewEncryptionBackend(s3Backend, key)
//	store := smarterbase.NewStore(encryptedBackend)
type EncryptionBackend struct {
	Backend
	key []byte // 32 bytes for AES-256
}

// NewEncryptionBackend wraps a backend with AES-256-GCM encryption.
// Key must be exactly 32 bytes for AES-256.
func NewEncryptionBackend(backend Backend, key []byte) (*EncryptionBackend, error) {
	if len(key) != 32 {
		return nil, WithContext(ErrInvalidConfig, map[string]interface{}{
			"expected_key_length": 32,
			"actual_key_length":   len(key),
			"reason":              "AES-256 requires 32-byte key",
		})
	}

	return &EncryptionBackend{
		Backend: backend,
		key:     key,
	}, nil
}

// Put encrypts data before storing
func (e *EncryptionBackend) Put(ctx context.Context, key string, data []byte) error {
	encrypted, err := e.encrypt(data)
	if err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}
	return e.Backend.Put(ctx, key, encrypted)
}

// Get decrypts data after retrieving
func (e *EncryptionBackend) Get(ctx context.Context, key string) ([]byte, error) {
	encrypted, err := e.Backend.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return e.decrypt(encrypted)
}

// PutIfMatch encrypts and stores with optimistic locking
func (e *EncryptionBackend) PutIfMatch(ctx context.Context, key string, data []byte, expectedETag string) (string, error) {
	encrypted, err := e.encrypt(data)
	if err != nil {
		return "", fmt.Errorf("encryption failed: %w", err)
	}
	return e.Backend.PutIfMatch(ctx, key, encrypted, expectedETag)
}

// GetWithETag decrypts data and returns ETag
func (e *EncryptionBackend) GetWithETag(ctx context.Context, key string) ([]byte, string, error) {
	encrypted, etag, err := e.Backend.GetWithETag(ctx, key)
	if err != nil {
		return nil, "", err
	}
	decrypted, err := e.decrypt(encrypted)
	if err != nil {
		return nil, "", err
	}
	return decrypted, etag, nil
}

// PutStream encrypts streaming data
func (e *EncryptionBackend) PutStream(ctx context.Context, key string, reader io.Reader, size int64) error {
	// Read all data (streaming encryption requires buffering for GCM)
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read stream: %w", err)
	}

	encrypted, err := e.encrypt(data)
	if err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}

	// Use Put instead of PutStream since we have the full encrypted data
	return e.Backend.Put(ctx, key, encrypted)
}

// GetStream decrypts streaming data
func (e *EncryptionBackend) GetStream(ctx context.Context, key string) (io.ReadCloser, error) {
	// Get encrypted data
	encrypted, err := e.Backend.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	// Decrypt
	decrypted, err := e.decrypt(encrypted)
	if err != nil {
		return nil, err
	}

	// Return as ReadCloser
	return io.NopCloser(io.Reader(newBytesReader(decrypted))), nil
}

// Append encrypts and appends data
func (e *EncryptionBackend) Append(ctx context.Context, key string, data []byte) error {
	// Get existing data
	existing, err := e.Get(ctx, key)
	if err != nil && !IsNotFound(err) {
		return err
	}

	// Append new data
	combined := append(existing, data...)

	// Encrypt and store
	return e.Put(ctx, key, combined)
}

// encrypt uses AES-256-GCM with random nonce
func (e *EncryptionBackend) encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt and prepend nonce to ciphertext
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// decrypt reverses encryption
func (e *EncryptionBackend) decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, WithContext(ErrInvalidData, map[string]interface{}{
			"reason":     "ciphertext too short",
			"min_length": nonceSize,
			"actual":     len(ciphertext),
		})
	}

	// Extract nonce and ciphertext
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// Helper to create io.Reader from bytes
type bytesReader struct {
	data []byte
	pos  int
}

func newBytesReader(data []byte) *bytesReader {
	return &bytesReader{data: data}
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
