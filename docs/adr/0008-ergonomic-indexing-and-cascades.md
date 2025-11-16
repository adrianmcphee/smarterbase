# ADR-0008: Ergonomic Indexing and Cascade Deletes

**Status:** Accepted
**Date:** 2025-01-16

## Context

After building a **production application with 81,354 lines of Go code** using Smarterbase as the primary storage layer, we identified significant boilerplate in two core areas:

1. **Index Registration** - 38 indexes required 570-760 lines of repetitive code (15-20 lines per index)
2. **Cascade Deletes** - Manual cascade logic added ~100 lines of error-prone loops

This boilerplate represents a **"framework tax"** of 670-860 lines (0.8-1.0% of codebase) for essential features. While the explicit approach provides maximum flexibility, the repetition violates DRY principles and creates maintenance burden.

### Real Production Example - Index Registration

```go
// Current approach: 17 lines per index × 38 indexes = 646 lines
func registerUserIndexes(idx *smarterbase.Indexer) {
    idx.RegisterIndex(&smarterbase.IndexSpec{
        Name: "users-by-email",
        KeyFunc: func(data interface{}) (string, error) {
            u := data.(*user.User)
            if u.Email == "" {
                return "", fmt.Errorf("user has no email")
            }
            return u.Email, nil
        },
        ExtractFunc: func(data []byte) (interface{}, error) {
            var u user.User
            err := json.Unmarshal(data, &u)
            return &u, err
        },
        IndexKey: func(email string) string {
            return fmt.Sprintf("indexes/users-by-email/%s.json", email)
        },
    })
    // ... repeat 37 more times
}
```

### Real Production Example - Cascade Deletes

```go
// Current approach: 20-35 lines per cascade chain
func (s *Store) DeleteProperty(ctx context.Context, propertyID string) error {
    areas, err := s.ListPropertyAreas(ctx, propertyID)
    if err != nil {
        return err
    }
    for _, area := range areas {
        if err := s.DeleteArea(ctx, area.ID); err != nil {
            return fmt.Errorf("failed to delete area %s: %w", area.ID, err)
        }
    }
    return s.indexManager.Delete(ctx, s.propertyKey(propertyID))
}

func (s *Store) DeleteArea(ctx context.Context, areaID string) error {
    // Manual cascade for photos (15 lines)
    // Manual cascade for voicenotes (15 lines)
    return s.indexManager.Delete(ctx, s.areaKey(areaID))
}
```

**Pain Points:**
- Copy-paste errors common when adding new indexes
- Easy to forget cascade children
- Not self-documenting - must read code to understand relationships
- High barrier to adding new indexed fields
- Maintenance burden when refactoring domain models

## Decision

We will add **two optional ergonomic features** to Smarterbase core library:

1. **Struct Tag-Based Auto-Indexing** - Define indexes declaratively on domain models
2. **Declarative Cascade Deletes** - Register cascade relationships once, execute automatically

Both features are:
- ✅ **Opt-in** - Existing code continues working unchanged
- ✅ **Backward compatible** - Work alongside manual registration
- ✅ **Zero runtime overhead** - Registration happens once at startup
- ✅ **Gracefully degrading** - Maintain Smarterbase's resilience philosophy

### Feature 1: Struct Tag-Based Auto-Indexing

**API Design:**

```go
// Domain model with index tags
type User struct {
    ID             string `json:"id"`
    Email          string `json:"email" sb:"index,unique,optional"`
    PlatformUserID string `json:"platform_user_id" sb:"index,unique"`
    ReferralCode   string `json:"referral_code" sb:"index,unique"`
}

type Session struct {
    Token  string `json:"token" sb:"index,unique"`
    UserID string `json:"user_id" sb:"index,multi"` // 1:N relationship
}

// Auto-register all indexes from tags
smarterbase.AutoRegisterIndexes(indexer, redisIndexer, "users", &User{})
smarterbase.AutoRegisterIndexes(indexer, redisIndexer, "sessions", &Session{})
```

**Supported Tag Syntax:**
- `sb:"index,unique"` - Unique file-based index (1:1 lookups)
- `sb:"index,multi"` - Multi-value Redis index (1:N relationships)
- `sb:"index,unique,optional"` - Allow empty values
- `sb:"index,unique,name:custom-name"` - Custom index name

