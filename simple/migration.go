package simple

import "github.com/adrianmcphee/smarterbase"

// Migrate registers schema migrations that work automatically with Simple API.
// Migrations apply lazily when reading data via Collection.Get(), Collection.All(), etc.
//
// RECOMMENDED: Use WithTypeSafe() for type-safe migrations:
//
//	func migrateUserV0ToV2(old UserV0) (UserV2, error) {
//	    parts := strings.Fields(old.Name)
//	    return UserV2{
//	        V:         2,
//	        FirstName: parts[0],
//	        LastName:  strings.Join(parts[1:], " "),
//	        Email:     old.Email,
//	    }, nil
//	}
//
//	simple.WithTypeSafe(
//	    simple.Migrate("User").From(0).To(2),
//	    migrateUserV0ToV2,
//	)
//
// Legacy map-based migrations also work:
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
// For complete documentation see:
// https://github.com/adrianmcphee/smarterbase/blob/main/docs/adr/0001-schema-versioning-and-migrations.md
func Migrate(typeName string) *smarterbase.MigrationBuilder {
	return smarterbase.Migrate(typeName)
}

// WithTypeSafe registers a type-safe migration function (RECOMMENDED).
//
// This is a thin wrapper around smarterbase.WithTypeSafe() that provides
// full type safety for Simple API migrations.
//
// Example:
//
//	// Define a pure, type-safe migration function
//	func migrateUserV0ToV2(old UserV0) (UserV2, error) {
//	    parts := strings.Fields(old.Name)
//	    return UserV2{
//	        V:         2,
//	        FirstName: parts[0],
//	        LastName:  strings.Join(parts[1:], " "),
//	        Email:     old.Email,
//	    }, nil
//	}
//
//	// Register it with zero boilerplate
//	simple.WithTypeSafe(
//	    simple.Migrate("User").From(0).To(2),
//	    migrateUserV0ToV2,
//	)
//
// Benefits:
//   - ✅ Full type safety - no map[string]interface{}
//   - ✅ Compiler catches errors at build time
//   - ✅ IDE autocomplete works
//   - ✅ Easy to unit test in isolation
//   - ✅ Self-documenting with concrete types
func WithTypeSafe[From any, To any](builder *smarterbase.MigrationBuilder, migrateFn func(From) (To, error)) *smarterbase.MigrationBuilder {
	return smarterbase.WithTypeSafe(builder, migrateFn)
}
