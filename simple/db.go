package simple

import (
	"context"
	"fmt"
	"os"

	"github.com/adrianmcphee/smarterbase"
	"github.com/redis/go-redis/v9"
)

// DB is the simple API entry point.
// It wraps the Core API components with sensible defaults.
//
// Example:
//
//	db, err := simple.Connect()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Close()
type DB struct {
	store        *smarterbase.Store
	backend      smarterbase.Backend
	redisClient  *redis.Client
	redisIndexer *smarterbase.RedisIndexer
	lock         *smarterbase.DistributedLock
	indexManager *smarterbase.IndexManager
}

// Option is a functional option for configuring DB.
type Option func(*DB) error

// Connect creates a new DB with auto-detected configuration.
//
// Environment variables:
//   - DATA_PATH: Filesystem backend path (default: "./data")
//   - AWS_BUCKET: S3 bucket name (enables S3 backend)
//   - REDIS_ADDR: Redis address (default: "localhost:6379")
//
// Example:
//
//	db, err := simple.Connect()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Close()
func Connect(opts ...Option) (*DB, error) {
	db := &DB{}

	// Detect backend from environment
	backend, err := detectBackend()
	if err != nil {
		return nil, fmt.Errorf("failed to detect backend: %w", err)
	}
	db.backend = backend

	// Create store
	db.store = smarterbase.NewStore(backend)

	// Setup Redis if available
	// Redis is optional - continue without Redis features if setup fails
	if err := db.setupRedis(); err != nil {
		// Redis features will be disabled - this is expected behavior
	}

	// Create index manager
	db.indexManager = smarterbase.NewIndexManager(db.store)
	if db.redisIndexer != nil {
		db.indexManager.WithRedisIndexer(db.redisIndexer)
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(db); err != nil {
			if closeErr := db.Close(); closeErr != nil {
				// Best-effort cleanup failed - ignore
			}
			return nil, err
		}
	}

	return db, nil
}

// MustConnect is like Connect but panics on error.
// Use this for demos, prototypes, and when failure should crash the app.
//
// Example:
//
//	db := simple.MustConnect()
//	defer db.Close()
func MustConnect(opts ...Option) *DB {
	db, err := Connect(opts...)
	if err != nil {
		panic(fmt.Sprintf("simple.MustConnect failed: %v", err))
	}
	return db
}

// Close closes the database and all underlying resources.
func (db *DB) Close() error {
	var errs []error

	if db.backend != nil {
		if err := db.backend.Close(); err != nil {
			errs = append(errs, fmt.Errorf("backend close: %w", err))
		}
	}

	if db.redisClient != nil {
		if err := db.redisClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("redis close: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}

	return nil
}

// Store returns the underlying Core API store.
// Use this to drop down to the Core API when needed.
//
// Example:
//
//	store := db.Store()
//	store.Query("users/").FilterJSON(...).All(ctx, &users)
func (db *DB) Store() *smarterbase.Store {
	return db.store
}

// RedisIndexer returns the underlying Redis indexer.
// Use this for advanced index operations.
func (db *DB) RedisIndexer() *smarterbase.RedisIndexer {
	return db.redisIndexer
}

// Lock returns the distributed lock manager.
// Use this for custom atomic operations.
func (db *DB) Lock() *smarterbase.DistributedLock {
	return db.lock
}

// IndexManager returns the index manager.
// Use this for advanced index coordination.
func (db *DB) IndexManager() *smarterbase.IndexManager {
	return db.indexManager
}

// setupRedis initializes Redis components.
func (db *DB) setupRedis() error {
	ctx := context.Background()

	// Create Redis client
	db.redisClient = redis.NewClient(smarterbase.RedisOptions())

	// Test connection
	if err := db.redisClient.Ping(ctx).Err(); err != nil {
		// Redis not available - disable Redis features
		db.redisClient = nil
		return fmt.Errorf("redis not available: %w", err)
	}

	// Create Redis indexer
	db.redisIndexer = smarterbase.NewRedisIndexer(db.redisClient)

	// Create distributed lock
	db.lock = smarterbase.NewDistributedLock(db.redisClient, "smarterbase")

	return nil
}

// detectBackend auto-detects the appropriate backend from environment.
func detectBackend() (smarterbase.Backend, error) {
	// Check for S3 bucket
	if bucket := os.Getenv("AWS_BUCKET"); bucket != "" {
		return nil, fmt.Errorf("S3 backend auto-detection not implemented yet - use Core API")
	}

	// Default to filesystem
	dataPath := os.Getenv("DATA_PATH")
	if dataPath == "" {
		dataPath = "./data"
	}

	return smarterbase.NewFilesystemBackend(dataPath), nil
}

// Functional options

// WithBackend sets a custom backend.
func WithBackend(backend smarterbase.Backend) Option {
	return func(db *DB) error {
		if db.backend != nil {
			if err := db.backend.Close(); err != nil {
				// Best-effort cleanup failed - ignore
			}
		}
		db.backend = backend
		db.store = smarterbase.NewStore(backend)

		// Recreate index manager with new store
		db.indexManager = smarterbase.NewIndexManager(db.store)
		if db.redisIndexer != nil {
			db.indexManager.WithRedisIndexer(db.redisIndexer)
		}

		return nil
	}
}

// WithRedis sets a custom Redis client.
func WithRedis(client *redis.Client) Option {
	return func(db *DB) error {
		db.redisClient = client
		db.redisIndexer = smarterbase.NewRedisIndexer(client)
		db.lock = smarterbase.NewDistributedLock(client, "smarterbase")
		if db.indexManager != nil {
			db.indexManager.WithRedisIndexer(db.redisIndexer)
		}
		return nil
	}
}