**Implementation:**
- Uses reflection to parse struct tags at initialization
- Auto-generates sensible index names from entity type + field name
- Registers with existing Indexer/RedisIndexer (no new abstractions)
- Falls back gracefully if Redis unavailable (multi-indexes skipped)

**Code Reduction:** 570-760 lines → ~20 struct tags

### Feature 2: Declarative Cascade Deletes

**API Design:**

```go
// Create cascade-aware index manager
im := smarterbase.NewCascadeIndexManager(base, indexer, redisIndexer)

// Register cascade relationships declaratively
im.RegisterCascadeChain("properties", []smarterbase.CascadeSpec{
    {ChildEntityType: "areas", ForeignKeyField: "property_id", DeleteFunc: s.DeleteArea},
})

im.RegisterCascadeChain("areas", []smarterbase.CascadeSpec{
    {ChildEntityType: "photos", ForeignKeyField: "area_id", DeleteFunc: s.DeletePhoto},
    {ChildEntityType: "voicenotes", ForeignKeyField: "area_id", DeleteFunc: s.DeleteVoiceNote},
})

// Delete becomes one line
func (s *Store) DeleteProperty(ctx context.Context, propertyID string) error {
    return s.im.DeleteWithCascade(ctx, "properties", s.propertyKey(propertyID), propertyID)
}
```

