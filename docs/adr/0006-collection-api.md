# ADR-0006: Pragmatic Helper Functions to Reduce Boilerplate

**Status:** Accepted
**Date:** 2025-01-18
**Implemented:** 2025-01-19
**Deciders:** Engineering Team
**Related:** ADR-0003 (Simple API Layer), ADR-0004 (Redis Indexing)

## Context

After analyzing real-world usage in multiple production codebases, we identified repetitive patterns that add unnecessary boilerplate without providing value.

### Real Problems from Production Code

#### 1. **Repetitive Query Fallback Pattern**

The same 40-line Redis→scan fallback + profiling pattern appears in 20+ places across production stores:

```go
// Seen in property stores, planning stores, execution stores, etc.
func (s *Store) GetPrimaryProperty(ctx context.Context, userID string) (*property.Property, error) {
    profiler := smarterbase.GetProfilerFromContext(ctx)
    profile := profiler.StartProfile("GetPrimaryProperty")
    if profile != nil {
        profile.FilterFields = []string{"user_id"}
        defer func() { profiler.Record(profile) }()
    }

    // Try Redis index first
    if s.redisIndexer != nil {
        keys, err := s.redisIndexer.Query(ctx, "properties", "user_id", userID)
        if err == nil && len(keys) > 0 {
            // ... 10 lines of loading and filtering ...
            if profile != nil {
                profile.Complexity = smarterbase.ComplexityO1
                profile.IndexUsed = "redis:properties-by-user-id"
                profile.StorageOps = len(keys)
                profile.ResultCount = 1
            }
            return &prop, nil
        }
    }

    // Fallback to full scan
    var props []*property.Property
    err := s.base.Query("properties/").FilterJSON(/* ... */).All(ctx, &props)
    if profile != nil {
        profile.Complexity = smarterbase.ComplexityON
        profile.IndexUsed = "none:full-scan"
        profile.FallbackPath = true
        profile.ResultCount = len(props)
    }
    return props, err
}
```

**Impact:** ~400 lines of duplicated fallback logic in a typical medium-sized application.

#### 2. **Manual Index Coordination on Updates**

Updating indexed fields requires remembering to update indexes manually (seen in case stores, user stores, etc.):

```go
func (s *Store) CreateCase(ctx context.Context, caseObj *casedom.Case) error {
    // ... set ID, number, timestamps ...

    // Save to backend
    caseKey := fmt.Sprintf("workspaces/%s/cases/%s/metadata.json", caseObj.WorkspaceID, caseObj.ID)
    if err := s.putJSON(ctx, caseKey, caseObj); err != nil {
        return err
    }

    // Update file-based case number index
    if err := s.updateCaseNumberIndex(ctx, caseObj.WorkspaceID, caseObj.Number, caseObj.ID); err != nil {
        return fmt.Errorf("failed to update case number index: %w", err)
    }

    // Update Redis global index
    if s.cache != nil {
        caseIndex := shared.NewRedisGlobalIndex(s.cache, "cases")
        if err := caseIndex.Set(ctx, caseObj.ID, caseObj.WorkspaceID); err != nil {
            return fmt.Errorf("failed to update global index: %w", err)
        }
    }

    // Update Redis secondary indexes (status, priority, assignee)
    if s.cache != nil {
        newFields := &shared.CaseIndexFields{/* ... */}
        if err := shared.UpdateCaseIndexes(ctx, s.cache, caseObj.WorkspaceID, caseObj.ID, nil, newFields); err != nil {
            return fmt.Errorf("failed to update secondary indexes: %w", err)
        }
    }

    return nil
}
```

**Real Bug:** Case status indexes became stale when a developer updated case.Status directly but forgot to call `shared.UpdateCaseIndexes()`. Explicit index management is error-prone.

### What We Don't Want

We considered building a MongoDB-style Collection API with automatic index management, fluent query builders, and magic updates. **This is over-engineering.**

The problems above are solved with simple helper functions, not a new abstraction layer. We need to reduce boilerplate without hiding errors or adding performance overhead.

## Decision

Add **three focused helper functions** that eliminate 90% of the boilerplate without building a new abstraction layer:

