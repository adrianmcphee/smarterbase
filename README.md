# SmarterBase

## Skip the Database. Use Redis + S3 Instead.

**You already have Redis and S3. Use them as your database.**

SmarterBase turns **Redis** (fast indexes) + **S3** (durable storage) into a queryable, transactional document store. No PostgreSQL, no MySQL, no MongoDB. No migrations, no backups, no database operations.

**85% cost savings. Zero database complexity.**

[![Go Version](https://img.shields.io/badge/Go-1.18+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Tests](https://github.com/adrianmcphee/smarterbase/workflows/Tests/badge.svg)](https://github.com/adrianmcphee/smarterbase/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/adrianmcphee/smarterbase)](https://goreportcard.com/report/github.com/adrianmcphee/smarterbase)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Coverage](https://img.shields.io/badge/coverage-70%25-brightgreen)](https://github.com/adrianmcphee/smarterbase)

---

## The Core Value Proposition

### Instead of This:
```
Your App → RDS PostgreSQL ($271/month + DBA time)
  ❌ Schema migrations to plan
  ❌ Database servers to patch
  ❌ Backup strategies to test
  ❌ Connection pools to tune
  ❌ Queries to optimize
  ❌ Scaling decisions to make
```

### Do This:
```
Your App → SmarterBase → Redis (indexes) + S3 (storage)
                         $13/month    $23/month

  ✅ Redis you already have (for caching)
  ✅ S3 you already have (for files)
  ✅ SmarterBase coordinates them
  ✅ Zero database operations
  ✅ Total: $36/month (85% savings)
```

---

## How Redis + S3 Becomes a Database

### What SmarterBase Does:

1. **Writes:** Store JSON document to S3 → Update Redis indexes automatically
2. **Reads by ID:** Fetch from S3 (simple GET request)
3. **Reads by index:** Query Redis for ID → Fetch from S3
4. **Locking:** Use Redis distributed locks for race-free updates
5. **Health:** Monitor index drift, auto-repair from S3

### Key Insight:

- **Redis = Speed** (O(1) index lookups, distributed locks)
- **S3 = Truth** (11 9s durability, source of truth for everything)
- **SmarterBase = Glue** (keeps them in sync automatically)

**Redis can fail?** No problem - rebuild indexes from S3.
**S3 can fail?** AWS problem (99.99% SLA) - better than most databases.

## What You Get

### Database Features (Without the Database)
- ✅ **Secondary indexes** - O(1) lookups via Redis Sets
- ✅ **Query interface** - Filter, sort, paginate JSON documents
- ✅ **Transactions** - Optimistic locking with rollback
- ✅ **Distributed locking** - Redis-based coordination across servers
- ✅ **Batch operations** - Parallel bulk reads/writes
- ✅ **Full observability** - Prometheus metrics + structured logging

### Redis Integration (The Performance Layer)
- ✅ **Automatic index updates** - Write to S3 → Redis indexes updated
- ✅ **Multi-value indexes** - `user_id → [order1, order2, ...]` (Redis Sets)
- ✅ **Index health monitoring** - Detect drift, auto-repair from S3
- ✅ **Distributed locks** - Eliminate S3 race conditions
- ✅ **Graceful degradation** - Redis down? Rebuild from S3

### S3 Integration (The Durability Layer)
- ✅ **11 9s durability** - AWS multi-AZ replication
- ✅ **Infinite scale** - No capacity planning needed
- ✅ **Zero backups** - S3 handles durability automatically
- ✅ **Schema-less** - JSON structure, no migrations ever
- ✅ **JSONL support** - Append-only event logs

### What You Skip
- ❌ No database servers to run
- ❌ No schema migrations to plan
- ❌ No backup strategies to implement
- ❌ No connection pools to tune
- ❌ No query performance to optimize
- ❌ No DBA expertise required

## Installation

```bash
go get github.com/adrianmcphee/smarterbase
```

---

## ⚠️ Critical Gotchas (Read This First!)

Before using SmarterBase in production, understand these important limitations:

### 1. **S3 Race Conditions: Use S3BackendWithRedisLock in Production**

**Problem:** Plain `S3Backend` has a race window in `PutIfMatch` operations:
```go
// ❌ UNSAFE for multi-writer production use
backend := smarterbase.NewS3Backend(s3Client, "my-bucket")
// Race condition: HeadObject → another process writes → PutObject overwrites
```

**Solution:** Always use `S3BackendWithRedisLock` for production:
```go
// ✅ SAFE for multi-writer production use
backend := smarterbase.NewS3BackendWithRedisLock(s3Client, "my-bucket", redisClient)
// Distributed locks prevent race conditions
```

Plain `S3Backend` is **only safe for single-writer scenarios** (e.g., batch jobs, development).

---

### 2. **Transactions Are NOT ACID**

**Problem:** `WithTransaction()` does **NOT** provide isolation:
```go
// ⚠️ WARNING: Another process can modify data during this transaction
store.WithTransaction(ctx, func(tx *smarterbase.OptimisticTransaction) error {
    var account Account
    tx.Get(ctx, "accounts/123", &account)
    // ← RACE: Another process can modify account here!
    account.Balance += 100
    tx.Put("accounts/123", account) // May conflict with concurrent update
    return nil
})
```

**Solution:** Use `WithAtomicUpdate()` with distributed locks for critical operations:
```go
// ✅ SAFE: True isolation with distributed lock
lock := smarterbase.NewDistributedLock(redisClient, "smarterbase")
smarterbase.WithAtomicUpdate(ctx, store, lock, "accounts/123", 10*time.Second,
    func(ctx context.Context) error {
        // No other process can modify this account during this function
        var account Account
        store.GetJSON(ctx, "accounts/123", &account)
        account.Balance += 100
        store.PutJSON(ctx, "accounts/123", &account)
        return nil
    })
```

**Use `WithAtomicUpdate()` for:**
- Financial transactions (balances, payments)
- Inventory updates
- Any read-modify-write that must be atomic

**Use `WithTransaction()` only for:**
- Non-critical updates where conflicts are acceptable
- Low-contention scenarios

---

### 3. **Query.All() Loads Everything Into Memory**

**Problem:** Can cause OOM on large datasets:
```go
// ❌ Loads all users into memory at once
var users []*User
store.Query("users/").All(ctx, &users) // OOM risk if millions of users
```

**Solution:** Use streaming or pagination:
```go
// ✅ Process one at a time (memory efficient)
store.Query("users/").Each(ctx, func(key string, data []byte) error {
    var user User
    json.Unmarshal(data, &user)
    processUser(&user)
    return nil
})

// ✅ Or use pagination
store.Query("users/").Offset(0).Limit(100).All(ctx, &users)
```

---

### 4. **Index Drift Can Happen**

**Problem:** Redis indexes can become stale due to:
- Network partitions during writes
- Application crashes mid-update
- Redis failures

**Solution:** Enable index health monitoring (auto-repair by default):
```go
// Self-healing index monitoring with opinionated defaults
monitor := smarterbase.NewIndexHealthMonitor(store, redisIndexer)
monitor.Start(ctx)

// Monitor automatically:
// - Checks every 5 minutes
// - Repairs drift >5%
// - Logs and emits metrics
```

---

### 5. **S3 Has 50-100ms Base Latency**

SmarterBase is **not suitable** for sub-millisecond response requirements. Add caching for hot data:
```go
// Add Redis or in-memory cache for frequently accessed data
cache.Get("users/123") // Check cache first
if notFound {
    store.GetJSON(ctx, "users/123", &user) // Fallback to S3
    cache.Set("users/123", user)
}
```

---

## Architecture: How "No Database" Works

```
┌─────────────────────────────────────────────────────────┐
│                   Your Application                      │
│            (No database drivers needed!)                │
└────────────────┬───────────────────┬────────────────────┘
                 │                   │
         ┌───────▼────────┐  ┌──────▼─────────┐
         │  Redis/Valkey  │  │  S3 / GCS      │
         │  (Indexes)     │  │  (Storage)     │
         │                │  │                │
         │  • Fast O(1)   │  │  • Durable     │
         │    lookups     │  │    (11 9s)     │
         │  • Ephemeral   │  │  • Serverless  │
         │    (rebuild)   │  │  • Managed     │
         └────────────────┘  └────────────────┘
```

**How it works:**
1. **Write:** Store JSON to S3, update Redis indexes automatically
2. **Read by ID:** Fetch directly from S3 (or cache)
3. **Read by index:** Query Redis for ID, then fetch from S3
4. **Redis fails?** Rebuild indexes from S3 (source of truth)
5. **S3 fails?** AWS guarantees 99.99% availability (better than most DBs)

**Why this beats a traditional database:**
- No schema migrations (JSON is schema-less)
- No backup strategies (S3 = 11 9s durability)
- No connection pooling (HTTP-based)
- No query optimization (simple key-value + indexes)
- No scaling decisions (S3 scales infinitely)
- 85% cost savings

## Quick Start

### Basic Operations (No Database Required!)

```go
package main

import (
    "context"
    "github.com/adrianmcphee/smarterbase"
)

type User struct {
    ID    string `json:"id"`
    Email string `json:"email"`
    Name  string `json:"name"`
}

func main() {
    // Create backend (filesystem for development)
    backend := smarterbase.NewFilesystemBackend("./data")
    defer backend.Close()

    // Create store
    store := smarterbase.NewStore(backend)
    ctx := context.Background()

    // Create
    user := &User{
        ID:    smarterbase.NewID(),
        Email: "alice@example.com",
        Name:  "Alice",
    }
    store.PutJSON(ctx, "users/"+user.ID, user)

    // Read
    var retrieved User
    store.GetJSON(ctx, "users/"+user.ID, &retrieved)

    // Update
    retrieved.Name = "Alice Smith"
    store.PutJSON(ctx, "users/"+user.ID, &retrieved)

    // Delete
    store.Delete(ctx, "users/"+user.ID)
}
```

### With Indexing

```go
// Setup Redis client for both locking and indexing
redisClient := redis.NewClient(&redis.Options{Addr: "localhost:6379"})

// ✅ Production-safe: S3 backend with distributed locking
backend := smarterbase.NewS3BackendWithRedisLock(s3Client, "my-bucket", redisClient)
store := smarterbase.NewStore(backend)

// Create Redis indexer
redisIndexer := smarterbase.NewRedisIndexer(redisClient)

// Register index
redisIndexer.RegisterMultiValueIndex("users", "email", func(data []byte) (string, string, error) {
    var user User
    json.Unmarshal(data, &user)
    return user.ID, user.Email, nil
})

// Create with automatic indexing
indexManager := smarterbase.NewIndexManager(store).
    WithRedisIndexer(redisIndexer)

user := &User{ID: smarterbase.NewID(), Email: "bob@example.com"}
indexManager.Create(ctx, "users/"+user.ID, user)

// Query by index - O(1) lookup
userIDs, _ := redisIndexer.QueryMultiValueIndex(ctx, "users", "email", "bob@example.com")
```

### Query Builder

```go
// Find all active users created in the last week
var users []*User
err := store.Query("users/").
    FilterJSON(func(obj map[string]interface{}) bool {
        createdAt, _ := time.Parse(time.RFC3339, obj["created_at"].(string))
        isActive, _ := obj["active"].(bool)
        return isActive && createdAt.After(time.Now().AddDate(0, 0, -7))
    }).
    SortByField("created_at", false).
    Limit(50).
    All(ctx, &users)
```

### Batch Operations

```go
// Batch write
items := map[string]interface{}{
    "users/1": &User{ID: "1", Email: "user1@example.com"},
    "users/2": &User{ID: "2", Email: "user2@example.com"},
    "users/3": &User{ID: "3", Email: "user3@example.com"},
}
results := store.BatchPutJSON(ctx, items)

// Check results
for _, result := range results {
    if result.Error != nil {
        log.Printf("Failed to write %s: %v", result.Key, result.Error)
    }
}
```

### Transactions & Atomic Updates

SmarterBase provides two approaches for coordinating multiple operations:

#### ✅ Atomic Updates (Recommended for Critical Operations)

Use `WithAtomicUpdate()` with distributed locks for operations that require true isolation:

```go
// ✅ SAFE: Fully atomic with distributed lock protection
lock := smarterbase.NewDistributedLock(redisClient, "smarterbase")

err := smarterbase.WithAtomicUpdate(ctx, store, lock, "accounts/123", 10*time.Second,
    func(ctx context.Context) error {
        var account Account
        store.GetJSON(ctx, "accounts/123", &account)

        // ✅ SAFE: No other process can modify account during this function
        account.Balance += 100
        store.PutJSON(ctx, "accounts/123", &account)

        // Can also update related records atomically
        store.PutJSON(ctx, "transactions/"+txnID, &Transaction{
            AccountID: account.ID,
            Amount:    100,
            Timestamp: time.Now(),
        })

        return nil
    })
```

**Use atomic updates for:**
- Financial transactions (account balances, payments)
- Inventory updates
- Counter increments
- Any operation where race conditions would cause data corruption

**Performance characteristics:**
- No contention: +2-5ms latency (lock acquisition overhead)
- Under contention: +10-50ms per retry with exponential backoff
- Automatic retry: 3 attempts with exponential backoff before failure
- Metrics tracked: `smarterbase.lock.contention`, `smarterbase.lock.wait_duration`, `smarterbase.lock.timeout`

#### ⚠️ Optimistic Transactions (Low-Contention Only)

For non-critical updates where eventual consistency is acceptable:

```go
// ⚠️ WARNING: NO ISOLATION - Another process can modify data concurrently
err := store.WithTransaction(ctx, func(tx *smarterbase.OptimisticTransaction) error {
    var user User
    tx.Get(ctx, "users/123", &user)

    // ⚠️ CAUTION: Another process could modify user here
    user.LastSeen = time.Now()
    user.LoginCount++

    tx.Put("users/123", user) // ETag checked on commit
    return nil
})
```

**Limitations:**
- **NOT true ACID transactions** - No isolation between concurrent operations
- **Best-effort rollback** - Rollback may fail, leaving partial writes
- **Low-contention only** - High concurrency causes conflicts
- ETag conflicts will cause transaction to fail and retry

## Storage Backends

### Filesystem (Development)

```go
backend := smarterbase.NewFilesystemBackend("./storage")
```

- Fast local testing
- Easy debugging (inspect JSON files directly)
- No external dependencies

### S3 (Production)

```go
cfg, _ := config.LoadDefaultConfig(ctx)
s3Client := s3.NewFromConfig(cfg)

// Initialize Redis for distributed locking
redisClient := redis.NewClient(&redis.Options{Addr: "localhost:6379"})

// ✅ RECOMMENDED: S3 with Redis distributed locks (prevents race conditions)
backend := smarterbase.NewS3BackendWithRedisLock(s3Client, "my-bucket", redisClient)

// ⚠️ ONLY for single-writer scenarios (batch jobs, development):
// backend := smarterbase.NewS3Backend(s3Client, "my-bucket")
```

- Works with AWS S3, MinIO, DigitalOcean Spaces, Wasabi, Cloudflare R2
- Scalable and durable
- Cost-effective at scale
- **Always use `S3BackendWithRedisLock` for multi-writer production deployments**

### Google Cloud Storage

```go
gcsClient, _ := storage.NewClient(ctx)
backend := smarterbase.NewGCSBackend(gcsClient, "my-bucket")
```

- Native GCS support
- Strong consistency
- Global availability

### Custom Backend

Implement the `Backend` interface:

```go
type Backend interface {
    Get(ctx context.Context, key string) ([]byte, error)
    Put(ctx context.Context, key string, data []byte) error
    Delete(ctx context.Context, key string) error
    List(ctx context.Context, prefix string) ([]string, error)
    // ... more methods
}
```

### Encryption at Rest

Wrap any backend with AES-256-GCM encryption:

```go
// Generate or load 32-byte encryption key
key := make([]byte, 32)
rand.Read(key) // Or load from secrets manager

// Wrap backend with encryption
s3Backend := smarterbase.NewS3Backend(s3Client, "my-bucket")
encryptedBackend, _ := smarterbase.NewEncryptionBackend(s3Backend, key)

store := smarterbase.NewStore(encryptedBackend)
// All data now encrypted before S3 upload, decrypted on retrieval
```

**Features:**
- AES-256-GCM authenticated encryption
- Random nonces for each operation
- Transparent encryption/decryption
- Works with any backend (S3, GCS, Filesystem)

## Indexing

### File-Based Indexes (1:1)

For unique mappings like email → user ID:

```go
indexer := smarterbase.NewIndexer(store)

indexer.RegisterIndex(&smarterbase.IndexSpec{
    Name: "users-by-email",
    KeyFunc: func(data interface{}) (string, error) {
        return data.(*User).Email, nil
    },
})
```

### Redis Indexes (1:N)

For queries like "all orders for user X":

```go
redisIndexer := smarterbase.NewRedisIndexer(redisClient)

redisIndexer.RegisterMultiValueIndex("orders", "user_id", func(data []byte) (string, string, error) {
    var order Order
    json.Unmarshal(data, &order)
    return order.ID, order.UserID, nil
})

// Query - O(1) lookup
orderIDs, _ := redisIndexer.QueryMultiValueIndex(ctx, "orders", "user_id", "user-123")
```

## Reliability Features

### Circuit Breaker

Automatic circuit breaker protection prevents cascading failures when Redis becomes unavailable:

```go
// Circuit breaker is enabled by default in RedisIndexer
redisIndexer := smarterbase.NewRedisIndexer(redisClient)

// Automatically opens after 5 consecutive failures
// Retries after 30 seconds in half-open state
// Fails fast when open (no Redis calls)
```

**States:**
- **Closed**: Normal operation, all requests pass through
- **Open**: Redis failing, requests fail fast without calling Redis (prevents cascading failures)
- **Half-Open**: Testing recovery, limited requests allowed

**Benefits:**
- Prevents application slowdown when Redis is down
- Automatic recovery detection
- Graceful degradation for non-critical operations

## Observability

### Metrics (Prometheus)

```go
metrics := smarterbase.NewPrometheusMetrics(prometheus.DefaultRegisterer)
metrics.RegisterAll()

store := smarterbase.NewStoreWithObservability(backend, logger, metrics)

// Automatically tracks:
// - smarterbase_get_success, smarterbase_get_error
// - smarterbase_put_duration (histogram)
// - smarterbase_query_results (histogram)
```

### Logging

```go
logger, _ := smarterbase.NewProductionZapLogger()
store := smarterbase.NewStoreWithObservability(backend, logger, &smarterbase.NoOpMetrics{})

// All operations logged with structured fields
```

## Advanced Examples

### Complete Production Setup

```go
package main

import (
    "context"
    "log"

    "github.com/adrianmcphee/smarterbase"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/s3"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/redis/go-redis/v9"
)

func main() {
    ctx := context.Background()

    // 1. Initialize S3 backend
    cfg, _ := config.LoadDefaultConfig(ctx)
    s3Client := s3.NewFromConfig(cfg)

    // 2. Initialize Redis for locking and indexing
    redisClient := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })

    // 3. Create S3 backend with Redis distributed locking (production-safe)
    s3Backend := smarterbase.NewS3BackendWithRedisLock(
        s3Client,
        "my-bucket",
        redisClient,
    )

    // 4. Wrap with encryption (recommended for sensitive data)
    encryptionKey := loadEncryptionKeyFromSecretsManager() // 32-byte key
    backend, _ := smarterbase.NewEncryptionBackend(s3Backend, encryptionKey)

    // 5. Add observability
    logger, _ := smarterbase.NewProductionZapLogger()
    metrics := smarterbase.NewPrometheusMetrics(prometheus.DefaultRegisterer)
    store := smarterbase.NewStoreWithObservability(backend, logger, metrics)

    // 6. Configure Redis indexes
    redisIndexer := smarterbase.NewRedisIndexer(redisClient)

    // Multi-value index: user_id → [order1, order2, ...]
    redisIndexer.RegisterMultiIndex(&smarterbase.MultiIndexSpec{
        Name:       "orders-by-user",
        EntityType: "orders",
        ExtractFunc: smarterbase.ExtractJSONField("user_id"),
    })

    // Multi-value index: status → [order1, order2, ...]
    redisIndexer.RegisterMultiIndex(&smarterbase.MultiIndexSpec{
        Name:       "orders-by-status",
        EntityType: "orders",
        ExtractFunc: smarterbase.ExtractJSONField("status"),
        TTL:        24 * time.Hour, // Auto-expire after 24h
    })

    // 7. Create index manager
    indexManager := smarterbase.NewIndexManager(store).
        WithRedisIndexer(redisIndexer)

    // 8. Start health monitoring with self-healing (opinionated defaults)
    monitor := smarterbase.NewIndexHealthMonitor(store, redisIndexer)

    if err := monitor.Start(ctx); err != nil {
        log.Fatal(err)
    }
    defer monitor.Stop()

    // That's it! Monitor will automatically:
    // - Check index health every 5 minutes
    // - Repair drift >5% automatically
    // - Log all actions with Prometheus metrics

    // 9. Use in application
    order := &Order{
        ID:      smarterbase.NewID(),
        UserID:  "user-123",
        Status:  "pending",
        Total:   99.99,
    }

    key := fmt.Sprintf("orders/%s.json", order.ID)
    indexManager.Create(ctx, key, order)

    // Query orders by user
    orderKeys, _ := redisIndexer.Query(ctx, "orders", "user_id", "user-123")

    // Query pending orders
    pendingKeys, _ := redisIndexer.Query(ctx, "orders", "status", "pending")
}
```

---

### Error Handling Patterns

```go
// Retry on transient errors
func saveWithRetry(ctx context.Context, store *smarterbase.Store, key string, data interface{}) error {
    config := smarterbase.DefaultRetryConfig()

    for i := 0; i < config.MaxRetries; i++ {
        err := store.PutJSON(ctx, key, data)
        if err == nil {
            return nil
        }

        // Check if error is retryable
        if !smarterbase.IsRetryable(err) {
            return fmt.Errorf("permanent error: %w", err)
        }

        // Exponential backoff
        backoff := config.InitialBackoff * time.Duration(1<<uint(i))
        time.Sleep(backoff)
    }

    return fmt.Errorf("failed after %d retries", config.MaxRetries)
}

// Handle not found errors
func getUser(ctx context.Context, store *smarterbase.Store, userID string) (*User, error) {
    var user User
    key := fmt.Sprintf("users/%s.json", userID)

    err := store.GetJSON(ctx, key, &user)
    if smarterbase.IsNotFound(err) {
        // User doesn't exist - return nil, not an error
        return nil, nil
    }
    if err != nil {
        return nil, fmt.Errorf("failed to get user: %w", err)
    }

    return &user, nil
}
```

---

### Advanced Queries

```go
// Complex filtering
var premiumUsers []*User
err := store.Query("users/").
    FilterJSON(func(obj map[string]interface{}) bool {
        // Multiple conditions
        isPremium, _ := obj["subscription"].(string)
        lastLogin, _ := time.Parse(time.RFC3339, obj["last_login"].(string))
        age, _ := obj["age"].(float64)

        return isPremium == "premium" &&
               lastLogin.After(time.Now().AddDate(0, 0, -30)) &&
               age >= 18
    }).
    SortByField("created_at", false). // Newest first
    Limit(100).
    All(ctx, &premiumUsers)

// Streaming large result sets (memory efficient)
err := store.Query("users/").Each(ctx, func(key string, data []byte) error {
    var user User
    json.Unmarshal(data, &user)

    // Process one at a time
    processUser(&user)

    return nil // Continue, or return error to stop
})

// Pagination
page := 0
pageSize := 50

for {
    var users []*User
    err := store.Query("users/").
        Offset(page * pageSize).
        Limit(pageSize).
        All(ctx, &users)

    if err != nil || len(users) == 0 {
        break
    }

    processPage(users)
    page++
}
```

---

### Multi-Value Index Queries

```go
// OR query: Get orders for multiple users
userIDs := []string{"user-1", "user-2", "user-3"}
orderKeys, _ := redisIndexer.QueryMultiple(ctx, "orders", "user_id", userIDs)
// Returns all orders for any of the 3 users

// Count items in index
count, _ := redisIndexer.Count(ctx, "orders", "status", "pending")
fmt.Printf("Pending orders: %d\n", count)

// Get index statistics
stats, _ := redisIndexer.GetIndexStats(ctx, "orders", "status",
    []string{"pending", "processing", "completed", "cancelled"})
// stats = {"pending": 42, "processing": 15, "completed": 1203, "cancelled": 8}
```

---

### Atomic Update Patterns

```go
// ✅ RECOMMENDED: Transfer between accounts with distributed locks
lock := smarterbase.NewDistributedLock(redisClient, "smarterbase")

// Lock the "from" account to prevent concurrent modifications
err := smarterbase.WithAtomicUpdate(ctx, store, lock, "accounts/from", 10*time.Second,
    func(ctx context.Context) error {
        var fromAccount Account
        if err := store.GetJSON(ctx, "accounts/from", &fromAccount); err != nil {
            return err
        }

        // Check balance
        if fromAccount.Balance < 100 {
            return fmt.Errorf("insufficient funds")
        }

        // Get destination account
        var toAccount Account
        if err := store.GetJSON(ctx, "accounts/to", &toAccount); err != nil {
            return err
        }

        // Update balances (protected by lock)
        fromAccount.Balance -= 100
        toAccount.Balance += 100

        // Save both accounts
        store.PutJSON(ctx, "accounts/from", &fromAccount)
        store.PutJSON(ctx, "accounts/to", &toAccount)

        // Add audit log
        store.PutJSON(ctx, "audit/txn-"+smarterbase.NewID(), AuditLog{
            Type:      "transfer",
            From:      fromAccount.ID,
            To:        toAccount.ID,
            Amount:    100,
            Timestamp: time.Now(),
        })

        return nil
    })

if err != nil {
    log.Printf("Transfer failed: %v", err)
}
```

---

### Batch Operations

```go
// Bulk import with progress tracking
func bulkImport(ctx context.Context, store *smarterbase.Store, users []*User) error {
    batchSize := 100
    batchWriter := store.NewBatchWriter(batchSize)

    for i, user := range users {
        key := fmt.Sprintf("users/%s.json", user.ID)

        if err := batchWriter.Add(ctx, key, user); err != nil {
            return fmt.Errorf("failed at user %d: %w", i, err)
        }

        // Progress tracking
        if (i+1) % 1000 == 0 {
            log.Printf("Imported %d/%d users", i+1, len(users))
        }
    }

    // Flush remaining items
    return batchWriter.Flush(ctx)
}

// Parallel batch operations with error handling
items := map[string]interface{}{
    "users/1": &User{ID: "1"},
    "users/2": &User{ID: "2"},
    // ... thousands more
}

results := store.BatchPutJSON(ctx, items)
analysis := smarterbase.AnalyzeBatchResults(results)

if analysis.Failed > 0 {
    log.Printf("Batch operation: %d succeeded, %d failed",
               analysis.Successful, analysis.Failed)

    // Retry failed items
    for _, op := range analysis.Errors {
        log.Printf("Failed: %s - %v", op.Key, op.Error)
    }
}
```

---

### Index Health Monitoring

```go
// Simple: Just start the monitor with opinionated defaults
// - Checks every 5 minutes
// - Auto-repairs drift >5%
// - Logs everything with metrics
monitor := smarterbase.NewIndexHealthMonitor(store, redisIndexer)
monitor.Start(ctx)
defer monitor.Stop()

// That's it! Self-healing by default.

// Optional: Customize if needed
monitor := smarterbase.NewIndexHealthMonitor(store, redisIndexer).
    WithInterval(10 * time.Minute).  // Less frequent checks
    WithDriftThreshold(10.0).         // Higher tolerance
    WithAutoRepair(false)             // Disable auto-repair

// Manual health check (if auto-repair disabled)
report, err := monitor.Check(ctx, "users")
if err != nil {
    log.Printf("Health check failed: %v", err)
}

if report.DriftPercentage > 5.0 {
    log.Printf("WARNING: Index drift detected: %.2f%%", report.DriftPercentage)
    log.Printf("Missing in Redis: %d", report.MissingInRedis)
    log.Printf("Extra in Redis: %d", report.ExtraInRedis)

    // Manual repair
    if err := monitor.RepairDrift(ctx, report); err != nil {
        log.Printf("Repair failed: %v", err)
    }
}
```

---

### Load Testing Your Setup

```go
// Test your production configuration
func benchmarkSetup() {
    config := smarterbase.LoadTestConfig{
        Duration:    60 * time.Second,
        Concurrency: 20,
        OperationMix: smarterbase.OperationMix{
            ReadPercent:   70,
            WritePercent:  25,
            DeletePercent: 5,
        },
        KeyCount:  10000,
        TargetRPS: 1000,
    }

    tester := smarterbase.NewLoadTester(store, config)
    results, err := tester.Run(ctx)

    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Load Test Results:\n")
    fmt.Printf("  Total operations: %d\n", results.TotalOperations)
    fmt.Printf("  Success rate: %.2f%%\n", results.SuccessRate)
    fmt.Printf("  Actual RPS: %.2f\n", results.ActualRPS)
    fmt.Printf("  Avg latency: %v\n", results.AvgLatency)
    fmt.Printf("  P95 latency: %v\n", results.P95Latency)
    fmt.Printf("  P99 latency: %v\n", results.P99Latency)

    // Validate performance requirements
    if results.P95Latency > 200*time.Millisecond {
        log.Printf("WARNING: P95 latency exceeds 200ms threshold")
    }
}
```

---

### Streaming Large Files

```go
// Upload large file (photos, videos)
func uploadLargeFile(ctx context.Context, backend smarterbase.Backend, filePath string) error {
    file, err := os.Open(filePath)
    if err != nil {
        return err
    }
    defer file.Close()

    stat, _ := file.Stat()
    key := fmt.Sprintf("files/%s", filepath.Base(filePath))

    return backend.PutStream(ctx, key, file, stat.Size())
}

// Download large file
func downloadLargeFile(ctx context.Context, backend smarterbase.Backend, key, outputPath string) error {
    reader, err := backend.GetStream(ctx, key)
    if err != nil {
        return err
    }
    defer reader.Close()

    output, err := os.Create(outputPath)
    if err != nil {
        return err
    }
    defer output.Close()

    _, err = io.Copy(output, reader)
    return err
}
```

---

### Append-Only Event Logs (JSONL)

```go
// Append events to log file
func logEvent(ctx context.Context, backend smarterbase.Backend, event Event) error {
    eventJSON, _ := json.Marshal(event)
    eventJSON = append(eventJSON, '\n') // JSONL format

    key := fmt.Sprintf("logs/%s.jsonl", time.Now().Format("2006-01-02"))
    return backend.Append(ctx, key, eventJSON)
}

// Read and process event log
func processEventLog(ctx context.Context, store *smarterbase.Store, date string) error {
    key := fmt.Sprintf("logs/%s.jsonl", date)
    data, err := store.Backend().Get(ctx, key)
    if err != nil {
        return err
    }

    // Parse JSONL
    scanner := bufio.NewScanner(bytes.NewReader(data))
    for scanner.Scan() {
        var event Event
        json.Unmarshal(scanner.Bytes(), &event)
        processEvent(&event)
    }

    return scanner.Err()
}
```

---

## Examples Directory

See [examples/](./examples/) directory for complete examples:

- **metrics-dashboard** - Prometheus metrics integration
- More examples coming soon

## Testing

```bash
go test -v              # All tests
go test -bench=.        # Benchmarks
go test -cover          # Coverage
go test -race           # Race detection
```

All tests use filesystem backend - no external dependencies required.

## Performance

| Operation | Complexity | Notes |
|-----------|-----------|-------|
| Put | O(1) | Plus O(n) for n indexes |
| Get | O(1) | Direct key lookup |
| List | O(n) | Scans all keys with prefix |
| Index Query | O(1) | Redis or file lookup |

**Recommended limits:**
- Objects: < 10M per backend
- Indexes per object: < 10
- Concurrent writes to same index: < 100/sec

## When to Use SmarterBase

### ✅ Perfect For "No Database" Applications

**Use cases where you DON'T need a database:**
- **User management** - Profiles, preferences, settings
- **Configuration storage** - App configs, feature flags
- **Content management** - Blog posts, articles, pages
- **Order/invoice storage** - E-commerce transactions
- **Metadata catalogs** - File metadata, asset tracking
- **Event logs** - Audit trails, activity logs (JSONL)
- **API caching** - Long-lived cached responses

**Team benefits:**
- ✅ No database expertise required
- ✅ No migration planning
- ✅ Deploy like any Go app (no DB dependency)
- ✅ Redis is just for speed (can rebuild indexes)
- ✅ S3 is managed by AWS (11 9s durability)

### ❌ Still Need a Database For

**Use a real database when you need:**
- Complex JOINs across multiple entity types
- Real-time aggregations (SUM, COUNT, GROUP BY)
- Strict ACID transactions (financial transfers)
- Sub-millisecond response times at scale
- Full-text search (use Elasticsearch)
- Graph queries (use Neo4j)
- Time-series analytics (use TimescaleDB)

## Production Deployment

**Critical requirements:**

- ✅ Use `S3BackendWithRedisLock` (NOT plain `S3Backend`) - prevents race conditions
- ✅ Enable encryption (`EncryptionBackend` wrapper) - 32-byte key from secrets manager
- ✅ Redis cluster with persistence (AOF + RDB) - circuit breaker protects against failures
- ✅ Observability configured (Prometheus + Zap) - monitor drift, locks, errors
- ✅ Index health monitoring (5min checks, 5% drift threshold, auto-repair)
- ✅ Load testing completed (20+ concurrent, failover scenarios validated)

**Performance targets:** P95 < 200ms (reads), P99 < 500ms (writes), drift < 1%

## Known Limitations

- ⚠️ Plain `S3Backend` has race window in PutIfMatch - **Use `S3BackendWithRedisLock` for production**
- ⚠️ Transactions are NOT ACID (no isolation) - **Use distributed locks for critical operations**
- ⚠️ Query.All() loads into memory - **Use Each() or pagination for large datasets**
- ⚠️ S3 base latency is 50-100ms - **Add caching layer for read-heavy workloads**

## Documentation

- [DATASHEET.md](./DATASHEET.md) - Technical specifications and architecture
- [CONTRIBUTING.md](./CONTRIBUTING.md) - Contributing guidelines

## Development Setup

### Installing Git Hooks

Install pre-commit hooks to ensure code quality and proper commit messages:

```bash
./scripts/install-hooks.sh
```

This installs:
- **commit-msg hook** - Validates [Conventional Commits](https://www.conventionalcommits.org/) format
- **pre-commit hook** - Runs build and tests before committing

Commit messages must follow the format:
```
<type>: <description>

Types: feat, fix, docs, refactor, test, chore
Examples:
  feat: add distributed lock support
  fix: resolve race condition in index updates
```

See [.github/SEMANTIC_VERSIONING.md](./.github/SEMANTIC_VERSIONING.md) for details on semantic versioning.

## Contributing

Contributions welcome! Please ensure:
- Git hooks installed: `./scripts/install-hooks.sh`
- Tests pass: `go test -v -race`
- Code is formatted: `go fmt`
- Commit messages follow Conventional Commits format
- Documentation is updated

## License

MIT License - See [LICENSE](./LICENSE) file for details

## Credits

Developed for production use at scale. Battle-tested with millions of objects.
