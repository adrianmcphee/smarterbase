# ADR-0004: Simple API Versioning Discoverability

**Status:** Accepted
**Date:** 2025-10-14
**Related ADRs:**
- [ADR-0001: Schema Versioning and Migrations](0001-schema-versioning-and-migrations.md)
- [ADR-0003: Simple API Layer](0003-simple-api-layer.md)

## Context

Versioning already works with Simple API. The migration system operates via `store.GetJSON()`, which `Collection.Get()`, `Collection.All()`, etc. call automatically.

**Current usage:**
```go
// Register migrations with Core API
smarterbase.Migrate("User").From(0).To(2).Do(func(data map[string]interface{}) (map[string]interface{}, error) {
    // Migration logic
    return data, nil
})

// Use Simple API - migrations apply automatically
db := simple.MustConnect()
users := simple.NewCollection[UserV2](db)
user, _ := users.Get(ctx, "user-123")  // ✅ Auto-migrates!
```

**The problem is discoverability:** Users don't know versioning works with Simple API because:
1. Documentation doesn't mention it
2. Requires importing both `simple` and `smarterbase` packages
3. No examples showing the pattern

## The Core Question

**Is schema versioning "simple" or "advanced"?**

**It's ADVANCED:**
- Added months after initial development, not day one
- Touches production data (high stakes, requires careful testing)
- Can't be simplified—migration bugs corrupt data
- Type safety doesn't prevent logic errors
- Requires understanding: version chains, rollback strategies, lazy evaluation, migration policies

**When you're ready to add versioning, you're ready for explicit syntax.**

Migrations are hard because the problem is hard, not because the API is complex.

## Options Considered

### Option 1: Status Quo
Keep versioning as Core API-only. Users import both packages.

**Verdict:** Works, but poor discoverability.

---

### Option 2: Add Abstraction Layers

Various approaches to "simplify" migrations:
- **Typed helpers**: 4 JSON operations per migration for "type safety" that doesn't prevent logic bugs
- **Struct tags**: DSL in tags (`sb:"migrate:name>split(0)"`) - can't express real migrations
- **Collection methods**: Go methods can't have type parameters
- **Fluent builders**: Duplicates Core API registry

**Verdict:** NO. Migrations touch production data. Abstraction doesn't make hard problems easy. Type safety doesn't prevent you from writing `data["first_name"] = data["last_name"]` (logic bug). You still need UserV0, UserV1, UserV2 definitions. You add complexity for zero actual benefit.

---

### Option 3: Re-export + Documentation ✅

**One line of code:**
```go
// simple/migration.go
func Migrate(typeName string) *smarterbase.MigrationBuilder {
    return smarterbase.Migrate(typeName)
}
```

Plus documentation and an example. That's it.

**Why this works:**
- Zero abstraction - just a different import path to the same function
- No performance overhead - direct pass-through
- Single source of truth - Core API registry
- Solves discoverability - users find it via `simple` package

**Verdict:** Solves the actual problem (discoverability) without creating new problems (abstraction, performance, complexity).

## Decision

**Option 3: Re-export + Documentation**

Implementation:
1. Add `simple/migration.go` (one line of code)
2. Create `examples/simple/04-versioning/`
3. Update `simple/doc.go` with versioning section
4. Mention on website

**Why:** Solves discoverability. One line of code. Zero abstraction. No performance overhead. Maintains "explicit is better than implicit" philosophy.

## Consequences

**Good:**
- Discoverability solved
- One line of code to maintain
- Zero performance overhead
- Philosophy maintained

**Trade-offs:**
- Still uses `map[string]interface{}` (not type-safe)
  - Intentional: migrations touch production data, should be explicit
  - Type safety doesn't prevent logic bugs anyway

## Implementation

**`simple/migration.go` (~15 lines with docs):**
```go
package simple

import "github.com/adrianmcphee/smarterbase"

// Migrate registers schema migrations that work automatically with Simple API.
// Migrations apply lazily when reading data via Collection.Get(), Collection.All(), etc.
//
// For complete documentation see:
// https://github.com/adrianmcphee/smarterbase/blob/main/docs/adr/0001-schema-versioning-and-migrations.md
func Migrate(typeName string) *smarterbase.MigrationBuilder {
    return smarterbase.Migrate(typeName)
}
```

**Also create:**
- `examples/simple/04-versioning/` - Show UserV0 → UserV2 migration
- Update `simple/doc.go` - Add versioning section
- Update website - Mention versioning works with Simple API

## Review Criteria

- [x] Implement `simple/migration.go`
- [x] Create `examples/simple/04-versioning/`
- [x] Update `simple/doc.go`
- [x] Verify re-export works (tested and working)

## References

- ADR-0001: Schema Versioning and Migrations
- ADR-0003: Simple API Layer
