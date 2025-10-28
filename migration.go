package smarterbase

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// MigrationFunc transforms data from one version to another.
//
// The function receives the JSON data as a map[string]interface{} and must return
// the transformed data. It should set the "_v" field to the target version.
//
// Example custom migration:
//
//	smarterbase.Migrate("Product").From(1).To(2).Do(func(data map[string]interface{}) (map[string]interface{}, error) {
//	    // Convert price to cents
//	    if price, ok := data["price"].(float64); ok {
//	        data["price_cents"] = int(price * 100)
//	        delete(data, "price")
//	    }
//	    data["_v"] = 2
//	    return data, nil
//	})
type MigrationFunc func(data map[string]interface{}) (map[string]interface{}, error)

// MigrationRegistry manages schema migrations
type MigrationRegistry struct {
	mu         sync.RWMutex
	migrations map[string]map[int]map[int]MigrationFunc // typeName -> fromVersion -> toVersion -> func
}

// Global migration registry
var globalRegistry = &MigrationRegistry{
	migrations: make(map[string]map[int]map[int]MigrationFunc),
}

// MigrationBuilder provides a fluent API for registering migrations
type MigrationBuilder struct {
	typeName    string
	fromVersion int
	toVersion   int
}

// Migrate starts building a migration for a type.
//
// Migrations enable schema evolution without downtime. When data is read from storage,
// it is automatically migrated if its version doesn't match the expected version in the
// destination struct.
//
// RECOMMENDED: Use WithTypeSafe() for type-safe migrations with concrete types:
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
//	// Register with zero boilerplate
//	smarterbase.WithTypeSafe(
//	    smarterbase.Migrate("User").From(0).To(2),
//	    migrateUserV0ToV2,
//	)
//
// Helper methods for simple transformations:
//
//	// Split a field into multiple fields
//	smarterbase.Migrate("User").From(0).To(1).
//	    Split("name", " ", "first_name", "last_name")
//
//	// Add a new field with default value
//	smarterbase.Migrate("User").From(1).To(2).AddField("phone", "")
//
//	// Rename a field
//	smarterbase.Migrate("Order").From(2).To(3).
//	    RenameField("price", "total_amount")
//
//	// Remove a deprecated field
//	smarterbase.Migrate("Config").From(3).To(4).
//	    RemoveField("legacy_flag")
//
// Migration chaining - automatically finds shortest path:
//
//	smarterbase.Migrate("Product").From(0).To(1).AddField("sku", "")
//	smarterbase.Migrate("Product").From(1).To(2).Split("name", " ", "brand", "product_name")
//	smarterbase.WithTypeSafe(smarterbase.Migrate("Product").From(2).To(3), customMigrate)
//
//	// Reading v0 data with v3 struct → automatically runs 0→1→2→3
//
// Migration policies:
//
//	// Default: Migrate in memory only (no write-back)
//	store := smarterbase.NewStore(backend)
//
//	// Write-back policy: Gradually upgrade stored data
//	store.WithMigrationPolicy(smarterbase.MigrateAndWrite)
//
// The typeName parameter must match the struct's type name (not the JSON field name).
// For example, if you have "type UserV2 struct {...}", use "UserV2" as the typeName.
//
// See docs/adr/0007-type-safe-migrations.md for implementation details and testing examples.
func Migrate(typeName string) *MigrationBuilder {
	return &MigrationBuilder{typeName: typeName}
}

// From sets the source version
func (b *MigrationBuilder) From(version int) *MigrationBuilder {
	b.fromVersion = version
	return b
}

// To sets the target version
func (b *MigrationBuilder) To(version int) *MigrationBuilder {
	b.toVersion = version
	return b
}

// Do registers a custom migration function
func (b *MigrationBuilder) Do(fn MigrationFunc) *MigrationBuilder {
	globalRegistry.Register(b.typeName, b.fromVersion, b.toVersion, fn)
	return b
}

// WithTypeSafe registers a type-safe migration function.
//
// This is the RECOMMENDED way to write migrations. Instead of working with
// map[string]interface{}, you write a pure function that transforms concrete
// types. This provides full type safety, IDE autocomplete, and compile-time
// error checking.
//
// Example:
//
//	// Define your migration as a pure, type-safe function
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
//	smarterbase.Migrate("User").From(0).To(2).
//	    WithTypeSafe(migrateUserV0ToV2)
//
// Benefits over Do():
//   - ✅ Full type safety - no map[string]interface{}
//   - ✅ Compiler catches errors at build time
//   - ✅ IDE autocomplete works
//   - ✅ Easy to unit test in isolation
//   - ✅ Self-documenting with concrete types
//   - ✅ Refactoring tools work correctly
func WithTypeSafe[From any, To any](b *MigrationBuilder, migrateFn func(From) (To, error)) *MigrationBuilder {
	// Wrap the type-safe function with JSON marshaling adapter
	fn := func(data map[string]interface{}) (map[string]interface{}, error) {
		// Marshal map to JSON bytes
		jsonBytes, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal input: %w", err)
		}

		// Unmarshal to concrete source type
		var old From
		if err := json.Unmarshal(jsonBytes, &old); err != nil {
			return nil, fmt.Errorf("failed to unmarshal to source type: %w", err)
		}

		// Call the type-safe migration function
		new, err := migrateFn(old)
		if err != nil {
			return nil, err
		}

		// Marshal result back to JSON
		newBytes, err := json.Marshal(new)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		// Unmarshal to map for registry
		var result map[string]interface{}
		if err := json.Unmarshal(newBytes, &result); err != nil {
			return nil, fmt.Errorf("failed to unmarshal result: %w", err)
		}

		return result, nil
	}

	return b.Do(fn)
}

