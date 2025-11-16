# ADR-0009: Redis-Only Indexing Architecture

**Status:** Accepted
**Date:** 2025-01-16
**Authors:** Adrian McPhee
**Supersedes:** Portions of ADR-0008 (file-based indexing)

## Context

Smarterbase previously supported two indexing systems:

1. **File-based indexes** - 1:1 unique mappings stored as JSON files in `indexes/` directory
2. **Redis indexes** - Multi-value indexes stored in Redis using sorted sets

This dual-indexing approach was designed to provide:
- Graceful degradation when Redis unavailable
- Simple unique lookups via filesystem
- Scalable multi-value indexes via Redis

However, this created several problems:

### Problems with Dual-Indexing

1. **Increased Complexity**
   - Two completely different indexing systems to maintain
   - Complex decision logic: when to use file vs Redis indexes
   - `AutoRegisterIndexes()` needed to handle both `unique` and `multi` tag types
   - `IndexManager` coordinated updates across both systems

2. **Filesystem I/O Overhead**
   - File-based index lookups require disk reads
   - Significantly slower than in-memory Redis lookups
   - Index files clutter the `/data/*/indexes/` directories

3. **Redis Already Required**
   - Redis is already a hard dependency for:
     - Rate limiting (authentication)
     - Photo storage metadata
     - Session management
     - Distributed locks
   - No real benefit to "graceful degradation" when Redis down = app down

4. **API Confusion**
   - Users confused about when to use `sb:"index,unique"` vs `sb:"index,multi"`
   - Two different query patterns: `indexer.QueryIndex()` vs `redisIndexer.Query()`
   - Unclear performance characteristics

5. **Index Inconsistency Risk**
   - File indexes updated separately from Redis indexes
   - Best-effort updates could lead to stale file indexes
   - No transactional guarantees across systems

## Decision

**Remove file-based indexing entirely. Use Redis-only for all indexes.**

### Changes Made

#### 1. Core Library Simplification

**Removed:**
- `indexer.go` - Entire file (120 lines)
- `indexer_test.go` - Entire file (291 lines)
- `Indexer` type and all related functions
- `IndexSpec` type for file-based indexes
- `registerUniqueIndex()` function from `auto_indexing.go`

**Updated:**
- `AutoRegisterIndexes()` - Signature changed from `(fileIndexer, redisIndexer, entityType, example)` to `(redisIndexer, entityType, example)`
- `IndexManager` - Removed `fileIndexer` field and `WithFileIndexer()` method
- `NewCascadeIndexManager()` - Signature changed from `(base, indexer, redisIndexer)` to `(base, redisIndexer)`
- `ParseIndexTag()` - Now rejects `sb:"index,unique"` tags (returns `false`)

#### 2. Unified Struct Tags

**Before:**
```go
type User struct {
    Email string `json:"email" sb:"index,unique"`  // File-based unique index
    Role  string `json:"role" sb:"index,multi"`     // Redis multi-value index
}
```

**After:**
```go
type User struct {
    Email string `json:"email" sb:"index"`  // Redis multi-value index
    Role  string `json:"role" sb:"index"`    // Redis multi-value index
}
```

All indexes are now Redis-based multi-value indexes. For fields that are logically unique (like email), the application layer enforces uniqueness constraints.

#### 3. Simplified API

**Before:**
```go
indexer := smarterbase.NewIndexer(base)
smarterbase.AutoRegisterIndexes(indexer, redisIndexer, "users", &User{})
im := smarterbase.NewIndexManager(base).
    WithFileIndexer(indexer).
    WithRedisIndexer(redisIndexer)
```

**After:**
```go
smarterbase.AutoRegisterIndexes(redisIndexer, "users", &User{})
im := smarterbase.NewIndexManager(base, redisIndexer)
```

#### 4. Documentation Updates

Updated all documentation to reflect Redis-only architecture:
- ADR-0003 (Simple API Layer)
- ADR-0008 (Ergonomic Indexing and Cascades)
- Website HTML files (index.html, examples.html)
- Example code (03-with-indexing, 04-versioning)
- DATASHEET.md
- RFC_SMARTERBASE_IMPROVEMENTS.md
- Simple API documentation

## Consequences

