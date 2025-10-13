package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/adrianmcphee/smarterbase"
	"github.com/redis/go-redis/v9"
)

// User represents a user in the system
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserManager handles all user-related operations
type UserManager struct {
	store        *smarterbase.Store
	indexManager *smarterbase.IndexManager
	redisIndexer *smarterbase.RedisIndexer
}

// NewUserManager creates a new user manager
func NewUserManager(store *smarterbase.Store, redisClient *redis.Client) *UserManager {
	// Create Redis indexer for fast lookups
	redisIndexer := smarterbase.NewRedisIndexer(redisClient)

	// Register indexes
	redisIndexer.RegisterMultiIndex(&smarterbase.MultiIndexSpec{
		Name:        "users-by-email",
		EntityType:  "users",
		ExtractFunc: smarterbase.ExtractJSONField("email"),
	})

	redisIndexer.RegisterMultiIndex(&smarterbase.MultiIndexSpec{
		Name:        "users-by-role",
		EntityType:  "users",
		ExtractFunc: smarterbase.ExtractJSONField("role"),
	})

	// Create index manager to coordinate updates
	indexManager := smarterbase.NewIndexManager(store).
		WithRedisIndexer(redisIndexer)

	return &UserManager{
		store:        store,
		indexManager: indexManager,
		redisIndexer: redisIndexer,
	}
}

