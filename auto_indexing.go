package smarterbase

import (
	"encoding/json"
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
//   - sb:"index:unique" or sb:"index,unique" - creates unique file-based index
//   - sb:"index:multi" or sb:"index,multi" - creates Redis multi-index
//   - sb:"index:unique,name:custom-name" - with custom name
//   - sb:"index:unique,optional" or sb:"index,unique,optional" - allows empty values
func ParseIndexTag(tag string) (*IndexTag, bool) {
	if tag == "" {
		return nil, false
	}

	// Support both "index:unique" and "index,unique" formats
	if !strings.Contains(tag, "index") {
		return nil, false
	}

	parts := strings.Split(tag, ",")

	// Find index type
	var indexType string
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "index:") {
			indexType = strings.TrimPrefix(part, "index:")
		} else if part == "index" && i+1 < len(parts) {
			// Format: "index,unique"
			nextPart := strings.TrimSpace(parts[i+1])
			if nextPart == "unique" || nextPart == "multi" {
				indexType = nextPart
			}
		} else if i == 0 && part == "index" {
			// Simple "index" tag defaults to multi
			indexType = "multi"
		}
	}

	if indexType == "" {
		// Default to unique if "unique" keyword found
		for _, part := range parts {
			if strings.TrimSpace(part) == "unique" {
				indexType = "unique"
				break
			}
		}
		if indexType == "" {
			indexType = "multi" // default
		}
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
//	    Email      string `json:"email" sb:"index:unique"`
//	    PlatformID string `json:"platform_id" sb:"index:unique"`
//	}
//	AutoRegisterIndexes(indexer, redisIndexer, "users", &User{})
func AutoRegisterIndexes(
	fileIndexer *Indexer,
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

		switch indexTag.Type {
		case "unique":
			if fileIndexer == nil {
				return fmt.Errorf("file indexer required for unique index on %s.%s", t.Name(), field.Name)
			}
			err := registerUniqueIndex(fileIndexer, indexTag.Name, entityType, jsonName, field.Name, example, indexTag.Optional)
			if err != nil {
				return fmt.Errorf("failed to register unique index for %s.%s: %w", t.Name(), field.Name, err)
			}

		case "multi":
			if redisIndexer == nil {
				// Silently skip if Redis not available (graceful degradation)
				continue
			}
			err := registerMultiIndex(redisIndexer, indexTag.Name, entityType, jsonName)
			if err != nil {
				return fmt.Errorf("failed to register multi index for %s.%s: %w", t.Name(), field.Name, err)
			}

		default:
			return fmt.Errorf("unknown index type '%s' for %s.%s", indexTag.Type, t.Name(), field.Name)
		}
	}

	return nil
}

// registerUniqueIndex creates a file-based unique index
func registerUniqueIndex(
	indexer *Indexer,
	indexName string,
	entityType string,
	jsonFieldName string,
	structFieldName string,
	example interface{},
	optional bool,
) error {
	exampleType := reflect.TypeOf(example)
	if exampleType.Kind() == reflect.Ptr {
		exampleType = exampleType.Elem()
	}

	indexer.RegisterIndex(&IndexSpec{
		Name: indexName,
		KeyFunc: func(data interface{}) (string, error) {
			v := reflect.ValueOf(data)
			if v.Kind() == reflect.Ptr {
				v = v.Elem()
			}

			field := v.FieldByName(structFieldName)
			if !field.IsValid() {
				return "", fmt.Errorf("field %s not found", structFieldName)
			}

			fieldValue := fmt.Sprintf("%v", field.Interface())
			if fieldValue == "" && !optional {
				return "", fmt.Errorf("%s has no %s", entityType, jsonFieldName)
			}

			return fieldValue, nil
		},
		ExtractFunc: func(data []byte) (interface{}, error) {
			// Create new instance of the type
			instance := reflect.New(exampleType).Interface()
			err := json.Unmarshal(data, instance)
			return instance, err
		},
		IndexKey: func(value string) string {
			return fmt.Sprintf("indexes/%s/%s.json", indexName, value)
		},
	})

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
	FileIndexer  *Indexer
	RedisIndexer *RedisIndexer
}

// RegisterIndexesForType is a convenience function that combines auto-registration
// with manual index registration for complex cases
func RegisterIndexesForType(cfg IndexConfig, example interface{}, manualIndexes func()) error {
	// Auto-register from struct tags
	if err := AutoRegisterIndexes(cfg.FileIndexer, cfg.RedisIndexer, cfg.EntityType, example); err != nil {
		return err
	}

	// Allow manual registration for complex indexes
	if manualIndexes != nil {
		manualIndexes()
	}

	return nil
}