// Split is a helper that splits a field by delimiter into multiple fields.
//
// Common use case: splitting a full name into first and last names.
//
// Example:
//
//	// Split "name" field by space into "first_name" and "last_name"
//	smarterbase.Migrate("User").From(0).To(1).
//	    Split("name", " ", "first_name", "last_name")
//
//	// Before: {"name": "Alice Smith"}
//	// After:  {"first_name": "Alice", "last_name": "Smith", "_v": 1}
//
// If the source field contains fewer parts than target fields, remaining fields
// are set to empty strings. The source field is removed after splitting.
func (b *MigrationBuilder) Split(sourceField, delimiter string, targetFields ...string) *MigrationBuilder {
	fn := func(data map[string]interface{}) (map[string]interface{}, error) {
		if val, ok := data[sourceField].(string); ok {
			parts := strings.SplitN(val, delimiter, len(targetFields))
			for i, field := range targetFields {
				if i < len(parts) {
					data[field] = parts[i]
				} else {
					data[field] = ""
				}
			}
			delete(data, sourceField)
		}
		data["_v"] = b.toVersion
		return data, nil
	}
	return b.Do(fn)
}

// AddField adds a new field with a default value.
//
// Use this when introducing new required fields to your schema. The default value
// is only added if the field doesn't already exist in the data.
//
// Examples:
//
//	// Add a phone field with empty string default
//	smarterbase.Migrate("User").From(0).To(1).
//	    AddField("phone", "")
//
//	// Add an inventory count with zero default
//	smarterbase.Migrate("Product").From(1).To(2).
//	    AddField("stock_count", 0)
//
//	// Add a boolean flag with false default
//	smarterbase.Migrate("Config").From(2).To(3).
//	    AddField("enabled", false)
//
//	// Before: {"id": "123", "name": "Product"}
//	// After:  {"id": "123", "name": "Product", "stock_count": 0, "_v": 2}
func (b *MigrationBuilder) AddField(field string, defaultValue interface{}) *MigrationBuilder {
	fn := func(data map[string]interface{}) (map[string]interface{}, error) {
		if _, exists := data[field]; !exists {
			data[field] = defaultValue
		}
		data["_v"] = b.toVersion
		return data, nil
	}
	return b.Do(fn)
}

// RenameField renames a field while preserving its value.
//
// Use this when you want to change a field name for clarity or consistency.
// The old field is removed and its value is copied to the new field name.
//
// Examples:
//
//	// Rename price to total_amount
//	smarterbase.Migrate("Order").From(0).To(1).
//	    RenameField("price", "total_amount")
//
//	// Rename created to created_at for consistency
//	smarterbase.Migrate("Document").From(1).To(2).
//	    RenameField("created", "created_at")
//
//	// Before: {"id": "123", "price": 99.99}
//	// After:  {"id": "123", "total_amount": 99.99, "_v": 1}
func (b *MigrationBuilder) RenameField(oldName, newName string) *MigrationBuilder {
	fn := func(data map[string]interface{}) (map[string]interface{}, error) {
		if val, exists := data[oldName]; exists {
			data[newName] = val
			delete(data, oldName)
		}
		data["_v"] = b.toVersion
		return data, nil
	}
	return b.Do(fn)
}

// RemoveField removes a deprecated field from the data.
//
// Use this to clean up old fields that are no longer needed in your schema.
//
// Examples:
//
//	// Remove a legacy flag that's no longer used
//	smarterbase.Migrate("Config").From(1).To(2).
//	    RemoveField("legacy_feature_flag")
//
//	// Remove temporary migration field
//	smarterbase.Migrate("User").From(2).To(3).
//	    RemoveField("migration_temp_field")
//
//	// Before: {"id": "123", "name": "User", "legacy_flag": true}
//	// After:  {"id": "123", "name": "User", "_v": 2}
func (b *MigrationBuilder) RemoveField(field string) *MigrationBuilder {
	fn := func(data map[string]interface{}) (map[string]interface{}, error) {
		delete(data, field)
		data["_v"] = b.toVersion
		return data, nil
	}
	return b.Do(fn)
}

