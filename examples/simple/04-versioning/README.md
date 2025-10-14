# Schema Versioning with Simple API

**The wow factor: Old data migrates automatically.**

Register migrations once at startup. Old data transforms automatically when read. No downtime, no backfill scripts.

## Running

```bash
go run main.go
```

## What This Example Shows

This example demonstrates schema evolution from V0 → V2:

**V0 (Original):**
```go
type UserV0 struct {
    ID    string
    Name  string  // Single field
    Email string
}
```

**V2 (Evolved):**
```go
type UserV2 struct {
    V         int     // Version field
    ID        string
    FirstName string  // Split from Name
    LastName  string  // Split from Name
    Email     string
    Phone     string  // New field
}
```

## How It Works

### 1. Register Migration Before Connecting

```go
simple.Migrate("User").From(0).To(2).Do(func(data map[string]interface{}) (map[string]interface{}, error) {
    // Split name into first_name and last_name
    parts := strings.SplitN(data["name"].(string), " ", 2)
    data["first_name"] = parts[0]
    data["last_name"] = parts[1]
    delete(data, "name")

    // Add new field
    data["phone"] = ""
    data["_v"] = 2

    return data, nil
})
```

### 2. Old Data Migrates Automatically

```go
// Write V0 data
usersV0 := simple.NewCollection[UserV0](db, "users")
usersV0.Create(ctx, &UserV0{Name: "Alice Smith", Email: "alice@example.com"})

// Read with V2 schema - migration happens automatically
usersV2 := simple.NewCollection[UserV2](db, "users")
user, _ := usersV2.Get(ctx, userID)

// user.FirstName = "Alice"
// user.LastName = "Smith"
// user.Phone = "" (default value)
// user.V = 2
```

## Migration Features

### Lazy Evaluation
- Migrations only run when data is read
- No upfront backfill required
- Old and new code can coexist

### Automatic Chaining
Multiple migrations chain together automatically:
```go
simple.Migrate("User").From(0).To(1).Do(...)
simple.Migrate("User").From(1).To(2).Do(...)

// Reading V0 data with V2 schema runs both: 0→1→2
```

### Write-Back Policy
```go
// Default: migrate in memory only
db := simple.MustConnect()

// Write back migrated data (gradual upgrade)
store := db.Store()
store.WithMigrationPolicy(smarterbase.MigrateAndWrite)
```

## When to Use Versioning

**Start using migrations when:**
- You need to change existing data structure
- You want to add/remove/rename fields
- You need backward compatibility during deployments
- You have production data that can't be lost

**Typically added months after initial development**, not day one.

## Key Points

1. **Migrations are explicit** - No magic, you write the transformation logic
2. **Versioning is opt-in** - Only add `_v` field when you need it
3. **Zero downtime** - Old and new code work with same data
4. **No backfill needed** - Data migrates lazily on read
5. **Works with Simple API** - Just use `simple.Migrate()` instead of `smarterbase.Migrate()`

## Common Migration Patterns

```go
// Split a field
simple.Migrate("User").From(0).To(1).
    Split("name", " ", "first_name", "last_name")

// Add a field with default
simple.Migrate("Product").From(1).To(2).
    AddField("stock", 0)

// Rename a field
simple.Migrate("Order").From(2).To(3).
    RenameField("price", "total_amount")

// Remove a field
simple.Migrate("Config").From(3).To(4).
    RemoveField("deprecated_flag")
```

## Performance

- **No migration**: Zero overhead (fast path)
- **Version match**: ~50ns overhead (single field check)
- **Migration needed**: ~2-5ms per version step

## Complete Documentation

See [ADR-0001: Schema Versioning and Migrations](../../../docs/adr/0001-schema-versioning-and-migrations.md) for:
- Complete migration API reference
- Migration helpers and utilities
- Version chaining details
- Migration policies
- Best practices and gotchas

## Next Steps

After this example, explore:
- [examples/schema-migrations](../../schema-migrations) - More complex Core API migrations
- Combine versioning with atomic updates for critical data transformations
