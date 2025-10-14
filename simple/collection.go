package simple

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/adrianmcphee/smarterbase"
)

// Collection provides type-safe CRUD operations for a specific entity type.
// It uses generics to eliminate boilerplate and provide compile-time safety.
//
// Example:
//
//	type User struct {
//	    ID    string `json:"id" sb:"id"`
//	    Email string `json:"email" sb:"index,unique"`
//	    Name  string `json:"name"`
//	}
//
//	users := simple.NewCollection[User](db)
//	user, err := users.Create(ctx, &User{Email: "alice@example.com", Name: "Alice"})
type Collection[T any] struct {
	db        *DB
	name      string
	kb        smarterbase.KeyBuilder
	idField   string
	indexes   map[string]IndexSpec
	modelInfo *ModelInfo
}

// IndexSpec describes an index on a field.
type IndexSpec struct {
	Field  string
	Unique bool
}

// ModelInfo contains metadata about the model type.
type ModelInfo struct {
	Name      string
	IDField   string
	Indexes   map[string]IndexSpec
	Validated bool
}

// NewCollection creates a new type-safe collection.
// Collection name is inferred from type name (User -> "users").
// Override with explicit name: NewCollection[User](db, "customers")
//
// Example:
//
//	users := simple.NewCollection[User](db)  // Uses "users"
//	users := simple.NewCollection[User](db, "customers")  // Override
func NewCollection[T any](db *DB, name ...string) *Collection[T] {
	var t T
	typeName := getTypeName(t)

	collectionName := pluralize(typeName)
	if len(name) > 0 && name[0] != "" {
		collectionName = name[0]
	}

	c := &Collection[T]{
		db:      db,
		name:    collectionName,
		kb:      smarterbase.KeyBuilder{Prefix: collectionName, Suffix: ".json"},
		idField: "ID",
		indexes: make(map[string]IndexSpec),
	}

	// Parse struct tags and register indexes
	c.parseModelInfo()
	c.registerIndexes()

	return c
}

// Create stores a new item and returns a copy with ID populated.
// This is IMMUTABLE - the input is not modified.
//
// Example:
//
//	user := &User{Email: "alice@example.com", Name: "Alice"}
//	created, err := users.Create(ctx, user)
//	// created.ID is now set, original user unchanged
func (c *Collection[T]) Create(ctx context.Context, item *T) (*T, error) {
	if item == nil {
		return nil, fmt.Errorf("item cannot be nil")
	}

	// Create a copy to avoid mutating input
	created := c.copyItem(item)

	// Generate ID if not set
	id := c.getID(created)
	if id == "" {
		id = smarterbase.NewID()
		c.setID(created, id)
	}

	// Construct key
	key := c.kb.Key(id)

	// Store with index updates
	if err := c.db.indexManager.Create(ctx, key, created); err != nil {
		return nil, fmt.Errorf("failed to create: %w", err)
	}

	return created, nil
}

// Get retrieves an item by ID.
//
// Example:
//
//	user, err := users.Get(ctx, "user-123")
func (c *Collection[T]) Get(ctx context.Context, id string) (*T, error) {
	if id == "" {
		return nil, fmt.Errorf("id cannot be empty")
	}

	key := c.kb.Key(id)
	var item T

	if err := c.db.store.GetJSON(ctx, key, &item); err != nil {
		if smarterbase.IsNotFound(err) {
			return nil, fmt.Errorf("%s not found: %s", c.name, id)
		}
		return nil, err
	}

	return &item, nil
}

// Update updates an existing item.
// The item must have an ID field set.
//
// Example:
//
//	user.Name = "Alice Smith"
//	err := users.Update(ctx, user)
func (c *Collection[T]) Update(ctx context.Context, item *T) error {
	if item == nil {
		return fmt.Errorf("item cannot be nil")
	}

	id := c.getID(item)
	if id == "" {
		return fmt.Errorf("item must have ID set")
	}

	key := c.kb.Key(id)

	if err := c.db.indexManager.Update(ctx, key, item); err != nil {
		return fmt.Errorf("failed to update: %w", err)
	}

	return nil
}

