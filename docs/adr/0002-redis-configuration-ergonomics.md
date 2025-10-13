# ADR-0002: Redis Configuration Ergonomics

**Status:** Accepted
**Date:** 2025-10-13

## Context

Redis is central to SmarterBase's optional but powerful features (indexing, distributed locking). Currently, every example and application must manually:
1. Create a Redis client with hardcoded `"localhost:6379"`
2. Manage Redis lifecycle (ping, close)
3. Thread the Redis client through multiple components (RedisIndexer, DistributedLock)
4. Handle environment-specific configuration manually

This creates friction:
- **Examples** hardcode localhost instead of showing production patterns
- **Boilerplate** repeated in every application (~10 lines)
- **Configuration** management pushed entirely to users (no guidance)
- **Production readiness** unclear (where do env vars go?)

The question: Should SmarterBase provide more ergonomic Redis configuration while preserving its library (not framework) philosophy?

## Options Considered

### Option 1: Status Quo (Explicit Dependency Injection)

Keep current approach - users create and manage Redis client:

```go
redisClient := redis.NewClient(&redis.Options{
    Addr: "localhost:6379", // hardcoded!
})
defer redisClient.Close()

store := smarterbase.NewStore(backend)
redisIndexer := smarterbase.NewRedisIndexer(redisClient)
indexManager := smarterbase.NewIndexManager(store).WithRedisIndexer(redisIndexer)
lock := smarterbase.NewDistributedLock(redisClient, "smarterbase")
```

**Pros:**
- Maximum flexibility - user controls Redis completely
- Clear ownership - who created it, closes it
- No magic - all dependencies explicit
- Easy to test - inject mocks at any level
- Library stays unopinionated
- User can share Redis client with other parts of application
- No version lock-in on go-redis library
- Follows Go idioms (explicit > implicit)

**Cons:**
- Verbose boilerplate in every example
- Redis threaded through multiple components
- Easy to forget to wire Redis everywhere
- Configuration management punted to user
- Hardcoded localhost in all examples (bad practice)
- No guidance on production setup

### Option 2: Store-Managed Redis (Service Locator Pattern)

Store owns Redis connection and provides it to components:

```go
store := smarterbase.NewStore(backend).
    WithRedis(&smarterbase.RedisConfig{
        Addr: os.Getenv("REDIS_ADDR"),
    })
defer store.Close() // closes backend AND redis

indexManager := smarterbase.NewIndexManager(store) // auto-discovers Redis
lock := store.DistributedLock() // gets lock from store
```

**Pros:**
- Much less boilerplate (~60% reduction)
- Single lifecycle management
- Consistent with existing logger/metrics pattern in Store
- Components auto-discover Redis through Store
- Single source of truth for Redis instance

