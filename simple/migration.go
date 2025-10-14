package simple

import "github.com/adrianmcphee/smarterbase"

// Migrate registers schema migrations that work automatically with Simple API.
// Migrations apply lazily when reading data via Collection.Get(), Collection.All(), etc.
//
// Example:
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