### 1. QueryWithFallback - Handles Redis→Scan Pattern

```go
// Wraps the common Redis index query → fallback to scan pattern
func QueryWithFallback[T any](
    ctx context.Context,
    base *Store,
    redisIndexer *RedisIndexer,
    collection string,       // e.g., "properties"
    indexField string,       // e.g., "user_id"
    indexValue string,       // e.g., "user-123"
    scanPrefix string,       // e.g., "properties/"
    filter func(*T) bool,    // Fallback filter function
) ([]*T, error)
```

**Usage:**
```go
// Before: 40 lines of boilerplate
func (s *Store) ListUserProperties(ctx context.Context, userID string) ([]*property.Property, error) {
    profiler := smarterbase.GetProfilerFromContext(ctx)
    profile := profiler.StartProfile("ListUserProperties")
    // ... 35 lines of Redis query, fallback, profiling ...
}

// After: 1 function call
func (s *Store) ListUserProperties(ctx context.Context, userID string) ([]*property.Property, error) {
    return smarterbase.QueryWithFallback[property.Property](
        ctx, s.base, s.redisIndexer,
        "properties", "user_id", userID,
        "properties/",
        func(p *property.Property) bool { return p.UserID == userID },
    )
}
```

### 2. UpdateWithIndexes - Coordinates Index Updates

```go
// Updates data and coordinated indexes in one call
type IndexUpdate struct {
    RedisKey   string  // Redis index key
    OldValue   string  // Old index value (to remove)
    NewValue   string  // New index value (to add)
}

func UpdateWithIndexes(
    ctx context.Context,
    base *Store,
    redisIndexer *RedisIndexer,
    key string,
    data interface{},
    updates []IndexUpdate,
) error
```

**Usage:**
```go
// Before: Manual index coordination (error-prone)
func (s *Store) UpdateUserEmail(ctx context.Context, user *User, newEmail string) error {
    oldEmail := user.Email
    user.Email = newEmail

    if err := s.base.PutJSON(ctx, userKey, user); err != nil {
        return err
    }

    // Easy to forget!
    if s.redisEmailIndex != nil {
        s.redisEmailIndex.Delete(ctx, oldEmail)
        s.redisEmailIndex.Set(ctx, newEmail, user.ID)
    }
    return nil
}

// After: Atomic update with indexes
func (s *Store) UpdateUserEmail(ctx context.Context, user *User, newEmail string) error {
    oldEmail := user.Email
    user.Email = newEmail

    return smarterbase.UpdateWithIndexes(ctx, s.base, s.redisIndexer,
        userKey(user.ID), user,
        []smarterbase.IndexUpdate{
            {RedisKey: "users-by-email", OldValue: oldEmail, NewValue: newEmail},
        },
    )
}
```

### 3. BatchGetWithFilter - Load and Filter Results

```go
// Loads multiple keys and applies optional filter
func BatchGetWithFilter[T any](
    ctx context.Context,
    base *Store,
    keys []string,
    filter func(*T) bool,  // Optional: nil = no filter
) ([]*T, error)
```

**Usage:**
```go
// Before: Manual batching and filtering
results := make([]*Property, 0)
for _, key := range keys {
    var prop Property
    if err := s.base.GetJSON(ctx, key, &prop); err == nil {
        if prop.IsPrimary {  // Manual filter
            results = append(results, &prop)
        }
    }
}

// After: One call
results, err := smarterbase.BatchGetWithFilter[Property](ctx, s.base, keys,
    func(p *Property) bool { return p.IsPrimary },
)
```

### Why This Approach

1. **No performance overhead** - These are thin wrappers around existing code, no reflection or magic
2. **Explicit error handling** - Index failures are visible, not hidden
3. **Implementation: ~150 lines total** - Not a 6-month ORM project
4. **Backward compatible** - Opt-in helpers, existing code unchanged
5. **Solves real problems** - Addresses the actual pain points from production code

## Implementation Details

### QueryWithFallback Implementation

