package smarterbase

import (
	"context"
	"io"
)

// Backend defines the interface for different storage implementations
// This allows SmarterBase to work with S3, local filesystem, or any S3-compatible storage
type Backend interface {
	// Object operations
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, data []byte) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)

	// Conditional operations (for optimistic locking)
	// Returns ETag after successful put
	PutIfMatch(ctx context.Context, key string, data []byte, expectedETag string) (string, error)
	GetWithETag(ctx context.Context, key string) (data []byte, etag string, err error)

	// List operations
	List(ctx context.Context, prefix string) ([]string, error)
	ListPaginated(ctx context.Context, prefix string, handler func(keys []string) error) error

	// Streaming (for large files like photos/audio)
	GetStream(ctx context.Context, key string) (io.ReadCloser, error)
	PutStream(ctx context.Context, key string, reader io.Reader, size int64) error

	// Append operations (for JSONL event logs)
	// Appends data to existing key, or creates if not exists
	Append(ctx context.Context, key string, data []byte) error

	// Health check
	Ping(ctx context.Context) error

	// Resource cleanup
	Close() error
}

// BackendConfig holds configuration for any backend
type BackendConfig struct {
	Type       string            // "s3", "filesystem", "minio", etc.
	Bucket     string            // S3 bucket or base directory
	Region     string            // AWS region (S3 only)
	Endpoint   string            // Custom endpoint (for S3-compatible services)
	PathPrefix string            // Optional prefix for all keys
	Options    map[string]string // Backend-specific options
}

// Validate checks if the BackendConfig is valid
func (c BackendConfig) Validate() error {
	if c.Type == "" {
		return WithContext(ErrInvalidConfig, map[string]interface{}{
			"field":  "Type",
			"reason": "backend type is required",
		})
	}
	if c.Bucket == "" {
		return WithContext(ErrInvalidConfig, map[string]interface{}{
			"field":  "Bucket",
			"reason": "bucket/base path is required",
		})
	}

	// Type-specific validation
	switch c.Type {
	case "s3", "minio":
		if c.Region == "" && c.Endpoint == "" {
			return WithContext(ErrInvalidConfig, map[string]interface{}{
				"field":  "Region/Endpoint",
				"reason": "S3 backend requires either Region or Endpoint",
			})
		}
	case "filesystem":
		// No additional validation needed
	default:
		return WithContext(ErrInvalidConfig, map[string]interface{}{
			"field":  "Type",
			"value":  c.Type,
			"reason": "unknown backend type",
		})
	}

	return nil
}
