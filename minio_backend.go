package smarterbase

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/redis/go-redis/v9"
)

// MinIOConfig contains MinIO-specific configuration
type MinIOConfig struct {
	Endpoint        string // e.g., "localhost:9000" or "minio.example.com"
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool   // Whether to use HTTPS (default: false for localhost)
	Bucket          string
}

// NewMinIOBackend creates a new MinIO backend
// MinIO is S3-compatible, so this wraps S3Backend with MinIO-specific configuration
func NewMinIOBackend(cfg MinIOConfig) (Backend, error) {
	scheme := "http"
	if cfg.UseSSL {
		scheme = "https"
	}
	endpoint := fmt.Sprintf("%s://%s", scheme, cfg.Endpoint)

	// Create S3 client configured for MinIO
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint),
		Region:       "us-east-1", // MinIO doesn't enforce regions, but SDK requires it
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		UsePathStyle: true, // MinIO uses path-style addressing: http://host/bucket/key
	})

	return NewS3Backend(client, cfg.Bucket), nil
}

// NewMinIOBackendWithRedisLock creates a MinIO backend with distributed locking
func NewMinIOBackendWithRedisLock(cfg MinIOConfig, redisClient *redis.Client) (Backend, error) {
	scheme := "http"
	if cfg.UseSSL {
		scheme = "https"
	}
	endpoint := fmt.Sprintf("%s://%s", scheme, cfg.Endpoint)

	// Create S3 client configured for MinIO
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint),
		Region:       "us-east-1",
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		UsePathStyle: true,
	})

	return NewS3BackendWithRedisLock(client, cfg.Bucket, redisClient), nil
}

// Example usage:
//
//	// Start MinIO with docker:
//	// docker run -p 9000:9000 -p 9001:9001 \
//	//   -e "MINIO_ROOT_USER=minioadmin" \
//	//   -e "MINIO_ROOT_PASSWORD=minioadmin" \
//	//   minio/minio server /data --console-address ":9001"
//
//	backend, err := smarterbase.NewMinIOBackend(smarterbase.MinIOConfig{
//	    Endpoint:        "localhost:9000",
//	    AccessKeyID:     "minioadmin",
//	    SecretAccessKey: "minioadmin",
//	    UseSSL:          false,
//	    Bucket:          "my-bucket",
//	})
//
//	store := smarterbase.NewStore(backend)
//
// MinIO advantages over S3:
// - Self-hosted (no AWS dependency)
// - No egress costs
// - Identical S3 API (drop-in replacement)
// - Great for development/testing
// - Multi-cloud portability
//
// When to use MinIO vs S3:
// - Development: MinIO (faster, free, local)
// - Production (low volume): MinIO (cost savings)
// - Production (high volume): S3 (managed, global CDN)
// - Hybrid cloud: MinIO (data sovereignty)
