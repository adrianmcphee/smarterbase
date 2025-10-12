# ADR-0001: Schema Versioning and Migrations

**Status:** Accepted
**Date:** 2025-10-12
**Related Release:** [v1.3.0](https://github.com/adrianmcphee/smarterbase/releases/tag/v1.3.0)
**Related Commit:** [6d5fe08](https://github.com/adrianmcphee/smarterbase/commit/6d5fe08f0566d99ced1ed387a14a596fe027ae41)
**Changelog:** [CHANGELOG.md](../../CHANGELOG.md#130)

## Context

We received feedback suggesting adding version numbers to JSON documents to enable schema migrations. This raised the question: how should SmarterBase handle schema evolution over time?

Traditional approaches include:
- **ALTER TABLE statements** - require downtime, complex rollback strategies
- **Backfill scripts** - error-prone, require coordination with deployments
- **Envelope patterns** - add storage overhead, break query filters
- **Automatic versioning** - too much "magic", unclear migration paths

## Options Considered

### 1. Envelope Pattern
Wrap all data in `{"v": 1, "data": {...}}`

**Rejected**: Transparency problem - if the library auto-strips the envelope, how does it know what version to expect? Also adds storage overhead and breaks direct JSON queries.

### 2. Struct Tags
```go
type User struct {
    Name string `json:"name" smarterbase:"version=2"`
}
```

**Rejected**: Verbose, less flexible, ties version to Go code rather than data.

### 3. Hash-Based Auto-Versioning
Hash struct definition → base58 version identifier

**Rejected**: Hashes have no ordering - cannot determine migration path (which version is "newer"?).

### 4. Automated Migration Registry
Code generation or reflection to auto-discover migrations

**Rejected**: Too much magic, unclear migration logic, difficult to debug.

### 5. Explicit Opt-In with Manual Migrations ✅
- Optional `_v int` field in structs
- Manual migration registration via fluent API
- Automatic migration execution on read (lazy)
- Zero overhead when not used

**Accepted**: Best balance of explicitness, power, and simplicity.

## Decision

We will implement **explicit opt-in versioning with zero magic**:

1. **Version Field**: Developers add optional `_v int` field to structs that need versioning
2. **Migration Registry**: Register migrations at app startup using fluent API:
   ```go
   smarterbase.Migrate("User").From(0).To(1).Do(func(data map[string]interface{}) ...)
   ```
3. **Lazy Migration**: Data migrates automatically on read when version mismatch detected
4. **Migration Chaining**: BFS algorithm finds shortest path between versions (0→1→2→3)
5. **Two Policies**:
   - `MigrateOnRead`: Transform in-memory only (default)
   - `MigrateAndWrite`: Write back migrated data to storage
6. **Zero Overhead**: Fast path when no migrations registered (~50ns version check)

## Consequences

### Positive
- ✅ No downtime for schema changes
- ✅ Explicit control over migration logic
- ✅ Zero cost when feature not used
- ✅ Gradual migration via write-back policy
- ✅ Type-safe migrations (just Go functions)
- ✅ Clear migration paths (manual registration)

### Negative
- ⚠️ Manual migration registration required
- ⚠️ First read of old data adds latency (~5-10ms per version step)
- ⚠️ Developers must remember to increment versions

### Neutral
- Migration chaining enables complex version paths
- Helper functions reduce boilerplate (Split, AddField, RenameField, RemoveField)
- Migration state lives in global registry

## Implementation

Files created/modified:
- `migration.go` - Core migration system with BFS path finding
- `store.go` - Integration into GetJSON for automatic migration
- `migration_test.go` - Comprehensive test coverage
- `examples/schema-migrations/` - Working example with V0→V1→V2→V3 evolution

## Philosophy

**Explicit is better than implicit.**

Developers should understand and control their migration logic. The library handles the mechanical work (version checking, path finding, chaining) but the developer defines the transformations.

This aligns with Go's philosophy: clear is better than clever.
