# ADR-0005: Core API Helpers - When and How to Use

**Status:** Accepted
**Date:** 2025-10-14
**Author:** Adrian McPhee
**Related:** [ADR-0002](./0002-redis-configuration-ergonomics.md), [ADR-0003](./0003-simple-api-layer.md)

---

## Context

ADR-0003 introduced several Core API helpers to reduce boilerplate:
- `BatchGet[T]` - Generic bulk entity loading
- `KeyBuilder` - Type-safe key construction
- `RedisOptions()` - Environment-based Redis config

After real-world usage in production codebases (tuinplan, hectic), we discovered:
1. **BatchGet[T]** is universally beneficial
2. **KeyBuilder** adds overhead for simple keys but helps with complex ones
3. **RedisOptions()** causes confusion when callers immediately override values

This ADR codifies **when** and **how** to use each helper effectively.

---

## Decision

### 1. BatchGet[T] - Use Universally ✅

**Always use `BatchGet[T]`** when loading multiple entities from keys:

```go
// ✅ GOOD: Type-safe, clear intent, consistent error handling
users, err := smarterbase.BatchGet[User](ctx, store, userIDs)

// ❌ BAD: Manual iteration, repetitive, error-prone
users := make([]*User, 0, len(userIDs))
for _, id := range userIDs {
    var user User
    if err := store.GetJSON(ctx, id, &user); err == nil {
        users = append(users, &user)
    }
}
```

**Benefits:**
- 6-7 lines → 1 line
- Type-safe at compile time
- Consistent error handling (continues on individual failures)
- Clearer intent

**Exception:** Don't use if you need to filter entities after loading (see below).

### 2. KeyBuilder - Use Selectively 🤔

Use `KeyBuilder` **ONLY** when keys have complexity:

#### ✅ Use KeyBuilder For:

**Complex nested paths:**
```go
// Multiple path segments that vary
keyBuilder := KeyBuilder{
    Prefix: "workspaces/%s/projects/%s/tasks",
    Suffix: ".json",
}
key := keyBuilder.KeyWithParts(workspaceID, projectID, taskID)
```

**Environment-dependent keys:**
```go
// Keys that change between environments
keyBuilder := KeyBuilder{
    Prefix: os.Getenv("KEY_PREFIX"),  // Varies: dev-, prod-, test-
    Suffix: ".json",
}
```

**Keys with validation logic:**
```go
type ValidatedKeyBuilder struct {
    smarterbase.KeyBuilder
}

func (kb *ValidatedKeyBuilder) Key(id string) string {
    if !isValidID(id) {
        panic("invalid ID format")
    }
    return kb.KeyBuilder.Key(id)
}
```

#### ❌ Don't Use KeyBuilder For:

**Simple single-segment paths:**
```go
// ❌ UNNECESSARY: KeyBuilder adds indirection for no benefit
userKB := KeyBuilder{Prefix: "users", Suffix: ".json"}
key := userKB.Key(userID)

// ✅ BETTER: Direct and clear
key := fmt.Sprintf("users/%s.json", userID)
```

**Rationale:**
- Simple keys are self-documenting: `"users/%s.json"` is immediately clear
- KeyBuilder adds struct fields, initialization code, indirection
- Key formats rarely change (breaking change if they do)
- Inconsistency: If you use KeyBuilder, apply it *everywhere* (including cache layers)

**Key format changes:**
If you find yourself changing key formats often, you have a design problem - not a KeyBuilder problem. Fix the design.

### 3. RedisOptions() - Use Correctly ⚠️

**Problem pattern we observed:**
```go
// ❌ BAD: Call helper then immediately override everything
opts := smarterbase.RedisOptions()
opts.Addr = fmt.Sprintf("%s:%s", host, port)  // Override
opts.Password = password                       // Override
opts.PoolSize = 10                            // Override
```

**Solutions:**

#### Option A: Use helper OR explicit config (not both)
```go
// ✅ GOOD: Use helper for pure environment-based config
redisClient := redis.NewClient(smarterbase.RedisOptions())

// ✅ GOOD: Use explicit config when you have app-specific values
redisClient := redis.NewClient(&redis.Options{
    Addr:         fmt.Sprintf("%s:%s", host, port),
    Password:     password,
    DB:           0,
    PoolSize:     10,
    MinIdleConns: 5,
})
```

#### Option B: Add new helper for mixed scenarios
```go
// New helper: RedisOptionsWithOverrides
opts := smarterbase.RedisOptionsWithOverrides(
    fmt.Sprintf("%s:%s", host, port),  // addr (empty = use env)
    password,                           // password (empty = use env)
    10,                                 // poolSize
    5,                                  // minIdleConns
)
```

We chose **Option B** - implement `RedisOptionsWithOverrides()` for common pattern.

---

## Implementation Guidance

### For Library Authors (smarterbase developers)

**Do:**
- ✅ Keep helpers **composable** (not monolithic)
- ✅ Provide **escape hatches** (direct access when needed)
- ✅ Document **when NOT to use** helpers
- ✅ Use generics for type-safe helpers (`BatchGet[T]`)

