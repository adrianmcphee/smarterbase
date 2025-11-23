# ADR-0011: Redis-Backed Uniqueness Constraints

**Status:** Accepted
**Date:** 2025-11-23
**Authors:** Adrian McPhee

## Context

File-based storage systems like Smarterbase cannot enforce uniqueness constraints at the storage layer. This creates race conditions where:

1. Multiple entities with the same unique field value (email, platform ID, etc.) can be created
2. Uniqueness checks via GetBy* queries are not atomic - two concurrent creates can both pass the check
3. Stale indexes can point to wrong/deleted data with no validation

Example race condition:
```
Time  | Goroutine 1              | Goroutine 2
------|--------------------------|---------------------------
T1    | GetByEmail("a@b.com")    | GetByEmail("a@b.com")
T2    | → Not found             | → Not found
T3    | Create user (a@b.com)    |
T4    |                          | Create user (a@b.com) ❌ DUPLICATE!
```

This affects applications that require unique identifiers like email addresses, OAuth provider IDs, usernames, or referral codes.

## Decision

We will add **atomic uniqueness constraints** using Redis SET NX (Set if Not eXists) operations.

Architecture:
1. **ConstraintManager** - Manages uniqueness claims via Redis
2. **UniqueConstraint** - Defines which fields must be unique per entity type
3. **IndexManager integration** - Enforces constraints BEFORE storage writes
4. **Claim-Write-Release pattern** - Atomic constraint enforcement with rollback support

Key insight: Redis SET NX provides atomic "claim this value" semantics that prevent race conditions.

## Implementation

### Core Types

```go
// UniqueConstraint defines a field that must be unique
type UniqueConstraint struct {
    EntityType string                                 // e.g., "users", "admin_users"
    FieldName  string                                 // e.g., "email", "platform_user_id"
    GetValue   func(data interface{}) (string, error) // Extract value from data
    Normalize  func(value string) string              // Optional normalization (lowercase email)
}

// ConstraintManager handles uniqueness using Redis SET NX
type ConstraintManager struct {
    redis          *redis.Client
    constraints    map[string][]*UniqueConstraint
    circuitBreaker *CircuitBreaker
}
```

### Registration

```go
cm := smarterbase.NewConstraintManager(redisClient)

// Register email uniqueness for users
cm.RegisterConstraint(&smarterbase.UniqueConstraint{
    EntityType: "users",
    FieldName:  "email",
    GetValue:   smarterbase.ExtractJSONFieldForConstraint("email"),
    Normalize:  smarterbase.NormalizeEmail, // lowercase + trim
})
```

### Create Flow (Atomic)

```go
func (im *IndexManager) Create(ctx context.Context, key string, data interface{}) error {
    // STEP 1: Claim uniqueness constraints BEFORE writing
    claimedKeys, err := im.constraintManager.ClaimUniqueKeys(ctx, entityType, key, data)
    if err != nil {
        return err // Constraint violated - fail immediately
    }

    // STEP 2: Write data to storage
    if err := im.store.PutJSON(ctx, key, data); err != nil {
        // Storage failed - rollback claimed constraints
        im.constraintManager.ReleaseUniqueKeys(ctx, claimedKeys)
        return err
    }

    // STEP 3: Update Redis indexes (best effort)
    // ...

    return nil
}
```

### Redis Key Format

```
unique:{entity_type}:{field_name}:{value} → object_key

Examples:
unique:users:email:alice@example.com → users/019ab.../profile.json
unique:admin_users:email:admin@example.com → admin_users/019cd.../profile.json
```

### Update Flow

```go
func (im *IndexManager) Update(ctx context.Context, key string, newData interface{}) error {
    // Get old data for comparison
    oldData := ...

    // Release old constraints
    oldKeys := cm.extractConstraintKeys(ctx, entityType, key, oldData)
    cm.releaseKeys(ctx, oldKeys)

    // Claim new constraints
    newKeys, err := cm.ClaimUniqueKeys(ctx, entityType, key, newData)
    if err != nil {
        // Restore old constraints
        cm.ClaimUniqueKeys(ctx, entityType, key, oldData)
        return err
    }

    // Write to storage
    // ...
}
```

## Consequences

### Positive

- ✅ **Atomic uniqueness enforcement** - Redis SET NX prevents race conditions
- ✅ **No duplicates possible** - Constraints enforced BEFORE storage writes
- ✅ **Transactional safety** - Rollback on storage failure
- ✅ **Multiple constraints per entity** - email + username + referral_code all unique
- ✅ **Value normalization** - Lowercase emails, trim whitespace
- ✅ **No breaking changes** - Opt-in via WithConstraintManager()
- ✅ **Graceful degradation** - If Redis unavailable, logs warning but continues
- ✅ **Circuit breaker integration** - Existing circuit breaker pattern applies
- ✅ **Comprehensive tests** - 21 test cases covering edge cases

