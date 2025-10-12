package smarterbase

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// MigrationFunc transforms data from one version to another
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

// Migrate starts building a migration for a type
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

// Split is a helper that splits a field by delimiter into multiple fields
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

// AddField adds a new field with a default value
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

// RenameField renames a field
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

// RemoveField removes a field
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

// MigrationPolicy defines how migrations are applied
type MigrationPolicy int

const (
	// MigrateOnRead only migrates data in memory (default)
	MigrateOnRead MigrationPolicy = iota
	// MigrateAndWrite migrates and writes back to storage
	MigrateAndWrite
)