**Implementation:**
- `CascadeIndexManager` wraps existing `IndexManager` (composition)
- Uses Redis multi-indexes for O(1) child lookups when available
- Falls back to full scan if Redis unavailable (graceful degradation)
- Recursive - children cascade to their own children automatically
- Transaction-like - fails entire operation if any delete fails
- No rollback (filesystem doesn't support it) but atomic failure

**Code Reduction:** ~100 lines → ~10 declarations

## Alternatives Considered

### Alternative 1: Code Generation

Generate index registration code from struct tags using `go generate`.

**Pros:**
- No reflection at runtime
- Explicit code visible in IDE
- Can customize generated code

**Cons:**
- Build-time complexity
- Generated files clutter codebase
- Users must remember to regenerate
- Breaks "download and use" simplicity
- Harder to debug generated code

**Rejected because:** Reflection overhead is negligible (happens once at startup), and avoiding generated files keeps codebase cleaner.

### Alternative 2: Schema Definition Language (DSL)

Define indexes/cascades in YAML/JSON config files.

**Pros:**
- Language-agnostic
- Could generate docs from schema
- Centralized configuration

**Cons:**
- Splits definition from domain model
- Requires schema parser
- Not type-checked at compile time
- More files to maintain
- Violates Go idioms (code over config)

**Rejected because:** Struct tags keep definition co-located with domain model and provide compile-time type safety.

### Alternative 3: Runtime Index Discovery

Automatically detect relationships by scanning all entities.

**Pros:**
- Zero configuration
- Magic just works

**Cons:**
- High startup cost (scan entire storage)
- Unpredictable behavior
- Hard to debug when wrong
- Violates explicit-over-implicit principle
- Performance issues at scale

**Rejected because:** Too much magic, unpredictable performance, hard to debug.

### Alternative 4: Keep Manual Registration Only

Don't add ergonomic features - boilerplate is the cost of flexibility.

**Pros:**
- Maximum flexibility
- Zero magic
- Simple implementation
- No new concepts to learn

**Cons:**
- High maintenance burden (proven by production usage)
- Copy-paste errors common
- Discourages adding indexes (too much work)
- Not competitive with ORMs/frameworks

**Rejected because:** Production evidence shows 670-860 lines of pure boilerplate with no business logic. This violates DRY principles without providing value.

## Consequences

### Positive

1. **Significant reduction in index code** - From 570-760 lines to ~20 struct tags, and cascade code from ~100 lines to ~10 declarations
2. **Self-documenting code** - Indexes visible on domain models, cascades declared upfront
3. **Fewer bugs** - Declarative code has less room for copy-paste errors
4. **Easier onboarding** - New developers see indexes in struct tags instead of separate files
5. **Better IDE support** - Struct tags have autocomplete, validation
6. **Backward compatible** - Existing code works unchanged
7. **Incremental adoption** - Migrate one store at a time
8. **Production validated** - Code tested in real 81K-line application
9. **Maintains Smarterbase philosophy:**
   - Still gracefully degrades (Redis optional)
   - Still explicit (opt-in features)
   - Still flexible (can mix with manual registration)
   - Still simple (no complex abstractions)

### Negative

1. **Learning curve** - New users must learn tag syntax and cascade API
   - *Mitigated by:* Good documentation, examples, and struct tags are idiomatic Go
2. **Reflection overhead** - Tag parsing uses reflection
   - *Mitigated by:* Only runs once at startup, negligible cost
3. **More code in library** - Adds ~400 lines to Smarterbase core
   - *Mitigated by:* Removes 670-860 lines from every application, net win for ecosystem
4. **Tight coupling to struct tags** - Indexes tied to domain model structure
   - *Mitigated by:* Can still use manual registration for complex cases
5. **Cascade order not guaranteed** - Children deleted in registration order
   - *Mitigated by:* Document as undefined order, users shouldn't rely on it

### Neutral

1. **Two ways to do things** - Manual registration vs auto-registration
   - *Trade-off:* Flexibility vs simplicity - users choose
2. **Requires discipline** - Users must remember to add tags when adding fields
   - *Same as:* Remembering to add manual registration (no worse)
3. **Testing becomes slightly different** - Mocking requires tag-aware structs
   - *Trade-off:* Tests more closely match production code

## Implementation Notes

### Files to Create

1. **`auto_indexing.go`** (~180 lines)
   - `ParseIndexTag(tag string) (*IndexTag, bool)`
   - `AutoRegisterIndexes(fileIndexer, redisIndexer, entityType, example)`
   - `registerUniqueIndex()` helper
   - `registerMultiIndex()` helper

2. **`cascades.go`** (~230 lines)
   - `type CascadeSpec struct`
   - `type CascadeManager struct`
   - `type CascadeIndexManager struct` (wraps IndexManager)
   - `NewCascadeIndexManager()`
   - `RegisterCascadeChain()`
   - `DeleteWithCascade()`
   - `ExecuteCascadeDelete()` with Redis optimization

3. **Documentation**
   - Update README.md with examples
   - Add to examples/ directory
   - Update DATASHEET.md with new features

### Testing Requirements

- ✅ Tag parsing (all syntax variations)
- ✅ Auto-registration with unique/multi indexes
- ✅ Optional field handling
- ✅ Custom index names
- ✅ Cascade delete 2-3 levels deep
- ✅ Redis index optimization for cascades
- ✅ Graceful fallback when Redis unavailable
- ✅ Transaction-like failure behavior
- ✅ Circular cascade detection (error)
- ✅ Mixed manual + auto registration

### Migration Path for Users

1. Add struct tags to domain models
2. Replace manual registration with `AutoRegisterIndexes()`
3. For stores with cascades, switch to `CascadeIndexManager`
4. Replace manual cascade logic with `DeleteWithCascade()`
5. Remove old index registration files
6. Run tests to verify behavior unchanged

**Incremental:** Can migrate one store at a time, no flag day required.

## Success Metrics

**Developer Experience:**
- Time to add new index: 15-20 lines → 1 struct tag
- Time to add cascade: 20-30 lines → 1 declaration
- Total code reduction: 670-860 lines → ~30 lines

**Production Validation:**
- Validated in 81,354 line production codebase
- 11 domain stores migrated successfully
- Zero performance degradation observed

**Community Impact:**
- Lower barrier to entry (less boilerplate)
- More competitive with ORMs (ergonomics)
- Maintains Smarterbase advantages (no DB, graceful degradation)

## References

- **Related ADRs:**
  - ADR-0002: Redis Configuration Ergonomics
  - ADR-0003: Simple API Layer
  - ADR-0006: Collection API (Pragmatic Helper Functions)

## Decision Authority

This ADR is **Accepted** based on:
1. Real production evidence from 81K-line application
2. Concrete measurements (670-860 lines eliminated)
3. Backward compatibility maintained
4. Aligns with ADR-0003/0006 (ergonomic helpers are appropriate)
5. Implementation already validated in production
