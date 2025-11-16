package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/adrianmcphee/smarterbase/simple"
)

// UserV0 - Original schema (no version field)
type UserV0 struct {
	ID    string `json:"id" sb:"id"`
	Name  string `json:"name"`
	Email string `json:"email" sb:"index"`
}

// UserV2 - Evolved schema with version field
type UserV2 struct {
	V         int    `json:"_v" sb:"version"`
	ID        string `json:"id" sb:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email" sb:"index"`
	Phone     string `json:"phone"`
}

// ============================================================================
// Type-Safe Migration Function
// ============================================================================
//
// This is a pure, testable function with full type safety.
// No map[string]interface{}, no type assertions, no runtime panics.

// migrateUserV0ToV2 transforms a V0 user to V2 by splitting the name
func migrateUserV0ToV2(old UserV0) (UserV2, error) {
	if old.Email == "" || old.Name == "" {
		return UserV2{}, errors.New("name and email required")
	}

	// Split "name" into "first_name" and "last_name"
	parts := strings.Fields(old.Name)
	firstName := parts[0]
	lastName := ""
	if len(parts) > 1 {
		lastName = strings.Join(parts[1:], " ")
	}

	return UserV2{
		V:         2,
		ID:        old.ID,
		FirstName: firstName,
		LastName:  lastName,
		Email:     old.Email,
		Phone:     "", // New field with default value
	}, nil
}

func main() {
	ctx := context.Background()

	// Register migration BEFORE connecting
	// This migration transforms UserV0 → UserV2
	// IMPORTANT: Use the FINAL type name ("UserV2") not the base name
	//
	// Using WithTypeSafe() gives us full type safety with zero boilerplate!
	simple.WithTypeSafe(
		simple.Migrate("UserV2").From(0).To(2),
		migrateUserV0ToV2,
	)

	// Connect to database
	db := simple.MustConnect()
	defer db.Close()

	fmt.Println("=== SCHEMA VERSIONING EXAMPLE ===")
	fmt.Println()

	// Step 1: Create V0 users collection (for writing old data)
	fmt.Println("Step 1: Creating V0 users (simulating old data)...")
	usersV0 := simple.NewCollection[UserV0](db, "users")

	oldUser := &UserV0{
		Name:  "Alice Smith",
		Email: "alice@example.com",
	}
	created, err := usersV0.Create(ctx, oldUser)
	if err != nil {
		log.Fatal(err)
	}
	userID := created.ID

	fmt.Printf("  Created V0 user: %s (%s)\n", created.Name, created.Email)
	fmt.Printf("  ID: %s\n", userID)
	fmt.Println()

	// Step 2: Read using V2 schema (triggers migration)
	fmt.Println("Step 2: Reading with V2 schema (auto-migration)...")

	// For version-aware reads, we use the escape hatch to access the Store directly
	// This is because Collection.Get() doesn't support setting expected version yet
	store := db.Store()
	key := fmt.Sprintf("users/%s.json", userID)

	var migratedUser UserV2
	migratedUser.V = 2 // Set expected version BEFORE reading

	err = store.GetJSON(ctx, key, &migratedUser)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("  ✅ Migration happened automatically!")
	fmt.Printf("  Version: %d\n", migratedUser.V)
	fmt.Printf("  First Name: %s\n", migratedUser.FirstName)
	fmt.Printf("  Last Name: %s\n", migratedUser.LastName)
	fmt.Printf("  Email: %s\n", migratedUser.Email)
	fmt.Printf("  Phone: %s (new field with default)\n", migratedUser.Phone)
	fmt.Println()

	// Step 3: Update migrated user
	fmt.Println("Step 3: Updating migrated user...")
	migratedUser.Phone = "+1-555-0123"

	// Write back with the full V2 schema
	err = store.PutJSON(ctx, key, &migratedUser)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("  Updated phone: %s\n", migratedUser.Phone)
	fmt.Println()

	// Step 4: Create new V2 user (Collection API works fine for new data)
	fmt.Println("Step 4: Creating new V2 user...")
	usersV2 := simple.NewCollection[UserV2](db, "users")

	newUser := &UserV2{
		V:         2,
		FirstName: "Bob",
		LastName:  "Jones",
		Email:     "bob@example.com",
		Phone:     "+1-555-0456",
	}

	createdV2, err := usersV2.Create(ctx, newUser)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("  Created V2 user: %s %s (%s)\n", createdV2.FirstName, createdV2.LastName, createdV2.Email)
	fmt.Println()

	// Step 5: List all users with V2 schema
	fmt.Println("Step 5: Listing all users with V2 schema...")

	// For listing with migrations, we need to use Each() with version-aware reads
	fmt.Printf("  All users:\n")
	err = store.Query("users/").Each(ctx, func(key string, data []byte) error {
		var u UserV2
		u.V = 2 // Set expected version
		err := store.GetJSON(ctx, key, &u)
		if err != nil {
			return err
		}
		fmt.Printf("  - %s %s (%s) [v%d]\n", u.FirstName, u.LastName, u.Email, u.V)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println()

	// Cleanup
	fmt.Println("=== CLEANUP ===")
	err = usersV2.Delete(ctx, userID)
	if err != nil {
		log.Fatal(err)
	}
	err = usersV2.Delete(ctx, createdV2.ID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("All test users deleted")
	fmt.Println()

	fmt.Println("💡 Key Points:")
	fmt.Println("  1. Migrations registered with simple.Migrate() before connecting")
	fmt.Println("  2. Old data (V0) migrates automatically when read with V2 schema")
	fmt.Println("  3. Split name into first_name/last_name automatically")
	fmt.Println("  4. Added 'phone' field with default value")
	fmt.Println("  5. Both old and new data work seamlessly")
	fmt.Println()
	fmt.Println("For details: docs/adr/0001-schema-versioning-and-migrations.md")
}
