# SmarterBase Examples

This directory contains complete, production-ready examples showing common use cases for SmarterBase.

## Getting Started: Two Approaches

SmarterBase supports two usage patterns - choose based on your needs:

### 🚀 Simple Pattern (Recommended for Most Use Cases)
**Example:** `user-management/`, `simple/`

The simple pattern uses core abstractions without advanced features:
- ✅ Direct `Backend` access for storage
- ✅ Helper functions (`GetJSON`, `PutJSON`, `BatchGet[T]`)
- ✅ Simple ID and time utilities
- ✅ Custom Redis integration as needed
- ✅ **80% of value with 20% of features**

This is the pattern used in the "hectic" codebase - pragmatic, focused, and easy to understand.

### 🏗️ Advanced Pattern (For High-Scale Systems)
**Example:** `production-patterns/`, `ecommerce-orders/`

The advanced pattern adds Redis indexing and resilience:
- ✅ `IndexManager` for automatic index coordination
- ✅ `RedisIndexer` for O(1) lookups
- ✅ Redis fallback paths (graceful degradation)
- ✅ Query profiling and complexity tracking
- ✅ **Production-ready resilience patterns**

This is the pattern used in the "tuinplan" codebase - advanced indexing with observability.

---

## Available Examples

### 0. Production Patterns (NEW!)
**Directory:** `production-patterns/`

**⭐ Best practices for production systems:**
- Redis-first with automatic fallback to full scans
- Query profiling and complexity tracking (O(1) vs O(n))
- Graceful degradation during Redis outages
- Observability for index usage and performance
- Simple key generation (ADR-0005 pattern)

```bash
cd production-patterns
go run main.go
```

**Use cases:**
- High-traffic production systems
- Systems requiring resilience
- Performance-critical applications
- Query optimization and monitoring

**Key Learnings:**
- When to use Redis fallback pattern
- How to profile query performance
- Why simple `fmt.Sprintf()` is better than `KeyBuilder` for simple keys

---

### 1. User Management System
**Directory:** `user-management/`

Complete user management with:
- Create, read, update, delete operations
- Email-based lookups (O(1) with Redis indexes)
- Role-based queries
- Active user filtering
- Automatic index coordination

```bash
cd user-management
go run main.go
```

**Use cases:**
- Authentication systems
- User profile management
- Permission systems

---

### 2. E-Commerce Order Storage
**Directory:** `ecommerce-orders/`

Order management system with:
- Order creation with automatic totaling
- Status transitions with validation
- User-based order queries
- Status-based filtering
- Atomic status updates (distributed locks)
- Revenue calculations

```bash
cd ecommerce-orders
go run main.go
```

**Use cases:**
- Order management
- Invoice storage
- Fulfillment tracking

---

### 3. Multi-Tenant Configuration
**Directory:** `multi-tenant-config/`

SaaS configuration management with:
- Per-tenant isolated configuration
- Plan-based features and limits
- Atomic configuration updates
- Plan upgrade workflows
- Plan statistics

```bash
cd multi-tenant-config
go run main.go
```

**Use cases:**
- SaaS platform configuration
- Feature flags
- Tenant settings
- Subscription management

---

### 4. Event Logging with JSONL
**Directory:** `event-logging/`

High-performance append-only logging with:
- JSONL format (one event per line)
- Append-only operations (no locks needed)
- Memory-efficient streaming
- Event filtering and statistics
- Audit trail support

```bash
cd event-logging
go run main.go
```

**Use cases:**
- Application logs
- Audit trails
- Activity tracking
- Security events

---

### 5. Metrics Dashboard
**Directory:** `metrics-dashboard/`

Prometheus metrics integration showing:
- Operation counters
- Latency histograms
- Error tracking
- Custom metrics

```bash
cd metrics-dashboard
go run main.go
# Visit http://localhost:9090/metrics
```

---

## Prerequisites

All examples require:
- Go 1.18+
- Redis (default: localhost:6379, or set REDIS_ADDR env var)

### Start Redis Locally

```bash
# Using Docker (recommended)
docker run -d -p 6379:6379 redis:7-alpine

# Or using Homebrew (macOS)
brew install redis
brew services start redis
```

## Running Examples

Each example can run standalone:

```bash
# Navigate to example directory
cd user-management

# Run directly
go run main.go

# Or build and run
go build -o user-manager
./user-manager
```