// CreateUser creates a new user
func (m *UserManager) CreateUser(ctx context.Context, email, name, role string) (*User, error) {
	user := &User{
		ID:        smarterbase.NewID(),
		Email:     email,
		Name:      name,
		Role:      role,
		Active:    true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	key := fmt.Sprintf("users/%s.json", user.ID)

	// Create with automatic index updates
	err := m.indexManager.Create(ctx, key, user)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	log.Printf("Created user: %s (%s)", user.Name, user.Email)
	return user, nil
}

// GetUserByID retrieves a user by ID
func (m *UserManager) GetUserByID(ctx context.Context, userID string) (*User, error) {
	key := fmt.Sprintf("users/%s.json", userID)

	var user User
	err := m.store.GetJSON(ctx, key, &user)
	if err != nil {
		if smarterbase.IsNotFound(err) {
			return nil, fmt.Errorf("user not found: %s", userID)
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// GetUserByEmail retrieves a user by email (using index)
func (m *UserManager) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	// Query index - O(1) lookup
	keys, err := m.redisIndexer.Query(ctx, "users", "email", email)
	if err != nil {
		return nil, fmt.Errorf("failed to query index: %w", err)
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("user not found: %s", email)
	}

	// Get user data
	var user User
	err = m.store.GetJSON(ctx, keys[0], &user)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// ListUsersByRole returns all users with a specific role
func (m *UserManager) ListUsersByRole(ctx context.Context, role string) ([]*User, error) {
	// Query index for all users with this role
	keys, err := m.redisIndexer.Query(ctx, "users", "role", role)
	if err != nil {
		return nil, fmt.Errorf("failed to query index: %w", err)
	}

	// Batch fetch all users
	users := make([]*User, 0, len(keys))
	results, err := m.store.BatchGetJSON(ctx, keys, User{})
	if err != nil {
		return nil, fmt.Errorf("failed to batch get users: %w", err)
	}

	for key, value := range results {
		// Convert map to User struct
		data, _ := json.Marshal(value)
		var user User
		if err := json.Unmarshal(data, &user); err != nil {
			log.Printf("Warning: failed to unmarshal user %s: %v", key, err)
			continue
		}
		users = append(users, &user)
	}

	return users, nil
}

// UpdateUser updates an existing user
func (m *UserManager) UpdateUser(ctx context.Context, userID string, updateFn func(*User) error) error {
	key := fmt.Sprintf("users/%s.json", userID)

	// Get current user
	var user User
	if err := m.store.GetJSON(ctx, key, &user); err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Apply updates
	if err := updateFn(&user); err != nil {
		return fmt.Errorf("update function failed: %w", err)
	}

	user.UpdatedAt = time.Now()

	// Update with index coordination
	err := m.indexManager.Update(ctx, key, &user)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	log.Printf("Updated user: %s (%s)", user.Name, user.Email)
	return nil
}

// DeleteUser deletes a user
func (m *UserManager) DeleteUser(ctx context.Context, userID string) error {
	key := fmt.Sprintf("users/%s.json", userID)

	// Delete with index cleanup
	err := m.indexManager.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	log.Printf("Deleted user: %s", userID)
	return nil
}

// ListActiveUsers returns all active users
func (m *UserManager) ListActiveUsers(ctx context.Context) ([]*User, error) {
	var users []*User

	err := m.store.Query("users/").
		FilterJSON(func(obj map[string]interface{}) bool {
			active, ok := obj["active"].(bool)
			return ok && active
		}).
		SortByField("created_at", false). // Newest first
		All(ctx, &users)

	if err != nil {
		return nil, fmt.Errorf("failed to list active users: %w", err)
	}

	return users, nil
}

// CountUsersByRole returns the count of users for each role
func (m *UserManager) CountUsersByRole(ctx context.Context) (map[string]int64, error) {
	roles := []string{"admin", "user", "guest"}
	stats, err := m.redisIndexer.GetIndexStats(ctx, "users", "role", roles)
	if err != nil {
		return nil, fmt.Errorf("failed to get role statistics: %w", err)
	}

	return stats, nil
}

func main() {
	ctx := context.Background()

	fmt.Println("\n=== User Management with SmarterBase ===")
	fmt.Println("\n📋 THE CHALLENGE:")
	fmt.Println("Traditional user management systems require:")
	fmt.Println("  • Complex database schemas with migrations")
	fmt.Println("  • Expensive horizontal scaling for millions of users")
	fmt.Println("  • Backup/restore infrastructure")
	fmt.Println("  • Slow lookups by email or role without proper indexes")
	fmt.Println("\n✨ THE SMARTERBASE SOLUTION:")
	fmt.Println("  ✅ 11 9s durability - AWS multi-AZ replication, no backup needed")
	fmt.Println("  ✅ Infinite scale - S3 scales automatically, no capacity planning")
	fmt.Println("  ✅ Zero backups - S3 handles durability automatically")
	fmt.Println("  ✅ Schema-less - JSON structure, add fields without migrations")
	fmt.Println("  ✅ O(1) lookups - Redis indexes for instant email/role queries")
	fmt.Println()

	// Development setup: Filesystem backend
	backend := smarterbase.NewFilesystemBackend("./data")
	defer backend.Close()

	store := smarterbase.NewStore(backend)

	// Production would use:
	// cfg, _ := config.LoadDefaultConfig(ctx)
	// s3Client := s3.NewFromConfig(cfg)
	// redisClient := redis.NewClient(smarterbase.RedisOptions())
	// backend := smarterbase.NewS3BackendWithRedisLock(s3Client, "my-bucket", redisClient)
	// logger, _ := smarterbase.NewProductionZapLogger()
	// metrics := smarterbase.NewPrometheusMetrics(prometheus.DefaultRegisterer)
	// store := smarterbase.NewStoreWithObservability(backend, logger, metrics)

	// Redis configuration from environment (REDIS_ADDR, REDIS_PASSWORD, REDIS_DB)
	// Defaults to localhost:6379 for local development
	redisClient := redis.NewClient(smarterbase.RedisOptions())
	defer redisClient.Close()

	// Test Redis connection
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatal("Redis connection failed (start Redis or use in-memory option):", err)
	}

	// Create user manager
	userManager := NewUserManager(store, redisClient)

	// Example operations
	fmt.Println("=== Running Example Operations ===")

	// 1. Create users
	fmt.Println("1. Creating users...")
	alice, _ := userManager.CreateUser(ctx, "alice@example.com", "Alice Smith", "admin")
	bob, _ := userManager.CreateUser(ctx, "bob@example.com", "Bob Johnson", "user")
	charlie, _ := userManager.CreateUser(ctx, "charlie@example.com", "Charlie Brown", "user")
	fmt.Printf("   Created %d users\n", 3)

	// 2. Get user by ID
	fmt.Println("\n2. Getting user by ID...")
	user, err := userManager.GetUserByID(ctx, alice.ID)
	if err != nil {
		log.Printf("   Error: %v", err)
	} else {
		fmt.Printf("   Found: %s (%s)\n", user.Name, user.Email)
	}

	// 3. Get user by email (indexed lookup - O(1))
	fmt.Println("\n3. Getting user by email (indexed)...")
	user, err = userManager.GetUserByEmail(ctx, "bob@example.com")
	if err != nil {
		log.Printf("   Error: %v", err)
	} else {
		fmt.Printf("   Found: %s (ID: %s)\n", user.Name, user.ID)
	}

	// 4. List users by role
	fmt.Println("\n4. Listing users by role...")
	users, _ := userManager.ListUsersByRole(ctx, "user")
	fmt.Printf("   Found %d users with role 'user':\n", len(users))
	for _, u := range users {
		fmt.Printf("   - %s (%s)\n", u.Name, u.Email)
	}

	// 5. Update user
	fmt.Println("\n5. Updating user...")
	err = userManager.UpdateUser(ctx, bob.ID, func(u *User) error {
		u.Name = "Robert Johnson"
		u.Role = "admin"
		return nil
	})
	if err != nil {
		log.Printf("   Error: %v", err)
	} else {
		fmt.Println("   Updated Bob's name and role")
	}

	// 6. List active users
	fmt.Println("\n6. Listing active users...")
	activeUsers, _ := userManager.ListActiveUsers(ctx)
	fmt.Printf("   Found %d active users\n", len(activeUsers))

	// 7. Count users by role
	fmt.Println("\n7. Counting users by role...")
	roleCounts, _ := userManager.CountUsersByRole(ctx)
	for role, count := range roleCounts {
		fmt.Printf("   %s: %d users\n", role, count)
	}

	// 8. Delete user
	fmt.Println("\n8. Deleting user...")
	err = userManager.DeleteUser(ctx, charlie.ID)
	if err != nil {
		log.Printf("   Error: %v", err)
	} else {
		fmt.Println("   Deleted Charlie's account")
	}

	// Verify deletion
	_, err = userManager.GetUserByID(ctx, charlie.ID)
	if err != nil {
		fmt.Println("   Confirmed: user no longer exists")
	}

	fmt.Println("\n=== Example Complete ===")
}