// Register adds a migration to the registry
func (r *MigrationRegistry) Register(typeName string, fromVersion, toVersion int, fn MigrationFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.migrations[typeName]; !exists {
		r.migrations[typeName] = make(map[int]map[int]MigrationFunc)
	}
	if _, exists := r.migrations[typeName][fromVersion]; !exists {
		r.migrations[typeName][fromVersion] = make(map[int]MigrationFunc)
	}
	r.migrations[typeName][fromVersion][toVersion] = fn
}

// Run executes migration chain from source to target version
func (r *MigrationRegistry) Run(typeName string, fromVersion, toVersion int, data []byte) ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// No migration needed
	if fromVersion == toVersion {
		return data, nil
	}

	// Parse JSON to map
	var dataMap map[string]interface{}
	if err := json.Unmarshal(data, &dataMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal for migration: %w", err)
	}

	// Find migration path
	path := r.findPath(typeName, fromVersion, toVersion)
	if path == nil {
		return nil, fmt.Errorf("no migration path from version %d to %d for type %s", fromVersion, toVersion, typeName)
	}

	// Execute migration chain
	current := dataMap
	for i := 0; i < len(path)-1; i++ {
		from := path[i]
		to := path[i+1]

		fn, exists := r.migrations[typeName][from][to]
		if !exists {
			return nil, fmt.Errorf("missing migration %s:%d→%d", typeName, from, to)
		}

		migrated, err := fn(current)
		if err != nil {
			return nil, fmt.Errorf("migration %s:%d→%d failed: %w", typeName, from, to, err)
		}
		current = migrated
	}

	// Marshal back to JSON
	return json.Marshal(current)
}

// findPath finds shortest migration path using BFS
func (r *MigrationRegistry) findPath(typeName string, from, to int) []int {
	if from == to {
		return []int{from}
	}

	typeMap, exists := r.migrations[typeName]
	if !exists {
		return nil
	}

	// BFS to find shortest path
	queue := [][]int{{from}}
	visited := make(map[int]bool)

	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]

		current := path[len(path)-1]
		if current == to {
			return path
		}

		if visited[current] {
			continue
		}
		visited[current] = true

		// Explore neighbors
		if neighbors, ok := typeMap[current]; ok {
			for next := range neighbors {
				if !visited[next] {
					newPath := append([]int{}, path...)
					newPath = append(newPath, next)
					queue = append(queue, newPath)
				}
			}
		}
	}

	return nil
}

// HasMigrations checks if any migrations are registered
func (r *MigrationRegistry) HasMigrations() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.migrations) > 0
}

// extractVersion gets version from JSON data
func extractVersion(data []byte) int {
	var versionMap struct {
		Version int `json:"_v"`
	}
	if err := json.Unmarshal(data, &versionMap); err != nil {
		return 0 // No version = version 0
	}
	return versionMap.Version
}

// extractExpectedVersion gets expected version from struct using reflection
func extractExpectedVersion(dest interface{}) int {
	val := reflect.ValueOf(dest)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return 0
	}

	// Look for _v or V field
	for i := 0; i < val.NumField(); i++ {
		field := val.Type().Field(i)
		jsonTag := field.Tag.Get("json")

		// Check if json tag is exactly "_v" (parse tag properly)
		if jsonTag != "" {
			tagParts := strings.Split(jsonTag, ",")
			tagName := strings.TrimSpace(tagParts[0])
			if tagName == "_v" {
				if val.Field(i).Kind() == reflect.Int {
					return int(val.Field(i).Int())
				}
			}
		}
	}

	return 0 // No version field = version 0
}

// getTypeName extracts type name from struct
func getTypeName(dest interface{}) string {
	t := reflect.TypeOf(dest)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// MigrationPolicy defines how migrations are applied when data is read from storage.
//
// The policy determines whether migrated data should be written back to storage
// or kept only in memory.
type MigrationPolicy int

const (
	// MigrateOnRead only migrates data in memory without writing back to storage (default).
	//
	// Use this policy for:
	//   - Production environments where you want to test migrations without modifying data
	//   - Read-heavy workloads where write-back would add unnecessary latency
	//   - Scenarios where you want to defer data upgrades
	//
	// Example:
	//
	//	store := smarterbase.NewStore(backend)
	//	// Data is migrated when read but not written back
	//	store.GetJSON(ctx, "users/123", &user)
	MigrateOnRead MigrationPolicy = iota

	// MigrateAndWrite migrates data and writes it back to storage with the new version.
	//
	// Use this policy for:
	//   - Gradual data upgrades during low-traffic periods
	//   - Ensuring all data is eventually upgraded to the latest version
	//   - When you want to measure migration success rates before forcing upgrades
	//
	// Example:
	//
	//	store := smarterbase.NewStore(backend)
	//	store.WithMigrationPolicy(smarterbase.MigrateAndWrite)
	//	// Data is migrated and written back to storage with updated version
	//	store.GetJSON(ctx, "users/123", &user)
	//
	// Performance note: Write-back adds latency (~10-50ms depending on backend)
	// but ensures data is upgraded over time as it's accessed.
	MigrateAndWrite
)