// Delete removes an item by ID.
//
// Example:
//
//	err := users.Delete(ctx, "user-123")
func (c *Collection[T]) Delete(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id cannot be empty")
	}

	key := c.kb.Key(id)

	if err := c.db.indexManager.Delete(ctx, key); err != nil {
		return fmt.Errorf("failed to delete: %w", err)
	}

	return nil
}

// Find queries by indexed field and returns all matching items.
//
// Example:
//
//	users, err := users.Find(ctx, "role", "admin")
func (c *Collection[T]) Find(ctx context.Context, field, value string) ([]*T, error) {
	if c.db.redisIndexer == nil {
		return nil, fmt.Errorf("redis indexer not available - cannot query by field")
	}

	// Use the Core API helper
	return smarterbase.QueryIndexTyped[T](ctx, c.db.indexManager, c.name, field, value)
}

// FindOne queries by indexed field and returns the first match.
// Returns error if no items found.
//
// Example:
//
//	user, err := users.FindOne(ctx, "email", "alice@example.com")
func (c *Collection[T]) FindOne(ctx context.Context, field, value string) (*T, error) {
	if c.db.redisIndexer == nil {
		return nil, fmt.Errorf("redis indexer not available - cannot query by field")
	}

	// Use the Core API helper
	return smarterbase.GetByIndex[T](ctx, c.db.indexManager, c.name, field, value)
}

// Atomic performs an atomic read-modify-write operation with distributed locking.
// The function receives the current item and can modify it.
//
// Example:
//
//	err := users.Atomic(ctx, userID, 10*time.Second, func(user *User) error {
//	    user.Balance += 100
//	    return nil
//	})
func (c *Collection[T]) Atomic(ctx context.Context, id string, timeout time.Duration, fn func(*T) error) error {
	if id == "" {
		return fmt.Errorf("id cannot be empty")
	}

	if c.db.lock == nil {
		return fmt.Errorf("distributed lock not available - redis required")
	}

	key := c.kb.Key(id)

	return smarterbase.WithAtomicUpdate(ctx, c.db.store, c.db.lock, key, timeout, func(ctx context.Context) error {
		// Get current item
		var item T
		if err := c.db.store.GetJSON(ctx, key, &item); err != nil {
			return err
		}

		// Apply modifications
		if err := fn(&item); err != nil {
			return err
		}

		// Save with index updates
		return c.db.indexManager.Update(ctx, key, &item)
	})
}

// All returns all items in the collection.
// WARNING: Loads everything into memory. Use with caution.
//
// Example:
//
//	users, err := users.All(ctx)
func (c *Collection[T]) All(ctx context.Context) ([]*T, error) {
	var items []*T

	err := c.db.store.Query(c.name+"/").All(ctx, &items)
	if err != nil {
		return nil, fmt.Errorf("failed to query all: %w", err)
	}

	return items, nil
}

// Each iterates over all items without loading them into memory.
// The handler is called for each item. Return an error to stop iteration.
//
// Example:
//
//	err := users.Each(ctx, func(user *User) error {
//	    fmt.Println(user.Name)
//	    return nil
//	})
func (c *Collection[T]) Each(ctx context.Context, handler func(*T) error) error {
	return c.db.store.Query(c.name+"/").Each(ctx, func(key string, data []byte) error {
		var item T
		if err := json.Unmarshal(data, &item); err != nil {
			return fmt.Errorf("failed to unmarshal %s: %w", key, err)
		}

		return handler(&item)
	})
}

// Count returns the total number of items.
//
// Example:
//
//	count, err := users.Count(ctx)
func (c *Collection[T]) Count(ctx context.Context) (int, error) {
	count := 0
	err := c.db.store.Query(c.name+"/").Each(ctx, func(key string, data []byte) error {
		count++
		return nil
	})
	return count, err
}

// Helper methods

