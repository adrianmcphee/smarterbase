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
- Queries by indexed field (email)
- Queries by indexed field (role)
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
Indexes auto-registered in Redis:
- users-by-email
- users-by-role

=== CREATE USERS ===
Created: Alice (alice@example.com) - admin
Created: Bob (bob@example.com) - user
Created: Charlie (charlie@example.com) - user
Created: Diana (diana@example.com) - admin

=== FIND BY EMAIL (Index) ===
Found: Alice (ID: user-abc123)

=== FIND BY ROLE (Index) ===
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
    Email string `json:"email" sb:"index"` // Index on email
    Role  string `json:"role" sb:"index"`  // Index on role
    Name  string `json:"name"`             // Not indexed
}
```

- `sb:"id"` - Marks the ID field (auto-detected if field named "ID")
- `sb:"index"` - Creates a queryable index in Redis

### Automatic Index Registration

When you create a collection, the Simple API:
1. Parses struct tags via reflection
2. Registers indexes with Redis automatically
3. Updates indexes on Create/Update/Delete

No manual index registration needed!

### Query Operations

```go
// FindOne - returns first match
user, err := users.FindOne(ctx, "email", "alice@example.com")

// Find - returns all matches
admins, err := users.Find(ctx, "role", "admin")
```

### Index Updates

When you update an object, Redis indexes are automatically updated:

```go
bob.Role = "admin"
users.Update(ctx, bob)
// Redis index updated: old entry (role=user) removed, new entry (role=admin) added
```

## Redis Requirement

This example requires Redis to be running:
- `Find()` and `FindOne()` queries use Redis indexes
- Redis is checked on startup and the example exits if unavailable
- All indexes are stored and managed in Redis
- Data is persisted to the backend (filesystem/S3), indexes are in Redis

## Next Steps

- See Core API examples for advanced queries
- See atomic operations example for distributed locking
