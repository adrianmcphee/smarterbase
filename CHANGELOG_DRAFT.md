# Changelog Draft - v1.6.0

## New Features

### RedisOptionsWithOverrides() Helper
Added `RedisOptionsWithOverrides()` for applications that need explicit configuration with environment variable fallback:

```go
// Use app config if present, else environment variables
opts := smarterbase.RedisOptionsWithOverrides(
    cfg.RedisAddr,     // empty = use REDIS_ADDR env var
    cfg.RedisPassword, // empty = use REDIS_PASSWORD env var
    10,                // app-specific pool size
    5,                 // app-specific min idle
)
redisClient := redis.NewClient(opts)
```

This addresses the common anti-pattern of calling `RedisOptions()` then immediately overriding values.

## Documentation

### New ADR-0005: Core API Helpers Guidance
Added comprehensive guidance on when and how to use Core API helpers:

- **BatchGet[T]**: Use universally for bulk entity loading (replaces 7 lines with 1)
- **KeyBuilder**: Use selectively for complex keys only (not for simple `"prefix/%s.json"` patterns)
- **RedisOptions()**: Clear guidance on helper vs. direct `redis.Options` construction

See [docs/adr/0005-core-api-helpers-guidance.md](./docs/adr/0005-core-api-helpers-guidance.md)

### Updated README
Added "Core API Helpers - Best Practices" section with:
- ✅ When to use each helper
- ❌ Anti-patterns to avoid
- Clear examples of good vs. bad usage

### Updated Examples
- **user-management**: Refactored to use new `BatchGet[T]` pattern (removed 15 lines of boilerplate)

## Breaking Changes

None - this is a backwards-compatible release.

## Migration Guide

### If you're using RedisOptions() then overriding everything:

**Before:**
```go
opts := smarterbase.RedisOptions()
opts.Addr = fmt.Sprintf("%s:%s", host, port)  // Override
opts.Password = password                      // Override
opts.PoolSize = 10                           // Override
```

**After (Option A - Use new helper):**
```go
opts := smarterbase.RedisOptionsWithOverrides(
    fmt.Sprintf("%s:%s", host, port),
    password,
    10, 5,
)
```

**After (Option B - Direct construction):**
```go
// If you're overriding everything, just construct directly
opts := &redis.Options{
    Addr:         fmt.Sprintf("%s:%s", host, port),
    Password:     password,
    DB:           0,
    PoolSize:     10,
    MinIdleConns: 5,
}
```

### If you're using manual iteration for batch loading:

**Before:**
```go
users := make([]*User, 0, len(keys))
for _, key := range keys {
    var user User
    if err := store.GetJSON(ctx, key, &user); err == nil {
        users = append(users, &user)
    }
}
```

**After:**
```go
users, err := smarterbase.BatchGet[User](ctx, store, keys)
```

## Internal Changes

- Added unit tests for `RedisOptionsWithOverrides()`
- Updated example code to follow ADR-0005 guidance

---

**Release Date:** TBD
**Full Changelog:** https://github.com/adrianmcphee/smarterbase/compare/v1.5.0...v1.6.0