func (c *Collection[T]) parseModelInfo() {
	var t T
	typ := reflect.TypeOf(t)

	// Handle pointer types
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return
	}

	c.modelInfo = &ModelInfo{
		Name:    typ.Name(),
		Indexes: make(map[string]IndexSpec),
	}

	// Parse struct tags
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("sb")

		if tag == "" {
			continue
		}

		parts := strings.Split(tag, ",")

		// Check for id tag
		if parts[0] == "id" || contains(parts, "id") {
			c.idField = field.Name
			c.modelInfo.IDField = field.Name
		}

		// Check for index tags
		if parts[0] == "index" || contains(parts, "index") {
			jsonName := field.Tag.Get("json")
			if jsonName == "" {
				jsonName = strings.ToLower(field.Name)
			} else {
				// Extract just the name (before comma)
				if idx := strings.Index(jsonName, ","); idx >= 0 {
					jsonName = jsonName[:idx]
				}
			}

			spec := IndexSpec{
				Field:  jsonName,
				Unique: contains(parts, "unique"),
			}
			c.indexes[jsonName] = spec
			c.modelInfo.Indexes[jsonName] = spec
		}
	}

	c.modelInfo.Validated = true
}

func (c *Collection[T]) registerIndexes() {
	if c.db.redisIndexer == nil {
		return
	}

	for fieldName, spec := range c.indexes {
		indexName := fmt.Sprintf("%s-by-%s", c.name, fieldName)

		// Note: Both unique and multi-value indexes use MultiIndexSpec
		// Uniqueness constraints are enforced at application level
		_ = spec.Unique // Planned for future use

		c.db.redisIndexer.RegisterMultiIndex(&smarterbase.MultiIndexSpec{
			Name:        indexName,
			EntityType:  c.name,
			ExtractFunc: smarterbase.ExtractJSONField(fieldName),
		})
	}
}

func (c *Collection[T]) getID(item *T) string {
	val := reflect.ValueOf(item).Elem()
	field := val.FieldByName(c.idField)
	if !field.IsValid() {
		return ""
	}
	return field.String()
}

func (c *Collection[T]) setID(item *T, id string) {
	val := reflect.ValueOf(item).Elem()
	field := val.FieldByName(c.idField)
	if field.IsValid() && field.CanSet() {
		field.SetString(id)
	}
}

func (c *Collection[T]) copyItem(item *T) *T {
	// Marshal and unmarshal to create a deep copy
	data, err := json.Marshal(item)
	if err != nil {
		// Should never happen for valid structs
		panic(fmt.Sprintf("failed to marshal item: %v", err))
	}

	var copy T
	if err := json.Unmarshal(data, &copy); err != nil {
		// Should never happen if marshal succeeded
		panic(fmt.Sprintf("failed to unmarshal item: %v", err))
	}

	return &copy
}

func getTypeName(v interface{}) string {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

func pluralize(s string) string {
	// Simple pluralization rules
	lower := strings.ToLower(s)

	// Irregular plurals
	irregulars := map[string]string{
		"person": "people",
		"child":  "children",
		"goose":  "geese",
		"tooth":  "teeth",
		"foot":   "feet",
		"mouse":  "mice",
	}

	if plural, ok := irregulars[lower]; ok {
		return plural
	}

	// Words ending in 'y' (preceded by consonant) -> 'ies'
	if len(s) > 1 && s[len(s)-1] == 'y' {
		preceding := s[len(s)-2]
		if !isVowel(rune(preceding)) {
			return s[:len(s)-1] + "ies"
		}
	}

	// Words ending in s, x, z, ch, sh -> add 'es'
	if strings.HasSuffix(lower, "s") || strings.HasSuffix(lower, "x") ||
		strings.HasSuffix(lower, "z") || strings.HasSuffix(lower, "ch") ||
		strings.HasSuffix(lower, "sh") {
		return s + "es"
	}

	// Default: add 's'
	return s + "s"
}

func isVowel(r rune) bool {
	return r == 'a' || r == 'e' || r == 'i' || r == 'o' || r == 'u'
}

func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}