**Don't:**
- ❌ Create helpers that are immediately overridden
- ❌ Force abstraction on simple cases
- ❌ Hide complexity that users need to understand

### For Application Developers (smarterbase users)

**Do:**
- ✅ Use `BatchGet[T]` whenever loading multiple entities
- ✅ Use `KeyBuilder` for complex/nested keys
- ✅ Use `RedisOptions()` for pure environment config
- ✅ Use direct `fmt.Sprintf()` for simple keys

**Don't:**
- ❌ Use KeyBuilder for every key (creates noise)
- ❌ Call helper then override everything
- ❌ Create abstraction before you need it

---

## Examples

### Example 1: BatchGet with Filtering

**When you need to filter after loading, use manual iteration:**

```go
// ✅ GOOD: Can't use BatchGet because we filter after loading
func GetActiveSessionByUserID(ctx context.Context, userID string) (*Session, error) {
    keys, _ := redisIndexer.Query(ctx, "sessions", "user_id", userID)

    // Must iterate manually to filter
    for _, key := range keys {
        var session Session
        if err := store.GetJSON(ctx, key, &session); err == nil && !session.IsExpired() {
            return &session, nil  // Return first active
        }
    }
    return nil, ErrNoActiveSession
}
```

**When you filter BEFORE loading, use Redis index:**

```go
// ✅ BETTER: Filter in Redis, then BatchGet
keys, _ := redisIndexer.Query(ctx, "sessions", "user_id", userID)
keys = redisIndexer.Query(ctx, "sessions", "status", "active")  // Filter in Redis

// Now use BatchGet
sessions, _ := smarterbase.BatchGet[Session](ctx, store, keys)
```

### Example 2: Mixed Config Pattern

```go
// Application has explicit config but wants environment fallback
func NewCache(cfg Config, logger Logger) (*Cache, error) {
    var redisAddr string
    if cfg.RedisHost != "" {
        redisAddr = fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort)
    }

    // Use new helper for mixed scenario
    opts := smarterbase.RedisOptionsWithOverrides(
        redisAddr,       // empty = read from REDIS_ADDR env
        cfg.RedisPassword, // empty = read from REDIS_PASSWORD env
        cfg.PoolSize,    // app-specific
        cfg.MinIdle,     // app-specific
    )

    return &Cache{
        client: redis.NewClient(opts),
        logger: logger,
    }
}
```

### Example 3: When KeyBuilder Makes Sense

```go
// ✅ GOOD: Complex multi-tenant key structure
type TenantKeyBuilder struct {
    TenantID string
}

func (kb *TenantKeyBuilder) UserKey(userID string) string {
    return fmt.Sprintf("tenants/%s/users/%s.json", kb.TenantID, userID)
}

func (kb *TenantKeyBuilder) ProjectKey(projectID string) string {
    return fmt.Sprintf("tenants/%s/projects/%s.json", kb.TenantID, projectID)
}

// Centralized tenant-scoping, prevents accidental cross-tenant access
```

---

## Consequences

### Positive
- ✅ Clear guidance prevents overuse of abstractions
- ✅ `BatchGet[T]` adoption reduces 70-100 lines across typical codebase
- ✅ KeyBuilder guidance prevents "abstraction for abstraction's sake"
- ✅ Redis config helpers now match real-world usage patterns

### Negative
- ⚠️ Developers must understand when to use each helper (not automatic)
- ⚠️ Existing code may have overused KeyBuilder (requires refactoring)

### Risks
- 📉 Risk of inconsistency if team doesn't follow guidance
  - **Mitigation:** Code review checklist + examples in docs

---

## Alternatives Considered

### Alternative 1: Force KeyBuilder Everywhere
**Rejected:** Creates noise for simple keys. `fmt.Sprintf("users/%s.json", id)` is clearer than `userKB.Key(id)`.

### Alternative 2: Remove KeyBuilder Entirely
**Rejected:** Useful for complex keys (multi-tenant, nested paths).

### Alternative 3: Make RedisOptions() More Flexible
**Accepted:** Added `RedisOptionsWithOverrides()` for common pattern.

---

## References

- [ADR-0002: Redis Configuration Ergonomics](./0002-redis-configuration-ergonomics.md)
- [ADR-0003: Simple API Layer](./0003-simple-api-layer.md)
- Real-world usage: tuinplan platform refactoring (Oct 2025)
- Go Proverbs: "Clear is better than clever" - https://go-proverbs.github.io/

---

## Decision Log

| Date | Decision | Rationale |
|------|----------|-----------|
| 2025-10-14 | Codify BatchGet[T] as universal best practice | Unanimous positive feedback from production usage |
| 2025-10-14 | Limit KeyBuilder to complex keys only | Simple keys (95% of cases) don't benefit from abstraction |
| 2025-10-14 | Add RedisOptionsWithOverrides() | Observed pattern: call helper then override everything |