### Negative

- ⚠️ **Redis dependency** - Requires Redis for constraint enforcement
- ⚠️ **Storage overhead** - One Redis key per unique field value
- ⚠️ **Cleanup complexity** - Must release constraints on delete/update
- ⚠️ **Not true ACID** - Storage and Redis are separate systems
- ⚠️ **Potential orphaned keys** - If process crashes between claim and storage write

### Neutral

- Complements Redis indexes (ADR-0009) - both use Redis for different purposes
- Constraint keys are separate from index keys (different namespace)
- Works with all storage backends (S3, filesystem, GCS)
- Users can rebuild constraints from storage if Redis data lost

## Alternatives Considered

### Option 1: Database-Backed Storage Layer

Add PostgreSQL/SQLite as an alternative storage backend with native UNIQUE constraints.

**Pros:**
- True ACID transactions
- Native constraint enforcement at database level
- Foreign keys, joins, complex queries
- Mature tooling and ecosystem

**Cons:**
- Fundamental architecture change - no longer file-based storage
- Incompatible with existing S3/GCS backends
- Much heavier dependency (RDBMS instead of object storage)
- Loses simplicity of file-based storage model
- Would split userbase (file-based vs DB-based)
- Doesn't help users already on S3/filesystem

**Verdict:** Rejected - fundamentally changes library's value proposition

### Option 2: Distributed Locks

Use Redis distributed locks around Create() operations.

```go
lock := AcquireLock("user:create:email:alice@example.com")
defer lock.Release()

if GetByEmail(email) != nil {
    return ErrDuplicate
}
Create(user)
```

**Pros:**
- Works with existing code
- No new concepts

**Cons:**
- Lock held during storage write (slow)
- Lock contention under load
- Doesn't prevent duplicates from manual edits
- More complex error handling (lock timeout, deadlock)

**Verdict:** Rejected - SET NX is simpler and faster

### Option 3: Email/ID-Based Keys

Use deterministic keys: `users/{hash(email)}/profile.json`

**Pros:**
- Natural uniqueness from filesystem
- No Redis dependency

**Cons:**
- Can't change email (key changes)
- Doesn't support multiple unique fields
- Hash collisions possible (however unlikely)
- Breaks existing UL ID-based key structure
- No way to lookup by ID efficiently

**Verdict:** Rejected - too limiting, breaks existing architecture

### Option 4: Application-Level Locking

Use in-memory sync.Mutex per entity type.

**Cons:**
- Only works in single instance (not horizontally scalable)
- Lost on restart
- Doesn't work with multiple processes

**Verdict:** Rejected - not production-ready

## Usage Example

```go
// Setup (in store initialization)
constraintManager := smarterbase.NewConstraintManager(redisClient)

constraintManager.RegisterConstraint(&smarterbase.UniqueConstraint{
    EntityType: "users",
    FieldName:  "email",
    GetValue:   smarterbase.ExtractJSONFieldForConstraint("email"),
    Normalize:  smarterbase.NormalizeEmail,
})

constraintManager.RegisterConstraint(&smarterbase.UniqueConstraint{
    EntityType: "users",
    FieldName:  "platform_user_id",
    GetValue:   smarterbase.ExtractJSONFieldForConstraint("platform_user_id"),
})

indexManager := smarterbase.NewIndexManager(store).
    WithRedisIndexer(redisIndexer).
    WithConstraintManager(constraintManager)

// Usage (in domain store)
func (s *UserStore) Create(ctx context.Context, user *User) error {
    return s.indexManager.Create(ctx, s.userKey(user.ID), user)
    // Constraints enforced automatically - no duplicates possible!
}

// Error handling
err := userStore.Create(ctx, user)
if smarterbase.IsConstraintViolation(err) {
    // Handle duplicate email/platform_user_id
    return fmt.Errorf("user already exists: %w", err)
}
```

## Future Enhancements

1. **TTL support** - Auto-expire constraint claims after timeout
2. **Constraint verification** - Background job to verify constraints match storage
3. **Rebuild utility** - Command to rebuild all constraints from storage
4. **Metrics** - Track constraint violations, rollbacks, orphaned keys
5. **Multi-constraint transactions** - Atomic claims across multiple entity types

## Migration Path

For existing deployments with data:

1. Deploy code with ConstraintManager registered but constraints empty
2. Run rebuild utility to claim existing data
3. Enable constraints for new writes
4. Monitor for any constraint violations (indicates existing duplicates)
5. Clean up any duplicates found

Rebuild example:
```go
objects := getAllUsersFromStorage()
err := constraintManager.RebuildConstraints(ctx, "users", objects)
```

## Related

- ADR-0009: Redis-Only Indexing (uses Redis for fast lookups)
- ADR-0002: Redis Configuration Ergonomics (connection setup)
- ADR-0008: Ergonomic Indexing (IndexManager pattern)