```go
func QueryWithFallback[T any](
    ctx context.Context,
    base *Store,
    redisIndexer *RedisIndexer,
    collection string,
    indexField string,
    indexValue string,
    scanPrefix string,
    filter func(*T) bool,
) ([]*T, error) {
    profiler := GetProfilerFromContext(ctx)
    profile := profiler.StartProfile(fmt.Sprintf("Query%s", collection))
    if profile != nil {
        profile.FilterFields = []string{indexField}
        defer func() { profiler.Record(profile) }()
    }

    // Try Redis index first
    if redisIndexer != nil {
        keys, err := redisIndexer.Query(ctx, collection, indexField, indexValue)
        if err == nil {
            results, err := BatchGet[T](ctx, base, keys)
            if profile != nil {
                profile.Complexity = ComplexityO1
                profile.IndexUsed = fmt.Sprintf("redis:%s-by-%s", collection, indexField)
                profile.StorageOps = len(keys)
                profile.ResultCount = len(results)
            }
            return results, err
        }
    }

    // Fallback to full scan
    var results []*T
    err := base.Query(scanPrefix).
        Filter(filter).
        All(ctx, &results)

    if profile != nil {
        profile.Complexity = ComplexityON
        profile.IndexUsed = "none:full-scan"
        profile.FallbackPath = true
        profile.ResultCount = len(results)
        profile.Error = err
    }

    return results, err
}
```

### UpdateWithIndexes Implementation

```go
type IndexUpdate struct {
    RedisKey string
    OldValue string
    NewValue string
}

func UpdateWithIndexes(
    ctx context.Context,
    base *Store,
    redisIndexer *RedisIndexer,
    key string,
    data interface{},
    updates []IndexUpdate,
) error {
    // Write data first
    if err := base.PutJSON(ctx, key, data); err != nil {
        return err
    }

    // Update indexes (best effort if Redis available)
    if redisIndexer != nil {
        for _, update := range updates {
            if update.OldValue != "" {
                redisIndexer.Delete(ctx, update.RedisKey, update.OldValue)
            }
            if update.NewValue != "" {
                if err := redisIndexer.Set(ctx, update.RedisKey, update.NewValue); err != nil {
                    return fmt.Errorf("failed to update index %s: %w", update.RedisKey, err)
                }
            }
        }
    }

    return nil
}
```

### BatchGetWithFilter Implementation

```go
func BatchGetWithFilter[T any](
    ctx context.Context,
    base *Store,
    keys []string,
    filter func(*T) bool,
) ([]*T, error) {
    results := make([]*T, 0, len(keys))

    for _, key := range keys {
        var item T
        if err := base.GetJSON(ctx, key, &item); err != nil {
            continue // Skip missing items
        }

        if filter == nil || filter(&item) {
            results = append(results, &item)
        }
    }

    return results, nil
}
```

**Total implementation: ~80 lines of straightforward code**

## Implementation Plan

Single implementation phase (estimated 1-2 days):

1. **Add helper functions to helpers.go** (~80 lines)
   - Implement `QueryWithFallback[T]`
   - Implement `UpdateWithIndexes`
   - Implement `BatchGetWithFilter[T]`

2. **Add tests** (~150 lines)
   - Test each helper with Redis available
   - Test fallback when Redis unavailable
   - Verify profiling integration
   - Test error handling

3. **Update documentation** (~30 mins)
   - Add examples to README
   - Document helper functions
   - Add migration guide showing before/after

4. **Benchmark performance** (~1 hour)
   - Verify zero overhead vs manual code
   - Document any performance characteristics

**Total effort: ~2 days for complete implementation, testing, and documentation**

## Migration Path

These are opt-in helpers. Existing code continues to work unchanged. Migrate on a case-by-case basis:

### Migration Example

