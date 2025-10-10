package smarterbase

import (
	"context"
	"fmt"
	"io"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// GCSBackend implements Backend using Google Cloud Storage
type GCSBackend struct {
	client *storage.Client
	bucket string
}

// GCSConfig contains GCS-specific configuration
type GCSConfig struct {
	ProjectID       string
	Bucket          string
	CredentialsFile string // Path to service account JSON file (optional, uses ADC if empty)
}

// NewGCSBackend creates a new GCS backend
func NewGCSBackend(ctx context.Context, cfg GCSConfig) (Backend, error) {
	var opts []option.ClientOption
	if cfg.CredentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(cfg.CredentialsFile))
	}
	// If no credentials file, uses Application Default Credentials (ADC)

	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}

	return &GCSBackend{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

func (b *GCSBackend) Get(ctx context.Context, key string) ([]byte, error) {
	obj := b.client.Bucket(b.bucket).Object(key)
	reader, err := obj.NewReader(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

func (b *GCSBackend) Put(ctx context.Context, key string, data []byte) error {
	obj := b.client.Bucket(b.bucket).Object(key)
	writer := obj.NewWriter(ctx)
	defer writer.Close()

	if _, err := writer.Write(data); err != nil {
		return err
	}

	return writer.Close()
}

func (b *GCSBackend) Delete(ctx context.Context, key string) error {
	obj := b.client.Bucket(b.bucket).Object(key)
	return obj.Delete(ctx)
}

func (b *GCSBackend) Exists(ctx context.Context, key string) (bool, error) {
	obj := b.client.Bucket(b.bucket).Object(key)
	_, err := obj.Attrs(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (b *GCSBackend) GetWithETag(ctx context.Context, key string) ([]byte, string, error) {
	obj := b.client.Bucket(b.bucket).Object(key)

	// Get attributes first to get generation number (GCS's version of ETag)
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		return nil, "", err
	}

	reader, err := obj.NewReader(ctx)
	if err != nil {
		return nil, "", err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, "", err
	}

	// Use generation as ETag (GCS's object versioning)
	etag := fmt.Sprintf("%d", attrs.Generation)
	return data, etag, nil
}

// PutIfMatch provides optimistic locking using GCS preconditions
// Unlike S3, GCS supports true conditional writes via generation matching!
func (b *GCSBackend) PutIfMatch(ctx context.Context, key string, data []byte, expectedETag string) (string, error) {
	obj := b.client.Bucket(b.bucket).Object(key)

	// Parse expected generation from ETag
	var expectedGen int64
	if expectedETag != "" {
		if _, err := fmt.Sscanf(expectedETag, "%d", &expectedGen); err != nil {
			return "", fmt.Errorf("invalid ETag format: %w", err)
		}
	}

	// Create writer with precondition
	writer := obj.If(storage.Conditions{GenerationMatch: expectedGen}).NewWriter(ctx)
	defer writer.Close()

	if _, err := writer.Write(data); err != nil {
		return "", err
	}

	if err := writer.Close(); err != nil {
		// Check if it was a precondition failure
		if strings.Contains(err.Error(), "conditionNotMet") || strings.Contains(err.Error(), "precondition") {
			return "", WithContext(ErrConflict, map[string]interface{}{
				"expected": expectedETag,
			})
		}
		return "", err
	}

	// Get new generation
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		return "", err
	}

	newETag := fmt.Sprintf("%d", attrs.Generation)
	return newETag, nil
}

func (b *GCSBackend) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string

	query := &storage.Query{Prefix: prefix}
	it := b.client.Bucket(b.bucket).Objects(ctx, query)

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		keys = append(keys, attrs.Name)
	}

	return keys, nil
}

func (b *GCSBackend) ListPaginated(ctx context.Context, prefix string, handler func(keys []string) error) error {
	query := &storage.Query{Prefix: prefix}
	it := b.client.Bucket(b.bucket).Objects(ctx, query)

	var batch []string
	batchSize := 1000

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			// Handle last batch
			if len(batch) > 0 {
				if err := handler(batch); err != nil {
					return err
				}
			}
			break
		}
		if err != nil {
			return err
		}

		batch = append(batch, attrs.Name)
		if len(batch) >= batchSize {
			if err := handler(batch); err != nil {
				return err
			}
			batch = batch[:0] // Reset batch
		}
	}

	return nil
}

func (b *GCSBackend) GetStream(ctx context.Context, key string) (io.ReadCloser, error) {
	obj := b.client.Bucket(b.bucket).Object(key)
	return obj.NewReader(ctx)
}

func (b *GCSBackend) PutStream(ctx context.Context, key string, reader io.Reader, size int64) error {
	obj := b.client.Bucket(b.bucket).Object(key)
	writer := obj.NewWriter(ctx)
	defer writer.Close()

	if _, err := io.Copy(writer, reader); err != nil {
		return err
	}

	return writer.Close()
}

// Append appends data to an existing GCS object using read-modify-write.
// GCS doesn't support true append operations, so this reads, combines, and writes back.
func (b *GCSBackend) Append(ctx context.Context, key string, data []byte) error {
	// Read existing content (if exists)
	existing, err := b.Get(ctx, key)
	if err != nil && err != ErrNotFound {
		return fmt.Errorf("failed to read existing object: %w", err)
	}

	// Append new data
	combined := append(existing, data...)

	// Write back
	return b.Put(ctx, key, combined)
}

func (b *GCSBackend) Ping(ctx context.Context) error {
	// Check bucket access
	bucket := b.client.Bucket(b.bucket)
	_, err := bucket.Attrs(ctx)
	return err
}

func (b *GCSBackend) Close() error {
	return b.client.Close()
}

// Example usage:
//
//	// Using Application Default Credentials (for GCE, Cloud Run, etc.)
//	backend, err := smarterbase.NewGCSBackend(ctx, smarterbase.GCSConfig{
//	    ProjectID: "my-project",
//	    Bucket:    "my-bucket",
//	})
//
//	// Using service account JSON file
//	backend, err := smarterbase.NewGCSBackend(ctx, smarterbase.GCSConfig{
//	    ProjectID:       "my-project",
//	    Bucket:          "my-bucket",
//	    CredentialsFile: "/path/to/service-account.json",
//	})
//
// GCS advantages over S3:
// - True atomic conditional writes (no race conditions!)
// - Stronger consistency guarantees
// - Object versioning built-in
// - Better integration with GCP services
// - No data transfer costs within same region
//
// When to use GCS vs S3:
// - GCP ecosystem: GCS (better integration)
// - True ACID needs: GCS (atomic conditional writes)
// - AWS ecosystem: S3 (better integration)
// - Multi-cloud: MinIO (portable)
