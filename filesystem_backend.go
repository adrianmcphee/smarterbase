package smarterbase

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// FilesystemBackend implements Backend using local filesystem
type FilesystemBackend struct {
	basePath string
	locks    *StripedLocks // Fine-grained locking per key
}

// NewFilesystemBackend creates a new filesystem backend with 32 lock stripes
func NewFilesystemBackend(basePath string) *FilesystemBackend {
	return &FilesystemBackend{
		basePath: basePath,
		locks:    NewStripedLocks(32),
	}
}

// NewFilesystemBackendWithStripes creates a filesystem backend with custom stripe count
func NewFilesystemBackendWithStripes(basePath string, stripes int) *FilesystemBackend {
	return &FilesystemBackend{
		basePath: basePath,
		locks:    NewStripedLocks(stripes),
	}
}

func (b *FilesystemBackend) getPath(key string) string {
	return filepath.Join(b.basePath, key)
}

// Get retrieves data for the given key from the filesystem
func (b *FilesystemBackend) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := os.ReadFile(b.getPath(key))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		if os.IsPermission(err) {
			return nil, ErrUnauthorized
		}
		return nil, err
	}
	return data, nil
}

// Put stores data for the given key to the filesystem
func (b *FilesystemBackend) Put(ctx context.Context, key string, data []byte) error {
	path := b.getPath(key)
	if err := os.MkdirAll(filepath.Dir(path), DefaultDirPermissions); err != nil {
		return err
	}
	return os.WriteFile(path, data, DefaultFilePermissions)
}

// Delete removes the object at the given key from the filesystem
func (b *FilesystemBackend) Delete(ctx context.Context, key string) error {
	err := os.Remove(b.getPath(key))
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		if os.IsPermission(err) {
			return ErrUnauthorized
		}
		return err
	}
	return nil
}

// Exists checks if an object exists at the given key
func (b *FilesystemBackend) Exists(ctx context.Context, key string) (bool, error) {
	_, err := os.Stat(b.getPath(key))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetWithETag retrieves data and its ETag for optimistic locking
func (b *FilesystemBackend) GetWithETag(ctx context.Context, key string) ([]byte, string, error) {
	data, err := b.Get(ctx, key)
	if err != nil {
		return nil, "", err
	}

	// Generate ETag from MD5 hash
	hasher := md5.New()
	hasher.Write(data)
	etag := hex.EncodeToString(hasher.Sum(nil))

	return data, etag, nil
}

// PutIfMatch performs a conditional put operation using optimistic locking
func (b *FilesystemBackend) PutIfMatch(ctx context.Context, key string, data []byte, expectedETag string) (string, error) {
	// Lock this specific key to ensure atomic check-and-write
	unlock := b.locks.Lock(key)
	defer unlock()

	if expectedETag != "" {
		_, currentETag, err := b.GetWithETag(ctx, key)
		if err != nil {
			return "", err
		}

		if currentETag != expectedETag {
			return "", WithContext(ErrConflict, map[string]interface{}{
				"expected": expectedETag,
				"actual":   currentETag,
			})
		}
	}

	if err := b.Put(ctx, key, data); err != nil {
		return "", err
	}

	// Generate new ETag
	hasher := md5.New()
	hasher.Write(data)
	newETag := hex.EncodeToString(hasher.Sum(nil))

	return newETag, nil
}

// List returns all keys with the given prefix
func (b *FilesystemBackend) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	searchPath := b.getPath(prefix)

	// Check if path exists
	if _, err := os.Stat(searchPath); os.IsNotExist(err) {
		// Return empty list if prefix directory doesn't exist
		return keys, nil
	}

	err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, err := filepath.Rel(b.basePath, path)
			if err != nil {
				return err
			}
			// Convert to forward slashes for consistency with S3
			relPath = filepath.ToSlash(relPath)
			keys = append(keys, relPath)
		}
		return nil
	})

	return keys, err
}

// ListPaginated streams keys with the given prefix in batches
func (b *FilesystemBackend) ListPaginated(ctx context.Context, prefix string, handler func(keys []string) error) error {
	searchPath := b.getPath(prefix)

	// Check if path exists
	if _, err := os.Stat(searchPath); os.IsNotExist(err) {
		// Return no error if prefix directory doesn't exist
		return nil
	}

	batch := make([]string, 0, DefaultListPaginatedSize)

	err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, err := filepath.Rel(b.basePath, path)
			if err != nil {
				return err
			}
			relPath = filepath.ToSlash(relPath)
			batch = append(batch, relPath)

			if len(batch) >= DefaultListPaginatedSize {
				if err := handler(batch); err != nil {
					return err
				}
				batch = batch[:0]
			}
		}
		return nil
	})

	// Handle remaining items
	if len(batch) > 0 && err == nil {
		err = handler(batch)
	}

	return err
}

// GetStream returns a reader for streaming large objects
func (b *FilesystemBackend) GetStream(ctx context.Context, key string) (io.ReadCloser, error) {
	return os.Open(b.getPath(key))
}

// PutStream writes large objects from a stream
func (b *FilesystemBackend) PutStream(ctx context.Context, key string, reader io.Reader, size int64) error {
	path := b.getPath(key)
	if err := os.MkdirAll(filepath.Dir(path), DefaultDirPermissions); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close() //nolint:errcheck // Deferred close, error already captured
	}()

	_, err = io.Copy(file, reader)
	return err
}

// Append appends data to an existing key or creates it if it doesn't exist
func (b *FilesystemBackend) Append(ctx context.Context, key string, data []byte) error {
	path := b.getPath(key)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), DefaultDirPermissions); err != nil {
		return err
	}

	// Lock this specific key for thread-safe append
	unlock := b.locks.Lock(key)
	defer unlock()

	// Open file in append mode (creates if not exists)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, DefaultFilePermissions)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close() //nolint:errcheck // Deferred close, error already captured
	}()

	// Append data
	_, err = file.Write(data)
	return err
}

// Ping checks if the backend is accessible and operational
func (b *FilesystemBackend) Ping(ctx context.Context) error {
	// Check if base directory exists and is writable
	info, err := os.Stat(b.basePath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("base path is not a directory: %s", b.basePath)
	}

	// Try to create a temp file to verify write access
	testFile := filepath.Join(b.basePath, ".health_check")
	if err := os.WriteFile(testFile, []byte("ok"), DefaultFilePermissions); err != nil {
		return fmt.Errorf("cannot write to base path: %w", err)
	}
	_ = os.Remove(testFile) //nolint:errcheck // Cleanup operation, safe to ignore

	return nil
}

// Close releases any resources held by the filesystem backend
func (b *FilesystemBackend) Close() error {
	// Filesystem doesn't need cleanup, but implement for interface compliance
	return nil
}
