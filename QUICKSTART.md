# Quick Start Guide

Get started with SmarterBase in 5 minutes!

## 1. Install

```bash
go get github.com/adrianmcphee/smarterbase
```

## 2. Basic Usage

Create a file `main.go`:

```go
package main

import (
    "context"
    "fmt"
    "github.com/adrianmcphee/smarterbase"
)

type User struct {
    ID    string `json:"id"`
    Email string `json:"email"`
    Name  string `json:"name"`
}

func main() {
    ctx := context.Background()

    // 1. Create a store (filesystem for dev)
    backend := smarterbase.NewFilesystemBackend("./data")
    defer backend.Close()

    store := smarterbase.NewStore(backend)

    // 2. Create a user
    user := &User{
        ID:    smarterbase.NewID(),
        Email: "alice@example.com",
        Name:  "Alice",
    }

    key := "users/" + user.ID
    store.PutJSON(ctx, key, user)

    fmt.Println("✅ Created user:", user.Email)

    // 3. Read it back
    var retrieved User
    store.GetJSON(ctx, key, &retrieved)

    fmt.Println("✅ Retrieved:", retrieved.Name)
}
```

Run it:
```bash
go run main.go
```

## 3. Add Redis Indexing (Optional)

Install Redis:
```bash
# Using Docker
docker run -d -p 6379:6379 redis:7-alpine

# Or macOS
brew install redis && brew services start redis
```

Update your code:
```go
package main

import (
    "context"
    "fmt"
    "github.com/adrianmcphee/smarterbase"
    "github.com/redis/go-redis/v9"
)

type User struct {
    ID    string `json:"id"`
    Email string `json:"email"`
    Name  string `json:"name"`
}

func main() {
    ctx := context.Background()

    // Storage backend
    backend := smarterbase.NewFilesystemBackend("./data")
    defer backend.Close()
    store := smarterbase.NewStore(backend)

    // Redis for indexing
    redisClient := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })
    defer redisClient.Close()

    // Create indexer
    redisIndexer := smarterbase.NewRedisIndexer(redisClient)

    // Register email index (1:1 mapping)
    redisIndexer.RegisterMultiIndex(&smarterbase.MultiIndexSpec{
        Name:       "users-by-email",
        EntityType: "users",
        ExtractFunc: smarterbase.ExtractJSONField("email"),
    })

    // Index manager coordinates everything
    indexManager := smarterbase.NewIndexManager(store).
        WithRedisIndexer(redisIndexer)

    // Create with automatic indexing
    user := &User{
        ID:    smarterbase.NewID(),
        Email: "alice@example.com",
        Name:  "Alice",
    }

    key := "users/" + user.ID
    indexManager.Create(ctx, key, user)

    fmt.Println("✅ Created with index:", user.Email)

    // Query by email - O(1) lookup!
    keys, _ := redisIndexer.Query(ctx, "users", "email", "alice@example.com")

    fmt.Printf("✅ Found %d users\n", len(keys))
}
```

## 4. Production Setup

For production, use S3 + Redis locks:

```go
package main

import (
    "context"
    "github.com/adrianmcphee/smarterbase"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/s3"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/redis/go-redis/v9"
)

func main() {
    ctx := context.Background()

    // 1. Initialize S3
    cfg, _ := config.LoadDefaultConfig(ctx)
    s3Client := s3.NewFromConfig(cfg)

    // 2. Initialize Redis
    redisClient := redis.NewClient(&redis.Options{
        Addr: "redis:6379",
    })

    // 3. Create production backend with locks
    backend := smarterbase.NewS3BackendWithRedisLock(
        s3Client,
        "my-production-bucket",
        redisClient,
    )

    // 4. Add observability
    logger, _ := smarterbase.NewProductionZapLogger()
    metrics := smarterbase.NewPrometheusMetrics(prometheus.DefaultRegisterer)

    store := smarterbase.NewStoreWithObservability(backend, logger, metrics)

    // 5. Now use it exactly like before!
    // store.PutJSON(ctx, key, data)
}
```

## Key Concepts

### Storage Backend
Choose based on your environment:
- **Development**: `NewFilesystemBackend("./data")`
- **Production**: `NewS3BackendWithRedisLock(s3, bucket, redis)`
- **Google Cloud**: `NewGCSBackend(client, bucket)`

### Indexing
Two types of indexes:
- **1:1 (unique)**: email → user_id
- **1:N (multi-value)**: user_id → [order1, order2, ...]

Use `RegisterMultiIndex()` for both types.

### Operations
- **Create**: `indexManager.Create(ctx, key, data)`
- **Read**: `store.GetJSON(ctx, key, &dest)`
- **Update**: `indexManager.Update(ctx, key, newData)`
- **Delete**: `indexManager.Delete(ctx, key)`
- **Query**: `redisIndexer.Query(ctx, entity, field, value)`

## Next Steps

1. **Run examples**: See [examples/](./examples/) for complete use cases
2. **Read docs**: Check [README.md](./README.md) for full API
3. **Production guide**: See [DEPLOYMENT.md](./DEPLOYMENT.md) if it exists
4. **Architecture**: Read [DATASHEET.md](./DATASHEET.md) for internals

## Common Patterns

### Atomic Updates (Critical!)
Use locks for financial transactions:
```go
lock := smarterbase.NewDistributedLock(redisClient, "smarterbase")

smarterbase.WithAtomicUpdate(ctx, store, lock, key, 10*time.Second,
    func(ctx context.Context) error {
        // This is fully isolated - no race conditions
        var account Account
        store.GetJSON(ctx, key, &account)
        account.Balance += 100
        store.PutJSON(ctx, key, &account)
        return nil
    })
```

### Batch Operations
10-100x faster for multiple items:
```go
// Batch write
items := map[string]interface{}{
    "users/1": &User{ID: "1", Email: "user1@example.com"},
    "users/2": &User{ID: "2", Email: "user2@example.com"},
}
results := store.BatchPutJSON(ctx, items)

// Batch read
keys := []string{"users/1", "users/2"}
users := store.BatchGetJSON(ctx, keys)
```

### Streaming Large Datasets
Memory-efficient processing:
```go
store.Query("users/").Each(ctx, func(key string, data []byte) error {
    var user User
    json.Unmarshal(data, &user)
    processUser(&user)
    return nil // or error to stop
})
```

## Getting Help

- **Documentation**: [README.md](./README.md)
- **Examples**: [examples/](./examples/)
- **Issues**: [GitHub Issues](https://github.com/adrianmcphee/smarterbase/issues)
- **Contributing**: [CONTRIBUTING.md](./CONTRIBUTING.md)

## Important Warnings ⚠️

1. **Production S3**: Always use `S3BackendWithRedisLock` to prevent race conditions
2. **Transactions**: `WithTransaction()` is NOT ACID - use `WithAtomicUpdate()` for critical ops
3. **Memory**: `Query().All()` loads everything - use `Each()` or `Limit()` for large datasets
4. **Latency**: S3 has 50-100ms base latency - add caching for hot data

Happy building! 🚀