## Adapting for Production

All examples use filesystem backend for simplicity. For production:

1. **Replace backend:**
```go
// Development
backend := smarterbase.NewFilesystemBackend("./data")

// Production
cfg, _ := config.LoadDefaultConfig(ctx)
s3Client := s3.NewFromConfig(cfg)
redisClient := redis.NewClient(smarterbase.RedisOptions())
backend := smarterbase.NewS3BackendWithRedisLock(s3Client, "bucket", redisClient)
```

2. **Add observability:**
```go
logger, _ := smarterbase.NewProductionZapLogger()
metrics := smarterbase.NewPrometheusMetrics(prometheus.DefaultRegisterer)
store := smarterbase.NewStoreWithObservability(backend, logger, metrics)
```

3. **Enable encryption:**
```go
encryptionKey := loadFromSecretsManager() // 32-byte key
backend, _ := smarterbase.NewEncryptionBackend(s3Backend, encryptionKey)
```

See the main [README.md](../README.md) for complete production setup guide.

## Learning Path

### For Most Applications (Simple Pattern)

1. **Start with:** `simple/01-quickstart/` - Absolute basics
2. **Then:** `user-management/` - Basic CRUD operations
3. **Next:** `event-logging/` - Understand JSONL and streaming
4. **Finally:** `ecommerce-orders/` - Learn atomic updates and locks

### For High-Scale Production Systems (Advanced Pattern)

1. **Start with:** `production-patterns/` - Redis fallback and profiling ⭐
2. **Then:** `ecommerce-orders/` - IndexManager coordination
3. **Next:** `multi-tenant-config/` - Multi-value indexes
4. **Study:** [ADR-0005](../docs/adr/0005-core-api-helpers-guidance.md) - Core API guidance

## Key Concepts Demonstrated

### Indexing
All examples show two types of indexes:
- **Unique (1:1):** email → user_id
- **Multi-value (1:N):** user_id → [order1, order2, ...]

### Distributed Locking
`ecommerce-orders` and `multi-tenant-config` demonstrate:
- `WithAtomicUpdate()` for critical operations
- Lock contention handling
- Retry strategies

### Batch Operations
All examples use batch operations where appropriate:
- `BatchGetJSON` for parallel reads
- `BatchPutJSON` for parallel writes

### Query Patterns
Examples show various query approaches:
- Direct key access (fastest)
- Index-based queries (O(1) with Redis)
- Filtered queries (full scan)
- Streaming for large datasets

## Common Patterns

### Creating an Entity
```go
item := &Item{ID: smarterbase.NewID(), ...}
key := fmt.Sprintf("items/%s.json", item.ID)
indexManager.Create(ctx, key, item)
```

### Updating an Entity
```go
oldData, _ := store.Backend().Get(ctx, key)
// Modify item
indexManager.Update(ctx, key, &item, oldData)
```

### Querying by Index
```go
keys, _ := redisIndexer.Query(ctx, "orders", "user_id", "user-123")
results := store.BatchGetJSON(ctx, keys)
```

### Atomic Operations
```go
smarterbase.WithAtomicUpdate(ctx, store, lock, key, 10*time.Second,
    func(ctx context.Context) error {
        // Critical section - fully isolated
        return nil
    })
```

## Performance Tips

1. **Use indexes for lookups** - Avoid full scans
2. **Batch operations** - 10-100x faster for multiple items
3. **Stream large datasets** - Use `Each()` instead of `All()`
4. **Cache hot data** - Add Redis cache for frequently accessed items
5. **Monitor drift** - Enable index health monitoring

## Troubleshooting

### Redis Connection Failed
```bash
# Check Redis is running
redis-cli ping

# Start Redis if needed
docker run -d -p 6379:6379 redis:7-alpine
```

### High Memory Usage
- Use streaming (`Each()`) instead of loading all results
- Enable pagination with `Offset()` and `Limit()`

### Lock Contention
- Reduce concurrency
- Increase lock TTL
- Add retry logic

## Next Steps

1. Run all examples to understand capabilities
2. Check main [README.md](../README.md) for API reference and production setup
3. See [DATASHEET.md](../DATASHEET.md) for architecture details
4. Review [SECURITY.md](../SECURITY.md) for security best practices

## Contributing

Have a useful example? See [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.
