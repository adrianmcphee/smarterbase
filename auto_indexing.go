package smarterbase

import (
	"fmt"
	"reflect"
	"strings"
)

// IndexTag represents parsed struct tag for automatic indexing
// Usage: Field string `json:"email" sb:"index:unique,name:users-by-email"`
type IndexTag struct {
	Type     string // "unique" or "multi"
	Name     string // index name (auto-generated if not provided)
	Optional bool   // if true, empty values don't error
}

// ParseIndexTag parses a struct tag for indexing configuration
// Supported formats:
//   - sb:"index" or sb:"index,multi" - creates Redis multi-index
//   - sb:"index,name:custom-name" - with custom name
//   - sb:"index,optional" - allows empty values
//
// Note: "unique" indexes are no longer supported - use Redis multi-indexes only
func ParseIndexTag(tag string) (*IndexTag, bool) {
	if tag == "" {
		return nil, false
	}

	// Support both "index:unique" and "index,unique" formats
	if !strings.Contains(tag, "index") {
		return nil, false
	}

	parts := strings.Split(tag, ",")

	// Check for and reject "unique" tags early
	for _, part := range parts {
		if strings.TrimSpace(part) == "unique" {
			return nil, false // Unique indexes no longer supported
		}
	}

	// Find index type
	var indexType string
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "index:") {
			extractedType := strings.TrimPrefix(part, "index:")
			if extractedType == "unique" {
				return nil, false // Reject unique
			}
			indexType = extractedType
		} else if part == "index" && i+1 < len(parts) {
			// Format: "index,multi"
			nextPart := strings.TrimSpace(parts[i+1])
			if nextPart == "multi" {
				indexType = nextPart
			}
		} else if i == 0 && part == "index" {
			// Simple "index" tag defaults to multi
			indexType = "multi"
		}
	}

	if indexType == "" {
		indexType = "multi" // default
	}

	it := &IndexTag{
		Type: indexType,
	}

	// Parse additional options
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "name:") {
			it.Name = strings.TrimPrefix(part, "name:")
		} else if part == "optional" {
			it.Optional = true
		}
	}

	return it, true
}

// AutoRegisterIndexes automatically registers indexes for a struct type based on struct tags
// Example usage:
//
//	type User struct {
//	    Email      string `json:"email" sb:"index"`
//	    PlatformID string `json:"platform_id" sb:"index"`
//	}
//	AutoRegisterIndexes(redisIndexer, "users", &User{})
func AutoRegisterIndexes(
	redisIndexer *RedisIndexer,
	entityType string,
	example interface{},
) error {
	t := reflect.TypeOf(example)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return fmt.Errorf("expected struct type, got %v", t.Kind())
	}

	// Iterate through struct fields
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		sbTag := field.Tag.Get("sb")

		indexTag, ok := ParseIndexTag(sbTag)
		if !ok {
			continue
		}

		jsonName := field.Tag.Get("json")
		if jsonName == "" {
			jsonName = field.Name
		} else {
			// Remove omitempty and other options
			jsonName = strings.Split(jsonName, ",")[0]
		}

		// Auto-generate index name if not provided
		if indexTag.Name == "" {
			indexTag.Name = fmt.Sprintf("%s-by-%s", entityType, strings.ReplaceAll(jsonName, "_", "-"))
		}

		// Only support multi (Redis) indexes
		if indexTag.Type != "multi" {
			return fmt.Errorf("unsupported index type '%s' for %s.%s - only 'multi' (Redis) indexes are supported", indexTag.Type, t.Name(), field.Name)
		}

		if redisIndexer == nil {
			return fmt.Errorf("Redis indexer required for index on %s.%s", t.Name(), field.Name)
		}

		err := registerMultiIndex(redisIndexer, indexTag.Name, entityType, jsonName)
		if err != nil {
			return fmt.Errorf("failed to register multi index for %s.%s: %w", t.Name(), field.Name, err)
		}
	}

	return nil
}

// registerMultiIndex creates a Redis-backed multi-value index
func registerMultiIndex(
	redisIndexer *RedisIndexer,
	indexName string,
	entityType string,
	jsonFieldName string,
) error {
	redisIndexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        indexName,
		EntityType:  entityType,
		ExtractFunc: ExtractJSONField(jsonFieldName),
	})
	return nil
}

// IndexConfig represents configuration for struct-based indexing
type IndexConfig struct {
	EntityType   string
	KeyPrefix    string
	RedisIndexer *RedisIndexer
}

// RegisterIndexesForType is a convenience function that combines auto-registration
// with manual index registration for complex cases
func RegisterIndexesForType(cfg IndexConfig, example interface{}, manualIndexes func()) error {
	// Auto-register from struct tags
	if err := AutoRegisterIndexes(cfg.RedisIndexer, cfg.EntityType, example); err != nil {
		return err
	}

	// Allow manual registration for complex indexes
	if manualIndexes != nil {
		manualIndexes()
	}

	return nil
}
