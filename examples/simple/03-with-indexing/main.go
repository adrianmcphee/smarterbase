// With Indexing demonstrates querying by indexed fields using Redis.
// This example requires Redis to be running.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/adrianmcphee/smarterbase/simple"
)

// User demonstrates struct tags for indexing
type User struct {
	ID    string `json:"id" sb:"id"`
	Email string `json:"email" sb:"index"` // Index on email
	Role  string `json:"role" sb:"index"`  // Index on role
	Name  string `json:"name"`
	Age   int    `json:"age"`
}

func main() {
	ctx := context.Background()

	// Setup - requires Redis for indexing
	// Set REDIS_ADDR environment variable if not on localhost
	if os.Getenv("REDIS_ADDR") == "" {
		fmt.Println("Note: Using default Redis at localhost:6379")
		fmt.Println("Set REDIS_ADDR to override")
	}

	db, err := simple.Connect()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Check if Redis is available
	if db.RedisIndexer() == nil {
		log.Fatal("Redis is required for this example. Please start Redis and try again.")
	}

	users := simple.NewCollection[User](db)

	fmt.Println("=== SETUP ===")
	fmt.Println("Indexes auto-registered in Redis:")
	fmt.Println("- users-by-email")
	fmt.Println("- users-by-role")

	// CREATE - Seed some test data
	fmt.Println("\n=== CREATE USERS ===")

	testUsers := []*User{
		{Email: "alice@example.com", Name: "Alice", Role: "admin", Age: 30},
		{Email: "bob@example.com", Name: "Bob", Role: "user", Age: 25},
		{Email: "charlie@example.com", Name: "Charlie", Role: "user", Age: 35},
		{Email: "diana@example.com", Name: "Diana", Role: "admin", Age: 28},
	}

	for _, user := range testUsers {
		created, err := users.Create(ctx, user)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Created: %s (%s) - %s\n", created.Name, created.Email, created.Role)
	}

	// FIND BY EMAIL INDEX
	fmt.Println("\n=== FIND BY EMAIL (Index) ===")

	alice, err := users.FindOne(ctx, "email", "alice@example.com")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Found: %s (ID: %s)\n", alice.Name, alice.ID)

	// FIND BY ROLE INDEX
	fmt.Println("\n=== FIND BY ROLE (Index) ===")

	admins, err := users.Find(ctx, "role", "admin")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Found %d admins:\n", len(admins))
	for _, admin := range admins {
		fmt.Printf("  - %s (%s)\n", admin.Name, admin.Email)
	}

	regularUsers, err := users.Find(ctx, "role", "user")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Found %d regular users:\n", len(regularUsers))
	for _, user := range regularUsers {
		fmt.Printf("  - %s (%s)\n", user.Name, user.Email)
	}

	// UPDATE AND QUERY AGAIN
	fmt.Println("\n=== UPDATE: Promote Bob to Admin ===")

	bob, err := users.FindOne(ctx, "email", "bob@example.com")
	if err != nil {
		log.Fatal(err)
	}

	bob.Role = "admin"
	if err := users.Update(ctx, bob); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Updated %s's role to admin\n", bob.Name)

	// Query again to see updated results
	admins, err = users.Find(ctx, "role", "admin")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\nNow %d admins:\n", len(admins))
	for _, admin := range admins {
		fmt.Printf("  - %s (%s)\n", admin.Name, admin.Email)
	}

	// ALL USERS
	fmt.Println("\n=== ALL USERS ===")

	allUsers, err := users.All(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Total: %d users\n", len(allUsers))
	for _, user := range allUsers {
		fmt.Printf("  - %s: %s (%s, age %d)\n",
			user.Role, user.Name, user.Email, user.Age)
	}

	// CLEANUP
	fmt.Println("\n=== CLEANUP ===")
	for _, user := range allUsers {
		if err := users.Delete(ctx, user.ID); err != nil {
			log.Printf("Warning: failed to delete %s: %v", user.ID, err)
		}
	}
	fmt.Println("All test users deleted")
}
