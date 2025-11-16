// Package simple provides a high-level, batteries-included API for SmarterBase.
//
// # Philosophy
//
// The Simple API is designed for rapid prototyping, demos, and applications that
// prioritize developer experience over fine-grained control. It provides:
//
//   - Automatic configuration from environment variables
//   - Type-safe CRUD operations using generics
//   - Automatic index management via struct tags
//   - Sensible defaults for common use cases
//   - Graceful degradation when Redis is unavailable
//
// # Quick Start
//
// Create a struct with tags and start storing data:
//
//	type User struct {
//	    ID    string `json:"id" sb:"id"`
//	    Email string `json:"email" sb:"index"`
//	    Name  string `json:"name"`
//	}
//
//	db := simple.MustConnect()
//	defer db.Close()
//
//	users := simple.NewCollection[User](db)
//	user, err := users.Create(ctx, &User{
//	    Email: "alice@example.com",
//	    Name:  "Alice",
//	})
//
// # Struct Tags
//
// The Simple API uses struct tags to configure behavior:
//
//   - sb:"id" - Marks the ID field (defaults to field named "ID")
//   - sb:"index" - Creates a queryable Redis index on this field
//
// Example:
//
//	type Product struct {
//	    ID       string  `json:"id" sb:"id"`
//	    SKU      string  `json:"sku" sb:"index"`
//	    Category string  `json:"category" sb:"index"`
//	    Name     string  `json:"name"`
//	    Price    float64 `json:"price"`
//	}
//
// # Configuration
//
// The Simple API auto-detects configuration from environment:
//
//   - DATA_PATH: Filesystem backend path (default: "./data")
//   - AWS_BUCKET: S3 bucket name (enables S3 backend - not yet implemented)
//   - REDIS_ADDR: Redis address (default: "localhost:6379") - REQUIRED for indexing
//   - REDIS_PASSWORD: Redis password (optional)
//   - REDIS_DB: Redis database number (default: 0)
//
// Example .env file:
//
//	DATA_PATH=./myapp-data
//	REDIS_ADDR=localhost:6379
//
// # Error Handling
//
// The Simple API provides two initialization styles:
//
// 1. Connect() - Returns error for production use:
//
//	db, err := simple.Connect()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Close()
//
// 2. MustConnect() - Panics on error for demos/prototypes:
//
//	db := simple.MustConnect()
//	defer db.Close()
//
// # Escape Hatches
//
// The Simple API provides access to the underlying Core API when you need it:
//
//	// Access Core API Store
//	store := db.Store()
//	store.Query("users/").FilterJSON("age", ">", 18).All(ctx, &users)
//
//	// Access Redis Indexer
//	indexer := db.RedisIndexer()
//	indexer.RegisterMultiIndex(...)
//
//	// Access Distributed Lock
//	lock := db.Lock()
//	lock.Lock(ctx, "critical-section", 10*time.Second)
//
// This design follows the principle of "progressive disclosure" - start simple,
// add complexity only when needed.
//
// # Collections
//
// Collections provide type-safe CRUD operations:
//
//	users := simple.NewCollection[User](db)
//
//	// Create
//	user, err := users.Create(ctx, &User{Email: "alice@example.com"})
//
//	// Get by ID
//	user, err := users.Get(ctx, userID)
//
//	// Update
//	user.Name = "Alice Smith"
//	err := users.Update(ctx, user)
//
//	// Delete
//	err := users.Delete(ctx, userID)
//
//	// Query by indexed field
//	admins, err := users.Find(ctx, "role", "admin")
//	user, err := users.FindOne(ctx, "email", "alice@example.com")
//
//	// Atomic operations
//	err := users.Atomic(ctx, userID, 10*time.Second, func(user *User) error {
//	    user.Balance += 100
//	    return nil
//	})
//
// # Collection Naming
//
// Collection names are inferred from type names with smart pluralization:
//
//	Collection[User](db)     // -> "users"
//	Collection[Person](db)   // -> "people"
//	Collection[Child](db)    // -> "children"
//
// Override with explicit name:
//
//	Collection[User](db, "customers")  // -> "customers"
//
// # Immutability
//
// The Create() method returns a new object with ID populated, leaving the
// input unchanged:
//
//	user := &User{Email: "alice@example.com"}
//	created, err := users.Create(ctx, user)
//	// user.ID == ""        (unchanged)
//	// created.ID == "..."  (populated)
//
// # Schema Versioning
//
// The Simple API supports schema versioning through simple.Migrate().
// Migrations are applied lazily when data is read via Collection.Get(),
// Collection.All(), Collection.Each(), etc.
//
//	simple.Migrate("User").From(0).To(2).Do(func(data map[string]interface{}) (map[string]interface{}, error) {
//	    parts := strings.Split(data["name"].(string), " ")
//	    data["first_name"] = parts[0]
//	    data["last_name"] = parts[1]
//	    data["_v"] = 2
//	    return data, nil
//	})
//
//	users := simple.NewCollection[UserV2](db)
//	user, _ := users.Get(ctx, "user-123")  // Auto-migrates V0 → V2
//
// Migrations are an advanced feature typically added months after initial development.
// See examples/simple/04-versioning/ for a complete example.
// For details: https://github.com/adrianmcphee/smarterbase/blob/main/docs/adr/0001-schema-versioning-and-migrations.md
//
// # When to Use Simple vs Core API
//
// Use Simple API when:
//   - Building prototypes or demos
//   - Startup time is not critical
//   - You want automatic index management
//   - You prefer convention over configuration
//
// Use Core API when:
//   - You need fine-grained control
//   - Startup time matters (no reflection)
//   - You want explicit index registration
//   - You're building a library on top of SmarterBase
//
// Both APIs can be used together - the Simple API is built on top of the Core API
// and provides escape hatches when you need more control.
package simple