```go
// Before: 40 lines
func (s *Store) ListUserProperties(ctx context.Context, userID string) ([]*property.Property, error) {
    profiler := smarterbase.GetProfilerFromContext(ctx)
    profile := profiler.StartProfile("ListUserProperties")
    if profile != nil {
        profile.FilterFields = []string{"user_id"}
        defer func() { profiler.Record(profile) }()
    }

    if s.redisIndexer != nil {
        keys, err := s.redisIndexer.Query(ctx, "properties", "user_id", userID)
        if err == nil {
            props, err := smarterbase.BatchGet[property.Property](ctx, s.base, keys)
            if profile != nil {
                profile.Complexity = smarterbase.ComplexityO1
                profile.IndexUsed = "redis:properties-by-user-id"
                profile.StorageOps = len(keys)
                profile.ResultCount = len(props)
            }
            return props, err
        }
    }

    var props []*property.Property
    err := s.base.Query("properties/").
        FilterJSON(func(obj map[string]interface{}) bool {
            uid, _ := obj["user_id"].(string)
            return uid == userID
        }).
        All(ctx, &props)

    if profile != nil {
        profile.Complexity = smarterbase.ComplexityON
        profile.IndexUsed = "none:full-scan"
        profile.FallbackPath = true
        profile.ResultCount = len(props)
    }

    return props, err
}

// After: 6 lines
func (s *Store) ListUserProperties(ctx context.Context, userID string) ([]*property.Property, error) {
    return smarterbase.QueryWithFallback[property.Property](
        ctx, s.base, s.redisIndexer,
        "properties", "user_id", userID,
        "properties/",
        func(p *property.Property) bool { return p.UserID == userID },
    )
}
```

**Migrate incrementally - no flag day required.**

## Consequences

### Positive

1. **85-90% reduction in boilerplate** for common patterns (40 lines → 6 lines for queries)
2. **Reduced index bugs** - `UpdateWithIndexes` makes coordination explicit and atomic
3. **Minimal implementation effort** - ~2 days vs 6 months for a full ORM
4. **Zero performance overhead** - thin wrappers around existing code paths
5. **Backward compatible** - opt-in helpers, no breaking changes
6. **Type-safe** - leverages Go generics for compile-time safety
7. **Explicit error handling** - index failures are visible, not hidden

### Negative

1. **Still requires some boilerplate** - These are helpers, not magic. You still need to specify filters and index names.
2. **Learning curve** - Developers need to know when to use helpers vs manual code
3. **Function parameter count** - `QueryWithFallback` takes 8 parameters (though all are necessary)

### Risks

1. **Performance not yet validated** - Need benchmarks to prove zero overhead claim
2. **May not cover all edge cases** - Some complex queries might still need manual code
3. **Filter function duplication** - The filter parameter duplicates the index field logic

## Alternatives Considered

### Alternative 1: Full Collection API (MongoDB-style)

Build a complete ORM with automatic index management, fluent query builders, magic updates, etc.

**Rejected:**
- Massive implementation effort (6+ months)
- Hides errors (automatic index updates can silently fail)
- Performance overhead not justified
- We're a key-value store with optional Redis indexes, not a document database
- Over-engineering for the actual problem

### Alternative 2: Code Generation

Generate helper functions from schema definitions.

**Rejected:**
- Adds build complexity
- Less flexible than runtime helpers
- Overkill for wrapping 40 lines into 6 lines

### Alternative 3: Do Nothing

Keep the current API, accept the boilerplate as the cost of doing business.

**Rejected:**
- 400+ lines of duplicated code across production systems is real waste
- Index coordination bugs are preventable
- Simple helpers can solve 90% of the problem with minimal effort

## Success Metrics

Before accepting this ADR, validate:

1. **Benchmarks show <5% overhead** vs manual code (measure with production-like data)
2. **Implementation takes <3 days** (if it takes longer, we're over-engineering)
3. **Code reduction confirmed** in real migration (measure actual before/after in production stores)
4. **Zero new bugs** introduced by helpers in first 30 days of use

Adoption metrics after implementation:

1. **Lines of code** - Track reduction in store files that migrate
2. **Index bugs** - Monitor for stale index issues in stores using `UpdateWithIndexes`
3. **Developer feedback** - Are the helpers actually useful or just different boilerplate?

## References

- [ADR-0003: Simple API Layer](./0003-simple-api-layer.md)
- [ADR-0005: Core API Helpers Guidance](./0005-core-api-helpers-guidance.md)
- Go Generics Best Practices: https://go.dev/blog/when-generics