**Cons:**
- Store becomes service locator (anti-pattern)
- Tight coupling between Store and Redis
- Hard to share Redis with non-SmarterBase code
- Store accumulates too many responsibilities
- Testing requires mocking at Store level
- What if user wants Redis for indexing but not locking?
- `Store.DistributedLock()` feels wrong semantically (locking isn't storage)
- Violates single responsibility principle

### Option 3: Configuration Helper Functions (Library Utilities) ✅

Provide utility functions, keep architecture decoupled:

```go
// Library provides config helper
redisClient := redis.NewClient(smarterbase.RedisOptions())
defer redisClient.Close()

// Rest stays the same (explicit DI)
store := smarterbase.NewStore(backend)
redisIndexer := smarterbase.NewRedisIndexer(redisClient)
indexManager := smarterbase.NewIndexManager(store).WithRedisIndexer(redisIndexer)
lock := smarterbase.NewDistributedLock(redisClient, "smarterbase")
```

Where `RedisOptions()` reads standard env vars:
```go
func RedisOptions() *redis.Options {
    addr := os.Getenv("REDIS_ADDR")
    if addr == "" {
        addr = "localhost:6379" // dev default
    }
    return &redis.Options{
        Addr:     addr,
        Password: os.Getenv("REDIS_PASSWORD"),
        DB:       getEnvAsInt("REDIS_DB", 0),
    }
}
```

**Pros:**
- Minimal library opinion - just helpful utility
- Reduces boilerplate for env var reading
- Preserves all flexibility of Option 1
- Clear ownership still with user
- Easy to test (mock what you need)
- Library stays focused on its domain
- Works locally (defaults) AND in production (env vars)
- Follows 12-factor app principles
- Examples now show production-ready patterns
- User can still use custom redis.Options if needed
- Small API surface (~20 lines of code)

**Cons:**
- Still somewhat verbose (but that's explicit Go style)
- Doesn't reduce "wire Redis to multiple places" problem
- Library now opinionated about env var names
- Minimal ergonomic improvement (~1 line saved per example)

### Option 4: Context/Builder Pattern with Optional Convenience

High-level convenience constructor for common case:

```go
// Simple case for 80% of users:
app := smarterbase.NewApp(
    smarterbase.WithBackend(backend),
    smarterbase.WithRedisFromEnv(),
    smarterbase.WithLogger(logger),
)
defer app.Close()

store := app.Store()
indexManager := app.IndexManager()
lock := app.Lock()

// Power user case (still supported):
redisClient := redis.NewClient(...)
store := smarterbase.NewStore(backend)
// full control
```

**Pros:**
- Beginner-friendly simple path
- Power users still have full control
- Lifecycle managed for simple case
- Components properly wired automatically
- Optional Redis (graceful degradation)
- Follows functional options pattern (Go best practice)

**Cons:**
- New abstraction layer (App/Builder)
- Two ways to do the same thing (learning curve)
- App struct holds all components (complexity)
- More API surface area
- Framework-like behavior (anti-goal)
- Need to design what App provides vs doesn't
- Might attract framework expectations

## Decision

We will implement **Option 3: Configuration Helper Functions**.

Create a minimal `RedisOptions()` utility function that:
1. Reads standard environment variables (REDIS_ADDR, REDIS_PASSWORD, REDIS_DB)
2. Provides sensible defaults for local development
3. Returns standard `redis.Options` struct
4. Remains completely optional (users can ignore it)

This preserves SmarterBase's library philosophy while providing production-ready guidance.

## Consequences

### Positive
- ✅ Examples now show production-ready patterns (env vars)
- ✅ Local development "just works" (defaults to localhost:6379)
- ✅ Production deployment clear (set REDIS_ADDR env var)
- ✅ Follows 12-factor app principles
- ✅ Zero architectural coupling
- ✅ Minimal API surface (one function)
- ✅ Preserves all flexibility
- ✅ Optional (power users can ignore it)
- ✅ Library stays library (not framework)

### Negative
- ⚠️ Opinionated about env var names (REDIS_ADDR, etc.)
- ⚠️ Minimal ergonomic improvement (~10% less boilerplate)
- ⚠️ Doesn't solve "wire Redis everywhere" problem
- ⚠️ Users still manage lifecycle explicitly

### Neutral
- Helper is 100% optional - power users can ignore
- Could add more helpers later (RedisClusterOptions, etc.)
- Documentation becomes critical (show the pattern clearly)
- Env var names follow common conventions (REDIS_ADDR standard)

## Implementation

Files to create/modify:
- `redis_config.go` - New file with `RedisOptions()` helper
- `redis_config_test.go` - Comprehensive test coverage
- `examples/*/main.go` - Update all Redis-using examples (user-management, ecommerce-orders, multi-tenant-config)
- `examples/README.md` - Update production setup guidance
- `README.md` - Add production configuration section and update code examples
- `docs/adr/README.md` - Add ADR-0002 to index

Environment variables supported:
- `REDIS_ADDR` - Redis server address (default: `localhost:6379`)
- `REDIS_PASSWORD` - Redis password (default: empty)
- `REDIS_DB` - Redis database number (default: `0`)

## Philosophy

**Libraries provide sharp tools, not complete solutions.**

SmarterBase should make the right thing easy without prescribing architecture. A helper function respects user autonomy while reducing copy-paste errors.

This aligns with Go's philosophy: explicit is better than implicit. We're not hiding Redis configuration behind abstractions - we're just reading environment variables that the user could read themselves.

The verbosity of dependency injection is a feature, not a bug. It makes dependencies clear and testing straightforward.

## Alternatives Rejected

**Why not Option 2 (Store-Managed)?**
- Violates single responsibility principle
- Store is for storage, not service location
- Hard to test components in isolation
- Tight coupling between concerns
- Framework-like behavior (anti-goal)

**Why not Option 4 (Builder/App)?**
- Too much abstraction for minimal gain
- Two ways to do everything (confusion)
- Attracts framework expectations
- SmarterBase is composable blocks, not a framework

**Why not keep Status Quo?**
- Examples teaching bad practices (hardcoded localhost)
- No guidance on production deployment
- Missing obvious opportunity to help users
- 12-factor apps should use env vars

## Future Considerations

Could add additional helpers later:
- `RedisClusterOptions()` - for Redis Cluster
- `RedisSentinelOptions()` - for Redis Sentinel
- `RedisOptionsFromPrefix(prefix string)` - for multi-tenant setups

But starting minimal is better than over-engineering.
