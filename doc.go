// Package smarterbase provides database-like functionality using Redis (for indexes) + S3/GCS (for storage)
// instead of traditional databases, offering 85% cost savings while maintaining high durability and availability.
//
// # Overview
//
// SmarterBase turns existing infrastructure (Redis for caching, S3 for files) into a queryable,
// transactional document store with zero database operations overhead. It provides:
//
//   - Secondary indexes with O(1) lookups via Redis
//   - Query interface for filtering, sorting, and pagination
//   - Optimistic transactions with rollback
//   - Distributed locking for race-free updates
//   - Batch operations for parallel reads/writes
//   - Schema versioning and migrations
//   - Boilerplate reduction helpers (85% less code for common patterns)
//   - Full observability (Prometheus metrics + structured logging)
//
// # Quick Start
//
// Basic usage with filesystem backend (development):
//
//	backend := smarterbase.NewFilesystemBackend("./data")
//	store := smarterbase.NewStore(backend)
//	ctx := context.Background()
//
//	// Create
//	user := &User{ID: smarterbase.NewID(), Email: "alice@example.com"}
//	store.PutJSON(ctx, "users/"+user.ID, user)
//
//	// Read
//	var retrieved User
//	store.GetJSON(ctx, "users/"+user.ID, &retrieved)
//
// Production setup with S3, Redis locking, and encryption:
//
//	// Initialize S3 and Redis
//	s3Client := s3.NewFromConfig(cfg)
//	redisClient := redis.NewClient(smarterbase.RedisOptions())
//
//	// Create production-safe backend with distributed locking
//	backend := smarterbase.NewS3BackendWithRedisLock(s3Client, "my-bucket", redisClient)
//
//	// Add encryption
//	encKey := loadFromSecretsManager() // 32-byte key
//	encBackend, _ := smarterbase.NewEncryptionBackend(backend, encKey)
//
//	// Add observability
//	logger, _ := smarterbase.NewProductionZapLogger()
//	metrics := smarterbase.NewPrometheusMetrics(prometheus.DefaultRegisterer)
//	store := smarterbase.NewStoreWithObservability(encBackend, logger, metrics)
//
// # Core Concepts
//
// Backend: Storage abstraction layer supporting S3, GCS, filesystem, and MinIO.
// All data operations go through the Backend interface for portability.
//
// Store: High-level API for JSON operations, queries, transactions, and batch operations.
// The Store wraps a Backend and provides convenience methods.
//
// IndexManager: Coordinates automatic index updates across file-based and Redis indexes
// when creating, updating, or deleting entities.
//
// Redis Indexing: Fast O(1) lookups for secondary indexes (e.g., email -> user_id, user_id -> [order_ids]).
// Enables queries without scanning all objects.
//
// Distributed Locking: Redis-based locks prevent race conditions in multi-writer scenarios.
// Critical for S3 backends where native locking is unavailable.
//
// Schema Versioning: Optional migration system for evolving JSON schemas over time without downtime.
// Migrations run automatically on read when version mismatches are detected.
//
// # Indexing and Queries
//
// Register indexes for fast lookups:
//
//	redisIndexer := smarterbase.NewRedisIndexer(redisClient)
//
//	// Multi-value index (1:N): user_id -> [order1, order2, ...]
//	redisIndexer.RegisterMultiValueIndex("orders", "user_id", func(data []byte) (string, string, error) {
//	    var order Order
//	    json.Unmarshal(data, &order)
//	    return order.ID, order.UserID, nil
//	})
//
//	// Create IndexManager for automatic updates
//	indexManager := smarterbase.NewIndexManager(store).WithRedisIndexer(redisIndexer)
//
//	// Create with automatic indexing
//	order := &Order{ID: smarterbase.NewID(), UserID: "user-123"}
//	indexManager.Create(ctx, "orders/"+order.ID, order)
//
//	// Query by index - O(1) lookup
//	orderIDs, _ := redisIndexer.QueryMultiValueIndex(ctx, "orders", "user_id", "user-123")
//
// Query builder for filtering and sorting:
//
//	var users []*User
//	store.Query("users/").
//	    FilterJSON(func(obj map[string]interface{}) bool {
//	        return obj["active"].(bool)
//	    }).
//	    SortByField("created_at", false).
//	    Limit(50).
//	    All(ctx, &users)
//
// # Schema Versioning and Migrations
//
// Evolve schemas without downtime:
//
//	// Version 0 (original schema)
//	type User struct {
//	    ID    string `json:"id"`
//	    Name  string `json:"name"`
//	}
//
//	// Version 1 (evolved schema)
//	type User struct {
//	    V         int    `json:"_v"`          // Add version field
//	    ID        string `json:"id"`
//	    FirstName string `json:"first_name"`  // Split from name
//	    LastName  string `json:"last_name"`   // Split from name
//	}
//
//	// Register migration at app startup
//	func init() {
//	    smarterbase.Migrate("User").From(0).To(1).
//	        Split("name", " ", "first_name", "last_name")
//	}
//
//	// Old data migrates automatically on read
//	var user User
//	user.V = 1  // Set expected version
//	store.GetJSON(ctx, "users/123", &user)  // Migration happens here
//
// See ADR-0001 (docs/adr/0001-schema-versioning-and-migrations.md) for design rationale
// and migration patterns.
//
// # Atomic Updates and Distributed Locking
//
// For critical operations requiring true isolation (financial transactions, inventory updates):
//
//	lock := smarterbase.NewDistributedLock(redisClient, "smarterbase")
//
//	err := smarterbase.WithAtomicUpdate(ctx, store, lock, "accounts/123", 10*time.Second,
//	    func(ctx context.Context) error {
//	        var account Account
//	        store.GetJSON(ctx, "accounts/123", &account)
//	        account.Balance += 100
//	        store.PutJSON(ctx, "accounts/123", &account)
//	        return nil
//	    })
//
// For non-critical updates where eventual consistency is acceptable:
//
//	err := store.WithTransaction(ctx, func(tx *smarterbase.OptimisticTransaction) error {
//	    var user User
//	    tx.Get(ctx, "users/123", &user)
//	    user.LastSeen = time.Now()
//	    tx.Put("users/123", user)
//	    return nil
//	})
//
// Note: WithTransaction provides optimistic locking (NOT ACID). Use WithAtomicUpdate
// with distributed locks for operations requiring true isolation.
//
// # Batch Operations
//
// Efficient parallel operations:
//
//	// Type-safe batch read (recommended)
//	keys := []string{"users/1.json", "users/2.json", "users/3.json"}
//	users, err := smarterbase.BatchGet[User](ctx, store, keys)
//
//	// Batch write
//	items := map[string]interface{}{
//	    "users/1": &User{ID: "1", Email: "user1@example.com"},
//	    "users/2": &User{ID: "2", Email: "user2@example.com"},
//	}
//	results := store.BatchPutJSON(ctx, items)
//
// # Helper Functions
//
// The package provides several helper functions for common patterns:
//
// BatchGet[T] - Type-safe batch reads with automatic unmarshaling
//
// BatchGetWithErrors[T] - Like BatchGet but returns errors per-key
//
// GetByIndex[T] - Fetch a single entity by index value
//
// QueryIndexTyped[T] - Query an index and return typed results
//
// # RedisOptions() - Production-ready Redis configuration from environment variables
//
// See ADR-0005 (docs/adr/0005-core-api-helpers-guidance.md) for usage guidance.
//
// # Boilerplate Reduction Helpers (ADR-0006)
//
// Three focused helpers eliminate 85-90% of repetitive patterns:
//
// QueryWithFallback[T] - Redis → scan fallback with automatic profiling (50→6 lines):
//
//	admins, err := smarterbase.QueryWithFallback[User](
//	    ctx, store, redisIndexer,
//	    "users", "role", "admin",         // Redis index lookup
//	    "users/",                          // Fallback scan prefix
//	    func(u *User) bool { return u.Role == "admin" },  // Fallback filter
//	)
//
// UpdateWithIndexes - Atomic update with coordinated index updates:
//
//	err := smarterbase.UpdateWithIndexes(
//	    ctx, store, redisIndexer,
//	    "users/user-123.json", user,
//	    []smarterbase.IndexUpdate{
//	        {EntityType: "users", IndexField: "email", OldValue: old, NewValue: new},
//	    },
//	)
//
// BatchGetWithFilter[T] - Load and filter in one call:
//
//	active, err := smarterbase.BatchGetWithFilter[User](
//	    ctx, store, keys,
//	    func(u *User) bool { return u.Active },
//	)
//
// See ADR-0006 (docs/adr/0006-collection-api.md) and examples/production-patterns/ for details.
//
// # Critical Gotchas
//
// 1. S3 Race Conditions: Always use S3BackendWithRedisLock for production multi-writer scenarios.
// Plain S3Backend has a race window in PutIfMatch operations.
//
// 2. Transactions Are NOT ACID: WithTransaction() does NOT provide isolation. Another process
// can modify data during the transaction. Use WithAtomicUpdate() + distributed locks for
// critical operations.
//
// 3. Memory Usage: Query.All() loads everything into memory. Use Each() or pagination for
// large datasets.
//
// 4. Index Drift: Enable IndexHealthMonitor to auto-detect and repair stale Redis indexes.
//
// 5. S3 Latency: Base latency is 50-100ms. Add caching for hot data if sub-millisecond
// response times are required.
//
// # Storage Backends
//
// Filesystem (development):
//
//	backend := smarterbase.NewFilesystemBackend("./storage")
//
// S3 (production - recommended):
//
//	backend := smarterbase.NewS3BackendWithRedisLock(s3Client, "my-bucket", redisClient)
//
// Google Cloud Storage:
//
//	backend := smarterbase.NewGCSBackend(ctx, smarterbase.GCSConfig{
//	    ProjectID: "my-project",
//	    Bucket:    "my-bucket",
//	})
//
// MinIO / S3-compatible:
//
//	backend := smarterbase.NewMinIOBackend(smarterbase.MinIOConfig{
//	    Endpoint:  "localhost:9000",
//	    Bucket:    "my-bucket",
//	    AccessKey: "minioadmin",
//	    SecretKey: "minioadmin",
//	})
//
// Encryption wrapper (any backend):
//
//	encBackend, _ := smarterbase.NewEncryptionBackend(backend, encryptionKey)
//
// # When to Use SmarterBase
//
// Perfect for:
//   - User management (profiles, preferences, settings)
//   - Configuration storage (app configs, feature flags)
//   - Content management (blog posts, articles, pages)
//   - Order/invoice storage (e-commerce transactions)
//   - Metadata catalogs (file metadata, asset tracking)
//   - Event logs (audit trails, activity logs)
//   - API caching (long-lived cached responses)
//
// Not suitable for:
//   - Complex JOINs across multiple entity types
//   - Real-time aggregations (SUM, COUNT, GROUP BY)
//   - Strict ACID transactions
//   - Sub-millisecond response times at scale
//   - Full-text search (use Elasticsearch)
//   - Graph queries (use Neo4j)
//   - Time-series analytics (use TimescaleDB)
//
// # Observability
//
// Metrics (Prometheus):
//
//	metrics := smarterbase.NewPrometheusMetrics(prometheus.DefaultRegisterer)
//	metrics.RegisterAll()
//	store := smarterbase.NewStoreWithObservability(backend, logger, metrics)
//
// Logging (Zap structured logging):
//
//	logger, _ := smarterbase.NewProductionZapLogger()
//	store := smarterbase.NewStoreWithObservability(backend, logger, metrics)
//
// Index health monitoring with auto-repair:
//
//	monitor := smarterbase.NewIndexHealthMonitor(store, redisIndexer)
//	monitor.Start(ctx)  // Checks every 5 minutes, auto-repairs drift >5%
//	defer monitor.Stop()
//
// # Documentation and Examples
//
// Package documentation:
//   - README.md - Comprehensive guide with examples
//   - DATASHEET.md - Technical specifications and API reference
//   - CHANGELOG.md - Version history and release notes
//
// Architecture Decision Records (ADRs):
//   - docs/adr/0001-schema-versioning-and-migrations.md - Migration system design
//   - docs/adr/0002-redis-configuration-ergonomics.md - Redis config patterns
//   - docs/adr/0003-simple-api-layer.md - Simple API design (optional high-level API)
//   - docs/adr/0004-simple-api-versioning.md - Simple API versioning strategy
//   - docs/adr/0005-core-api-helpers-guidance.md - When to use helper functions
//   - docs/adr/0006-collection-api.md - Boilerplate reduction helpers (QueryWithFallback, etc.)
//
// Working examples:
//   - examples/simple/ - Progressive tutorials (quickstart to versioning)
//   - examples/schema-migrations/ - Schema evolution patterns
//   - examples/user-management/ - CRUD with Redis indexing
//   - examples/ecommerce-orders/ - Order management with atomic updates
//   - examples/production-patterns/ - Complete production setup
//   - examples/multi-tenant-config/ - Multi-tenant scenarios
//   - examples/metrics-dashboard/ - Observability integration
//   - examples/event-logging/ - JSONL append-only logs
//
// AI Assistant Context:
//   - .ai-context - Quick reference for LLMs working with this codebase
//
// # Performance Characteristics
//
// Latency (typical):
//   - Filesystem Get: 1-3ms
//   - S3 Get: 50-80ms
//   - Put with indexes: +5-10ms (Redis updates)
//   - Distributed lock: +2-5ms (no contention)
//
// Throughput:
//   - Filesystem: 10,000+ ops/sec (with striped locks)
//   - S3: Up to 3,500 PUT/sec per prefix (AWS limit)
//
// Scalability:
//   - Tested with millions of objects
//   - Redis can handle billions of index entries
//   - S3 scales infinitely
//
// # Repository and License
//
// Repository: https://github.com/adrianmcphee/smarterbase
//
// License: MIT License - See LICENSE file for details
//
// Issues and feature requests: https://github.com/adrianmcphee/smarterbase/issues
//
// Security: See SECURITY.md for reporting vulnerabilities
//
// Contributing: See CONTRIBUTING.md for development guidelines
package smarterbase