### Positive

✅ **Simpler Architecture**
- Single indexing system instead of dual systems
- Easier to understand and maintain
- Clearer performance characteristics (always in-memory)

✅ **Faster Performance**
- All index lookups use in-memory Redis
- No filesystem I/O for index queries
- Consistent O(log N) or better lookup times

✅ **Less Code**
- ~200+ lines removed from core library
- Fewer test files to maintain
- Simpler store initialization

✅ **Cleaner Filesystem**
- No `indexes/` directories cluttering `/data`
- All index data consolidated in Redis
- Easier backup/restore (just Redis + data files)

✅ **Better Developer Experience**
- Single `sb:"index"` tag for all indexes
- No confusion about unique vs multi-value
- Consistent query API

✅ **Index Consistency**
- Single source of truth for all indexes
- No risk of file/Redis index divergence
- Simpler index repair logic

### Negative

⚠️ **Redis Now Required**
- Redis is a hard dependency for indexing
- Cannot run without Redis (but already true for rate limiting, sessions, etc.)
- Local development requires Redis running

⚠️ **Migration Required**
- Existing file-based indexes are no longer used
- Applications must update struct tags from `unique` to `multi`
- Must update `AutoRegisterIndexes()` call sites

⚠️ **No Unique Constraint Enforcement**
- Redis multi-value indexes don't enforce uniqueness
- Application layer must handle uniqueness validation
- Risk of duplicate entries if not properly validated

### Neutral

🔵 **Deployment Considerations**
- Redis must be available before app starts
- Index data lives in Redis (already true for most indexes)
- Redis memory usage slightly higher (but negligible)

## Implementation

### Migration Path

1. **Update Struct Tags**
   ```go
   // Old
   Email string `sb:"index,unique"`

   // New
   Email string `sb:"index"`
   ```

2. **Update Index Registration**
   ```go
   // Old
   indexer := smarterbase.NewIndexer(base)
   smarterbase.AutoRegisterIndexes(indexer, redisIndexer, "users", &User{})

   // New
   smarterbase.AutoRegisterIndexes(redisIndexer, "users", &User{})
   ```

3. **Update IndexManager Initialization**
   ```go
   // Old
   im := smarterbase.NewIndexManager(base).
       WithFileIndexer(indexer).
       WithRedisIndexer(redisIndexer)

   // New
   im := smarterbase.NewIndexManager(base, redisIndexer)
   ```

4. **Clean Up File Indexes** (optional)
   ```bash
   rm -rf /data/*/indexes/
   ```

### Backward Compatibility

⚠️ **Breaking Changes:**
- `Indexer` type removed - compile-time error
- `AutoRegisterIndexes()` signature changed - compile-time error
- `WithFileIndexer()` method removed - compile-time error
- `sb:"index,unique"` tags no longer recognized - runtime warning/error

All breaking changes result in compile-time errors, making migration straightforward and safe.

## Alternatives Considered

### 1. Keep File-Based Indexes for Unique Constraints

**Rejected because:**
- Adds complexity for minimal benefit
- Redis already required, so no graceful degradation benefit
- Application-layer uniqueness validation is more flexible

### 2. Use Redis SETNX for Unique Indexes

**Rejected because:**
- Requires different Redis commands for unique vs multi-value
- Still maintains dual indexing mental model
- Application-layer validation is sufficient

### 3. Make Redis Optional with Fallback to Full Scans

**Rejected because:**
- Poor performance without indexes
- Redis already required for other features
- Adds complexity for edge case

## References

- ADR-0008: Ergonomic Indexing and Cascades
- ADR-0003: Simple API Layer
- [Redis Sorted Sets Documentation](https://redis.io/docs/data-types/sorted-sets/)
- [Smarterbase DATASHEET.md](../../DATASHEET.md)

## Verification

✅ Build verification:
- `smarterbase` builds successfully
- `tuinplan/platform` builds successfully
- All tests pass

✅ Documentation updated:
- 3 ADR files updated
- 2 website HTML files updated
- 5 example code files updated
- 2 reference documentation files updated

✅ Code changes:
- 2 files removed (indexer.go, indexer_test.go)
- 4 core library files updated
- 24 store files updated
- 4 domain model files updated
