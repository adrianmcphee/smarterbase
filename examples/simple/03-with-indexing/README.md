# With Indexing Example

Demonstrates querying by indexed fields using Redis.

## Prerequisites

This example requires Redis to be running:

```bash
# Start Redis with Docker
docker run -d -p 6379:6379 redis

# Or install and start Redis locally
# macOS: brew install redis && brew services start redis
# Linux: sudo apt install redis-server && sudo systemctl start redis
```

## What it does

- Defines a User struct with indexed fields (email, role)
- Creates multiple users with different roles
- Queries by unique index (email)
- Queries by multi-value index (role)
- Updates a user and re-queries to see changes
- Lists all users

## Running

```bash
# With default Redis (localhost:6379)
go run main.go

# With custom Redis address
REDIS_ADDR=redis.example.com:6379 go run main.go
```

## Expected Output

```
=== SETUP ===
Indexes auto-registered:
- users-by-email (unique)
- users-by-role (multi-value)

=== CREATE USERS ===
Created: Alice (alice@example.com) - admin
Created: Bob (bob@example.com) - user
Created: Charlie (charlie@example.com) - user
Created: Diana (diana@example.com) - admin

=== FIND BY EMAIL (Unique Index) ===
Found: Alice (ID: user-abc123)

=== FIND BY ROLE (Multi-Value Index) ===
Found 2 admins:
  - Alice (alice@example.com)
  - Diana (diana@example.com)
Found 2 regular users:
  - Bob (bob@example.com)
  - Charlie (charlie@example.com)

=== UPDATE: Promote Bob to Admin ===
Updated Bob's role to admin

Now 3 admins:
  - Alice (alice@example.com)
  - Bob (bob@example.com)
  - Diana (diana@example.com)

=== ALL USERS ===
Total: 4 users
  - admin: Alice (alice@example.com, age 30)
  - admin: Bob (bob@example.com, age 25)
  - user: Charlie (charlie@example.com, age 35)
  - admin: Diana (diana@example.com, age 28)

=== CLEANUP ===
All test users deleted
```

## Key Concepts

### Struct Tags for Indexing

```go
type User struct {
    ID    string `json:"id" sb:"id"`
    Email string `json:"email" sb:"index,unique"` // Unique index
    Role  string `json:"role" sb:"index"`         // Multi-value index
    Name  string `json:"name"`                    // Not indexed
}
```

- `sb:"id"` - Marks the ID field (auto-detected if field named "ID")
- `sb:"index"` - Creates a queryable index
- `sb:"index,unique"` - Creates a unique index (optimized for 1:1 lookups)

### Automatic Index Registration

When you create a collection, the Simple API:
1. Parses struct tags via reflection
2. Registers indexes with Redis automatically
3. Updates indexes on Create/Update/Delete

No manual index registration needed!

### Query Operations

```go
// FindOne - returns first match (useful for unique indexes)
user, err := users.FindOne(ctx, "email", "alice@example.com")

// Find - returns all matches (useful for multi-value indexes)
admins, err := users.Find(ctx, "role", "admin")
```

### Index Updates

When you update an object, indexes are automatically updated:

```go
bob.Role = "admin"
users.Update(ctx, bob)
// Old index entry (role=user) removed
// New index entry (role=admin) added
```

## Graceful Degradation

If Redis is not available:
- `Find()` and `FindOne()` return errors
- Other operations (Get, Create, Update, Delete) work normally
- Data is still persisted to the backend (filesystem/S3)

This allows you to develop without Redis and add it later when needed.

## Next Steps

- See Core API examples for advanced queries
- See atomic operations example for distributed locking
